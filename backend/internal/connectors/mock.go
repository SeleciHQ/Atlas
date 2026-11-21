package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Mock is a deterministic connector used in tests and demos. Fixtures cover
// suppliers, purchase orders, delivery records and invoices.
type Mock struct{}

// NewMock constructs the mock connector.
func NewMock() Connector { return Mock{} }

var mockFixtures = map[ObjectType][]map[string]any{
	ObjectSupplier: {
		{"external_id": "sup-1", "name": "Alpha Supplies", "email": "billing@alpha.test", "currency": "EUR"},
		{"external_id": "sup-2", "name": "Beta Parts", "email": "billing@beta.test", "currency": "EUR"},
		{"external_id": "sup-3", "name": "Gamma Logistics", "email": "billing@gamma.test", "currency": "USD"},
	},
	ObjectPurchaseOrder: {
		{"external_id": "po-1001", "supplier_external_id": "sup-1", "number": "PO-1001", "total_minor": 125000, "currency": "EUR"},
		{"external_id": "po-1002", "supplier_external_id": "sup-2", "number": "PO-1002", "total_minor": 78000, "currency": "EUR"},
	},
	ObjectDelivery: {
		{"external_id": "dn-2001", "purchase_order_external_id": "po-1001", "number": "DN-2001", "delivered_on": "2026-11-01"},
		{"external_id": "dn-2002", "purchase_order_external_id": "po-1002", "number": "DN-2002", "delivered_on": "2026-11-03"},
	},
	ObjectInvoice: {
		{"external_id": "inv-3001", "supplier_external_id": "sup-1", "number": "INV-3001", "total_minor": 125000, "currency": "EUR"},
		{"external_id": "inv-3002", "supplier_external_id": "sup-2", "number": "INV-3002", "total_minor": 76000, "currency": "EUR"},
		{"external_id": "inv-3003", "supplier_external_id": "sup-3", "number": "INV-3003", "total_minor": 43000, "currency": "USD"},
	},
}

func (Mock) ValidateConnection(_ context.Context, cfg ConnectionConfig) error {
	if require, _ := cfg.Settings["require_credentials"].(bool); require && len(cfg.Credentials) == 0 {
		return fmt.Errorf("mock connector requires credentials")
	}
	return nil
}

func (Mock) DiscoverCapabilities(_ context.Context, _ ConnectionConfig) (Capabilities, error) {
	return Capabilities{
		Objects:     []ObjectType{ObjectSupplier, ObjectPurchaseOrder, ObjectDelivery, ObjectInvoice},
		MaxPageSize: 100,
	}, nil
}

func (Mock) Sync(_ context.Context, cfg ConnectionConfig, objectType ObjectType, cursor Cursor) (SyncPage, error) {
	switch mode, _ := cfg.Settings["failure_mode"].(string); mode {
	case "rate_limited":
		return SyncPage{RateLimit: &RateLimit{Remaining: 0, RetryAfter: time.Millisecond}}, nil
	case "server_error":
		return SyncPage{}, fmt.Errorf("mock upstream returned 500")
	}
	records, ok := mockFixtures[objectType]
	if !ok {
		return SyncPage{}, fmt.Errorf("mock connector does not support object %q", objectType)
	}
	offset := 0
	if cursor != "" {
		parsed, err := strconv.Atoi(string(cursor))
		if err != nil || parsed < 0 {
			return SyncPage{}, fmt.Errorf("invalid mock cursor %q", cursor)
		}
		offset = parsed
	}
	pageSize := 2
	if raw, ok := cfg.Settings["page_size"]; ok {
		switch value := raw.(type) {
		case int:
			pageSize = value
		case float64:
			pageSize = int(value)
		}
		if pageSize < 1 {
			pageSize = 1
		}
	}
	if offset >= len(records) {
		return SyncPage{Records: nil, NextCursor: Cursor(strconv.Itoa(offset)), HasMore: false}, nil
	}
	end := offset + pageSize
	if end > len(records) {
		end = len(records)
	}
	page := SyncPage{HasMore: end < len(records), NextCursor: Cursor(strconv.Itoa(end))}
	for _, fixture := range records[offset:end] {
		payload, err := json.Marshal(fixture)
		if err != nil {
			return SyncPage{}, err
		}
		page.Records = append(page.Records, Record{
			ExternalID: fixture["external_id"].(string),
			ObjectType: objectType,
			Payload:    payload,
			UpdatedAt:  time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		})
	}
	return page, nil
}

func (Mock) FetchDocument(context.Context, ConnectionConfig, DocumentRef) (DocumentStream, error) {
	return DocumentStream{}, fmt.Errorf("mock connector does not provide documents")
}

func (Mock) ExecuteWrite(context.Context, ConnectionConfig, WriteOperation) (WriteResult, error) {
	return WriteResult{}, fmt.Errorf("external writes are not permitted in Phase 4")
}

// DocumentInbox is the intake connector. Uploads are handled by the documents
// API; this adapter reports no pullable objects.
type DocumentInbox struct{}

// NewDocumentInbox constructs the document-inbox connector.
func NewDocumentInbox() Connector { return DocumentInbox{} }

func (DocumentInbox) ValidateConnection(context.Context, ConnectionConfig) error { return nil }

func (DocumentInbox) DiscoverCapabilities(context.Context, ConnectionConfig) (Capabilities, error) {
	return Capabilities{Objects: nil, MaxPageSize: 0}, nil
}

func (DocumentInbox) Sync(context.Context, ConnectionConfig, ObjectType, Cursor) (SyncPage, error) {
	return SyncPage{}, fmt.Errorf("document inbox is upload-driven")
}

func (DocumentInbox) FetchDocument(context.Context, ConnectionConfig, DocumentRef) (DocumentStream, error) {
	return DocumentStream{}, fmt.Errorf("document inbox documents arrive through /v1/documents")
}

func (DocumentInbox) ExecuteWrite(context.Context, ConnectionConfig, WriteOperation) (WriteResult, error) {
	return WriteResult{}, fmt.Errorf("external writes are not permitted in Phase 4")
}
