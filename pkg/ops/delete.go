package ops

import (
	"fmt"
	"path/filepath"

	"github.com/joescharf/wt/pkg/gitops"
)

// Delete removes a single worktree with safety checks.
// safetyCheck is called if opts.Force is false — return false to abort.
// cleanup handles iTerm window closing, state removal, trust removal, etc.
func Delete(git gitops.Client, log Logger, opts DeleteOptions, safetyCheck SafetyCheckFunc, cleanup CleanupFunc) error {
	dirname := filepath.Base(opts.WtPath)

	log.Info("Deleting worktree '%s'", dirname)

	// Safety checks (skip with --force)
	if !opts.Force && safetyCheck != nil {
		safe, err := safetyCheck(opts.WtPath)
		if err != nil {
			return err
		}
		if !safe {
			return fmt.Errorf("delete aborted")
		}
	}

	if cleanup != nil {
		if err := cleanup(opts.WtPath, opts.Branch); err != nil {
			return err
		}
	}

	return nil
}

// DeleteAll removes all worktrees (except the main repo).
// safetyCheck is called per worktree if opts.Force is false.
// cleanup handles per-worktree cleanup (iTerm, state, trust).
func DeleteAll(git gitops.Client, log Logger, opts DeleteOptions, safetyCheck SafetyCheckFunc, cleanup CleanupFunc) (int, error) {
	worktrees, err := git.WorktreeList(opts.RepoPath)
	if err != nil {
		return 0, err
	}

	// Filter out main repo
	var toDelete []gitops.WorktreeInfo
	for _, wt := range worktrees {
		if wt.Path != opts.RepoPath {
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

		if !isDirectory(wt.Path) {
			continue
		}

		// Safety checks
		if !opts.Force && safetyCheck != nil {
			safe, err := safetyCheck(wt.Path)
			if err != nil {
				log.Warning("Safety check failed for '%s': %v (skipping)", dirname, err)
				continue
			}
			if !safe {
				log.Info("Skipping '%s'", dirname)
				continue
			}
		}

		// Cleanup (iTerm, worktree remove, branch delete, state, trust)
		if cleanup != nil {
			if err := cleanup(wt.Path, wt.Branch); err != nil {
				log.Warning("Cleanup failed for '%s': %v", dirname, err)
				continue
			}
		}

		deleted++
	}

	// Run git worktree prune after bulk delete
	if err := git.WorktreePrune(opts.RepoPath); err != nil {
		log.Warning("Failed to run git worktree prune: %v", err)
	}

	log.Success("Deleted %d worktrees", deleted)
	return deleted, nil
}
