package agent

import (
	"testing"
	"time"
	v2 "github.com/komari-monitor/komari/protocol/v2"
)

func TestDiskIOCacheOwnsItsSamples(t *testing.T) {
	const uuid = "io-cache-copy"
	defer DeleteLatestReport(uuid)
	read, write := 1.0, 2.0
	RecordReport(v2.Report{UUID:uuid, UpdatedAt:time.Now(), DiskIO:&v2.DiskIOReport{Status:"ok", ReadBytesPerSec:&read, WriteBytesPerSec:&write, SampleIntervalMS:2000}})
	read = 999
	latest := GetLatestReport()[uuid]
	if *latest.DiskIO.ReadBytesPerSec != 1 { t.Fatal("collector mutated latest") }
	*latest.DiskIO.ReadBytesPerSec = 888
	recent := GetRecentReports(uuid)
	if *recent[0].DiskIO.ReadBytesPerSec != 1 { t.Fatal("latest caller mutated recent") }
	*recent[0].DiskIO.WriteBytesPerSec = 777
	if *GetRecentReports(uuid)[0].DiskIO.WriteBytesPerSec != 2 { t.Fatal("recent caller mutated cache") }
}
