package ops

import "github.com/joescharf/wt/pkg/gitops"

// Prune cleans up stale state, trust entries, and git worktree tracking.
// statePrune and trustPrune are callback functions for pruning state and trust;
// trustPrune may be nil if trust management is not configured.
func Prune(git gitops.Client, log Logger, opts PruneOptions, statePrune StatePruner, trustPrune TrustPruner) (*PruneResult, error) {
	result := &PruneResult{}

	// Prune stale state entries
	if statePrune != nil {
		pruned, err := statePrune()
		if err != nil {
			log.Warning("Failed to prune state: %v", err)
		}
		if pruned > 0 {
			log.Info("Pruned %d stale state entries", pruned)
		}
		result.StatePruned = pruned
	}

	// Prune stale trust entries
	if trustPrune != nil {
		wtDir, err := git.WorktreesDir(opts.RepoPath)
		if err == nil {
			pruned, err := trustPrune(wtDir)
			if err != nil {
				log.Warning("Failed to prune trust entries: %v", err)
			} else if pruned > 0 {
				log.Info("Pruned %d stale trust entries", pruned)
			}
			result.TrustPruned = pruned
		}
	}

	// Run git worktree prune
	if opts.DryRun {
		log.Info("Would run git worktree prune")
	} else {
		if err := git.WorktreePrune(opts.RepoPath); err != nil {
			log.Warning("Failed to run git worktree prune: %v", err)
		} else {
			log.Verbose("Ran git worktree prune")
			result.GitPruned = true
		}
	}

	totalPruned := result.StatePruned + result.TrustPruned
	if totalPruned == 0 {
		log.Success("Everything clean, nothing to prune")
	} else {
		log.Success("Prune complete")
	}

	return result, nil
}
