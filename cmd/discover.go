package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	state "github.com/joescharf/wt/pkg/wtstate"
	"github.com/joescharf/wt/internal/ui"
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
	repoName, err := gitClient.RepoName(repoRoot)
	if err != nil {
		return err
	}

	wtDir, err := gitClient.WorktreesDir(repoRoot)
	if err != nil {
		return err
	}

	worktrees, err := gitClient.WorktreeList(repoRoot)
	if err != nil {
		return err
	}

	// Find worktrees that are NOT in wt state
	var unmanaged []struct {
		path   string
		branch string
	}

	for _, wt := range worktrees {
		// Skip the main repo
		if wt.Path == repoRoot {
			continue
		}

		// Check if already in state
		ws, _ := stateMgr.GetWorktree(wt.Path)
		if ws != nil {
			continue
		}

		unmanaged = append(unmanaged, struct {
			path   string
			branch string
		}{path: wt.Path, branch: wt.Branch})
	}

	if len(unmanaged) == 0 {
		output.Success("No unmanaged worktrees found")
		return nil
	}

	output.Info("Found %d unmanaged worktrees for '%s'", len(unmanaged), ui.Cyan(repoName))
	fmt.Fprintln(output.Out)

	for _, wt := range unmanaged {
		source := classifySource(wt.path, wtDir)
		output.Info("  %s  %s  (%s)", ui.Cyan(wt.branch), wt.path, source)
	}
	fmt.Fprintln(output.Out)

	if !discoverAdopt {
		output.Info("Run 'wt discover --adopt' to create state entries for these worktrees")
		return nil
	}

	if dryRun {
		output.DryRunMsg("Would adopt %d worktrees", len(unmanaged))
		return nil
	}

	adopted := 0
	for _, wt := range unmanaged {
		err := stateMgr.SetWorktree(wt.path, &state.WorktreeState{
			Repo:      repoName,
			Branch:    wt.branch,
			CreatedAt: state.FlexTime{Time: time.Now().UTC()},
		})
		if err != nil {
			output.Warning("Failed to adopt '%s': %v", wt.branch, err)
			continue
		}
		output.Success("Adopted '%s'", ui.Cyan(wt.branch))
		adopted++
	}

	fmt.Fprintln(output.Out)
	output.Success("Adopted %d worktrees", adopted)
	return nil
}

// classifySource determines the source label for a worktree path.
// "wt" if in the standard worktrees dir, "external" otherwise.
func classifySource(wtPath, standardDir string) string {
	if len(wtPath) > len(standardDir) && wtPath[:len(standardDir)] == standardDir {
		return "wt"
	}
	return "external"
}
