package client

import (
	"context"
	"fmt"
	"github.com/fiatjaf/khatru"
	"github.com/girino/nostr-lib/logging"
	"github.com/nbd-wtf/go-nostr"
)

// SetupRelay configures a khatru relay to intercept incoming events,
// wrap them using the provided Renoter path, and forward to the server relay.
func SetupRelay(relay *khatru.Relay, renterPath [][]byte, serverRelayURL string) error {
	logging.Info("client.relay.SetupRelay: Setting up khatru relay with %d Renoters, server relay: %s", len(renterPath), serverRelayURL)

	// Create connection to server relay for forwarding wrapped events
	logging.DebugMethod("client.relay", "SetupRelay", "Connecting to server relay: %s", serverRelayURL)
	serverRelay, err := nostr.RelayConnect(context.Background(), serverRelayURL)
	if err != nil {
		logging.Error("client.relay.SetupRelay: failed to connect to server relay %s: %v", serverRelayURL, err)
		return fmt.Errorf("failed to connect to server relay: %w", err)
	}
	logging.Info("client.relay.SetupRelay: Successfully connected to server relay: %s", serverRelayURL)

	// Hook into StoreEvent to intercept incoming events
	relay.StoreEvent = append(relay.StoreEvent, func(ctx context.Context, event *nostr.Event) error {
		logging.DebugMethod("client.relay", "StoreEvent", "Intercepted incoming event: ID=%s, Kind=%d, PubKey=%s", event.ID, event.Kind, event.PubKey[:16])

		// Skip wrapper events (kind 29000) to prevent infinite loops
		if event.Kind == 29000 {
			logging.DebugMethod("client.relay", "StoreEvent", "Skipping wrapper event (kind 29000), allowing normal storage: %s", event.ID)
			return nil // Allow wrapper events to be stored normally
		}

		logging.Info("client.relay.StoreEvent: Processing event for wrapping: ID=%s, Kind=%d", event.ID, event.Kind)

		// Wrap the event for the Renoter path
		wrappedEvent, err := WrapEvent(event, renterPath)
		if err != nil {
			logging.Error("client.relay.StoreEvent: failed to wrap event %s: %v", event.ID, err)
			return fmt.Errorf("failed to wrap event: %w", err)
		}
		logging.DebugMethod("client.relay", "StoreEvent", "Successfully wrapped event %s -> %s", event.ID, wrappedEvent.ID)

		// Publish wrapped event to server relay
		logging.DebugMethod("client.relay", "StoreEvent", "Publishing wrapped event %s to server relay", wrappedEvent.ID)
		err = serverRelay.Publish(ctx, *wrappedEvent)
		if err != nil {
			logging.Error("client.relay.StoreEvent: failed to publish wrapped event %s to server relay: %v", wrappedEvent.ID, err)
			return fmt.Errorf("failed to publish wrapped event: %w", err)
		}

		logging.Info("client.relay.StoreEvent: Successfully wrapped and forwarded event %s (original kind: %d) -> wrapper %s", event.ID, event.Kind, wrappedEvent.ID)

		// Return error to prevent storing the original event
		// This ensures only wrapped events go through
		return fmt.Errorf("event intercepted and wrapped")
	})

	logging.Info("client.relay.SetupRelay: Successfully configured khatru relay with event interception")
	return nil
}

