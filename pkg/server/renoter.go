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
	eventStore map[string]bool
	eventMu    sync.RWMutex

	// Relay connection for forwarding events
	forwardRelay *nostr.Relay
	forwardURL   string
}

// NewRenoter creates a new Renoter instance.
func NewRenoter(privateKey string, forwardRelayURL string) (*Renoter, error) {
	logging.DebugMethod("server.renoter", "NewRenoter", "Creating new Renoter instance, forward relay: %s", forwardRelayURL)

	if privateKey == "" {
		logging.Error("server.renoter.NewRenoter: private key cannot be empty")
		return nil, fmt.Errorf("private key cannot be empty")
	}

	logging.DebugMethod("server.renoter", "NewRenoter", "Deriving public key from private key")
	pubkey, err := nostr.GetPublicKey(privateKey)
	if err != nil {
		logging.Error("server.renoter.NewRenoter: failed to get public key: %v", err)
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	logging.Info("server.renoter.NewRenoter: Created Renoter instance, pubkey: %s (first 16 chars), forward relay: %s", pubkey[:16], forwardRelayURL)

	return &Renoter{
		PrivateKey:   privateKey,
		PublicKey:    pubkey,
		eventStore:   make(map[string]bool),
		forwardURL:   forwardRelayURL,
		forwardRelay: nil, // Will be connected when needed
	}, nil
}

// ConnectForwardRelay connects to the forward relay.
func (r *Renoter) ConnectForwardRelay(ctx context.Context) error {
	if r.forwardRelay != nil {
		logging.DebugMethod("server.renoter", "ConnectForwardRelay", "Forward relay already connected")
		return nil // Already connected
	}

	logging.DebugMethod("server.renoter", "ConnectForwardRelay", "Connecting to forward relay: %s", r.forwardURL)
	relay, err := nostr.RelayConnect(ctx, r.forwardURL)
	if err != nil {
		logging.Error("server.renoter.ConnectForwardRelay: failed to connect to forward relay %s: %v", r.forwardURL, err)
		return fmt.Errorf("failed to connect to forward relay: %w", err)
	}

	r.forwardRelay = relay
	logging.Info("server.renoter.ConnectForwardRelay: Successfully connected to forward relay: %s", r.forwardURL)
	return nil
}

// ProcessEvent processes a wrapped event by verifying signature,
// decrypting one layer, and forwarding the inner event.
func (r *Renoter) ProcessEvent(ctx context.Context, event *nostr.Event) error {
	logging.DebugMethod("server.renoter", "ProcessEvent", "Processing wrapped event: ID=%s, Kind=%d, PubKey=%s", event.ID, event.Kind, event.PubKey[:16])

	// Check for replay attacks
	r.eventMu.RLock()
	if r.eventStore[event.ID] {
		r.eventMu.RUnlock()
		logging.Warn("server.renoter.ProcessEvent: Replay attack detected, event %s already processed", event.ID)
		return fmt.Errorf("event %s already processed (replay attack)", event.ID)
	}
	r.eventMu.RUnlock()

	// Mark event as seen
	r.eventMu.Lock()
	r.eventStore[event.ID] = true
	r.eventMu.Unlock()
	logging.DebugMethod("server.renoter", "ProcessEvent", "Marked event %s as seen in event store", event.ID)

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

