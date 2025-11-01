package client

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/girino/renoter/internal/config"

	"github.com/girino/nostr-lib/logging"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip44"
)

const MaxWrappedEventSize = 32 * 1024 // 32KB maximum size for wrapped events after encryption

// padEventToMultipleOf32 adds padding tags to an event to make its serialized size a multiple of 32 bytes.
// Returns a new event with padding tags added, or the original event if already a multiple of 32.
// Accounts for padding tag overhead before calculating target size.
// Limits padding to MaxWrappedEventSize.
func padEventToMultipleOf32(event *nostr.Event) (*nostr.Event, error) {
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
	// ID and signature are already present in signed events, so we only need to account for the padding tag overhead
	testEventWithEmptyPadding := *event
	if testEventWithEmptyPadding.Tags == nil {
		testEventWithEmptyPadding.Tags = nostr.Tags{}
	}
	testEventWithEmptyPadding.Tags = append(testEventWithEmptyPadding.Tags, nostr.Tag{"padding", ""})
	testJSONWithEmptyPadding, _ := json.Marshal(&testEventWithEmptyPadding)
	tagBaseSize := len(testJSONWithEmptyPadding) - currentSize
	logging.DebugMethod("client.wrapper", "padEventToMultipleOf32", "Padding tag base size: %d bytes", tagBaseSize)

	// Calculate total size including padding tag base
	totalSize := currentSize + tagBaseSize
	logging.Info("client.wrapper.padEventToMultipleOf32: tagBaseSize=%d, currentSize=%d, totalSize=%d", tagBaseSize, currentSize, totalSize)

	// Find next multiple of 32 for the total size
	nextMultipleOf32 := nextMultipleOf32(totalSize)

	// Limit to MaxWrappedEventSize
	if nextMultipleOf32 > MaxWrappedEventSize {
		logging.Error("client.wrapper.padEventToMultipleOf32: event size %d exceeds maximum %d bytes after padding", nextMultipleOf32, MaxWrappedEventSize)
		return nil, fmt.Errorf("event size would exceed maximum %d bytes", MaxWrappedEventSize)
	}

	// Calculate exact padding needed in the padding string
	paddingNeeded := nextMultipleOf32 - totalSize

	// If no padding needed (already multiple of 32), return as-is
	if paddingNeeded == 0 {
		logging.DebugMethod("client.wrapper", "padEventToMultipleOf32", "Event already at multiple of 32 size: %d bytes", currentSize)
		return &paddedEvent, nil
	}

	// Generate padding string of exactly the needed length
	// Each byte of random data becomes 2 hex chars, so we need paddingNeeded/2 bytes
	// But we need exactly paddingNeeded chars, so generate enough and truncate
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

	logging.DebugMethod("client.wrapper", "padEventToMultipleOf32", "Added padding: %d bytes needed, event size: %d -> target: %d", paddingNeeded, currentSize, nextMultipleOf32)

	return &paddedEvent, nil
}

// nextMultipleOf32 returns the smallest multiple of 32 that is >= n.
func nextMultipleOf32(n int) int {
	if n <= 0 {
		return 32
	}
	if n%32 == 0 {
		return n
	}
	return ((n + 31) / 32) * 32
}

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
	// Note: We don't pad the original event because it's already signed,
	// and padding would invalidate the signature. We only pad wrapper events
	// (which we create and sign ourselves after padding).
	currentEvent := originalEvent

	logging.DebugMethod("client.wrapper", "WrapEvent", "Beginning nested wrapping in reverse order (last Renoter first)")

	// Wrap in reverse order (last Renoter first)
	for i := len(renterPath) - 1; i >= 0; i-- {
		renoterPubkeyBytes := renterPath[i]
		renoterPubkey := hex.EncodeToString(renoterPubkeyBytes)

		logging.DebugMethod("client.wrapper", "WrapEvent", "Wrapping layer %d/%d for Renoter pubkey: %s (first 16 chars: %s)", len(renterPath)-i, len(renterPath), renoterPubkey, renoterPubkey[:16])

		// Pad inner events BEFORE encrypting (padding is hidden inside encrypted content)
		// We don't pad outer wrapper events because they're public and padding would leak metadata
		// IMPORTANT: Pad WITHOUT modifying ID or signature. The server will remove padding tags
		// before publishing, restoring the original event structure with valid signature.
		logging.DebugMethod("client.wrapper", "WrapEvent", "Padding inner event before encryption (preserving original ID and signature, layer %d)", i)
		paddedEvent, err := padEventToMultipleOf32(currentEvent)
		if err != nil {
			logging.Error("client.wrapper.WrapEvent: failed to pad inner event at layer %d: %v", i, err)
			return nil, fmt.Errorf("failed to pad inner event: %w", err)
		}

		// Keep original ID and signature - don't recalculate them
		// The padding tags will be removed by the server before publishing, restoring the original event
		// with valid signature. The ID in the event is for the unpadded structure.
		idPrefix := ""
		if len(paddedEvent.ID) >= 16 {
			idPrefix = paddedEvent.ID[:16]
		}
		logging.DebugMethod("client.wrapper", "WrapEvent", "Inner event padded to multiple of 32 (original ID=%s signature preserved, layer %d)", idPrefix, i)

		// Serialize padded inner event for encryption
		logging.DebugMethod("client.wrapper", "WrapEvent", "Serializing padded inner event to JSON (layer %d)", i)
		eventJSON, err := json.Marshal(paddedEvent)
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

		// DON'T pad wrapper events - they're public and padding would leak metadata about size
		// Compute ID and sign the wrapper event as-is
		wrapperEvent.ID = wrapperEvent.GetID()
		if !wrapperEvent.CheckID() {
			logging.Error("client.wrapper.WrapEvent: wrapper event ID %s failed CheckID validation (layer %d)", wrapperEvent.ID, i)
			return nil, fmt.Errorf("invalid wrapper event ID at layer %d", i)
		}

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

	// Check final wrapper event size (after all encryption layers)
	// Serialize the final wrapper event to check its size
	finalEventJSON, err := json.Marshal(currentEvent)
	if err != nil {
		logging.Error("client.wrapper.WrapEvent: failed to serialize final wrapper event to check size: %v", err)
		return nil, fmt.Errorf("failed to serialize final wrapper event: %w", err)
	}
	finalSize := len(finalEventJSON)
	if finalSize > MaxWrappedEventSize {
		logging.Error("client.wrapper.WrapEvent: final wrapped event size %d bytes exceeds maximum %d bytes", finalSize, MaxWrappedEventSize)
		return nil, fmt.Errorf("final wrapped event size %d bytes exceeds maximum %d bytes", finalSize, MaxWrappedEventSize)
	}
	logging.Info("client.wrapper.WrapEvent: Successfully wrapped event through %d Renoter layers, final wrapper event ID: %s, size: %d bytes", len(renterPath), currentEvent.ID, finalSize)
	return currentEvent, nil
}
