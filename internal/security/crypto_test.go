package security

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestSecretBoxRoundTripAndNonceUniqueness(t *testing.T) {
	box, err := NewSecretBox(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatal(err)
	}
	first, err := box.Seal("pve-api-secret")
	if err != nil {
		t.Fatal(err)
	}
	second, err := box.Seal("pve-api-secret")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("expected unique ciphertext because every encryption must use a fresh nonce")
	}
	plain, err := box.Open(first)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "pve-api-secret" {
		t.Fatalf("unexpected plaintext %q", plain)
	}
}

func TestSecretBoxRejectsTampering(t *testing.T) {
	box, err := NewSecretBox(bytes.Repeat([]byte{0x24}, 32))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal("secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed) == 0 {
		t.Fatal("expected encrypted payload")
	}
	payload, err := base64.RawStdEncoding.DecodeString(sealed)
	if err != nil {
		t.Fatal(err)
	}
	payload[len(payload)-1] ^= 0x01
	sealed = base64.RawStdEncoding.EncodeToString(payload)
	if _, err := box.Open(sealed); err == nil {
		t.Fatal("expected tampered ciphertext to fail authentication")
	}
}

func TestSecretBoxEnforces256BitKey(t *testing.T) {
	for _, size := range []int{0, 16, 24, 31, 33} {
		if _, err := NewSecretBox(make([]byte, size)); err == nil {
			t.Fatalf("accepted %d-byte key", size)
		}
	}
}
