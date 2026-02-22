package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var configForce bool

// configDirFunc returns the config directory path, replaceable in tests.
var configDirFunc = defaultConfigDir

func defaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "wt"), nil
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show or manage configuration",
	Long: `Show or manage wt configuration.

Running bare 'wt config' is the same as 'wt config show'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return configShowRun()
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create config file with commented defaults",
	RunE: func(cmd *cobra.Command, args []string) error {
		return configInitRun()
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show effective configuration with sources",
	RunE: func(cmd *cobra.Command, args []string) error {
		return configShowRun()
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open config file in $EDITOR",
	RunE: func(cmd *cobra.Command, args []string) error {
		return configEditRun()
	},
}

func init() {
	configInitCmd.Flags().BoolVar(&configForce, "force", false, "Overwrite existing config file")
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configEditCmd)
	rootCmd.AddCommand(configCmd)
}

// configTemplate is the template for generating config.yaml with comments.
const configTemplate = `# wt configuration
# See: wt config show (for effective values and sources)

# Base branch for sync/merge/create (default: "main")
base_branch: {{ .BaseBranch }}

# Use rebase strategy by default for sync/merge (default: false)
rebase: {{ .Rebase }}

# Skip Claude Code launch in new worktree windows (default: false)
no_claude: {{ .NoClaude }}

# State file directory (uncomment to override)
# state_dir: {{ .StateDir }}
`

type configTemplateData struct {
	BaseBranch string
	Rebase     bool
	NoClaude   bool
	StateDir   string
}

func configFilePath() (string, error) {
	dir, err := configDirFunc()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func configInitRun() error {
	cfgPath, err := configFilePath()
	if err != nil {
		return err
	}

	// Check if file already exists
	if _, err := os.Stat(cfgPath); err == nil {
		if !configForce {
			return fmt.Errorf("config file already exists: %s (use --force to overwrite)", cfgPath)
		}
		output.Warning("Overwriting existing config file")
	}

	// Build template data from current viper values
	data := configTemplateData{
		BaseBranch: viper.GetString("base_branch"),
		Rebase:     viper.GetBool("rebase"),
		NoClaude:   viper.GetBool("no_claude"),
		StateDir:   viper.GetString("state_dir"),
	}

	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("template execute error: %w", err)
	}

	if dryRun {
		output.DryRunMsg("Would create config file: %s", cfgPath)
		_, _ = fmt.Fprintln(output.Out)
		_, _ = fmt.Fprint(output.Out, buf.String())
		return nil
	}

	// Create config directory
	dir := filepath.Dir(cfgPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(cfgPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	output.Success("Config file created: %s", cfgPath)
	_, _ = fmt.Fprintln(output.Out)
	_, _ = fmt.Fprint(output.Out, buf.String())
	return nil
}

// configKeyInfo describes a config key for display purposes.
type configKeyInfo struct {
	Key    string
	EnvVar string
}

var configKeys = []configKeyInfo{
	{Key: "base_branch", EnvVar: "WT_BASE_BRANCH"},
	{Key: "rebase", EnvVar: "WT_REBASE"},
	{Key: "no_claude", EnvVar: "WT_NO_CLAUDE"},
	{Key: "state_dir", EnvVar: "WT_STATE_DIR"},
}

func configShowRun() error {
	cfgPath, err := configFilePath()
	if err != nil {
		return err
	}

	// Check if config file exists
	if _, err := os.Stat(cfgPath); err == nil {
		output.Info("Config file: %s", cfgPath)
	} else {
		output.Info("Config file: (none)")
	}
	_, _ = fmt.Fprintln(output.Out)

	// Read config file values to determine file source
	fileValues := readConfigFileValues(cfgPath)

	for _, k := range configKeys {
		val := viper.Get(k.Key)
		source := detectSource(k.Key, k.EnvVar, fileValues)
		_, _ = fmt.Fprintf(output.Out, "  %-14s %v  %s\n", k.Key, val, source)
	}

	return nil
}

// readConfigFileValues reads the raw YAML file and returns a map of keys present in it.
func readConfigFileValues(path string) map[string]bool {
	result := make(map[string]bool)

	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return result
	}

	for key := range parsed {
		result[key] = true
	}
	return result
}

// detectSource determines where a config value is coming from.
func detectSource(key, envVar string, fileValues map[string]bool) string {
	if _, ok := os.LookupEnv(envVar); ok {
		return fmt.Sprintf("(env: %s)", envVar)
	}
	if fileValues[key] {
		return "(file)"
	}
	return "(default)"
}

func configEditRun() error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		return fmt.Errorf("$EDITOR is not set — set it to your preferred editor (e.g. export EDITOR=vim)")
	}

	cfgPath, err := configFilePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s (run 'wt config init' first)", cfgPath)
	}

	if dryRun {
		output.DryRunMsg("Would open %s in %s", cfgPath, editor)
		return nil
	}

	cmd := exec.Command(editor, cfgPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
