// Package prompt holds the shell-scoped prompt contract: the AskRequest/
// AskReply types, the few-shot template that turns a natural-language sentence
// into a parseable shell command, and the destructive-verb heuristic that gates
// Safe-confirm.
//
// The template is the primitive worth version-locking: the same GGUF that
// scores 0.620 on InterCode-ALFA only feels like magic inside $SHELL if the
// prompt is shell-scoped (POSIX verbs, the user's real cwd, their real
// filenames) and the reply is parseable into Command/Safe/Explain. Swapping
// the GGUF later is a one-line model bump; the contract here is what keeps the
// in-shell UX stable.
package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AskRequest pins the in-shell UX and makes the model swappable later.
type AskRequest struct {
	Shell   string   // "zsh" | "bash" — selects flag grammar + syntax
	Cwd     string   // resolves relative paths, file globs
	Glob    []string // files the user just named (ls / git status) → injected as context
	Intent  string   // the natural-language sentence
	History []string // last N prompt lines, for "do it again but to *.png"
}

// AskReply is the parseable shape the model must return.
type AskReply struct {
	Command string // the shell command, ready to insert/print
	Safe    bool   // destructive? (rm, sudo, >) → needs confirm
	Explain string // one-line why
}

var (
	cmdRe     = regexp.MustCompile(`(?s)<cmd>(.*?)</cmd>`)
	safeRe    = regexp.MustCompile(`(?si)<safe>(.*?)</safe>`)
	explainRe = regexp.MustCompile(`(?s)<explain>(.*?)</explain>`)
)

// BuildPrompt assembles the shell-scoped few-shot prompt for an AskRequest.
//
// The prompt is deliberately POSIX-scoped: every example is a real dev command
// (tar/ffmpeg/magick/sed/kubectl/rsync), the system instruction fixes the
// output contract, and the live request section injects the user's actual
// shell, cwd, glob and history so the model resolves real paths instead of
// inventing them. Few-shot is ordered hardest-first so the model sees the long
// tail before the easy wins.
func BuildPrompt(req AskRequest) string {
	if req.Shell == "" {
		req.Shell = "bash"
	}
	if req.Cwd == "" {
		if cwd, err := os.Getwd(); err == nil {
			req.Cwd = cwd
		}
	}
	// Normalise globs to paths relative to cwd so they read like real flags.
	globs := make([]string, 0, len(req.Glob))
	for _, g := range req.Glob {
		if g == "" {
			continue
		}
		if req.Cwd != "" && strings.HasPrefix(g, req.Cwd+"/") {
			g = strings.TrimPrefix(g, req.Cwd+"/")
		}
		globs = append(globs, g)
	}

	var b strings.Builder
	b.WriteString(systemInstruction(req.Shell))
	b.WriteString("\n\n")
	b.WriteString(fewShot())
	b.WriteString("\n\nNow answer the following request in the same format.\n")
	b.WriteString("Return exactly one command. No markdown fences. No prose outside the tags.\n\n")
	b.WriteString(requestBlock(req, globs))
	return b.String()
}

// ParseReply turns raw model output into an AskReply. It prefers the tagged
// format, then falls back to taking the first non-empty fenced/code line as
// the command. Safe is the OR of the model's <safe> tag and the local
// IsDestructive heuristic — defense in depth, because a 1.5B model under-reports
// destructive verbs often enough to matter.
func ParseReply(raw string) (AskReply, error) {
	if strings.TrimSpace(raw) == "" {
		return AskReply{}, fmt.Errorf("empty model reply")
	}
	// The prompt ends with a bare "Answer:" label; small models echo the
	// label at the start of the reply and it must not leak into the command.
	raw = stripAnswerLabel(raw)

	var reply AskReply

	if m := cmdRe.FindStringSubmatch(raw); len(m) == 2 {
		reply.Command = strings.TrimSpace(stripFences(m[1]))
	}
	if m := safeRe.FindStringSubmatch(raw); len(m) == 2 {
		reply.Safe = strings.EqualFold(strings.TrimSpace(m[1]), "true")
	}
	if m := explainRe.FindStringSubmatch(raw); len(m) == 2 {
		reply.Explain = strings.TrimSpace(m[1])
	}

	// Fallback 1: an opened <cmd> whose closing tag never arrived. The 96-token
	// generation cap cuts replies mid-line, and the command before the cut is
	// still the answer — dropping it made the ?-alias silently insert nothing.
	// Fallback 2: no <cmd> tag at all → take the first non-empty, non-tag line
	// that looks like a command (starts with a word char and contains a space or
	// path separator). This recovers the many cases where a small model just
	// emits the bare command.
	if reply.Command == "" {
		reply.Command = unclosedCmdTag(raw)
	}
	if reply.Command == "" {
		reply.Command = firstCommandLine(raw)
	}
	if reply.Command == "" {
		return AskReply{}, fmt.Errorf("no command found in model reply")
	}

	// Defense in depth: local heuristic overrides toward unsafe.
	if IsDestructive(reply.Command) {
		reply.Safe = false
	}
	if reply.Explain == "" {
		reply.Explain = oneLineIntent(reply.Command)
	}
	return reply, nil
}

