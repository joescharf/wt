package ops

import (
	"context"

	"github.com/joescharf/wt/pkg/gitops"
	"github.com/joescharf/wt/pkg/wtstate"
)

// Prune cleans up stale state entries and runs git worktree prune.
func Prune(ctx context.Context, git gitops.Client, state *wtstate.Manager,
	log Logger, dryRun bool) (*PruneResult, error) {

	result := &PruneResult{
		DryRun: dryRun,
	}

	// Prune stale state entries
	if state != nil {
		pruned, err := state.Prune()
		if err != nil {
			log.Warning("Failed to prune state: %v", err)
		}
		result.StatePruned = pruned

		if pruned > 0 {
			log.Info("Pruned %d stale state entries", pruned)
		}
	}

	// Run git worktree prune
	if dryRun {
		log.Info("Would run git worktree prune")
	} else {
		if err := git.WorktreePrune(); err != nil {
			log.Warning("Failed to run git worktree prune: %v", err)
		} else {
			result.GitPruned = true
			log.Verbose("Ran git worktree prune")
		}
	}

	if result.StatePruned == 0 {
		log.Success("Everything clean, nothing to prune")
	} else {
		log.Success("Prune complete")
	}

	return result, nil
}
