package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/joescharf/wt/pkg/gitops"
	wmcp "github.com/joescharf/wt/internal/mcp"
	state "github.com/joescharf/wt/pkg/wtstate"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP server for Claude Code integration",
	Long: `Start an MCP (Model Context Protocol) server on stdio.

This allows Claude Code to manage worktrees natively via MCP tools.
Configure in Claude Code with:

  wt mcp install

Or manually add to ~/.claude.json:

  {
    "mcpServers": {
      "wt": { "command": "/path/to/wt", "args": ["mcp"] }
    }
  }

Available tools: wt_list, wt_create, wt_open, wt_delete, wt_sync, wt_merge`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return mcpServeRun()
	},
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start MCP stdio server",
	RunE: func(cmd *cobra.Command, args []string) error {
		return mcpServeRun()
	},
}

var mcpInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install wt as an MCP server in Claude Code",
	Long:  "Write the MCP server configuration to ~/.claude.json so Claude Code can use wt tools.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return mcpInstallRun()
	},
}

var mcpStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check MCP server installation status",
	RunE: func(cmd *cobra.Command, args []string) error {
		return mcpStatusRun()
	},
}

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
	mcpCmd.AddCommand(mcpInstallCmd)
	mcpCmd.AddCommand(mcpStatusCmd)
	rootCmd.AddCommand(mcpCmd)
}

func mcpServeRun() error {
	gc := gitops.NewClient()

	// For the MCP server, create a minimal state manager.
	// The state file location matches initDeps() in root.go.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}
	stateDir := filepath.Join(home, ".config", "wt")
	sm := state.NewManager(filepath.Join(stateDir, "state.json"))

	// iTerm client uses the existing package-level itermClient if initialized,
	// but for MCP serve we need to create our own since cobra.OnInitialize
	// may not have run for the MCP command's deps.
	initDeps()

	cfg := wmcp.Config{
		BaseBranch: viper.GetString("base_branch"),
	}
	srv := wmcp.NewServer(gc, itermClient, sm, cfg)
	return srv.ServeStdio(context.Background())
}

func mcpInstallRun() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	claudeJSON := filepath.Join(home, ".claude.json")

	// Get the full path to the current executable
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	// Read existing config or start fresh
	config := make(map[string]any)
	if data, err := os.ReadFile(claudeJSON); err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("parse %s: %w", claudeJSON, err)
		}
	}

	// Merge mcpServers.wt entry
	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		servers = make(map[string]any)
	}
	servers["wt"] = map[string]any{
		"command": exePath,
		"args":    []string{"mcp"},
	}
	config["mcpServers"] = servers

	// Write back
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if dryRun {
		output.DryRunMsg("Would write to %s:\n%s", claudeJSON, string(data))
		return nil
	}

	if err := os.WriteFile(claudeJSON, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", claudeJSON, err)
	}

	output.Success("Installed wt MCP server in %s", claudeJSON)
	output.Info("  Command: %s mcp", exePath)
	output.Info("  Restart Claude Code to pick up the change.")
	return nil
}

func mcpStatusRun() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	// Check ~/.claude.json
	claudeJSON := filepath.Join(home, ".claude.json")
	checkWTMCPConfig("~/.claude.json", claudeJSON)

	// Check .mcp.json in cwd
	cwd, _ := os.Getwd()
	if cwd != "" {
		mcpJSON := filepath.Join(cwd, ".mcp.json")
		checkWTMCPConfig(".mcp.json (cwd)", mcpJSON)
	}

	return nil
}

func checkWTMCPConfig(label, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		output.Info("%s: not found", label)
		return
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		output.Warning("%s: invalid JSON", label)
		return
	}

	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		output.Info("%s: no mcpServers configured", label)
		return
	}

	wt, ok := servers["wt"]
	if !ok {
		output.Info("%s: wt not configured (other servers present)", label)
		return
	}

	wtConfig, ok := wt.(map[string]any)
	if !ok {
		output.Warning("%s: wt entry has unexpected format", label)
		return
	}

	cmd, _ := wtConfig["command"].(string)
	output.Success("%s: wt configured (command: %s)", label, cmd)
}
