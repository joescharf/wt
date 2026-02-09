package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/wt/internal/state"
	"github.com/joescharf/wt/internal/ui"
)

var openNoClaude bool

var openCmd = &cobra.Command{
	Use:               "open <branch>",
	Short:             "Open or focus iTerm2 window for an existing worktree",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		return openRun(args[0])
	},
}

func init() {
	openCmd.Flags().BoolVar(&openNoClaude, "no-claude", false, "Don't auto-launch claude in top pane")
	rootCmd.AddCommand(openCmd)
}

// openRun is the core logic for opening/focusing an iTerm2 window.
// Exported for reuse by create and root shorthand.
func openRun(branch string) error {
	repoName, err := gitClient.RepoName()
	if err != nil {
		return err
	}

	wtPath, err := gitClient.ResolveWorktree(branch)
	if err != nil {
		return err
	}
	dirname := filepath.Base(wtPath)

	if !isDirectory(wtPath) {
		output.Error("Worktree not found: %s", wtPath)
		output.Info("Use 'wt create %s' to create it", branch)
		return fmt.Errorf("worktree not found: %s", wtPath)
	}

	// Check if window already exists
	ws, err := stateMgr.GetWorktree(wtPath)
	if err != nil {
		return err
	}
	if ws != nil && ws.ClaudeSessionID != "" {
		if itermClient.IsRunning() && itermClient.SessionExists(ws.ClaudeSessionID) {
			output.Info("iTerm2 window already open, focusing it")
			return itermClient.FocusWindow(ws.ClaudeSessionID)
		}
	}

	if dryRun {
		output.DryRunMsg("Would open iTerm2 window for %s", wtPath)
		return nil
	}

	sessionName := fmt.Sprintf("wt:%s:%s", repoName, dirname)
	output.Info("Opening iTerm2 window for '%s'", ui.Cyan(dirname))

	noClaude := openNoClaude || viper.GetBool("no_claude")
	sessions, err := itermClient.CreateWorktreeWindow(wtPath, sessionName, noClaude)
	if err != nil {
		return fmt.Errorf("failed to create iTerm2 window: %w", err)
	}

	// Get branch from state or git
	branchName := branch
	if ws != nil && ws.Branch != "" {
		branchName = ws.Branch
	} else {
		if b, err := gitClient.CurrentBranch(wtPath); err == nil {
			branchName = b
		}
	}

	err = stateMgr.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            repoName,
		Branch:          branchName,
		ClaudeSessionID: sessions.ClaudeSessionID,
		ShellSessionID:  sessions.ShellSessionID,
		CreatedAt:       state.FlexTime{Time: time.Now().UTC()},
	})
	if err != nil {
		output.Warning("Window opened but failed to save state: %v", err)
	}

	output.Success("iTerm2 window opened for '%s'", ui.Cyan(dirname))
	return nil
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
