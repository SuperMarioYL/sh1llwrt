// Package llama runs the cached GGUF through llama.cpp and returns the model's
// raw text reply.
//
// Two runners ship in m1:
//
//   - CLIRunner: the production m1 path. It exec's the llama.cpp `llama-cli`
//     binary (which IS llama.cpp), loading the cached GGUF with CPU threads +
//     Metal. This makes the repo runnable out of the box on any machine that
//     has llama.cpp installed, fully offline after the one-time GGUF download.
//
//   - MockRunner: a deterministic stand-in used by the test suite and the
//     rendered demo gif (SH1LLWRT_MOCK=1), so the 941MB model is not required
//     to exercise the prompt→parse→insert flow.
//
// The CGO in-process path (a single static binary with llama.cpp vendored
// in-process, CPU threads + Metal) is the m3 deliverable and is intentionally
// absent from this m1 build; when m3 lands it will live in this package behind
// the `llama` build tag, sharing the Runner interface so callers do not
// change. The default build is pure Go with no C++ dependency.
package llama

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Options configures a runner.
type Options struct {
	// ModelPath is the absolute path to the cached GGUF.
	ModelPath string
	// Threads caps CPU threads (0 → runtime.NumCPU).
	Threads int
	// MaxTokens caps the reply length.
	MaxTokens int
	// Temperature; 0 = greedy/deterministic.
	Temperature float64
	// Verbose prints the underlying command to stderr.
	Verbose bool
}

func (o Options) withDefaults() Options {
	if o.Threads <= 0 {
		o.Threads = runtime.NumCPU()
	}
	if o.MaxTokens <= 0 {
		o.MaxTokens = 96
	}
	if o.Temperature < 0 {
		o.Temperature = 0
	}
	return o
}

// Runner completes a prompt and returns the raw model text.
type Runner interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// CLIRunner runs llama.cpp via the `llama-cli` binary.
//
// It resolves the binary in this order: the SH1LLWRT_LLAMA_CLI env var, the
// PATH (names: llama-cli, llama-cli.exe, main), and the cache dir. When found,
// it exec's `llama-cli -m <model> -t <threads> -n <max> --temp <t>
// --no-display-prompt -p <prompt>` and returns stdout. The flag set matches
// recent llama.cpp builds (b4502+); older builds named the binary `main` and
// used the same flags.
type CLIRunner struct {
	Options Options
	// Binary, if set, overrides the lookup. Mainly for tests (point at a
	// fake binary that echoes a canned reply).
	Binary string
}

