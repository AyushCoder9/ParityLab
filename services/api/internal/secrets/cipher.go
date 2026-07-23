package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

type Cipher struct {
	aead     cipher.AEAD
	indexKey [sha256.Size]byte
}

func New(encodedKey string) (*Cipher, error) {
	if encodedKey == "" {
		return nil, errors.New("PARITYLAB_ENCRYPTION_KEY is required")
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(encodedKey)
	}
	if err != nil || len(key) != 32 {
		return nil, errors.New("PARITYLAB_ENCRYPTION_KEY must be base64-encoded 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead, indexKey: sha256.Sum256(append([]byte("paritylab:blind-index:v1:"), key...))}, nil
}

func (c *Cipher) BlindIndex(value string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, c.indexKey[:])
	_, _ = mac.Write([]byte(value))
	var result [sha256.Size]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func (c *Cipher) Encrypt(plaintext []byte, associatedData string) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate encryption nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, plaintext, []byte(associatedData)), nil
}

func (c *Cipher) Decrypt(ciphertext []byte, associatedData string) ([]byte, error) {
	if len(ciphertext) < c.aead.NonceSize() {
		return nil, errors.New("encrypted secret is truncated")
	}
	nonce := ciphertext[:c.aead.NonceSize()]
	payload := ciphertext[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, payload, []byte(associatedData))
	if err != nil {
		return nil, errors.New("encrypted secret authentication failed")
	}
	return plaintext, nil
}
