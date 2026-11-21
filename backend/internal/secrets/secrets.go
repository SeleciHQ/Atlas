// Package secrets provides envelope encryption for integration credentials:
// a random data key encrypts the credential; the master key wraps the data key.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// Sealed is an encrypted credential. Nonces are stored alongside ciphertexts.
type Sealed struct {
	Ciphertext   []byte
	WrappedKey   []byte
	Nonce        []byte
	WrappedNonce []byte
	KeyVersion   int
}

// Sealer encrypts and decrypts credentials with a 256-bit master key.
type Sealer struct {
	master []byte
}

// NewSealer accepts a base64 or hex encoded 32-byte key.
func NewSealer(encoded string) (*Sealer, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) != 32 {
		key, err = hex.DecodeString(encoded)
	}
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("secret key must be 32 bytes encoded as base64 or hex")
	}
	return &Sealer{master: key}, nil
}

// Seal encrypts plaintext bound to aad (for example organization + integration).
func (s *Sealer) Seal(plaintext, aad []byte) (Sealed, error) {
	dataKey := make([]byte, 32)
	if _, err := rand.Read(dataKey); err != nil {
		return Sealed{}, fmt.Errorf("generate data key: %w", err)
	}
	payload, nonce, err := encrypt(dataKey, plaintext, aad)
	if err != nil {
		return Sealed{}, err
	}
	wrapped, wrappedNonce, err := encrypt(s.master, dataKey, aad)
	if err != nil {
		return Sealed{}, err
	}
	return Sealed{Ciphertext: payload, Nonce: nonce, WrappedKey: wrapped, WrappedNonce: wrappedNonce, KeyVersion: 1}, nil
}

// Open decrypts a sealed credential.
func (s *Sealer) Open(sealed Sealed, aad []byte) ([]byte, error) {
	dataKey, err := decrypt(s.master, sealed.WrappedKey, sealed.WrappedNonce, aad)
	if err != nil {
		return nil, fmt.Errorf("unwrap credential key: %w", err)
	}
	return decrypt(dataKey, sealed.Ciphertext, sealed.Nonce, aad)
}

func encrypt(key, plaintext, aad []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return gcm.Seal(nil, nonce, plaintext, aad), nonce, nil
}

func decrypt(key, ciphertext, nonce, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid nonce")
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}
