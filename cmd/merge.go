package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/wt/pkg/lifecycle"
	"github.com/joescharf/wt/pkg/ops"
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
	_ = mergeCmd.RegisterFlagCompletionFunc("base", completeBranchNames)
	rootCmd.AddCommand(mergeCmd)
}

func mergeRun(branch string) error {
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

	baseBranch := mergeBase
	if baseBranch == "" {
		baseBranch = viper.GetString("base_branch")
	}

	// Build cleanup callback using lifecycle manager
	cleanup := func(cleanupWtPath, cleanupBranch string) error {
		return lcMgr.Delete(lifecycle.DeleteOptions{
			RepoPath:     repoRoot,
			WtPath:       cleanupWtPath,
			Branch:       cleanupBranch,
			Force:        true,
			DeleteBranch: !mergePR, // delete branch for local merge, not for PR
			DryRun:       dryRun,
		})
	}

	result, err := ops.Merge(gitClient, opsLogger, ops.MergeOptions{
		RepoPath:  repoRoot,
		BaseBranch: baseBranch,
		Branch:    branchName,
		WtPath:    wtPath,
		Strategy:  resolveStrategy(mergeRebase, mergeMerge),
		Force:     mergeForce,
		DryRun:    dryRun,
		CreatePR:  mergePR,
		NoCleanup: mergeNoCleanup,
		PRTitle:   mergeTitle,
		PRBody:    mergeBody,
		PRDraft:   mergeDraft,
	}, cleanup, ghPRCreateFunc)
	if err != nil {
		return err
	}

	if result.PRCreated && result.PRURL != "" {
		_, _ = fmt.Fprintln(output.Out, result.PRURL)
	}

	_, _ = fmt.Fprintln(output.Out)
	return nil
}
