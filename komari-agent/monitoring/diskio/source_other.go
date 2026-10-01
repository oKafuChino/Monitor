//go:build !linux

package diskio

import "time"

type unsupportedSource struct{}
func NewSource([]string) Source { return unsupportedSource{} }
func NewSourceAt([]string, string, string) Source { return unsupportedSource{} }
func (unsupportedSource) Read(time.Time) (Snapshot, error) { return nil, ErrUnsupported }
func ValidateConfiguration(Source) error { return nil }
