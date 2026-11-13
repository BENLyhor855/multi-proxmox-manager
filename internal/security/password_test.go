package security

import "testing"

func TestArgon2PasswordHash(t *testing.T) {
	password := "correct-horse-battery-staple-2026"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, password) {
		t.Fatal("expected matching password to verify")
	}
	if VerifyPassword(hash, "not-the-password") {
		t.Fatal("expected non-matching password to fail")
	}
}
