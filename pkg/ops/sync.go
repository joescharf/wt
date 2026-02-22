package ops

import (
	"fmt"
	"path/filepath"

	"github.com/joescharf/wt/pkg/gitops"
)

// Sync synchronizes a single worktree with its base branch.
// The caller must resolve WtPath and Branch before calling (e.g., from state or git).
func Sync(git gitops.Client, log Logger, opts SyncOptions) (*SyncResult, error) {
	result := &SyncResult{
		Branch:   opts.Branch,
		Strategy: opts.Strategy,
	}
	dirname := filepath.Base(opts.WtPath)

	// Safety check: dirty worktree
	if !opts.Force {
		dirty, err := git.IsWorktreeDirty(opts.WtPath)
		if err != nil {
			log.Warning("Could not check worktree status: %v", err)
		}
		if dirty {
			return result, fmt.Errorf("worktree '%s' has uncommitted changes (use --force to skip)", dirname)
		}
	}

	// Check if a merge or rebase is already in progress (idempotent — pick up where we left off)
	mergeInProgress, err := git.IsMergeInProgress(opts.WtPath)
	if err != nil {
		log.Verbose("Could not check merge status: %v", err)
	}
	if mergeInProgress {
		return syncContinueMerge(git, log, opts, result)
	}

	rebaseInProgress, err := git.IsRebaseInProgress(opts.WtPath)
	if err != nil {
		log.Verbose("Could not check rebase status: %v", err)
	}
	if rebaseInProgress {
		return syncContinueRebase(git, log, opts, result)
	}

	// Determine merge source based on remote availability
	mergeSource, _ := resolveMergeSource(git, log, opts.RepoPath, opts.BaseBranch, opts.DryRun)

	// Resolve effective merge source (check local vs remote behind counts)
	effectiveSource, ahead, behind := resolveEffectiveMergeSource(git, log, opts.WtPath, opts.BaseBranch, mergeSource)
	result.Ahead = ahead
	result.Behind = behind

	log.Info("Status of '%s' vs '%s': %s", opts.Branch, opts.BaseBranch, FormatSyncStatus(ahead, behind))

	if behind == 0 {
		log.Success("'%s' is already in sync with '%s'", opts.Branch, opts.BaseBranch)
		result.AlreadySynced = true
		result.Success = true
		return result, nil
	}

	if opts.Strategy == "rebase" {
		log.Info("Rebasing '%s' onto '%s' (%d commit(s) behind)", opts.Branch, opts.BaseBranch, behind)

		if opts.DryRun {
			log.Info("Would rebase '%s' onto '%s'", opts.Branch, effectiveSource)
			result.Success = true
		} else {
			if err := git.Rebase(opts.WtPath, effectiveSource); err != nil {
				log.Warning("Rebase failed — resolve conflicts, then run sync again (or 'git -C %s rebase --abort' to cancel)", opts.WtPath)
				result.Conflict = true
				return result, fmt.Errorf("rebase conflict: %w", err)
			}
			log.Success("Rebased '%s' onto '%s'", opts.Branch, opts.BaseBranch)
			result.Success = true
		}
	} else {
		log.Info("Merging %d commit(s) from '%s' into '%s'", behind, opts.BaseBranch, opts.Branch)

		if opts.DryRun {
			log.Info("Would merge '%s' into '%s'", effectiveSource, opts.Branch)
			result.Success = true
		} else {
			if err := git.Merge(opts.WtPath, effectiveSource); err != nil {
				log.Warning("Merge failed — resolve conflicts, then run sync again")
				result.Conflict = true
				return result, fmt.Errorf("merge conflict: %w", err)
			}
			log.Success("Synced '%s' with '%s'", opts.Branch, opts.BaseBranch)
			result.Success = true
		}
	}

	return result, nil
}

// syncContinueMerge resumes a merge that was started but had conflicts.
func syncContinueMerge(git gitops.Client, log Logger, opts SyncOptions, result *SyncResult) (*SyncResult, error) {
	dirname := filepath.Base(opts.WtPath)
	log.Info("Merge in progress — continuing sync of '%s' with '%s'", opts.Branch, opts.BaseBranch)

	hasConflicts, err := git.HasConflicts(opts.WtPath)
	if err != nil {
		log.Verbose("Could not check conflict status: %v", err)
	}
	if hasConflicts {
		result.Conflict = true
		return result, fmt.Errorf("worktree '%s' has unresolved conflicts — resolve all conflicts and stage files, then run sync again", dirname)
	}

	if opts.DryRun {
		log.Info("Would run: git merge --continue")
		result.Success = true
	} else {
		if err := git.MergeContinue(opts.WtPath); err != nil {
			return result, fmt.Errorf("merge --continue failed: %w", err)
		}
		log.Success("Sync continued — '%s' synced with '%s'", opts.Branch, opts.BaseBranch)
		result.Success = true
	}
	return result, nil
}

