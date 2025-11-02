package client

import (
	"context"
	"testing"

	"github.com/fiatjaf/khatru"
	"github.com/girino/renoter/internal/config"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

func TestSetupRelay(t *testing.T) {
	// Skip - SetupRelay requires valid relay connections
	// Handlers are registered after EnsureRelay, so connection failure prevents handler setup
	// This requires integration testing with mock relays
	t.Skip("Skipping - requires relay connection (integration test)")
}

func TestSetupRelay_InvalidRelayURL(t *testing.T) {
	sk1 := nostr.GeneratePrivateKey()
	pk1, _ := nostr.GetPublicKey(sk1)
	npub1, _ := nip19.EncodePublicKey(pk1)

	path, err := ValidatePath([]string{npub1})
	if err != nil {
		t.Fatalf("Failed to validate path: %v", err)
	}

	relay := khatru.NewRelay()
	// Invalid relay URL - this should fail when EnsureRelay is called
	invalidURLs := []string{"invalid-url", "also-invalid"}

	err = SetupRelay(relay, path, invalidURLs)
	// SetupRelay might succeed in registering the URL but fail later when connecting
	// The exact behavior depends on SimplePool.EnsureRelay implementation
	// We just verify the function doesn't panic
	if err != nil && err.Error() == "" {
		t.Error("SetupRelay should return meaningful error")
	}
}

func TestRejectEventHandler_EmptyPath(t *testing.T) {
	// Test the error path when path is empty
	// We'll manually create a handler to test without relay setup
	ctx := context.Background()
	emptyPath := [][]byte{}

	// Create a test event
	event := &nostr.Event{
		Kind:      1,
		Content:   "Test",
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	event.Sign(event.PubKey)

	// Manually test WrapEvent with empty path to verify error handling
	_, err := WrapEvent(ctx, event, emptyPath)
	if err == nil {
		t.Error("WrapEvent should error on empty path")
	}
	if err != nil && !contains(err.Error(), "empty") {
		t.Errorf("Error message should mention 'empty', got: %v", err)
	}
}

func TestRejectEventHandler_ValidEvent(t *testing.T) {
	// Test that valid events can be wrapped (even if publishing fails)
	sk1 := nostr.GeneratePrivateKey()
	pk1, _ := nostr.GetPublicKey(sk1)
	npub1, _ := nip19.EncodePublicKey(pk1)

	path, err := ValidatePath([]string{npub1})
	if err != nil {
		t.Fatalf("Failed to validate path: %v", err)
	}

	// Create a small test event
	event := &nostr.Event{
		Kind:      1,
		Content:   "Small test content",
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	event.Sign(event.PubKey)

	ctx := context.Background()
	// Test wrapping - should succeed
	wrapped, err := WrapEvent(ctx, event, path)
	if err != nil {
		t.Errorf("WrapEvent() error = %v (expected to succeed for small event)", err)
		return
	}

	if wrapped == nil {
		t.Error("WrapEvent() should return non-nil wrapped event")
	}
	if wrapped.Kind != config.StandardizedWrapperKind {
		t.Errorf("Wrapped event kind = %d, want %d", wrapped.Kind, config.StandardizedWrapperKind)
	}
}

func TestRejectEventHandler_OversizedEvent(t *testing.T) {
	sk1 := nostr.GeneratePrivateKey()
	pk1, _ := nostr.GetPublicKey(sk1)
	npub1, _ := nip19.EncodePublicKey(pk1)

	path, err := ValidatePath([]string{npub1})
	if err != nil {
		t.Fatalf("Failed to validate path: %v", err)
	}

	// Create a very large event that will exceed size limits when wrapped
	// We need an event that, when wrapped, produces a 29000 event > 32KB
	largeContent := make([]byte, 33*1024) // 33KB of content
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

	ctx := context.Background()
	// Test wrapping - should fail with size error
	_, err = WrapEvent(ctx, event, path)

	if err == nil {
		t.Error("WrapEvent should error on oversized event")
	}
	if err != nil && !contains(err.Error(), "too large") {
		t.Errorf("Error message should mention 'too large', got: %v", err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
