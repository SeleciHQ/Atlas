// Package documents implements the authorized upload lifecycle for private
// documents: authorize -> presigned PUT -> finalize (size/checksum verified) ->
// available -> short-lived presigned download.
package documents

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/audit"
	"atlas/internal/platform/apperr"
	"atlas/internal/storage"
	"atlas/internal/tenancy"
)

const (
	maxSizeBytes = 25 << 20
	uploadTTL    = 15 * time.Minute
	downloadTTL  = 5 * time.Minute
)

var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

var allowedKinds = map[string]struct{}{
	"invoice": {}, "credit_note": {}, "purchase_order": {}, "delivery_note": {}, "contract": {}, "other": {},
}

var allowedContentTypes = map[string]string{
	"application/pdf": ".pdf",
	"image/png":       ".png",
	"image/jpeg":      ".jpg",
	"image/tiff":      ".tiff",
	"text/csv":        ".csv",
}

// Document is document metadata returned by the service.
type Document struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Status         string    `json:"status"`
	CurrentVersion int       `json:"current_version"`
	RetentionClass string    `json:"retention_class"`
	VersionID      string    `json:"version_id"`
	ContentType    string    `json:"content_type"`
	SizeBytes      int64     `json:"size_bytes"`
	SHA256         string    `json:"sha256"`
	CreatedAt      time.Time `json:"created_at"`
}

// UploadAuthorization is returned when an upload is authorized.
type UploadAuthorization struct {
	Document Document
	Upload   storage.Request
}

// Service coordinates document metadata, storage and audit.
type Service struct {
	pool  *pgxpool.Pool
	store storage.Store
}

// NewService constructs the document service.
func NewService(pool *pgxpool.Pool, store storage.Store) *Service {
	return &Service{pool: pool, store: store}
}

// CreateUpload validates the request, creates a pending document/version and
// returns a short-lived presigned PUT for a server-chosen key.
func (s *Service) CreateUpload(ctx context.Context, organizationID, actorID, kind, contentType string, size int64, sha256, retentionClass string) (*UploadAuthorization, error) {
	kind = strings.TrimSpace(kind)
	if _, ok := allowedKinds[kind]; !ok {
		return nil, apperr.New(apperr.KindInvalid, "unsupported_document_kind", "Unsupported document kind.")
	}
	contentType = strings.TrimSpace(strings.ToLower(contentType))
	if _, ok := allowedContentTypes[contentType]; !ok {
		return nil, apperr.New(apperr.KindInvalid, "unsupported_content_type", "Unsupported document content type.")
	}
	if size <= 0 || size > maxSizeBytes {
		return nil, apperr.New(apperr.KindInvalid, "invalid_size", "Document size must be between 1 byte and 25 MiB.")
	}
	sha256 = strings.ToLower(strings.TrimSpace(sha256))
	if !sha256Pattern.MatchString(sha256) {
		return nil, apperr.New(apperr.KindInvalid, "invalid_checksum", "A lowercase hex SHA-256 checksum is required.")
	}
	if retentionClass == "" {
		retentionClass = "standard"
	}

	documentID := uuid.NewString()
	versionID := uuid.NewString()
	key := fmt.Sprintf("%s/%s/v1", organizationID, documentID)
	document := Document{
		ID: documentID, Kind: kind, Status: "pending_upload", CurrentVersion: 1,
		RetentionClass: retentionClass, VersionID: versionID, ContentType: contentType,
		SizeBytes: size, SHA256: sha256,
	}

	var actor any
	if actorID != "" {
		actor = actorID
	}
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO documents (organization_id, id, kind, status, current_version, retention_class, created_by)
			VALUES ($1, $2, $3, 'pending_upload', 1, $4, $5)`,
			organizationID, documentID, kind, retentionClass, actor); err != nil {
			return fmt.Errorf("create document: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO document_versions
				(organization_id, id, document_id, version, content_type, size_bytes, sha256, storage_key)
			VALUES ($1, $2, $3, 1, $4, $5, $6, $7)`,
			organizationID, versionID, documentID, contentType, size, sha256, key); err != nil {
			return fmt.Errorf("create document version: %w", err)
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: "human", ActorID: actorID,
			Action: "document.upload.authorized", ResourceType: "document", ResourceID: documentID,
			Decision: "allow",
			Metadata: map[string]any{"kind": kind, "content_type": contentType, "size_bytes": size, "sha256": sha256},
		})
	})
	if err != nil {
		return nil, err
	}

	upload, err := s.store.PresignPut(ctx, key, contentType, size, sha256, uploadTTL)
	if err != nil {
		return nil, fmt.Errorf("presign document upload: %w", err)
	}
	return &UploadAuthorization{Document: document, Upload: upload}, nil
}

