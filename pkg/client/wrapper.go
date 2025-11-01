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

// padEventToPowerOfTwo adds padding tags to an event to make its serialized size a power of 2.
// Returns a new event with padding tags added, or the original event if already a power of 2.
// Accounts for ID, signature, and padding tag overhead before calculating target size.
func padEventToPowerOfTwo(event *nostr.Event) (*nostr.Event, error) {
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

	// Calculate additional sizes that will be added if not present:
	// - ID: 64 hex chars = 64 bytes in JSON string field (with quotes)
	// - Signature: 128 hex chars = 128 bytes in JSON string field (with quotes)
	// - Padding tag base: ["padding",""] = ~18 bytes
	// In JSON: "id":"64hexchars" = 2 + 64 + 2 = 68 bytes, but we'll be more precise
	// Let's estimate: "id":"..." with quotes and comma = ~70 bytes
	// "sig":"..." = ~134 bytes
	// ["padding",""] = ~18 bytes

	idSize := 0
	if paddedEvent.ID == "" {
		// ID is 64 hex chars, JSON encoded as "id":"64chars" = 70 bytes
		idSize = 70
	}

	sigSize := 0
	if paddedEvent.Sig == "" {
		// Signature is 128 hex chars, JSON encoded as "sig":"128chars" = 134 bytes
		sigSize = 134
	}

	// Padding tag base size: ["padding",""] = ~18 bytes in JSON
	tagBaseSize := 18

	// Calculate total size including missing fields and padding tag
	totalSize := currentSize + idSize + sigSize + tagBaseSize

	// Find next power of 2 for the total size
	nextPowerOf2 := nextPowerOfTwo(totalSize)

	// Calculate exact padding needed
	paddingNeeded := nextPowerOf2 - totalSize

	// If no padding needed (already power of 2), return as-is
	if paddingNeeded == 0 {
		logging.DebugMethod("client.wrapper", "padEventToPowerOfTwo", "Event already at power of 2 size: %d bytes", currentSize)
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

	logging.DebugMethod("client.wrapper", "padEventToPowerOfTwo", "Added padding: %d bytes needed, event size: %d -> target: %d", paddingNeeded, currentSize, nextPowerOf2)

	return &paddedEvent, nil
}

// nextPowerOfTwo returns the smallest power of 2 that is >= n.
func nextPowerOfTwo(n int) int {
	if n <= 0 {
		return 1
	}
	if isPowerOfTwo(n) {
		return n
	}
	// Find next power of 2 using bit manipulation
	power := 1
	for power < n {
		power <<= 1
	}
	return power
}

// isPowerOfTwo checks if a number is a power of 2.
func isPowerOfTwo(n int) bool {
	if n <= 0 {
		return false
	}
	return (n & (n - 1)) == 0
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
	currentEvent := originalEvent

	logging.DebugMethod("client.wrapper", "WrapEvent", "Beginning nested wrapping in reverse order (last Renoter first)")

	// Wrap in reverse order (last Renoter first)
	for i := len(renterPath) - 1; i >= 0; i-- {
		renoterPubkeyBytes := renterPath[i]
		renoterPubkey := hex.EncodeToString(renoterPubkeyBytes)

		logging.DebugMethod("client.wrapper", "WrapEvent", "Wrapping layer %d/%d for Renoter pubkey: %s (first 16 chars: %s)", len(renterPath)-i, len(renterPath), renoterPubkey, renoterPubkey[:16])

		// Pad event to power of 2 before encrypting
		logging.DebugMethod("client.wrapper", "WrapEvent", "Padding event to power of 2 (layer %d)", i)
		paddedEvent, err := padEventToPowerOfTwo(currentEvent)
		if err != nil {
			logging.Error("client.wrapper.WrapEvent: failed to pad event at layer %d: %v", i, err)
			return nil, fmt.Errorf("failed to pad event: %w", err)
		}
		logging.DebugMethod("client.wrapper", "WrapEvent", "Event padded to power of 2 (layer %d)", i)

		// Serialize padded event to JSON
		logging.DebugMethod("client.wrapper", "WrapEvent", "Serializing padded event to JSON (layer %d)", i)
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
