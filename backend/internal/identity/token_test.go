package identity

import (
	"strings"
	"testing"
)

func TestSessionTokensAreOpaqueAndHashed(t *testing.T) {
	first, firstHash, err := NewSessionToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	second, secondHash, err := NewSessionToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	if first == second {
		t.Fatal("tokens must be unique")
	}
	if first == firstHash {
		t.Fatal("token must not equal its stored hash")
	}
	if len(firstHash) != 64 || strings.Contains(firstHash, first) {
		t.Fatalf("unexpected hash: %q", firstHash)
	}
	if HashSessionToken(first) != firstHash {
		t.Fatal("hash must be deterministic")
	}
	if HashSessionToken("  "+first+"  ") != firstHash {
		t.Fatal("hash must tolerate surrounding whitespace")
	}
	if secondHash == firstHash {
		t.Fatal("different tokens must have different hashes")
	}
}
