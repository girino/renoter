package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/girino/nostr-lib/logging"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip44"
)

// HandleEvent handles a wrapped event by decrypting it and forwarding the inner event.
func (r *Renoter) HandleEvent(ctx context.Context, event *nostr.Event) error {
	logging.DebugMethod("server.handler", "HandleEvent", "Handling wrapped event: ID=%s", event.ID)

	// Verify signature (already done in ProcessEvent, but double-check)
	valid, err := event.CheckSignature()
	if err != nil {
		logging.Error("server.handler.HandleEvent: signature check failed for event %s: %v", event.ID, err)
		return fmt.Errorf("signature check failed: %w", err)
	}
	if !valid {
		logging.Error("server.handler.HandleEvent: invalid signature for event %s", event.ID)
		return fmt.Errorf("invalid signature for event %s", event.ID)
	}

	// Decrypt the content using this Renoter's private key
	// First, we need to get the sender's public key from the event
	senderPubkey := event.PubKey
	logging.DebugMethod("server.handler", "HandleEvent", "Sender pubkey: %s (first 16 chars)", senderPubkey[:16])

	// Generate conversation key using sender's public key and our private key
	logging.DebugMethod("server.handler", "HandleEvent", "Generating conversation key")
	conversationKey, err := nip44.GenerateConversationKey(senderPubkey, r.PrivateKey)
	if err != nil {
		logging.Error("server.handler.HandleEvent: failed to generate conversation key for event %s: %v", event.ID, err)
		return fmt.Errorf("failed to generate conversation key: %w", err)
	}
	logging.DebugMethod("server.handler", "HandleEvent", "Generated conversation key")

	// Decrypt the content
	logging.DebugMethod("server.handler", "HandleEvent", "Decrypting content, ciphertext length: %d bytes", len(event.Content))
	plaintext, err := nip44.Decrypt(event.Content, conversationKey)
	if err != nil {
		logging.Error("server.handler.HandleEvent: failed to decrypt content for event %s: %v", event.ID, err)
		return fmt.Errorf("failed to decrypt content: %w", err)
	}
	logging.DebugMethod("server.handler", "HandleEvent", "Decrypted content, plaintext length: %d bytes", len(plaintext))

	// Deserialize the inner event
	logging.DebugMethod("server.handler", "HandleEvent", "Deserializing inner event")
	var innerEvent nostr.Event
	err = json.Unmarshal([]byte(plaintext), &innerEvent)
	if err != nil {
		logging.Error("server.handler.HandleEvent: failed to deserialize inner event for event %s: %v", event.ID, err)
		return fmt.Errorf("failed to deserialize inner event: %w", err)
	}
	logging.DebugMethod("server.handler", "HandleEvent", "Deserialized inner event: ID=%s, Kind=%d", innerEvent.ID, innerEvent.Kind)

	// Check if this is a final event or another wrapper
	if innerEvent.Kind == 29000 {
		logging.DebugMethod("server.handler", "HandleEvent", "Inner event is another wrapper (kind 29000), forwarding to next Renoter")
	} else {
		logging.Info("server.handler.HandleEvent: Inner event is final event (kind %d), will forward to network", innerEvent.Kind)
	}

	// Ensure forward relay is connected
	err = r.ConnectForwardRelay(ctx)
	if err != nil {
		logging.Error("server.handler.HandleEvent: failed to connect to forward relay: %v", err)
		return fmt.Errorf("failed to connect to forward relay: %w", err)
	}

	// Publish inner event to forward relay
	logging.DebugMethod("server.handler", "HandleEvent", "Publishing inner event %s to forward relay", innerEvent.ID)
	err = r.forwardRelay.Publish(ctx, innerEvent)
	if err != nil {
		logging.Error("server.handler.HandleEvent: failed to publish inner event %s: %v", innerEvent.ID, err)
		return fmt.Errorf("failed to publish inner event: %w", err)
	}

	logging.Info("server.handler.HandleEvent: Successfully processed and forwarded event %s -> inner event %s (kind %d)", event.ID, innerEvent.ID, innerEvent.Kind)
	return nil
}

// SubscribeToWrappedEvents subscribes to wrapper events (kind 29000) on the listen relay.
func (r *Renoter) SubscribeToWrappedEvents(ctx context.Context, listenRelayURL string) error {
	logging.Info("server.handler.SubscribeToWrappedEvents: Connecting to listen relay: %s", listenRelayURL)

	relay, err := nostr.RelayConnect(ctx, listenRelayURL)
	if err != nil {
		logging.Error("server.handler.SubscribeToWrappedEvents: failed to connect to listen relay %s: %v", listenRelayURL, err)
		return fmt.Errorf("failed to connect to listen relay: %w", err)
	}
	logging.Info("server.handler.SubscribeToWrappedEvents: Successfully connected to listen relay: %s", listenRelayURL)

	// Create filter for wrapper events addressed to this Renoter
	// Filter by events with kind 29000 that have our pubkey in a "p" tag
	filter := nostr.Filter{
		Kinds: []int{29000}, // Wrapper event kind
		Tags: nostr.TagMap{
			"p": []string{r.PublicKey}, // Only events with our pubkey in "p" tag
		},
	}

	logging.DebugMethod("server.handler", "SubscribeToWrappedEvents", "Creating subscription filter: kind=29000, p tag=%s (first 16 chars)", r.PublicKey[:16])

	sub, err := relay.Subscribe(ctx, []nostr.Filter{filter})
	if err != nil {
		logging.Error("server.handler.SubscribeToWrappedEvents: failed to subscribe: %v", err)
		return fmt.Errorf("failed to subscribe: %w", err)
	}
	logging.Info("server.handler.SubscribeToWrappedEvents: Successfully subscribed to wrapper events (kind 29000) with our pubkey in 'p' tag")

	// Handle incoming events
	go func() {
		logging.Info("server.handler.SubscribeToWrappedEvents: Started event processing goroutine")
		for {
			select {
			case <-ctx.Done():
				logging.Info("server.handler.SubscribeToWrappedEvents: Context cancelled, stopping event processing")
				return
			case ev := <-sub.Events:
				logging.DebugMethod("server.handler", "SubscribeToWrappedEvents", "Received wrapper event: ID=%s", ev.ID)
				// Process the event
				err := r.ProcessEvent(ctx, ev)
				if err != nil {
					// Log error but continue processing
					logging.Warn("server.handler.SubscribeToWrappedEvents: Error processing event %s: %v", ev.ID, err)
					continue
				}

				// Handle (decrypt and forward)
				err = r.HandleEvent(ctx, ev)
				if err != nil {
					logging.Warn("server.handler.SubscribeToWrappedEvents: Error handling event %s: %v", ev.ID, err)
					continue
				}

				logging.Info("server.handler.SubscribeToWrappedEvents: Successfully processed and forwarded event %s", ev.ID)
			case <-sub.EndOfStoredEvents:
				logging.DebugMethod("server.handler", "SubscribeToWrappedEvents", "End of stored events, continuing to listen for new ones")
				// End of stored events, continue listening for new ones
			}
		}
	}()

	return nil
}

