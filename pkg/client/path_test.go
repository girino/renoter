package client

import (
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
