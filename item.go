package main

import "time"

// CacheItem represents a single entry in the cache.
// It holds the value, when it was created, and when it expires.
type CacheItem struct {
	Value     any       `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsExpired checks whether this item has passed its TTL.
func (i CacheItem) IsExpired() bool {
	return time.Now().After(i.ExpiresAt)
}

// TTLRemaining returns how long until this item expires.
// Returns 0 if already expired.
func (i CacheItem) TTLRemaining() time.Duration {
	remaining := time.Until(i.ExpiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}
