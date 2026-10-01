// Package diskio measures activity at the selected logical block-device layer.
package diskio

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	v2 "github.com/komari-monitor/komari-agent/protocol/v2"
)

var ErrUnsupported = errors.New("disk IO unsupported on this platform")

type Counter struct {
	Read, Write uint64
}
type Snapshot map[string]Counter
type Source interface {
	Read(time.Time) (Snapshot, error)
}

// Collector owns the baseline; keep one instance for the lifetime of the probe.
type Collector struct {
	mu       sync.Mutex
	source   Source
	now      func() time.Time
	interval time.Duration
	previous Snapshot
	at       time.Time
	disabled bool
}

func New(source Source, now func() time.Time, interval time.Duration, disabled bool) *Collector {
	if now == nil {
		now = time.Now
	}
	if interval < time.Second {
		interval = time.Second
	}
	return &Collector{source: source, now: now, interval: interval, disabled: disabled}
}

// ParseDevices accepts auto, disabled, or a comma-separated list of kernel names.
func ParseDevices(value string) ([]string, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "auto" {
		return nil, false, nil
	}
	if value == "disabled" {
		return nil, true, nil
	}
	names := strings.Split(value, ",")
	seen := map[string]bool{}
	for i, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || name == "auto" || name == "disabled" || seen[name] {
			return nil, false, fmt.Errorf("invalid or duplicate disk IO device %q", name)
		}
		seen[name] = true
		names[i] = name
	}
	return names, false, nil
}

func (c *Collector) Sample() (*v2.DiskIOReport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.disabled {
		return &v2.DiskIOReport{Status: "disabled"}, nil
	}
	now := c.now()
	current, err := c.source.Read(now)
	if err != nil || len(current) == 0 {
		c.previous = nil
		c.at = time.Time{}
		if errors.Is(err, ErrUnsupported) {
			return &v2.DiskIOReport{Status: "unsupported"}, nil
		}
		if err == nil {
			err = errors.New("no disk IO devices available")
		}
		return &v2.DiskIOReport{Status: "unavailable"}, err
	}
	elapsed := now.Sub(c.at)
	valid := c.previous != nil && len(c.previous) == len(current) && elapsed >= time.Millisecond && elapsed <= 5*c.interval
	var read, write float64
	for id, value := range current {
		previous, exists := c.previous[id]
		if !exists || value.Read < previous.Read || value.Write < previous.Write {
			valid = false
			continue
		}
		// Integer subtraction precedes conversion, preserving small deltas of large counters.
		read += float64(value.Read - previous.Read)
		write += float64(value.Write - previous.Write)
	}
	c.previous = make(Snapshot, len(current))
	for id, value := range current {
		c.previous[id] = value
	}
	c.at = now
	if !valid {
		return &v2.DiskIOReport{Status: "warming_up"}, nil
	}
	read /= elapsed.Seconds()
	write /= elapsed.Seconds()
	return &v2.DiskIOReport{Status: "ok", ReadBytesPerSec: &read, WriteBytesPerSec: &write, SampleIntervalMS: elapsed.Milliseconds()}, nil
}
