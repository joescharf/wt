package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/wt/pkg/claude"
	"github.com/joescharf/wt/pkg/gitops"
	"github.com/joescharf/wt/pkg/iterm"
	state "github.com/joescharf/wt/pkg/wtstate"
	"github.com/joescharf/wt/internal/ui"
)

// Package-level shared dependencies, initialized in cobra.OnInitialize.
var (
	gitClient   gitops.Client
	itermClient iterm.Client
	stateMgr    *state.Manager
	claudeTrust *claude.TrustManager
	output      *ui.UI

	verbose bool
	dryRun  bool
)

var rootCmd = &cobra.Command{
	Use:   "wt",
	Short: "Git worktree manager with iTerm2 integration",
	Long: `wt manages git worktrees with dedicated iTerm2 windows.
Each worktree gets a window with Claude on top and a shell on bottom.

Shorthand: wt <branch>   (same as: wt open <branch>)`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	SilenceUsage:      true,
	SilenceErrors:     true,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return openRun(args[0])
		}
		return listRun()
	},
}

// Execute is the main entry point called from main.go.
func Execute(version, commit, date string) {
	// Set version info for the version subcommand
	buildVersion = version
	buildCommit = commit
	buildDate = date

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig, initDeps)

	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolVarP(&dryRun, "dry-run", "n", false, "Show what would happen without making changes")
}

func initConfig() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot find home directory: %v\n", err)
		os.Exit(1)
	}

	configDir := filepath.Join(home, ".config", "wt")
	viper.AddConfigPath(configDir)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("WT")
	viper.AutomaticEnv()

	// Defaults via viper.SetDefault()
	viper.SetDefault("state_dir", configDir)
	viper.SetDefault("base_branch", "main")
	viper.SetDefault("no_claude", false)
	viper.SetDefault("rebase", false)

	// Read config file if it exists (optional)
	_ = viper.ReadInConfig()
}

func initDeps() {
	output = ui.New()
	output.Verbose = verbose
	output.DryRun = dryRun

	stateDir := viper.GetString("state_dir")
	statePath := filepath.Join(stateDir, "state.json")
	stateMgr = state.NewManager(statePath)

	gitClient = gitops.NewClient()
	itermClient = iterm.NewClient()

	if claudePath, err := claude.DefaultPath(); err == nil {
		claudeTrust = claude.NewTrustManager(claudePath)
	}
}

// resolveStrategy determines the merge strategy based on flags and config.
// --rebase flag wins, then --merge flag wins, then config, then default "merge".
func resolveStrategy(rebaseFlag, mergeFlag bool) string {
	if rebaseFlag {
		return "rebase"
	}
	if mergeFlag {
		return "merge"
	}
	if viper.GetBool("rebase") {
		return "rebase"
	}
	return "merge"
}
