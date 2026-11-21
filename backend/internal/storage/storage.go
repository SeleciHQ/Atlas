// Package storage abstracts private object storage. Uploads and downloads are
// authorized by the caller before a short-lived URL is issued; keys are always
// server-chosen.
package storage

import (
	"context"
	"time"
)

// Object is the metadata read back from storage.
type Object struct {
	ContentType string
	Size        int64
	SHA256      string
}

// Request is a presigned object operation.
type Request struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
}

// Store is the object-storage contract. Implementations: Local and S3 (R2).
type Store interface {
	PresignPut(ctx context.Context, key, contentType string, size int64, sha256 string, ttl time.Duration) (Request, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (Request, error)
	Head(ctx context.Context, key string) (Object, error)
	Delete(ctx context.Context, key string) error
}
