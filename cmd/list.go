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
	state "github.com/joescharf/wt/pkg/wtstate"
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
	repoName, err := gitClient.RepoName(repoRoot)
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

	_, _ = fmt.Fprintf(output.Out, "Worktrees for %s\n\n", ui.Cyan(repoName))

	worktrees, err := gitClient.WorktreeList(repoRoot)
	if err != nil {
		return err
	}

	termWidth := ui.TermWidth()
	baseBranch := viper.GetString("base_branch")

	wtDir, err := gitClient.WorktreesDir(repoRoot)
	if err != nil {
		output.VerboseLog("Could not get worktrees dir: %v", err)
	}

	// Budget column widths based on terminal size.
	// Table overhead: 7 border chars + 12 padding chars (1 each side × 6 cols) = 19
	// Fixed columns: SOURCE(8) + WINDOW(6) + STATUS(15) + AGE(4) = 33
	const tableOverhead = 19
	const fixedCols = 33
	available := termWidth - tableOverhead - fixedCols
	if available < 20 {
		available = 20
	}
	maxBranch := available * 55 / 100
	maxPath := available - maxBranch
	if maxBranch < 10 {
		maxBranch = 10
	}
	if maxPath < 10 {
		maxPath = 10
	}

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
			if rebasing, err := gitClient.IsRebaseInProgress(wt.Path); err != nil {
				output.VerboseLog("Could not check rebase status for %s: %v", wt.Branch, err)
			} else if rebasing {
				parts = append(parts, "rebasing")
			}
			if merging, err := gitClient.IsMergeInProgress(wt.Path); err != nil {
				output.VerboseLog("Could not check merge status for %s: %v", wt.Branch, err)
			} else if merging {
				parts = append(parts, "merging")
			}
			if dirty {
				parts = append(parts, "dirty")
			}
			if ahead > 0 {
				parts = append(parts, fmt.Sprintf("↑%d", ahead))
			}
			if behind > 0 {
				parts = append(parts, fmt.Sprintf("↓%d", behind))
			}
			if len(parts) > 0 {
				gitStatus = strings.Join(parts, " ")
			}
		}

		// Calculate age
		age := "-"
		if ws != nil && !ws.CreatedAt.IsZero() {
			age = formatAge(time.Since(ws.CreatedAt.Time))
		}

		// Determine source
		source := worktreeSource(wt.Path, wtDir, ws)

		displayBranch := truncRight(wt.Branch, maxBranch)
		displayPath := truncLeft(wt.Path, maxPath)

		rows = append(rows, []string{
			displayBranch,
			displayPath,
			ui.SourceColor(source),
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
					Formatting: tw.CellFormatting{AutoFormat: tw.Off, AutoWrap: tw.WrapTruncate},
				},
				Row: tw.CellConfig{
					Alignment:  tw.CellAlignment{Global: tw.AlignLeft},
					Formatting: tw.CellFormatting{AutoWrap: tw.WrapTruncate},
				},
			}),
			tablewriter.WithHeaderAutoFormat(tw.Off),
		)

		table.Header("BRANCH", "PATH", "SOURCE", "WINDOW", "STATUS", "AGE")
		_ = table.Bulk(rows)
		_ = table.Render()
	}
	_, _ = fmt.Fprintln(output.Out)
	return nil
}

// truncRight truncates s from the right if it exceeds max, appending "…".
func truncRight(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return s[:max-1] + "…"
}

// truncLeft truncates s from the left if it exceeds max, prepending "…".
func truncLeft(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return "…" + s[len(s)-(max-1):]
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

// worktreeSource classifies a worktree as "wt" (standard dir), "adopted" (external but has state),
// or "external" (external with no state).
func worktreeSource(wtPath, standardDir string, ws *state.WorktreeState) string {
	inStandardDir := standardDir != "" && strings.HasPrefix(wtPath, standardDir+"/")
	if inStandardDir {
		return "wt"
	}
	if ws != nil {
		return "adopted"
	}
	return "external"
}
