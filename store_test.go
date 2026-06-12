package main

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestBasicOperations tests the happy path for Get/Set/Delete.
func TestBasicOperations(t *testing.T) {
	store := NewStore(0)

	// Set
	if err := store.Set("name", "hermes", time.Minute); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	val, found := store.Get("name")
	if !found {
		t.Fatal("expected to find 'name'")
	}
	if val != "hermes" {
		t.Fatalf("expected 'hermes', got %v", val)
	}

	// Delete
	deleted := store.Delete("name")
	if !deleted {
		t.Fatal("expected delete to return true")
	}

	// Get after delete
	_, found = store.Get("name")
	if found {
		t.Fatal("expected 'name' to be gone after delete")
	}
}

// TestExpiration verifies that expired items return as misses.
func TestExpiration(t *testing.T) {
	store := NewStore(0)

	store.Set("short", "lived", 50*time.Millisecond)

	// Should exist immediately
	_, found := store.Get("short")
	if !found {
		t.Fatal("expected 'short' to exist immediately after set")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	_, found = store.Get("short")
	if found {
		t.Fatal("expected 'short' to be expired")
	}
}

// TestMaxKeys verifies the capacity limit.
func TestMaxKeys(t *testing.T) {
	store := NewStore(2)

	store.Set("a", 1, time.Minute)
	store.Set("b", 2, time.Minute)

	// Third key should fail
	err := store.Set("c", 3, time.Minute)
	if err == nil {
		t.Fatal("expected error when cache is full")
	}

	// Updating existing key should work
	err = store.Set("a", 99, time.Minute)
	if err != nil {
		t.Fatalf("updating existing key should not fail: %v", err)
	}
}

// TestStats verifies that stats counters are updated.
func TestStats(t *testing.T) {
	store := NewStore(0)

	store.Set("x", 1, time.Minute)
	store.Get("x")    // hit
	store.Get("nope") // miss
	store.Delete("x")

	snap := store.stats.Snapshot()
	if snap.Sets != 1 {
		t.Errorf("expected 1 set, got %d", snap.Sets)
	}
	if snap.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", snap.Hits)
	}
	if snap.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", snap.Misses)
	}
	if snap.Deletes != 1 {
		t.Errorf("expected 1 delete, got %d", snap.Deletes)
	}
}

// TestCleanup verifies that the cleanup function removes expired items.
func TestCleanup(t *testing.T) {
	store := NewStore(0)

	store.Set("alive", "yes", time.Hour)
	store.Set("dead", "soon", 50*time.Millisecond)

	time.Sleep(100 * time.Millisecond)

	removed := store.cleanup()
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}

	if store.Len() != 1 {
		t.Fatalf("expected 1 item remaining, got %d", store.Len())
	}

	_, found := store.Get("alive")
	if !found {
		t.Fatal("expected 'alive' to survive cleanup")
	}
}

// =============================================================
// RACE CONDITION TESTS
// Run with: go test -race -run TestRace -v ./...
// These tests are designed to DETECT races, not necessarily pass
// cleanly without the -race flag.
// =============================================================

// TestRaceConcurrentReadsAndWrites hammers the store with concurrent
// readers and writers. Without proper locking, this will panic.
func TestRaceConcurrentReadsAndWrites(t *testing.T) {
	store := NewStore(0)
	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", n%10) // force key collisions
			store.Set(key, n, 100*time.Millisecond)
		}(i)
	}

	// Readers (at the same time)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", n%10)
			store.Get(key)
		}(i)
	}

	wg.Wait()
}

// TestRaceCleanupVsReads runs the cleanup goroutine while reads happen.
// This simulates the real-world scenario where the reaper deletes
// keys at the exact moment handlers are reading them.
func TestRaceCleanupVsReads(t *testing.T) {
	store := NewStore(0)

	// Fill with short-lived items
	for i := 0; i < 100; i++ {
		store.Set(fmt.Sprintf("k%d", i), i, 50*time.Millisecond)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start cleanup in background (like the real server does)
	go StartCleanup(ctx, store, 10*time.Millisecond)

	// Hammer reads while cleanup runs
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				store.Get(fmt.Sprintf("k%d", n))
			}
		}(i)
	}

	wg.Wait()
}

// TestRaceStatsContention checks that atomic counters work correctly
// under heavy concurrent updates.
func TestRaceStatsContention(t *testing.T) {
	store := NewStore(0)
	var wg sync.WaitGroup

	// All goroutines increment the same counter
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				store.stats.Hits.Add(1)
			}
		}()
	}

	wg.Wait()

	expected := int64(100 * 1000)
	if store.stats.Hits.Load() != expected {
		t.Fatalf("expected %d hits, got %d", expected, store.stats.Hits.Load())
	}
}

// TestRaceDeleteDuringCleanup deletes keys while cleanup is running.
// Both operations acquire a write lock — this tests for deadlocks.
func TestRaceDeleteDuringCleanup(t *testing.T) {
	store := NewStore(0)

	for i := 0; i < 50; i++ {
		store.Set(fmt.Sprintf("d%d", i), i, 30*time.Millisecond)
	}

	var wg sync.WaitGroup

	// Cleanup goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			store.cleanup()
			time.Sleep(time.Millisecond)
		}
	}()

	// Delete goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			store.Delete(fmt.Sprintf("d%d", i))
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
}
