package server

import (
	"fmt"
	"testing"
	"time"
)

func TestNewEventCache(t *testing.T) {
	tests := []struct {
		name           string
		maxSize        int
		cutoffDuration time.Duration
	}{
		{
			name:           "default cache",
			maxSize:        5000,
			cutoffDuration: 2 * time.Hour,
		},
		{
			name:           "small cache",
			maxSize:        100,
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

	// When we add new1, cleanup runs and should remove old1 and old2 (they're > 1 hour old)
	// But cleanup runs BEFORE checking/marking, so old events are still there when we check
	// The cleanup happens during CheckAndMark, so old events are removed
	cache.CheckAndMark("new1", now)

	// Verify old events were cleaned up - they should NOT be detected as replays
	// because they were removed during cleanup
	if cache.CheckAndMark("old1", now) {
		t.Error("Old event 'old1' should have been cleaned up")
	}
	if cache.CheckAndMark("old2", now) {
		t.Error("Old event 'old2' should have been cleaned up")
	}

	// Verify new event is in cache (replay detected)
	if !cache.CheckAndMark("new1", now) {
		t.Error("New event 'new1' should be in cache")
	}
}

func TestEventCache_Pruning(t *testing.T) {
	cache := NewEventCache(10, 2*time.Hour) // Small cache for testing
	now := time.Now()

	// Fill cache to exactly max size
	for i := 0; i < 10; i++ {
		eventID := fmt.Sprintf("event%d", i) // Use fmt.Sprintf for reliable string conversion
		cache.CheckAndMark(eventID, now)
	}

	if cache.Size() != 10 {
		t.Fatalf("Cache should have 10 events, got %d", cache.Size())
	}

	// Add one more event to trigger pruning (prune happens at >= maxSize)
	// Pruning removes 25% (10/4 = 2 events), then adds the new event
	cache.CheckAndMark("eventX", now)

	// Cache should be pruned - 25% removed (2 events), so 8 old + 1 new = 9
	size := cache.Size()
	expectedSize := 10 - (10 / 4) + 1 // 10 - 2 + 1 = 9
	if size != expectedSize {
		t.Errorf("Cache should be pruned to %d, got %d", expectedSize, size)
	}

	// Verify pruning happened by checking that cache size decreased correctly
	// Note: We can't easily verify which specific events were pruned without
	// exposing internal state, so we just verify the size is correct
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
