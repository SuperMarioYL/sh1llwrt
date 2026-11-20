package shell

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestInstallerWgetOnlyPath exercises the shipped curl|sh entry point on a host
// that has wget but no curl. v0.3.0 resolved the release URL through wget and
// then downloaded with curl anyway, exiting 127 with "curl: command not found".
func TestInstallerWgetOnlyPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh targets darwin/linux")
	}
	script, err := filepath.Abs("../../scripts/install.sh")
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	home := filepath.Join(root, "home")
	archive := filepath.Join(root, "release.tar.gz")
	for _, dir := range []string{bin, home} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Only the utilities install.sh needs — notably no curl.
	for _, name := range []string{"uname", "mkdir", "mktemp", "head", "sed", "grep", "tar", "mv", "chmod", "rm", "cat", "cp"} {
		p, err := exec.LookPath(name)
		if err != nil {
			t.Skipf("%s not available: %v", name, err)
		}
		if err := os.Symlink(p, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}

	writeFakeRelease(t, archive)
	// Fake wget: GitHub's API (pretty-printed, one URL per line) and the asset.
	wget := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"*api.github.com*)\n" +
		"  printf '%s\\n' '{\"assets\":[' \\\n" +
		"    '{\"browser_download_url\":\"https://example.invalid/sh1llwrt_0.4.0_darwin_amd64.tar.gz\"},' \\\n" +
		"    '{\"browser_download_url\":\"https://example.invalid/sh1llwrt_0.4.0_darwin_arm64.tar.gz\"},' \\\n" +
		"    '{\"browser_download_url\":\"https://example.invalid/sh1llwrt_0.4.0_linux_amd64.tar.gz\"}]}'\n" +
		"  exit 0 ;;\n" +
		"esac\n" +
		"out=\"\"\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"-O\" ]; then out=\"$2\"; shift 2; continue; fi\n" +
		"  shift\n" +
		"done\n" +
		"[ -n \"$out\" ] || { echo 'fake wget: no -O target' >&2; exit 2; }\n" +
		"cp " + archive + " \"$out\"\n"
	if err := os.WriteFile(filepath.Join(bin, "wget"), []byte(wget), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("/bin/sh", script)
	cmd.Env = []string{"PATH=" + bin, "HOME=" + home}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("installer must work with wget and no curl, got %v:\n%s", err, out)
	}
	if !strings.Contains(string(out), "installed") {
		t.Fatalf("installer did not report an install:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "sh1llwrt")); err != nil {
		t.Fatalf("binary not installed into ~/.local/bin: %v\n%s", err, out)
	}
}

// writeFakeRelease writes a goreleaser-shaped tarball whose root holds a
// runnable stand-in for the binary.
func writeFakeRelease(t *testing.T, path string) {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	body := []byte("#!/bin/sh\necho fake sh1llwrt\n")
	if err := tw.WriteHeader(&tar.Header{
		Name: "sh1llwrt", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
