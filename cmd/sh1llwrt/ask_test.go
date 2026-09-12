package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SuperMarioYL/sh1llwrt/internal/llama"
	"github.com/SuperMarioYL/sh1llwrt/internal/model"
	"github.com/spf13/cobra"
)

// --- test doubles -----------------------------------------------------------

type recordingCache struct {
	called   bool
	deadline time.Time
}

func (c *recordingCache) Ensure(ctx context.Context, _ func(string, ...any)) (string, error) {
	c.called = true
	c.deadline, _ = ctx.Deadline()
	return "/tmp/fake-model.gguf", nil
}

type fakeRunner struct {
	deadline time.Time
	reply    string
}

func (f *fakeRunner) Complete(ctx context.Context, _ string) (string, error) {
	f.deadline, _ = ctx.Deadline()
	return f.reply, nil
}

func swapCache(t *testing.T, c modelCache) {
	t.Helper()
	orig := newCache
	newCache = func() modelCache { return c }
	t.Cleanup(func() { newCache = orig })
}

func swapRunner(t *testing.T, r llama.Runner) {
	t.Helper()
	orig := resolveRunner
	resolveRunner = func(llama.Options, bool) llama.Runner { return r }
	t.Cleanup(func() { resolveRunner = orig })
}

func resetAskFlags() { askFlags = askOptions{} }

// newTestCmd returns a command with a live context: cobra only sets one when
// run through Execute, and runAsk derives its timeout contexts from it.
func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	return cmd
}

// captureStdout swaps os.Stdout for a pipe and returns a flush function that
// restores it and returns everything written since.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	t.Cleanup(func() {
		if os.Stdout == w {
			os.Stdout = old
			w.Close()
		}
	})
	return func() string {
		os.Stdout = old
		w.Close()
		b, _ := io.ReadAll(r)
		return string(b)
	}
}

// withStdin replaces os.Stdin with content (an empty string means EOF).
func withStdin(t *testing.T, content string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if content != "" {
		w.WriteString(content)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })
}

// --- m5: the download and inference budgets are separate --------------------

func TestAskSplitBudgets(t *testing.T) {
	t.Setenv("SH1LLWRT_MOCK", "")
	cache := &recordingCache{}
	runner := &fakeRunner{reply: "<cmd>ls -la</cmd><safe>true</safe><explain>list files</explain>"}
	swapCache(t, cache)
	swapRunner(t, runner)
	resetAskFlags()

	if err := runAsk(newTestCmd(), []string{"list files"}); err != nil {
		t.Fatalf("runAsk: %v", err)
	}
	if !cache.called {
		t.Fatal("without --mock the model cache must run")
	}
	// The one-time 941MB download must run under the long budget (v0.1.0 gave
	// it the 30s inference budget and killed every slow cold start).
	if d := time.Until(cache.deadline); d < 25*time.Minute {
		t.Fatalf("download budget must be >= 25 minutes, got %v", d)
	}
	// Inference must keep the tight interactive budget.
	if d := time.Until(runner.deadline); d > 35*time.Second || d < 20*time.Second {
		t.Fatalf("inference budget must stay tight (~30s), got %v", d)
	}
}

func TestAskSlowDownloadSurvivesTightInferenceBudget(t *testing.T) {
	// A download slower than the inference budget must still complete. If the
	// two phases shared one context again, the 50ms cap below would kill the
	// 800ms fetch — exactly the shipped v0.1.0 failure mode (a 30s cap
	// killing a 30-90s download).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(800 * time.Millisecond)
		_, _ = io.WriteString(w, "gguf")
	}))
	defer srv.Close()
	dir := t.TempDir()
	t.Setenv("SH1LLWRT_CACHE_DIR", dir)
	t.Setenv("SH1LLWRT_MODEL_URL", srv.URL+"/model.gguf")
	t.Setenv("SH1LLWRT_MOCK", "")

	origInference := inferenceTimeout
	inferenceTimeout = 50 * time.Millisecond
	t.Cleanup(func() { inferenceTimeout = origInference })

	swapRunner(t, &fakeRunner{reply: "<cmd>ls -la</cmd><safe>true</safe><explain>list files</explain>"})
	resetAskFlags()

	if err := runAsk(newTestCmd(), []string{"list files"}); err != nil {
		t.Fatalf("a slow download must not be killed by the inference budget: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, model.ModelFilename)); err != nil {
		t.Fatalf("downloaded model must be cached: %v", err)
	}
}

