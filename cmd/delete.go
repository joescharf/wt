package cmd

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/joescharf/worktree-dev/internal/ui"
)

var (
	deleteForce       bool
	deleteBranchFlag  bool
)

var deleteCmd = &cobra.Command{
	Use:               "delete <branch>",
	Aliases:           []string{"rm"},
	Short:             "Close iTerm2 window + remove worktree",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		return deleteRun(args[0])
	},
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Force removal with uncommitted changes")
	deleteCmd.Flags().BoolVar(&deleteBranchFlag, "delete-branch", false, "Also delete the git branch")
	rootCmd.AddCommand(deleteCmd)
}

func deleteRun(branch string) error {
	wtPath, err := gitClient.ResolveWorktree(branch)
	if err != nil {
		return err
	}
	dirname := filepath.Base(wtPath)

	if !isDirectory(wtPath) {
		output.Error("Worktree not found: %s", wtPath)
		// Clean up stale state
		_ = stateMgr.RemoveWorktree(wtPath)
		return fmt.Errorf("worktree not found: %s", wtPath)
	}

	output.Info("Deleting worktree '%s'", ui.Cyan(dirname))

	// Close iTerm2 window if it exists
	ws, _ := stateMgr.GetWorktree(wtPath)
	if ws != nil && ws.ClaudeSessionID != "" {
		if dryRun {
			output.DryRunMsg("Would close iTerm2 window")
		} else if itermClient.IsRunning() && itermClient.SessionExists(ws.ClaudeSessionID) {
			if err := itermClient.CloseWindow(ws.ClaudeSessionID); err != nil {
				output.Warning("Failed to close iTerm2 window: %v", err)
			} else {
				output.Success("Closed iTerm2 window")
				time.Sleep(500 * time.Millisecond) // small delay for iTerm2 to process
			}
		}
	}

	// Remove worktree
	if dryRun {
		output.DryRunMsg("Would remove git worktree: %s", wtPath)
	} else {
		output.Info("Removing git worktree")
		if err := gitClient.WorktreeRemove(wtPath, deleteForce); err != nil {
			return err
		}
		output.Success("Removed git worktree")
	}

	// Delete branch if requested
	if deleteBranchFlag {
		branchName := branch
		if ws != nil && ws.Branch != "" {
			branchName = ws.Branch
		}

		if dryRun {
			output.DryRunMsg("Would delete branch '%s'", branchName)
		} else {
			err := gitClient.BranchDelete(branchName, false)
			if err != nil {
				if deleteForce {
					err = gitClient.BranchDelete(branchName, true)
					if err == nil {
						output.Success("Force-deleted branch '%s'", branchName)
					} else {
						output.Warning("Could not delete branch '%s': %v", branchName, err)
					}
				} else {
					output.Warning("Could not delete branch '%s' (may not exist or not fully merged)", branchName)
				}
			} else {
				output.Success("Deleted branch '%s'", branchName)
			}
		}
	}

	// Remove state entry
	if !dryRun {
		_ = stateMgr.RemoveWorktree(wtPath)
	}

	fmt.Fprintln(output.Out)
	output.Success("Worktree '%s' removed", ui.Cyan(dirname))
	return nil
}
