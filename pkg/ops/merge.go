package ops

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/joescharf/wt/pkg/gitops"
	"github.com/joescharf/wt/pkg/wtstate"
)

// Merge merges a worktree's feature branch into the base branch.
// For local merge: merges into the base branch in the main repo.
// For PR merge: pushes the branch and calls prCreateFunc to create a PR.
func Merge(ctx context.Context, git gitops.Client, state *wtstate.Manager,
	log Logger, wtPath string, opts MergeOptions, prCreateFunc PRCreateFunc) (*MergeResult, error) {

	dirname := filepath.Base(wtPath)

	result := &MergeResult{
		WorktreePath: wtPath,
		BaseBranch:   opts.BaseBranch,
		Strategy:     opts.Strategy,
		DryRun:       opts.DryRun,
	}

	// Get branch name: try state, then git, fall back to dirname
	branchName := dirname
	if state != nil {
		ws, _ := state.GetWorktree(wtPath)
		if ws != nil && ws.Branch != "" {
			branchName = ws.Branch
		}
	}
	if branchName == dirname {
		if cb, err := git.CurrentBranch(wtPath); err == nil && cb != "" {
			branchName = cb
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

	// Check if there are commits to merge
	hasCommits, err := git.HasUnpushedCommits(wtPath, opts.BaseBranch)
	if err != nil {
		log.Verbose("Could not check for commits: %v", err)
	}
	if !hasCommits {
		log.Info("No commits to merge for '%s'", branchName)
		result.Success = true
		return result, nil
	}

	if opts.CreatePR {
		return mergePR(ctx, git, log, wtPath, branchName, opts, prCreateFunc)
	}
	return mergeLocal(ctx, git, state, log, wtPath, branchName, opts)
}

// mergeLocal performs a local merge of the feature branch into the base branch.
func mergeLocal(ctx context.Context, git gitops.Client, state *wtstate.Manager,
	log Logger, wtPath, branchName string, opts MergeOptions) (*MergeResult, error) {

	result := &MergeResult{
		Branch:       branchName,
		BaseBranch:   opts.BaseBranch,
		WorktreePath: wtPath,
		Strategy:     opts.Strategy,
		DryRun:       opts.DryRun,
	}

	repoRoot, err := git.RepoRoot()
	if err != nil {
		result.Error = err
		return result, err
	}

	// Check if a merge is already in progress in main repo
	mergeInProgress, err := git.IsMergeInProgress(repoRoot)
	if err != nil {
		log.Verbose("Could not check merge status: %v", err)
	}
	if mergeInProgress {
		return mergeLocalContinue(ctx, git, log, repoRoot, wtPath, branchName, opts)
	}

	// Check if a rebase is in progress in the worktree
	rebaseInProgress, err := git.IsRebaseInProgress(wtPath)
	if err != nil {
		log.Verbose("Could not check rebase status: %v", err)
	}
	if rebaseInProgress {
		return mergeLocalContinueRebase(ctx, git, log, repoRoot, wtPath, branchName, opts)
	}

	// Verify main repo is on the base branch
	currentBranch, err := git.CurrentBranch(repoRoot)
	if err != nil {
		result.Error = err
		return result, err
	}
	if currentBranch != opts.BaseBranch {
		result.Error = fmt.Errorf("main repo is on '%s', expected '%s'", currentBranch, opts.BaseBranch)
		return result, result.Error
	}

	// Pull base branch if remote exists
	hasRemote, err := git.HasRemote()
	if err != nil {
		log.Verbose("Could not check for remote: %v", err)
	}

	if hasRemote {
		if opts.DryRun {
			log.Info("Would pull '%s'", opts.BaseBranch)
		} else {
			log.Info("Pulling '%s'", opts.BaseBranch)
			if err := git.Pull(repoRoot); err != nil {
				log.Warning("Pull failed: %v (continuing with merge)", err)
			}
		}
	}

	if opts.Strategy == "rebase" {
		// Rebase-then-fast-forward flow
		rebaseTarget := opts.BaseBranch
		if hasRemote {
			rebaseTarget = "origin/" + opts.BaseBranch
		}

		log.Info("Rebasing '%s' onto '%s'", branchName, opts.BaseBranch)

		if opts.DryRun {
			log.Info("Would rebase '%s' onto '%s'", branchName, rebaseTarget)
			log.Info("Would fast-forward merge '%s' into '%s'", branchName, opts.BaseBranch)
			result.Success = true
			return result, nil
		}

		if err := git.Rebase(wtPath, rebaseTarget); err != nil {
			result.HasConflicts = true
			result.ConflictFiles = getConflictFiles(git, wtPath)
			result.Error = fmt.Errorf("rebase conflict: %w", err)
			log.Error("Rebase failed — resolve conflicts, then merge again")
			return result, result.Error
		}
		log.Success("Rebased '%s' onto '%s'", branchName, opts.BaseBranch)

		// Fast-forward merge into base
		log.Info("Fast-forward merging '%s' into '%s'", branchName, opts.BaseBranch)
		if err := git.Merge(repoRoot, branchName); err != nil {
			result.Error = fmt.Errorf("fast-forward merge failed: %w", err)
			return result, result.Error
		}
		log.Success("Merged '%s' into '%s'", branchName, opts.BaseBranch)
	} else {
		log.Info("Merging '%s' into '%s'", branchName, opts.BaseBranch)

		if opts.DryRun {
			log.Info("Would merge '%s' into '%s'", branchName, opts.BaseBranch)
			result.Success = true
			return result, nil
		}

		if err := git.Merge(repoRoot, branchName); err != nil {
			result.HasConflicts = true
			result.ConflictFiles = getConflictFiles(git, repoRoot)
			result.Error = fmt.Errorf("merge conflict: %w", err)
			log.Error("Merge failed — resolve conflicts, then merge again")
			return result, result.Error
		}
		log.Success("Merged '%s' into '%s'", branchName, opts.BaseBranch)
	}

	// Push base branch if remote exists
	if hasRemote {
		if opts.DryRun {
			log.Info("Would push '%s'", opts.BaseBranch)
		} else {
			log.Info("Pushing '%s'", opts.BaseBranch)
			if err := git.Push(repoRoot, opts.BaseBranch, false); err != nil {
				log.Warning("Push failed: %v (merge succeeded locally)", err)
			} else {
				log.Success("Pushed '%s'", opts.BaseBranch)
			}
		}
	}

	result.Success = true
	return result, nil
}

// mergeLocalContinue resumes a merge in the main repo that had conflicts.
func mergeLocalContinue(ctx context.Context, git gitops.Client, log Logger,
	repoRoot, wtPath, branchName string, opts MergeOptions) (*MergeResult, error) {

	result := &MergeResult{
		Branch:       branchName,
		BaseBranch:   opts.BaseBranch,
		WorktreePath: wtPath,
		Strategy:     "merge",
		DryRun:       opts.DryRun,
	}

	log.Info("Merge in progress — continuing merge of '%s' into '%s'", branchName, opts.BaseBranch)

	hasConflicts, err := git.HasConflicts(repoRoot)
	if err != nil {
		log.Verbose("Could not check conflict status: %v", err)
	}
	if hasConflicts {
		result.HasConflicts = true
		result.ConflictFiles = getConflictFiles(git, repoRoot)
		result.Error = fmt.Errorf("main repo has unresolved conflicts")
		return result, result.Error
	}

	if opts.DryRun {
		log.Info("Would run: git merge --continue")
		result.Success = true
		return result, nil
	}

	if err := git.MergeContinue(repoRoot); err != nil {
		result.Error = fmt.Errorf("merge --continue failed: %w", err)
		return result, result.Error
	}

	log.Success("Merge continued — '%s' merged into '%s'", branchName, opts.BaseBranch)

	// Push after continue
	hasRemote, _ := git.HasRemote()
	if hasRemote {
		log.Info("Pushing '%s'", opts.BaseBranch)
		if err := git.Push(repoRoot, opts.BaseBranch, false); err != nil {
			log.Warning("Push failed: %v", err)
		}
	}

	result.Success = true
	return result, nil
}

// mergeLocalContinueRebase resumes a rebase-then-ff merge when the rebase had conflicts.
func mergeLocalContinueRebase(ctx context.Context, git gitops.Client, log Logger,
	repoRoot, wtPath, branchName string, opts MergeOptions) (*MergeResult, error) {

	result := &MergeResult{
		Branch:       branchName,
		BaseBranch:   opts.BaseBranch,
		WorktreePath: wtPath,
		Strategy:     "rebase",
		DryRun:       opts.DryRun,
	}

	log.Info("Rebase in progress — continuing merge of '%s' into '%s'", branchName, opts.BaseBranch)

	hasConflicts, err := git.HasConflicts(wtPath)
	if err != nil {
		log.Verbose("Could not check conflict status: %v", err)
	}
	if hasConflicts {
		result.HasConflicts = true
		result.ConflictFiles = getConflictFiles(git, wtPath)
		result.Error = fmt.Errorf("worktree has unresolved conflicts")
		return result, result.Error
	}

	if opts.DryRun {
		log.Info("Would run: git rebase --continue")
		log.Info("Would fast-forward merge '%s' into '%s'", branchName, opts.BaseBranch)
		result.Success = true
		return result, nil
	}

	if err := git.RebaseContinue(wtPath); err != nil {
		result.Error = fmt.Errorf("rebase --continue failed: %w", err)
		return result, result.Error
	}
	log.Success("Rebase continued — '%s' rebased onto '%s'", branchName, opts.BaseBranch)

	// Fast-forward merge into base
	log.Info("Fast-forward merging '%s' into '%s'", branchName, opts.BaseBranch)
	if err := git.Merge(repoRoot, branchName); err != nil {
		result.Error = fmt.Errorf("fast-forward merge failed: %w", err)
		return result, result.Error
	}
	log.Success("Merged '%s' into '%s'", branchName, opts.BaseBranch)

	// Push
	hasRemote, _ := git.HasRemote()
	if hasRemote {
		log.Info("Pushing '%s'", opts.BaseBranch)
		if err := git.Push(repoRoot, opts.BaseBranch, false); err != nil {
			log.Warning("Push failed: %v", err)
		}
	}

	result.Success = true
	return result, nil
}

// mergePR pushes the branch and creates a PR.
func mergePR(ctx context.Context, git gitops.Client, log Logger,
	wtPath, branchName string, opts MergeOptions, prCreateFunc PRCreateFunc) (*MergeResult, error) {

	result := &MergeResult{
		Branch:       branchName,
		BaseBranch:   opts.BaseBranch,
		WorktreePath: wtPath,
		Strategy:     opts.Strategy,
		DryRun:       opts.DryRun,
	}

	if opts.Strategy == "rebase" {
		log.Warning("rebase is ignored for PR mode (merge strategy is configured on GitHub)")
	}

	log.Info("Creating PR for '%s' → '%s'", branchName, opts.BaseBranch)

	// Push branch
	if opts.DryRun {
		log.Info("Would push branch '%s'", branchName)
	} else {
		log.Info("Pushing branch '%s'", branchName)
		if err := git.Push(wtPath, branchName, true); err != nil {
			result.Error = fmt.Errorf("push failed: %w", err)
			return result, result.Error
		}
		log.Success("Pushed '%s'", branchName)
	}

	// Build PR creation args
	args := []string{"pr", "create", "--base", opts.BaseBranch, "--head", branchName}
	if opts.PRTitle != "" {
		args = append(args, "--title", opts.PRTitle)
	}
	if opts.PRBody != "" {
		args = append(args, "--body", opts.PRBody)
	}
	if opts.PRTitle == "" && opts.PRBody == "" {
		args = append(args, "--fill")
	}
	if opts.PRDraft {
		args = append(args, "--draft")
	}

	// Create PR
	if opts.DryRun {
		log.Info("Would create PR")
		result.Success = true
		result.PRCreated = true
		return result, nil
	}

	if prCreateFunc == nil {
		result.Error = fmt.Errorf("PR creation function not provided")
		return result, result.Error
	}

	prOutput, err := prCreateFunc(args)
	if err != nil {
		log.Warning("Branch was pushed — you can create the PR manually")
		result.Error = fmt.Errorf("PR create failed: %s: %w", prOutput, err)
		return result, result.Error
	}

	result.Success = true
	result.PRCreated = true
	result.PRURL = prOutput
	log.Success("Pull request created: %s", prOutput)

	return result, nil
}
