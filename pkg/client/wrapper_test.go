package client

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/girino/renoter/internal/config"
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
		name       string
		event      *nostr.Event
		path       [][]byte
		wantErr    bool
		wantLayers int
	}{
		{
			name:       "wrap with single Renoter",
			event:      testEvent,
			path:       [][]byte{path[0]},
			wantErr:    false,
			wantLayers: 1,
		},
		{
			name:       "wrap with multiple Renoters",
			event:      testEvent,
			path:       path,
			wantErr:    false,
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
			// The final event is now a 29001 standardized container
			if wrapped.Kind != config.StandardizedWrapperKind {
				t.Errorf("WrapEvent() wrapper kind = %v, want %d", wrapped.Kind, config.StandardizedWrapperKind)
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

func TestPadEventToMultipleOf32(t *testing.T) {
	// Create a test event
	testEvent := &nostr.Event{
		Kind:      1,
		Content:   "Test content",
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	testEvent.Sign(testEvent.PubKey)

	// Test padding
	padded, err := padEventToMultipleOf32(testEvent)
	if err != nil {
		t.Fatalf("padEventToMultipleOf32() error = %v", err)
	}

	if padded == nil {
		t.Fatal("padEventToMultipleOf32() returned nil")
	}

	// Verify padded event size is multiple of 32
	paddedJSON, err := json.Marshal(padded)
	if err != nil {
		t.Fatalf("Failed to serialize padded event: %v", err)
	}

	size := len(paddedJSON)
	if size%32 != 0 {
		t.Errorf("Padded event size %d is not a multiple of 32", size)
	}

	// Verify padding tags are present
	hasPadding := false
	for _, tag := range padded.Tags {
		if len(tag) >= 1 && tag[0] == "padding" {
			hasPadding = true
			break
		}
	}
	if !hasPadding && size%32 == 0 {
		// Only check for padding tag if size changed (might already be multiple of 32)
		t.Log("Event may have already been multiple of 32, no padding needed")
	}
}

func TestNextMultipleOf32(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want int
	}{
		{"n=1", 1, 32},
		{"n=31", 31, 32},
		{"n=32", 32, 32},
		{"n=33", 33, 64},
		{"n=64", 64, 64},
		{"n=65", 65, 96},
		{"n=100", 100, 128},
		{"n=128", 128, 128},
		{"n=129", 129, 160},
		{"n=0", 0, 32},
		{"n=-1", -1, 32},
		{"n=1024", 1024, 1024},
		{"n=1025", 1025, 1056},
		{"n=32768", 32768, 32768},
		{"n=32769", 32769, 32800},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextMultipleOf32(tt.n); got != tt.want {
				t.Errorf("nextMultipleOf32() = %v, want %v", got, tt.want)
			}
		})
	}
}
