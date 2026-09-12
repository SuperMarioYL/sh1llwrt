package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoFile resolves a path relative to the repository root (tests run with
// cwd = cmd/sh1llwrt).
func repoFile(parts ...string) string {
	return filepath.Join(append([]string{"..", ".."}, parts...)...)
}

func readVersionFile(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(repoFile("VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	return strings.TrimSpace(string(b))
}

func TestVersionLockstep(t *testing.T) {
	ver := readVersionFile(t)
	if ver != "0.2.0" {
		t.Fatalf("VERSION file = %q, want 0.2.0", ver)
	}
	if want := ver + "-dev"; version != want {
		t.Fatalf("main.version dev constant = %q, want %q (the Go-side surfaces must stay in lockstep with the VERSION file)", version, want)
	}
}

func TestRootCmdVersionFlag(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"--version"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("--version: %v", err)
	}
	if !strings.Contains(buf.String(), version) {
		t.Fatalf("--version output must contain %q, got %q", version, buf.String())
	}
}

func TestChangelogHasBothVersions(t *testing.T) {
	b, err := os.ReadFile(repoFile("CHANGELOG.md"))
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}
	s := string(b)
	for _, want := range []string{"## [0.1.0]", "## [0.2.0]"} {
		if !strings.Contains(s, want) {
			t.Fatalf("CHANGELOG.md must contain a %q section", want)
		}
	}
}

func TestSiteJSONContentVersionWhenPresent(t *testing.T) {
	b, err := os.ReadFile(repoFile("web", "site.json"))
	if os.IsNotExist(err) {
		t.Skip("web/site.json not present in this checkout (the site lives on main)")
	}
	if err != nil {
		t.Fatal(err)
	}
	var site struct {
		Meta struct {
			ContentVersion string `json:"content_version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(b, &site); err != nil {
		t.Fatalf("parse web/site.json: %v", err)
	}
	want := readVersionFile(t)
	if site.Meta.ContentVersion != want {
		t.Fatalf("web/site.json meta.content_version = %q, want %q (site content must stay in lockstep)", site.Meta.ContentVersion, want)
	}
}
