package client

import (
	"fmt"
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

		pubkey, ok := data.([]byte)
		if !ok {
			logging.Error("client.path.ValidatePath: npub at index %d decoded to unexpected type", i)
			return nil, fmt.Errorf("npub at index %d decoded to unexpected type", i)
		}

		if len(pubkey) != 32 {
			logging.Error("client.path.ValidatePath: npub at index %d has invalid length: %d bytes (expected 32)", i, len(pubkey))
			return nil, fmt.Errorf("npub at index %d has invalid length: %d bytes (expected 32)", i, len(pubkey))
		}

		publicKeys[i] = pubkey
		logging.DebugMethod("client.path", "ValidatePath", "Successfully validated npub %d", i)
	}

	logging.Info("client.path.ValidatePath: Successfully validated all %d npubs in Renoter path", len(npubs))
	return publicKeys, nil
}

