package client

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/girino/renoter/internal/config"

	"github.com/girino/nostr-lib/logging"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip44"
)

// WrapEvent creates nested wrapper events for the given Renoter path.
// Events are wrapped in reverse order (last Renoter first, first Renoter last).
// Each wrapper event encrypts the inner event for the next Renoter in the path.
func WrapEvent(originalEvent *nostr.Event, renterPath [][]byte) (*nostr.Event, error) {
	logging.DebugMethod("client.wrapper", "WrapEvent", "Starting event wrapping, path length: %d, original event ID: %s, kind: %d", len(renterPath), originalEvent.ID, originalEvent.Kind)

	if len(renterPath) == 0 {
		logging.Error("client.wrapper.WrapEvent: renoter path cannot be empty")
		return nil, fmt.Errorf("renoter path cannot be empty")
	}

	// Start with the original event
	currentEvent := originalEvent

	logging.DebugMethod("client.wrapper", "WrapEvent", "Beginning nested wrapping in reverse order (last Renoter first)")

	// Wrap in reverse order (last Renoter first)
	for i := len(renterPath) - 1; i >= 0; i-- {
		renoterPubkeyBytes := renterPath[i]
		renoterPubkey := hex.EncodeToString(renoterPubkeyBytes)

		logging.DebugMethod("client.wrapper", "WrapEvent", "Wrapping layer %d/%d for Renoter pubkey: %s (first 16 chars: %s)", len(renterPath)-i, len(renterPath), renoterPubkey, renoterPubkey[:16])

		// Serialize current event to JSON
		logging.DebugMethod("client.wrapper", "WrapEvent", "Serializing event to JSON (layer %d)", i)
		eventJSON, err := json.Marshal(currentEvent)
		if err != nil {
			logging.Error("client.wrapper.WrapEvent: failed to serialize event at layer %d: %v", i, err)
			return nil, fmt.Errorf("failed to serialize event: %w", err)
		}
		logging.DebugMethod("client.wrapper", "WrapEvent", "Serialized event JSON length: %d bytes (layer %d)", len(eventJSON), i)

		// Generate random key for this wrapper event
		logging.DebugMethod("client.wrapper", "WrapEvent", "Generating random key for wrapper (layer %d)", i)
		sk := nostr.GeneratePrivateKey()

		// Get public key from private key
		pubkey, err := nostr.GetPublicKey(sk)
		if err != nil {
			logging.Error("client.wrapper.WrapEvent: failed to get public key at layer %d: %v", i, err)
			return nil, fmt.Errorf("failed to get public key: %w", err)
		}
		logging.DebugMethod("client.wrapper", "WrapEvent", "Generated wrapper key pubkey: %s (first 16 chars, layer %d)", pubkey[:16], i)

		// Generate conversation key using Renoter's public key and our random private key
		logging.DebugMethod("client.wrapper", "WrapEvent", "Generating conversation key (layer %d)", i)
		conversationKey, err := nip44.GenerateConversationKey(renoterPubkey, sk)
		if err != nil {
			logging.Error("client.wrapper.WrapEvent: failed to generate conversation key for renoter %d: %v", i, err)
			return nil, fmt.Errorf("failed to generate conversation key for renoter %d: %w", i, err)
		}
		logging.DebugMethod("client.wrapper", "WrapEvent", "Generated conversation key (layer %d)", i)

		// Encrypt for this Renoter using NIP-44
		logging.DebugMethod("client.wrapper", "WrapEvent", "Encrypting with NIP-44 (layer %d)", i)
		ciphertext, err := nip44.Encrypt(string(eventJSON), conversationKey)
		if err != nil {
			logging.Error("client.wrapper.WrapEvent: failed to encrypt for renoter %d: %v", i, err)
			return nil, fmt.Errorf("failed to encrypt for renoter %d: %w", i, err)
		}
		logging.DebugMethod("client.wrapper", "WrapEvent", "Encrypted ciphertext length: %d bytes (layer %d)", len(ciphertext), i)

		// Create wrapper event with encrypted content
		wrapperEvent := &nostr.Event{
			Kind:      config.WrapperEventKind,
			Content:   ciphertext,
			CreatedAt: nostr.Now(),
			PubKey:    pubkey,
			Tags: nostr.Tags{
				// Add "p" tag with destination Renoter's pubkey for routing
				{"p", renoterPubkey},
			},
		}

		logging.DebugMethod("client.wrapper", "WrapEvent", "Created wrapper event structure (layer %d)", i)

		// Sign the wrapper event
		err = wrapperEvent.Sign(sk)
		if err != nil {
			logging.Error("client.wrapper.WrapEvent: failed to sign wrapper event at layer %d: %v", i, err)
			return nil, fmt.Errorf("failed to sign wrapper event: %w", err)
		}
		logging.DebugMethod("client.wrapper", "WrapEvent", "Signed wrapper event, ID: %s (layer %d)", wrapperEvent.ID, i)

		// This wrapper becomes the current event for the next iteration
		currentEvent = wrapperEvent
		logging.DebugMethod("client.wrapper", "WrapEvent", "Completed wrapping layer %d, proceeding to next layer", i)
	}

	logging.Info("client.wrapper.WrapEvent: Successfully wrapped event through %d Renoter layers, final wrapper event ID: %s", len(renterPath), currentEvent.ID)
	return currentEvent, nil
}
