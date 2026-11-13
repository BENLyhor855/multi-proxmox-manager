package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const issuer = "multi-proxmox-manager"

type SessionClaims struct {
	ID      string `json:"jti"`
	Subject string `json:"sub"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	Issuer  string `json:"iss"`
	Issued  int64  `json:"iat"`
	Expiry  int64  `json:"exp"`
}

func NewSession(secret []byte, userID, email, role string, lifetime time.Duration) (string, error) {
	if len(secret) < 32 || lifetime <= 0 || lifetime > 15*time.Minute {
		return "", errors.New("invalid session configuration")
	}
	id, err := RandomToken(32)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	claims := SessionClaims{
		ID:      id,
		Subject: userID,
		Email:   email,
		Role:    role,
		Issuer:  issuer,
		Issued:  now.Unix(),
		Expiry:  now.Add(lifetime).Unix(),
	}
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func ParseSession(secret []byte, token string) (SessionClaims, error) {
	var claims SessionClaims
	parts := strings.Split(token, ".")
	if len(secret) < 32 || len(token) > 4096 || len(parts) != 3 {
		return claims, errors.New("invalid session")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	var h struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err != nil || json.Unmarshal(header, &h) != nil || h.Alg != "HS256" || h.Typ != "JWT" {
		return claims, errors.New("invalid session header")
	}
	unsigned := parts[0] + "." + parts[1]
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claims, errors.New("invalid session")
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(unsigned))
	if !hmac.Equal(got, mac.Sum(nil)) {
		return claims, errors.New("invalid session")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(payload, &claims) != nil {
		return claims, errors.New("invalid session")
	}
	now := time.Now().UTC().Unix()
	if claims.Issuer != issuer || claims.ID == "" || claims.Subject == "" || claims.Expiry <= claims.Issued || claims.Expiry-claims.Issued > 900 || claims.Expiry <= now || claims.Issued > now+60 {
		return claims, errors.New("expired or invalid session")
	}
	return claims, nil
}

func RandomToken(n int) (string, error) {
	if n < 16 || n > 128 {
		return "", fmt.Errorf("random token length out of range")
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
