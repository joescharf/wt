package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/wt/pkg/ops"
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
	_ = syncCmd.RegisterFlagCompletionFunc("base", completeBranchNames)
	rootCmd.AddCommand(syncCmd)
}

func syncRun(branch string) error {
	// Resolve worktree
	wtPath, err := gitClient.ResolveWorktree(repoRoot, branch)
	if err != nil {
		return err
	}

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

	_, err = ops.Sync(gitClient, opsLogger, ops.SyncOptions{
		RepoPath:   repoRoot,
		BaseBranch: baseBranch,
		Branch:     branchName,
		WtPath:     wtPath,
		Strategy:   resolveStrategy(syncRebase, syncMerge),
		Force:      syncForce,
		DryRun:     dryRun,
	})
	return err
}

func syncAllRun() error {
	baseBranch := syncBase
	if baseBranch == "" {
		baseBranch = viper.GetString("base_branch")
	}

	results, err := ops.SyncAll(gitClient, opsLogger, ops.SyncOptions{
		RepoPath:   repoRoot,
		BaseBranch: baseBranch,
		Strategy:   resolveStrategy(syncRebase, syncMerge),
		Force:      syncForce,
		DryRun:     dryRun,
	})
	if err != nil {
		return err
	}

	// Print blank line before summary if there were results
	if len(results) > 0 {
		_, _ = fmt.Fprintln(output.Out)
	}
	return nil
}
