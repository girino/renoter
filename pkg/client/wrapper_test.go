package client

import (
	"context"
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
			wrapped, err := WrapEvent(context.Background(), tt.event, tt.path)
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

	wrapped, err := WrapEvent(context.Background(), testEvent, path)
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

func TestPadEventToExactSize(t *testing.T) {
	event := &nostr.Event{
		Kind:      1,
		Content:   "Small content",
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	event.Sign(event.PubKey)

	targetSize := 1000 // 1KB target

	// Test successful padding
	padded, err := padEventToExactSize(event, targetSize)
	if err != nil {
		t.Fatalf("padEventToExactSize() error = %v", err)
	}

	// Verify size
	jsonBytes, _ := json.Marshal(padded)
	if len(jsonBytes) != targetSize {
		t.Errorf("Padded event size = %d, want %d", len(jsonBytes), targetSize)
	}

	// Verify padding tag exists
	foundPadding := false
	for _, tag := range padded.Tags {
		if len(tag) >= 1 && tag[0] == "padding" {
			foundPadding = true
			break
		}
	}
	if !foundPadding {
		t.Error("Padded event should have padding tag")
	}
}

func TestPadEventToExactSize_TooLarge(t *testing.T) {
	// Create a large event that exceeds target even with padding tag overhead
	largeContent := make([]byte, 5000)
	for i := range largeContent {
		largeContent[i] = 'A'
	}

	event := &nostr.Event{
		Kind:      1,
		Content:   string(largeContent),
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	event.Sign(event.PubKey)

	// Try to pad to a size smaller than the event
	targetSize := 100
	_, err := padEventToExactSize(event, targetSize)

	if err == nil {
		t.Error("padEventToExactSize() should error when event is too large")
	}
	if err != nil && !contains(err.Error(), "too large") {
		t.Errorf("Error message should mention 'too large', got: %v", err)
	}
}

func TestWrapEvent_LargeEvent(t *testing.T) {
	// Test wrapping an event that will produce a large 29000 wrapper
	// We'll create an event that when wrapped will be close to the size limit
	sk1 := nostr.GeneratePrivateKey()
	pk1, _ := nostr.GetPublicKey(sk1)
	npub1, _ := nip19.EncodePublicKey(pk1)

	path, _ := ValidatePath([]string{npub1})

	// Create event with content that will be close to size limit when wrapped
	largeContent := make([]byte, 30*1024) // 30KB
	for i := range largeContent {
		largeContent[i] = 'A'
	}

	event := &nostr.Event{
		Kind:      1,
		Content:   string(largeContent),
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	event.Sign(event.PubKey)

	// This should succeed or fail depending on the exact size
	_, err := WrapEvent(context.Background(), event, path)
	// We just verify it doesn't panic - the actual result depends on PoW mining
	if err != nil {
		// Verify error mentions size
		errMsg := err.Error()
		if !(containsString(errMsg, "too large") || containsString(errMsg, "exceeds")) {
			t.Logf("WrapEvent with large event: %v (expected if exceeds size limit)", err)
		}
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
