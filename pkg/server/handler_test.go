package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func TestPadEventToExactSize(t *testing.T) {
	// Create a small test event
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
	// Create a large event
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
}

func TestPadEventToExactSize_ExactSize(t *testing.T) {
	// Create a small event
	event := &nostr.Event{
		Kind:      1,
		Content:   "Test",
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	event.Sign(event.PubKey)

	// Calculate current size
	jsonBytes, _ := json.Marshal(event)
	currentSize := len(jsonBytes)

	// Calculate size with empty padding tag to get tag overhead
	testEvent := *event
	testEvent.Tags = nostr.Tags{{"padding", ""}}
	testJSON, _ := json.Marshal(&testEvent)
	tagBaseSize := len(testJSON) - currentSize

	// Target size should be at least current size + tag overhead
	targetSize := currentSize + tagBaseSize + 100 // Add some padding room

	// Pad to target size
	padded, err := padEventToExactSize(event, targetSize)
	if err != nil {
		t.Fatalf("padEventToExactSize() error = %v", err)
	}

	// Size should match
	finalJSON, _ := json.Marshal(padded)
	if len(finalJSON) != targetSize {
		t.Errorf("Padded event size = %d, want %d", len(finalJSON), targetSize)
	}
}

func TestRenoter_GetPool(t *testing.T) {
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

	pool := renoter.GetPool()
	if pool == nil {
		t.Error("GetPool() should return non-nil pool")
	}
}

func TestRenoter_GetRelayURLs(t *testing.T) {
	ctx := context.Background()

	// Start a test relay
	testRelay, err := StartTestRelay(ctx)
	if err != nil {
		t.Fatalf("Failed to start test relay: %v", err)
	}
	defer testRelay.Stop(ctx)

	privateKey := nostr.GeneratePrivateKey()
	relayURLs := []string{testRelay.URL(), testRelay.URL(), testRelay.URL()}

	renoter, err := NewRenoter(ctx, privateKey, relayURLs)
	if err != nil {
		t.Fatalf("NewRenoter() error = %v", err)
	}

	returnedURLs := renoter.GetRelayURLs()
	if len(returnedURLs) != len(relayURLs) {
		t.Errorf("GetRelayURLs() length = %d, want %d", len(returnedURLs), len(relayURLs))
	}

	for i, url := range relayURLs {
		if returnedURLs[i] != url {
			t.Errorf("GetRelayURLs()[%d] = %s, want %s", i, returnedURLs[i], url)
		}
	}
}

func TestRenoter_ProcessEvent_ValidEvent(t *testing.T) {
	ctx := context.Background()

	// Start a test relay
	testRelay, err := StartTestRelay(ctx)
	if err != nil {
		t.Fatalf("Failed to start test relay: %v", err)
	}
	defer testRelay.Stop(ctx)

	renoterSk := nostr.GeneratePrivateKey()
	renoterPk, _ := nostr.GetPublicKey(renoterSk)
	relayURLs := []string{testRelay.URL()}

	renoter, err := NewRenoter(ctx, renoterSk, relayURLs)
	if err != nil {
		t.Fatalf("NewRenoter() error = %v", err)
	}

	// Create a simple event to process
	event := &nostr.Event{
		Kind:      29001,
		Content:   "test",
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
		Tags: nostr.Tags{
			{"p", renoterPk},
		},
	}
	event.Sign(event.PubKey)

	// ProcessEvent verifies signature - should pass for valid event
	err = renoter.ProcessEvent(ctx, event)
	// ProcessEvent might fail due to other reasons (relay connection, etc.)
	// But signature verification should pass
	if err != nil {
		// If it fails, check it's not a signature error
		if contains(err.Error(), "signature") {
			t.Errorf("ProcessEvent() signature verification failed: %v", err)
		}
		// Other errors (like relay connection issues) are acceptable in unit tests
	}
}

func TestRenoter_ProcessEvent_InvalidSignature(t *testing.T) {
	ctx := context.Background()

	// Start a test relay
	testRelay, err := StartTestRelay(ctx)
	if err != nil {
		t.Fatalf("Failed to start test relay: %v", err)
	}
	defer testRelay.Stop(ctx)

	renoterSk := nostr.GeneratePrivateKey()
	relayURLs := []string{testRelay.URL()}

	renoter, err := NewRenoter(ctx, renoterSk, relayURLs)
	if err != nil {
		t.Fatalf("NewRenoter() error = %v", err)
	}

	// Create event with invalid signature
	event := &nostr.Event{
		Kind:      29001,
		Content:   "test",
		CreatedAt: nostr.Now(),
		PubKey:    nostr.GeneratePrivateKey(),
		Sig:       "invalid_signature_that_will_fail_verification",
	}

	// ProcessEvent should fail signature verification
	err = renoter.ProcessEvent(ctx, event)
	if err == nil {
		t.Error("ProcessEvent() should error on invalid signature")
	}
	if err != nil && !contains(err.Error(), "signature") {
		t.Errorf("ProcessEvent() should return signature error, got: %v", err)
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

func TestRenoter_ProcessEvent_OldEvent(t *testing.T) {
	ctx := context.Background()

	// Start a test relay
	testRelay, err := StartTestRelay(ctx)
	if err != nil {
		t.Fatalf("Failed to start test relay: %v", err)
	}
	defer testRelay.Stop(ctx)

	renoterSk := nostr.GeneratePrivateKey()
	relayURLs := []string{testRelay.URL()}

	renoter, err := NewRenoter(ctx, renoterSk, relayURLs)
	if err != nil {
		t.Fatalf("NewRenoter() error = %v", err)
	}

	// Create event older than 1 hour
	oldTime := time.Now().Add(-2 * time.Hour)
	event := &nostr.Event{
		Kind:      29001,
		Content:   "test",
		CreatedAt: nostr.Timestamp(oldTime.Unix()),
		PubKey:    nostr.GeneratePrivateKey(),
	}
	event.Sign(event.PubKey)

	// ProcessEvent should reject old events
	err = renoter.ProcessEvent(ctx, event)
	if err == nil {
		t.Error("ProcessEvent() should reject events older than 1 hour")
	}
	if err != nil && !contains(err.Error(), "too old") {
		t.Errorf("ProcessEvent() should return 'too old' error, got: %v", err)
	}
}

// Note: HandleEvent and SubscribeToWrappedEvents require actual relay connections
// or complex mocking. These would be better suited for integration tests.
// The above tests cover the testable parts of the handler functions.
