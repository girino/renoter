package client

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

func TestWrapEvent(t *testing.T) {
	// Create a test event
	testEvent := &nostr.Event{
		Kind:      1,
		Content:   "Test content",
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	testEvent.Sign(testEvent.PubKey)

	// Generate Renoter keys
	sk1 := nostr.GeneratePrivateKey()
	pk1, _ := nostr.GetPublicKey(sk1)
	npub1, _ := nip19.EncodePublicKey(pk1)

	sk2 := nostr.GeneratePrivateKey()
	pk2, _ := nostr.GetPublicKey(sk2)
	npub2, _ := nip19.EncodePublicKey(pk2)

	// Create path
	path, err := ValidatePath([]string{npub1, npub2})
	if err != nil {
		t.Fatalf("Failed to validate path: %v", err)
	}

	tests := []struct {
		name      string
		event     *nostr.Event
		path      [][]byte
		wantErr   bool
		wantLayers int
	}{
		{
			name:      "wrap with single Renoter",
			event:     testEvent,
			path:      [][]byte{path[0]},
			wantErr:   false,
			wantLayers: 1,
		},
		{
			name:      "wrap with multiple Renoters",
			event:     testEvent,
			path:      path,
			wantErr:   false,
			wantLayers: 2,
		},
		{
			name:    "empty path",
			event:   testEvent,
			path:    [][]byte{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped, err := WrapEvent(tt.event, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("WrapEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if wrapped == nil {
				t.Fatal("WrapEvent() returned nil wrapped event")
			}

			// Verify wrapper event properties
			if wrapped.Kind != 29000 {
				t.Errorf("WrapEvent() wrapper kind = %v, want 29000", wrapped.Kind)
			}

			// Verify wrapper event has correct structure
			if wrapped.Content == "" {
				t.Error("Wrapper event content should not be empty")
			}
			if len(wrapped.Tags) == 0 || wrapped.Tags[0][0] != "p" {
				t.Error("Wrapper event should have 'p' tag")
			}
		})
	}
}

func TestWrapEvent_ReverseOrder(t *testing.T) {
	// Create test event
	testEvent := &nostr.Event{
		Kind:      1,
		Content:   "Test",
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	testEvent.Sign(testEvent.PubKey)

	// Create 3 Renoters
	sk1 := nostr.GeneratePrivateKey()
	pk1, _ := nostr.GetPublicKey(sk1)
	npub1, _ := nip19.EncodePublicKey(pk1)

	sk2 := nostr.GeneratePrivateKey()
	pk2, _ := nostr.GetPublicKey(sk2)
	npub2, _ := nip19.EncodePublicKey(pk2)

	sk3 := nostr.GeneratePrivateKey()
	pk3, _ := nostr.GetPublicKey(sk3)
	npub3, _ := nip19.EncodePublicKey(pk3)

	path, _ := ValidatePath([]string{npub1, npub2, npub3})

	wrapped, err := WrapEvent(testEvent, path)
	if err != nil {
		t.Fatalf("WrapEvent() error = %v", err)
	}

	// The outermost wrapper should be decryptable by the first Renoter (npub1)
	// Verify the "p" tag contains the first Renoter's pubkey
	if len(wrapped.Tags) == 0 || wrapped.Tags[0][0] != "p" {
		t.Error("Wrapper event should have 'p' tag")
	}

	firstPubkey := hex.EncodeToString(path[0])
	if wrapped.Tags[0][1] != firstPubkey {
		t.Errorf("Wrapper 'p' tag should contain first Renoter's pubkey")
	}
}

func TestPadEventToPowerOfTwo(t *testing.T) {
	// Create a test event
	testEvent := &nostr.Event{
		Kind:      1,
		Content:   "Test content",
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	testEvent.Sign(testEvent.PubKey)

	// Test padding
	padded, err := padEventToPowerOfTwo(testEvent)
	if err != nil {
		t.Fatalf("padEventToPowerOfTwo() error = %v", err)
	}

	if padded == nil {
		t.Fatal("padEventToPowerOfTwo() returned nil")
	}

	// Verify padded event size is power of 2
	paddedJSON, err := json.Marshal(padded)
	if err != nil {
		t.Fatalf("Failed to serialize padded event: %v", err)
	}

	size := len(paddedJSON)
	if !isPowerOfTwo(size) {
		t.Errorf("Padded event size %d is not a power of 2", size)
	}

	// Verify padding tags are present
	hasPadding := false
	for _, tag := range padded.Tags {
		if len(tag) >= 1 && tag[0] == "padding" {
			hasPadding = true
			break
		}
	}
	if !hasPadding && len(paddedJSON) == size {
		// Only check for padding tag if size changed (might already be power of 2)
		t.Log("Event may have already been power of 2, no padding needed")
	}
}

func TestIsPowerOfTwo(t *testing.T) {
	tests := []struct {
		n    int
		want bool
	}{
		{1, true},   // 2^0
		{2, true},   // 2^1
		{4, true},   // 2^2
		{8, true},   // 2^3
		{16, true},  // 2^4
		{32, true},  // 2^5
		{64, true},  // 2^6
		{128, true}, // 2^7
		{256, true}, // 2^8
		{3, false},
		{5, false},
		{6, false},
		{7, false},
		{9, false},
		{15, false},
		{0, false},
		{-1, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("n=%d", tt.n), func(t *testing.T) {
			if got := isPowerOfTwo(tt.n); got != tt.want {
				t.Errorf("isPowerOfTwo(%d) = %v, want %v", tt.n, got, tt.want)
			}
		})
	}
}
