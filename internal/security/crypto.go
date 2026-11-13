package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

type SecretBox struct {
	aead cipher.AEAD
}

func NewSecretBox(key []byte) (*SecretBox, error) {
	if len(key) != 32 {
		return nil, errors.New("AES-256 requires a 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{aead: aead}, nil
}

// Seal encrypts a credential using AES-256-GCM and a fresh nonce.
func (b *SecretBox) Seal(plain string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := b.aead.Seal(nil, nonce, []byte(plain), []byte("multi-proxmox-manager:v1"))
	payload := append(nonce, sealed...)
	return base64.RawStdEncoding.EncodeToString(payload), nil
}

func (b *SecretBox) Open(encoded string) (string, error) {
	payload, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	if len(payload) < b.aead.NonceSize()+b.aead.Overhead() {
		return "", errors.New("encrypted secret payload is invalid")
	}
	nonce := payload[:b.aead.NonceSize()]
	ciphertext := payload[b.aead.NonceSize():]
	plain, err := b.aead.Open(nil, nonce, ciphertext, []byte("multi-proxmox-manager:v1"))
	if err != nil {
		return "", errors.New("unable to decrypt stored credential")
	}
	return string(plain), nil
}
