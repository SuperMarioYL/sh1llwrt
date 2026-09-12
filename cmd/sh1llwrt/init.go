package main

import (
	"fmt"
	"os"

	"github.com/SuperMarioYL/sh1llwrt/internal/shell"
	"github.com/spf13/cobra"
)

var initFlags = struct {
	shell  string
	dryRun bool
}{}

// initCmd implements: sh1llwrt init
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Wire the sh1llwrt ?-prefix + Ctrl-Space into your zsh/bash rc file",
	Long:  "Writes the sh1llwrt block to ~/.zshrc (honouring $ZDOTDIR) or ~/.bashrc: the ?-prefix, the Ctrl-Space line-rewrite widget, and cursor insertion on zsh. Idempotent: re-running replaces the block. Restart your shell after.",
	RunE: func(cmd *cobra.Command, args []string) error {
		shellName, err := shell.ResolveShell(initFlags.shell)
		if err != nil {
			return err
		}
		if initFlags.dryRun {
			// Preview the exact block Install would write (no markers, no rc
			// file touched).
			fmt.Printf("# (dry-run) sh1llwrt would install this %s block:\n", shellName)
			fmt.Print(shell.FunctionBlock(shellName))
			return nil
		}
		path, err := shell.Install(initFlags.shell)
		if err != nil {
			return fmt.Errorf("install: %w", err)
		}
		fmt.Printf("sh1llwrt: wrote block to %s\n", path)
		fmt.Println("sh1llwrt: restart your shell, then: ? extract foo.tar.gz to /tmp")
		fmt.Println("sh1llwrt: Ctrl-Space rewrites the current line; destructive commands ask y/N")
		// Exit cleanly.
		_ = os.Stdout.Sync()
		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initFlags.shell, "shell", "", "shell to wire (zsh|bash); empty = detect $SHELL")
	initCmd.Flags().BoolVar(&initFlags.dryRun, "dry-run", false, "print the block that would be installed, write nothing")
	rootCmd.AddCommand(initCmd)
}