// syncContinueRebase resumes a rebase that was started but had conflicts.
func syncContinueRebase(git gitops.Client, log Logger, opts SyncOptions, result *SyncResult) (*SyncResult, error) {
	dirname := filepath.Base(opts.WtPath)
	log.Info("Rebase in progress — continuing sync of '%s' with '%s'", opts.Branch, opts.BaseBranch)

	hasConflicts, err := git.HasConflicts(opts.WtPath)
	if err != nil {
		log.Verbose("Could not check conflict status: %v", err)
	}
	if hasConflicts {
		result.Conflict = true
		return result, fmt.Errorf("worktree '%s' has unresolved conflicts — resolve all conflicts and stage files, then run sync again (or 'git -C %s rebase --abort' to cancel)", dirname, opts.WtPath)
	}

	if opts.DryRun {
		log.Info("Would run: git rebase --continue")
		result.Success = true
	} else {
		if err := git.RebaseContinue(opts.WtPath); err != nil {
			return result, fmt.Errorf("rebase --continue failed: %w", err)
		}
		log.Success("Sync continued — '%s' rebased onto '%s'", opts.Branch, opts.BaseBranch)
		result.Success = true
	}
	return result, nil
}

// SyncAll synchronizes all worktrees with the base branch.
// It fetches once, then syncs each worktree, skipping dirty or in-progress ones.
func SyncAll(git gitops.Client, log Logger, opts SyncOptions) ([]SyncResult, error) {
	worktrees, err := git.WorktreeList(opts.RepoPath)
	if err != nil {
		return nil, err
	}

	// Filter out main repo entry
	type wtEntry struct {
		path   string
		branch string
	}
	var entries []wtEntry
	for _, wt := range worktrees {
		if wt.Path == opts.RepoPath {
			continue
		}
		entries = append(entries, wtEntry{path: wt.Path, branch: wt.Branch})
	}

	if len(entries) == 0 {
		log.Info("No worktrees to sync")
		return nil, nil
	}

	// Fetch once if remote exists
	mergeSource, _ := resolveMergeSource(git, log, opts.RepoPath, opts.BaseBranch, opts.DryRun)

	var results []SyncResult
	for _, entry := range entries {
		dirname := filepath.Base(entry.path)

		// Skip if dirty
		dirty, err := git.IsWorktreeDirty(entry.path)
		if err != nil {
			log.Warning("Could not check status of '%s': %v (skipping)", dirname, err)
			results = append(results, SyncResult{Branch: entry.branch, Skipped: true, SkipReason: "status check failed"})
			continue
		}
		if dirty && !opts.Force {
			log.Warning("Skipping '%s' — has uncommitted changes", dirname)
			results = append(results, SyncResult{Branch: entry.branch, Skipped: true, SkipReason: "uncommitted changes"})
			continue
		}

		// Skip if merge or rebase in progress
		mergeIP, err := git.IsMergeInProgress(entry.path)
		if err != nil {
			log.Verbose("Could not check merge status of '%s': %v", dirname, err)
		}
		if mergeIP {
			log.Warning("Skipping '%s' — merge in progress", dirname)
			results = append(results, SyncResult{Branch: entry.branch, Skipped: true, SkipReason: "merge in progress"})
			continue
		}

		rebaseIP, err := git.IsRebaseInProgress(entry.path)
		if err != nil {
			log.Verbose("Could not check rebase status of '%s': %v", dirname, err)
		}
		if rebaseIP {
			log.Warning("Skipping '%s' — rebase in progress", dirname)
			results = append(results, SyncResult{Branch: entry.branch, Skipped: true, SkipReason: "rebase in progress"})
			continue
		}

		// Resolve effective merge source
		effectiveSource, ahead, behind := resolveEffectiveMergeSource(git, log, entry.path, opts.BaseBranch, mergeSource)

		if behind == 0 {
			log.Info("'%s' is already in sync (%s)", entry.branch, FormatSyncStatus(ahead, behind))
			results = append(results, SyncResult{Branch: entry.branch, Ahead: ahead, Behind: behind, AlreadySynced: true, Success: true})
			continue
		}

		r := SyncResult{Branch: entry.branch, Ahead: ahead, Behind: behind, Strategy: opts.Strategy}

		if opts.Strategy == "rebase" {
			log.Info("'%s' %s — rebasing onto %s", entry.branch, FormatSyncStatus(ahead, behind), opts.BaseBranch)

			if opts.DryRun {
				log.Info("Would rebase '%s' onto '%s'", entry.branch, effectiveSource)
				r.Success = true
			} else {
				if err := git.Rebase(entry.path, effectiveSource); err != nil {
					log.Warning("Conflict rebasing '%s' — resolve and run sync", dirname)
					r.Conflict = true
				} else {
					log.Success("Rebased '%s'", entry.branch)
					r.Success = true
				}
			}
		} else {
			log.Info("'%s' %s — merging %d commit(s)", entry.branch, FormatSyncStatus(ahead, behind), behind)

			if opts.DryRun {
				log.Info("Would merge '%s' into '%s'", effectiveSource, entry.branch)
				r.Success = true
			} else {
				if err := git.Merge(entry.path, effectiveSource); err != nil {
					log.Warning("Conflict syncing '%s' — resolve and run sync", dirname)
					r.Conflict = true
				} else {
					log.Success("Synced '%s'", entry.branch)
					r.Success = true
				}
			}
		}
		results = append(results, r)
	}

	// Summary
	var synced, skipped, upToDate, conflicts int
	for _, r := range results {
		switch {
		case r.Skipped:
			skipped++
		case r.AlreadySynced:
			upToDate++
		case r.Conflict:
			conflicts++
		case r.Success:
			synced++
		}
	}
	log.Info("Sync complete: %d synced, %d up-to-date, %d skipped, %d conflicts", synced, upToDate, skipped, conflicts)

	return results, nil
}
