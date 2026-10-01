package monitoring

import (
	"context"
	"sync"
	"time"
)

var latestSample struct {
	sync.RWMutex
	report   []byte
	at       time.Time
	taken    time.Time
	interval time.Duration
}

// StartReports is the single sampling loop. Network retries read only the latest
// immutable sample, so disconnections never freeze the IO baseline or build queues.
func StartReports(ctx context.Context, interval time.Duration) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	latestSample.Lock()
	latestSample.report = nil
	latestSample.taken = time.Time{}
	latestSample.interval = interval
	latestSample.Unlock()
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			data := GenerateReport()
			latestSample.Lock()
			latestSample.report = data
			latestSample.at = time.Now()
			latestSample.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func LatestReport() ([]byte, bool) {
	latestSample.Lock()
	defer latestSample.Unlock()
	if len(latestSample.report) == 0 || latestSample.taken == latestSample.at || time.Since(latestSample.at) > 2*latestSample.interval {
		return nil, false
	}
	latestSample.taken = latestSample.at
	return append([]byte(nil), latestSample.report...), true
}
