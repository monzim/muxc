// Package cli — version subcommand.
// Fully implemented in Wave 0. Build metadata is injected via SetBuildInfo
// called from cmd/muxc/main.go before Execute. See spec §11.9.
package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// versionCmd prints the binary version line.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print muxc version, git commit, build date, and Go runtime version.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("muxc %s (commit %s, built %s, go %s)\n",
			buildInfo.version,
			buildInfo.commit,
			buildInfo.date,
			runtime.Version(),
		)
	},
}
