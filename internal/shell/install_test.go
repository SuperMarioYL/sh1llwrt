package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFunctionBlocksAreGlobSafe(t *testing.T) {
	zsh := FunctionBlock("zsh")
	if !strings.Contains(zsh, "alias '?'='noglob sh1llwrt_ask'") {
		t.Fatalf("zsh block must wire ? through the quoted noglob alias:\n%s", zsh)
	}
	if strings.Contains(zsh, "function ?(") {
		t.Fatalf("zsh block must not define a bare ? function (glob-expanded at source time under default zsh options):\n%s", zsh)
	}
	bash := FunctionBlock("bash")
	if !strings.Contains(bash, "alias '?'='sh1llwrt_ask'") {
		t.Fatalf("bash block must wire ? through the quoted alias:\n%s", bash)
	}
}

func TestZshBlockSourcesCleanly(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not available on this machine")
	}
	dir := t.TempDir() // empty cwd: no 1-char files for a bare ? to glob
	script := FunctionBlock("zsh") + `
print -r -- SOURCE-OK
whence -w sh1llwrt_ask > /dev/null && print -r -- ASK-OK
alias '?' > /dev/null && print -r -- ALIAS-OK
whence -w _sh1llwrt_widget > /dev/null && print -r -- WIDGET-OK
`
	cmd := exec.Command(zsh, "-c", script)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zsh failed to source the block (the v0.1 block aborts with 'no matches found: ?'): %v\n%s", err, out)
	}
	for _, want := range []string{"SOURCE-OK", "ASK-OK", "ALIAS-OK", "WIDGET-OK"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("expected %s in zsh output:\n%s", want, out)
		}
	}
}

func TestBashBlockSourcesCleanly(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available on this machine")
	}
	dir := t.TempDir()
	script := FunctionBlock("bash") + `
echo SOURCE-OK
declare -F sh1llwrt_ask > /dev/null && echo ASK-OK
alias '?' > /dev/null && echo ALIAS-OK
declare -F _sh1llwrt_widget > /dev/null && echo WIDGET-OK
`
	cmd := exec.Command(bashPath, "-c", script)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash failed to source the block: %v\n%s", err, out)
	} else {
		for _, want := range []string{"SOURCE-OK", "ASK-OK", "ALIAS-OK", "WIDGET-OK"} {
			if !strings.Contains(string(out), want) {
				t.Fatalf("expected %s in bash output:\n%s", want, out)
			}
		}
	}
}

func TestInstallRejectsUnknownShell(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if _, err := Install("fish"); err == nil {
		t.Fatal("Install must reject shells other than zsh/bash (v0.1.0 silently wrote the block into the detected rc)")
	}
}

func TestInstallZshOverrideHonoursZDOTDIR(t *testing.T) {
	home := t.TempDir()
	zdot := filepath.Join(home, "zdot")
	if err := os.MkdirAll(zdot, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", zdot)
	t.Setenv("SHELL", "/bin/bash") // detection alone would say bash + ~/.bashrc
	path, err := Install("zsh")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if want := filepath.Join(zdot, ".zshrc"); path != want {
		t.Fatalf("--shell zsh must honour $ZDOTDIR (v0.1.0 wrote $HOME/.zshrc): got %s, want %s", path, want)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("block not written: %v", err)
	}
	if !strings.Contains(string(b), "noglob sh1llwrt_ask") {
		t.Fatalf("zsh block content missing the alias wiring:\n%s", b)
	}
}

func TestSpliceBlockReplacesInPlace(t *testing.T) {
	content := "export A=1\n\n" + blockFor("zsh") + "\nexport B=2\n"
	updated := spliceBlock(content, blockFor("bash"))
	if strings.Count(updated, markerStart) != 1 || strings.Count(updated, markerEnd) != 1 {
		t.Fatalf("re-running init must replace, not duplicate, the block:\n%s", updated)
	}
	if !strings.HasPrefix(updated, "export A=1\n") || !strings.Contains(updated, "export B=2") {
		t.Fatalf("content outside the markers must survive:\n%s", updated)
	}
	if !strings.Contains(updated, "alias '?'='sh1llwrt_ask'") {
		t.Fatalf("replacement block must be the new one:\n%s", updated)
	}
}
