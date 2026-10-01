package v2

// DiskIOReport matches the server's optional disk_io contract, in bytes/s.
type DiskIOReport struct {
	Status           string   `json:"status"`
	ReadBytesPerSec   *float64 `json:"read_bytes_per_sec,omitempty"`
	WriteBytesPerSec  *float64 `json:"write_bytes_per_sec,omitempty"`
	SampleIntervalMS int64    `json:"sample_interval_ms"`
}
