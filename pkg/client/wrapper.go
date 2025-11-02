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

// padEventToExactSize adds padding tags to an event to make its serialized size exactly targetSize.
// Returns a new event with padding tags added, or an error if the base event is too large.
// Accounts for padding tag overhead before calculating padding needed.
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
	logging.DebugMethod("client.wrapper", "padEventToExactSize", "Padding tag base size: %d bytes", tagBaseSize)

	// Calculate total size including padding tag base
	totalSize := currentSize + tagBaseSize

	// Check if base event is too large
	if totalSize > targetSize {
		logging.Error("client.wrapper.padEventToExactSize: event base size %d (with tag overhead %d) exceeds target size %d", currentSize, tagBaseSize, targetSize)
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
		logging.Error("client.wrapper.padEventToExactSize: padded event size %d does not match target %d", len(finalJSON), targetSize)
		return nil, fmt.Errorf("padded event size %d does not match target %d", len(finalJSON), targetSize)
	}

	logging.DebugMethod("client.wrapper", "padEventToExactSize", "Added padding: %d bytes needed, event size: %d -> target: %d", paddingNeeded, currentSize, targetSize)

	return &paddedEvent, nil
}

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

	// After creating all 29000 layers, pad the outermost 29000 to exactly 4KB
	// and wrap it in a 29001 standardized container addressed to the first Renoter
	logging.DebugMethod("client.wrapper", "WrapEvent", "Padding outermost 29000 event to %d bytes", config.StandardizedSize)
	padded29000, err := padEventToExactSize(currentEvent, config.StandardizedSize)
	if err != nil {
		logging.Error("client.wrapper.WrapEvent: failed to pad outermost 29000 event: %v", err)
		return nil, fmt.Errorf("failed to pad outermost 29000 event: %w", err)
	}

	// Get first Renoter's pubkey for addressing the 29001 container
	firstRenoterPubkeyBytes := renterPath[0]
	firstRenoterPubkey := hex.EncodeToString(firstRenoterPubkeyBytes)

	// Serialize the padded 29000 for encryption
	padded29000JSON, err := json.Marshal(padded29000)
	if err != nil {
		logging.Error("client.wrapper.WrapEvent: failed to serialize padded 29000 event: %v", err)
		return nil, fmt.Errorf("failed to serialize padded 29000 event: %w", err)
	}

	// Generate random key for the 29001 container
	sk29001 := nostr.GeneratePrivateKey()
	pubkey29001, err := nostr.GetPublicKey(sk29001)
	if err != nil {
		logging.Error("client.wrapper.WrapEvent: failed to get public key for 29001: %v", err)
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	// Encrypt the padded 29000 for the first Renoter
	conversationKey29001, err := nip44.GenerateConversationKey(firstRenoterPubkey, sk29001)
	if err != nil {
		logging.Error("client.wrapper.WrapEvent: failed to generate conversation key for 29001: %v", err)
		return nil, fmt.Errorf("failed to generate conversation key: %w", err)
	}

	ciphertext29001, err := nip44.Encrypt(string(padded29000JSON), conversationKey29001)
	if err != nil {
		logging.Error("client.wrapper.WrapEvent: failed to encrypt for 29001: %v", err)
		return nil, fmt.Errorf("failed to encrypt for 29001: %w", err)
	}

	// Create 29001 standardized container event
	standardizedEvent := &nostr.Event{
		Kind:      config.StandardizedWrapperKind,
		Content:   ciphertext29001,
		CreatedAt: nostr.Now(),
		PubKey:    pubkey29001,
		Tags: nostr.Tags{
			// Add "p" tag with first Renoter's pubkey for routing
			{"p", firstRenoterPubkey},
		},
	}

	// Compute ID and sign the 29001 event
	standardizedEvent.ID = standardizedEvent.GetID()
	if !standardizedEvent.CheckID() {
		logging.Error("client.wrapper.WrapEvent: 29001 event ID %s failed CheckID validation", standardizedEvent.ID)
		return nil, fmt.Errorf("invalid 29001 event ID")
	}

	err = standardizedEvent.Sign(sk29001)
	if err != nil {
		logging.Error("client.wrapper.WrapEvent: failed to sign 29001 event: %v", err)
		return nil, fmt.Errorf("failed to sign 29001 event: %w", err)
	}

	logging.Info("client.wrapper.WrapEvent: Successfully wrapped event through %d Renoter layers, created 29001 container, ID: %s", len(renterPath), standardizedEvent.ID)
	return standardizedEvent, nil
}
