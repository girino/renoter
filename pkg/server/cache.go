package server

import (
	"sort"
	"sync"
	"time"

	"github.com/girino/nostr-lib/logging"
)

// EventCache maintains a bounded in-memory cache of event IDs for replay attack protection.
// The cache is limited to a maximum size and automatically prunes entries older than the cutoff duration.
type EventCache struct {
	// Map event ID to when it was first seen
	eventStore map[string]time.Time
	// Track insertion order for pruning
	eventKeys []string
	// Mutex for thread-safe access
	mu sync.RWMutex
	// Maximum cache size
	maxSize int
	// Maximum age for cached entries (older entries are removed)
	cutoffDuration time.Duration
}

// NewEventCache creates a new EventCache with the specified maximum size and cutoff duration.
// Entries older than cutoffDuration will be automatically removed during cleanup.
func NewEventCache(maxSize int, cutoffDuration time.Duration) *EventCache {
	return &EventCache{
		eventStore:     make(map[string]time.Time),
		eventKeys:      make([]string, 0, maxSize+100), // Pre-allocate slightly more to reduce reallocations
		maxSize:        maxSize,
		cutoffDuration: cutoffDuration,
	}
}

// CheckAndMark checks if an event ID has been seen before and marks it as seen.
// Returns true if the event was already seen (replay attack), false otherwise.
// The event is marked as seen with the current timestamp.
func (c *EventCache) CheckAndMark(eventID string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Prune cache if it's at or exceeds max size (remove 25% for performance)
	// Do this first since it's less resource intensive
	// We prune at maxSize (not just above) to ensure there's room for the new event we'll add
	if len(c.eventKeys) >= c.maxSize {
		logging.DebugMethod("server.cache", "CheckAndMark", "Cache at max size (%d), pruning...", len(c.eventKeys))
		c.pruneLocked()
	}

	// Clean up old events (binary search, more resource intensive)
	logging.DebugMethod("server.cache", "CheckAndMark", "Cleaning up old events (cache size: %d)...", len(c.eventKeys))
	c.cleanupOldEventsLocked(now)
	logging.DebugMethod("server.cache", "CheckAndMark", "Cleanup complete (cache size: %d)", len(c.eventKeys))

	// Check if event already exists
	if _, exists := c.eventStore[eventID]; exists {
		logging.Warn("server.cache: Replay attack detected, event %s already processed", eventID)
		return true
	}

	// Mark event as seen
	c.eventStore[eventID] = now
	c.eventKeys = append(c.eventKeys, eventID)

	return false
}

// cleanupOldEvents removes events older than the cutoff duration from the cache.
// Since events are added sequentially and ordered by timestamp, we use binary search
// to find the cutoff point efficiently.
// Must be called with mu locked.
func (c *EventCache) cleanupOldEventsLocked(now time.Time) {
	if len(c.eventKeys) == 0 {
		logging.DebugMethod("server.cache", "cleanupOldEventsLocked", "Cache is empty, skipping cleanup")
		return
	}

	initialSize := len(c.eventKeys)
	cutoffTime := now.Add(-c.cutoffDuration)
	logging.DebugMethod("server.cache", "cleanupOldEventsLocked", "Starting cleanup: cache size=%d, cutoff duration=%v, cutoff time=%v", initialSize, c.cutoffDuration, cutoffTime)

	// Binary search to find the first index where timestamp is >= cutoffTime
	// (i.e., first event newer than cutoffDuration ago)
	// We want to keep all events from this index onwards
	firstNewIndex := sort.Search(len(c.eventKeys), func(i int) bool {
		eventID := c.eventKeys[i]
		if seenTime, exists := c.eventStore[eventID]; exists {
			return seenTime.After(cutoffTime) || seenTime.Equal(cutoffTime)
		}
		// If event doesn't exist in map, treat it as old to remove
		return false
	})

	// Remove all events before firstNewIndex (older than cutoff duration)
	if firstNewIndex > 0 {
		removedCount := firstNewIndex
		logging.DebugMethod("server.cache", "cleanupOldEventsLocked", "Binary search found cutoff at index %d (will remove %d old entries)", firstNewIndex, removedCount)
		for i := 0; i < firstNewIndex; i++ {
			delete(c.eventStore, c.eventKeys[i])
		}
		// Keep only events from firstNewIndex onwards
		c.eventKeys = c.eventKeys[firstNewIndex:]
		logging.DebugMethod("server.cache", "cleanupOldEventsLocked", "Cleanup complete: removed %d events older than cutoff duration, cache size %d -> %d", removedCount, initialSize, len(c.eventKeys))
	} else {
		logging.DebugMethod("server.cache", "cleanupOldEventsLocked", "No old events to remove (all events are within cutoff duration)")
	}
}

// pruneLocked removes 25% of the oldest entries when cache exceeds max size.
// Must be called with mu locked.
func (c *EventCache) pruneLocked() {
	initialSize := len(c.eventKeys)
	// Remove 25% of oldest entries
	removeCount := c.maxSize / 4 // 25% of max cache size
	if removeCount == 0 {
		removeCount = 1 // Ensure at least one is removed
	}
	logging.DebugMethod("server.cache", "pruneLocked", "Starting prune: cache size=%d, maxSize=%d, will remove %d entries (25%%)", initialSize, c.maxSize, removeCount)
	for i := 0; i < removeCount; i++ {
		oldestID := c.eventKeys[i]
		delete(c.eventStore, oldestID)
	}
	// Keep only the recent entries
	c.eventKeys = c.eventKeys[removeCount:]
	logging.DebugMethod("server.cache", "pruneLocked", "Prune complete: removed %d oldest entries, cache size %d -> %d", removeCount, initialSize, len(c.eventKeys))
}

// Size returns the current number of entries in the cache.
func (c *EventCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.eventKeys)
}
