package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/joescharf/wt/pkg/gitops"
	"github.com/joescharf/wt/pkg/iterm"
	state "github.com/joescharf/wt/pkg/wtstate"
)

// Server wraps the wt dependencies and exposes them as MCP tools.
type Server struct {
	git   GitClient
	iterm iterm.Client
	state *state.Manager
}

// NewServer creates the MCP server wrapper with all required dependencies.
func NewServer(gc GitClient, ic iterm.Client, sm *state.Manager) *Server {
	return &Server{
		git:   gc,
		iterm: ic,
		state: sm,
	}
}

// MCPServer returns a configured mcp-go server with all tools registered.
func (s *Server) MCPServer() *server.MCPServer {
	srv := server.NewMCPServer("wt", "1.0.0", server.WithToolCapabilities(true))

	srv.AddTool(s.listTool())
	srv.AddTool(s.createTool())
	srv.AddTool(s.openTool())
	srv.AddTool(s.deleteTool())
	srv.AddTool(s.syncTool())
	srv.AddTool(s.mergeTool())

	return srv
}

// ServeStdio starts the stdio transport, blocking until ctx is cancelled.
func (s *Server) ServeStdio(ctx context.Context) error {
	srv := s.MCPServer()
	stdioServer := server.NewStdioServer(srv)
	return stdioServer.Listen(ctx, os.Stdin, os.Stdout)
}

// ---------------------------------------------------------------------------
// Tool definitions and handlers
// ---------------------------------------------------------------------------

// wt_list
func (s *Server) listTool() (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.NewTool("wt_list",
		mcp.WithDescription("List worktrees for a repository with iTerm2 window status, git status, and age."),
		mcp.WithString("repo_path", mcp.Required(), mcp.Description("Absolute path to the git repository")),
	)
	return tool, s.handleList
}

