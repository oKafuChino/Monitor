package diskio

import (
	"errors"
	"testing"
	"time"
)

type fakeSource struct { values Snapshot; err error }
func (s *fakeSource) Read(time.Time) (Snapshot, error) { return s.values, s.err }

func TestRatesAndReset(t *testing.T) {
	now := time.Now()
	source := &fakeSource{values: Snapshot{"8:0": {Read: 1<<60, Write: 1<<60}}}
	c := New(source, func() time.Time { return now }, 2*time.Second, false)
	io, err := c.Sample(); if err != nil || io.Status != "warming_up" { t.Fatalf("first sample: %+v %v", io, err) }
	now = now.Add(2*time.Second)
	source.values["8:0"] = Counter{Read: (1<<60)+(2<<20), Write: (1<<60)+(1<<20)}
	io, err = c.Sample()
	if err != nil || io.Status != "ok" || *io.ReadBytesPerSec != 1<<20 || *io.WriteBytesPerSec != 1<<19 || io.SampleIntervalMS != 2000 { t.Fatalf("rates: %+v %v", io, err) }
	now = now.Add(4*time.Second)
	source.values["8:0"] = Counter{Read: (1<<60)+(4<<20), Write: (1<<60)+(2<<20)}
	io, _ = c.Sample(); if *io.ReadBytesPerSec != 1<<19 || *io.WriteBytesPerSec != 1<<18 { t.Fatal("rate ignored actual interval") }
	now = now.Add(2*time.Second)
	io, _ = c.Sample(); if io.Status != "ok" || *io.ReadBytesPerSec != 0 || *io.WriteBytesPerSec != 0 { t.Fatal("idle is not a real zero") }
	for _, anomaly := range []string{"reset", "replacement", "hotplug", "pause", "backward", "failure"} {
		t.Run(anomaly, func(t *testing.T) {
			now = now.Add(2*time.Second)
			switch anomaly {
			case "reset": source.values["8:0"] = Counter{}
			case "replacement": source.values = Snapshot{"8:1": {Read: 100, Write: 100}}
			case "hotplug": source.values["8:2"] = Counter{}
			case "pause": now = now.Add(20*time.Second)
			case "backward": now = now.Add(-4*time.Second)
			case "failure": source.err = errors.New("permission denied")
			}
			io, err := c.Sample()
			if anomaly == "failure" {
				if io.Status != "unavailable" || err == nil { t.Fatal("expected unavailable") }
				source.err = nil; now = now.Add(2*time.Second); io, _ = c.Sample()
			}
			if io.Status != "warming_up" || io.ReadBytesPerSec != nil || io.WriteBytesPerSec != nil { t.Fatalf("bad anomaly baseline: %+v", io) }
			now = now.Add(2*time.Second); io, err = c.Sample()
			if err != nil || io.Status != "ok" || *io.ReadBytesPerSec != 0 { t.Fatalf("bad recovery: %+v %v", io, err) }
		})
	}
}

func TestDisabledUnsupportedAndConfiguration(t *testing.T) {
	io, err := New(nil, nil, time.Second, true).Sample()
	if err != nil || io.Status != "disabled" { t.Fatal("disabled collector read source") }
	io, err = New(&fakeSource{err: ErrUnsupported}, nil, time.Second, false).Sample()
	if err != nil || io.Status != "unsupported" { t.Fatal("bad unsupported status") }
	for _, value := range []string{"sda,sda", "../sda", "sda,", "auto,sda", "disabled,sda"} {
		if _, _, err := ParseDevices(value); err == nil { t.Fatalf("accepted %q", value) }
	}
	devices, disabled, err := ParseDevices(" nvme0n1, vda ")
	if err != nil || disabled || len(devices) != 2 || devices[0] != "nvme0n1" { t.Fatal("bad explicit configuration") }
}
