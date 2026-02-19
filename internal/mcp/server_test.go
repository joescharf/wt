package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joescharf/wt/pkg/gitops"
	"github.com/joescharf/wt/internal/iterm"
	state "github.com/joescharf/wt/pkg/wtstate"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

// mockGitClient implements GitClient for testing.
type mockGitClient struct {
	repoRoot     string
	repoName     string
	worktreesDir string
	worktrees    []gitops.WorktreeInfo
	branches     map[string]bool
	currentBranch string
	dirty        bool
	hasRemote    bool
	commitsAhead int
	commitsBehind int
	hasUnpushed  bool

	// Track calls
	addedWorktrees   []wtAddCall
	removedWorktrees []wtRemoveCall
	fetchCalls       int
	mergeCalls       []string
	rebaseCalls      []string
	pushCalls        []pushCall
	pullCalls        int
	pruneCalls       int
	deletedBranches  []string

	// Error injection
	repoRootErr      error
	worktreeListErr  error
	worktreeAddErr   error
	worktreeRemoveErr error
	branchExistsErr  error
	currentBranchErr error
	dirtyErr         error
	unpushedErr      error
	hasRemoteErr     error
	fetchErr         error
	mergeErr         error
	rebaseErr        error
	pushErr          error
	pullErr          error
	aheadErr         error
	behindErr        error
	pruneErr         error
}

type wtAddCall struct {
	repoPath, wtPath, branch, base string
	newBranch                      bool
}

type wtRemoveCall struct {
	repoPath, wtPath string
	force            bool
}

type pushCall struct {
	worktreePath, branch string
	setUpstream          bool
}

func (m *mockGitClient) RepoRoot(repoPath string) (string, error) {
	if m.repoRootErr != nil {
		return "", m.repoRootErr
	}
	if m.repoRoot != "" {
		return m.repoRoot, nil
	}
	return repoPath, nil
}

func (m *mockGitClient) RepoName(repoPath string) (string, error) {
	if m.repoRootErr != nil {
		return "", m.repoRootErr
	}
	if m.repoName != "" {
		return m.repoName, nil
	}
	return filepath.Base(repoPath), nil
}

func (m *mockGitClient) WorktreesDir(repoPath string) (string, error) {
	if m.repoRootErr != nil {
		return "", m.repoRootErr
	}
	if m.worktreesDir != "" {
		return m.worktreesDir, nil
	}
	return repoPath + ".worktrees", nil
}

func (m *mockGitClient) WorktreeList(repoPath string) ([]gitops.WorktreeInfo, error) {
	if m.worktreeListErr != nil {
		return nil, m.worktreeListErr
	}
	return m.worktrees, nil
}

func (m *mockGitClient) WorktreeAdd(repoPath, wtPath, branch, base string, newBranch bool) error {
	if m.worktreeAddErr != nil {
		return m.worktreeAddErr
	}
	m.addedWorktrees = append(m.addedWorktrees, wtAddCall{repoPath, wtPath, branch, base, newBranch})
	return nil
}

func (m *mockGitClient) WorktreeRemove(repoPath, wtPath string, force bool) error {
	if m.worktreeRemoveErr != nil {
		return m.worktreeRemoveErr
	}
	m.removedWorktrees = append(m.removedWorktrees, wtRemoveCall{repoPath, wtPath, force})
	return nil
}

func (m *mockGitClient) BranchExists(repoPath, branch string) (bool, error) {
	if m.branchExistsErr != nil {
		return false, m.branchExistsErr
	}
	return m.branches[branch], nil
}

func (m *mockGitClient) BranchDelete(repoPath, branch string, force bool) error {
	m.deletedBranches = append(m.deletedBranches, branch)
	return nil
}

func (m *mockGitClient) CurrentBranch(worktreePath string) (string, error) {
	if m.currentBranchErr != nil {
		return "", m.currentBranchErr
	}
	return m.currentBranch, nil
}

func (m *mockGitClient) IsWorktreeDirty(worktreePath string) (bool, error) {
	if m.dirtyErr != nil {
		return false, m.dirtyErr
	}
	return m.dirty, nil
}

func (m *mockGitClient) HasUnpushedCommits(worktreePath, baseBranch string) (bool, error) {
	if m.unpushedErr != nil {
		return false, m.unpushedErr
	}
	return m.hasUnpushed, nil
}

