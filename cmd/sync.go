package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/wt/internal/ui"
)

var (
	syncBase   string
	syncForce  bool
	syncAll    bool
	syncRebase bool
	syncMerge  bool
)

var syncCmd = &cobra.Command{
	Use:               "sync [branch]",
	Aliases:           []string{"sy"},
	Short:             "Sync worktree with base branch (merge base into feature)",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if syncAll {
			return syncAllRun()
		}
		if len(args) == 0 {
			return fmt.Errorf("branch name required (or use --all)")
		}
		return syncRun(args[0])
	},
}

func init() {
	syncCmd.Flags().StringVar(&syncBase, "base", "", "Base branch to sync from (default from config)")
	syncCmd.Flags().BoolVar(&syncForce, "force", false, "Skip dirty worktree safety check")
	syncCmd.Flags().BoolVar(&syncAll, "all", false, "Sync all worktrees")
	syncCmd.Flags().BoolVar(&syncRebase, "rebase", false, "Use rebase instead of merge")
	syncCmd.Flags().BoolVar(&syncMerge, "merge", false, "Use merge (overrides config rebase default)")
	syncCmd.RegisterFlagCompletionFunc("base", completeBranchNames)
	rootCmd.AddCommand(syncCmd)
}

func syncRun(branch string) error {
	// Resolve worktree
	wtPath, err := gitClient.ResolveWorktree(branch)
	if err != nil {
		return err
	}
	dirname := filepath.Base(wtPath)

	// Get branch name from state or fall back to input
	branchName := branch
	ws, _ := stateMgr.GetWorktree(wtPath)
	if ws != nil && ws.Branch != "" {
		branchName = ws.Branch
	}

	baseBranch := syncBase
	if baseBranch == "" {
		baseBranch = viper.GetString("base_branch")
	}

	// Safety check: dirty worktree
	if !syncForce {
		dirty, err := gitClient.IsWorktreeDirty(wtPath)
		if err != nil {
			output.Warning("Could not check worktree status: %v", err)
		}
		if dirty {
			return fmt.Errorf("worktree '%s' has uncommitted changes (use --force to skip)", dirname)
		}
	}

	strategy := resolveStrategy(syncRebase, syncMerge)

	// Check if a merge or rebase is already in progress (idempotent — pick up where we left off)
	mergeInProgress, err := gitClient.IsMergeInProgress(wtPath)
	if err != nil {
		output.VerboseLog("Could not check merge status: %v", err)
	}
	if mergeInProgress {
		return syncContinue(wtPath, branchName, baseBranch)
	}

	rebaseInProgress, err := gitClient.IsRebaseInProgress(wtPath)
	if err != nil {
		output.VerboseLog("Could not check rebase status: %v", err)
	}
	if rebaseInProgress {
		return syncContinueRebase(wtPath, branchName, baseBranch)
	}

	// Determine merge source based on remote availability
	hasRemote, err := gitClient.HasRemote()
	if err != nil {
		output.VerboseLog("Could not check for remote: %v", err)
	}

	mergeSource := baseBranch
	if hasRemote {
		// Fetch to get latest changes
		repoRoot, err := gitClient.RepoRoot()
		if err != nil {
			return err
		}
		if dryRun {
			output.DryRunMsg("Would fetch from remote")
		} else {
			output.Info("Fetching latest changes")
			if err := gitClient.Fetch(repoRoot); err != nil {
				output.Warning("Fetch failed: %v (continuing with local state)", err)
			}
		}
		mergeSource = "origin/" + baseBranch
	}

	// Report current status
	ahead, err := gitClient.CommitsAhead(wtPath, mergeSource)
	if err != nil {
		output.VerboseLog("Could not check ahead status: %v", err)
	}
	behind, err := gitClient.CommitsBehind(wtPath, mergeSource)
	if err != nil {
		output.VerboseLog("Could not check behind status: %v", err)
	}

	// Also check local base branch — catches unpushed commits on base
	if mergeSource != baseBranch {
		localBehind, err := gitClient.CommitsBehind(wtPath, baseBranch)
		if err != nil {
			output.VerboseLog("Could not check local behind status: %v", err)
		}
		if localBehind > behind {
			behind = localBehind
			mergeSource = baseBranch
			ahead, _ = gitClient.CommitsAhead(wtPath, mergeSource)
		}
	}

	output.Info("Status of '%s' vs '%s': %s", ui.Cyan(branchName), ui.Cyan(baseBranch), formatSyncStatus(ahead, behind))

	if behind == 0 {
		output.Success("'%s' is already in sync with '%s'", branchName, baseBranch)
		return nil
	}

	if strategy == "rebase" {
		output.Info("Rebasing '%s' onto '%s' (%d commit(s) behind)", ui.Cyan(branchName), ui.Cyan(baseBranch), behind)

		if dryRun {
			output.DryRunMsg("Would rebase '%s' onto '%s'", branchName, mergeSource)
		} else {
			if err := gitClient.Rebase(wtPath, mergeSource); err != nil {
				output.Error("Rebase failed — resolve conflicts, then run 'wt sync %s' again (or 'git -C %s rebase --abort' to cancel)", dirname, wtPath)
				return fmt.Errorf("rebase conflict: %w", err)
			}
			output.Success("Rebased '%s' onto '%s'", branchName, baseBranch)
		}
	} else {
		output.Info("Merging %d commit(s) from '%s' into '%s'", behind, ui.Cyan(baseBranch), ui.Cyan(branchName))

		if dryRun {
			output.DryRunMsg("Would merge '%s' into '%s'", mergeSource, branchName)
		} else {
			if err := gitClient.Merge(wtPath, mergeSource); err != nil {
				output.Error("Merge failed — resolve conflicts, then run 'wt sync %s' again", dirname)
				return fmt.Errorf("merge conflict: %w", err)
			}
			output.Success("Synced '%s' with '%s'", branchName, baseBranch)
		}
	}

	return nil
}

