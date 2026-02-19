package ops

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/joescharf/wt/pkg/gitops"
	"github.com/joescharf/wt/pkg/wtstate"
)

// Sync synchronizes a single worktree with its base branch.
// It fetches from remote (if available), checks ahead/behind status,
// and merges or rebases the base branch into the feature branch.
func Sync(ctx context.Context, git gitops.Client, state *wtstate.Manager,
	log Logger, wtPath string, opts SyncOptions) (*SyncResult, error) {

	dirname := filepath.Base(wtPath)

	result := &SyncResult{
		WorktreePath: wtPath,
		BaseBranch:   opts.BaseBranch,
		Strategy:     opts.Strategy,
		DryRun:       opts.DryRun,
	}

	// Get branch name from state or derive from path
	branchName := dirname
	if state != nil {
		ws, _ := state.GetWorktree(wtPath)
		if ws != nil && ws.Branch != "" {
			branchName = ws.Branch
		}
	}
	result.Branch = branchName

	// Safety check: dirty worktree
	if !opts.Force {
		dirty, err := git.IsWorktreeDirty(wtPath)
		if err != nil {
			log.Warning("Could not check worktree status: %v", err)
		}
		if dirty {
			result.Error = fmt.Errorf("worktree '%s' has uncommitted changes (use force to skip)", dirname)
			return result, result.Error
		}
	}

	// Check if a merge or rebase is already in progress
	mergeInProgress, err := git.IsMergeInProgress(wtPath)
	if err != nil {
		log.Verbose("Could not check merge status: %v", err)
	}
	if mergeInProgress {
		return syncContinueMerge(ctx, git, log, wtPath, branchName, opts)
	}

	rebaseInProgress, err := git.IsRebaseInProgress(wtPath)
	if err != nil {
		log.Verbose("Could not check rebase status: %v", err)
	}
	if rebaseInProgress {
		return syncContinueRebase(ctx, git, log, wtPath, branchName, opts)
	}

	// Determine merge source based on remote availability
	hasRemote, err := git.HasRemote()
	if err != nil {
		log.Verbose("Could not check for remote: %v", err)
	}

	mergeSource := opts.BaseBranch
	if hasRemote {
		repoRoot, err := git.RepoRoot()
		if err != nil {
			result.Error = err
			return result, err
		}
		if opts.DryRun {
			log.Info("Would fetch from remote")
		} else {
			log.Info("Fetching latest changes")
			if err := git.Fetch(repoRoot); err != nil {
				log.Warning("Fetch failed: %v (continuing with local state)", err)
			}
		}
		mergeSource = "origin/" + opts.BaseBranch
	}

	// Calculate ahead/behind
	ahead, err := git.CommitsAhead(wtPath, mergeSource)
	if err != nil {
		log.Verbose("Could not check ahead status: %v", err)
	}
	behind, err := git.CommitsBehind(wtPath, mergeSource)
	if err != nil {
		log.Verbose("Could not check behind status: %v", err)
	}

	// Also check local base branch — catches unpushed commits on base
	if mergeSource != opts.BaseBranch {
		localBehind, err := git.CommitsBehind(wtPath, opts.BaseBranch)
		if err != nil {
			log.Verbose("Could not check local behind status: %v", err)
		}
		if localBehind > behind {
			behind = localBehind
			mergeSource = opts.BaseBranch
			ahead, _ = git.CommitsAhead(wtPath, mergeSource)
		}
	}

	result.Ahead = ahead
	result.Behind = behind

	log.Info("Status of '%s' vs '%s': %s", branchName, opts.BaseBranch, FormatSyncStatus(ahead, behind))

	if behind == 0 {
		result.Success = true
		result.AlreadySynced = true
		log.Success("'%s' is already in sync with '%s'", branchName, opts.BaseBranch)
		return result, nil
	}

	if opts.Strategy == "rebase" {
		log.Info("Rebasing '%s' onto '%s' (%d commit(s) behind)", branchName, opts.BaseBranch, behind)

		if opts.DryRun {
			log.Info("Would rebase '%s' onto '%s'", branchName, mergeSource)
			result.Success = true
			return result, nil
		}

		if err := git.Rebase(wtPath, mergeSource); err != nil {
			result.HasConflicts = true
			result.ConflictFiles = getConflictFiles(git, wtPath)
			result.Error = fmt.Errorf("rebase conflict: %w", err)
			log.Error("Rebase failed — resolve conflicts, then sync again")
			return result, result.Error
		}
		result.Success = true
		log.Success("Rebased '%s' onto '%s'", branchName, opts.BaseBranch)
	} else {
		log.Info("Merging %d commit(s) from '%s' into '%s'", behind, opts.BaseBranch, branchName)

		if opts.DryRun {
			log.Info("Would merge '%s' into '%s'", mergeSource, branchName)
			result.Success = true
			return result, nil
		}

		if err := git.Merge(wtPath, mergeSource); err != nil {
			result.HasConflicts = true
			result.ConflictFiles = getConflictFiles(git, wtPath)
			result.Error = fmt.Errorf("merge conflict: %w", err)
			log.Error("Merge failed — resolve conflicts, then sync again")
			return result, result.Error
		}
		result.Success = true
		log.Success("Synced '%s' with '%s'", branchName, opts.BaseBranch)
	}

	return result, nil
}

