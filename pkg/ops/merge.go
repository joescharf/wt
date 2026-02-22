package ops

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/joescharf/wt/pkg/gitops"
)

// Merge merges a worktree branch into the base branch (local or PR).
// The caller must resolve WtPath and Branch before calling.
// For cleanup after local merge, provide a non-nil cleanup function.
// For PR creation, provide a non-nil prCreate function.
func Merge(git gitops.Client, log Logger, opts MergeOptions, cleanup CleanupFunc, prCreate PRCreateFunc) (*MergeResult, error) {
	result := &MergeResult{Branch: opts.Branch}
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

	// Check if there are commits to merge
	hasCommits, err := git.HasUnpushedCommits(opts.WtPath, opts.BaseBranch)
	if err != nil {
		log.Verbose("Could not check for commits: %v", err)
	}
	if !hasCommits {
		log.Info("No commits to merge for '%s'", opts.Branch)
		result.Success = true
		return result, nil
	}

	if opts.CreatePR {
		return mergePR(git, log, opts, result, prCreate)
	}
	return mergeLocal(git, log, opts, result, cleanup)
}

// mergeLocal performs a local merge of the feature branch into the base branch.
func mergeLocal(git gitops.Client, log Logger, opts MergeOptions, result *MergeResult, cleanup CleanupFunc) (*MergeResult, error) {
	// Check if a merge is already in progress in main repo
	mergeInProgress, err := git.IsMergeInProgress(opts.RepoPath)
	if err != nil {
		log.Verbose("Could not check merge status: %v", err)
	}
	if mergeInProgress {
		return mergeLocalContinue(git, log, opts, result, cleanup)
	}

	// Check if a rebase is in progress in the worktree (rebase-then-ff flow)
	rebaseInProgress, err := git.IsRebaseInProgress(opts.WtPath)
	if err != nil {
		log.Verbose("Could not check rebase status: %v", err)
	}
	if rebaseInProgress {
		return mergeLocalContinueRebase(git, log, opts, result, cleanup)
	}

	// Verify main repo is on the base branch
	currentBranch, err := git.CurrentBranch(opts.RepoPath)
	if err != nil {
		return result, err
	}
	if currentBranch != opts.BaseBranch {
		return result, fmt.Errorf("main repo is on '%s', expected '%s' — switch to '%s' first", currentBranch, opts.BaseBranch, opts.BaseBranch)
	}

	// Pull base branch if remote exists
	hasRemote, err := git.HasRemote(opts.RepoPath)
	if err != nil {
		log.Verbose("Could not check for remote: %v", err)
	}

	if hasRemote {
		if opts.DryRun {
			log.Info("Would pull '%s'", opts.BaseBranch)
		} else {
			log.Info("Pulling '%s'", opts.BaseBranch)
			if err := git.Pull(opts.RepoPath); err != nil {
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

		log.Info("Rebasing '%s' onto '%s'", opts.Branch, opts.BaseBranch)

		if opts.DryRun {
			log.Info("Would rebase '%s' onto '%s'", opts.Branch, rebaseTarget)
			log.Info("Would fast-forward merge '%s' into '%s'", opts.Branch, opts.BaseBranch)
		} else {
			if err := git.Rebase(opts.WtPath, rebaseTarget); err != nil {
				log.Warning("Rebase failed — resolve conflicts, then run merge again (or 'git -C %s rebase --abort' to cancel)", opts.WtPath)
				return result, fmt.Errorf("rebase conflict: %w", err)
			}
			log.Success("Rebased '%s' onto '%s'", opts.Branch, opts.BaseBranch)

			// Fast-forward merge into base
			log.Info("Fast-forward merging '%s' into '%s'", opts.Branch, opts.BaseBranch)
			if err := git.Merge(opts.RepoPath, opts.Branch); err != nil {
				return result, fmt.Errorf("fast-forward merge failed: %w", err)
			}
			log.Success("Merged '%s' into '%s'", opts.Branch, opts.BaseBranch)
		}
	} else {
		log.Info("Merging '%s' into '%s'", opts.Branch, opts.BaseBranch)

		if opts.DryRun {
			log.Info("Would merge '%s' into '%s'", opts.Branch, opts.BaseBranch)
		} else {
			if err := git.Merge(opts.RepoPath, opts.Branch); err != nil {
				log.Warning("Merge failed — resolve conflicts, then run merge again")
				return result, fmt.Errorf("merge conflict: %w", err)
			}
			log.Success("Merged '%s' into '%s'", opts.Branch, opts.BaseBranch)
		}
	}

	return mergeLocalFinish(git, log, opts, result, cleanup)
}

// mergeLocalContinue resumes a merge that was started but had conflicts.
func mergeLocalContinue(git gitops.Client, log Logger, opts MergeOptions, result *MergeResult, cleanup CleanupFunc) (*MergeResult, error) {
	log.Info("Merge in progress — continuing merge of '%s' into '%s'", opts.Branch, opts.BaseBranch)

	hasConflicts, err := git.HasConflicts(opts.RepoPath)
	if err != nil {
		log.Verbose("Could not check conflict status: %v", err)
	}
	if hasConflicts {
		return result, fmt.Errorf("main repo has unresolved conflicts — resolve all conflicts and stage files, then run merge again")
	}

	if opts.DryRun {
		log.Info("Would run: git merge --continue")
	} else {
		if err := git.MergeContinue(opts.RepoPath); err != nil {
			return result, fmt.Errorf("merge --continue failed: %w", err)
		}
		log.Success("Merge continued — '%s' merged into '%s'", opts.Branch, opts.BaseBranch)
	}

	return mergeLocalFinish(git, log, opts, result, cleanup)
}

// mergeLocalContinueRebase resumes a rebase-then-ff merge when the rebase had conflicts.
func mergeLocalContinueRebase(git gitops.Client, log Logger, opts MergeOptions, result *MergeResult, cleanup CleanupFunc) (*MergeResult, error) {
	dirname := filepath.Base(opts.WtPath)
	log.Info("Rebase in progress — continuing merge of '%s' into '%s'", opts.Branch, opts.BaseBranch)

	hasConflicts, err := git.HasConflicts(opts.WtPath)
	if err != nil {
		log.Verbose("Could not check conflict status: %v", err)
	}
	if hasConflicts {
		return result, fmt.Errorf("worktree has unresolved conflicts — resolve all conflicts and stage files, then run merge '%s' again (or 'git -C %s rebase --abort' to cancel)", dirname, opts.WtPath)
	}

	if opts.DryRun {
		log.Info("Would run: git rebase --continue")
		log.Info("Would fast-forward merge '%s' into '%s'", opts.Branch, opts.BaseBranch)
	} else {
		if err := git.RebaseContinue(opts.WtPath); err != nil {
			return result, fmt.Errorf("rebase --continue failed: %w", err)
		}
		log.Success("Rebase continued — '%s' rebased onto '%s'", opts.Branch, opts.BaseBranch)

		// Fast-forward merge into base
		log.Info("Fast-forward merging '%s' into '%s'", opts.Branch, opts.BaseBranch)
		if err := git.Merge(opts.RepoPath, opts.Branch); err != nil {
			return result, fmt.Errorf("fast-forward merge failed: %w", err)
		}
		log.Success("Merged '%s' into '%s'", opts.Branch, opts.BaseBranch)
	}

	return mergeLocalFinish(git, log, opts, result, cleanup)
}

// mergeLocalFinish handles push + cleanup after a successful local merge.
func mergeLocalFinish(git gitops.Client, log Logger, opts MergeOptions, result *MergeResult, cleanup CleanupFunc) (*MergeResult, error) {
	// Push base branch if remote exists
	hasRemote, err := git.HasRemote(opts.RepoPath)
	if err != nil {
		log.Verbose("Could not check for remote: %v", err)
	}

	if hasRemote {
		if opts.DryRun {
			log.Info("Would push '%s'", opts.BaseBranch)
		} else {
			log.Info("Pushing '%s'", opts.BaseBranch)
			if err := git.Push(opts.RepoPath, opts.BaseBranch, false); err != nil {
				log.Warning("Push failed: %v (merge succeeded locally)", err)
			} else {
				log.Success("Pushed '%s'", opts.BaseBranch)
			}
		}
	}

	// Cleanup unless --no-cleanup
	if !opts.NoCleanup && cleanup != nil {
		log.Info("Cleaning up worktree")
		if err := cleanup(opts.WtPath, opts.Branch); err != nil {
			log.Warning("Cleanup failed: %v (merge succeeded)", err)
		}
	}

	result.Success = true
	log.Success("Merge complete")
	return result, nil
}

// mergePR creates a pull request for the feature branch.
func mergePR(git gitops.Client, log Logger, opts MergeOptions, result *MergeResult, prCreate PRCreateFunc) (*MergeResult, error) {
	if opts.Strategy == "rebase" {
		log.Warning("--rebase is ignored for PR mode (merge strategy is configured on GitHub)")
	}

	log.Info("Creating PR for '%s' → '%s'", opts.Branch, opts.BaseBranch)

	// Push branch
	if opts.DryRun {
		log.Info("Would push branch '%s'", opts.Branch)
	} else {
		log.Info("Pushing branch '%s'", opts.Branch)
		if err := git.Push(opts.WtPath, opts.Branch, true); err != nil {
			return result, fmt.Errorf("push failed: %w", err)
		}
		log.Success("Pushed '%s'", opts.Branch)
	}

	// Build gh pr create args
	args := []string{"pr", "create", "--base", opts.BaseBranch, "--head", opts.Branch}
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
		log.Info("Would run: gh %s", strings.Join(args, " "))
	} else {
		if prCreate == nil {
			return result, fmt.Errorf("PR creation function not provided")
		}
		log.Info("Creating pull request")
		prOutput, err := prCreate(args)
		if err != nil {
			log.Warning("Branch was pushed — you can create the PR manually")
			return result, fmt.Errorf("gh pr create failed: %s: %w", prOutput, err)
		}
		log.Success("Pull request created")
		result.PRCreated = true
		result.PRURL = prOutput
	}

	result.Success = true
	log.Success("PR workflow complete")
	return result, nil
}
