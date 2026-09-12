// Package shell owns the "meets you in your $SHELL" half: turning an AskReply
// into something the user can act on (printed for copy, a JSON object for
// scripts, or the command-only line the insert path captures), and writing
// the shell function that wires the binary into zsh/bash.
package shell

import (
	"bufio"
	"encoding/json"
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

// PrintIntegrator prints the command and a one-line explain to stdout for the
// user to copy or read. When the command is destructive it prints a confirm
// hint; sh1llwrt never auto-executes, so this is advisory, never blocking.
type PrintIntegrator struct {
	W io.Writer
}

// Insert writes the reply in the print format.
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
// and for the demo harness. It marshals via encoding/json so quotes,
// backslashes, control characters, and invalid UTF-8 round-trip exactly (the
// v0.1 hand-rolled escaper double-escaped them and corrupted the reply).
type JSONIntegrator struct {
	W io.Writer
}

// jsonView pins the wire shape: one line, fields command/safe/explain.
type jsonView struct {
	Command string `json:"command"`
	Safe    bool   `json:"safe"`
	Explain string `json:"explain"`
}

// Insert writes a compact JSON object followed by a newline.
func (j *JSONIntegrator) Insert(v AskView) error {
	w := j.W
	if w == nil {
		w = os.Stdout
	}
	b, err := json.Marshal(jsonView{Command: v.Command, Safe: v.Safe, Explain: v.Explain})
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// InlineIntegrator is the m2 insert surface: it prints ONLY the command so the
// shell function can capture it with $(...) and place it at the cursor. A
// destructive reply gates on an explicit y/N read from R (default stdin) —
// anything other than y/Y, including EOF, declines with no output: the insert
// path fails closed. The prompt goes to Err (default stderr) so it stays
// visible even while stdout is being captured by the calling shell function.
type InlineIntegrator struct {
	// W receives the command (default os.Stdout).
	W io.Writer
	// Err receives the y/N prompt (default os.Stderr).
	Err io.Writer
	// R is the confirm input stream (default os.Stdin).
	R io.Reader
}

// Insert prints the command line, after the y/N gate when destructive.
func (i *InlineIntegrator) Insert(v AskView) error {
	if v.Command == "" {
		return fmt.Errorf("nothing to insert")
	}
	if !v.Safe {
		errw := i.Err
		if errw == nil {
			errw = os.Stderr
		}
		fmt.Fprintf(errw, "sh1llwrt: destructive — review: %s\n", v.Command)
		fmt.Fprint(errw, "proceed? [y/N] ")
		r := i.R
		if r == nil {
			r = os.Stdin
		}
		line, _ := bufio.NewReader(r).ReadString('\n')
		if t := strings.TrimSpace(line); t != "y" && t != "Y" {
			return fmt.Errorf("declined — nothing inserted")
		}
	}
	w := i.W
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintln(w, v.Command)
	return nil
}
