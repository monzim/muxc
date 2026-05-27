// Package cli — completion subcommand.
// This command is fully implemented in Wave 0 because it has no external deps;
// cobra provides all generation logic. See spec §11.8.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// completionCmd emits shell completion scripts to stdout.
var completionCmd = &cobra.Command{
	Use:   "completion <shell>",
	Short: "Generate shell completion scripts",
	Long: `Generate a shell completion script for muxc and print it to stdout.

Supported shells: bash, zsh, fish

Installation examples:
  Bash:   muxc completion bash > /etc/bash_completion.d/muxc
  Zsh:    muxc completion zsh > "${fpath[1]}/_muxc"
  Fish:   muxc completion fish > ~/.config/fish/completions/muxc.fish`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish"},
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		default:
			return fmt.Errorf("muxc: unsupported shell %q; choose bash, zsh, or fish", args[0])
		}
	},
}
