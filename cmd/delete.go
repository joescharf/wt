package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/wt/internal/ui"
	"github.com/joescharf/wt/pkg/lifecycle"
	"github.com/joescharf/wt/pkg/ops"
)

var (
	deleteForce      bool
	deleteBranchFlag bool
	deleteAll        bool
)

// promptFunc is the confirmation prompt (default No), replaceable in tests.
var promptFunc = defaultPrompt

// promptDefaultYes is a confirmation prompt that defaults to Yes.
var promptDefaultYes = defaultPromptYes

func defaultPrompt(msg string) bool {
	fmt.Fprintf(output.ErrOut, "%s [y/N] ", msg)
	var answer string
	fmt.Fscanln(os.Stdin, &answer)
	return strings.ToLower(strings.TrimSpace(answer)) == "y"
}

func defaultPromptYes(msg string) bool {
	fmt.Fprintf(output.ErrOut, "%s [Y/n] ", msg)
	var answer string
	fmt.Fscanln(os.Stdin, &answer)
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "" || answer == "y"
}

var deleteCmd = &cobra.Command{
	Use:               "delete [branch]",
	Aliases:           []string{"rm"},
	Short:             "Close iTerm2 window + remove worktree",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if deleteAll {
			return deleteAllRun()
		}
		if len(args) == 0 {
			return fmt.Errorf("branch argument required (or use --all)")
		}
		return deleteRun(args[0])
	},
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Force removal, skip safety checks")
	deleteCmd.Flags().BoolVar(&deleteBranchFlag, "delete-branch", false, "Also delete the git branch")
	deleteCmd.Flags().BoolVar(&deleteAll, "all", false, "Delete all worktrees")
	rootCmd.AddCommand(deleteCmd)
}

// checkWorktreeSafety returns true if the worktree is safe to delete (no data loss risk).
// If unsafe, it prints warnings and prompts the user. Returns false if the user declines.
func checkWorktreeSafety(wtPath, dirname string) bool {
	dirty, err := gitClient.IsWorktreeDirty(wtPath)
	if err != nil {
		output.VerboseLog("Could not check worktree status: %v", err)
	}

	if dirty {
		output.Warning("'%s' has uncommitted changes", dirname)
		if dryRun {
			output.DryRunMsg("Would prompt for confirmation (uncommitted changes)")
			return true
		}
		return promptFunc(fmt.Sprintf("Delete '%s' with uncommitted changes?", dirname))
	}

	baseBranch := viper.GetString("base_branch")
	unpushed, err := gitClient.HasUnpushedCommits(wtPath, baseBranch)
	if err != nil {
		output.VerboseLog("Could not check unpushed commits: %v", err)
	}

	if unpushed {
		output.Warning("'%s' has unpushed commits", dirname)
		if dryRun {
			output.DryRunMsg("Would prompt for confirmation (unpushed commits)")
			return true
		}
		return promptFunc(fmt.Sprintf("Delete '%s' with unpushed commits?", dirname))
	}

	return true
}

func deleteRun(branch string) error {
	wtPath, err := gitClient.ResolveWorktree(repoRoot, branch)
	if err != nil {
		return err
	}
	dirname := filepath.Base(wtPath)

	// Build safety check callback
	safetyCheck := func(checkPath string) (bool, error) {
		checkDirname := filepath.Base(checkPath)
		if !checkWorktreeSafety(checkPath, checkDirname) {
			return false, nil
		}
		return true, nil
	}

	// Build cleanup callback using lifecycle manager
	cleanup := func(cleanupWtPath, cleanupBranch string) error {
		return lcMgr.Delete(lifecycle.DeleteOptions{
			RepoPath:     repoRoot,
			WtPath:       cleanupWtPath,
			Branch:       cleanupBranch,
			Force:        deleteForce,
			DeleteBranch: deleteBranchFlag,
			DryRun:       dryRun,
		})
	}

	output.Info("Deleting worktree '%s'", ui.Cyan(dirname))

	if err := ops.Delete(gitClient, opsLogger, ops.DeleteOptions{
		RepoPath:     repoRoot,
		WtPath:       wtPath,
		Branch:       branch,
		Force:        deleteForce,
		DeleteBranch: deleteBranchFlag,
		DryRun:       dryRun,
	}, safetyCheck, cleanup); err != nil {
		return err
	}

	fmt.Fprintln(output.Out)
	return nil
}

func deleteAllRun() error {
	// Build safety check callback
	safetyCheck := func(checkPath string) (bool, error) {
		checkDirname := filepath.Base(checkPath)
		if !checkWorktreeSafety(checkPath, checkDirname) {
			return false, nil
		}
		return true, nil
	}

	// Build cleanup callback using lifecycle manager
	cleanup := func(cleanupWtPath, cleanupBranch string) error {
		return lcMgr.Delete(lifecycle.DeleteOptions{
			RepoPath:     repoRoot,
			WtPath:       cleanupWtPath,
			Branch:       cleanupBranch,
			Force:        deleteForce,
			DeleteBranch: deleteBranchFlag,
			DryRun:       dryRun,
		})
	}

	deleted, err := ops.DeleteAll(gitClient, opsLogger, ops.DeleteOptions{
		RepoPath:     repoRoot,
		Force:        deleteForce,
		DeleteBranch: deleteBranchFlag,
		DryRun:       dryRun,
	}, safetyCheck, cleanup)
	if err != nil {
		return err
	}

	if deleted > 0 {
		fmt.Fprintln(output.Out)
	}
	return nil
}
