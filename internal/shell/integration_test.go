package shell

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONIntegratorRoundTrip(t *testing.T) {
	// A command with a double quote, a backslash, and an embedded newline, and
	// an explain with quotes: everything the v0.1 double-escaper corrupted.
	view := AskView{
		Command: `echo "hello \" there` + "\nsecond line",
		Safe:    false,
		Explain: `say "hello" and more`,
	}
	var buf bytes.Buffer
	if err := (&JSONIntegrator{W: &buf}).Insert(view); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	out := buf.String()
	if !strings.HasSuffix(out, "\n") || strings.Count(out, "\n") != 1 {
		t.Fatalf("output must be exactly one line ending in \\n, got %q", out)
	}
	var got AskView
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}
	if got.Command != view.Command || got.Safe != view.Safe || got.Explain != view.Explain {
		t.Fatalf("round-trip mismatch:\nwant %+v\n got %+v", view, got)
	}
}

func TestPrintIntegratorFormat(t *testing.T) {
	view := AskView{
		Command: "tar -xzf foo.tar.gz -C /tmp",
		Safe:    false,
		Explain: "extract gzipped tarball into /tmp",
	}
	var buf bytes.Buffer
	if err := (&PrintIntegrator{W: &buf}).Insert(view); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	want := "# extract gzipped tarball into /tmp\ntar -xzf foo.tar.gz -C /tmp\n# destructive — review before pressing Enter\n"
	if buf.String() != want {
		t.Fatalf("print format changed:\nwant %q\n got %q", want, buf.String())
	}
}

func TestInlineIntegratorSafeCommandOnly(t *testing.T) {
	var buf bytes.Buffer
	if err := (&InlineIntegrator{W: &buf}).Insert(AskView{Command: "kubectl get pods", Safe: true, Explain: "read-only"}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if buf.String() != "kubectl get pods\n" {
		t.Fatalf("insert must print the command only, got %q", buf.String())
	}
}

func TestInlineIntegratorDestructiveConfirmAccepted(t *testing.T) {
	var out, errb bytes.Buffer
	in := strings.NewReader("y\n")
	err := (&InlineIntegrator{W: &out, Err: &errb, R: in}).Insert(AskView{Command: "rm -rf /tmp/old", Safe: false})
	if err != nil {
		t.Fatalf("y must approve: %v", err)
	}
	if out.String() != "rm -rf /tmp/old\n" {
		t.Fatalf("approved insert must print the command, got %q", out.String())
	}
	if !strings.Contains(errb.String(), "proceed? [y/N]") || !strings.Contains(errb.String(), "rm -rf /tmp/old") {
		t.Fatalf("confirm prompt must show the command and [y/N] on stderr, got %q", errb.String())
	}
}

func TestInlineIntegratorDestructiveDeclinesClosed(t *testing.T) {
	// EOF (no input at all — scripts, tests, </dev/null): must decline with no output.
	var out1, errb1 bytes.Buffer
	err := (&InlineIntegrator{W: &out1, Err: &errb1, R: strings.NewReader("")}).Insert(AskView{Command: "rm -rf /tmp/old", Safe: false})
	if err == nil {
		t.Fatal("EOF must decline")
	}
	if out1.Len() != 0 {
		t.Fatalf("declined insert must write no command, got %q", out1.String())
	}
	// An explicit "n" declines the same way.
	var out2 bytes.Buffer
	err = (&InlineIntegrator{W: &out2, Err: &bytes.Buffer{}, R: strings.NewReader("n\n")}).Insert(AskView{Command: "rm -rf /tmp/old", Safe: false})
	if err == nil {
		t.Fatal("n must decline")
	}
	if out2.Len() != 0 {
		t.Fatalf("declined insert must write no command, got %q", out2.String())
	}
}