// SyncAll synchronizes all worktrees with the base branch.
func SyncAll(ctx context.Context, git gitops.Client, state *wtstate.Manager,
	log Logger, opts SyncOptions) ([]SyncResult, error) {

	worktrees, err := git.WorktreeList()
	if err != nil {
		return nil, err
	}

	repoRoot, err := git.RepoRoot()
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
		if wt.Path == repoRoot {
			continue
		}
		entries = append(entries, wtEntry{path: wt.Path, branch: wt.Branch})
	}

	if len(entries) == 0 {
		log.Info("No worktrees to sync")
		return nil, nil
	}

	// Fetch once if remote exists
	hasRemote, err := git.HasRemote()
	if err != nil {
		log.Verbose("Could not check for remote: %v", err)
	}

	if hasRemote {
		if opts.DryRun {
			log.Info("Would fetch from remote")
		} else {
			log.Info("Fetching latest changes")
			if err := git.Fetch(repoRoot); err != nil {
				log.Warning("Fetch failed: %v (continuing with local state)", err)
			}
		}
	}

	mergeSource := opts.BaseBranch
	if hasRemote {
		mergeSource = "origin/" + opts.BaseBranch
	}

	var results []SyncResult
	for _, entry := range entries {
		result := SyncResult{
			Branch:       entry.branch,
			BaseBranch:   opts.BaseBranch,
			WorktreePath: entry.path,
			Strategy:     opts.Strategy,
			DryRun:       opts.DryRun,
		}
		dirname := filepath.Base(entry.path)

		// Skip if dirty
		dirty, err := git.IsWorktreeDirty(entry.path)
		if err != nil {
			log.Warning("Could not check status of '%s': %v (skipping)", dirname, err)
			result.Error = err
			results = append(results, result)
			continue
		}
		if dirty && !opts.Force {
			log.Warning("Skipping '%s' — has uncommitted changes", dirname)
			result.Error = fmt.Errorf("uncommitted changes")
			results = append(results, result)
			continue
		}

		// Skip if merge or rebase in progress
		mergeIP, _ := git.IsMergeInProgress(entry.path)
		if mergeIP {
			log.Warning("Skipping '%s' — merge in progress", dirname)
			result.HasConflicts = true
			result.Error = fmt.Errorf("merge in progress")
			results = append(results, result)
			continue
		}

		rebaseIP, _ := git.IsRebaseInProgress(entry.path)
		if rebaseIP {
			log.Warning("Skipping '%s' — rebase in progress", dirname)
			result.HasConflicts = true
			result.Error = fmt.Errorf("rebase in progress")
			results = append(results, result)
			continue
		}

		// Check ahead/behind status
		ahead, _ := git.CommitsAhead(entry.path, mergeSource)
		behind, _ := git.CommitsBehind(entry.path, mergeSource)

		// Also check local base branch
		effectiveMergeSource := mergeSource
		if mergeSource != opts.BaseBranch {
			localBehind, _ := git.CommitsBehind(entry.path, opts.BaseBranch)
			if localBehind > behind {
				behind = localBehind
				effectiveMergeSource = opts.BaseBranch
				ahead, _ = git.CommitsAhead(entry.path, opts.BaseBranch)
			}
		}

		result.Ahead = ahead
		result.Behind = behind

		if behind == 0 {
			log.Info("'%s' is already in sync (%s)", entry.branch, FormatSyncStatus(ahead, behind))
			result.Success = true
			result.AlreadySynced = true
			results = append(results, result)
			continue
		}

		if opts.Strategy == "rebase" {
			log.Info("'%s' %s — rebasing onto %s", entry.branch, FormatSyncStatus(ahead, behind), opts.BaseBranch)

			if opts.DryRun {
				log.Info("Would rebase '%s' onto '%s'", entry.branch, effectiveMergeSource)
				result.Success = true
				results = append(results, result)
				continue
			}

			if err := git.Rebase(entry.path, effectiveMergeSource); err != nil {
				log.Error("Conflict rebasing '%s'", dirname)
				result.HasConflicts = true
				result.ConflictFiles = getConflictFiles(git, entry.path)
				result.Error = err
				results = append(results, result)
				continue
			}
			log.Success("Rebased '%s'", entry.branch)
			result.Success = true
		} else {
			log.Info("'%s' %s — merging %d commit(s)", entry.branch, FormatSyncStatus(ahead, behind), behind)

			if opts.DryRun {
				log.Info("Would merge '%s' into '%s'", effectiveMergeSource, entry.branch)
				result.Success = true
				results = append(results, result)
				continue
			}

			if err := git.Merge(entry.path, effectiveMergeSource); err != nil {
				log.Error("Conflict syncing '%s'", dirname)
				result.HasConflicts = true
				result.ConflictFiles = getConflictFiles(git, entry.path)
				result.Error = err
				results = append(results, result)
				continue
			}
			log.Success("Synced '%s'", entry.branch)
			result.Success = true
		}

		results = append(results, result)
	}

	return results, nil
}

