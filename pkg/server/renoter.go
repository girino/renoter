package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/girino/nostr-lib/logging"
	"github.com/nbd-wtf/go-nostr"
)

// Renoter represents a Renoter server that decrypts wrapper events
// and forwards them to the next Renoter or final destination.
type Renoter struct {
	// Private key for this Renoter (used for decryption)
	PrivateKey string

	// Public key derived from private key
	PublicKey string

	// Event store for replay detection (in-memory map of event IDs)
	// Kept small (max 5K entries) to fit in memory
	eventStore map[string]bool
	eventKeys  []string // Track insertion order for pruning
	eventMu    sync.RWMutex
	maxCacheSize int

	// SimplePool for managing multiple relay connections (used for both listening and forwarding)
	pool      *nostr.SimplePool
	relayURLs []string
}

// NewRenoter creates a new Renoter instance with a SimplePool for multiple relay connections.
func NewRenoter(ctx context.Context, privateKey string, relayURLs []string) (*Renoter, error) {
	logging.DebugMethod("server.renoter", "NewRenoter", "Creating new Renoter instance with %d relays", len(relayURLs))

	if privateKey == "" {
		logging.Error("server.renoter.NewRenoter: private key cannot be empty")
		return nil, fmt.Errorf("private key cannot be empty")
	}

	if len(relayURLs) == 0 {
		logging.Error("server.renoter.NewRenoter: relay URLs cannot be empty")
		return nil, fmt.Errorf("relay URLs cannot be empty")
	}

	logging.DebugMethod("server.renoter", "NewRenoter", "Deriving public key from private key")
	pubkey, err := nostr.GetPublicKey(privateKey)
	if err != nil {
		logging.Error("server.renoter.NewRenoter: failed to get public key: %v", err)
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	// Create SimplePool for managing relay connections
	pool := nostr.NewSimplePool(ctx)
	logging.DebugMethod("server.renoter", "NewRenoter", "Created SimplePool for %d relays", len(relayURLs))

	// Ensure all relays are available in the pool (they'll be connected on-demand)
	for _, url := range relayURLs {
		_, err := pool.EnsureRelay(url)
		if err != nil {
			logging.Error("server.renoter.NewRenoter: failed to ensure relay %s in pool: %v", url, err)
			return nil, fmt.Errorf("failed to ensure relay %s: %w", url, err)
		}
	}

	logging.Info("server.renoter.NewRenoter: Created Renoter instance, pubkey: %s (first 16 chars), %d relays", pubkey[:16], len(relayURLs))

		return &Renoter{
		PrivateKey:  privateKey,
		PublicKey:   pubkey,
		eventStore:  make(map[string]bool),
		eventKeys:   make([]string, 0, 5100), // Pre-allocate slightly more than max to reduce reallocations
		pool:        pool,
		relayURLs:   relayURLs,
		maxCacheSize: 5000,
	}, nil
}

// GetPool returns the SimplePool used by this Renoter.
func (r *Renoter) GetPool() *nostr.SimplePool {
	return r.pool
}

// GetRelayURLs returns the list of relay URLs used by this Renoter.
func (r *Renoter) GetRelayURLs() []string {
	return r.relayURLs
}

// ProcessEvent processes a wrapped event by verifying signature,
// decrypting one layer, and forwarding the inner event.
func (r *Renoter) ProcessEvent(ctx context.Context, event *nostr.Event) error {
	// Check for replay attacks - mark event as seen atomically to prevent race conditions
	r.eventMu.Lock()
	if r.eventStore[event.ID] {
		r.eventMu.Unlock()
		logging.Warn("server.renoter.ProcessEvent: Replay attack detected, event %s already processed", event.ID)
		return fmt.Errorf("event %s already processed (replay attack)", event.ID)
	}
	// Mark event as seen immediately (before any other processing) to prevent concurrent processing
	r.eventStore[event.ID] = true
	r.eventKeys = append(r.eventKeys, event.ID)
	
	// Prune cache if it exceeds max size (keep at most 5K entries)
	if len(r.eventKeys) > r.maxCacheSize {
		// Remove oldest entries (FIFO eviction)
		removeCount := len(r.eventKeys) - r.maxCacheSize
		for i := 0; i < removeCount; i++ {
			oldestID := r.eventKeys[i]
			delete(r.eventStore, oldestID)
		}
		// Keep only the recent entries
		r.eventKeys = r.eventKeys[removeCount:]
		logging.DebugMethod("server.renoter", "ProcessEvent", "Pruned replay cache: removed %d oldest entries, cache size now: %d", removeCount, len(r.eventKeys))
	}
	
	r.eventMu.Unlock()
	logging.DebugMethod("server.renoter", "ProcessEvent", "Atomically checked and marked event %s as seen in event store (cache size: %d)", event.ID, len(r.eventKeys))

	// Verify signature
	logging.DebugMethod("server.renoter", "ProcessEvent", "Verifying signature for event %s", event.ID)
	valid, err := event.CheckSignature()
	if err != nil {
		logging.Error("server.renoter.ProcessEvent: signature check failed for event %s: %v", event.ID, err)
		return fmt.Errorf("signature check failed: %w", err)
	}
	if !valid {
		logging.Error("server.renoter.ProcessEvent: invalid signature for event %s", event.ID)
		return fmt.Errorf("invalid signature for event %s", event.ID)
	}
	logging.DebugMethod("server.renoter", "ProcessEvent", "Signature verified successfully for event %s", event.ID)

	// Process will be handled by handler.go
	return nil
}

// GetPublicKey returns this Renoter's public key.
func (r *Renoter) GetPublicKey() string {
	return r.PublicKey
}
