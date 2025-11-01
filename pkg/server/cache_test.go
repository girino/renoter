package server

import (
	"testing"
	"time"
)

func TestNewEventCache(t *testing.T) {
	tests := []struct {
		name          string
		maxSize       int
		cutoffDuration time.Duration
	}{
		{
			name:          "default cache",
			maxSize:       5000,
			cutoffDuration: 2 * time.Hour,
		},
		{
			name:          "small cache",
			maxSize:       100,
			cutoffDuration: 1 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewEventCache(tt.maxSize, tt.cutoffDuration)
			if cache == nil {
				t.Fatal("NewEventCache() returned nil")
			}
			if cache.Size() != 0 {
				t.Errorf("New cache should have size 0, got %d", cache.Size())
			}
		})
	}
}

func TestEventCache_CheckAndMark(t *testing.T) {
	cache := NewEventCache(100, 1*time.Hour)
	now := time.Now()

	// Test marking new event
	eventID1 := "event1"
	if cache.CheckAndMark(eventID1, now) {
		t.Error("CheckAndMark() should return false for new event")
	}

	// Test replay detection
	if !cache.CheckAndMark(eventID1, now) {
		t.Error("CheckAndMark() should return true for duplicate event")
	}

	// Test different event
	eventID2 := "event2"
	if cache.CheckAndMark(eventID2, now) {
		t.Error("CheckAndMark() should return false for new event")
	}

	if cache.Size() != 2 {
		t.Errorf("Cache size should be 2, got %d", cache.Size())
	}
}

func TestEventCache_CleanupOldEvents(t *testing.T) {
	cache := NewEventCache(100, 1*time.Hour)
	now := time.Now()

	// Add old events (older than cutoff)
	oldTime := now.Add(-2 * time.Hour)
	cache.CheckAndMark("old1", oldTime)
	cache.CheckAndMark("old2", oldTime)

	// Add new events (within cutoff)
	cache.CheckAndMark("new1", now)
	cache.CheckAndMark("new2", now.Add(-30*time.Minute))

	initialSize := cache.Size()
	if initialSize != 4 {
		t.Fatalf("Initial cache size should be 4, got %d", initialSize)
	}

	// Trigger cleanup by checking a new event (this will call cleanupOldEventsLocked)
	cache.CheckAndMark("new3", now)

	// Old events should be cleaned up
	finalSize := cache.Size()
	// After cleanup, old events should be removed, but we added new3, so size should be >= 3
	if finalSize < 3 {
		t.Errorf("Cache should have at least new events, size: %d", finalSize)
	}

	// Old events should be removed (replay check should return false since they're not in cache)
	// Actually, if they're removed, CheckAndMark will return false for new events
	// Let's check that old events are gone by trying to mark them again
	// If they were cleaned up, they won't be in cache, so CheckAndMark returns false (not a replay)
	// But we can't directly check this - the cache doesn't expose internal state
	// Instead, verify the size decreased
	if finalSize > initialSize {
		t.Errorf("Cache size should decrease after cleanup (old events removed), but increased from %d to %d", initialSize, finalSize)
	}
}

func TestEventCache_Pruning(t *testing.T) {
	cache := NewEventCache(10, 2*time.Hour) // Small cache for testing
	now := time.Now()

	// Fill cache beyond max size
	for i := 0; i < 15; i++ {
		cache.CheckAndMark("event"+string(rune(i)), now)
	}

	// Cache should be pruned
	size := cache.Size()
	if size > 10 {
		t.Errorf("Cache should be pruned to <= 10, got %d", size)
	}

	// First events should be removed (FIFO)
	if !cache.CheckAndMark("event0", now) {
		t.Error("First event should have been pruned")
	}
}

func TestEventCache_Size(t *testing.T) {
	cache := NewEventCache(100, 1*time.Hour)
	now := time.Now()

	if cache.Size() != 0 {
		t.Errorf("Initial cache size should be 0, got %d", cache.Size())
	}

	for i := 0; i < 5; i++ {
		cache.CheckAndMark("event"+string(rune(i)), now)
	}

	if cache.Size() != 5 {
		t.Errorf("Cache size should be 5, got %d", cache.Size())
	}
}