// syncContinueMerge resumes a merge that was started but had conflicts.
func syncContinueMerge(ctx context.Context, git gitops.Client, log Logger,
	wtPath, branchName string, opts SyncOptions) (*SyncResult, error) {

	dirname := filepath.Base(wtPath)
	result := &SyncResult{
		Branch:       branchName,
		BaseBranch:   opts.BaseBranch,
		WorktreePath: wtPath,
		Strategy:     "merge",
		DryRun:       opts.DryRun,
	}

	log.Info("Merge in progress — continuing sync of '%s' with '%s'", branchName, opts.BaseBranch)

	hasConflicts, err := git.HasConflicts(wtPath)
	if err != nil {
		log.Verbose("Could not check conflict status: %v", err)
	}
	if hasConflicts {
		result.HasConflicts = true
		result.ConflictFiles = getConflictFiles(git, wtPath)
		result.Error = fmt.Errorf("worktree '%s' has unresolved conflicts", dirname)
		return result, result.Error
	}

	if opts.DryRun {
		log.Info("Would run: git merge --continue")
		result.Success = true
		return result, nil
	}

	if err := git.MergeContinue(wtPath); err != nil {
		result.Error = fmt.Errorf("merge --continue failed: %w", err)
		return result, result.Error
	}

	result.Success = true
	log.Success("Sync continued — '%s' synced with '%s'", branchName, opts.BaseBranch)
	return result, nil
}

// syncContinueRebase resumes a rebase that was started but had conflicts.
func syncContinueRebase(ctx context.Context, git gitops.Client, log Logger,
	wtPath, branchName string, opts SyncOptions) (*SyncResult, error) {

	dirname := filepath.Base(wtPath)
	result := &SyncResult{
		Branch:       branchName,
		BaseBranch:   opts.BaseBranch,
		WorktreePath: wtPath,
		Strategy:     "rebase",
		DryRun:       opts.DryRun,
	}

	log.Info("Rebase in progress — continuing sync of '%s' with '%s'", branchName, opts.BaseBranch)

	hasConflicts, err := git.HasConflicts(wtPath)
	if err != nil {
		log.Verbose("Could not check conflict status: %v", err)
	}
	if hasConflicts {
		result.HasConflicts = true
		result.ConflictFiles = getConflictFiles(git, wtPath)
		result.Error = fmt.Errorf("worktree '%s' has unresolved conflicts", dirname)
		return result, result.Error
	}

	if opts.DryRun {
		log.Info("Would run: git rebase --continue")
		result.Success = true
		return result, nil
	}

	if err := git.RebaseContinue(wtPath); err != nil {
		result.Error = fmt.Errorf("rebase --continue failed: %w", err)
		return result, result.Error
	}

	result.Success = true
	log.Success("Sync continued — '%s' rebased onto '%s'", branchName, opts.BaseBranch)
	return result, nil
}

// FormatSyncStatus returns a human-readable string for ahead/behind counts.
func FormatSyncStatus(ahead, behind int) string {
	switch {
	case ahead > 0 && behind > 0:
		return fmt.Sprintf("↑%d ↓%d", ahead, behind)
	case ahead > 0:
		return fmt.Sprintf("↑%d", ahead)
	case behind > 0:
		return fmt.Sprintf("↓%d", behind)
	default:
		return "clean"
	}
}

// getConflictFiles returns a list of files with merge conflicts.
func getConflictFiles(git gitops.Client, wtPath string) []string {
	hasConflicts, _ := git.HasConflicts(wtPath)
	if !hasConflicts {
		return nil
	}
	// HasConflicts uses `git diff --name-only --diff-filter=U` but doesn't return the file list.
	// For now, return a placeholder. A future enhancement could add a ConflictFiles() method to Client.
	return []string{"(conflict files detected — check git status)"}
}

