package v2

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestDiskIOContract(t *testing.T) {
	zero, negative, nan, inf := 0.0, -1.0, math.NaN(), math.Inf(1)
	tests := []struct { io *DiskIOReport; valid bool }{
		{nil, true}, {&DiskIOReport{Status:"ok", ReadBytesPerSec:&zero, WriteBytesPerSec:&zero, SampleIntervalMS:2000}, true},
		{&DiskIOReport{Status:"warming_up"}, true}, {&DiskIOReport{Status:"unsupported"}, true},
		{&DiskIOReport{Status:"unavailable"}, true}, {&DiskIOReport{Status:"disabled"}, true},
		{&DiskIOReport{Status:"ok"}, false}, {&DiskIOReport{Status:"ok", ReadBytesPerSec:&negative, WriteBytesPerSec:&zero, SampleIntervalMS:2000}, false},
		{&DiskIOReport{Status:"ok", ReadBytesPerSec:&nan, WriteBytesPerSec:&zero, SampleIntervalMS:2000}, false},
		{&DiskIOReport{Status:"ok", ReadBytesPerSec:&zero, WriteBytesPerSec:&inf, SampleIntervalMS:2000}, false},
		{&DiskIOReport{Status:"disabled", ReadBytesPerSec:&zero}, false}, {&DiskIOReport{Status:"invalid"}, false},
	}
	for _, tt := range tests { if got := tt.io.Validate() == nil; got != tt.valid { t.Fatalf("validation %+v = %v", tt.io, got) } }
	old, _ := json.Marshal(Report{}); if strings.Contains(string(old), "disk_io") { t.Fatal("old report acquired IO") }
	io := &DiskIOReport{Status:"ok", ReadBytesPerSec:&zero, WriteBytesPerSec:&zero, SampleIntervalMS:2000}
	copy := io.Clone(); *copy.ReadBytesPerSec = 10; if *io.ReadBytesPerSec != 0 { t.Fatal("clone shares mutable rates") }
	encoded, _ := json.Marshal(io); if !strings.Contains(string(encoded), `"read_bytes_per_sec":0`) { t.Fatal("idle zero omitted") }
}
