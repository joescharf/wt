package ops

import "github.com/joescharf/wt/pkg/gitops"

// Discover finds worktrees that are not managed by wt state.
// stateCheck returns true if a worktree path is already managed.
// stateAdopt creates a state entry for an unmanaged worktree (called if opts.Adopt is true).
func Discover(git gitops.Client, log Logger, opts DiscoverOptions, stateCheck StateChecker, stateAdopt StateAdopter) (*DiscoverResult, error) {
	repoName, err := git.RepoName(opts.RepoPath)
	if err != nil {
		return nil, err
	}

	wtDir, err := git.WorktreesDir(opts.RepoPath)
	if err != nil {
		return nil, err
	}

	worktrees, err := git.WorktreeList(opts.RepoPath)
	if err != nil {
		return nil, err
	}

	result := &DiscoverResult{RepoName: repoName}

	// Find worktrees not in state
	for _, wt := range worktrees {
		if wt.Path == opts.RepoPath {
			continue
		}

		managed, err := stateCheck(wt.Path)
		if err != nil {
			log.Verbose("Could not check state for '%s': %v", wt.Path, err)
			continue
		}
		if managed {
			continue
		}

		result.Unmanaged = append(result.Unmanaged, UnmanagedWorktree{
			Path:   wt.Path,
			Branch: wt.Branch,
			Source: classifySource(wt.Path, wtDir),
		})
	}

	if len(result.Unmanaged) == 0 {
		log.Success("No unmanaged worktrees found")
		return result, nil
	}

	log.Info("Found %d unmanaged worktrees for '%s'", len(result.Unmanaged), repoName)
	for _, wt := range result.Unmanaged {
		log.Info("  %s  %s  (%s)", wt.Branch, wt.Path, wt.Source)
	}

	if !opts.Adopt {
		log.Info("Run with --adopt to create state entries for these worktrees")
		return result, nil
	}

	if opts.DryRun {
		log.Info("Would adopt %d worktrees", len(result.Unmanaged))
		return result, nil
	}

	if stateAdopt == nil {
		return result, nil
	}

	for _, wt := range result.Unmanaged {
		if err := stateAdopt(wt.Path, repoName, wt.Branch); err != nil {
			log.Warning("Failed to adopt '%s': %v", wt.Branch, err)
			continue
		}
		log.Success("Adopted '%s'", wt.Branch)
		result.Adopted++
	}

	log.Success("Adopted %d worktrees", result.Adopted)
	return result, nil
}
