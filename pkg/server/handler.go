package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/girino/nostr-lib/logging"
	"github.com/girino/renoter/internal/config"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip13"
	"github.com/nbd-wtf/go-nostr/nip44"
)

// padEventToExactSize is a helper function to pad events to exact size (same as client version).
func padEventToExactSize(event *nostr.Event, targetSize int) (*nostr.Event, error) {
	// Create a copy to avoid modifying the original
	paddedEvent := *event
	if paddedEvent.Tags == nil {
		paddedEvent.Tags = nostr.Tags{}
	}

	// Serialize event to get current size (without padding)
	eventJSON, err := json.Marshal(&paddedEvent)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize event for padding: %w", err)
	}
	currentSize := len(eventJSON)

	// Calculate padding tag base size: ["padding",""]
	testEventWithEmptyPadding := *event
	if testEventWithEmptyPadding.Tags == nil {
		testEventWithEmptyPadding.Tags = nostr.Tags{}
	}
	testEventWithEmptyPadding.Tags = append(testEventWithEmptyPadding.Tags, nostr.Tag{"padding", ""})
	testJSONWithEmptyPadding, _ := json.Marshal(&testEventWithEmptyPadding)
	tagBaseSize := len(testJSONWithEmptyPadding) - currentSize
	logging.DebugMethod("server.handler", "padEventToExactSize", "Padding tag base size: %d bytes", tagBaseSize)

	// Calculate total size including padding tag base
	totalSize := currentSize + tagBaseSize

	// Check if base event is too large
	if totalSize > targetSize {
		logging.Error("server.handler.padEventToExactSize: event base size %d (with tag overhead %d) exceeds target size %d", currentSize, tagBaseSize, targetSize)
		return nil, fmt.Errorf("event base size %d exceeds target size %d", totalSize, targetSize)
	}

	// Calculate exact padding needed
	paddingNeeded := targetSize - totalSize

	// Generate padding string of exactly the needed length
	paddingBytes := make([]byte, (paddingNeeded+1)/2) // Round up
	if len(paddingBytes) > 0 {
		if _, err := rand.Read(paddingBytes); err != nil {
			return nil, fmt.Errorf("failed to generate random padding: %w", err)
		}
	}
	paddingString := hex.EncodeToString(paddingBytes)

	// Truncate to exact length needed
	if len(paddingString) > paddingNeeded {
		paddingString = paddingString[:paddingNeeded]
	}

	// Add padding tag
	paddedEvent.Tags = append(paddedEvent.Tags, nostr.Tag{"padding", paddingString})

	// Verify final size
	finalJSON, _ := json.Marshal(&paddedEvent)
	if len(finalJSON) != targetSize {
		logging.Error("server.handler.padEventToExactSize: padded event size %d does not match target %d", len(finalJSON), targetSize)
		return nil, fmt.Errorf("padded event size %d does not match target %d", len(finalJSON), targetSize)
	}

	logging.DebugMethod("server.handler", "padEventToExactSize", "Added padding: %d bytes needed, event size: %d -> target: %d", paddingNeeded, currentSize, targetSize)

	return &paddedEvent, nil
}

