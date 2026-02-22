package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joescharf/wt/pkg/claude"
	gmocks "github.com/joescharf/wt/pkg/gitops/mocks"
	"github.com/joescharf/wt/pkg/iterm"
	imocks "github.com/joescharf/wt/pkg/iterm/mocks"
	state "github.com/joescharf/wt/pkg/wtstate"
)

// testLogger captures log output for assertions.
type testLogger struct {
	infos     []string
	successes []string
	warnings  []string
	verboses  []string
}

func (l *testLogger) Info(format string, args ...interface{})    { l.infos = append(l.infos, fmt.Sprintf(format, args...)) }
func (l *testLogger) Success(format string, args ...interface{}) { l.successes = append(l.successes, fmt.Sprintf(format, args...)) }
func (l *testLogger) Warning(format string, args ...interface{}) { l.warnings = append(l.warnings, fmt.Sprintf(format, args...)) }
func (l *testLogger) Verbose(format string, args ...interface{}) { l.verboses = append(l.verboses, fmt.Sprintf(format, args...)) }

// setupManager creates a Manager with mocks and a real state manager in a temp dir.
func setupManager(t *testing.T) (*Manager, *gmocks.MockClient, *imocks.MockClient, *state.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir) // macOS /var -> /private/var

	mg := gmocks.NewMockClient(t)
	mi := imocks.NewMockClient(t)
	sm := state.NewManager(filepath.Join(dir, "state.json"))
	log := &testLogger{}

	m := NewManager(mg, mi, sm, nil, log)
	return m, mg, mi, sm, dir
}

// --- Create Tests ---

func TestCreate_NewBranch(t *testing.T) {
	m, mg, mi, sm, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtDir := repoPath + ".worktrees"

	mg.EXPECT().RepoName(repoPath).Return("myrepo", nil)
	mg.EXPECT().WorktreesDir(repoPath).Return(wtDir, nil)
	mg.EXPECT().BranchExists(repoPath, "feature/auth").Return(false, nil)
	mg.EXPECT().WorktreeAdd(repoPath, filepath.Join(wtDir, "auth"), "feature/auth", "main", true).Return(nil)
	mi.EXPECT().CreateWorktreeWindow(filepath.Join(wtDir, "auth"), "wt:myrepo:auth", false).
		Return(&iterm.SessionIDs{ClaudeSessionID: "claude-123", ShellSessionID: "shell-456"}, nil)

	result, err := m.Create(CreateOptions{
		RepoPath:   repoPath,
		Branch:     "feature/auth",
		BaseBranch: "main",
	})

	require.NoError(t, err)
	assert.True(t, result.Created)
	assert.Equal(t, filepath.Join(wtDir, "auth"), result.WtPath)
	assert.Equal(t, "myrepo", result.RepoName)
	assert.Equal(t, "claude-123", result.SessionID)

	// Verify state was saved
	ws, err := sm.GetWorktree(filepath.Join(wtDir, "auth"))
	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, "feature/auth", ws.Branch)
	assert.Equal(t, "myrepo", ws.Repo)
	assert.Equal(t, "claude-123", ws.ClaudeSessionID)
}

func TestCreate_ExistingBranch(t *testing.T) {
	m, mg, mi, _, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtDir := repoPath + ".worktrees"

	mg.EXPECT().RepoName(repoPath).Return("myrepo", nil)
	mg.EXPECT().WorktreesDir(repoPath).Return(wtDir, nil)
	mg.EXPECT().BranchExists(repoPath, "feature/auth").Return(true, nil)
	mg.EXPECT().WorktreeAdd(repoPath, filepath.Join(wtDir, "auth"), "feature/auth", "", false).Return(nil)
	mi.EXPECT().CreateWorktreeWindow(filepath.Join(wtDir, "auth"), "wt:myrepo:auth", false).
		Return(&iterm.SessionIDs{ClaudeSessionID: "c1", ShellSessionID: "s1"}, nil)

	result, err := m.Create(CreateOptions{
		RepoPath:   repoPath,
		Branch:     "feature/auth",
		BaseBranch: "main",
	})

	require.NoError(t, err)
	assert.True(t, result.Created)
}

