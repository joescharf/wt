package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/worktree-dev/internal/git"
	"github.com/joescharf/worktree-dev/internal/state"
	"github.com/joescharf/worktree-dev/internal/ui"
)

var (
	createBase     string
	createNoClaude bool
	createExisting bool
)

var createCmd = &cobra.Command{
	Use:     "create <branch>",
	Aliases: []string{"new"},
	Short:   "Create worktree + branch + iTerm2 window",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return createRun(args[0])
	},
}

func init() {
	createCmd.Flags().StringVar(&createBase, "base", "", "Base branch (default from config)")
	createCmd.Flags().BoolVar(&createNoClaude, "no-claude", false, "Don't auto-launch claude in top pane")
	createCmd.Flags().BoolVar(&createExisting, "existing", false, "Use existing branch instead of creating new")
	createCmd.RegisterFlagCompletionFunc("base", completeBranchNames)
	rootCmd.AddCommand(createCmd)
}

func createRun(branch string) error {
	repoName, err := gitClient.RepoName()
	if err != nil {
		return err
	}

	wtDir, err := gitClient.WorktreesDir()
	if err != nil {
		return err
	}

	dirname := git.BranchToDirname(branch)
	wtPath := filepath.Join(wtDir, dirname)

	output.Info("Creating worktree for branch '%s' in repo '%s'", ui.Cyan(branch), ui.Cyan(repoName))
	output.VerboseLog("Worktrees dir: %s", wtDir)
	output.VerboseLog("Worktree path: %s", wtPath)

	baseBranch := createBase
	if baseBranch == "" {
		baseBranch = viper.GetString("base_branch")
	}
	output.VerboseLog("Base branch: %s", baseBranch)

	// If worktree already exists, delegate to open
	if isDirectory(wtPath) {
		output.Info("Worktree already exists, opening iTerm2 window")
		return openRun(branch)
	}

	// Create worktrees directory if needed
	if !isDirectory(wtDir) {
		if dryRun {
			output.DryRunMsg("Would create worktrees directory: %s", wtDir)
		} else {
			if err := os.MkdirAll(wtDir, 0755); err != nil {
				return fmt.Errorf("failed to create worktrees directory: %w", err)
			}
			output.Success("Created worktrees directory")
		}
	}

	// Check if branch already exists (auto-detect)
	branchExists, err := gitClient.BranchExists(branch)
	if err != nil {
		return err
	}

	useExisting := createExisting || branchExists
	if branchExists && !createExisting {
		output.Info("Branch '%s' already exists, using it", branch)
	}

	if dryRun {
		if useExisting {
			output.DryRunMsg("Would create worktree from existing branch '%s'", branch)
		} else {
			output.DryRunMsg("Would create worktree with new branch '%s' from '%s'", branch, baseBranch)
		}
		output.DryRunMsg("Would create iTerm2 window for %s", wtPath)
		output.DryRunMsg("Would save state")
		return nil
	}

	// Create worktree
	if useExisting {
		output.Info("Creating worktree from existing branch '%s'", branch)
		err = gitClient.WorktreeAdd(wtPath, branch, "", false)
	} else {
		output.Info("Creating worktree with new branch '%s' from '%s'", branch, baseBranch)
		err = gitClient.WorktreeAdd(wtPath, branch, baseBranch, true)
	}
	if err != nil {
		return err
	}
	output.Success("Git worktree created")

	// Create iTerm2 window
	sessionName := fmt.Sprintf("wt:%s:%s", repoName, dirname)
	output.Info("Creating iTerm2 window (session: %s)", ui.Cyan(sessionName))

	noClaude := createNoClaude || viper.GetBool("no_claude")
	sessions, err := itermClient.CreateWorktreeWindow(wtPath, sessionName, noClaude)
	if err != nil {
		output.Warning("Worktree created but failed to open iTerm2 window: %v", err)
		output.Info("Use 'wt open %s' to try again", branch)
		return nil
	}

	output.VerboseLog("Claude session: %s", sessions.ClaudeSessionID)
	output.VerboseLog("Shell session:  %s", sessions.ShellSessionID)

	// Save state
	err = stateMgr.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            repoName,
		Branch:          branch,
		ClaudeSessionID: sessions.ClaudeSessionID,
		ShellSessionID:  sessions.ShellSessionID,
		CreatedAt:       state.FlexTime{Time: time.Now().UTC()},
	})
	if err != nil {
		output.Warning("Failed to save state: %v", err)
	}

	fmt.Fprintln(output.Out)
	output.Success("Worktree ready: %s", ui.Cyan(wtPath))
	output.Success("iTerm2 window opened with Claude + shell panes")
	return nil
}