func (m *mockGitClient) HasRemote(repoPath string) (bool, error) {
	if m.hasRemoteErr != nil {
		return false, m.hasRemoteErr
	}
	return m.hasRemote, nil
}

func (m *mockGitClient) Fetch(repoPath string) error {
	if m.fetchErr != nil {
		return m.fetchErr
	}
	m.fetchCalls++
	return nil
}

func (m *mockGitClient) Merge(repoPath, branch string) error {
	if m.mergeErr != nil {
		return m.mergeErr
	}
	m.mergeCalls = append(m.mergeCalls, branch)
	return nil
}

func (m *mockGitClient) Rebase(repoPath, branch string) error {
	if m.rebaseErr != nil {
		return m.rebaseErr
	}
	m.rebaseCalls = append(m.rebaseCalls, branch)
	return nil
}

func (m *mockGitClient) Push(worktreePath, branch string, setUpstream bool) error {
	if m.pushErr != nil {
		return m.pushErr
	}
	m.pushCalls = append(m.pushCalls, pushCall{worktreePath, branch, setUpstream})
	return nil
}

func (m *mockGitClient) Pull(repoPath string) error {
	if m.pullErr != nil {
		return m.pullErr
	}
	m.pullCalls++
	return nil
}

func (m *mockGitClient) CommitsAhead(worktreePath, baseBranch string) (int, error) {
	if m.aheadErr != nil {
		return 0, m.aheadErr
	}
	return m.commitsAhead, nil
}

func (m *mockGitClient) CommitsBehind(worktreePath, baseBranch string) (int, error) {
	if m.behindErr != nil {
		return 0, m.behindErr
	}
	return m.commitsBehind, nil
}

func (m *mockGitClient) WorktreePrune(repoPath string) error {
	if m.pruneErr != nil {
		return m.pruneErr
	}
	m.pruneCalls++
	return nil
}

// mockItermClient implements iterm.Client for testing.
type mockItermClient struct {
	running  bool
	sessions map[string]bool

	createCalls []itermCreateCall
	focusCalls  []string
	closeCalls  []string

	createErr error
}

type itermCreateCall struct {
	path, name string
	noClaude   bool
}

