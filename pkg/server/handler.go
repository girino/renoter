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

	// Check if inner event was already published (replay protection for inner events)
	// This prevents the same inner event from being published multiple times
	r.eventMu.Lock()
	innerAlreadyProcessed := r.eventStore[innerEvent.ID]
	if innerAlreadyProcessed {
		r.eventMu.Unlock()
		logging.Warn("server.handler.HandleEvent: Inner event %s already published, skipping duplicate", innerEvent.ID)
		return fmt.Errorf("inner event %s already published", innerEvent.ID)
	}
	r.eventStore[innerEvent.ID] = true
	r.eventMu.Unlock()
	logging.DebugMethod("server.handler", "HandleEvent", "Marked inner event %s as published", innerEvent.ID)

	// Publish inner event to all relays using SimplePool
	relayURLs := r.GetRelayURLs()
	logging.DebugMethod("server.handler", "HandleEvent", "Publishing inner event %s to %d relays", innerEvent.ID, len(relayURLs))
	publishResults := r.GetPool().PublishMany(ctx, relayURLs, innerEvent)
	
	// Collect results
	successCount := 0
	for result := range publishResults {
		if result.Error != nil {
			logging.Error("server.handler.HandleEvent: failed to publish inner event %s to relay %s: %v", innerEvent.ID, result.RelayURL, result.Error)
		} else {
			successCount++
			logging.DebugMethod("server.handler", "HandleEvent", "Successfully published inner event %s to relay %s", innerEvent.ID, result.RelayURL)
		}
	}
	
	if successCount == 0 {
		return fmt.Errorf("failed to publish inner event %s to any relay", innerEvent.ID)
	}

	logging.Info("server.handler.HandleEvent: Successfully processed and forwarded event %s -> inner event %s (kind %d) to %d/%d relays", event.ID, innerEvent.ID, innerEvent.Kind, successCount, len(relayURLs))
	return nil
}

// SubscribeToWrappedEvents subscribes to wrapper events (kind 29000) on multiple relays.
func (r *Renoter) SubscribeToWrappedEvents(ctx context.Context) error {
	relayURLs := r.GetRelayURLs()
	logging.Info("server.handler.SubscribeToWrappedEvents: Subscribing to %d relays: %v", len(relayURLs), relayURLs)

	// Create filter for wrapper events addressed to this Renoter
	// Filter by events with kind 29000 that have our pubkey in a "p" tag
	filter := nostr.Filter{
		Kinds: []int{29000}, // Wrapper event kind
		Tags: nostr.TagMap{
			"p": []string{r.PublicKey}, // Only events with our pubkey in "p" tag
		},
	}

	logging.DebugMethod("server.handler", "SubscribeToWrappedEvents", "Creating subscription filter: kind=29000, p tag=%s (first 16 chars)", r.PublicKey[:16])

	// Subscribe to all relays using SimplePool
	events := r.GetPool().SubscribeMany(ctx, relayURLs, filter)
	logging.Info("server.handler.SubscribeToWrappedEvents: Successfully subscribed to wrapper events (kind 29000) with our pubkey in 'p' tag on %d relays", len(relayURLs))

	// Handle incoming events from all relays
	go func() {
		logging.Info("server.handler.SubscribeToWrappedEvents: Started event processing goroutine")
		for {
			select {
			case <-ctx.Done():
				logging.Info("server.handler.SubscribeToWrappedEvents: Context cancelled, stopping event processing")
				return
			case relayEvent, ok := <-events:
				if !ok {
					logging.Info("server.handler.SubscribeToWrappedEvents: Event channel closed, stopping")
					return
				}
				
				ev := relayEvent.Event
				logging.DebugMethod("server.handler", "SubscribeToWrappedEvents", "Received wrapper event: ID=%s from relay %s", ev.ID, relayEvent.Relay.URL)
				
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
			}
		}
	}()

	return nil
}