func TestCreate_AlreadyExists_DelegatesToOpen(t *testing.T) {
	m, mg, mi, _, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtDir := repoPath + ".worktrees"

	// Create the worktree directory to trigger delegation
	wtPath := filepath.Join(wtDir, "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	mg.EXPECT().RepoName(repoPath).Return("myrepo", nil).Times(2)
	mg.EXPECT().WorktreesDir(repoPath).Return(wtDir, nil)
	// Open path — no existing session
	mi.EXPECT().CreateWorktreeWindow(wtPath, "wt:myrepo:auth", false).
		Return(&iterm.SessionIDs{ClaudeSessionID: "c1", ShellSessionID: "s1"}, nil)
	mg.EXPECT().CurrentBranch(wtPath).Return("feature/auth", nil)

	result, err := m.Create(CreateOptions{
		RepoPath:   repoPath,
		Branch:     "feature/auth",
		BaseBranch: "main",
	})

	require.NoError(t, err)
	assert.False(t, result.Created)
}

func TestCreate_DryRun(t *testing.T) {
	m, mg, _, _, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtDir := repoPath + ".worktrees"

	mg.EXPECT().RepoName(repoPath).Return("myrepo", nil)
	mg.EXPECT().WorktreesDir(repoPath).Return(wtDir, nil)
	mg.EXPECT().BranchExists(repoPath, "feature/auth").Return(false, nil)
	// Should NOT call WorktreeAdd or CreateWorktreeWindow

	result, err := m.Create(CreateOptions{
		RepoPath:   repoPath,
		Branch:     "feature/auth",
		BaseBranch: "main",
		DryRun:     true,
	})

	require.NoError(t, err)
	assert.False(t, result.Created)
	assert.Equal(t, filepath.Join(wtDir, "auth"), result.WtPath)
}

func TestCreate_NoClaude(t *testing.T) {
	m, mg, mi, _, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtDir := repoPath + ".worktrees"

	mg.EXPECT().RepoName(repoPath).Return("myrepo", nil)
	mg.EXPECT().WorktreesDir(repoPath).Return(wtDir, nil)
	mg.EXPECT().BranchExists(repoPath, "feature/auth").Return(false, nil)
	mg.EXPECT().WorktreeAdd(repoPath, filepath.Join(wtDir, "auth"), "feature/auth", "main", true).Return(nil)
	mi.EXPECT().CreateWorktreeWindow(filepath.Join(wtDir, "auth"), "wt:myrepo:auth", true). // noClaude=true
		Return(&iterm.SessionIDs{ClaudeSessionID: "c1", ShellSessionID: "s1"}, nil)

	result, err := m.Create(CreateOptions{
		RepoPath:   repoPath,
		Branch:     "feature/auth",
		BaseBranch: "main",
		NoClaude:   true,
	})

	require.NoError(t, err)
	assert.True(t, result.Created)
}

func TestCreate_ITermFails_WorktreeStillCreated(t *testing.T) {
	m, mg, mi, _, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtDir := repoPath + ".worktrees"

	mg.EXPECT().RepoName(repoPath).Return("myrepo", nil)
	mg.EXPECT().WorktreesDir(repoPath).Return(wtDir, nil)
	mg.EXPECT().BranchExists(repoPath, "feature/auth").Return(false, nil)
	mg.EXPECT().WorktreeAdd(repoPath, filepath.Join(wtDir, "auth"), "feature/auth", "main", true).Return(nil)
	mi.EXPECT().CreateWorktreeWindow(filepath.Join(wtDir, "auth"), "wt:myrepo:auth", false).
		Return(nil, fmt.Errorf("osascript failed"))

	result, err := m.Create(CreateOptions{
		RepoPath:   repoPath,
		Branch:     "feature/auth",
		BaseBranch: "main",
	})

	require.NoError(t, err) // Should NOT error — worktree was created
	assert.True(t, result.Created)
	assert.Empty(t, result.SessionID)
}

func TestCreate_WithTrust(t *testing.T) {
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	repoPath := filepath.Join(dir, "repo")
	wtDir := repoPath + ".worktrees"

	mg := gmocks.NewMockClient(t)
	mi := imocks.NewMockClient(t)
	sm := state.NewManager(filepath.Join(dir, "state.json"))
	trustPath := filepath.Join(dir, "claude.json")
	trust := claude.NewTrustManager(trustPath)
	log := &testLogger{}

	m := NewManager(mg, mi, sm, trust, log)

	mg.EXPECT().RepoName(repoPath).Return("myrepo", nil)
	mg.EXPECT().WorktreesDir(repoPath).Return(wtDir, nil)
	mg.EXPECT().BranchExists(repoPath, "feature/auth").Return(false, nil)
	mg.EXPECT().WorktreeAdd(repoPath, filepath.Join(wtDir, "auth"), "feature/auth", "main", true).Return(nil)
	mi.EXPECT().CreateWorktreeWindow(filepath.Join(wtDir, "auth"), "wt:myrepo:auth", false).
		Return(&iterm.SessionIDs{ClaudeSessionID: "c1", ShellSessionID: "s1"}, nil)

	_, err := m.Create(CreateOptions{
		RepoPath:   repoPath,
		Branch:     "feature/auth",
		BaseBranch: "main",
	})
	require.NoError(t, err)

	// Trust file should exist with the worktree path
	_, statErr := os.Stat(trustPath)
	assert.NoError(t, statErr)
}

// --- Open Tests ---

func TestOpen_NewWindow(t *testing.T) {
	m, mg, mi, _, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtPath := filepath.Join(dir, "wt", "auth")

	mg.EXPECT().RepoName(repoPath).Return("myrepo", nil)
	mg.EXPECT().CurrentBranch(wtPath).Return("feature/auth", nil)
	mi.EXPECT().CreateWorktreeWindow(wtPath, "wt:myrepo:auth", false).
		Return(&iterm.SessionIDs{ClaudeSessionID: "c1", ShellSessionID: "s1"}, nil)

	result, err := m.Open(OpenOptions{
		RepoPath: repoPath,
		WtPath:   wtPath,
		Branch:   "auth",
	})

	require.NoError(t, err)
	assert.False(t, result.Focused)
	assert.Equal(t, "c1", result.SessionID)
	assert.Equal(t, "feature/auth", result.Branch) // resolved from git
}

func TestOpen_FocusExistingWindow(t *testing.T) {
	m, mg, mi, sm, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtPath := filepath.Join(dir, "wt", "auth")

	// Pre-populate state with existing session
	require.NoError(t, sm.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "existing-session",
	}))

	mg.EXPECT().RepoName(repoPath).Return("myrepo", nil)
	mi.EXPECT().IsRunning().Return(true)
	mi.EXPECT().SessionExists("existing-session").Return(true)
	mi.EXPECT().FocusWindow("existing-session").Return(nil)

	result, err := m.Open(OpenOptions{
		RepoPath: repoPath,
		WtPath:   wtPath,
		Branch:   "feature/auth",
	})

	require.NoError(t, err)
	assert.True(t, result.Focused)
	assert.Equal(t, "existing-session", result.SessionID)
}