func TestAskNoModelRequiresMock(t *testing.T) {
	t.Setenv("SH1LLWRT_MOCK", "")
	cache := &recordingCache{}
	swapCache(t, cache)
	resetAskFlags()
	askFlags.noModel = true

	err := runAsk(newTestCmd(), []string{"list files"})
	if err == nil || !strings.Contains(err.Error(), "--no-model requires --mock") {
		t.Fatalf("expected the clean --no-model error, got %v", err)
	}
	if cache.called {
		t.Fatal("--no-model without --mock must not touch the model cache (v0.1.0 silently downloaded anyway)")
	}
}

func TestAskMockSkipsCache(t *testing.T) {
	cache := &recordingCache{}
	swapCache(t, cache)
	swapRunner(t, &fakeRunner{reply: "<cmd>ls -la</cmd><safe>true</safe><explain>list files</explain>"})
	resetAskFlags()
	askFlags.mock = true

	if err := runAsk(newTestCmd(), []string{"list files"}); err != nil {
		t.Fatalf("runAsk: %v", err)
	}
	if cache.called {
		t.Fatal("--mock must skip the model cache entirely")
	}
}

// --- m4/m7: end-to-end reply surfaces through the cobra command -------------

func TestAskJSONEndToEnd(t *testing.T) {
	swapRunner(t, &fakeRunner{reply: `<cmd>echo "hi there"</cmd><safe>true</safe><explain>greet</explain>`})
	resetAskFlags()
	askFlags.mock = true
	askFlags.json = true
	flush := captureStdout(t)

	if err := runAsk(newTestCmd(), []string{"say hi"}); err != nil {
		t.Fatalf("runAsk: %v", err)
	}
	out := flush()
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Fatalf("--json output must be one line, got %q", out)
	}
	var v struct {
		Command string `json:"command"`
		Safe    bool   `json:"safe"`
		Explain string `json:"explain"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("--json output is not valid JSON: %v (%s)", err, out)
	}
	if v.Command != `echo "hi there"` {
		t.Fatalf("quotes must round-trip exactly (v0.1.0 double-escaped them), got %q", v.Command)
	}
	if !v.Safe || v.Explain != "greet" {
		t.Fatalf("unexpected reply: %+v", v)
	}
}

func TestAskInsertEndToEnd(t *testing.T) {
	swapRunner(t, &fakeRunner{reply: "<cmd>tar -xzf foo.tar.gz -C /tmp</cmd><safe>false</safe><explain>extract</explain>"})
	resetAskFlags()
	askFlags.mock = true
	askFlags.insert = true
	flush := captureStdout(t)
	withStdin(t, "y\n")

	if err := runAsk(newTestCmd(), []string{"extract foo.tar.gz to /tmp"}); err != nil {
		t.Fatalf("runAsk: %v", err)
	}
	if out := flush(); out != "tar -xzf foo.tar.gz -C /tmp\n" {
		t.Fatalf("--insert must print the command only, got %q", out)
	}
}

func TestAskInsertDeclinesClosedEndToEnd(t *testing.T) {
	swapRunner(t, &fakeRunner{reply: "<cmd>rm -rf /tmp/old</cmd><safe>false</safe><explain>clean up</explain>"})
	resetAskFlags()
	askFlags.mock = true
	askFlags.insert = true
	flush := captureStdout(t)
	withStdin(t, "") // EOF: nothing piped, must decline

	if err := runAsk(newTestCmd(), []string{"clean up tmp"}); err == nil {
		t.Fatal("a declined destructive insert must surface as an error")
	}
	if out := flush(); out != "" {
		t.Fatalf("a declined insert must print no command, got %q", out)
	}
}

// --- m6: init --dry-run previews the real block -----------------------------

func TestInitDryRunPreviewsRealBlock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ZDOTDIR", "")
	t.Setenv("SHELL", "/bin/zsh")
	initFlags.dryRun = true
	t.Cleanup(func() { initFlags.dryRun = false })

	flush := captureStdout(t)
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("init --dry-run: %v", err)
	}
	out := flush()
	if !strings.Contains(out, "# (dry-run) sh1llwrt would install this zsh block:") {
		t.Fatalf("dry-run header missing:\n%s", out)
	}
	if !strings.Contains(out, "alias '?'='noglob sh1llwrt_ask'") {
		t.Fatalf("dry-run must preview the real glob-safe zsh block, got:\n%s", out)
	}
	if strings.Contains(out, "TODO(m2_shell_meet)") || strings.Contains(out, "function ?()") {
		t.Fatalf("dry-run must not show the stale v0.1 preview, got:\n%s", out)
	}
}
