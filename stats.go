package main

import "sync/atomic"

// Stats tracks cache operation counters using atomic operations.
// This avoids mutex contention for simple counter increments.
type Stats struct {
	Hits      atomic.Int64 `json:"hits"`
	Misses    atomic.Int64 `json:"misses"`
	Sets      atomic.Int64 `json:"sets"`
	Deletes   atomic.Int64 `json:"deletes"`
	Evictions atomic.Int64 `json:"evictions"`
}

// Snapshot returns a point-in-time copy of the stats.
// Safe to call concurrently — each read is atomic.
func (s *Stats) Snapshot() StatsSnapshot {
	return StatsSnapshot{
		Hits:      s.Hits.Load(),
		Misses:    s.Misses.Load(),
		Sets:      s.Sets.Load(),
		Deletes:   s.Deletes.Load(),
		Evictions: s.Evictions.Load(),
	}
}

// StatsSnapshot is a plain struct copy of Stats for JSON serialization.
type StatsSnapshot struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Sets      int64 `json:"sets"`
	Deletes   int64 `json:"deletes"`
	Evictions int64 `json:"evictions"`
}
