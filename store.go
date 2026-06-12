package main

import (
	"fmt"
	"sync"
	"time"
)

// Store is the core thread-safe in-memory cache.
// It uses a RWMutex to protect concurrent access to the items map.
//
// RACE CONDITION HOTSPOTS (intentional for learning):
//   - The items map is read/written from multiple goroutines
//   - The stats counters are updated from request goroutines
//   - The cleanup goroutine deletes keys while handlers read/write
//
// Run `go test -race ./...` to detect data races.
type Store struct {
	mu      sync.RWMutex
	items   map[string]CacheItem
	stats   Stats
	maxKeys int // 0 = unlimited
}

// NewStore creates a new cache store.
// maxKeys: maximum number of keys (0 for unlimited).
func NewStore(maxKeys int) *Store {
	return &Store{
		items:   make(map[string]CacheItem),
		maxKeys: maxKeys,
	}
}

// Get retrieves a value by key.
// Returns the value and true if found and not expired.
// Returns nil and false if missing or expired.
func (s *Store) Get(key string) (any, bool) {
	s.mu.RLock()
	item, exists := s.items[key]
	s.mu.RUnlock()

	if !exists {
		s.stats.Misses.Add(1)
		return nil, false
	}

	if item.IsExpired() {
		// Expired but not yet cleaned up — count as miss.
		// NOTE: we don't delete here to keep the RLock path simple.
		// The cleanup goroutine handles deletion.
		s.stats.Misses.Add(1)
		return nil, false
	}

	s.stats.Hits.Add(1)
	return item.Value, true
}

// Set stores a value with the given TTL.
// Returns an error if the cache is at max capacity.
func (s *Store) Set(key string, value any, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check capacity (only if key doesn't already exist)
	if s.maxKeys > 0 {
		if _, exists := s.items[key]; !exists && len(s.items) >= s.maxKeys {
			return fmt.Errorf("cache full: max %d keys reached", s.maxKeys)
		}
	}

	s.items[key] = CacheItem{
		Value:     value,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	}

	s.stats.Sets.Add(1)
	return nil
}

// Delete removes a key from the cache.
// Returns true if the key existed, false otherwise.
func (s *Store) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.items[key]
	if exists {
		delete(s.items, key)
		s.stats.Deletes.Add(1)
	}
	return exists
}

// Keys returns a list of all non-expired keys.
func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.items))
	for k, item := range s.items {
		if !item.IsExpired() {
			keys = append(keys, k)
		}
	}
	return keys
}

// Len returns the number of items (including expired but not yet cleaned).
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// cleanup removes all expired items from the store.
// Called by the background reaper goroutine.
func (s *Store) cleanup() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed := 0
	for key, item := range s.items {
		if item.IsExpired() {
			delete(s.items, key)
			removed++
		}
	}

	s.stats.Evictions.Add(int64(removed))
	return removed
}