// Complete exec's llama-cli and returns its stdout.
func (r *CLIRunner) Complete(ctx context.Context, prompt string) (string, error) {
	opts := r.Options.withDefaults()
	bin, err := r.resolveBinary()
	if err != nil {
		return "", err
	}
	args := []string{
		"-m", opts.ModelPath,
		"-t", itoa(opts.Threads),
		"-n", itoa(opts.MaxTokens),
		"--temp", ftoa(opts.Temperature),
		"--no-display-prompt",
		"-p", prompt,
	}
	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "sh1llwrt: exec %s %s\n", bin, redact(args))
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	// llama-cli writes the prompt echo to stdout before the completion unless
	// --no-display-prompt is honoured; we strip a leading copy of the prompt
	// defensively in parseReply anyway.
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("llama-cli: %w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("llama-cli: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *CLIRunner) resolveBinary() (string, error) {
	if r.Binary != "" {
		return r.Binary, nil
	}
	if v := os.Getenv("SH1LLWRT_LLAMA_CLI"); v != "" {
		return v, nil
	}
	names := []string{"llama-cli", "main"}
	if runtime.GOOS == "windows" {
		names = []string{"llama-cli.exe", "main.exe"}
	}
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	// Cache dir fallback (the installer may drop a prebuilt llama-cli there).
	if cache := os.Getenv("SH1LLWRT_CACHE_DIR"); cache != "" {
		p := filepath.Join(cache, "llama-cli")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("llama-cli not found in PATH — install llama.cpp or set SH1LLWRT_LLAMA_CLI (m3 will vendor it in-process)")
}

// MockRunner is a deterministic stand-in for tests and the demo gif. It
// recognises a handful of canonical intents and returns the exact tagged reply
// a well-behaved model would. It is NOT a substitute for the model; it exists
// so the prompt→parse→insert path is exercisable without the 941MB download.
type MockRunner struct{}

func (r *MockRunner) Complete(ctx context.Context, prompt string) (string, error) {
	return mockReply(firstIntent(prompt)), nil
}

func mockReply(intent string) string {
	l := strings.ToLower(intent)
	switch {
	case strings.Contains(l, "tar") && (strings.Contains(l, "extract") || strings.Contains(l, "untar") || strings.Contains(l, "unzip")):
		return "<cmd>tar -xzf foo.tar.gz -C /tmp</cmd><safe>false</safe><explain>extract gzipped tarball into /tmp</explain>"
	case strings.Contains(l, "resize") || strings.Contains(l, "magick") || (strings.Contains(l, "convert") && strings.Contains(l, "png")):
		return "<cmd>magick cat.png -resize 200x200 cat_small.png</cmd><safe>false</safe><explain>resize image to 200x200 box</explain>"
	case strings.Contains(l, "ffmpeg") || (strings.Contains(l, "mov") && strings.Contains(l, "mp4")):
		return "<cmd>ffmpeg -i clip.mov -c:v libx264 -c:a aac clip.mp4</cmd><safe>false</safe><explain>transcode mov to h264/aac mp4</explain>"
	case strings.Contains(l, "sed") || (strings.Contains(l, "replace") && strings.Contains(l, "foo")):
		return "<cmd>sed -i 's/foo/bar/g' notes.md</cmd><safe>false</safe><explain>in-place global replace</explain>"
	case strings.Contains(l, "pod") || strings.Contains(l, "kubectl"):
		return "<cmd>kubectl get pods -n kube-system</cmd><safe>true</safe><explain>read-only pod listing</explain>"
	case strings.Contains(l, "rsync") || strings.Contains(l, "mirror"):
		return "<cmd>rsync -a --delete src/ dest/</cmd><safe>false</safe><explain>--delete removes files at the destination</explain>"
	default:
		// Generic fallback so any sentence still produces a parseable reply.
		return "<cmd>echo sh1llwrt-mock: " + strings.TrimSpace(intent) + "</cmd><safe>true</safe><explain>mock echo of the request</explain>"
	}
}

func firstIntent(prompt string) string {
	// The few-shot examples each begin with "Request:", so the user's actual
	// request is the LAST "Request:" line (it is emitted by requestBlock after
	// all the examples). Iterate to the last match, not the first.
	intent := ""
	for _, line := range strings.Split(prompt, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "Request:") {
			intent = strings.TrimPrefix(l, "Request:")
		}
	}
	if intent != "" {
		return intent
	}
	return prompt
}

// ResolveRunner picks Mock when mock is true or SH1LLWRT_MOCK is set, else CLI.
func ResolveRunner(opts Options, mock bool) Runner {
	if mock || os.Getenv("SH1LLWRT_MOCK") != "" {
		return &MockRunner{}
	}
	return &CLIRunner{Options: opts}
}

// --- small helpers (avoid strconv to keep the import list tight) ----------

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func ftoa(f float64) string {
	// llama-cli accepts e.g. "0" or "0.2"; keep it simple for our defaults.
	if f == 0 {
		return "0"
	}
	s := fmt.Sprintf("%.2f", f)
	return strings.TrimSuffix(strings.TrimSuffix(s, "0"), ".")
}

func redact(args []string) string {
	// The prompt can be long; show a stub for the verbose log.
	out := make([]string, 0, len(args))
	for _, a := range args {
		if len(a) > 40 {
			out = append(out, fmt.Sprintf("%q...", a[:37]))
		} else {
			out = append(out, fmt.Sprintf("%q", a))
		}
	}
	return strings.Join(out, " ")
}
