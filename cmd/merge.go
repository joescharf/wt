package cmd

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/wt/internal/ui"
)

var (
	mergePR        bool
	mergeNoCleanup bool
	mergeBase      string
	mergeTitle     string
	mergeBody      string
	mergeDraft     bool
	mergeForce     bool
	mergeRebase    bool
	mergeMerge     bool
)

// ghPRCreateFunc is the function used to create a PR via gh CLI, replaceable in tests.
var ghPRCreateFunc = defaultGHPRCreate

func defaultGHPRCreate(args []string) (string, error) {
	out, err := exec.Command("gh", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

var mergeCmd = &cobra.Command{
	Use:               "merge [branch]",
	Aliases:           []string{"mg"},
	Short:             "Merge worktree branch into base branch or create PR",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		return mergeRun(args[0])
	},
}

func init() {
	mergeCmd.Flags().BoolVar(&mergePR, "pr", false, "Create PR instead of local merge")
	mergeCmd.Flags().BoolVar(&mergeNoCleanup, "no-cleanup", false, "Keep worktree after merge")
	mergeCmd.Flags().StringVar(&mergeBase, "base", "", "Target branch (default from config)")
	mergeCmd.Flags().StringVar(&mergeTitle, "title", "", "PR title (--pr only)")
	mergeCmd.Flags().StringVar(&mergeBody, "body", "", "PR body (--pr only, uses --fill if empty)")
	mergeCmd.Flags().BoolVar(&mergeDraft, "draft", false, "Create draft PR (--pr only)")
	mergeCmd.Flags().BoolVar(&mergeForce, "force", false, "Skip safety checks")
	mergeCmd.Flags().BoolVar(&mergeRebase, "rebase", false, "Use rebase-then-fast-forward instead of merge")
	mergeCmd.Flags().BoolVar(&mergeMerge, "merge", false, "Use merge (overrides config rebase default)")
	mergeCmd.RegisterFlagCompletionFunc("base", completeBranchNames)
	rootCmd.AddCommand(mergeCmd)
}

func mergeRun(branch string) error {
	// Resolve worktree
	wtPath, err := gitClient.ResolveWorktree(branch)
	if err != nil {
		return err
	}
	dirname := filepath.Base(wtPath)

	if !isDirectory(wtPath) {
		return fmt.Errorf("worktree not found: %s", wtPath)
	}

	// Get branch name from state or fall back to input
	branchName := branch
	ws, _ := stateMgr.GetWorktree(wtPath)
	if ws != nil && ws.Branch != "" {
		branchName = ws.Branch
	}

	baseBranch := mergeBase
	if baseBranch == "" {
		baseBranch = viper.GetString("base_branch")
	}

	// Safety check: dirty worktree
	if !mergeForce {
		dirty, err := gitClient.IsWorktreeDirty(wtPath)
		if err != nil {
			output.Warning("Could not check worktree status: %v", err)
		}
		if dirty {
			return fmt.Errorf("worktree '%s' has uncommitted changes (use --force to skip)", dirname)
		}
	}

	// Check if there are commits to merge
	hasCommits, err := gitClient.HasUnpushedCommits(wtPath, baseBranch)
	if err != nil {
		output.VerboseLog("Could not check for commits: %v", err)
	}
	if !hasCommits {
		output.Info("No commits to merge for '%s'", ui.Cyan(branchName))
		return nil
	}

	if mergePR {
		return mergePRRun(wtPath, branchName, baseBranch, dirname)
	}
	return mergeLocalRun(wtPath, branchName, baseBranch, dirname)
}

func mergeLocalRun(wtPath, branchName, baseBranch, dirname string) error {
	// Get repo root
	repoRoot, err := gitClient.RepoRoot()
	if err != nil {
		return err
	}

	strategy := resolveStrategy(mergeRebase, mergeMerge)

	// Check if a merge is already in progress in main repo
	mergeInProgress, err := gitClient.IsMergeInProgress(repoRoot)
	if err != nil {
		output.VerboseLog("Could not check merge status: %v", err)
	}
	if mergeInProgress {
		return mergeLocalContinue(repoRoot, wtPath, branchName, baseBranch)
	}

	// Check if a rebase is in progress in the worktree (rebase-then-ff flow)
	rebaseInProgress, err := gitClient.IsRebaseInProgress(wtPath)
	if err != nil {
		output.VerboseLog("Could not check rebase status: %v", err)
	}
	if rebaseInProgress {
		return mergeLocalContinueRebase(repoRoot, wtPath, branchName, baseBranch)
	}

	// Verify main repo is on the base branch
	currentBranch, err := gitClient.CurrentBranch(repoRoot)
	if err != nil {
		return err
	}
	if currentBranch != baseBranch {
		return fmt.Errorf("main repo is on '%s', expected '%s' — switch to '%s' first", currentBranch, baseBranch, baseBranch)
	}

	// Pull base branch if remote exists
	hasRemote, err := gitClient.HasRemote()
	if err != nil {
		output.VerboseLog("Could not check for remote: %v", err)
	}

	if hasRemote {
		if dryRun {
			output.DryRunMsg("Would pull '%s'", baseBranch)
		} else {
			output.Info("Pulling '%s'", baseBranch)
			if err := gitClient.Pull(repoRoot); err != nil {
				output.Warning("Pull failed: %v (continuing with merge)", err)
			}
		}
	}

	if strategy == "rebase" {
		// Rebase-then-fast-forward flow:
		// 1. Rebase feature branch onto base in the worktree
		// 2. Fast-forward merge base to rebased feature tip in main repo
		rebaseTarget := baseBranch
		if hasRemote {
			rebaseTarget = "origin/" + baseBranch
		}

		output.Info("Rebasing '%s' onto '%s'", ui.Cyan(branchName), ui.Cyan(baseBranch))

		if dryRun {
			output.DryRunMsg("Would rebase '%s' onto '%s'", branchName, rebaseTarget)
			output.DryRunMsg("Would fast-forward merge '%s' into '%s'", branchName, baseBranch)
		} else {
			if err := gitClient.Rebase(wtPath, rebaseTarget); err != nil {
				output.Error("Rebase failed — resolve conflicts, then run 'wt merge %s' again (or 'git -C %s rebase --abort' to cancel)", dirname, wtPath)
				output.Info("Worktree kept at: %s", wtPath)
				return fmt.Errorf("rebase conflict: %w", err)
			}
			output.Success("Rebased '%s' onto '%s'", branchName, baseBranch)

			// Fast-forward merge into base
			output.Info("Fast-forward merging '%s' into '%s'", ui.Cyan(branchName), ui.Cyan(baseBranch))
			if err := gitClient.Merge(repoRoot, branchName); err != nil {
				return fmt.Errorf("fast-forward merge failed: %w", err)
			}
			output.Success("Merged '%s' into '%s'", branchName, baseBranch)
		}
	} else {
		output.Info("Merging '%s' into '%s'", ui.Cyan(branchName), ui.Cyan(baseBranch))

		if dryRun {
			output.DryRunMsg("Would merge '%s' into '%s'", branchName, baseBranch)
		} else {
			output.Info("Merging branch '%s'", branchName)
			if err := gitClient.Merge(repoRoot, branchName); err != nil {
				output.Error("Merge failed — resolve conflicts, then run 'wt merge %s' again", dirname)
				output.Info("Worktree kept at: %s", wtPath)
				return fmt.Errorf("merge conflict: %w", err)
			}
			output.Success("Merged '%s' into '%s'", branchName, baseBranch)
		}
	}

	return mergeLocalFinish(repoRoot, wtPath, branchName, baseBranch)
}

// mergeLocalContinue resumes a merge that was started but had conflicts.
func mergeLocalContinue(repoRoot, wtPath, branchName, baseBranch string) error {
	output.Info("Merge in progress — continuing merge of '%s' into '%s'", ui.Cyan(branchName), ui.Cyan(baseBranch))

	// Check that all conflicts are resolved (no unmerged files)
	hasConflicts, err := gitClient.HasConflicts(repoRoot)
	if err != nil {
		output.VerboseLog("Could not check conflict status: %v", err)
	}
	if hasConflicts {
		return fmt.Errorf("main repo has unresolved conflicts — resolve all conflicts and stage files, then run 'wt merge' again")
	}

	if dryRun {
		output.DryRunMsg("Would run: git merge --continue")
	} else {
		if err := gitClient.MergeContinue(repoRoot); err != nil {
			return fmt.Errorf("merge --continue failed: %w", err)
		}
		output.Success("Merge continued — '%s' merged into '%s'", branchName, baseBranch)
	}

	return mergeLocalFinish(repoRoot, wtPath, branchName, baseBranch)
}

// mergeLocalContinueRebase resumes a rebase-then-ff merge when the rebase had conflicts.
func mergeLocalContinueRebase(repoRoot, wtPath, branchName, baseBranch string) error {
	output.Info("Rebase in progress — continuing merge of '%s' into '%s'", ui.Cyan(branchName), ui.Cyan(baseBranch))

	// Check that all conflicts are resolved
	hasConflicts, err := gitClient.HasConflicts(wtPath)
	if err != nil {
		output.VerboseLog("Could not check conflict status: %v", err)
	}
	if hasConflicts {
		return fmt.Errorf("worktree has unresolved conflicts — resolve all conflicts and stage files, then run 'wt merge %s' again (or 'git -C %s rebase --abort' to cancel)", filepath.Base(wtPath), wtPath)
	}

	if dryRun {
		output.DryRunMsg("Would run: git rebase --continue")
		output.DryRunMsg("Would fast-forward merge '%s' into '%s'", branchName, baseBranch)
	} else {
		if err := gitClient.RebaseContinue(wtPath); err != nil {
			return fmt.Errorf("rebase --continue failed: %w", err)
		}
		output.Success("Rebase continued — '%s' rebased onto '%s'", branchName, baseBranch)

		// Fast-forward merge into base
		output.Info("Fast-forward merging '%s' into '%s'", ui.Cyan(branchName), ui.Cyan(baseBranch))
		if err := gitClient.Merge(repoRoot, branchName); err != nil {
			return fmt.Errorf("fast-forward merge failed: %w", err)
		}
		output.Success("Merged '%s' into '%s'", branchName, baseBranch)
	}

	return mergeLocalFinish(repoRoot, wtPath, branchName, baseBranch)
}

// mergeLocalFinish handles push + cleanup after a successful merge.
func mergeLocalFinish(repoRoot, wtPath, branchName, baseBranch string) error {
	// Push base branch if remote exists
	hasRemote, err := gitClient.HasRemote()
	if err != nil {
		output.VerboseLog("Could not check for remote: %v", err)
	}

	if hasRemote {
		if dryRun {
			output.DryRunMsg("Would push '%s'", baseBranch)
		} else {
			output.Info("Pushing '%s'", baseBranch)
			if err := gitClient.Push(repoRoot, baseBranch, false); err != nil {
				output.Warning("Push failed: %v (merge succeeded locally)", err)
			} else {
				output.Success("Pushed '%s'", baseBranch)
			}
		}
	}

	// Cleanup unless --no-cleanup
	if !mergeNoCleanup {
		fmt.Fprintln(output.Out)
		output.Info("Cleaning up worktree")
		if err := cleanupWorktree(wtPath, branchName, true, true); err != nil {
			output.Warning("Cleanup failed: %v (merge succeeded)", err)
		}
	}

	fmt.Fprintln(output.Out)
	output.Success("Merge complete")
	return nil
}

func mergePRRun(wtPath, branchName, baseBranch, dirname string) error {
	if mergeRebase {
		output.Warning("--rebase is ignored for PR mode (merge strategy is configured on GitHub)")
	}

	output.Info("Creating PR for '%s' → '%s'", ui.Cyan(branchName), ui.Cyan(baseBranch))

	// Verify gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found — install from https://cli.github.com")
	}

	// Push branch
	if dryRun {
		output.DryRunMsg("Would push branch '%s'", branchName)
	} else {
		output.Info("Pushing branch '%s'", branchName)
		if err := gitClient.Push(wtPath, branchName, true); err != nil {
			return fmt.Errorf("push failed: %w", err)
		}
		output.Success("Pushed '%s'", branchName)
	}

	// Build gh pr create args
	args := []string{"pr", "create", "--base", baseBranch, "--head", branchName}
	if mergeTitle != "" {
		args = append(args, "--title", mergeTitle)
	}
	if mergeBody != "" {
		args = append(args, "--body", mergeBody)
	}
	if mergeTitle == "" && mergeBody == "" {
		args = append(args, "--fill")
	}
	if mergeDraft {
		args = append(args, "--draft")
	}

	// Create PR
	if dryRun {
		output.DryRunMsg("Would run: gh %s", strings.Join(args, " "))
	} else {
		output.Info("Creating pull request")
		prOutput, err := ghPRCreateFunc(args)
		if err != nil {
			output.Warning("Branch was pushed — you can create the PR manually")
			return fmt.Errorf("gh pr create failed: %s: %w", prOutput, err)
		}
		output.Success("Pull request created")
		fmt.Fprintln(output.Out, prOutput)
	}

	// Cleanup only if explicitly NOT --no-cleanup (default: no cleanup for PR)
	if !mergeNoCleanup && mergeForce {
		// Only clean up PR worktrees if --force is also set (intentional)
		fmt.Fprintln(output.Out)
		output.Info("Cleaning up worktree")
		if err := cleanupWorktree(wtPath, branchName, true, false); err != nil {
			output.Warning("Cleanup failed: %v", err)
		}
	}

	fmt.Fprintln(output.Out)
	output.Success("PR workflow complete")
	return nil
}
