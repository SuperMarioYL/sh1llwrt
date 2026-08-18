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
	Short: "Wire the sh1llwrt `?`-prefix into your zsh/bash rc file",
	Long:  "Appends a 4-line sh1llwrt block to ~/.zshrc (or ~/.bashrc). Idempotent: re-running replaces the block. Restart your shell after.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if initFlags.dryRun {
			// Print the block that would be installed without touching rc.
			fmt.Println("# (dry-run) sh1llwrt would append:")
			fmt.Print(sh1llwrtDryBlock())
			return nil
		}
		path, err := shell.Install(initFlags.shell)
		if err != nil {
			return fmt.Errorf("install: %w", err)
		}
		fmt.Printf("sh1llwrt: wrote block to %s\n", path)
		fmt.Println("sh1llwrt: restart your shell, then: ? extract foo.tar.gz to /tmp")
		fmt.Println("sh1llwrt: inline-insert Ctrl-Space widget lands in m2 (see roadmap)")
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

// sh1llwrtDryBlock returns a human-readable preview of the function block for
// --dry-run. It does not splice markers.
func sh1llwrtDryBlock() string {
	const block = `# sh1llwrt: the offline in-shell command generator. See https://github.com/SuperMarioYL/sh1llwrt
sh1llwrt_ask() {
  sh1llwrt ask "$@" 2>/dev/null || echo "sh1llwrt: ask failed (is the model cached?)"
}
# ?-prefix: type a sentence, get the command printed for copy.
function ?() { sh1llwrt_ask "$@"; }

# end of sh1llwrt block
`
	return block
}
