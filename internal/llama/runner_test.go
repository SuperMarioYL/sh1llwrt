package llama

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMockRunnerCanonicalIntents(t *testing.T) {
	cases := map[string]string{
		"Request: extract the gzipped tarball foo.tar.gz into /tmp\nShell: bash\n": "tar -xzf foo.tar.gz -C /tmp",
		"Request: resize cat.png to 200x200\nShell: zsh\n":                          "magick cat.png -resize 200x200 cat_small.png",
		"Request: list the running pods in kube-system\n":                           "kubectl get pods -n kube-system",
		"Request: mirror src/ to dest/ over ssh\n":                                   "rsync -a --delete src/ dest/",
	}
	r := &MockRunner{}
	for prompt, wantCmd := range cases {
		out, err := r.Complete(context.Background(), prompt)
		if err != nil {
			t.Fatalf("Complete: %v", err)
		}
		if !strings.Contains(out, "<cmd>") || !strings.Contains(out, wantCmd) {
			t.Errorf("for prompt %q got %q, want <cmd> containing %q", prompt, out, wantCmd)
		}
	}
}

func TestMockRunnerFallbackIsParseable(t *testing.T) {
	r := &MockRunner{}
	out, err := r.Complete(context.Background(), "Request: do something unusual\n")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !strings.Contains(out, "<cmd>") || !strings.Contains(out, "</cmd>") {
		t.Errorf("fallback reply not tagged: %q", out)
	}
}

func TestCLIRunnerBuildsExpectedArgs(t *testing.T) {
	// Fake llama-cli: writes a canned reply so we can assert the runner picks
	// it up and trims whitespace. The script also echoes the args it received
	// into a sibling .args file so we can verify the flag set.
	fake := writeFakeCLI(t, `#!/usr/bin/env bash
echo "$@" > "$0.args"
printf '<cmd>tar -xzf foo.tar.gz -C /tmp</cmd><safe>false</safe><explain>x</explain>'
`)
	r := &CLIRunner{
		Options: Options{
			ModelPath:  "/models/q.gguf",
			Threads:    4,
			MaxTokens:  32,
			Temperature: 0,
		},
		Binary: fake,
	}
	out, err := r.Complete(context.Background(), "Request: extract\n")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !strings.Contains(out, "tar -xzf foo.tar.gz") {
		t.Errorf("unexpected output: %q", out)
	}
	argsBytes, err := os.ReadFile(fake + ".args")
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(argsBytes)
	for _, want := range []string{"-m", "/models/q.gguf", "-t", "4", "-n", "32", "--temp", "0", "--no-display-prompt", "-p", "Request: extract"} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q in %q", want, args)
		}
	}
}

func TestCLIRunnerMissingBinaryErrors(t *testing.T) {
	r := &CLIRunner{Options: Options{ModelPath: "/x.gguf"}, Binary: "/nonexistent/llama-cli-xyz"}
	if _, err := r.Complete(context.Background(), "hi"); err == nil {
		t.Errorf("expected error when binary missing")
	}
}

func TestResolveRunnerMockEnv(t *testing.T) {
	t.Setenv("SH1LLWRT_MOCK", "1")
	got := ResolveRunner(Options{ModelPath: "/x"}, false)
	if _, ok := got.(*MockRunner); !ok {
		t.Errorf("expected MockRunner when SH1LLWRT_MOCK set, got %T", got)
	}
}

func TestResolveRunnerMockFlag(t *testing.T) {
	t.Setenv("SH1LLWRT_MOCK", "")
	got := ResolveRunner(Options{ModelPath: "/x"}, true)
	if _, ok := got.(*MockRunner); !ok {
		t.Errorf("expected MockRunner when mock=true, got %T", got)
	}
}

func TestResolveRunnerCLI(t *testing.T) {
	t.Setenv("SH1LLWRT_MOCK", "")
	got := ResolveRunner(Options{ModelPath: "/x"}, false)
	if _, ok := got.(*CLIRunner); !ok {
		t.Errorf("expected CLIRunner, got %T", got)
	}
}

func TestItoa(t *testing.T) {
	cases := map[int]string{0: "0", 1: "1", 4: "4", 32: "32", -5: "-5"}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

func writeFakeCLI(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	name := "fake-llama-cli"
	if runtime.GOOS == "windows" {
		name += ".bat"
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake cli: %v", err)
	}
	return p
}