func (s *Server) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoPath, err := request.RequireString("repo_path")
	if err != nil {
		return mcp.NewToolResultError("missing required parameter: repo_path"), nil
	}

	repoName, err := s.git.RepoName(repoPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get repo name: %v", err)), nil
	}

	repoRoot, err := s.git.RepoRoot(repoPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get repo root: %v", err)), nil
	}

	worktrees, err := s.git.WorktreeList(repoPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list worktrees: %v", err)), nil
	}

	type wtOut struct {
		Branch       string `json:"branch"`
		Path         string `json:"path"`
		WindowStatus string `json:"window_status"`
		GitStatus    string `json:"git_status"`
		Age          string `json:"age,omitempty"`
	}

	var out []wtOut
	for _, wt := range worktrees {
		if wt.Path == repoRoot {
			continue
		}

		windowStatus := "closed"
		ws, _ := s.state.GetWorktree(wt.Path)
		if ws != nil && ws.ClaudeSessionID != "" {
			if s.iterm.IsRunning() && s.iterm.SessionExists(ws.ClaudeSessionID) {
				windowStatus = "open"
			} else {
				windowStatus = "stale"
			}
		}

		gitStatus := "clean"
		dirty, err := s.git.IsWorktreeDirty(wt.Path)
		if err == nil && dirty {
			gitStatus = "dirty"
		}

		age := ""
		if ws != nil && !ws.CreatedAt.Time.IsZero() {
			age = formatAge(time.Since(ws.CreatedAt.Time))
		}

		out = append(out, wtOut{
			Branch:       wt.Branch,
			Path:         wt.Path,
			WindowStatus: windowStatus,
			GitStatus:    gitStatus,
			Age:          age,
		})
	}

	result := map[string]any{
		"repo":      repoName,
		"worktrees": out,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// wt_create
func (s *Server) createTool() (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.NewTool("wt_create",
		mcp.WithDescription("Create a new worktree + branch + iTerm2 window. If the branch already exists, uses it instead of creating a new one."),
		mcp.WithString("repo_path", mcp.Required(), mcp.Description("Absolute path to the git repository")),
		mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name for the worktree")),
		mcp.WithString("base", mcp.Description("Base branch to create from (default: main)")),
		mcp.WithBoolean("no_claude", mcp.Description("Don't auto-launch claude in top pane")),
	)
	return tool, s.handleCreate
}

func (s *Server) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoPath, err := request.RequireString("repo_path")
	if err != nil {
		return mcp.NewToolResultError("missing required parameter: repo_path"), nil
	}
	branch, err := request.RequireString("branch")
	if err != nil {
		return mcp.NewToolResultError("missing required parameter: branch"), nil
	}

	baseBranch := request.GetString("base", "main")
	noClaude := request.GetBool("no_claude", false)

	repoName, err := s.git.RepoName(repoPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get repo name: %v", err)), nil
	}

	wtDir, err := s.git.WorktreesDir(repoPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get worktrees dir: %v", err)), nil
	}

	dirname := gitops.BranchToDirname(branch)
	wtPath := filepath.Join(wtDir, dirname)

	// Create worktrees directory if needed
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create worktrees directory: %v", err)), nil
	}

	// Check if branch already exists
	branchExists, err := s.git.BranchExists(repoPath, branch)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to check branch: %v", err)), nil
	}

	newBranch := !branchExists
	var base string
	if newBranch {
		base = baseBranch
	}

	if err := s.git.WorktreeAdd(repoPath, wtPath, branch, base, newBranch); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create worktree: %v", err)), nil
	}

	// Create iTerm2 window
	sessionName := fmt.Sprintf("wt:%s:%s", repoName, dirname)
	sessions, err := s.iterm.CreateWorktreeWindow(wtPath, sessionName, noClaude)
	if err != nil {
		// Worktree was created but iTerm failed - still report partial success
		result := map[string]any{
			"repo":          repoName,
			"branch":        branch,
			"worktree_path": wtPath,
			"iterm_error":   err.Error(),
		}
		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	}

	// Save state
	_ = s.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            repoName,
		Branch:          branch,
		ClaudeSessionID: sessions.ClaudeSessionID,
		ShellSessionID:  sessions.ShellSessionID,
		CreatedAt:       state.FlexTime{Time: time.Now().UTC()},
	})

	result := map[string]any{
		"repo":             repoName,
		"branch":           branch,
		"worktree_path":    wtPath,
		"claude_session":   sessions.ClaudeSessionID,
		"shell_session":    sessions.ShellSessionID,
		"existing_branch":  branchExists,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// wt_open
func (s *Server) openTool() (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.NewTool("wt_open",
		mcp.WithDescription("Open or focus an iTerm2 window for an existing worktree."),
		mcp.WithString("repo_path", mcp.Required(), mcp.Description("Absolute path to the git repository")),
		mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree to open")),
		mcp.WithBoolean("no_claude", mcp.Description("Don't auto-launch claude in top pane")),
	)
	return tool, s.handleOpen
}

func (s *Server) handleOpen(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoPath, err := request.RequireString("repo_path")
	if err != nil {
		return mcp.NewToolResultError("missing required parameter: repo_path"), nil
	}
	branch, err := request.RequireString("branch")
	if err != nil {
		return mcp.NewToolResultError("missing required parameter: branch"), nil
	}

	noClaude := request.GetBool("no_claude", false)

	repoName, err := s.git.RepoName(repoPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get repo name: %v", err)), nil
	}

	// Find the worktree path
	wtPath, err := s.resolveWorktreePath(repoPath, branch)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("worktree not found for branch '%s': %v", branch, err)), nil
	}

	// Check if window already exists and focus it
	ws, _ := s.state.GetWorktree(wtPath)
	if ws != nil && ws.ClaudeSessionID != "" {
		if s.iterm.IsRunning() && s.iterm.SessionExists(ws.ClaudeSessionID) {
			if err := s.iterm.FocusWindow(ws.ClaudeSessionID); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to focus window: %v", err)), nil
			}
			result := map[string]any{
				"repo":           repoName,
				"branch":         branch,
				"worktree_path":  wtPath,
				"action":         "focused",
			}
			data, _ := json.Marshal(result)
			return mcp.NewToolResultText(string(data)), nil
		}
	}

	// Create new window
	dirname := filepath.Base(wtPath)
	sessionName := fmt.Sprintf("wt:%s:%s", repoName, dirname)
	sessions, err := s.iterm.CreateWorktreeWindow(wtPath, sessionName, noClaude)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create iTerm2 window: %v", err)), nil
	}

	// Get branch name
	branchName := branch
	if ws != nil && ws.Branch != "" {
		branchName = ws.Branch
	}

	_ = s.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            repoName,
		Branch:          branchName,
		ClaudeSessionID: sessions.ClaudeSessionID,
		ShellSessionID:  sessions.ShellSessionID,
		CreatedAt:       state.FlexTime{Time: time.Now().UTC()},
	})

	result := map[string]any{
		"repo":           repoName,
		"branch":         branchName,
		"worktree_path":  wtPath,
		"action":         "opened",
		"claude_session": sessions.ClaudeSessionID,
		"shell_session":  sessions.ShellSessionID,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// wt_delete
func (s *Server) deleteTool() (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.NewTool("wt_delete",
		mcp.WithDescription("Close iTerm2 window + remove worktree. Checks for uncommitted changes and unpushed commits unless force is set."),
		mcp.WithString("repo_path", mcp.Required(), mcp.Description("Absolute path to the git repository")),
		mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree to delete")),
		mcp.WithBoolean("force", mcp.Description("Force removal, skip safety checks")),
	)
	return tool, s.handleDelete
}

func (s *Server) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoPath, err := request.RequireString("repo_path")
	if err != nil {
		return mcp.NewToolResultError("missing required parameter: repo_path"), nil
	}
	branch, err := request.RequireString("branch")
	if err != nil {
		return mcp.NewToolResultError("missing required parameter: branch"), nil
	}

	force := request.GetBool("force", false)

	// Find the worktree path
	wtPath, err := s.resolveWorktreePath(repoPath, branch)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("worktree not found for branch '%s': %v", branch, err)), nil
	}

	// Safety checks (skip with force)
	if !force {
		dirty, err := s.git.IsWorktreeDirty(wtPath)
		if err == nil && dirty {
			return mcp.NewToolResultError(fmt.Sprintf("worktree '%s' has uncommitted changes (use force=true to override)", filepath.Base(wtPath))), nil
		}

		unpushed, err := s.git.HasUnpushedCommits(wtPath, "main")
		if err == nil && unpushed {
			return mcp.NewToolResultError(fmt.Sprintf("worktree '%s' has unpushed commits (use force=true to override)", filepath.Base(wtPath))), nil
		}
	}

	// Close iTerm2 window if it exists
	ws, _ := s.state.GetWorktree(wtPath)
	if ws != nil && ws.ClaudeSessionID != "" {
		if s.iterm.IsRunning() && s.iterm.SessionExists(ws.ClaudeSessionID) {
			_ = s.iterm.CloseWindow(ws.ClaudeSessionID)
		}
	}

	// Remove worktree
	if err := s.git.WorktreeRemove(repoPath, wtPath, force); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to remove worktree: %v", err)), nil
	}

	// Clean state
	_ = s.state.RemoveWorktree(wtPath)

	result := map[string]any{
		"branch":        branch,
		"worktree_path": wtPath,
		"deleted":       true,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// wt_sync
func (s *Server) syncTool() (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.NewTool("wt_sync",
		mcp.WithDescription("Sync worktree with base branch by merging or rebasing base into the feature branch."),
		mcp.WithString("repo_path", mcp.Required(), mcp.Description("Absolute path to the git repository")),
		mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree to sync")),
		mcp.WithString("strategy", mcp.Description("Sync strategy: merge (default) or rebase")),
	)
	return tool, s.handleSync
}

func (s *Server) handleSync(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoPath, err := request.RequireString("repo_path")
	if err != nil {
		return mcp.NewToolResultError("missing required parameter: repo_path"), nil
	}
	branch, err := request.RequireString("branch")
	if err != nil {
		return mcp.NewToolResultError("missing required parameter: branch"), nil
	}

	strategy := request.GetString("strategy", "merge")

	// Find the worktree path
	wtPath, err := s.resolveWorktreePath(repoPath, branch)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("worktree not found for branch '%s': %v", branch, err)), nil
	}

	baseBranch := "main"
	mergeSource := baseBranch

	// Fetch if remote exists
	hasRemote, _ := s.git.HasRemote(repoPath)
	if hasRemote {
		_ = s.git.Fetch(repoPath)
		mergeSource = "origin/" + baseBranch
	}

	// Check status
	behind, _ := s.git.CommitsBehind(wtPath, mergeSource)
	ahead, _ := s.git.CommitsAhead(wtPath, mergeSource)

	if behind == 0 {
		result := map[string]any{
			"branch":   branch,
			"status":   "already in sync",
			"ahead":    ahead,
			"behind":   behind,
		}
		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	}

	if strategy == "rebase" {
		if err := s.git.Rebase(wtPath, mergeSource); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("rebase failed: %v", err)), nil
		}
	} else {
		if err := s.git.Merge(wtPath, mergeSource); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("merge failed: %v", err)), nil
		}
	}

	result := map[string]any{
		"branch":   branch,
		"strategy": strategy,
		"synced":   true,
		"ahead":    ahead,
		"behind":   behind,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// wt_merge
func (s *Server) mergeTool() (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.NewTool("wt_merge",
		mcp.WithDescription("Merge worktree branch into base branch (local merge or push for PR)."),
		mcp.WithString("repo_path", mcp.Required(), mcp.Description("Absolute path to the git repository")),
		mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree to merge")),
		mcp.WithString("strategy", mcp.Description("Merge strategy: merge (default) or rebase (rebase-then-fast-forward)")),
		mcp.WithBoolean("pr", mcp.Description("Push branch for PR instead of local merge")),
	)
	return tool, s.handleMerge
}

func (s *Server) handleMerge(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoPath, err := request.RequireString("repo_path")
	if err != nil {
		return mcp.NewToolResultError("missing required parameter: repo_path"), nil
	}
	branch, err := request.RequireString("branch")
	if err != nil {
		return mcp.NewToolResultError("missing required parameter: branch"), nil
	}

	strategy := request.GetString("strategy", "merge")
	pr := request.GetBool("pr", false)

	// Find the worktree path
	wtPath, err := s.resolveWorktreePath(repoPath, branch)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("worktree not found for branch '%s': %v", branch, err)), nil
	}

	baseBranch := "main"

	// Check if there are commits to merge
	hasCommits, _ := s.git.HasUnpushedCommits(wtPath, baseBranch)
	if !hasCommits {
		result := map[string]any{
			"branch": branch,
			"status": "no commits to merge",
		}
		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	}

	if pr {
		return s.handleMergePR(repoPath, wtPath, branch, baseBranch)
	}
	return s.handleMergeLocal(repoPath, wtPath, branch, baseBranch, strategy)
}