func TestOpen_StaleSession_CreatesNew(t *testing.T) {
	m, mg, mi, sm, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtPath := filepath.Join(dir, "wt", "auth")

	// Pre-populate state with stale session
	require.NoError(t, sm.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "stale-session",
	}))

	mg.EXPECT().RepoName(repoPath).Return("myrepo", nil)
	mi.EXPECT().IsRunning().Return(true)
	mi.EXPECT().SessionExists("stale-session").Return(false) // session gone
	mi.EXPECT().CreateWorktreeWindow(wtPath, "wt:myrepo:auth", false).
		Return(&iterm.SessionIDs{ClaudeSessionID: "new-session", ShellSessionID: "s2"}, nil)

	result, err := m.Open(OpenOptions{
		RepoPath: repoPath,
		WtPath:   wtPath,
		Branch:   "auth",
	})

	require.NoError(t, err)
	assert.False(t, result.Focused)
	assert.Equal(t, "new-session", result.SessionID)
}

func TestOpen_DryRun(t *testing.T) {
	m, mg, _, _, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtPath := filepath.Join(dir, "wt", "auth")

	mg.EXPECT().RepoName(repoPath).Return("myrepo", nil)
	// Should NOT call CreateWorktreeWindow

	result, err := m.Open(OpenOptions{
		RepoPath: repoPath,
		WtPath:   wtPath,
		Branch:   "auth",
		DryRun:   true,
	})

	require.NoError(t, err)
	assert.Empty(t, result.SessionID)
}

// --- Delete Tests ---

func TestDelete_FullCleanup(t *testing.T) {
	m, mg, mi, sm, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtPath := filepath.Join(dir, "wt", "auth")

	// Pre-populate state
	require.NoError(t, sm.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "claude-123",
	}))

	mi.EXPECT().IsRunning().Return(true)
	mi.EXPECT().SessionExists("claude-123").Return(true)
	mi.EXPECT().CloseWindow("claude-123").Return(nil)
	mg.EXPECT().WorktreeRemove(repoPath, wtPath, false).Return(nil)
	mg.EXPECT().BranchDelete(repoPath, "feature/auth", false).Return(nil)

	err := m.Delete(DeleteOptions{
		RepoPath:     repoPath,
		WtPath:       wtPath,
		Branch:       "feature/auth",
		DeleteBranch: true,
	})

	require.NoError(t, err)

	// Verify state was removed
	ws, err := sm.GetWorktree(wtPath)
	require.NoError(t, err)
	assert.Nil(t, ws)
}

