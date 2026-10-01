package monitoring

import (
	"testing"
	"time"
)

func TestLatestReportNeverReplaysOrQueues(t *testing.T) {
	latestSample.Lock()
	latestSample.report = []byte(`{"disk_io":{"status":"warming_up"}}`)
	latestSample.at = time.Now(); latestSample.taken = time.Time{}; latestSample.interval = time.Second
	latestSample.Unlock()
	data, ok := LatestReport(); if !ok { t.Fatal("fresh report unavailable") }; data[0] = 'x'
	if _, ok := LatestReport(); ok { t.Fatal("sample was replayed") }
	latestSample.Lock(); latestSample.at = time.Now().Add(-3*time.Second); latestSample.taken = time.Time{}; latestSample.Unlock()
	if _, ok := LatestReport(); ok { t.Fatal("stale report sent") }
	latestSample.Lock(); latestSample.report = []byte(`{"latest":true}`); latestSample.at = time.Now(); latestSample.Unlock()
	data, ok = LatestReport(); if !ok || string(data) != `{"latest":true}` { t.Fatal("newest report not retained") }
}