// IsDestructive returns true for commands that can delete, overwrite, mutate
// privileges, or otherwise deserve a y/N gate. This is the local half of the
// Safe decision; the model's <safe> tag is the other half and we OR them so
// either signal flips unsafe.
func IsDestructive(cmd string) bool {
	s := strings.TrimSpace(cmd)
	if s == "" {
		return false
	}
	// Shell redirects that truncate/overwrite. Structural, not a substring
	// test: ">>" (append), ">&" (descriptor dup), quoted text, and /dev/null
	// targets do not truncate, and one ">>" must not excuse a real "> file".
	if hasTruncatingRedirect(s) {
		return true
	}
	// Token-level destructive verbs. We split on shell metacharacters so
	// "sudo rm" and "rm" both match, and "rm" inside a filename does not.
	// The list mirrors the unsafe verbs the system instruction itself
	// teaches the model: a heuristic that skips verbs the prompt calls
	// unsafe cannot catch the model under-reporting them. Segments are
	// evaluated separately so git's subcommand can be found after the global
	// options that would otherwise separate it from the "git" token.
	for _, seg := range commandSegments(s) {
		tokens := tokenize(seg)
		for _, t := range tokens {
			switch t {
			case "rm", "rmdir", "dd", "mkfs", "mkfs.ext2", "mkfs.ext3", "mkfs.ext4",
				"mkfs.vfat", "mkfs.ntfs", "shred", "fdisk", "parted", "pvremove",
				"vgremove", "lvremove", "chmod", "chown", "chgrp", "shutdown", "reboot",
				"halt", "poweroff", "kill", "killall", "pkill", "systemctl", "service",
				"mv", "truncate":
				// chmod/chown are only destructive with -R or on system files; we
				// gate hard on the verb and let Safe-confirm do the rest. mv and
				// truncate overwrite silently, exactly like the redirect case.
				return true
			}
			// sudo / doas / runuser → anything escalated is unsafe by default.
			if t == "sudo" || t == "doas" || t == "runuser" || t == "su" {
				return true
			}
		}
		// git push/reset/clean destroy remote state, uncommitted work, or
		// untracked files — the system instruction already declares them
		// unsafe, so the local gate must agree.
		if gitDestroysState(tokens) {
			return true
		}
	}
	// Recursive chmod/chown → unsafe even if verb somehow missed above.
	lower := strings.ToLower(s)
	if strings.Contains(lower, "chmod -r") || strings.Contains(lower, "chown -r") {
		return true
	}
	return false
}

// --- internals ------------------------------------------------------------

func systemInstruction(shell string) string {
	return fmt.Sprintf(`You are sh1llwrt, a shell-command generator. The user types a single
natural-language sentence describing what they want to do in %s; you return
exactly ONE shell command that does it. You are offline, no account, sub-second.

Rules:
- Output exactly three tags and nothing else:
    <cmd>the single command line</cmd>
    <safe>true|false</safe>
    <explain>one short clause on why</explain>
- <safe> is true ONLY for read-only commands (ls, cat, grep, find, stat, du,
  df, file, head, tail, tar -t, kubectl get/describe, git status/log/diff).
  It is false for anything that writes, deletes, overwrites, escalates, or
  mutates: rm, mv, cp onto existing, > redirect, sudo, dd, mkfs, chmod, chown,
  systemctl, kill, git push/reset/clean, etc.
- Resolve real paths from the user's cwd and the named files. Prefer absolute
  paths when the user named a directory; keep relative paths when the user
  spoke relatively.
- One command. No pipes-to-rm. No here-docs. No multi-line scripts. If the
  intent needs two steps, pick the one the user most likely meant and note the
  follow-up in <explain>.
- No markdown fences. No backticks. No leading $ or #. Just the tags.`, shell)
}

