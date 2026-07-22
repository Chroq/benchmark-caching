package tsid

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"time"
)

// Epoch is the TSID custom epoch (2020-01-01T00:00:00Z = 1577836800000 ms).
const Epoch int64 = 1577836800000

var counter uint64

// New generates a 64-bit Time-Sortable ID (TSID) as an int64.
// High 42 bits: timestamp in ms since custom epoch.
// Low 22 bits: sequence counter & random bits.
func New() int64 {
	ms := time.Now().UnixMilli() - Epoch
	if ms < 0 {
		ms = 0
	}
	c := atomic.AddUint64(&counter, 1) & 0x3FFFFF
	return (ms << 22) | int64(c)
}

// ParseInt parses a decimal string TSID into an int64.
func ParseInt(s string) (int64, error) {
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid TSID integer string %q: %w", s, err)
	}
	return val, nil
}

// FormatInt converts an int64 TSID to a 64-bit numeric string representation.
func FormatInt(id int64) string {
	return strconv.FormatInt(id, 10)
}
