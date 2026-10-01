package metricstore

import (
	"context"
	"testing"
	"time"

	"github.com/komari-monitor/komari/pkg/metric"
	v2 "github.com/komari-monitor/komari/protocol/v2"
)

func TestDiskIOPersistenceMissingZeroAndCleanup(t *testing.T) {
	s := useReportTestStore(t, nil)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Minute).Add(-2*time.Minute)
	zero, read, write := 0.0, 1048576.0, 524288.0
	reports := []v2.Report{
		{UUID:"io-node", UpdatedAt:base.Add(-time.Minute)},
		{UUID:"io-node", UpdatedAt:base.Add(-time.Minute+time.Second), DiskIO:&v2.DiskIOReport{Status:"warming_up"}},
		{UUID:"io-node", UpdatedAt:base.Add(2*time.Second), DiskIO:&v2.DiskIOReport{Status:"ok", ReadBytesPerSec:&read, WriteBytesPerSec:&write, SampleIntervalMS:2000}},
		{UUID:"io-node", UpdatedAt:base.Add(4*time.Second), DiskIO:&v2.DiskIOReport{Status:"ok", ReadBytesPerSec:&zero, WriteBytesPerSec:&zero, SampleIntervalMS:2000}},
		{UUID:"io-node", UpdatedAt:base.Add(6*time.Second), DiskIO:&v2.DiskIOReport{Status:"unavailable"}},
		// Legacy records use one-minute rollups. A separate idle bucket must
		// remain zero rather than being averaged with the earlier busy sample.
		{UUID:"io-node", UpdatedAt:base.Add(time.Minute+2*time.Second), DiskIO:&v2.DiskIOReport{Status:"ok", ReadBytesPerSec:&zero, WriteBytesPerSec:&zero, SampleIntervalMS:2000}},
	}
	if _, err := writeReportBatch(ctx, reports); err != nil { t.Fatal(err) }
	start, end := base.Add(-time.Minute-time.Second), base.Add(time.Minute+10*time.Second)
	assertMetricValues(t, s, MetricDiskIORead, "io-node", start, end, []float64{read, zero, zero})
	assertMetricValues(t, s, MetricDiskIOWrite, "io-node", start, end, []float64{write, zero, zero})
	assertMetricAggregate(t, s, MetricDiskIORead, "io-node", start, base.Add(10*time.Second), metric.AggAvg, read/2, 2)
	reconstructed, err := GetRecordsByClientAndTime(ctx, "io-node", start, end)
	if err != nil { t.Fatal(err) }
	var hasMissing, hasIdle bool
	for _, record := range reconstructed {
		if record.DiskReadRate == nil { hasMissing = true }
		if record.DiskReadRate != nil && *record.DiskReadRate == 0 { hasIdle = true }
	}
	if !hasMissing || !hasIdle { t.Fatalf("legacy records lost missing/zero distinction: %+v", reconstructed) }
	for _, name := range []string{MetricDiskIORead, MetricDiskIOWrite} {
		def, err := s.GetMetric(ctx, name); if err != nil || def.Type != metric.TypeGauge || def.Unit != "bytes/s" { t.Fatalf("bad IO definition: %+v %v", def, err) }
	}
	if err := DeleteAllRecords(ctx); err != nil { t.Fatal(err) }
	assertMetricValues(t, s, MetricDiskIORead, "io-node", start, end, nil)
	assertMetricValues(t, s, MetricDiskIOWrite, "io-node", start, end, nil)
	if _, err := writeReportBatch(ctx, reports[2:3]); err != nil { t.Fatal(err) }
	if err := DeleteEntity(ctx, "io-node"); err != nil { t.Fatal(err) }
	assertMetricValues(t, s, MetricDiskIORead, "io-node", start, end, nil)
}

func TestDiskIOUpgradeKeepsDisabledPersistence(t *testing.T) {
	s := useReportTestStore(t, nil); ctx := context.Background()
	// Simulate an old installation with saving disabled and no IO definitions.
	for _, name := range builtinMetricNames {
		if name == MetricDiskIORead || name == MetricDiskIOWrite { continue }
		def, err := s.GetMetric(ctx, name); if err != nil { t.Fatal(err) }
		def.RetentionDays = 0; if err := s.UpsertMetric(ctx, def); err != nil { t.Fatal(err) }
	}
	// Remove only the two newly introduced definitions from this isolated database.
	if err := s.DeleteMetric(ctx, MetricDiskIORead); err != nil { t.Fatal(err) }
	if err := s.DeleteMetric(ctx, MetricDiskIOWrite); err != nil { t.Fatal(err) }
	if err := createMetricDefinitions(ctx, s); err != nil { t.Fatal(err) }
	for _, name := range []string{MetricDiskIORead, MetricDiskIOWrite} {
		def, err := s.GetMetric(ctx, name); if err != nil || def.RetentionDays != 0 { t.Fatalf("upgrade enabled saving: %+v %v", def, err) }
	}
}
