package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SuperMarioYL/sh1llwrt/internal/llama"
	"github.com/SuperMarioYL/sh1llwrt/internal/model"
	"github.com/SuperMarioYL/sh1llwrt/internal/prompt"
	"github.com/SuperMarioYL/sh1llwrt/internal/shell"
	"github.com/spf13/cobra"
)

var askFlags = struct {
	mock      bool
	shellName string
	cwd       string
	glob      []string
	threads   int
	maxTokens int
	temperature float64
	verbose   bool
	insert    bool
	json      bool
	noModel   bool
}{}

// askCmd implements: sh1llwrt ask "<nl sentence>"
var askCmd = &cobra.Command{
	Use:   "ask [flags] <natural-language sentence>",
	Short: "Turn a natural-language sentence into a shell command, offline",
	Long:  "Turn a natural-language sentence into a parseable shell command via a local model. m1 prints the command for copy; the inline-insert (--insert) path is m2.",
	Args:  cobra.MinimumNArgs(1),
	RunE: runAsk,
}

func init() {
	askCmd.Flags().BoolVar(&askFlags.mock, "mock", false, "use the deterministic mock backend (no model needed; for tests/demo)")
	askCmd.Flags().StringVar(&askFlags.shellName, "shell", "", "shell grammar (zsh|bash); empty = detect $SHELL")
	askCmd.Flags().StringVar(&askFlags.cwd, "cwd", "", "working dir context; empty = $PWD")
	askCmd.Flags().StringSliceVar(&askFlags.glob, "glob", nil, "files the user named, injected as context (repeatable)")
	askCmd.Flags().IntVar(&askFlags.threads, "threads", 0, "CPU threads (0 = NumCPU)")
	askCmd.Flags().IntVar(&askFlags.maxTokens, "max-tokens", 96, "cap reply length in tokens")
	askCmd.Flags().Float64Var(&askFlags.temperature, "temperature", 0, "sampling temperature (0 = greedy/deterministic)")
	askCmd.Flags().BoolVar(&askFlags.verbose, "verbose", false, "print the underlying llama-cli command to stderr")
	askCmd.Flags().BoolVar(&askFlags.insert, "insert", false, "insert the command at the cursor (m2; m1 fails closed)")
	askCmd.Flags().BoolVar(&askFlags.json, "json", false, "emit the reply as a single JSON object")
	askCmd.Flags().BoolVar(&askFlags.noModel, "no-model", false, "skip model download/verify (only with --mock)")
	rootCmd.AddCommand(askCmd)
}

func runAsk(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	intent := strings.Join(args, " ")
	shellName := askFlags.shellName
	if shellName == "" {
		s, _, err := shell.Detect()
		if err == nil {
			shellName = s
		} else {
			shellName = "bash"
		}
	}
	cwd := askFlags.cwd
	if cwd == "" {
		cwd = os.Getenv("PWD")
		if cwd == "" {
			if wd, err := os.Getwd(); err == nil {
				cwd = wd
			}
		}
	}

	req := prompt.AskRequest{
		Shell:  shellName,
		Cwd:    cwd,
		Glob:   askFlags.glob,
		Intent: intent,
	}
	promptText := prompt.BuildPrompt(req)

	// Model path: skip entirely under --mock or SH1LLWRT_MOCK.
	modelPath := ""
	if !(askFlags.mock || os.Getenv("SH1LLWRT_MOCK") != "") {
		c := model.NewCache()
		p, err := c.Ensure(ctx, func(format string, a ...any) {
			if askFlags.verbose {
				fmt.Fprintf(os.Stderr, "sh1llwrt: "+format+"\n", a...)
			}
		})
		if err != nil {
			return fmt.Errorf("ensure model: %w", err)
		}
		modelPath = p
	}

	runner := llama.ResolveRunner(llama.Options{
		ModelPath: modelPath,
		Threads:   askFlags.threads,
		MaxTokens: askFlags.maxTokens,
		Temperature: askFlags.temperature,
		Verbose:   askFlags.verbose,
	}, askFlags.mock)

	raw, err := runner.Complete(ctx, promptText)
	if err != nil {
		return fmt.Errorf("inference: %w", err)
	}
	reply, err := prompt.ParseReply(raw)
	if err != nil {
		return fmt.Errorf("parse reply: %w (raw: %q)", err, raw)
	}

	view := shell.AskView{
		Command: reply.Command,
		Safe:    reply.Safe,
		Explain: reply.Explain,
	}

	var integrator shell.Integrator
	switch {
	case askFlags.insert:
		integrator = &shell.InlineIntegrator{}
	case askFlags.json:
		integrator = &shell.JSONIntegrator{W: os.Stdout}
	default:
		integrator = &shell.PrintIntegrator{W: os.Stdout}
	}
	return integrator.Insert(view)
}
