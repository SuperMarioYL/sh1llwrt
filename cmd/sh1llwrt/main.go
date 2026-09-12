// Command sh1llwrt is the offline in-shell command generator: a single binary
// that runs a 941MB local coder model on your machine, no account, no internet
// after the one-time model download.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is stamped at release time via goreleaser ldflags
// (-ldflags "-X main.version=<v>"). Dev builds keep the -dev suffix; the
// canonical version source is the repo-root VERSION file, read by goreleaser
// and the publish tooling (not the binary, which has no repo at runtime).
var version = "0.2.0-dev"

var rootCmd = &cobra.Command{
	Use:     "sh1llwrt",
	Short:   "Offline local model that writes the shell command you meant, no account",
	Long:    "sh1llwrt runs a 941MB Qwen2.5-Coder-1.5B Q4_K_M model on your machine, in your shell. Type a sentence; get the command in under a second. No account, no internet after the one-time model download.",
	Version: version,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
