package identity

import (
	"testing"
	"time"
)

func TestOIDCStateRoundTrip(t *testing.T) {
	now := time.Now()
	state, err := NewOIDCState("org_123", "user_456", "", now)
	if err != nil {
		t.Fatalf("new state: %v", err)
	}
	sealed, err := SealOIDCState(state, "sk_test")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	opened, err := OpenOIDCState(sealed, "sk_test", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if opened.State != state.State || opened.OrganizationID != "org_123" || opened.LinkUserID != "user_456" {
		t.Fatalf("state mismatch: %+v", opened)
	}
}

func TestOIDCStateRejectsTampering(t *testing.T) {
	now := time.Now()
	state, _ := NewOIDCState("", "", "", now)
	sealed, _ := SealOIDCState(state, "sk_test")
	if _, err := OpenOIDCState(sealed+"x", "sk_test", now); err == nil {
		t.Fatal("tampered state accepted")
	}
	if _, err := OpenOIDCState(sealed, "different_key", now); err == nil {
		t.Fatal("state accepted with wrong key")
	}
}

func TestOIDCStateExpires(t *testing.T) {
	now := time.Now()
	state, _ := NewOIDCState("", "", "", now)
	sealed, _ := SealOIDCState(state, "sk_test")
	if _, err := OpenOIDCState(sealed, "sk_test", now.Add(11*time.Minute)); err == nil {
		t.Fatal("expired state accepted")
	}
}
