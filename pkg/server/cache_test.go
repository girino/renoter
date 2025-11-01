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

	// Add old events
	oldTime := now.Add(-2 * time.Hour)
	cache.CheckAndMark("old1", oldTime)
	cache.CheckAndMark("old2", oldTime)

	// Add new events
	cache.CheckAndMark("new1", now)
	cache.CheckAndMark("new2", now.Add(-30*time.Minute))

	initialSize := cache.Size()

	// Trigger cleanup by checking a new event
	cache.CheckAndMark("new3", now)

	// Old events should be cleaned up
	finalSize := cache.Size()
	if finalSize >= initialSize {
		t.Errorf("Cache should have removed old events, size before: %d, after: %d", initialSize, finalSize)
	}

	// Old events should not be in cache anymore
	if !cache.CheckAndMark("old1", now) {
		t.Error("Old event should have been removed from cache")
	}

	// New events should still be in cache
	if cache.CheckAndMark("new1", now) {
		t.Error("New event should still be in cache")
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