func formatSyncStatus(ahead, behind int) string {
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

func syncContinue(wtPath, branchName, baseBranch string) error {
	dirname := filepath.Base(wtPath)
	output.Info("Merge in progress — continuing sync of '%s' with '%s'", ui.Cyan(branchName), ui.Cyan(baseBranch))

	hasConflicts, err := gitClient.HasConflicts(wtPath)
	if err != nil {
		output.VerboseLog("Could not check conflict status: %v", err)
	}
	if hasConflicts {
		return fmt.Errorf("worktree '%s' has unresolved conflicts — resolve all conflicts and stage files, then run 'wt sync %s' again", dirname, dirname)
	}

	if dryRun {
		output.DryRunMsg("Would run: git merge --continue")
	} else {
		if err := gitClient.MergeContinue(wtPath); err != nil {
			return fmt.Errorf("merge --continue failed: %w", err)
		}
		output.Success("Sync continued — '%s' synced with '%s'", branchName, baseBranch)
	}

	return nil
}

func syncContinueRebase(wtPath, branchName, baseBranch string) error {
	dirname := filepath.Base(wtPath)
	output.Info("Rebase in progress — continuing sync of '%s' with '%s'", ui.Cyan(branchName), ui.Cyan(baseBranch))

	hasConflicts, err := gitClient.HasConflicts(wtPath)
	if err != nil {
		output.VerboseLog("Could not check conflict status: %v", err)
	}
	if hasConflicts {
		return fmt.Errorf("worktree '%s' has unresolved conflicts — resolve all conflicts and stage files, then run 'wt sync %s' again (or 'git -C %s rebase --abort' to cancel)", dirname, dirname, wtPath)
	}

	if dryRun {
		output.DryRunMsg("Would run: git rebase --continue")
	} else {
		if err := gitClient.RebaseContinue(wtPath); err != nil {
			return fmt.Errorf("rebase --continue failed: %w", err)
		}
		output.Success("Sync continued — '%s' rebased onto '%s'", branchName, baseBranch)
	}

	return nil
}

func syncAllRun() error {
	worktrees, err := gitClient.WorktreeList()
	if err != nil {
		return err
	}

	repoRoot, err := gitClient.RepoRoot()
	if err != nil {
		return err
	}

	baseBranch := syncBase
	if baseBranch == "" {
		baseBranch = viper.GetString("base_branch")
	}

	strategy := resolveStrategy(syncRebase, syncMerge)

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
		output.Info("No worktrees to sync")
		return nil
	}

	// Fetch once if remote exists
	hasRemote, err := gitClient.HasRemote()
	if err != nil {
		output.VerboseLog("Could not check for remote: %v", err)
	}

	mergeSource := baseBranch
	if hasRemote {
		if dryRun {
			output.DryRunMsg("Would fetch from remote")
		} else {
			output.Info("Fetching latest changes")
			if err := gitClient.Fetch(repoRoot); err != nil {
				output.Warning("Fetch failed: %v (continuing with local state)", err)
			}
		}
		mergeSource = "origin/" + baseBranch
	}

	var synced, skipped, upToDate, conflicts int
	for _, entry := range entries {
		dirname := filepath.Base(entry.path)

		// Skip if dirty
		dirty, err := gitClient.IsWorktreeDirty(entry.path)
		if err != nil {
			output.Warning("Could not check status of '%s': %v (skipping)", dirname, err)
			skipped++
			continue
		}
		if dirty && !syncForce {
			output.Warning("Skipping '%s' — has uncommitted changes", dirname)
			skipped++
			continue
		}

		// Skip if merge or rebase in progress
		mergeIP, err := gitClient.IsMergeInProgress(entry.path)
		if err != nil {
			output.VerboseLog("Could not check merge status of '%s': %v", dirname, err)
		}
		if mergeIP {
			output.Warning("Skipping '%s' — merge in progress", dirname)
			skipped++
			continue
		}

		rebaseIP, err := gitClient.IsRebaseInProgress(entry.path)
		if err != nil {
			output.VerboseLog("Could not check rebase status of '%s': %v", dirname, err)
		}
		if rebaseIP {
			output.Warning("Skipping '%s' — rebase in progress", dirname)
			skipped++
			continue
		}

		// Check ahead/behind status
		ahead, _ := gitClient.CommitsAhead(entry.path, mergeSource)
		behind, _ := gitClient.CommitsBehind(entry.path, mergeSource)

		// Also check local base branch — catches unpushed commits on base
		effectiveMergeSource := mergeSource
		if mergeSource != baseBranch {
			localBehind, _ := gitClient.CommitsBehind(entry.path, baseBranch)
			if localBehind > behind {
				behind = localBehind
				effectiveMergeSource = baseBranch
				ahead, _ = gitClient.CommitsAhead(entry.path, baseBranch)
			}
		}

		if behind == 0 {
			output.Info("'%s' is already in sync (%s)", entry.branch, formatSyncStatus(ahead, behind))
			upToDate++
			continue
		}

		if strategy == "rebase" {
			output.Info("'%s' %s — rebasing onto %s", entry.branch, formatSyncStatus(ahead, behind), baseBranch)

			if dryRun {
				output.DryRunMsg("Would rebase '%s' onto '%s'", entry.branch, effectiveMergeSource)
				synced++
				continue
			}

			if err := gitClient.Rebase(entry.path, effectiveMergeSource); err != nil {
				output.Error("Conflict rebasing '%s' — resolve and run 'wt sync %s'", dirname, dirname)
				conflicts++
				continue
			}
			output.Success("Rebased '%s'", entry.branch)
			synced++
		} else {
			output.Info("'%s' %s — merging %d commit(s)", entry.branch, formatSyncStatus(ahead, behind), behind)

			if dryRun {
				output.DryRunMsg("Would merge '%s' into '%s'", effectiveMergeSource, entry.branch)
				synced++
				continue
			}

			if err := gitClient.Merge(entry.path, effectiveMergeSource); err != nil {
				output.Error("Conflict syncing '%s' — resolve and run 'wt sync %s'", dirname, dirname)
				conflicts++
				continue
			}
			output.Success("Synced '%s'", entry.branch)
			synced++
		}
	}

	fmt.Fprintln(output.Out)
	output.Info("Sync complete: %d synced, %d up-to-date, %d skipped, %d conflicts", synced, upToDate, skipped, conflicts)
	return nil
}
