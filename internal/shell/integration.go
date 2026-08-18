// Package shell owns the "meets you in your $SHELL" half: turning an AskReply
// into something the user can act on (printed for copy in m1; inserted at the
// cursor in m2), and writing the shell function that wires the binary into
// zsh/bash.
package shell

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Integrator renders an AskReply for the user.
type Integrator interface {
	Insert(reply AskView) error
}

// AskView is the minimal projection of a parsed reply onto the shell surface.
type AskView struct {
	Command string
	Safe    bool
	Explain string
}

// ErrNotImplementedM2 marks the m2-only inline-insert path as not yet wired.
var ErrNotImplementedM2 = fmt.Errorf("inline insert not implemented in m1 — see roadmap (m2_shell_meet)")

// PrintIntegrator is the m1 surface: it prints the command and a one-line
// explain to stdout for the user to copy or read. When the command is
// destructive it prints a confirm hint; v0.1 never auto-executes, so this is
// advisory, never blocking.
type PrintIntegrator struct {
	W io.Writer
}

// Insert writes the reply in the m1 print format.
func (p *PrintIntegrator) Insert(v AskView) error {
	w := p.W
	if w == nil {
		w = os.Stdout
	}
	if v.Command == "" {
		return fmt.Errorf("nothing to insert")
	}
	if v.Explain != "" {
		fmt.Fprintf(w, "# %s\n", v.Explain)
	}
	fmt.Fprintln(w, v.Command)
	if !v.Safe {
		fmt.Fprintln(w, "# destructive — review before pressing Enter")
	}
	return nil
}

// JSONIntegrator prints the parsed reply as a single JSON object for scripting
// and for the demo harness. (Strict shape; safe even when mock.)
type JSONIntegrator struct {
	W io.Writer
}

// Insert writes a compact JSON object.
func (j *JSONIntegrator) Insert(v AskView) error {
	w := j.W
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintf(w, `{"command":%q,"safe":%t,"explain":%q}`+"\n",
		escapeJSON(v.Command), v.Safe, escapeJSON(v.Explain))
	return nil
}

// InlineIntegrator is the m2 surface: insert the command at the cursor via a
// zsh/bash widget. It is stubbed in m1; calling it returns ErrNotImplementedM2
// so the --insert flag fails closed rather than silently printing.
type InlineIntegrator struct{}

func (i *InlineIntegrator) Insert(v AskView) error { return ErrNotImplementedM2 }

// escapeJSON returns a minimal JSON string escape. We avoid pulling encoding/json
// here so the surface stays tiny and the output is exactly one line.
func escapeJSON(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	)
	return r.Replace(s)
}
