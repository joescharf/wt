package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/joescharf/worktree-dev/internal/ui"
)

var switchCmd = &cobra.Command{
	Use:               "switch <branch>",
	Aliases:           []string{"go"},
	Short:             "Focus existing worktree's iTerm2 window",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		return switchRun(args[0])
	},
}

func init() {
	rootCmd.AddCommand(switchCmd)
}

func switchRun(branch string) error {
	wtPath, err := gitClient.ResolveWorktree(branch)
	if err != nil {
		return err
	}

	if !isDirectory(wtPath) {
		return fmt.Errorf("worktree not found: %s", wtPath)
	}

	ws, err := stateMgr.GetWorktree(wtPath)
	if err != nil {
		return err
	}

	if ws == nil || ws.ClaudeSessionID == "" {
		output.Warning("No iTerm2 session recorded for this worktree")
		output.Info("Use 'wt open %s' to create a window", branch)
		return fmt.Errorf("no iTerm2 session for worktree")
	}

	if dryRun {
		output.DryRunMsg("Would focus iTerm2 window for session %s", ws.ClaudeSessionID)
		return nil
	}

	if err := itermClient.EnsureRunning(); err != nil {
		return err
	}

	if itermClient.SessionExists(ws.ClaudeSessionID) {
		if err := itermClient.FocusWindow(ws.ClaudeSessionID); err != nil {
			return err
		}
		output.Success("Focused iTerm2 window for '%s'", ui.Cyan(filepath.Base(wtPath)))
	} else {
		output.Warning("iTerm2 window no longer exists")
		output.Info("Use 'wt open %s' to create a new window", branch)
	}

	return nil
}