func fewShot() string {
	return `Examples:

Request: extract the gzipped tarball foo.tar.gz into /tmp
<cmd>tar -xzf foo.tar.gz -C /tmp</cmd>
<safe>false</safe>
<explain>extract gzipped tarball into /tmp</explain>

Request: resize cat.png to 200x200 keeping aspect, write cat_small.png
<cmd>magick cat.png -resize 200x200 cat_small.png</cmd>
<safe>false</safe>
<explain>resize image to 200x200 box</explain>

Request: convert clip.mov to an mp4 h264 with aac audio
<cmd>ffmpeg -i clip.mov -c:v libx264 -c:a aac clip.mp4</cmd>
<safe>false</safe>
<explain>transcode mov to h264/aac mp4</explain>

Request: replace every "foo" with "bar" in notes.md in place
<cmd>sed -i 's/foo/bar/g' notes.md</cmd>
<safe>false</safe>
<explain>in-place global replace</explain>

Request: list the running pods in the kube-system namespace
<cmd>kubectl get pods -n kube-system</cmd>
<safe>true</safe>
<explain>read-only pod listing</explain>

Request: mirror src/ to dest/ over ssh, delete files in dest not in src
<cmd>rsync -a --delete src/ dest/</cmd>
<safe>false</safe>
<explain>--delete removes files at the destination</explain>`
}

func requestBlock(req AskRequest, globs []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Request: %s\n", strings.TrimSpace(req.Intent))
	fmt.Fprintf(&b, "Shell: %s\n", req.Shell)
	if req.Cwd != "" {
		fmt.Fprintf(&b, "Cwd: %s\n", req.Cwd)
	}
	if len(globs) > 0 {
		fmt.Fprintf(&b, "Named files: %s\n", strings.Join(globs, ", "))
	}
	if len(req.History) > 0 {
		fmt.Fprintf(&b, "Recent: %s\n", strings.Join(req.History, " | "))
	}
	b.WriteString("Answer:")
	return b.String()
}

// unclosedCmdTag recovers the command from a <cmd> tag whose closing tag never
// arrived. Only the tag's own line is used, because a truncated reply stops
// mid-line; a partially emitted trailing tag ("<sa", "</cm") is stripped.
func unclosedCmdTag(raw string) string {
	i := strings.LastIndex(raw, "<cmd>")
	if i < 0 {
		return ""
	}
	rest := raw[i+len("<cmd>"):]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	rest = strings.TrimSpace(stripFences(rest))
	if j := strings.LastIndex(rest, "<"); j >= 0 && !strings.Contains(rest[j:], ">") {
		rest = strings.TrimSpace(rest[:j])
	}
	if rest == "" || !firstTokenIsCommandWord(rest) {
		return ""
	}
	return rest
}

// firstCommandLine recovers a bare command when no <cmd> tag can be found.
func firstCommandLine(raw string) string {
	// strip common markdown fences first.
	raw = stripFences(raw)
	for _, line := range strings.Split(raw, "\n") {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}
		// Strip the prompt's "Answer:" label from the line instead of
		// skipping it: the label carries no information, the rest of the
		// line may be the command.
		l = stripAnswerLabel(l)
		if strings.HasPrefix(l, "<") || strings.HasPrefix(l, "#") {
			continue
		}
		// Strip a leading prompt char if present.
		l = strings.TrimLeft(l, "$> ")
		if isCommandLike(l) {
			return l
		}
	}
	// Last resort: the whole blob — but only a single line that reads as one
	// command word. Prose or a multi-line blob is not a command; returning ""
	// surfaces the honest "no command found" error instead of inserting junk.
	blob := strings.TrimSpace(stripFences(raw))
	if !strings.Contains(blob, "\n") && firstTokenIsCommandWord(blob) {
		return blob
	}
	return ""
}