func (s *Server) handleMergeLocal(repoPath, wtPath, branch, baseBranch, strategy string) (*mcp.CallToolResult, error) {
	repoRoot, err := s.git.RepoRoot(repoPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get repo root: %v", err)), nil
	}

	if strategy == "rebase" {
		// Rebase-then-fast-forward
		if err := s.git.Rebase(wtPath, baseBranch); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("rebase failed: %v", err)), nil
		}
		if err := s.git.Merge(repoRoot, branch); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("fast-forward merge failed: %v", err)), nil
		}
	} else {
		if err := s.git.Merge(repoRoot, branch); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("merge failed: %v", err)), nil
		}
	}

	// Push if remote exists
	hasRemote, _ := s.git.HasRemote(repoPath)
	if hasRemote {
		_ = s.git.Push(repoRoot, baseBranch, false)
	}

	result := map[string]any{
		"branch":      branch,
		"base":        baseBranch,
		"strategy":    strategy,
		"merged":      true,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleMergePR(repoPath, wtPath, branch, baseBranch string) (*mcp.CallToolResult, error) {
	// Push branch
	if err := s.git.Push(wtPath, branch, true); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("push failed: %v", err)), nil
	}

	result := map[string]any{
		"branch":   branch,
		"base":     baseBranch,
		"pushed":   true,
		"action":   "push for PR — create the PR on GitHub",
	}

	data, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolveWorktreePath finds the worktree path for a given branch by searching
// the worktree list using the shared resolution logic.
func (s *Server) resolveWorktreePath(repoPath, branch string) (string, error) {
	worktrees, err := s.git.WorktreeList(repoPath)
	if err != nil {
		return "", err
	}

	if path := gitops.ResolveWorktreeFromList(branch, worktrees); path != "" {
		return path, nil
	}

	return "", fmt.Errorf("not found")
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
