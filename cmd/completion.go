package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:       "completion [bash|zsh|fish]",
	Short:     "Generate shell completion script",
	Long: `Generate shell completion scripts for wt.

To load completions:

Bash:
  source <(wt completion bash)

  # To load completions for each session, execute once:
  # Linux:
  wt completion bash > /etc/bash_completion.d/wt
  # macOS:
  wt completion bash > $(brew --prefix)/etc/bash_completion.d/wt

Zsh:
  # If shell completion is not already enabled in your environment,
  # enable it by running:
  echo "autoload -U compinit; compinit" >> ~/.zshrc

  source <(wt completion zsh)

  # To load completions for each session, execute once:
  wt completion zsh > "${fpath[1]}/_wt"

Fish:
  wt completion fish | source

  # To load completions for each session, execute once:
  wt completion fish > ~/.config/fish/completions/wt.fish
`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish"},
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletionV2(os.Stdout, true)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		default:
			return cmd.Help()
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}

// completeWorktreeNames returns existing worktree dirnames for shell completion.
func completeWorktreeNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if gitClient == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	repoRoot, err := gitClient.RepoRoot()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	worktrees, err := gitClient.WorktreeList()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, wt := range worktrees {
		if wt.Path == repoRoot {
			continue
		}
		names = append(names, wt.Branch)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeBranchNames returns local branch names for shell completion.
func completeBranchNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if gitClient == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	branches, err := gitClient.BranchList()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return branches, cobra.ShellCompDirectiveNoFileComp
}
