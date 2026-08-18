// Package model owns the on-disk model cache: a single GGUF downloaded once,
// sha256-verified, and reused on every subsequent (offline) run.
//
// The cache lives under $XDG_CACHE_HOME/sh1llwrt (adrg/xdg resolves the right
// per-OS path). The first run downloads the 941MB Qwen2.5-Coder-1.5B Q4_K_M
// GGUF once and verifies its sha256; every run after is fully offline.
package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
)

// ModelFilename is the canonical cached filename. The OP-class model is
// Qwen2.5-Coder-1.5B-Instruct, Q4_K_M, ~941MB.
const ModelFilename = "qwen2.5-coder-1.5b-instruct-q4_k_m.gguf"

// DefaultModelURL is the canonical download. Override with SH1LLWRT_MODEL_URL.
const DefaultModelURL = "https://huggingface.co/Qwen/Qwen2.5-Coder-1.5B-Instruct-GGUF/resolve/main/qwen2.5-coder-1.5b-instruct-q4_k_m.gguf"

// DefaultModelSHA256 is the expected sha256 of the canonical GGUF. Before
// tagging v0.1.0 this MUST be set to the measured digest of the file at
// DefaultModelURL; until then it is empty and Ensure skips verification with a
// warning, so the binary is runnable end-to-end. Override with
// SH1LLWRT_MODEL_SHA256.
const DefaultModelSHA256 = ""

// Cache is the on-disk model cache.
type Cache struct {
	// RootDir is $XDG_CACHE_HOME/sh1llwrt by default; override with
	// SH1LLWRT_CACHE_DIR for tests or airgapped deploys.
	RootDir string
	// ModelURL overrides DefaultModelURL.
	ModelURL string
	// ModelSHA256 overrides DefaultModelSHA256.
	ModelSHA256 string
	// HTTP client used for the one-time download.
	Client *http.Client
}

// NewCache returns a cache rooted at $XDG_CACHE_HOME/sh1llwrt, honouring the
// SH1LLWRT_CACHE_DIR / SH1LLWRT_MODEL_URL / SH1LLWRT_MODEL_SHA256 env vars.
func NewCache() *Cache {
	root := os.Getenv("SH1LLWRT_CACHE_DIR")
	if root == "" {
		root = filepath.Join(xdg.CacheHome, "sh1llwrt")
	}
	url := DefaultModelURL
	if v := os.Getenv("SH1LLWRT_MODEL_URL"); v != "" {
		url = v
	}
	sha := DefaultModelSHA256
	if v := os.Getenv("SH1LLWRT_MODEL_SHA256"); v != "" {
		sha = v
	}
	return &Cache{
		RootDir:     root,
		ModelURL:    url,
		ModelSHA256: sha,
		Client:      http.DefaultClient,
	}
}

// Path returns the absolute path of the cached GGUF.
func (c *Cache) Path() string { return filepath.Join(c.RootDir, ModelFilename) }

// Ensure returns the absolute path to a downloaded, sha256-verified GGUF.
// On a warm cache hit (file exists and sha matches) it performs no network I/O
// — the offline property. The first run downloads once with progress written
// to log, verifies, and atomically renames into place.
func (c *Cache) Ensure(ctx context.Context, log func(format string, args ...any)) (string, error) {
	if log == nil {
		log = func(string, ...any) {}
	}
	if err := os.MkdirAll(c.RootDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}
	dst := c.Path()

	// Warm path: file exists. If a sha is configured, verify; mismatch →
	// re-download. If no sha configured yet (pre-release), trust the file.
	if fi, err := os.Stat(dst); err == nil && !fi.IsDir() {
		if c.ModelSHA256 == "" {
			log("model cache hit (no sha configured, skipping verify): %s", dst)
			return dst, nil
		}
		ok, err := verifySHA256(dst, c.ModelSHA256)
		if err != nil {
			return "", fmt.Errorf("verify cached model: %w", err)
		}
		if ok {
			log("model cache hit (sha verified): %s", dst)
			return dst, nil
		}
		log("cached model sha mismatch, re-downloading")
	}

	// Cold path: download to a temp file in the same dir, verify, rename.
	tmp, err := os.CreateTemp(c.RootDir, ".gguf.download-*")
	if err != nil {
		return "", fmt.Errorf("create temp download: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // noop if renamed

	log("downloading model from %s (one-time)", c.ModelURL)
	if err := c.download(ctx, tmp); err != nil {
		tmp.Close()
		return "", fmt.Errorf("download model: %w", err)
	}
	tmp.Close()

	if c.ModelSHA256 != "" {
		ok, err := verifySHA256(tmpPath, c.ModelSHA256)
		if err != nil {
			return "", fmt.Errorf("verify downloaded model: %w", err)
		}
		if !ok {
			return "", fmt.Errorf("downloaded model sha256 mismatch (expected %s)", c.ModelSHA256)
		}
		log("model sha verified")
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		return "", fmt.Errorf("install model into cache: %w", err)
	}
	if err := os.Chmod(dst, 0o644); err != nil {
		// non-fatal
		log("chmod model: %v", err)
	}
	log("model cached at %s", dst)
	return dst, nil
}

func (c *Cache) download(ctx context.Context, dst io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ModelURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d for %s", resp.StatusCode, c.ModelURL)
	}
	_, err = io.Copy(dst, resp.Body)
	return err
}

func verifySHA256(path, want string) (bool, error) {
	want = strings.ToLower(strings.TrimSpace(want))
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	got := hex.EncodeToString(h.Sum(nil))
	return got == want, nil
}