// stripAnswerLabel removes one leading, case-insensitive "Answer:" prompt
// label plus surrounding whitespace. requestBlock ends the prompt with a bare
// "Answer:" and small models routinely echo the label at the start of the
// reply; leaving it in would leak it into the parsed command.
func stripAnswerLabel(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= len("Answer:") && strings.EqualFold(s[:len("Answer:")], "Answer:") {
		s = strings.TrimSpace(s[len("Answer:"):])
	}
	return s
}

// firstTokenIsCommandWord reports whether the first whitespace-delimited
// token of s is a plausible command word: it starts with a letter (or a path
// char) and carries no sentence punctuation — "Sure!" and "Answer:" are
// prose, "tar", "git", "/usr/bin/tool" are not.
func firstTokenIsCommandWord(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	token := s
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		token = s[:i]
	}
	token = strings.TrimLeft(token, "$> ")
	if token == "" {
		return false
	}
	r := token[0]
	if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '/' || r == '.' || r == '_') {
		return false
	}
	return !strings.ContainsAny(token, "!?,;:'\"()<>|&*`\\")
}

func isCommandLike(s string) bool {
	if !firstTokenIsCommandWord(s) {
		return false
	}
	// The scan path wants a full command line (the one-word case is handled
	// by the gated last resort), so require a space or path separator.
	return strings.ContainsAny(s, " \t/") || strings.HasSuffix(s, ".sh")
}

func stripFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```sh")
	s = strings.TrimPrefix(s, "```bash")
	s = strings.TrimPrefix(s, "```")
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

func tokenize(cmd string) []string {
	// Split on shell metacharacters so "sudo rm -rf /" → ["sudo","rm","-rf","/"].
	re := regexp.MustCompile(`[\s|;&<>()]+`)
	out := make([]string, 0, 8)
	for _, t := range re.Split(cmd, -1) {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// commandSegments splits a command line on shell control operators (;, |, &,
// newline) while leaving quoted spans intact, so each segment can be judged as
// one command.
func commandSegments(s string) []string {
	var segs []string
	var b strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			b.WriteByte(c)
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
			b.WriteByte(c)
		case ';', '|', '&', '\n':
			segs = append(segs, b.String())
			b.Reset()
		default:
			b.WriteByte(c)
		}
	}
	return append(segs, b.String())
}

// gitDestroysState reports whether a segment runs git push/reset/clean. git
// accepts global options (-C, -c, --git-dir, --no-pager, ...) before the
// subcommand, so requiring the verb to sit next to the "git" token let
// "git -C repo clean -fd" through ungated.
func gitDestroysState(tokens []string) bool {
	if len(tokens) == 0 || tokens[0] != "git" {
		return false
	}
	for _, t := range tokens[1:] {
		switch t {
		case "push", "reset", "clean":
			return true
		}
	}
	return false
}

// hasTruncatingRedirect reports whether the command contains a single ">" that
// truncates a file. Quoted text, append (">>"), descriptor dup (">&"), escaped
// characters, and /dev/null targets (the conventional way to silence stderr)
// do not truncate anything; every other ">" does, regardless of whether some
// unrelated ">>" appears later in the command.
func hasTruncatingRedirect(s string) bool {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' && quote == '"' && i+1 < len(s) {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\\':
			i++
		case '\'', '"':
			quote = c
		case '>':
			if i+1 < len(s) && (s[i+1] == '>' || s[i+1] == '&') {
				i++
				continue
			}
			if redirectTarget(s[i+1:]) == "/dev/null" {
				continue
			}
			return true
		}
	}
	return false
}

// redirectTarget returns the filename a ">" redirect writes to: the next token,
// stopping at whitespace or a shell metacharacter.
func redirectTarget(rest string) string {
	rest = strings.TrimLeft(rest, " \t")
	if end := strings.IndexAny(rest, " \t\n|;&<>()"); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

func oneLineIntent(cmd string) string {
	tokens := tokenize(cmd)
	if len(tokens) == 0 {
		return ""
	}
	verb := filepath.Base(tokens[0])
	return verb + " command"
}
