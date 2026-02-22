package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/wt/pkg/lifecycle"
	"github.com/joescharf/wt/internal/ui"
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
	baseBranch := createBase
	if baseBranch == "" {
		baseBranch = viper.GetString("base_branch")
	}

	noClaude := createNoClaude || viper.GetBool("no_claude")

	result, err := lcMgr.Create(lifecycle.CreateOptions{
		RepoPath:   repoRoot,
		Branch:     branch,
		BaseBranch: baseBranch,
		NoClaude:   noClaude,
		Existing:   createExisting,
		DryRun:     dryRun,
	})
	if err != nil {
		return err
	}

	if result.Created {
		fmt.Fprintln(output.Out)
		output.Success("Worktree ready: %s", ui.Cyan(result.WtPath))
	}
	return nil
}
