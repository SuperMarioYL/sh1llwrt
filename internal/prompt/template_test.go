package prompt

import (
	"strings"
	"testing"
)

func TestBuildPromptIncludesContract(t *testing.T) {
	p := BuildPrompt(AskRequest{
		Shell:  "zsh",
		Cwd:    "/home/yu/proj",
		Glob:   []string{"/home/yu/proj/cat.png", "notes.md"},
		Intent: "resize cat.png to 200x200",
	})
	for _, want := range []string{
		"sh1llwrt", "shell-command generator",
		"<cmd>", "<safe>", "<explain>",
		"tar -xzf foo.tar.gz -C /tmp",
		"magick cat.png -resize 200x200",
		"rsync -a --delete",
		"Request: resize cat.png to 200x200",
		"Shell: zsh",
		"Cwd: /home/yu/proj",
		"cat.png", "notes.md",
		"Answer:",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q\n--- prompt ---\n%s", want, p)
		}
	}
}

func TestParseReplyTagged(t *testing.T) {
	raw := "some preamble\n<cmd>tar -xzf foo.tar.gz -C /tmp</cmd>\n<safe>false</safe>\n<explain>extract into tmp</explain>"
	r, err := ParseReply(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.Command != "tar -xzf foo.tar.gz -C /tmp" {
		t.Errorf("command = %q", r.Command)
	}
	if r.Safe {
		t.Errorf("safe should be false")
	}
	if r.Explain != "extract into tmp" {
		t.Errorf("explain = %q", r.Explain)
	}
}

func TestParseReplyModelFalsePositivesSafeOnDestructive(t *testing.T) {
	// Model wrongly says safe=true on a destructive command; the local
	// heuristic must override toward unsafe (defense in depth).
	raw := "<cmd>rm -rf /tmp/old</cmd><safe>true</safe><explain>cleanup</explain>"
	r, err := ParseReply(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.Safe {
		t.Errorf("destructive rm must be unsafe despite model tag")
	}
}

func TestParseReplyBareCommandFallback(t *testing.T) {
	// A small model often emits just the bare command.
	raw := "```\ntar -xzf foo.tar.gz -C /tmp\n```"
	r, err := ParseReply(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.Command != "tar -xzf foo.tar.gz -C /tmp" {
		t.Errorf("command = %q", r.Command)
	}
	if r.Safe {
		t.Errorf("tar -xzf writes, should be unsafe")
	}
}

func TestParseReplyFallbackStripsAnswerLabel(t *testing.T) {
	// The prompt ends with a bare "Answer:"; a model continuing on that line
	// must not carry the label into the command (v0.2.0 returned
	// "Answer: tar -xzf ..." as the command).
	r, err := ParseReply("Answer: tar -xzf foo.tar.gz -C /tmp")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.Command != "tar -xzf foo.tar.gz -C /tmp" {
		t.Errorf("command = %q, want the label stripped", r.Command)
	}
	// A label echoed before the tags must not break the tagged path either.
	r, err = ParseReply("Answer: <cmd>ls -la</cmd><safe>true</safe><explain>list</explain>")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.Command != "ls -la" || !r.Safe {
		t.Errorf("tagged reply behind a label parsed wrong: %+v", r)
	}
}

func TestParseReplyFallbackProseMustNotBeatCommand(t *testing.T) {
	// Prose on line 1 must not win over the real command on line 2
	// (v0.2.0 returned the prose line as the command).
	raw := "Sure! Here is what you want:\ntar -xzf foo.tar.gz -C /tmp\nHope that helps!"
	r, err := ParseReply(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.Command != "tar -xzf foo.tar.gz -C /tmp" {
		t.Errorf("command = %q, want the actual command line", r.Command)
	}
}

func TestParseReplyFallbackProseOnlyErrors(t *testing.T) {
	// Prose with no command anywhere must be an error, not a "command"
	// (v0.2.0 returned the prose blob as the command).
	if _, err := ParseReply("Sure! Here is what you want:"); err == nil {
		t.Errorf("expected an error for a prose-only reply")
	}
	if _, err := ParseReply("Sure! Here is what you want:\nWow, that was fast!"); err == nil {
		t.Errorf("expected an error for a multi-line blob with no command")
	}
}

func TestParseReplyReadOnlySafe(t *testing.T) {
	raw := "<cmd>kubectl get pods -n kube-system</cmd><safe>true</safe><explain>list pods</explain>"
	r, err := ParseReply(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !r.Safe {
		t.Errorf("read-only kubectl get should be safe")
	}
}

func TestParseReplyEmpty(t *testing.T) {
	if _, err := ParseReply("   \n  "); err == nil {
		t.Errorf("expected error on empty reply")
	}
}

func TestIsDestructive(t *testing.T) {
	cases := map[string]bool{
		"ls -la":                        false,
		"cat notes.md":                  false,
		"kubectl get pods":              false,
		"git status":                    false,
		"git log --oneline":             false,
		"git diff HEAD~1":               false,
		"git add -A && git commit -m x": false,
		"tar -xzf foo.tar.gz -C /tmp":   false, // writes but not destructive per heuristic
		"rm -rf /tmp/old":               true,
		"sudo apt update":               true,
		"doas rc-service restart nginx": true,
		"echo hi > /etc/hosts":          true,
		"chmod -R 755 .":                true,
		"dd if=img.iso of=/dev/sda":     true,
		"mkfs.ext4 /dev/sda1":           true,
		// mv/truncate overwrite silently and git push/reset/clean destroy
		// state — all declared unsafe by the prompt's own system instruction,
		// so the local gate must agree (v0.2.0 missed every one of these).
		"git push --force":         true,
		"git push":                 true,
		"git reset --hard HEAD~1":  true,
		"git clean -fd":            true,
		"mv new.config old.config": true,
		"truncate -s 0 huge.log":   true,
	}
	for cmd, want := range cases {
		if got := IsDestructive(cmd); got != want {
			t.Errorf("IsDestructive(%q) = %v, want %v", cmd, got, want)
		}
	}
}

func TestParseReplyHeuristicOverridesModelSafeTag(t *testing.T) {
	// The model mis-tags git reset --hard as safe; the local heuristic must
	// flip it so the insert path prompts (v0.2.0 let it through).
	r, err := ParseReply("<cmd>git reset --hard</cmd><safe>true</safe><explain>undo</explain>")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.Safe {
		t.Errorf("git reset --hard must be unsafe despite the model tag")
	}
}