func (m *mockItermClient) IsRunning() bool { return m.running }
func (m *mockItermClient) EnsureRunning() error {
	m.running = true
	return nil
}
func (m *mockItermClient) CreateWorktreeWindow(path, name string, noClaude bool) (*iterm.SessionIDs, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.createCalls = append(m.createCalls, itermCreateCall{path, name, noClaude})
	return &iterm.SessionIDs{
		ClaudeSessionID: "mock-claude-session",
		ShellSessionID:  "mock-shell-session",
	}, nil
}
func (m *mockItermClient) SessionExists(sessionID string) bool {
	return m.sessions[sessionID]
}
func (m *mockItermClient) FocusWindow(sessionID string) error {
	m.focusCalls = append(m.focusCalls, sessionID)
	return nil
}
func (m *mockItermClient) CloseWindow(sessionID string) error {
	m.closeCalls = append(m.closeCalls, sessionID)
	return nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestServer(t *testing.T) (*Server, *mockGitClient, *mockItermClient, *state.Manager) {
	t.Helper()

	gc := &mockGitClient{
		repoRoot:      "/tmp/testrepo",
		repoName:      "testrepo",
		worktreesDir:  "/tmp/testrepo.worktrees",
		currentBranch: "main",
		branches:      map[string]bool{"main": true},
		worktrees: []gitops.WorktreeInfo{
			{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		},
	}
	ic := &mockItermClient{
		running:  true,
		sessions: make(map[string]bool),
	}

	stateDir := t.TempDir()
	sm := state.NewManager(filepath.Join(stateDir, "state.json"))

	srv := NewServer(gc, ic, sm)
	require.NotNil(t, srv)

	return srv, gc, ic, sm
}

func callToolReq(name string, args map[string]any) mcpgo.CallToolRequest {
	return mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}
}

func resultText(t *testing.T, result *mcpgo.CallToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, c := range result.Content {
		tc, ok := c.(mcpgo.TextContent)
		if ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Tests: MCPServer registration
// ---------------------------------------------------------------------------

func TestNewServer(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	mcpSrv := srv.MCPServer()
	require.NotNil(t, mcpSrv, "MCPServer() should return non-nil")
}

// ---------------------------------------------------------------------------
// Tests: wt_list
// ---------------------------------------------------------------------------

func TestHandleList_Success(t *testing.T) {
	srv, gc, _, sm := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		{Path: "/tmp/testrepo.worktrees/feature", Branch: "feature/login", HEAD: "def456"},
	}

	// Add state for the worktree
	require.NoError(t, sm.SetWorktree("/tmp/testrepo.worktrees/feature", &state.WorktreeState{
		Repo:   "testrepo",
		Branch: "feature/login",
	}))

	req := callToolReq("wt_list", map[string]any{"repo_path": "/tmp/testrepo"})
	result, err := srv.handleList(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	text := resultText(t, result)
	assert.Contains(t, text, "testrepo")
	assert.Contains(t, text, "feature/login")
}

func TestHandleList_MissingRepoPath(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	ctx := context.Background()

	req := callToolReq("wt_list", nil)
	result, err := srv.handleList(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestHandleList_GitError(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.worktreeListErr = fmt.Errorf("git failed")

	req := callToolReq("wt_list", map[string]any{"repo_path": "/tmp/testrepo"})
	result, err := srv.handleList(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "git failed")
}

// ---------------------------------------------------------------------------
// Tests: wt_create
// ---------------------------------------------------------------------------

func TestHandleCreate_Success(t *testing.T) {
	srv, gc, ic, _ := newTestServer(t)
	ctx := context.Background()

	req := callToolReq("wt_create", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/new-thing",
	})
	result, err := srv.handleCreate(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	text := resultText(t, result)
	assert.Contains(t, text, "feature/new-thing")

	// Verify worktree was added
	require.Len(t, gc.addedWorktrees, 1)
	assert.Equal(t, "feature/new-thing", gc.addedWorktrees[0].branch)
	assert.Equal(t, "main", gc.addedWorktrees[0].base)
	assert.True(t, gc.addedWorktrees[0].newBranch)

	// Verify iTerm window was created
	require.Len(t, ic.createCalls, 1)
}

func TestHandleCreate_WithBase(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	req := callToolReq("wt_create", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/new",
		"base":      "develop",
	})
	result, err := srv.handleCreate(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	require.Len(t, gc.addedWorktrees, 1)
	assert.Equal(t, "develop", gc.addedWorktrees[0].base)
}

func TestHandleCreate_NoClaude(t *testing.T) {
	srv, _, ic, _ := newTestServer(t)
	ctx := context.Background()

	req := callToolReq("wt_create", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/test",
		"no_claude": true,
	})
	result, err := srv.handleCreate(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	require.Len(t, ic.createCalls, 1)
	assert.True(t, ic.createCalls[0].noClaude)
}

func TestHandleCreate_ExistingBranch(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.branches["existing-branch"] = true

	req := callToolReq("wt_create", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "existing-branch",
	})
	result, err := srv.handleCreate(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	require.Len(t, gc.addedWorktrees, 1)
	assert.False(t, gc.addedWorktrees[0].newBranch, "should use existing branch")
}

func TestHandleCreate_MissingBranch(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	ctx := context.Background()

	req := callToolReq("wt_create", map[string]any{
		"repo_path": "/tmp/testrepo",
	})
	result, err := srv.handleCreate(ctx, req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandleCreate_MissingRepoPath(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	ctx := context.Background()

	req := callToolReq("wt_create", map[string]any{
		"branch": "feature/test",
	})
	result, err := srv.handleCreate(ctx, req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandleCreate_WorktreeAddError(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.worktreeAddErr = fmt.Errorf("branch already exists")

	req := callToolReq("wt_create", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/test",
	})
	result, err := srv.handleCreate(ctx, req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "branch already exists")
}

// ---------------------------------------------------------------------------
// Tests: wt_open
// ---------------------------------------------------------------------------

func TestHandleOpen_Success(t *testing.T) {
	srv, gc, ic, sm := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		{Path: "/tmp/testrepo.worktrees/feature", Branch: "feature/login", HEAD: "def456"},
	}

	// Set state so the worktree is known
	require.NoError(t, sm.SetWorktree("/tmp/testrepo.worktrees/feature", &state.WorktreeState{
		Repo:   "testrepo",
		Branch: "feature/login",
	}))

	req := callToolReq("wt_open", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/login",
	})
	result, err := srv.handleOpen(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := resultText(t, result)
	assert.Contains(t, text, "feature/login")

	// Should have created a window
	require.Len(t, ic.createCalls, 1)
}

func TestHandleOpen_MissingBranch(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	ctx := context.Background()

	req := callToolReq("wt_open", map[string]any{
		"repo_path": "/tmp/testrepo",
	})
	result, err := srv.handleOpen(ctx, req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandleOpen_WorktreeNotFound(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	// Only main worktree exists
	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
	}

	req := callToolReq("wt_open", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "nonexistent",
	})
	result, err := srv.handleOpen(ctx, req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "not found")
}

// ---------------------------------------------------------------------------
// Tests: wt_delete
// ---------------------------------------------------------------------------

func TestHandleDelete_Success(t *testing.T) {
	srv, gc, ic, sm := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		{Path: "/tmp/testrepo.worktrees/feature", Branch: "feature/login", HEAD: "def456"},
	}

	// Set state with session info
	require.NoError(t, sm.SetWorktree("/tmp/testrepo.worktrees/feature", &state.WorktreeState{
		Repo:            "testrepo",
		Branch:          "feature/login",
		ClaudeSessionID: "sess-1",
	}))
	ic.sessions["sess-1"] = true

	req := callToolReq("wt_delete", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/login",
	})
	result, err := srv.handleDelete(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// Verify worktree was removed
	require.Len(t, gc.removedWorktrees, 1)

	// Verify iTerm window was closed
	require.Len(t, ic.closeCalls, 1)
}

func TestHandleDelete_Force(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		{Path: "/tmp/testrepo.worktrees/feature", Branch: "feature/login", HEAD: "def456"},
	}
	gc.dirty = true

	req := callToolReq("wt_delete", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/login",
		"force":     true,
	})
	result, err := srv.handleDelete(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	require.Len(t, gc.removedWorktrees, 1)
	assert.True(t, gc.removedWorktrees[0].force)
}

func TestHandleDelete_DirtyWithoutForce(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		{Path: "/tmp/testrepo.worktrees/feature", Branch: "feature/login", HEAD: "def456"},
	}
	gc.dirty = true

	req := callToolReq("wt_delete", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/login",
	})
	result, err := srv.handleDelete(ctx, req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "uncommitted changes")
}

func TestHandleDelete_MissingBranch(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	ctx := context.Background()

	req := callToolReq("wt_delete", map[string]any{
		"repo_path": "/tmp/testrepo",
	})
	result, err := srv.handleDelete(ctx, req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// ---------------------------------------------------------------------------
// Tests: wt_sync
// ---------------------------------------------------------------------------

func TestHandleSync_Success(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		{Path: "/tmp/testrepo.worktrees/feature", Branch: "feature/login", HEAD: "def456"},
	}
	gc.commitsBehind = 3

	req := callToolReq("wt_sync", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/login",
	})
	result, err := srv.handleSync(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := resultText(t, result)
	assert.Contains(t, text, "feature/login")

	// Default strategy is merge
	require.Len(t, gc.mergeCalls, 1)
}

func TestHandleSync_RebaseStrategy(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		{Path: "/tmp/testrepo.worktrees/feature", Branch: "feature/login", HEAD: "def456"},
	}
	gc.commitsBehind = 2

	req := callToolReq("wt_sync", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/login",
		"strategy":  "rebase",
	})
	result, err := srv.handleSync(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	require.Len(t, gc.rebaseCalls, 1)
	assert.Empty(t, gc.mergeCalls)
}

func TestHandleSync_AlreadyInSync(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		{Path: "/tmp/testrepo.worktrees/feature", Branch: "feature/login", HEAD: "def456"},
	}
	gc.commitsBehind = 0

	req := callToolReq("wt_sync", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/login",
	})
	result, err := srv.handleSync(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := resultText(t, result)
	assert.Contains(t, text, "already in sync")
}

func TestHandleSync_MissingBranch(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	ctx := context.Background()

	req := callToolReq("wt_sync", map[string]any{
		"repo_path": "/tmp/testrepo",
	})
	result, err := srv.handleSync(ctx, req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandleSync_WithFetch(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		{Path: "/tmp/testrepo.worktrees/feature", Branch: "feature/login", HEAD: "def456"},
	}
	gc.hasRemote = true
	gc.commitsBehind = 1

	req := callToolReq("wt_sync", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/login",
	})
	result, err := srv.handleSync(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	assert.Equal(t, 1, gc.fetchCalls, "should fetch when remote exists")
}

// ---------------------------------------------------------------------------
// Tests: wt_merge
// ---------------------------------------------------------------------------

func TestHandleMerge_LocalSuccess(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		{Path: "/tmp/testrepo.worktrees/feature", Branch: "feature/login", HEAD: "def456"},
	}
	gc.hasUnpushed = true

	req := callToolReq("wt_merge", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/login",
	})
	result, err := srv.handleMerge(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := resultText(t, result)
	assert.Contains(t, text, "feature/login")

	// Should have merged into main
	require.Len(t, gc.mergeCalls, 1)
}

func TestHandleMerge_RebaseStrategy(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		{Path: "/tmp/testrepo.worktrees/feature", Branch: "feature/login", HEAD: "def456"},
	}
	gc.hasUnpushed = true

	req := callToolReq("wt_merge", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/login",
		"strategy":  "rebase",
	})
	result, err := srv.handleMerge(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// Rebase-then-ff: should rebase first, then merge
	require.Len(t, gc.rebaseCalls, 1)
	require.Len(t, gc.mergeCalls, 1)
}

func TestHandleMerge_PR(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		{Path: "/tmp/testrepo.worktrees/feature", Branch: "feature/login", HEAD: "def456"},
	}
	gc.hasUnpushed = true

	req := callToolReq("wt_merge", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/login",
		"pr":        true,
	})
	result, err := srv.handleMerge(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := resultText(t, result)
	assert.Contains(t, text, "push")

	// Should have pushed the branch
	require.Len(t, gc.pushCalls, 1)
	assert.Equal(t, "feature/login", gc.pushCalls[0].branch)
}