// Finalize verifies the stored object and transitions the document to
// available, or quarantines it on any integrity mismatch.
func (s *Service) Finalize(ctx context.Context, organizationID, documentID string) (*Document, error) {
	var document Document
	var key string
	var versionStatus string
	var uploadedAt *time.Time
	var quarantineErr error
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT d.id, d.kind, d.status, d.current_version, d.retention_class, d.created_at,
			       v.id, v.content_type, v.size_bytes, v.sha256, v.storage_key, v.status, v.uploaded_at
			FROM documents d JOIN document_versions v
			  ON v.organization_id = d.organization_id AND v.document_id = d.id AND v.version = d.current_version
			WHERE d.organization_id = $1 AND d.id = $2
			FOR UPDATE OF d, v`, organizationID, documentID).Scan(
			&document.ID, &document.Kind, &document.Status, &document.CurrentVersion, &document.RetentionClass,
			&document.CreatedAt, &document.VersionID, &document.ContentType, &document.SizeBytes, &document.SHA256,
			&key, &versionStatus, &uploadedAt); err != nil {
			if err == pgx.ErrNoRows {
				return apperr.New(apperr.KindNotFound, "document_not_found", "Document not found.")
			}
			return fmt.Errorf("load document: %w", err)
		}
		if versionStatus == "available" {
			return nil
		}
		if versionStatus != "pending_upload" {
			return apperr.New(apperr.KindConflict, "document_not_pending", "Document is not awaiting upload completion.")
		}

		object, headErr := s.store.Head(ctx, key)
		problem := ""
		switch {
		case headErr != nil:
			problem = "object_missing"
		case object.Size != document.SizeBytes:
			problem = "size_mismatch"
		case object.SHA256 == "":
			problem = "checksum_unavailable"
		case !strings.EqualFold(object.SHA256, document.SHA256):
			problem = "checksum_mismatch"
		}

		if problem != "" {
			if _, err := tx.Exec(ctx, `
				UPDATE document_versions SET status = 'quarantined' WHERE organization_id = $1 AND id = $2`,
				organizationID, document.VersionID); err != nil {
				return fmt.Errorf("quarantine version: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE documents SET status = 'quarantined', updated_at = now() WHERE organization_id = $1 AND id = $2`,
				organizationID, documentID); err != nil {
				return fmt.Errorf("quarantine document: %w", err)
			}
			if err := audit.Record(ctx, tx, audit.Event{
				OrganizationID: organizationID, ActorKind: "system", Action: "document.quarantined",
				ResourceType: "document", ResourceID: documentID, Decision: "deny",
				Metadata: map[string]any{"reason": problem},
			}); err != nil {
				return err
			}
			document.Status = "quarantined"
			// Commit the quarantine state, then surface the failure.
			quarantineErr = apperr.New(apperr.KindConflict, "document_quarantined", "Document integrity verification failed; the document is quarantined.")
			return nil
		}

		if _, err := tx.Exec(ctx, `
			UPDATE document_versions SET status = 'available', uploaded_at = now()
			WHERE organization_id = $1 AND id = $2`, organizationID, document.VersionID); err != nil {
			return fmt.Errorf("finalize version: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE documents SET status = 'available', updated_at = now() WHERE organization_id = $1 AND id = $2`,
			organizationID, documentID); err != nil {
			return fmt.Errorf("finalize document: %w", err)
		}
		if err := audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: "system", Action: "document.finalized",
			ResourceType: "document", ResourceID: documentID, Decision: "allow",
			Metadata: map[string]any{"size_bytes": document.SizeBytes, "sha256": document.SHA256},
		}); err != nil {
			return err
		}
		document.Status = "available"
		return nil
	})
	if err != nil {
		return nil, err
	}
	if quarantineErr != nil {
		return nil, quarantineErr
	}
	return &document, nil
}

// Download authorizes and presigns a short-lived GET for an available document.
func (s *Service) Download(ctx context.Context, organizationID, documentID, actorID string) (*Document, storage.Request, error) {
	var document Document
	var key string
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT d.id, d.kind, d.status, d.current_version, d.retention_class, d.created_at,
			       v.id, v.content_type, v.size_bytes, v.sha256, v.storage_key
			FROM documents d JOIN document_versions v
			  ON v.organization_id = d.organization_id AND v.document_id = d.id AND v.version = d.current_version
			WHERE d.organization_id = $1 AND d.id = $2`, organizationID, documentID).Scan(
			&document.ID, &document.Kind, &document.Status, &document.CurrentVersion, &document.RetentionClass,
			&document.CreatedAt, &document.VersionID, &document.ContentType, &document.SizeBytes, &document.SHA256,
			&key); err != nil {
			if err == pgx.ErrNoRows {
				return apperr.New(apperr.KindNotFound, "document_not_found", "Document not found.")
			}
			return fmt.Errorf("load document: %w", err)
		}
		if document.Status != "available" {
			return apperr.New(apperr.KindConflict, "document_unavailable", "Document is not available for download.")
		}
		return audit.Record(ctx, tx, audit.Event{
			OrganizationID: organizationID, ActorKind: "human", ActorID: actorID,
			Action: "document.download.authorized", ResourceType: "document", ResourceID: documentID,
			Decision: "allow",
		})
	})
	if err != nil {
		return nil, storage.Request{}, err
	}
	request, err := s.store.PresignGet(ctx, key, downloadTTL)
	if err != nil {
		return nil, storage.Request{}, fmt.Errorf("presign document download: %w", err)
	}
	return &document, request, nil
}

// Get returns document metadata scoped to the organization.
func (s *Service) Get(ctx context.Context, organizationID, documentID string) (*Document, error) {
	var document Document
	var key string
	err := tenancy.WithOrganizationTx(ctx, s.pool, organizationID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT d.id, d.kind, d.status, d.current_version, d.retention_class, d.created_at,
			       v.id, v.content_type, v.size_bytes, v.sha256, v.storage_key
			FROM documents d JOIN document_versions v
			  ON v.organization_id = d.organization_id AND v.document_id = d.id AND v.version = d.current_version
			WHERE d.organization_id = $1 AND d.id = $2`, organizationID, documentID).Scan(
			&document.ID, &document.Kind, &document.Status, &document.CurrentVersion, &document.RetentionClass,
			&document.CreatedAt, &document.VersionID, &document.ContentType, &document.SizeBytes, &document.SHA256,
			&key)
	})
	if err == pgx.ErrNoRows {
		return nil, apperr.New(apperr.KindNotFound, "document_not_found", "Document not found.")
	}
	if err != nil {
		return nil, err
	}
	return &document, nil
}