// HandleEvent handles a standardized wrapper event (29001) by decrypting it,
// processing the inner 29000 event, and either re-wrapping or publishing the final event.
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

	// Decrypt the 29001 content using this Renoter's private key
	senderPubkey := event.PubKey
	logging.DebugMethod("server.handler", "HandleEvent", "Decrypting 29001 event, sender pubkey: %s (first 16 chars)", senderPubkey[:16])

	conversationKey, err := nip44.GenerateConversationKey(senderPubkey, r.PrivateKey)
	if err != nil {
		logging.Error("server.handler.HandleEvent: failed to generate conversation key for 29001 %s: %v", event.ID, err)
		return fmt.Errorf("failed to generate conversation key: %w", err)
	}

	plaintext29001, err := nip44.Decrypt(event.Content, conversationKey)
	if err != nil {
		logging.Error("server.handler.HandleEvent: failed to decrypt 29001 content for event %s: %v", event.ID, err)
		return fmt.Errorf("failed to decrypt 29001 content: %w", err)
	}

	// Deserialize the inner 29000 event
	var inner29000 nostr.Event
	err = json.Unmarshal([]byte(plaintext29001), &inner29000)
	if err != nil {
		logging.Error("server.handler.HandleEvent: failed to deserialize inner 29000 event for event %s: %v", event.ID, err)
		return fmt.Errorf("failed to deserialize inner 29000 event: %w", err)
	}

	// Verify the inner 29000 is addressed to us
	// Check "p" tag contains our pubkey
	isAddressedToUs := false
	for _, tag := range inner29000.Tags {
		if len(tag) >= 2 && tag[0] == "p" && tag[1] == r.PublicKey {
			isAddressedToUs = true
			break
		}
	}

	if !isAddressedToUs {
		logging.DebugMethod("server.handler", "HandleEvent", "Inner 29000 event not addressed to us, silently dropping")
		return nil // Silently drop
	}

	logging.DebugMethod("server.handler", "HandleEvent", "Inner 29000 event is addressed to us, decrypting")

	// Validate proof-of-work for 29000 event
	err = nip13.Check(inner29000.ID, config.PoWDifficulty)
	if err != nil {
		logging.Error("server.handler.HandleEvent: 29000 event PoW validation failed: %v", err)
		return fmt.Errorf("invalid PoW for 29000 event: %w", err)
	}
	logging.DebugMethod("server.handler", "HandleEvent", "29000 event PoW validated successfully")

	// Decrypt the 29000 event
	sender29000Pubkey := inner29000.PubKey
	conversationKey29000, err := nip44.GenerateConversationKey(sender29000Pubkey, r.PrivateKey)
	if err != nil {
		logging.Error("server.handler.HandleEvent: failed to generate conversation key for inner 29000: %v", err)
		return fmt.Errorf("failed to generate conversation key for 29000: %w", err)
	}

	plaintext29000, err := nip44.Decrypt(inner29000.Content, conversationKey29000)
	if err != nil {
		logging.Error("server.handler.HandleEvent: failed to decrypt inner 29000 content: %v", err)
		return fmt.Errorf("failed to decrypt inner 29000 content: %w", err)
	}

	// Deserialize the content inside 29000
	var innerEvent nostr.Event
	err = json.Unmarshal([]byte(plaintext29000), &innerEvent)
	if err != nil {
		logging.Error("server.handler.HandleEvent: failed to deserialize inner event: %v", err)
		return fmt.Errorf("failed to deserialize inner event: %w", err)
	}

	// Remove padding from inner event
	originalTags := innerEvent.Tags
	innerEvent.Tags = nostr.Tags{}
	for _, tag := range originalTags {
		if len(tag) > 0 && tag[0] != "padding" {
			innerEvent.Tags = append(innerEvent.Tags, tag)
		}
	}

	// Verify ID and signature after removing padding
	originalID := innerEvent.ID
	calculatedID := innerEvent.GetID()
	if originalID != calculatedID {
		logging.Error("server.handler.HandleEvent: inner event ID mismatch after removing padding: original=%s, calculated=%s", originalID, calculatedID)
		return fmt.Errorf("inner event ID mismatch after removing padding")
	}

	if innerEvent.Sig != "" {
		valid, err := innerEvent.CheckSignature()
		if err != nil {
			logging.Error("server.handler.HandleEvent: failed to check inner event signature: %v", err)
			return fmt.Errorf("failed to check inner event signature: %w", err)
		}
		if !valid {
			logging.Error("server.handler.HandleEvent: invalid signature for inner event %s", innerEvent.ID)
			return fmt.Errorf("invalid signature for inner event")
		}
	}

	// Check if inner event is another 29000 (next in path) or final event
	if innerEvent.Kind == config.WrapperEventKind {
		logging.DebugMethod("server.handler", "HandleEvent", "Inner event is another 29000, re-wrapping for next Renoter")

		// Validate proof-of-work for inner 29000 event
		err = nip13.Check(innerEvent.ID, config.PoWDifficulty)
		if err != nil {
			logging.Error("server.handler.HandleEvent: inner 29000 event PoW validation failed: %v", err)
			return fmt.Errorf("invalid PoW for inner 29000 event: %w", err)
		}
		logging.DebugMethod("server.handler", "HandleEvent", "Inner 29000 event PoW validated successfully")

		// Get next Renoter from "p" tag of inner 29000
		nextRenoterPubkey := ""
		for _, tag := range innerEvent.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				nextRenoterPubkey = tag[1]
				break
			}
		}

		if nextRenoterPubkey == "" {
			logging.Error("server.handler.HandleEvent: inner 29000 has no 'p' tag for next Renoter")
			return fmt.Errorf("inner 29000 has no 'p' tag for next Renoter")
		}

		// Pad inner 29000 to exactly 8KB
		padded29000, err := padEventToExactSize(&innerEvent, config.StandardizedSize)
		if err != nil {
			logging.Error("server.handler.HandleEvent: failed to pad inner 29000 to %d bytes: %v", config.StandardizedSize, err)
			return fmt.Errorf("failed to pad inner 29000: %w", err)
		}

		// Serialize padded 29000
		padded29000JSON, err := json.Marshal(padded29000)
		if err != nil {
			logging.Error("server.handler.HandleEvent: failed to serialize padded 29000: %v", err)
			return fmt.Errorf("failed to serialize padded 29000: %w", err)
		}

		// Generate key for new 29001
		sk29001 := nostr.GeneratePrivateKey()
		pubkey29001, err := nostr.GetPublicKey(sk29001)
		if err != nil {
			logging.Error("server.handler.HandleEvent: failed to get public key for 29001: %v", err)
			return fmt.Errorf("failed to get public key: %w", err)
		}

		// Encrypt for next Renoter
		conversationKey29001, err := nip44.GenerateConversationKey(nextRenoterPubkey, sk29001)
		if err != nil {
			logging.Error("server.handler.HandleEvent: failed to generate conversation key for next Renoter: %v", err)
			return fmt.Errorf("failed to generate conversation key: %w", err)
		}

		ciphertext29001, err := nip44.Encrypt(string(padded29000JSON), conversationKey29001)
		if err != nil {
			logging.Error("server.handler.HandleEvent: failed to encrypt for 29001: %v", err)
			return fmt.Errorf("failed to encrypt for 29001: %w", err)
		}

		// Create new 29001 container
		new29001 := &nostr.Event{
			Kind:      config.StandardizedWrapperKind,
			Content:   ciphertext29001,
			CreatedAt: nostr.Now(),
			PubKey:    pubkey29001,
			Tags: nostr.Tags{
				{"p", nextRenoterPubkey},
			},
		}

		new29001.ID = new29001.GetID()
		if !new29001.CheckID() {
			logging.Error("server.handler.HandleEvent: new 29001 ID validation failed")
			return fmt.Errorf("invalid new 29001 event ID")
		}

		err = new29001.Sign(sk29001)
		if err != nil {
			logging.Error("server.handler.HandleEvent: failed to sign new 29001: %v", err)
			return fmt.Errorf("failed to sign new 29001: %w", err)
		}

		// Publish new 29001
		relayURLs := r.GetRelayURLs()
		publishResults := r.GetPool().PublishMany(ctx, relayURLs, *new29001)
		successCount := 0
		failedRelays := []string{}
		for result := range publishResults {
			if result.Error != nil {
				failedRelays = append(failedRelays, result.RelayURL)
				logging.Error("server.handler.HandleEvent: failed to publish new 29001 %s to relay %s: %v", new29001.ID, result.RelayURL, result.Error)
			} else {
				successCount++
				logging.DebugMethod("server.handler", "HandleEvent", "Successfully published new 29001 %s to relay %s", new29001.ID, result.RelayURL)
			}
		}

		if successCount == 0 {
			logging.Error("server.handler.HandleEvent: Failed to publish new 29001 %s to any of %d relays. Failed relays: %v", new29001.ID, len(relayURLs), failedRelays)
			return fmt.Errorf("failed to publish new 29001 to any relay")
		}

		logging.Info("server.handler.HandleEvent: Successfully re-wrapped and published 29001 %s to %d/%d relays", new29001.ID, successCount, len(relayURLs))
		if len(failedRelays) > 0 {
			logging.Warn("server.handler.HandleEvent: Failed to publish 29001 %s to %d relay(s): %v", new29001.ID, len(failedRelays), failedRelays)
		}
		return nil
	} else {
		// Final event - publish as-is
		logging.DebugMethod("server.handler", "HandleEvent", "Inner event is final event (kind %d), publishing", innerEvent.Kind)
		relayURLs := r.GetRelayURLs()
		publishResults := r.GetPool().PublishMany(ctx, relayURLs, innerEvent)
		successCount := 0
		failedRelays := []string{}
		for result := range publishResults {
			if result.Error != nil {
				failedRelays = append(failedRelays, result.RelayURL)
				logging.Error("server.handler.HandleEvent: failed to publish final event %s to relay %s: %v", innerEvent.ID, result.RelayURL, result.Error)
			} else {
				successCount++
				logging.DebugMethod("server.handler", "HandleEvent", "Successfully published final event %s to relay %s", innerEvent.ID, result.RelayURL)
			}
		}

		if successCount == 0 {
			logging.Error("server.handler.HandleEvent: Failed to publish final event %s to any of %d relays. Failed relays: %v", innerEvent.ID, len(relayURLs), failedRelays)
			return fmt.Errorf("failed to publish final event to any relay")
		}

		logging.Info("server.handler.HandleEvent: Successfully published final event %s to %d/%d relays", innerEvent.ID, successCount, len(relayURLs))
		if len(failedRelays) > 0 {
			logging.Warn("server.handler.HandleEvent: Failed to publish final event %s to %d relay(s): %v", innerEvent.ID, len(failedRelays), failedRelays)
		}
		return nil
	}
}

// SubscribeToWrappedEvents subscribes to standardized wrapper events (kind 29001) on multiple relays.
func (r *Renoter) SubscribeToWrappedEvents(ctx context.Context) error {
	relayURLs := r.GetRelayURLs()

	// Create filter for standardized wrapper events addressed to this Renoter
	// Filter by events with kind 29001 that have our pubkey in a "p" tag
	filter := nostr.Filter{
		Kinds: []int{config.StandardizedWrapperKind}, // Standardized wrapper event kind (29001)
		Tags: nostr.TagMap{
			"p": []string{r.PublicKey}, // Only events with our pubkey in "p" tag
		},
	}

	logging.DebugMethod("server.handler", "SubscribeToWrappedEvents", "Creating subscription filter: kind=29001, p tag=%s (first 16 chars)", r.PublicKey[:16])

	// Subscribe to all relays using SimplePool
	events := r.GetPool().SubscribeMany(ctx, relayURLs, filter)
	logging.Info("server.handler.SubscribeToWrappedEvents: Successfully subscribed to standardized wrapper events (kind 29001) with our pubkey in 'p' tag on %d relays", len(relayURLs))

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
