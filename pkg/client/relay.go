package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fiatjaf/khatru"
	"github.com/girino/nostr-lib/logging"
	"github.com/girino/renoter/internal/config"
	"github.com/nbd-wtf/go-nostr"
)

// SetupRelay configures a khatru relay to intercept incoming events,
// wrap them using the provided Renoter path, and forward to the server relays.
func SetupRelay(relay *khatru.Relay, renterPath [][]byte, serverRelayURLs []string) error {
	logging.Info("client.relay.SetupRelay: Setting up khatru relay with %d Renoters, server relays: %v", len(renterPath), serverRelayURLs)

	// Create SimplePool for managing multiple relay connections
	ctx := context.Background()
	serverPool := nostr.NewSimplePool(ctx)
	logging.DebugMethod("client.relay", "SetupRelay", "Created SimplePool for %d server relays", len(serverRelayURLs))

	// Ensure all relays are available in the pool (they'll be connected on-demand)
	for _, url := range serverRelayURLs {
		_, err := serverPool.EnsureRelay(url)
		if err != nil {
			logging.Error("client.relay.SetupRelay: failed to ensure relay %s in pool: %v", url, err)
			return fmt.Errorf("failed to ensure relay %s: %w", url, err)
		}
	}
	logging.Info("client.relay.SetupRelay: Successfully initialized SimplePool with %d server relays", len(serverRelayURLs))

	// RejectEvent handler: Check size and process events
	// This runs before the event is accepted, allowing us to reject oversized events
	relay.RejectEvent = append(relay.RejectEvent, func(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
		return rejectEventHandler(ctx, event, renterPath, serverPool, serverRelayURLs)
	})

	// Do NOT set StoreEvent - khatru doesn't save by default
	// Events will be intercepted via RejectEvent, checked for size, wrapped, and forwarded
	// But won't be stored locally (unless StoreEvent is set elsewhere)

	logging.Info("client.relay.SetupRelay: Successfully configured khatru relay with event processing via RejectEvent (size checking and forwarding, no local storage)")
	return nil
}

// rejectEventHandler checks event size and processes acceptable events by wrapping and forwarding them.
func rejectEventHandler(ctx context.Context, event *nostr.Event, renterPath [][]byte, serverPool *nostr.SimplePool, serverRelayURLs []string) (reject bool, msg string) {
	// Shuffle the Renoter path for each event to randomize routing
	// This improves privacy by ensuring events don't always follow the same path
	shuffledPath := ShufflePath(renterPath)
	logging.DebugMethod("client.relay", "RejectEvent", "Checking event %s for size limits", event.ID)

	// Try to wrap the event - this will check if the outermost 29000 exceeds 4KB
	wrappedEvent, err := WrapEvent(event, shuffledPath)
	if err != nil {
		// Check if wrapping failed due to size limit
		errStr := err.Error()
		if contains(errStr, "outermost 29000 event size") && contains(errStr, "exceeds maximum") {
			logging.Error("client.relay.RejectEvent: event %s outermost 29000 would exceed %d bytes: %v", event.ID, config.StandardizedSize, err)
			return true, fmt.Sprintf("event too large: outermost 29000 event exceeds %d bytes", config.StandardizedSize)
		}
		if contains(errStr, "exceeds target size") || contains(errStr, "exceeds maximum") {
			logging.Error("client.relay.RejectEvent: event %s would exceed %d bytes after wrapping: %v", event.ID, config.StandardizedSize, err)
			return true, fmt.Sprintf("event too large: wrapped message would exceed %d bytes", config.StandardizedSize)
		}
		// Other wrapping errors - log but don't reject (let it go through normal processing)
		logging.Error("client.relay.RejectEvent: failed to wrap event %s: %v", event.ID, err)
		return false, ""
	}

	// Event is acceptable size - publish the wrapped event (29001 will be larger than 4KB due to encryption, which is expected)
	logging.DebugMethod("client.relay", "RejectEvent", "Event %s outermost 29000 size OK, publishing wrapped 29001 event", event.ID)

	// Publish wrapped event to all server relays using SimplePool
	publishResults := serverPool.PublishMany(ctx, serverRelayURLs, *wrappedEvent)

	// Collect results
	successCount := 0
	for result := range publishResults {
		if result.Error != nil {
			logging.Error("client.relay.RejectEvent: failed to publish wrapped event %s to relay %s: %v", wrappedEvent.ID, result.RelayURL, result.Error)
		} else {
			successCount++
			logging.DebugMethod("client.relay", "RejectEvent", "Successfully published wrapped event %s to relay %s", wrappedEvent.ID, result.RelayURL)
		}
	}

	if successCount == 0 {
		logging.Error("client.relay.RejectEvent: Failed to publish wrapped event %s to any relay", wrappedEvent.ID)
		// Don't reject the original event - wrapping/publishing failure shouldn't block the event
		// The sender will see the event was accepted even if forwarding failed
	}

	// Don't reject - return false so event continues (though it won't be stored since StoreEvent is not set)
	return false, ""
}

// contains is a helper function to check if a string contains a substring (case-insensitive).
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
