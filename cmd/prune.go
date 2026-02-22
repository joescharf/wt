package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
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
	// Prune stale state entries
	pruned, err := stateMgr.Prune()
	if err != nil {
		output.Warning("Failed to prune state: %v", err)
	}

	if pruned > 0 {
		output.Info("Pruned %d stale state entries", pruned)
	}

	// Prune stale Claude trust entries
	if claudeTrust != nil {
		wtDir, err := gitClient.WorktreesDir(repoRoot)
		if err == nil {
			trustPruned, err := claudeTrust.PruneProjects(wtDir)
			if err != nil {
				output.Warning("Failed to prune Claude trust entries: %v", err)
			} else if trustPruned > 0 {
				output.Info("Pruned %d stale Claude trust entries", trustPruned)
				pruned += trustPruned
			}
		}
	}

	// Run git worktree prune
	if dryRun {
		output.DryRunMsg("Would run git worktree prune")
	} else {
		if err := gitClient.WorktreePrune(repoRoot); err != nil {
			output.Warning("Failed to run git worktree prune: %v", err)
		} else {
			output.VerboseLog("Ran git worktree prune")
		}
	}

	if pruned == 0 {
		output.Success("Everything clean, nothing to prune")
	} else {
		fmt.Fprintln(output.Out)
		output.Success("Prune complete")
	}

	return nil
}
