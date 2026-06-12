package main

import (
	"context"
	"log"
	"time"
)

// StartCleanup runs a background goroutine that periodically
// removes expired items from the store.
//
// This goroutine is a major source of race conditions:
//   - It acquires a WRITE lock on the items map
//   - Meanwhile, request handlers hold READ locks
//   - If cleanup runs too frequently, it causes lock contention
//   - If a GET and DELETE happen at the exact same time for the same key,
//     the result depends on lock ordering
//
// The context is used for graceful shutdown.
func StartCleanup(ctx context.Context, store *Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[cleanup] started — running every %v", interval)

	for {
		select {
		case <-ctx.Done():
			log.Println("[cleanup] stopped")
			return
		case <-ticker.C:
			removed := store.cleanup()
			if removed > 0 {
				log.Printf("[cleanup] removed %d expired items", removed)
			}
		}
	}
}
