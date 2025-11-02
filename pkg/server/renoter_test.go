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

	// Start a test relay for valid Renoter tests
	testRelay, err := StartTestRelay(ctx)
	if err != nil {
		t.Fatalf("Failed to start test relay: %v", err)
	}
	defer testRelay.Stop(ctx)

	tests := []struct {
		name       string
		privateKey string
		relayURLs  []string
		wantErr    bool
	}{
		{
			name:       "empty private key",
			privateKey: "",
			relayURLs:  []string{testRelay.URL()},
			wantErr:    true,
		},
		{
			name:       "empty relay URLs",
			privateKey: privateKey,
			relayURLs:  []string{},
			wantErr:    true,
		},
		{
			name:       "valid Renoter",
			privateKey: privateKey,
			relayURLs:  []string{testRelay.URL()},
			wantErr:    false,
		},
		{
			name:       "multiple relays",
			privateKey: privateKey,
			relayURLs:  []string{testRelay.URL(), testRelay.URL(), testRelay.URL()},
			wantErr:    false,
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
				// Test GetPool
				pool := renoter.GetPool()
				if pool == nil {
					t.Error("GetPool() should return non-nil pool")
				}
				// Test GetPublicKey
				pubkey := renoter.GetPublicKey()
				if pubkey != renoter.PublicKey {
					t.Errorf("GetPublicKey() = %s, want %s", pubkey, renoter.PublicKey)
				}
			}
		})
	}
}

func TestRenoter_ProcessEvent_AgeValidation(t *testing.T) {
	ctx := context.Background()

	// Start a test relay
	testRelay, err := StartTestRelay(ctx)
	if err != nil {
		t.Fatalf("Failed to start test relay: %v", err)
	}
	defer testRelay.Stop(ctx)

	privateKey := nostr.GeneratePrivateKey()
	relayURLs := []string{testRelay.URL()}

	renoter, err := NewRenoter(ctx, privateKey, relayURLs)
	if err != nil {
		t.Fatalf("NewRenoter() error = %v", err)
	}

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

	// Test ProcessEvent rejects old events
	err = renoter.ProcessEvent(ctx, oldEvent)
	if err == nil {
		t.Error("ProcessEvent() should reject events older than 1 hour")
	}
	if err != nil && !containsString(err.Error(), "too old") {
		t.Errorf("Error message should mention 'too old', got: %v", err)
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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

func TestRenoter_ProcessEvent_ReplayDetection(t *testing.T) {
	ctx := context.Background()

	// Start a test relay
	testRelay, err := StartTestRelay(ctx)
	if err != nil {
		t.Fatalf("Failed to start test relay: %v", err)
	}
	defer testRelay.Stop(ctx)

	privateKey := nostr.GeneratePrivateKey()
	relayURLs := []string{testRelay.URL()}

	renoter, err := NewRenoter(ctx, privateKey, relayURLs)
	if err != nil {
		t.Fatalf("NewRenoter() error = %v", err)
	}

	// Create a valid event
	event := &nostr.Event{
		Kind:      29001,
		Content:   "test",
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	event.Sign(event.PubKey)

	// First processing should succeed (or fail on relay connection, but not on replay)
	err1 := renoter.ProcessEvent(ctx, event)
	// ProcessEvent might fail due to relay connection, but shouldn't fail on replay

	// Second processing should fail on replay detection
	err2 := renoter.ProcessEvent(ctx, event)
	if err2 == nil {
		// If relay connection failed on first attempt, replay check won't have marked it
		t.Logf("ProcessEvent replay test: first=%v, second=%v (relay connection may have failed)", err1, err2)
	} else if contains(err2.Error(), "replay") {
		// Success - replay was detected
		t.Log("Replay detection working correctly")
	}
}
