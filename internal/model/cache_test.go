package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// knownBlob + its sha256 exercise the full download→verify→cache flow without
// the 941MB canonical model.
const knownBlob = "sh1llwrt-fake-gguf-blob-for-cache-test-0123456789"

func mustSHA(t *testing.T, s string) string {
	t.Helper()
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func newTestCache(t *testing.T, srv *httptest.Server, sha string) *Cache {
	t.Helper()
	dir := t.TempDir()
	return &Cache{
		RootDir:     dir,
		ModelURL:    srv.URL + "/model.gguf",
		ModelSHA256: sha,
		Client:      srv.Client(),
	}
}

func TestEnsureColdDownloadVerifiesAndCaches(t *testing.T) {
	sha := mustSHA(t, knownBlob)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, knownBlob)
	}))
	defer srv.Close()

	c := newTestCache(t, srv, sha)
	var logs []string
	logger := func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }

	p, err := c.Ensure(context.Background(), logger)
	if err != nil {
		t.Fatalf("Ensure cold: %v", err)
	}
	if p != c.Path() {
		t.Errorf("path = %q, want %q", p, c.Path())
	}
	got, err := os.ReadFile(c.Path())
	if err != nil {
		t.Fatalf("read cached: %v", err)
	}
	if string(got) != knownBlob {
		t.Errorf("cached content mismatch")
	}
	if !contains(logs, "downloading model") {
		t.Errorf("expected download log, got %v", logs)
	}
	if !contains(logs, "model sha verified") {
		t.Errorf("expected verify log, got %v", logs)
	}
}

func TestEnsureWarmCacheHitIsOffline(t *testing.T) {
	sha := mustSHA(t, knownBlob)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, knownBlob)
	}))
	defer srv.Close()

	c := newTestCache(t, srv, sha)

	// Cold pass to populate the cache.
	if _, err := c.Ensure(context.Background(), nil); err != nil {
		t.Fatalf("Ensure cold: %v", err)
	}
	calls = 0

	// Warm pass must NOT hit the server.
	var logs []string
	logger := func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	if _, err := c.Ensure(context.Background(), logger); err != nil {
		t.Fatalf("Ensure warm: %v", err)
	}
	if calls != 0 {
		t.Errorf("warm cache made %d server calls (want 0)", calls)
	}
	if !contains(logs, "model cache hit (sha verified)") {
		t.Errorf("expected verified hit log, got %v", logs)
	}
}

func TestEnsureTamperedShaTriggersRedownload(t *testing.T) {
	sha := mustSHA(t, knownBlob)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, knownBlob)
	}))
	defer srv.Close()
	c := newTestCache(t, srv, sha)

	if _, err := c.Ensure(context.Background(), nil); err != nil {
		t.Fatalf("cold: %v", err)
	}
	// Tamper the cached file so the sha no longer matches; Ensure must detect
	// it and re-download (server still serves the good blob).
	if err := os.WriteFile(c.Path(), []byte("corrupted"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if _, err := c.Ensure(context.Background(), nil); err != nil {
		t.Fatalf("Ensure after tamper: %v", err)
	}
	got, err := os.ReadFile(c.Path())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != knownBlob {
		t.Errorf("cache not repaired after tamper")
	}
}

func TestEnsureWrongShaRejects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, knownBlob)
	}))
	defer srv.Close()
	c := newTestCache(t, srv, strings.Repeat("deadbeef", 8)) // 64 hex, wrong
	_, err := c.Ensure(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Errorf("expected sha mismatch error, got %v", err)
	}
}

func TestEnsureNoShaTrustsExistingFile(t *testing.T) {
	// Pre-release behaviour: no sha configured → an existing file is trusted,
	// even if its bytes are nonsense. This keeps the binary runnable before the
	// canonical digest is measured.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, knownBlob)
	}))
	defer srv.Close()
	c := newTestCache(t, srv, "")
	if err := os.MkdirAll(c.RootDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.Path(), []byte("already-here"), 0o644); err != nil {
		t.Fatal(err)
	}
	var logs []string
	logger := func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	p, err := c.Ensure(context.Background(), logger)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if p != c.Path() {
		t.Errorf("path = %q", p)
	}
	if !contains(logs, "no sha configured") {
		t.Errorf("expected trust log, got %v", logs)
	}
}

func TestNewCacheHonoursEnv(t *testing.T) {
	dir := t.TempDir()
	url := "https://example.invalid/model.gguf"
	sha := strings.Repeat("a", 64)
	t.Setenv("SH1LLWRT_CACHE_DIR", dir)
	t.Setenv("SH1LLWRT_MODEL_URL", url)
	t.Setenv("SH1LLWRT_MODEL_SHA256", sha)
	c := NewCache()
	if c.RootDir != dir {
		t.Errorf("RootDir = %q, want %q", c.RootDir, dir)
	}
	if c.ModelURL != url {
		t.Errorf("ModelURL = %q", c.ModelURL)
	}
	if c.ModelSHA256 != sha {
		t.Errorf("ModelSHA256 = %q", c.ModelSHA256)
	}
	if c.Path() != filepath.Join(dir, ModelFilename) {
		t.Errorf("Path = %q", c.Path())
	}
}

func contains(slice []string, sub string) bool {
	for _, s := range slice {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
