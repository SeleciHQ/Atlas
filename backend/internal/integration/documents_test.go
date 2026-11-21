package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"atlas/internal/documents"
	"atlas/internal/platform/apperr"
	"atlas/internal/storage"
)

func newDocumentHarness(t *testing.T) (*pgxpool.Pool, *pgxpool.Pool, string, *documents.Service, *storage.Local, *httptest.Server) {
	t.Helper()
	admin, app, _ := setupTenantSchema(t)
	organizationID := seedQueueOrganization(t, admin)
	dir := t.TempDir()
	local := storage.NewLocal(dir, []byte("test-storage-secret"), "")
	server := httptest.NewServer(local)
	t.Cleanup(server.Close)
	local = storage.NewLocal(dir, []byte("test-storage-secret"), server.URL)
	service := documents.NewService(app, local)
	return admin, app, organizationID, service, local, server
}

func checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func putObject(t *testing.T, request storage.Request, body []byte, contentType string) int {
	t.Helper()
	httpRequest, err := http.NewRequest(request.Method, request.URL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}
	for name, value := range request.Headers {
		httpRequest.Header.Set(name, value)
	}
	httpRequest.Header.Set("Content-Type", contentType)
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("upload object: %v", err)
	}
	defer response.Body.Close()
	return response.StatusCode
}

func TestDocumentUploadLifecycle(t *testing.T) {
	_, _, organizationID, service, _, _ := newDocumentHarness(t)
	ctx := context.Background()
	body := []byte("%PDF-1.7 test invoice")
	digest := checksum(body)

	authorization, err := service.CreateUpload(ctx, organizationID, "", "invoice", "application/pdf", int64(len(body)), digest, "standard")
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	if authorization.Document.Status != "pending_upload" || authorization.Upload.URL == "" {
		t.Fatalf("unexpected authorization: %+v", authorization)
	}
	if status := putObject(t, authorization.Upload, body, "application/pdf"); status != http.StatusNoContent {
		t.Fatalf("upload status = %d", status)
	}

	finalized, err := service.Finalize(ctx, organizationID, authorization.Document.ID)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if finalized.Status != "available" {
		t.Fatalf("finalized status = %q", finalized.Status)
	}

	document, download, err := service.Download(ctx, organizationID, authorization.Document.ID, "")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if document.Status != "available" || download.URL == "" {
		t.Fatalf("unexpected download: %+v %+v", document, download)
	}
	response, err := http.Get(download.URL)
	if err != nil {
		t.Fatalf("fetch download: %v", err)
	}
	defer response.Body.Close()
	got := new(bytes.Buffer)
	if _, err := got.ReadFrom(response.Body); err != nil {
		t.Fatalf("read download: %v", err)
	}
	if !bytes.Equal(got.Bytes(), body) {
		t.Fatalf("downloaded bytes differ")
	}
}

func TestDocumentValidationAndQuarantine(t *testing.T) {
	_, _, organizationID, service, local, _ := newDocumentHarness(t)
	ctx := context.Background()

	if _, err := service.CreateUpload(ctx, organizationID, "", "invoice", "application/zip", 10, checksum([]byte("x")), ""); err == nil {
		t.Fatal("unsupported content type was accepted")
	}
	if _, err := service.CreateUpload(ctx, organizationID, "", "invoice", "application/pdf", 30<<20, checksum([]byte("x")), ""); err == nil {
		t.Fatal("oversize document was accepted")
	}

	body := []byte("real content")
	digest := checksum(body)
	authorization, err := service.CreateUpload(ctx, organizationID, "", "contract", "application/pdf", int64(len(body)), digest, "")
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	if status := putObject(t, authorization.Upload, body, "application/pdf"); status != http.StatusNoContent {
		t.Fatalf("upload status = %d", status)
	}
	// Tamper with the stored metadata checksum: finalize must quarantine.
	metaPath := filepath.Join(localObjectDir(t, local), organizationID, authorization.Document.ID, "v1.meta.json")
	if err := os.WriteFile(metaPath, []byte(`{"content_type":"application/pdf","size":12,"sha256":"`+checksum([]byte("other"))+`"}`), 0o640); err != nil {
		t.Fatalf("tamper metadata: %v", err)
	}
	_, err = service.Finalize(ctx, organizationID, authorization.Document.ID)
	if err == nil {
		t.Fatal("finalize should fail on checksum mismatch")
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Code != "document_quarantined" {
		t.Fatalf("unexpected error: %v", err)
	}
	document, err := service.Get(ctx, organizationID, authorization.Document.ID)
	if err != nil {
		t.Fatalf("get document: %v", err)
	}
	if document.Status != "quarantined" {
		t.Fatalf("status = %q, want quarantined", document.Status)
	}
}

func localObjectDir(t *testing.T, local *storage.Local) string {
	t.Helper()
	// The harness stores under a temporary directory; the key is
	// organization/document/version, and the directory is the store root.
	return local.Root()
}
