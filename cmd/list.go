package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/wt/internal/ui"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List worktrees with iTerm2 window status",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listRun()
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func listRun() error {
	repoName, err := gitClient.RepoName()
	if err != nil {
		return err
	}
	repoRoot, err := gitClient.RepoRoot()
	if err != nil {
		return err
	}

	// Prune stale state
	pruned, err := stateMgr.Prune()
	if err != nil {
		output.Warning("Failed to prune state: %v", err)
	}
	if pruned > 0 {
		output.Info("Pruned %d stale state entries", pruned)
	}

	fmt.Fprintf(output.Out, "Worktrees for %s\n\n", ui.Cyan(repoName))

	worktrees, err := gitClient.WorktreeList()
	if err != nil {
		return err
	}

	baseBranch := viper.GetString("base_branch")

	var rows [][]string
	for _, wt := range worktrees {
		// Skip the main repo worktree
		if wt.Path == repoRoot {
			continue
		}

		// Check iTerm2 window status
		windowStatus := "closed"
		ws, _ := stateMgr.GetWorktree(wt.Path)
		if ws != nil && ws.ClaudeSessionID != "" {
			if itermClient.IsRunning() && itermClient.SessionExists(ws.ClaudeSessionID) {
				windowStatus = "open"
			} else {
				windowStatus = "stale"
			}
		}

		// Check git status
		gitStatus := "clean"
		dirty, err := gitClient.IsWorktreeDirty(wt.Path)
		if err != nil {
			output.VerboseLog("Could not check status for %s: %v", wt.Branch, err)
			gitStatus = "?"
		} else {
			ahead, aheadErr := gitClient.CommitsAhead(wt.Path, baseBranch)
			if aheadErr != nil {
				output.VerboseLog("Could not check ahead status for %s: %v", wt.Branch, aheadErr)
			}
			behind, behindErr := gitClient.CommitsBehind(wt.Path, baseBranch)
			if behindErr != nil {
				output.VerboseLog("Could not check behind status for %s: %v", wt.Branch, behindErr)
			}

			var parts []string
			if dirty {
				parts = append(parts, "dirty")
			}
			if ahead > 0 {
				parts = append(parts, fmt.Sprintf("+%d", ahead))
			}
			if behind > 0 {
				parts = append(parts, fmt.Sprintf("-%d", behind))
			}
			if len(parts) > 0 {
				gitStatus = strings.Join(parts, " ")
			}
		}

		// Calculate age
		age := "-"
		if ws != nil && !ws.CreatedAt.Time.IsZero() {
			age = formatAge(time.Since(ws.CreatedAt.Time))
		}

		displayPath := wt.Path
		if len(displayPath) > 30 {
			displayPath = "..." + displayPath[len(displayPath)-27:]
		}

		rows = append(rows, []string{
			wt.Branch,
			displayPath,
			ui.StatusColor(windowStatus),
			ui.GitStatusColor(gitStatus),
			age,
		})
	}

	if len(rows) == 0 {
		output.Warning("No worktrees found")
	} else {
		table := tablewriter.NewTable(output.Out,
			tablewriter.WithRenderer(renderer.NewColorized(renderer.ColorizedConfig{
				Header: renderer.Tint{FG: renderer.Colors{color.FgHiBlue, color.Bold}},
				Border: renderer.Tint{FG: renderer.Colors{color.FgHiBlack}},
			})),
			tablewriter.WithConfig(tablewriter.Config{
				Header: tw.CellConfig{
					Alignment:  tw.CellAlignment{Global: tw.AlignLeft},
					Formatting: tw.CellFormatting{AutoFormat: tw.Off},
				},
				Row: tw.CellConfig{
					Alignment: tw.CellAlignment{Global: tw.AlignLeft},
				},
			}),
			tablewriter.WithHeaderAutoFormat(tw.Off),
		)

		table.Header("BRANCH", "PATH", "WINDOW", "STATUS", "AGE")
		table.Bulk(rows)
		table.Render()
	}
	fmt.Fprintln(output.Out)
	return nil
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

