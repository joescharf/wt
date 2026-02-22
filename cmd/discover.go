package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/joescharf/wt/internal/ui"
	"github.com/joescharf/wt/pkg/ops"
	state "github.com/joescharf/wt/pkg/wtstate"
)

var discoverAdopt bool

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Find worktrees not managed by wt (e.g. created by Claude Code)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return discoverRun()
	},
}

func init() {
	discoverCmd.Flags().BoolVar(&discoverAdopt, "adopt", false, "Create state entries for discovered worktrees")
	rootCmd.AddCommand(discoverCmd)
}

func discoverRun() error {
	// Build state checker callback
	stateCheck := func(path string) (bool, error) {
		ws, _ := stateMgr.GetWorktree(path)
		return ws != nil, nil
	}

	// Build state adopter callback
	stateAdopt := func(path, repo, branch string) error {
		return stateMgr.SetWorktree(path, &state.WorktreeState{
			Repo:      repo,
			Branch:    branch,
			CreatedAt: state.FlexTime{Time: time.Now().UTC()},
		})
	}

	result, err := ops.Discover(gitClient, opsLogger, ops.DiscoverOptions{
		RepoPath: repoRoot,
		Adopt:    discoverAdopt,
		DryRun:   dryRun,
	}, stateCheck, stateAdopt)
	if err != nil {
		return err
	}

	// Print colored output for unmanaged worktrees (ops layer doesn't have UI colors)
	if len(result.Unmanaged) > 0 {
		_, _ = fmt.Fprintln(output.Out)
		for _, wt := range result.Unmanaged {
			output.Info("  %s  %s  (%s)", ui.Cyan(wt.Branch), wt.Path, wt.Source)
		}
		_, _ = fmt.Fprintln(output.Out)

		if !discoverAdopt {
			output.Info("Run 'wt discover --adopt' to create state entries for these worktrees")
		}
	}

	if result.Adopted > 0 {
		_, _ = fmt.Fprintln(output.Out)
	}
	return nil
}
