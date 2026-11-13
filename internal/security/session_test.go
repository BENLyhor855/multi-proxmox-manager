package security

import (
	"strings"
	"testing"
	"time"
)

func TestSessionRoundTrip(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	token, err := NewSession(key, "42", "admin@example.com", "admin", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseSession(key, token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "42" || claims.Email != "admin@example.com" || claims.Role != "admin" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}

func TestSessionRejectsTampering(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	token, err := NewSession(key, "42", "admin@example.com", "admin", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	parts[1] = parts[1] + "x"
	if _, err := ParseSession(key, strings.Join(parts, ".")); err == nil {
		t.Fatal("expected modified token to be rejected")
	}
}

func TestSessionUniqueAndExpired(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	a, _ := NewSession(key, "1", "a@example.com", "admin", time.Minute)
	b, _ := NewSession(key, "1", "a@example.com", "admin", time.Minute)
	if a == b {
		t.Fatal("sessions must have unique IDs")
	}
	expired, _ := NewSession(key, "1", "a@example.com", "admin", time.Nanosecond)
	if _, err := ParseSession(key, expired); err == nil {
		t.Fatal("expired session accepted")
	}
	if _, err := NewSession([]byte("short"), "1", "a", "admin", time.Minute); err == nil {
		t.Fatal("weak signing key accepted")
	}
	if _, err := NewSession(key, "1", "a", "admin", time.Hour); err == nil {
		t.Fatal("excessive lifetime accepted")
	}
}
