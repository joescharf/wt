package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/joescharf/wt/pkg/ops"
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Clean up stale state and git worktree tracking",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return pruneRun()
	},
}

func init() {
	rootCmd.AddCommand(pruneCmd)
}

func pruneRun() error {
	// Build trust pruner (nil-safe)
	var trustPrune ops.TrustPruner
	if claudeTrust != nil {
		trustPrune = func(dir string) (int, error) {
			return claudeTrust.PruneProjects(dir)
		}
	}

	result, err := ops.Prune(gitClient, opsLogger, ops.PruneOptions{
		RepoPath: repoRoot,
		DryRun:   dryRun,
	}, stateMgr.Prune, trustPrune)
	if err != nil {
		return err
	}

	totalPruned := result.StatePruned + result.TrustPruned
	if totalPruned > 0 {
		_, _ = fmt.Fprintln(output.Out)
	}
	return nil
}
