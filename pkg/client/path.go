package client

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"time"

	"github.com/girino/nostr-lib/logging"
	"github.com/nbd-wtf/go-nostr/nip19"
)

// ValidatePath validates a slice of npub strings using NIP-19 decoding.
// Returns an error if any npub is invalid, otherwise returns the decoded public keys.
func ValidatePath(npubs []string) ([][]byte, error) {
	logging.DebugMethod("client.path", "ValidatePath", "Validating Renoter path with %d npubs", len(npubs))

	if len(npubs) == 0 {
		logging.Error("client.path.ValidatePath: path cannot be empty")
		return nil, fmt.Errorf("path cannot be empty")
	}

	publicKeys := make([][]byte, len(npubs))
	for i, npub := range npubs {
		logging.DebugMethod("client.path", "ValidatePath", "Validating npub %d/%d: %s", i+1, len(npubs), npub)
		if npub == "" {
			logging.Error("client.path.ValidatePath: npub at index %d is empty", i)
			return nil, fmt.Errorf("npub at index %d is empty", i)
		}

		logging.DebugMethod("client.path", "ValidatePath", "Decoding npub %d with NIP-19", i)
		prefix, data, err := nip19.Decode(npub)
		if err != nil {
			logging.Error("client.path.ValidatePath: failed to decode npub at index %d: %v", i, err)
			return nil, fmt.Errorf("failed to decode npub at index %d: %w", i, err)
		}

		if prefix != "npub" {
			logging.Error("client.path.ValidatePath: npub at index %d has invalid prefix: %s", i, prefix)
			return nil, fmt.Errorf("npub at index %d is not a valid npub (prefix: %s)", i, prefix)
		}

		// nip19.Decode returns npub as a hex-encoded string, not []byte
		pubkeyHex, ok := data.(string)
		if !ok {
			logging.Error("client.path.ValidatePath: npub at index %d decoded to unexpected type (expected string, got %T)", i, data)
			return nil, fmt.Errorf("npub at index %d decoded to unexpected type", i)
		}

		// Decode hex string to bytes
		displayLen := 32
		if len(pubkeyHex) < displayLen {
			displayLen = len(pubkeyHex)
		}
		logging.DebugMethod("client.path", "ValidatePath", "Decoding hex pubkey: %s (first %d chars)", pubkeyHex[:displayLen], displayLen)
		pubkey, err := hex.DecodeString(pubkeyHex)
		if err != nil {
			logging.Error("client.path.ValidatePath: failed to decode hex pubkey at index %d: %v", i, err)
			return nil, fmt.Errorf("npub at index %d has invalid hex encoding: %w", i, err)
		}

		if len(pubkey) != 32 {
			logging.Error("client.path.ValidatePath: npub at index %d has invalid length: %d bytes (expected 32)", i, len(pubkey))
			return nil, fmt.Errorf("npub at index %d has invalid length: %d bytes (expected 32)", i, len(pubkey))
		}

		publicKeys[i] = pubkey
		logging.DebugMethod("client.path", "ValidatePath", "Successfully validated npub %d", i)
	}

	// Check for duplicate Renoters in the path
	// This prevents routing loops and ensures proper anonymization
	seen := make(map[string]int) // Map pubkey hex to first occurrence index

	for i, pubkey := range publicKeys {
		pubkeyHex := hex.EncodeToString(pubkey)
		if firstIndex, exists := seen[pubkeyHex]; exists {
			// Found duplicate - return error
			logging.Error("client.path.ValidatePath: Duplicate Renoter pubkey detected at index %d (duplicates index %d): %s (first 16 chars)", i, firstIndex, pubkeyHex[:16])
			return nil, fmt.Errorf("duplicate Renoters in path: npub at index %d duplicates npub at index %d (pubkey: %s...)", i, firstIndex, pubkeyHex[:16])
		}
		seen[pubkeyHex] = i
	}

	logging.Info("client.path.ValidatePath: Successfully validated all %d npubs in Renoter path (no duplicates)", len(npubs))
	return publicKeys, nil
}

// ShufflePath randomly shuffles the Renoter path to randomize routing order.
// This improves privacy by ensuring events don't always follow the same path.
// Returns a new slice with shuffled order (original slice is not modified).
func ShufflePath(path [][]byte) [][]byte {
	if len(path) <= 1 {
		// No need to shuffle if path has 0 or 1 Renoters
		return path
	}

	// Create a copy to avoid modifying the original
	shuffled := make([][]byte, len(path))
	copy(shuffled, path)

	// Use current time as seed for randomness
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	logging.DebugMethod("client.path", "ShufflePath", "Shuffled Renoter path with %d nodes", len(shuffled))
	return shuffled
}