func TestHandleMerge_NothingToMerge(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
		{Path: "/tmp/testrepo.worktrees/feature", Branch: "feature/login", HEAD: "abc123"},
	}
	gc.hasUnpushed = false

	req := callToolReq("wt_merge", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "feature/login",
	})
	result, err := srv.handleMerge(ctx, req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := resultText(t, result)
	assert.Contains(t, text, "no commits")
}

func TestHandleMerge_MissingBranch(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	ctx := context.Background()

	req := callToolReq("wt_merge", map[string]any{
		"repo_path": "/tmp/testrepo",
	})
	result, err := srv.handleMerge(ctx, req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandleMerge_WorktreeNotFound(t *testing.T) {
	srv, gc, _, _ := newTestServer(t)
	ctx := context.Background()

	gc.worktrees = []gitops.WorktreeInfo{
		{Path: "/tmp/testrepo", Branch: "main", HEAD: "abc123"},
	}

	req := callToolReq("wt_merge", map[string]any{
		"repo_path": "/tmp/testrepo",
		"branch":    "nonexistent",
	})
	result, err := srv.handleMerge(ctx, req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// ---------------------------------------------------------------------------
// Tests: Integration -- verify all tools are registered via HandleMessage
// ---------------------------------------------------------------------------

func TestMCPIntegration_ListTools(t *testing.T) {
	srv, _, _, _ := newTestServer(t)

	mcpSrv := srv.MCPServer()
	require.NotNil(t, mcpSrv)

	ctx := context.Background()
	reqJSON := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	respMsg := mcpSrv.HandleMessage(ctx, reqJSON)
	require.NotNil(t, respMsg)

	respBytes, err := json.Marshal(respMsg)
	require.NoError(t, err)

	var rpcResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	err = json.Unmarshal(respBytes, &rpcResp)
	require.NoError(t, err)

	toolNames := make(map[string]bool)
	for _, tool := range rpcResp.Result.Tools {
		toolNames[tool.Name] = true
	}

	expectedTools := []string{
		"wt_list",
		"wt_create",
		"wt_open",
		"wt_delete",
		"wt_sync",
		"wt_merge",
	}
	for _, name := range expectedTools {
		assert.True(t, toolNames[name], "expected tool %q to be registered", name)
	}
}

// Compile-time interface checks for mocks.
var (
	_ GitClient    = (*mockGitClient)(nil)
	_ iterm.Client = (*mockItermClient)(nil)
)

// Reference mcpserver to keep the import active.
var _ = (*mcpserver.MCPServer)(nil)
