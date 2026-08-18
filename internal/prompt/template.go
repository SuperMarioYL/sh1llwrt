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
	cmdRe    = regexp.MustCompile(`(?s)<cmd>(.*?)</cmd>`)
	safeRe   = regexp.MustCompile(`(?si)<safe>(.*?)</safe>`)
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

	// Fallback: no <cmd> tag → take the first non-empty, non-tag line that
	// looks like a command (starts with a word char and contains a space or
	// path separator). This recovers the many cases where a small model just
	// emits the bare command.
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
	// Shell redirects that truncate/overwrite.
	if strings.Contains(s, ">") && !strings.Contains(s, ">>") {
		return true
	}
	// Token-level destructive verbs. We split on shell metacharacters so
	// "sudo rm" and "rm" both match, and "rm" inside a filename does not.
	tokens := tokenize(s)
	for _, t := range tokens {
		switch t {
		case "rm", "rmdir", "dd", "mkfs", "mkfs.ext2", "mkfs.ext3", "mkfs.ext4",
			"mkfs.vfat", "mkfs.ntfs", "shred", "fdisk", "parted", "pvremove",
			"vgremove", "lvremove", "chmod", "chown", "chgrp", "shutdown", "reboot",
			"halt", "poweroff", "kill", "killall", "pkill", "systemctl", "service":
			// chmod/chown are only destructive with -R or on system files; we
			// gate hard on the verb and let Safe-confirm do the rest.
			return true
		}
		// sudo / doas / runuser → anything escalated is unsafe by default.
		if t == "sudo" || t == "doas" || t == "runuser" || t == "su" {
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

func firstCommandLine(raw string) string {
	// strip common markdown fences first.
	raw = stripFences(raw)
	for _, line := range strings.Split(raw, "\n") {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "<") || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "Answer") {
			continue
		}
		// Strip a leading prompt char if present.
		l = strings.TrimLeft(l, "$> ")
		if isCommandLike(l) {
			return l
		}
	}
	// Last resort: whole blob, trimmed.
	return strings.TrimSpace(stripFences(raw))
}

func isCommandLike(s string) bool {
	if s == "" {
		return false
	}
	// Must start with an ASCII letter (the verb) and contain a space or path
	// separator — filters out "yes", "tar" alone, or stray prose.
	r := s[0]
	if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
		return false
	}
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

func oneLineIntent(cmd string) string {
	tokens := tokenize(cmd)
	if len(tokens) == 0 {
		return ""
	}
	verb := filepath.Base(tokens[0])
	return verb + " command"
}
