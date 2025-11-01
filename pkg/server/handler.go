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

	// Validate and recalculate the event ID after deserialization if needed
	// The ID in the JSON might have been computed before padding was applied,
	// so we need to ensure it matches the current event structure (which includes padding tags)
	oldID := innerEvent.ID
	newID := innerEvent.GetID()
	
	// Check if the old ID is valid for the current event structure
	if oldID != "" && oldID == newID {
		// ID matches current structure, verify it with CheckID
		if innerEvent.CheckID() {
			logging.DebugMethod("server.handler", "HandleEvent", "Inner event ID verified: %s", innerEvent.ID)
		} else {
			// ID doesn't match computed ID, recalculate
			innerEvent.ID = newID
			logging.DebugMethod("server.handler", "HandleEvent", "Recalculated inner event ID: %s -> %s (CheckID failed)", oldID, innerEvent.ID)
		}
	} else {
		// Old ID doesn't match current structure or is missing, use recalculated one
		innerEvent.ID = newID
		if oldID != "" {
			logging.DebugMethod("server.handler", "HandleEvent", "Recalculated inner event ID: %s -> %s (ID was computed for different structure)", oldID, innerEvent.ID)
		} else {
			logging.DebugMethod("server.handler", "HandleEvent", "Computed inner event ID: %s", innerEvent.ID)
		}
	}
	
	// Final validation: ensure the ID is correct for the current event
	if !innerEvent.CheckID() {
		logging.Error("server.handler.HandleEvent: recalculated inner event ID %s failed CheckID validation", innerEvent.ID)
		return fmt.Errorf("invalid inner event ID after recalculation")
	}

	// Note: The wrapper event (outer event) was already checked for replay attacks in ProcessEvent
	// ProcessEvent ensures we don't process the same wrapper event twice, so HandleEvent
	// will only be called once per wrapper event. We don't need additional replay checks here.

	// Check if this is a final event or another wrapper
	if innerEvent.Kind == 29000 {
		logging.DebugMethod("server.handler", "HandleEvent", "Inner event is another wrapper (kind 29000), forwarding to next Renoter")
	}

	// Publish inner event to all relays using SimplePool
	// IMPORTANT: This should only be called once per inner event
	relayURLs := r.GetRelayURLs()
	publishResults := r.GetPool().PublishMany(ctx, relayURLs, innerEvent)
	logging.DebugMethod("server.handler", "HandleEvent", "PublishMany started for inner event %s, collecting results...", innerEvent.ID)

	// Collect results
	successCount := 0
	failedRelays := []string{}
	for result := range publishResults {
		if result.Error != nil {
			failedRelays = append(failedRelays, result.RelayURL)
			logging.Error("server.handler.HandleEvent: failed to publish inner event %s to relay %s: %v", innerEvent.ID, result.RelayURL, result.Error)
		} else {
			successCount++
			logging.DebugMethod("server.handler", "HandleEvent", "Successfully published inner event %s to relay %s", innerEvent.ID, result.RelayURL)
		}
	}

	if successCount == 0 {
		logging.Error("server.handler.HandleEvent: Failed to publish inner event %s to any relay. Failed relays: %v", innerEvent.ID, failedRelays)
		return fmt.Errorf("failed to publish inner event %s to any relay", innerEvent.ID)
	}

	return nil
}

// SubscribeToWrappedEvents subscribes to wrapper events (kind 29000) on multiple relays.
func (r *Renoter) SubscribeToWrappedEvents(ctx context.Context) error {
	relayURLs := r.GetRelayURLs()

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
	// Track processed events to avoid processing the same event multiple times from different relays
	// Also track events currently being processed to prevent concurrent processing
	processedEvents := make(map[string]bool)
	processingEvents := make(map[string]bool)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case relayEvent, ok := <-events:
				if !ok {
					return
				}

				ev := relayEvent.Event

				// Deduplicate: skip if we already processed this event
				if processedEvents[ev.ID] {
					continue
				}
				// Check if currently being processed (defense against race conditions)
				if processingEvents[ev.ID] {
					continue
				}
				// Mark as being processed immediately
				processingEvents[ev.ID] = true

				// Process the event (verify signature)
				err := r.ProcessEvent(ctx, ev)
				if err != nil {
					logging.Warn("server.handler.SubscribeToWrappedEvents: Error processing event %s: %v", ev.ID, err)
					continue
				}

				// Handle (decrypt and forward)
				err = r.HandleEvent(ctx, ev)

				// Mark as processed (regardless of success/failure)
				processedEvents[ev.ID] = true
				delete(processingEvents, ev.ID)

				if err != nil {
					logging.Warn("server.handler.SubscribeToWrappedEvents: Error handling event %s: %v", ev.ID, err)
					continue
				}
			}
		}
	}()

	return nil
}
