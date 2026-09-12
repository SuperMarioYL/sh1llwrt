package shell

import (
	"fmt"
	"os"
	"path/filepath"
)

// Detect returns the user's shell ("zsh" or "bash") and the rc file to write.
// $SHELL wins; otherwise we fall back to bash + ~/.bashrc so `sh1llwrt init`
// is always useful. Unknown shells also fall back to bash.
func Detect() (shell, rcPath string, err error) {
	s := ""
	if v := os.Getenv("SHELL"); v != "" {
		s = baseName(v)
	}
	if s != "zsh" && s != "bash" {
		s = "bash"
	}
	rc, err := rcPathFor(s)
	return s, rc, err
}

// ResolveShell returns the shell `init` should wire: the override when given
// (only "zsh" or "bash" — anything else is rejected, since writing
// shell-specific syntax into an unrelated rc is silent breakage), else the
// detected shell.
func ResolveShell(shellOverride string) (string, error) {
	s, _, err := Detect()
	if err != nil {
		return "", err
	}
	if shellOverride != "" {
		if shellOverride != "zsh" && shellOverride != "bash" {
			return "", fmt.Errorf("unsupported shell %q (supported: zsh, bash)", shellOverride)
		}
		s = shellOverride
	}
	return s, nil
}

// rcPathFor resolves the rc file for a shell. zsh honours $ZDOTDIR (the rule
// Detect has always applied for detection); bash uses ~/.bashrc.
func rcPathFor(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if shell == "zsh" {
		if zd := os.Getenv("ZDOTDIR"); zd != "" {
			return filepath.Join(zd, ".zshrc"), nil
		}
		return filepath.Join(home, ".zshrc"), nil
	}
	return filepath.Join(home, ".bashrc"), nil
}

// FunctionBlockZsh is the block wired into ~/.zshrc (or $ZDOTDIR/.zshrc).
//
// The `?` trigger is a QUOTED alias, not a bare function definition: an
// unquoted `?` word is pathname-expanded before command lookup, so the v0.1
// `function ?()` form aborted .zshrc sourcing with "no matches found: ?" on
// any system whose rc directory had no 1-char files. Alias resolution
// happens before pathname expansion, and noglob keeps glob characters in the
// typed sentence literal so the model sees the names the user typed.
const FunctionBlockZsh = `# sh1llwrt: the offline in-shell command generator. See https://github.com/SuperMarioYL/sh1llwrt
sh1llwrt_ask() {
  local out
  if out=$(sh1llwrt ask --insert "$*"); then
    print -z -- "$out"
  fi
}
# ?-prefix: type a sentence, the command lands at your cursor (Enter is yours).
alias '?'='noglob sh1llwrt_ask'
# Ctrl-Space: rewrite the current line through the model.
_sh1llwrt_widget() {
  local out
  if out=$(sh1llwrt ask --insert "$BUFFER"); then
    LBUFFER="$out"
    RBUFFER=""
  fi
}
zle -N _sh1llwrt_widget
bindkey '^@' _sh1llwrt_widget

# end of sh1llwrt block`

// FunctionBlockBash is the block wired into ~/.bashrc. A plain bash function
// cannot edit the readline edit buffer (READLINE_LINE is only writable inside
// a bind -x widget), so `?` prints the command for copy and the Ctrl-Space
// widget does the insertion. bind -x itself needs an active readline, hence
// the interactive guard.
const FunctionBlockBash = `# sh1llwrt: the offline in-shell command generator. See https://github.com/SuperMarioYL/sh1llwrt
sh1llwrt_ask() {
  sh1llwrt ask "$@"
}
# ?-prefix: type a sentence, get the command printed for copy. The quoted
# alias name matters: a bare ? would glob-expand to any 1-char filename.
alias '?'='sh1llwrt_ask'
# Ctrl-Space: rewrite the current line through the model.
_sh1llwrt_widget() {
  local out
  if out=$(sh1llwrt ask --insert "$READLINE_LINE"); then
    READLINE_LINE="$out"
  fi
}
if [[ $- == *i* ]]; then
  bind -x '"\C-@": _sh1llwrt_widget'
fi

# end of sh1llwrt block`

// FunctionBlock returns the installable block for a shell ("zsh" or "bash").
// `init --dry-run` uses it to preview exactly what Install writes.
func FunctionBlock(shell string) string {
	if shell == "zsh" {
		return FunctionBlockZsh
	}
	return FunctionBlockBash
}

// marker lines bracket the block so Install is idempotent: re-running
// `sh1llwrt init` replaces the block instead of appending a second copy.
const (
	markerStart = "# >>> sh1llwrt block >>>"
	markerEnd   = "# <<< sh1llwrt block <<<"
)

// blockFor returns the bracketed block to splice into an rc file.
func blockFor(shell string) string {
	return markerStart + "\n" + FunctionBlock(shell) + "\n" + markerEnd + "\n"
}

// Install writes the sh1llwrt block for the resolved shell into its rc file,
// creating it if needed. shellOverride, when non-empty, must be "zsh" or
// "bash" (see ResolveShell). It is idempotent: an existing block is replaced
// in place rather than duplicated. Returns the rc path written.
func Install(shellOverride string) (string, error) {
	shellName, err := ResolveShell(shellOverride)
	if err != nil {
		return "", err
	}
	rcPath, err := rcPathFor(shellName)
	if err != nil {
		return "", err
	}

	existing := []byte{}
	if b, err := os.ReadFile(rcPath); err == nil {
		existing = b
	}

	updated := spliceBlock(string(existing), blockFor(shellName))
	if err := os.WriteFile(rcPath, []byte(updated), 0o644); err != nil {
		return "", err
	}
	return rcPath, nil
}

// spliceBlock replaces an existing marker-delimited block, or appends one.
func spliceBlock(content, block string) string {
	start := markerStart
	end := markerEnd
	si := indexOf(content, start)
	ei := indexOf(content, end)
	if si >= 0 && ei > si {
		// Replace in place.
		before := content[:si]
		after := content[ei+len(end):]
		// Ensure newlines around the block.
		trimmed := trimRightSpace(before)
		return ensureNewline(trimmed) + block + ensureLeadingNewline(after)
	}
	// Append.
	trimmed := trimRightSpace(content)
	if trimmed != "" {
		trimmed = ensureNewline(trimmed)
	}
	return trimmed + block
}

func indexOf(s, sub string) int {
	n := len(sub)
	for i := 0; i+n <= len(s); i++ {
		if s[i:i+n] == sub {
			return i
		}
	}
	return -1
}

func trimRightSpace(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func ensureNewline(s string) string {
	if s == "" || s[len(s)-1] == '\n' {
		return s
	}
	return s + "\n"
}

func ensureLeadingNewline(s string) string {
	if s == "" || s[0] == '\n' {
		return s
	}
	return "\n" + s
}

// baseName returns the part of a path after the last slash; used to derive the
// shell name from $SHELL.
func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}
