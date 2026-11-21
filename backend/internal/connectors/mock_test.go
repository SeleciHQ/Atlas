package connectors

import (
	"context"
	"testing"
)

func TestMockPagination(t *testing.T) {
	mock := NewMock()
	ctx := context.Background()
	cfg := ConnectionConfig{Settings: map[string]any{"page_size": 2}}

	cursor := Cursor("")
	seen := 0
	pages := 0
	for {
		page, err := mock.Sync(ctx, cfg, ObjectInvoice, cursor)
		if err != nil {
			t.Fatalf("sync: %v", err)
		}
		pages++
		seen += len(page.Records)
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
		if pages > 10 {
			t.Fatal("pagination did not terminate")
		}
	}
	if seen != 3 || pages != 2 {
		t.Fatalf("seen=%d pages=%d, want 3 and 2", seen, pages)
	}
	if _, err := mock.Sync(ctx, cfg, ObjectType("unknown"), ""); err == nil {
		t.Fatal("unsupported object accepted")
	}
}

func TestMockDeterministicPayload(t *testing.T) {
	mock := NewMock()
	first, err := mock.Sync(context.Background(), ConnectionConfig{}, ObjectSupplier, "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	second, err := mock.Sync(context.Background(), ConnectionConfig{}, ObjectSupplier, "")
	if err != nil {
		t.Fatalf("sync again: %v", err)
	}
	if len(first.Records) == 0 || string(first.Records[0].Payload) != string(second.Records[0].Payload) {
		t.Fatal("mock payloads are not deterministic")
	}
}

func TestMockFailureFixtures(t *testing.T) {
	mock := NewMock()
	page, err := mock.Sync(context.Background(), ConnectionConfig{Settings: map[string]any{"failure_mode": "rate_limited"}}, ObjectInvoice, "")
	if err != nil || page.RateLimit == nil || page.RateLimit.RetryAfter <= 0 {
		t.Fatalf("rate-limit fixture = %+v, %v", page, err)
	}
	if _, err := mock.Sync(context.Background(), ConnectionConfig{Settings: map[string]any{"failure_mode": "server_error"}}, ObjectInvoice, ""); err == nil {
		t.Fatal("server-error fixture did not fail")
	}
	if err := mock.ValidateConnection(context.Background(), ConnectionConfig{Settings: map[string]any{"require_credentials": true}}); err == nil {
		t.Fatal("missing credentials should fail validation")
	}
}
