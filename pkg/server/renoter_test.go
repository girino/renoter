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

func TestRenoter_ProcessEvent(t *testing.T) {
	ctx := context.Background()
	privateKey := nostr.GeneratePrivateKey()
	renoter, err := NewRenoter(ctx, privateKey, []string{"wss://relay.example.com"})
	if err != nil {
		t.Fatalf("Failed to create Renoter: %v", err)
	}

	// Create a valid event
	validEvent := &nostr.Event{
		Kind:      29000,
		Content:   "test",
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	validEvent.Sign(validEvent.PubKey)

	// Create an old event (more than 1 hour)
	oldTime := time.Now().Add(-2 * time.Hour)
	oldEvent := &nostr.Event{
		Kind:      29000,
		Content:   "test",
		CreatedAt: nostr.Timestamp(oldTime.Unix()),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	oldEvent.Sign(oldEvent.PubKey)

	tests := []struct {
		name    string
		event   *nostr.Event
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid event",
			event:   validEvent,
			wantErr: false,
		},
		{
			name:    "old event rejected",
			event:   oldEvent,
			wantErr: true,
			errMsg:  "too old",
		},
		{
			name:    "replay attack",
			event:   validEvent,
			wantErr: true,
			errMsg:  "already processed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := renoter.ProcessEvent(ctx, tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err != nil && err.Error() == "" {
					t.Errorf("ProcessEvent() error message should contain '%s'", tt.errMsg)
				}
			}
		})
	}
}

func TestRenoter_GetPublicKey(t *testing.T) {
	ctx := context.Background()
	privateKey := nostr.GeneratePrivateKey()
	expectedPubkey, _ := nostr.GetPublicKey(privateKey)

	renoter, err := NewRenoter(ctx, privateKey, []string{"wss://relay.example.com"})
	if err != nil {
		t.Fatalf("Failed to create Renoter: %v", err)
	}

	pubkey := renoter.GetPublicKey()
	if pubkey != expectedPubkey {
		t.Errorf("GetPublicKey() = %v, want %v", pubkey, expectedPubkey)
	}
}

func TestRenoter_GetRelayURLs(t *testing.T) {
	ctx := context.Background()
	privateKey := nostr.GeneratePrivateKey()
	relayURLs := []string{"wss://relay1.com", "wss://relay2.com"}

	renoter, err := NewRenoter(ctx, privateKey, relayURLs)
	if err != nil {
		t.Fatalf("Failed to create Renoter: %v", err)
	}

	urls := renoter.GetRelayURLs()
	if len(urls) != len(relayURLs) {
		t.Errorf("GetRelayURLs() length = %v, want %v", len(urls), len(relayURLs))
	}

	for i, url := range urls {
		if url != relayURLs[i] {
			t.Errorf("GetRelayURLs()[%d] = %v, want %v", i, url, relayURLs[i])
		}
	}
}

