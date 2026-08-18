package shell

import "os"

// Detect returns the user's shell ("zsh" or "bash") and the rc file to write.
// $SHELL wins; otherwise we look at $ZDOTDIR/.zshrc, ~/.zshrc, ~/.bashrc in
// order. Unknown shells fall back to bash + ~/.bashrc so `sh1llwrt init` is
// always useful.
func Detect() (shell, rcPath string, err error) {
	if s := os.Getenv("SHELL"); s != "" {
		shell = baseName(s)
	}
	if shell != "zsh" && shell != "bash" {
		shell = "bash"
	}
	switch shell {
	case "zsh":
		if zd := os.Getenv("ZDOTDIR"); zd != "" {
			return "zsh", zd + "/.zshrc", nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		return "zsh", home + "/.zshrc", nil
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		return "bash", home + "/.bashrc", nil
	}
}

// FunctionBlock is the shell function wired by `sh1llwrt init`.
//
// m1 ships the print-mode `?` helper: `? extract this tar.gz to /tmp` runs
// `sh1llwrt ask "$@"` and prints the command for copy. The Ctrl-Space
// inline-insert widget (m2_shell_meet) is scaffolded in the block as a
// commented TODO so the rc file documents the m2 surface without breaking m1.
const FunctionBlock = `# sh1llwrt: the offline in-shell command generator. See https://github.com/SuperMarioYL/sh1llwrt
sh1llwrt_ask() {
  sh1llwrt ask "$@" 2>/dev/null || echo "sh1llwrt: ask failed (is the model cached?)"
}
# ?-prefix: type a sentence, get the command printed for copy.
function ?() { sh1llwrt_ask "$@"; }
# TODO(m2_shell_meet): zsh Ctrl-Space widget that inserts the command at the
# cursor and gates destructive verbs on y/N. m1 prints only; the human hits
# Enter. Sketch:
#   _sh1llwrt_widget() { LBUFFER=$(sh1llwrt ask --insert "$BUFFER"); zle reset-prompt }
#   zle -N _sh1llwrt_widget; bindkey '^ ' _sh1llwrt_widget  # ^ = Ctrl-Space

# end of sh1llwrt block`

// marker lines bracket the block so Install is idempotent: re-running
// `sh1llwrt init` replaces the block instead of appending a second copy.
const (
	markerStart = "# >>> sh1llwrt block >>>"
	markerEnd   = "# <<< sh1llwrt block <<<"
)

// blockFor returns the bracketed block to splice into an rc file.
func blockFor() string {
	return markerStart + "\n" + FunctionBlock + "\n" + markerEnd + "\n"
}

// Install writes the sh1llwrt function block into the detected rc file,
// creating it if needed. It is idempotent: an existing block is replaced in
// place rather than duplicated. Returns the rc path written.
func Install(shellOverride string) (string, error) {
	shell, rcPath, err := Detect()
	if err != nil {
		return "", err
	}
	if shellOverride != "" {
		shell = shellOverride
		if shell == "zsh" {
			home, _ := os.UserHomeDir()
			if home != "" {
				rcPath = home + "/.zshrc"
			}
		} else if shell == "bash" {
			home, _ := os.UserHomeDir()
			if home != "" {
				rcPath = home + "/.bashrc"
			}
		}
	}

	existing := []byte{}
	if b, err := os.ReadFile(rcPath); err == nil {
		existing = b
	}

	updated := spliceBlock(string(existing), blockFor())
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
