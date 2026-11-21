package secrets

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	sealer, err := NewSealer(hex.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}
	aad := []byte("org|integration")
	sealed, err := sealer.Seal([]byte("api-key-secret"), aad)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Contains(sealed.Ciphertext, []byte("api-key-secret")) {
		t.Fatal("ciphertext contains plaintext")
	}
	plaintext, err := sealer.Open(sealed, aad)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(plaintext) != "api-key-secret" {
		t.Fatalf("plaintext = %q", plaintext)
	}
	if _, err := sealer.Open(sealed, []byte("other")); err == nil {
		t.Fatal("wrong AAD should fail")
	}
}

func TestSealerRejectsBadKey(t *testing.T) {
	if _, err := NewSealer("short"); err == nil {
		t.Fatal("short key accepted")
	}
}
