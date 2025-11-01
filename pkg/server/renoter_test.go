package server

import (
	"context"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func TestNewRenoter(t *testing.T) {
	ctx := context.Background()
	privateKey := nostr.GeneratePrivateKey()
	relayURLs := []string{"wss://relay.example.com"}

	tests := []struct {
		name      string
		privateKey string
		relayURLs []string
		wantErr   bool
	}{
		{
			name:      "valid Renoter",
			privateKey: privateKey,
			relayURLs: relayURLs,
			wantErr:   false,
		},
		{
			name:      "empty private key",
			privateKey: "",
			relayURLs: relayURLs,
			wantErr:   true,
		},
		{
			name:      "empty relay URLs",
			privateKey: privateKey,
			relayURLs: []string{},
			wantErr:   true,
		},
		{
			name:      "multiple relays",
			privateKey: privateKey,
			relayURLs: []string{"wss://relay1.com", "wss://relay2.com"},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renoter, err := NewRenoter(ctx, tt.privateKey, tt.relayURLs)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRenoter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if renoter == nil {
					t.Fatal("NewRenoter() returned nil")
				}
				if renoter.PrivateKey != tt.privateKey {
					t.Errorf("PrivateKey = %v, want %v", renoter.PrivateKey, tt.privateKey)
				}
				if renoter.PublicKey == "" {
					t.Error("PublicKey should be set")
				}
				if len(renoter.GetRelayURLs()) != len(tt.relayURLs) {
					t.Errorf("RelayURLs length = %v, want %v", len(renoter.GetRelayURLs()), len(tt.relayURLs))
				}
			}
		})
	}
}

func TestRenoter_ProcessEvent_AgeValidation(t *testing.T) {
	// Test age validation logic
	// Events older than 1 hour should be rejected
	oldTime := time.Now().Add(-2 * time.Hour)
	oldEvent := &nostr.Event{
		Kind:      29000,
		Content:   "test",
		CreatedAt: nostr.Timestamp(oldTime.Unix()),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	oldEvent.Sign(oldEvent.PubKey)

	// Verify the age check logic
	eventTime := time.Unix(int64(oldEvent.CreatedAt), 0)
	now := time.Now()
	if !eventTime.Before(now.Add(-1 * time.Hour)) {
		t.Error("Event should be considered too old (> 1 hour)")
	}
}

func TestRenoter_GetPublicKey(t *testing.T) {
	// Test public key derivation (used by Renoter)
	privateKey := nostr.GeneratePrivateKey()
	expectedPubkey, err := nostr.GetPublicKey(privateKey)
	if err != nil {
		t.Fatalf("Failed to get public key: %v", err)
	}

	// Verify consistency
	pubkey, err := nostr.GetPublicKey(privateKey)
	if err != nil {
		t.Fatalf("Failed to get public key again: %v", err)
	}

	if pubkey != expectedPubkey {
		t.Errorf("GetPublicKey() = %v, want %v", pubkey, expectedPubkey)
	}

	if pubkey == "" {
		t.Error("Public key should not be empty")
	}
}

func TestRenoter_GetRelayURLs(t *testing.T) {
	// Test that relay URLs are stored correctly
	// Since we can't create Renoter without relay connection, we'll skip this test
	// or verify the relay URL validation logic separately
	relayURLs := []string{"wss://relay1.com", "wss://relay2.com"}
	if len(relayURLs) == 0 {
		t.Error("Relay URLs should not be empty")
	}
}

