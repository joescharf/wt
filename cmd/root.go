package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/wt/internal/claude"
	"github.com/joescharf/wt/internal/git"
	"github.com/joescharf/wt/internal/iterm"
	"github.com/joescharf/wt/internal/state"
	"github.com/joescharf/wt/internal/ui"
)

// Package-level shared dependencies, initialized in cobra.OnInitialize.
var (
	gitClient   git.Client
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

	gitClient = git.NewClient()
	itermClient = iterm.NewClient()

	if claudePath, err := claude.DefaultPath(); err == nil {
		claudeTrust = claude.NewTrustManager(claudePath)
	}
}