func TestDelete_NoBranchDelete(t *testing.T) {
	m, mg, _, sm, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtPath := filepath.Join(dir, "wt", "auth")

	mg.EXPECT().WorktreeRemove(repoPath, wtPath, false).Return(nil)
	// Should NOT call BranchDelete

	err := m.Delete(DeleteOptions{
		RepoPath:     repoPath,
		WtPath:       wtPath,
		Branch:       "feature/auth",
		DeleteBranch: false,
	})

	require.NoError(t, err)

	// Verify state was removed
	ws, _ := sm.GetWorktree(wtPath)
	assert.Nil(t, ws)
}

func TestDelete_Force_BranchFallback(t *testing.T) {
	m, mg, _, _, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtPath := filepath.Join(dir, "wt", "auth")

	mg.EXPECT().WorktreeRemove(repoPath, wtPath, true).Return(nil)
	mg.EXPECT().BranchDelete(repoPath, "feature/auth", false).Return(fmt.Errorf("not fully merged"))
	mg.EXPECT().BranchDelete(repoPath, "feature/auth", true).Return(nil) // force fallback

	err := m.Delete(DeleteOptions{
		RepoPath:     repoPath,
		WtPath:       wtPath,
		Branch:       "feature/auth",
		Force:        true,
		DeleteBranch: true,
	})

	require.NoError(t, err)
}

func TestDelete_BranchFromState(t *testing.T) {
	m, mg, _, sm, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtPath := filepath.Join(dir, "wt", "auth")

	// State has the real branch name
	require.NoError(t, sm.SetWorktree(wtPath, &state.WorktreeState{
		Branch: "feature/auth",
	}))

	mg.EXPECT().WorktreeRemove(repoPath, wtPath, false).Return(nil)
	mg.EXPECT().BranchDelete(repoPath, "feature/auth", false).Return(nil) // uses state branch, not opts.Branch

	err := m.Delete(DeleteOptions{
		RepoPath:     repoPath,
		WtPath:       wtPath,
		Branch:       "auth", // short name
		DeleteBranch: true,
	})

	require.NoError(t, err)
}

func TestDelete_DryRun(t *testing.T) {
	m, _, _, sm, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtPath := filepath.Join(dir, "wt", "auth")

	// Pre-populate state
	require.NoError(t, sm.SetWorktree(wtPath, &state.WorktreeState{
		Branch: "feature/auth",
	}))

	// Should NOT call WorktreeRemove, BranchDelete, or RemoveWorktree
	err := m.Delete(DeleteOptions{
		RepoPath:     repoPath,
		WtPath:       wtPath,
		Branch:       "feature/auth",
		DeleteBranch: true,
		DryRun:       true,
	})

	require.NoError(t, err)

	// State should still exist in dry-run
	ws, _ := sm.GetWorktree(wtPath)
	assert.NotNil(t, ws)
}

func TestDelete_WithTrust(t *testing.T) {
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	repoPath := filepath.Join(dir, "repo")
	wtPath := filepath.Join(dir, "wt", "auth")

	mg := gmocks.NewMockClient(t)
	mi := imocks.NewMockClient(t)
	sm := state.NewManager(filepath.Join(dir, "state.json"))
	trustPath := filepath.Join(dir, "claude.json")
	trust := claude.NewTrustManager(trustPath)
	log := &testLogger{}

	m := NewManager(mg, mi, sm, trust, log)

	// First trust the project
	_, err := trust.TrustProject(wtPath)
	require.NoError(t, err)

	mg.EXPECT().WorktreeRemove(repoPath, wtPath, false).Return(nil)

	err = m.Delete(DeleteOptions{
		RepoPath: repoPath,
		WtPath:   wtPath,
		Branch:   "feature/auth",
	})

	require.NoError(t, err)
}

func TestDelete_ITermNotRunning(t *testing.T) {
	m, mg, mi, sm, dir := setupManager(t)
	repoPath := filepath.Join(dir, "repo")
	wtPath := filepath.Join(dir, "wt", "auth")

	require.NoError(t, sm.SetWorktree(wtPath, &state.WorktreeState{
		ClaudeSessionID: "claude-123",
	}))

	mi.EXPECT().IsRunning().Return(false) // iTerm not running
	// Should NOT call SessionExists or CloseWindow
	mg.EXPECT().WorktreeRemove(repoPath, wtPath, false).Return(nil)

	err := m.Delete(DeleteOptions{
		RepoPath: repoPath,
		WtPath:   wtPath,
		Branch:   "feature/auth",
	})

	require.NoError(t, err)
}
