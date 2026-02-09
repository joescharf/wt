package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Set by Execute() from main.go ldflags.
var (
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("wt version %s (commit: %s, built: %s)\n", buildVersion, buildCommit, buildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
