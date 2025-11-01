package client

import (
	"context"

	"github.com/fiatjaf/khatru"
	"github.com/girino/nostr-lib/logging"
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

	// Note: We don't call EnsureRelay here - connections will be established lazily
	// when PublishMany is called. This allows the client to start even if relays
	// are temporarily unavailable.
	logging.Info("client.relay.SetupRelay: SimplePool initialized, connections will be established on-demand when publishing events")

	// Helper function to process and wrap events
	processEvent := func(ctx context.Context, event *nostr.Event) {
		logging.DebugMethod("client.relay", "processEvent", "Intercepted incoming event: ID=%s, Kind=%d, PubKey=%s", event.ID, event.Kind, event.PubKey[:16])

		// Skip wrapper events (kind 29000) to prevent infinite loops
		if event.Kind == 29000 {
			logging.DebugMethod("client.relay", "processEvent", "Skipping wrapper event (kind 29000): %s", event.ID)
			return
		}

		logging.Info("client.relay.processEvent: Processing event for wrapping: ID=%s, Kind=%d", event.ID, event.Kind)

		// Wrap the event for the Renoter path
		wrappedEvent, err := WrapEvent(event, renterPath)
		if err != nil {
			logging.Error("client.relay.processEvent: failed to wrap event %s: %v", event.ID, err)
			return
		}
		logging.DebugMethod("client.relay", "processEvent", "Successfully wrapped event %s -> %s", event.ID, wrappedEvent.ID)

		// Publish wrapped event to all server relays using SimplePool
		logging.DebugMethod("client.relay", "processEvent", "Publishing wrapped event %s to %d server relays", wrappedEvent.ID, len(serverRelayURLs))
		publishResults := serverPool.PublishMany(ctx, serverRelayURLs, *wrappedEvent)

		// Collect results
		successCount := 0
		for result := range publishResults {
			if result.Error != nil {
				logging.Error("client.relay.processEvent: failed to publish wrapped event %s to relay %s: %v", wrappedEvent.ID, result.RelayURL, result.Error)
			} else {
				successCount++
				logging.DebugMethod("client.relay", "processEvent", "Successfully published wrapped event %s to relay %s", wrappedEvent.ID, result.RelayURL)
			}
		}

		if successCount > 0 {
			logging.Info("client.relay.processEvent: Successfully wrapped and forwarded event %s (original kind: %d) -> wrapper %s to %d/%d relays", event.ID, event.Kind, wrappedEvent.ID, successCount, len(serverRelayURLs))
		} else {
			logging.Error("client.relay.processEvent: Failed to publish wrapped event %s to any relay", wrappedEvent.ID)
		}
	}

	// Hook into OnEventSaved to intercept saved events (non-ephemeral)
	relay.OnEventSaved = append(relay.OnEventSaved, processEvent)

	// Hook into OnEphemeralEvent to intercept ephemeral events
	relay.OnEphemeralEvent = append(relay.OnEphemeralEvent, processEvent)

	// Do NOT set StoreEvent - khatru doesn't save by default
	// Events will be intercepted via OnEventSaved/OnEphemeralEvent, wrapped, and forwarded
	// But won't be stored locally (unless StoreEvent is set elsewhere)

	logging.Info("client.relay.SetupRelay: Successfully configured khatru relay with event interception via OnEventSaved/OnEphemeralEvent (no local storage)")
	return nil
}
