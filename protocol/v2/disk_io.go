package v2

import (
	"fmt"
	"math"
)

// Validate accepts old probes (nil), but rejects partial or contradictory samples.
func (d *DiskIOReport) Validate() error {
	if d == nil {
		return nil
	}
	switch d.Status {
	case "ok":
		if d.SampleIntervalMS <= 0 || d.ReadBytesPerSec == nil || d.WriteBytesPerSec == nil {
			return fmt.Errorf("disk_io ok requires both rates and a positive sample_interval_ms")
		}
		for _, value := range []*float64{d.ReadBytesPerSec, d.WriteBytesPerSec} {
			if math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 {
				return fmt.Errorf("disk_io rates must be finite and non-negative")
			}
		}
	case "warming_up", "unsupported", "unavailable", "disabled":
		if d.ReadBytesPerSec != nil || d.WriteBytesPerSec != nil || d.SampleIntervalMS != 0 {
			return fmt.Errorf("disk_io without a valid sample must omit rates and use a zero interval")
		}
	default:
		return fmt.Errorf("invalid disk_io status")
	}
	return nil
}
