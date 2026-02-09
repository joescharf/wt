package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/wt/internal/git"
	"github.com/joescharf/wt/internal/ui"
)

var (
	deleteForce      bool
	deleteBranchFlag bool
	deleteAll        bool
)

// promptFunc is the confirmation prompt, replaceable in tests.
var promptFunc = defaultPrompt

func defaultPrompt(msg string) bool {
	fmt.Fprintf(output.ErrOut, "%s [y/N] ", msg)
	var answer string
	fmt.Fscanln(os.Stdin, &answer)
	return strings.ToLower(strings.TrimSpace(answer)) == "y"
}

var deleteCmd = &cobra.Command{
	Use:               "delete [branch]",
	Aliases:           []string{"rm"},
	Short:             "Close iTerm2 window + remove worktree",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if deleteAll {
			return deleteAllRun()
		}
		if len(args) == 0 {
			return fmt.Errorf("branch argument required (or use --all)")
		}
		return deleteRun(args[0])
	},
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Force removal, skip safety checks")
	deleteCmd.Flags().BoolVar(&deleteBranchFlag, "delete-branch", false, "Also delete the git branch")
	deleteCmd.Flags().BoolVar(&deleteAll, "all", false, "Delete all worktrees")
	rootCmd.AddCommand(deleteCmd)
}

// checkWorktreeSafety returns true if the worktree is safe to delete (no data loss risk).
// If unsafe, it prints warnings and prompts the user. Returns false if the user declines.
func checkWorktreeSafety(wtPath, dirname string) bool {
	dirty, err := gitClient.IsWorktreeDirty(wtPath)
	if err != nil {
		output.VerboseLog("Could not check worktree status: %v", err)
	}

	if dirty {
		output.Warning("'%s' has uncommitted changes", dirname)
		if dryRun {
			output.DryRunMsg("Would prompt for confirmation (uncommitted changes)")
			return true
		}
		return promptFunc(fmt.Sprintf("Delete '%s' with uncommitted changes?", dirname))
	}

	baseBranch := viper.GetString("base_branch")
	unpushed, err := gitClient.HasUnpushedCommits(wtPath, baseBranch)
	if err != nil {
		output.VerboseLog("Could not check unpushed commits: %v", err)
	}

	if unpushed {
		output.Warning("'%s' has unpushed commits", dirname)
		if dryRun {
			output.DryRunMsg("Would prompt for confirmation (unpushed commits)")
			return true
		}
		return promptFunc(fmt.Sprintf("Delete '%s' with unpushed commits?", dirname))
	}

	return true
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

	// Safety checks (skip with --force)
	if !deleteForce {
		if !checkWorktreeSafety(wtPath, dirname) {
			return fmt.Errorf("delete aborted")
		}
	}

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

func deleteAllRun() error {
	repoRoot, err := gitClient.RepoRoot()
	if err != nil {
		return err
	}

	worktrees, err := gitClient.WorktreeList()
	if err != nil {
		return err
	}

	// Filter out main repo
	var toDelete []git.WorktreeInfo
	for _, wt := range worktrees {
		if wt.Path != repoRoot {
			toDelete = append(toDelete, wt)
		}
	}

	if len(toDelete) == 0 {
		output.Info("No worktrees to delete")
		return nil
	}

	output.Info("Found %d worktrees to delete", len(toDelete))

	deleted := 0
	for _, wt := range toDelete {
		dirname := filepath.Base(wt.Path)

		if !isDirectory(wt.Path) {
			continue
		}

		// Safety checks (skip with --force)
		if !deleteForce {
			if !checkWorktreeSafety(wt.Path, dirname) {
				output.Info("Skipping '%s'", dirname)
				continue
			}
		}

		// Close iTerm2 window
		ws, _ := stateMgr.GetWorktree(wt.Path)
		if ws != nil && ws.ClaudeSessionID != "" {
			if itermClient.IsRunning() && itermClient.SessionExists(ws.ClaudeSessionID) {
				if err := itermClient.CloseWindow(ws.ClaudeSessionID); err != nil {
					output.Warning("Failed to close window for %s: %v", dirname, err)
				}
			}
		}

		// Remove worktree
		if err := gitClient.WorktreeRemove(wt.Path, deleteForce); err != nil {
			output.Warning("Failed to remove %s: %v", dirname, err)
			continue
		}

		// Delete branch if requested
		if deleteBranchFlag {
			branchName := wt.Branch
			if ws != nil && ws.Branch != "" {
				branchName = ws.Branch
			}
			if err := gitClient.BranchDelete(branchName, deleteForce); err != nil {
				output.Warning("Could not delete branch '%s': %v", branchName, err)
			}
		}

		_ = stateMgr.RemoveWorktree(wt.Path)
		output.Success("Removed '%s'", ui.Cyan(dirname))
		deleted++
	}

	// Run git worktree prune after bulk delete
	if err := gitClient.WorktreePrune(); err != nil {
		output.Warning("Failed to run git worktree prune: %v", err)
	}

	fmt.Fprintln(output.Out)
	output.Success("Deleted %d worktrees", deleted)
	return nil
}
