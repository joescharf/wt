package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/wt/pkg/lifecycle"
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
	// Resolve worktree path
	wtPath, err := gitClient.ResolveWorktree(repoRoot, branch)
	if err != nil {
		// Worktree not found — offer to create it
		output.Warning("Worktree not found: %s", branch)
		if promptDefaultYes(fmt.Sprintf("Create worktree '%s'?", branch)) {
			return createRun(branch)
		}
		return nil
	}

	noClaude := openNoClaude || viper.GetBool("no_claude")

	_, err = lcMgr.Open(lifecycle.OpenOptions{
		RepoPath: repoRoot,
		WtPath:   wtPath,
		Branch:   branch,
		NoClaude: noClaude,
		DryRun:   dryRun,
	})
	return err
}

