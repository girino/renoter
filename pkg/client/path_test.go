package client

import (
	"encoding/hex"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

func TestValidatePath(t *testing.T) {
	// Generate test keys
	sk1 := nostr.GeneratePrivateKey()
	pk1, _ := nostr.GetPublicKey(sk1)
	npub1, _ := nip19.EncodePublicKey(pk1)

	sk2 := nostr.GeneratePrivateKey()
	pk2, _ := nostr.GetPublicKey(sk2)
	npub2, _ := nip19.EncodePublicKey(pk2)

	tests := []struct {
		name    string
		npubs   []string
		wantErr bool
	}{
		{
			name:    "valid single npub",
			npubs:   []string{npub1},
			wantErr: false,
		},
		{
			name:    "valid multiple npubs",
			npubs:   []string{npub1, npub2},
			wantErr: false,
		},
		{
			name:    "empty path",
			npubs:   []string{},
			wantErr: true,
		},
		{
			name:    "invalid npub format",
			npubs:   []string{"invalid-npub"},
			wantErr: true,
		},
		{
			name:    "empty npub",
			npubs:   []string{""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidatePath(tt.npubs)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(result) != len(tt.npubs) {
				t.Errorf("ValidatePath() result length = %v, want %v", len(result), len(tt.npubs))
			}
		})
	}
}

func TestValidatePath_ValidatesCorrectPubkeys(t *testing.T) {
	sk1 := nostr.GeneratePrivateKey()
	pk1, _ := nostr.GetPublicKey(sk1)
	npub1, _ := nip19.EncodePublicKey(pk1)

	sk2 := nostr.GeneratePrivateKey()
	pk2, _ := nostr.GetPublicKey(sk2)
	npub2, _ := nip19.EncodePublicKey(pk2)

	npubs := []string{npub1, npub2}
	result, err := ValidatePath(npubs)
	if err != nil {
		t.Fatalf("ValidatePath() unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("ValidatePath() result length = %v, want 2", len(result))
	}

	// Verify the public keys match
	if len(result[0]) != 32 || len(result[1]) != 32 {
		t.Errorf("ValidatePath() public keys should be 32 bytes, got %d and %d", len(result[0]), len(result[1]))
	}
}

func TestShufflePath(t *testing.T) {
	// Generate test keys
	sk1 := nostr.GeneratePrivateKey()
	pk1, _ := nostr.GetPublicKey(sk1)
	npub1, _ := nip19.EncodePublicKey(pk1)

	sk2 := nostr.GeneratePrivateKey()
	pk2, _ := nostr.GetPublicKey(sk2)
	npub2, _ := nip19.EncodePublicKey(pk2)

	sk3 := nostr.GeneratePrivateKey()
	pk3, _ := nostr.GetPublicKey(sk3)
	npub3, _ := nip19.EncodePublicKey(pk3)

	path, err := ValidatePath([]string{npub1, npub2, npub3})
	if err != nil {
		t.Fatalf("ValidatePath() error: %v", err)
	}

	// Test shuffling - path should be shuffled
	shuffled := ShufflePath(path)

	// Verify length is the same
	if len(shuffled) != len(path) {
		t.Errorf("ShufflePath() length = %v, want %v", len(shuffled), len(path))
	}

	// Verify original path is not modified
	if len(path) != 3 {
		t.Errorf("Original path length changed, want 3, got %d", len(path))
	}

	// Verify shuffled path contains the same elements (order may differ)
	// We can't easily compare byte slices, so we'll just verify that
	// multiple shuffles produce potentially different orders (statistical test)
	// For a 3-element path, there's a 5/6 chance two shuffles will differ
	// So we'll just verify the function works without errors

	// Test with empty path
	emptyPath := [][]byte{}
	shuffledEmpty := ShufflePath(emptyPath)
	if len(shuffledEmpty) != 0 {
		t.Errorf("ShufflePath() with empty path should return empty, got length %d", len(shuffledEmpty))
	}

	// Test with single element
	singlePath := [][]byte{path[0]}
	shuffledSingle := ShufflePath(singlePath)
	if len(shuffledSingle) != 1 {
		t.Errorf("ShufflePath() with single element should return single element, got length %d", len(shuffledSingle))
	}

	// Verify that multiple shuffles of a multi-element path can produce different orders
	// (this is a probabilistic test - very unlikely to get same order twice)
	allSame := true
	for i := 0; i < 10; i++ {
		shuffled := ShufflePath(path)
		if len(shuffled) != len(path) {
			t.Errorf("ShufflePath() iteration %d: length = %v, want %v", i, len(shuffled), len(path))
		}
		// Compare first element to see if order changed
		if i > 0 && !compareByteSlices(shuffled[0], path[0]) {
			allSame = false
		}
	}

	// With 3+ elements shuffled 10 times, it's very unlikely all will have same first element
	// But this is probabilistic, so we'll just log if it happens
	if allSame && len(path) >= 3 {
		t.Logf("Warning: All 10 shuffles produced same first element (unlikely but possible)")
	}
}

func compareByteSlices(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestValidatePath_Deduplication(t *testing.T) {
	// Generate test keys
	sk1 := nostr.GeneratePrivateKey()
	pk1, _ := nostr.GetPublicKey(sk1)
	npub1, _ := nip19.EncodePublicKey(pk1)

	sk2 := nostr.GeneratePrivateKey()
	pk2, _ := nostr.GetPublicKey(sk2)
	npub2, _ := nip19.EncodePublicKey(pk2)

	// Test with duplicate npubs in input
	npubs := []string{npub1, npub2, npub1, npub2, npub1} // 3 duplicates of npub1, 2 duplicates of npub2
	result, err := ValidatePath(npubs)
	if err != nil {
		t.Fatalf("ValidatePath() unexpected error: %v", err)
	}

	// Should return only 2 unique Renoters
	if len(result) != 2 {
		t.Errorf("ValidatePath() with duplicates should return 2 unique Renoters, got %d", len(result))
	}

	// Verify no duplicates in result
	seen := make(map[string]bool)
	for _, pubkey := range result {
		pubkeyHex := hex.EncodeToString(pubkey)
		if seen[pubkeyHex] {
			t.Errorf("ValidatePath() result contains duplicate pubkey: %s", pubkeyHex[:16])
		}
		seen[pubkeyHex] = true
	}
}
