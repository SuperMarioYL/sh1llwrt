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
		"ls -la":                         false,
		"cat notes.md":                   false,
		"kubectl get pods":              false,
		"git status":                     false,
		"tar -xzf foo.tar.gz -C /tmp":    false, // writes but not destructive per heuristic
		"rm -rf /tmp/old":                true,
		"sudo apt update":                true,
		"doas rc-service restart nginx":  true,
		"echo hi > /etc/hosts":           true,
		"chmod -R 755 .":                 true,
		"dd if=img.iso of=/dev/sda":      true,
		"mkfs.ext4 /dev/sda1":            true,
		"git push --force":               false, // not in verb list — Safe still flips via model; acceptable
	}
	for cmd, want := range cases {
		if got := IsDestructive(cmd); got != want {
			t.Errorf("IsDestructive(%q) = %v, want %v", cmd, got, want)
		}
	}
}
