package ops

import (
	"context"
	"fmt"
	"path/filepath"
	"os"

	"github.com/joescharf/wt/pkg/gitops"
	"github.com/joescharf/wt/pkg/wtstate"
)

// Delete removes a worktree and optionally its branch.
// The safetyCheck callback is called when the worktree has uncommitted/unpushed changes.
// The cleanupFunc callback is called after worktree removal for wt-specific cleanup.
func Delete(ctx context.Context, git gitops.Client, state *wtstate.Manager,
	log Logger, wtPath string, opts DeleteOptions, safetyCheck SafetyCheckFunc, cleanupFunc CleanupFunc) error {

	dirname := filepath.Base(wtPath)

	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		// Clean up stale state
		if state != nil {
			_ = state.RemoveWorktree(wtPath)
		}
		return fmt.Errorf("worktree not found: %s", wtPath)
	}

	log.Info("Deleting worktree '%s'", dirname)

	// Get branch name from state
	branchName := dirname
	if state != nil {
		ws, _ := state.GetWorktree(wtPath)
		if ws != nil && ws.Branch != "" {
			branchName = ws.Branch
		}
	}

	// Safety checks (skip with Force)
	if !opts.Force && safetyCheck != nil {
		if !safetyCheck(wtPath, dirname) {
			return fmt.Errorf("delete aborted")
		}
	}

	// Remove git worktree
	if opts.DryRun {
		log.Info("Would remove git worktree: %s", wtPath)
	} else {
		log.Info("Removing git worktree")
		if err := git.WorktreeRemove(wtPath, opts.Force); err != nil {
			return err
		}
		log.Success("Removed git worktree")
	}

	// Delete branch if requested
	if opts.DeleteBranch {
		if opts.DryRun {
			log.Info("Would delete branch '%s'", branchName)
		} else {
			err := git.BranchDelete(branchName, false)
			if err != nil {
				if opts.Force {
					err = git.BranchDelete(branchName, true)
					if err == nil {
						log.Success("Force-deleted branch '%s'", branchName)
					} else {
						log.Warning("Could not delete branch '%s': %v", branchName, err)
					}
				} else {
					log.Warning("Could not delete branch '%s' (may not exist or not fully merged)", branchName)
				}
			} else {
				log.Success("Deleted branch '%s'", branchName)
			}
		}
	}

	// Remove state entry
	if !opts.DryRun && state != nil {
		_ = state.RemoveWorktree(wtPath)
	}

	// Run additional cleanup (iTerm2, Claude trust, etc.)
	if !opts.DryRun && cleanupFunc != nil {
		if err := cleanupFunc(wtPath, branchName); err != nil {
			log.Warning("Cleanup callback failed: %v", err)
		}
	}

	log.Success("Worktree '%s' removed", dirname)
	return nil
}

// DeleteAll removes all worktrees (excluding the main repo).
func DeleteAll(ctx context.Context, git gitops.Client, state *wtstate.Manager,
	log Logger, opts DeleteOptions, safetyCheck SafetyCheckFunc, cleanupFunc CleanupFunc) (int, error) {

	repoRoot, err := git.RepoRoot()
	if err != nil {
		return 0, err
	}

	worktrees, err := git.WorktreeList()
	if err != nil {
		return 0, err
	}

	// Filter out main repo
	var toDelete []gitops.WorktreeInfo
	for _, wt := range worktrees {
		if wt.Path != repoRoot {
			toDelete = append(toDelete, wt)
		}
	}

	if len(toDelete) == 0 {
		log.Info("No worktrees to delete")
		return 0, nil
	}

	log.Info("Found %d worktrees to delete", len(toDelete))

	deleted := 0
	for _, wt := range toDelete {
		dirname := filepath.Base(wt.Path)

		if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
			continue
		}

		// Safety checks
		if !opts.Force && safetyCheck != nil {
			if !safetyCheck(wt.Path, dirname) {
				log.Info("Skipping '%s'", dirname)
				continue
			}
		}

		// Run cleanup callback before removal
		if !opts.DryRun && cleanupFunc != nil {
			_ = cleanupFunc(wt.Path, wt.Branch)
		}

		// Remove worktree
		if err := git.WorktreeRemove(wt.Path, opts.Force); err != nil {
			log.Warning("Failed to remove %s: %v", dirname, err)
			continue
		}

		// Delete branch if requested
		if opts.DeleteBranch {
			branchName := wt.Branch
			if state != nil {
				ws, _ := state.GetWorktree(wt.Path)
				if ws != nil && ws.Branch != "" {
					branchName = ws.Branch
				}
			}
			if err := git.BranchDelete(branchName, opts.Force); err != nil {
				log.Warning("Could not delete branch '%s': %v", branchName, err)
			}
		}

		if state != nil {
			_ = state.RemoveWorktree(wt.Path)
		}

		log.Success("Removed '%s'", dirname)
		deleted++
	}

	// Run git worktree prune after bulk delete
	if err := git.WorktreePrune(); err != nil {
		log.Warning("Failed to run git worktree prune: %v", err)
	}

	return deleted, nil
}
