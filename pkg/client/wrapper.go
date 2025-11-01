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

// padEventToMultipleOf64 adds padding tags to an event to make its serialized size a multiple of 64 bytes.
// Returns a new event with padding tags added, or the original event if already a multiple of 64.
// Accounts for padding tag overhead before calculating target size.
func padEventToMultipleOf64(event *nostr.Event) (*nostr.Event, error) {
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
	logging.DebugMethod("client.wrapper", "padEventToMultipleOf64", "Padding tag base size: %d bytes", tagBaseSize)

	// Calculate total size including padding tag base
	totalSize := currentSize + tagBaseSize
	logging.Info("client.wrapper.padEventToMultipleOf64: tagBaseSize=%d, currentSize=%d, totalSize=%d", tagBaseSize, currentSize, totalSize)

	// Find next multiple of 64 for the total size
	nextMultipleOf64 := nextMultipleOf64(totalSize)

	// Calculate exact padding needed in the padding string
	paddingNeeded := nextMultipleOf64 - totalSize

	// If no padding needed (already multiple of 64), return as-is
	if paddingNeeded == 0 {
		logging.DebugMethod("client.wrapper", "padEventToMultipleOf64", "Event already at multiple of 64 size: %d bytes", currentSize)
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

	logging.DebugMethod("client.wrapper", "padEventToMultipleOf64", "Added padding: %d bytes needed, event size: %d -> target: %d", paddingNeeded, currentSize, nextMultipleOf64)

	return &paddedEvent, nil
}

// nextMultipleOf64 returns the smallest multiple of 64 that is >= n.
func nextMultipleOf64(n int) int {
	if n <= 0 {
		return 64
	}
	if n%64 == 0 {
		return n
	}
	return ((n + 63) / 64) * 64
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
		// If the inner event was already signed, we need to clear the signature before padding,
		// then the signature will be recalculated when the event is eventually published
		logging.DebugMethod("client.wrapper", "WrapEvent", "Padding inner event before encryption (layer %d)", i)

		// For inner events that are wrapper events (created by us), they were signed AFTER padding in the previous iteration.
		// For original events (from user), they were signed BEFORE padding (by user).
		// Either way, we need to clear the signature, pad, then re-sign if needed.
		// But we only re-sign wrapper events (which we create). Original events need original author's key.
		innerEventCopy := *currentEvent
		isWrapperEvent := innerEventCopy.Kind == 29000 // Wrapper events are kind 29000
		hadSignature := innerEventCopy.Sig != ""

		// Clear signature before padding - padding changes event structure, invalidating any existing signature
		if hadSignature {
			innerEventCopy.Sig = ""
			if isWrapperEvent {
				logging.DebugMethod("client.wrapper", "WrapEvent", "Cleared signature from wrapper inner event before padding (will be re-signed after wrapping, layer %d)", i)
			} else {
				logging.DebugMethod("client.wrapper", "WrapEvent", "Cleared signature from original event before padding (signature will need original author's key to re-sign, layer %d)", i)
			}
		}

		paddedEvent, err := padEventToMultipleOf64(&innerEventCopy)
		if err != nil {
			logging.Error("client.wrapper.WrapEvent: failed to pad inner event at layer %d: %v", i, err)
			return nil, fmt.Errorf("failed to pad inner event: %w", err)
		}

		// Recalculate ID after padding to ensure it matches the padded structure
		paddedEvent.ID = paddedEvent.GetID()
		if !paddedEvent.CheckID() {
			logging.Error("client.wrapper.WrapEvent: padded inner event ID %s failed CheckID validation (layer %d)", paddedEvent.ID, i)
			return nil, fmt.Errorf("invalid inner event ID after padding at layer %d", i)
		}

		logging.DebugMethod("client.wrapper", "WrapEvent", "Inner event padded to multiple of 64 bytes (layer %d)", i)

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

	logging.Info("client.wrapper.WrapEvent: Successfully wrapped event through %d Renoter layers, final wrapper event ID: %s", len(renterPath), currentEvent.ID)
	return currentEvent, nil
}
