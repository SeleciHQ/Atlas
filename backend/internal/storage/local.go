package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var validKey = regexp.MustCompile(`^[a-z0-9][a-z0-9/_-]{0,300}$`)

type localMeta struct {
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

// Local is a filesystem-backed store for development and tests. It serves
// HMAC-signed PUT/GET requests over HTTP at /v1/storage/local/object.
type Local struct {
	root    string
	secret  []byte
	baseURL string
	now     func() time.Time
}

// NewLocal constructs a local store rooted at dir. baseURL is the public API
// origin used in presigned URLs (for example http://localhost:8080).
func NewLocal(dir string, secret []byte, baseURL string) *Local {
	return &Local{root: dir, secret: secret, baseURL: strings.TrimSuffix(baseURL, "/"), now: time.Now}
}

// Root returns the local store root (tests and operators only).
func (l *Local) Root() string { return l.root }

func (l *Local) signature(method, key, contentHash string, expires int64, contentType string, size int64, sha string) string {
	mac := hmac.New(sha256.New, l.secret)
	fmt.Fprintf(mac, "%s\n%s\n%s\n%d\n%s\n%d\n%s", method, key, contentHash, expires, contentType, size, sha)
	return hex.EncodeToString(mac.Sum(nil))
}

func (l *Local) signQuery(method, key, contentType string, size int64, sha string, ttl time.Duration) string {
	expires := l.now().Add(ttl).Unix()
	values := url.Values{}
	values.Set("key", key)
	values.Set("method", method)
	values.Set("expires", strconv.FormatInt(expires, 10))
	values.Set("ct", contentType)
	values.Set("size", strconv.FormatInt(size, 10))
	values.Set("sha", sha)
	values.Set("sig", l.signature(method, key, "", expires, contentType, size, sha))
	return values.Encode()
}

// PresignPut returns a signed URL. The client must send the declared
// Content-Type and byte size; the local handler verifies the SHA-256.
func (l *Local) PresignPut(_ context.Context, key, contentType string, size int64, sha string, ttl time.Duration) (Request, error) {
	if !validKey.MatchString(key) {
		return Request{}, fmt.Errorf("invalid storage key")
	}
	if ttl <= 0 || ttl > 15*time.Minute {
		ttl = 15 * time.Minute
	}
	return Request{
		URL:     l.baseURL + "/v1/storage/local/object?" + l.signQuery(http.MethodPut, key, contentType, size, sha, ttl),
		Method:  http.MethodPut,
		Headers: map[string]string{"Content-Type": contentType, "X-Content-SHA256": sha},
	}, nil
}

// PresignGet returns a signed download URL.
func (l *Local) PresignGet(_ context.Context, key string, ttl time.Duration) (Request, error) {
	if !validKey.MatchString(key) {
		return Request{}, fmt.Errorf("invalid storage key")
	}
	if ttl <= 0 || ttl > 15*time.Minute {
		ttl = 15 * time.Minute
	}
	return Request{
		URL:     l.baseURL + "/v1/storage/local/object?" + l.signQuery(http.MethodGet, key, "", 0, "", ttl),
		Method:  http.MethodGet,
		Headers: map[string]string{},
	}, nil
}

func (l *Local) objectPath(key string) string {
	return filepath.Join(l.root, filepath.FromSlash(key))
}

// Head reports locally stored metadata.
func (l *Local) Head(_ context.Context, key string) (Object, error) {
	if !validKey.MatchString(key) {
		return Object{}, fmt.Errorf("invalid storage key")
	}
	data, err := os.ReadFile(l.objectPath(key) + ".meta.json")
	if err != nil {
		return Object{}, fmt.Errorf("read local object metadata: %w", err)
	}
	var meta localMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return Object{}, fmt.Errorf("decode local object metadata: %w", err)
	}
	return Object{ContentType: meta.ContentType, Size: meta.Size, SHA256: meta.SHA256}, nil
}

// Delete removes the object and its metadata.
func (l *Local) Delete(_ context.Context, key string) error {
	if !validKey.MatchString(key) {
		return fmt.Errorf("invalid storage key")
	}
	path := l.objectPath(key)
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete local object: %w", err)
	}
	_ = os.Remove(path + ".meta.json")
	return nil
}

// ServeHTTP implements the presigned local PUT/GET endpoints.
func (l *Local) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	key := query.Get("key")
	if !validKey.MatchString(key) {
		http.Error(w, "invalid key", http.StatusBadRequest)
		return
	}
	method := query.Get("method")
	if method != r.Method || (method != http.MethodPut && method != http.MethodGet) {
		http.Error(w, "method mismatch", http.StatusForbidden)
		return
	}
	expires, err := strconv.ParseInt(query.Get("expires"), 10, 64)
	if err != nil || l.now().Unix() > expires {
		http.Error(w, "expired", http.StatusForbidden)
		return
	}
	size, _ := strconv.ParseInt(query.Get("size"), 10, 64)
	expected := l.signature(method, key, "", expires, query.Get("ct"), size, query.Get("sha"))
	if !hmac.Equal([]byte(expected), []byte(query.Get("sig"))) {
		http.Error(w, "bad signature", http.StatusForbidden)
		return
	}

	switch method {
	case http.MethodGet:
		http.ServeFile(w, r, l.objectPath(key))
	case http.MethodPut:
		l.handlePut(w, r, key, query.Get("ct"), size, query.Get("sha"))
	}
}

func (l *Local) handlePut(w http.ResponseWriter, r *http.Request, key, contentType string, expectedSize int64, expectedSHA string) {
	path := l.objectPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	tempName := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}()

	hash := sha256.New()
	limited := io.LimitReader(r.Body, expectedSize+1)
	written, err := io.Copy(io.MultiWriter(temp, hash), limited)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if written != expectedSize {
		http.Error(w, "size mismatch", http.StatusBadRequest)
		return
	}
	if expectedSHA != "" {
		actual := hex.EncodeToString(hash.Sum(nil))
		if !hmac.Equal([]byte(actual), []byte(expectedSHA)) {
			http.Error(w, "checksum mismatch", http.StatusBadRequest)
			return
		}
	}
	if err := temp.Sync(); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if err := temp.Close(); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tempName, path); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	meta, _ := json.Marshal(localMeta{ContentType: contentType, Size: written, SHA256: expectedSHA})
	if err := os.WriteFile(path+".meta.json", meta, 0o640); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
