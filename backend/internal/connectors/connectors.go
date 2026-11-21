// Package connectors defines the connector contract and adapter registry.
// Connectors translate provider data into source records; they never make
// canonical business decisions or write canonical tables.
package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// ObjectType is a provider object category.
type ObjectType string

const (
	ObjectSupplier      ObjectType = "supplier"
	ObjectPurchaseOrder ObjectType = "purchase_order"
	ObjectDelivery      ObjectType = "delivery_record"
	ObjectInvoice       ObjectType = "invoice"
)

// ConnectionConfig is the per-run connection context. Credentials are decrypted
// for one integration and one sync run; adapters never read database rows.
type ConnectionConfig struct {
	OrganizationID string
	IntegrationID  string
	Provider       string
	Settings       map[string]any
	Credentials    []byte
}

// Capabilities describes what an adapter supports.
type Capabilities struct {
	Objects     []ObjectType
	Webhooks    bool
	MaxPageSize int
}

// Cursor is an opaque provider cursor.
type Cursor string

// Record is one provider record in provider shape.
type Record struct {
	ExternalID string
	ObjectType ObjectType
	Payload    json.RawMessage
	UpdatedAt  time.Time
}

// RateLimit carries provider throttling hints.
type RateLimit struct {
	Remaining  int
	RetryAfter time.Duration
}

// SyncPage is one page of provider records.
type SyncPage struct {
	Records    []Record
	NextCursor Cursor
	HasMore    bool
	RateLimit  *RateLimit
}

// DocumentRef identifies a provider document.
type DocumentRef struct {
	ExternalID string
	ObjectType ObjectType
}

// DocumentStream is an immutable document plus integrity metadata.
type DocumentStream struct {
	Reader      io.ReadCloser
	ContentType string
	Size        int64
	SHA256      string
}

// WriteOperation is a permitted external write (stubbed until Phase 8).
type WriteOperation struct {
	Kind           string
	Payload        json.RawMessage
	IdempotencyKey string
}

// WriteResult is the outcome of an external write.
type WriteResult struct {
	ExternalID string
	Status     string
}

// Connector is the shared adapter contract.
type Connector interface {
	ValidateConnection(ctx context.Context, cfg ConnectionConfig) error
	DiscoverCapabilities(ctx context.Context, cfg ConnectionConfig) (Capabilities, error)
	Sync(ctx context.Context, cfg ConnectionConfig, objectType ObjectType, cursor Cursor) (SyncPage, error)
	FetchDocument(ctx context.Context, cfg ConnectionConfig, ref DocumentRef) (DocumentStream, error)
	ExecuteWrite(ctx context.Context, cfg ConnectionConfig, op WriteOperation) (WriteResult, error)
}

// Factory builds a connector for a provider.
type Factory func() Connector

// Registry resolves provider names to adapters.
type Registry struct {
	mutex     sync.RWMutex
	factories map[string]Factory
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: map[string]Factory{}}
}

// Register adds a provider adapter.
func (r *Registry) Register(provider string, factory Factory) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.factories[provider] = factory
}

// Open returns a connector for provider.
func (r *Registry) Open(provider string) (Connector, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	factory, ok := r.factories[provider]
	if !ok {
		return nil, fmt.Errorf("unknown connector provider %q", provider)
	}
	return factory(), nil
}
