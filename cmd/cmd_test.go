package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/joescharf/wt/internal/claude"
	"github.com/joescharf/wt/internal/git"
	gitmocks "github.com/joescharf/wt/internal/git/mocks"
	"github.com/joescharf/wt/internal/iterm"
	itermmocks "github.com/joescharf/wt/internal/iterm/mocks"
	"github.com/joescharf/wt/internal/state"
	"github.com/joescharf/wt/internal/ui"
)

// testEnv sets up mocked dependencies for cmd tests and returns cleanup func.
type testEnv struct {
	git   *gitmocks.MockClient
	iterm *itermmocks.MockClient
	state *state.Manager
	claude *claude.TrustManager
	ui    *ui.UI
	out   *bytes.Buffer
	err   *bytes.Buffer
	dir   string
}

func setupTest(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)

	mockGit := gitmocks.NewMockClient(t)
	mockIterm := itermmocks.NewMockClient(t)

	statePath := filepath.Join(dir, "state.json")
	mgr := state.NewManager(statePath)

	claudePath := filepath.Join(dir, ".claude.json")
	trust := claude.NewTrustManager(claudePath)

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	u := &ui.UI{
		Out:    outBuf,
		ErrOut: errBuf,
	}

	// Replace package-level vars
	gitClient = mockGit
	itermClient = mockIterm
	stateMgr = mgr
	claudeTrust = trust
	output = u

	// Reset flags
	verbose = false
	dryRun = false
	openNoClaude = false
	createBase = ""
	createNoClaude = false
	createExisting = false
	deleteForce = false
	deleteBranchFlag = false
	deleteAll = false
	promptFunc = func(msg string) bool { return false } // default deny in tests

	// Set viper defaults for tests
	viper.Reset()
	viper.SetDefault("base_branch", "main")
	viper.SetDefault("no_claude", false)

	return &testEnv{
		git:    mockGit,
		iterm:  mockIterm,
		state:  mgr,
		claude: trust,
		ui:     u,
		out:    outBuf,
		err:    errBuf,
		dir:    dir,
	}
}

// ─── Create Tests ────────────────────────────────────────────────────────────

func TestCreate_NewBranch(t *testing.T) {
	env := setupTest(t)
	wtDir := filepath.Join(env.dir, "repo.worktrees")
	wtPath := filepath.Join(wtDir, "auth")

	env.git.EXPECT().RepoName().Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir().Return(wtDir, nil)
	env.git.EXPECT().BranchExists("feature/auth").Return(false, nil)
	env.git.EXPECT().WorktreeAdd(wtPath, "feature/auth", "main", true).
		Run(func(path, branch, base string, newBranch bool) {
			os.MkdirAll(path, 0755) // simulate worktree creation
		}).Return(nil)

	env.iterm.EXPECT().CreateWorktreeWindow(wtPath, "wt:myrepo:auth", false).
		Return(&iterm.SessionIDs{ClaudeSessionID: "c-123", ShellSessionID: "s-456"}, nil)

	err := createRun("feature/auth")
	require.NoError(t, err)

	// Verify state was written
	ws, err := env.state.GetWorktree(wtPath)
	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, "feature/auth", ws.Branch)
	assert.Equal(t, "c-123", ws.ClaudeSessionID)
	assert.Equal(t, "s-456", ws.ShellSessionID)

	assert.Contains(t, env.out.String(), "Worktree ready")
}

func TestCreate_ExistingWorktree(t *testing.T) {
	env := setupTest(t)
	wtDir := filepath.Join(env.dir, "repo.worktrees")
	wtPath := filepath.Join(wtDir, "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().RepoName().Return("myrepo", nil).Times(2) // once for create, once for open
	env.git.EXPECT().WorktreesDir().Return(wtDir, nil)
	env.git.EXPECT().ResolveWorktree("feature/auth").Return(wtPath, nil)
	env.git.EXPECT().CurrentBranch(wtPath).Return("feature/auth", nil)

	// open will be called since worktree exists
	env.iterm.EXPECT().CreateWorktreeWindow(wtPath, "wt:myrepo:auth", false).
		Return(&iterm.SessionIDs{ClaudeSessionID: "c-123", ShellSessionID: "s-456"}, nil)

	err := createRun("feature/auth")
	require.NoError(t, err)

	assert.Contains(t, env.out.String(), "Worktree already exists")
}

func TestCreate_ExistingBranch(t *testing.T) {
	env := setupTest(t)
	wtDir := filepath.Join(env.dir, "repo.worktrees")
	wtPath := filepath.Join(wtDir, "auth")

	env.git.EXPECT().RepoName().Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir().Return(wtDir, nil)
	env.git.EXPECT().BranchExists("feature/auth").Return(true, nil)
	env.git.EXPECT().WorktreeAdd(wtPath, "feature/auth", "", false).
		Run(func(path, branch, base string, newBranch bool) {
			os.MkdirAll(path, 0755)
		}).Return(nil)

	env.iterm.EXPECT().CreateWorktreeWindow(wtPath, "wt:myrepo:auth", false).
		Return(&iterm.SessionIDs{ClaudeSessionID: "c-123", ShellSessionID: "s-456"}, nil)

	err := createRun("feature/auth")
	require.NoError(t, err)

	assert.Contains(t, env.out.String(), "already exists, using it")
}

// ─── List Tests ──────────────────────────────────────────────────────────────

func TestList_WithWorktrees(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().RepoName().Return("myrepo", nil)
	env.git.EXPECT().RepoRoot().Return(env.dir, nil)
	env.git.EXPECT().WorktreeList().Return([]git.WorktreeInfo{
		{Path: env.dir, Branch: "main", HEAD: "abc123"},
		{Path: wtPath, Branch: "feature/auth", HEAD: "def456"},
	}, nil)

	// Set up state so we can check window status
	env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "c-123",
		CreatedAt:       state.FlexTime{Time: time.Now().UTC().Add(-2 * time.Hour)},
	})

	env.iterm.EXPECT().IsRunning().Return(true)
	env.iterm.EXPECT().SessionExists("c-123").Return(true)

	err := listRun()
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "feature/auth")
	assert.Contains(t, out, "open")
	assert.Contains(t, out, "2h")
}

func TestList_PrunesStaleState(t *testing.T) {
	env := setupTest(t)

	// Add state for a non-existent path
	env.state.SetWorktree("/nonexistent/path", &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "stale",
	})

	env.git.EXPECT().RepoName().Return("myrepo", nil)
	env.git.EXPECT().RepoRoot().Return(env.dir, nil)
	env.git.EXPECT().WorktreeList().Return([]git.WorktreeInfo{
		{Path: env.dir, Branch: "main", HEAD: "abc123"},
	}, nil)

	err := listRun()
	require.NoError(t, err)

	// Verify stale entry was pruned
	ws, _ := env.state.GetWorktree("/nonexistent/path")
	assert.Nil(t, ws)

	assert.Contains(t, env.out.String(), "Pruned 1 stale")
}

// ─── Switch Tests ────────────────────────────────────────────────────────────

func TestSwitch_FocusesWindow(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().ResolveWorktree("feature/auth").Return(wtPath, nil)
	env.iterm.EXPECT().EnsureRunning().Return(nil)
	env.iterm.EXPECT().SessionExists("c-123").Return(true)
	env.iterm.EXPECT().FocusWindow("c-123").Return(nil)

	env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "c-123",
	})

	err := switchRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Focused")
}

func TestSwitch_StaleSession(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().ResolveWorktree("feature/auth").Return(wtPath, nil)
	env.iterm.EXPECT().EnsureRunning().Return(nil)
	env.iterm.EXPECT().SessionExists("c-123").Return(false)

	env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "c-123",
	})

	err := switchRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.err.String(), "no longer exists")
}

// ─── Open Tests ──────────────────────────────────────────────────────────────

func TestOpen_AlreadyOpen(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().RepoName().Return("myrepo", nil)
	env.git.EXPECT().ResolveWorktree("feature/auth").Return(wtPath, nil)
	env.iterm.EXPECT().IsRunning().Return(true)
	env.iterm.EXPECT().SessionExists("c-123").Return(true)
	env.iterm.EXPECT().FocusWindow("c-123").Return(nil)

	env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "c-123",
	})

	err := openRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "already open")
}

func TestOpen_NewWindow(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().RepoName().Return("myrepo", nil)
	env.git.EXPECT().ResolveWorktree("feature/auth").Return(wtPath, nil)
	env.git.EXPECT().CurrentBranch(wtPath).Return("feature/auth", nil)

	env.iterm.EXPECT().CreateWorktreeWindow(wtPath, "wt:myrepo:auth", false).
		Return(&iterm.SessionIDs{ClaudeSessionID: "c-new", ShellSessionID: "s-new"}, nil)

	err := openRun("feature/auth")
	require.NoError(t, err)

	ws, _ := env.state.GetWorktree(wtPath)
	require.NotNil(t, ws)
	assert.Equal(t, "c-new", ws.ClaudeSessionID)
	assert.Contains(t, env.out.String(), "window opened")
}

func TestOpen_NotFoundPromptAccepted(t *testing.T) {
	env := setupTest(t)
	promptFunc = func(msg string) bool { return true }

	wtDir := filepath.Join(env.dir, "repo.worktrees")
	wtPath := filepath.Join(wtDir, "feat-mkdocs")

	// openRun calls RepoName + ResolveWorktree, then createRun calls RepoName + WorktreesDir
	env.git.EXPECT().RepoName().Return("myrepo", nil).Times(2)
	env.git.EXPECT().ResolveWorktree("feat-mkdocs").Return(wtPath, nil)
	env.git.EXPECT().WorktreesDir().Return(wtDir, nil)
	env.git.EXPECT().BranchExists("feat-mkdocs").Return(false, nil)
	env.git.EXPECT().WorktreeAdd(wtPath, "feat-mkdocs", "main", true).
		Run(func(path, branch, base string, newBranch bool) {
			os.MkdirAll(path, 0755)
		}).Return(nil)

	env.iterm.EXPECT().CreateWorktreeWindow(wtPath, "wt:myrepo:feat-mkdocs", false).
		Return(&iterm.SessionIDs{ClaudeSessionID: "c-123", ShellSessionID: "s-456"}, nil)

	err := openRun("feat-mkdocs")
	require.NoError(t, err)

	assert.Contains(t, env.err.String(), "not found")
	assert.Contains(t, env.out.String(), "Worktree ready")
}

func TestOpen_NotFoundPromptDenied(t *testing.T) {
	env := setupTest(t)
	promptFunc = func(msg string) bool { return false }

	wtDir := filepath.Join(env.dir, "repo.worktrees")
	wtPath := filepath.Join(wtDir, "feat-mkdocs")

	env.git.EXPECT().RepoName().Return("myrepo", nil)
	env.git.EXPECT().ResolveWorktree("feat-mkdocs").Return(wtPath, nil)

	err := openRun("feat-mkdocs")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree not found")
	assert.Contains(t, env.err.String(), "not found")
}

func TestOpen_BareShorthand(t *testing.T) {
	// wt <branch> delegates to openRun
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().RepoName().Return("myrepo", nil)
	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)
	env.git.EXPECT().CurrentBranch(wtPath).Return("feature/auth", nil)

	env.iterm.EXPECT().CreateWorktreeWindow(wtPath, "wt:myrepo:auth", false).
		Return(&iterm.SessionIDs{ClaudeSessionID: "c-new", ShellSessionID: "s-new"}, nil)

	// This simulates what root RunE does
	err := openRun("auth")
	require.NoError(t, err)
}

// ─── Delete Tests ────────────────────────────────────────────────────────────

func TestDelete_FullCleanup(t *testing.T) {
	env := setupTest(t)
	deleteBranchFlag = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().ResolveWorktree("feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(false, nil)
	env.git.EXPECT().WorktreeRemove(wtPath, false).
		Run(func(path string, force bool) {
			os.RemoveAll(path)
		}).Return(nil)
	env.git.EXPECT().BranchDelete("feature/auth", false).Return(nil)

	env.iterm.EXPECT().IsRunning().Return(true)
	env.iterm.EXPECT().SessionExists("c-123").Return(true)
	env.iterm.EXPECT().CloseWindow("c-123").Return(nil)

	env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "c-123",
	})

	err := deleteRun("feature/auth")
	require.NoError(t, err)

	// Verify state was removed
	ws, _ := env.state.GetWorktree(wtPath)
	assert.Nil(t, ws)

	assert.Contains(t, env.out.String(), "removed")
}

func TestDelete_Force(t *testing.T) {
	env := setupTest(t)
	deleteForce = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	// --force skips safety checks (no IsWorktreeDirty/HasUnpushedCommits calls)
	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)
	env.git.EXPECT().WorktreeRemove(wtPath, true).
		Run(func(path string, force bool) {
			os.RemoveAll(path)
		}).Return(nil)

	err := deleteRun("auth")
	require.NoError(t, err)
}

func TestDelete_SafeCleanWorktree(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(false, nil)
	env.git.EXPECT().WorktreeRemove(wtPath, false).
		Run(func(path string, force bool) { os.RemoveAll(path) }).Return(nil)

	err := deleteRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "removed")
}

func TestDelete_DirtyWorktreePromptDenied(t *testing.T) {
	env := setupTest(t)
	promptFunc = func(msg string) bool { return false }

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(true, nil)

	err := deleteRun("auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aborted")
}

func TestDelete_DirtyWorktreePromptAccepted(t *testing.T) {
	env := setupTest(t)
	promptFunc = func(msg string) bool { return true }

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(true, nil)
	env.git.EXPECT().WorktreeRemove(wtPath, false).
		Run(func(path string, force bool) { os.RemoveAll(path) }).Return(nil)

	err := deleteRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "removed")
}

func TestDelete_UnpushedCommitsPromptDenied(t *testing.T) {
	env := setupTest(t)
	promptFunc = func(msg string) bool { return false }

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)

	err := deleteRun("auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aborted")
}

func TestDelete_All(t *testing.T) {
	env := setupTest(t)
	deleteAll = true
	deleteForce = true // skip prompts for simplicity

	wtPath1 := filepath.Join(env.dir, "repo.worktrees", "auth")
	wtPath2 := filepath.Join(env.dir, "repo.worktrees", "api")
	os.MkdirAll(wtPath1, 0755)
	os.MkdirAll(wtPath2, 0755)

	env.git.EXPECT().RepoRoot().Return(env.dir, nil)
	env.git.EXPECT().WorktreeList().Return([]git.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
		{Path: wtPath1, Branch: "feature/auth"},
		{Path: wtPath2, Branch: "feature/api"},
	}, nil)
	env.git.EXPECT().WorktreeRemove(wtPath1, true).
		Run(func(path string, force bool) { os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().WorktreeRemove(wtPath2, true).
		Run(func(path string, force bool) { os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().WorktreePrune().Return(nil)

	err := deleteAllRun()
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Deleted 2 worktrees")
}

func TestDelete_All_NoneFound(t *testing.T) {
	env := setupTest(t)
	deleteAll = true

	env.git.EXPECT().RepoRoot().Return(env.dir, nil)
	env.git.EXPECT().WorktreeList().Return([]git.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
	}, nil)

	err := deleteAllRun()
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "No worktrees")
}

// ─── Dry-Run Tests ───────────────────────────────────────────────────────────

func TestDryRun_Create(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	wtDir := filepath.Join(env.dir, "repo.worktrees")

	env.git.EXPECT().RepoName().Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir().Return(wtDir, nil)
	env.git.EXPECT().BranchExists("feature/dry").Return(false, nil)

	err := createRun("feature/dry")
	require.NoError(t, err)

	// Verify no side effects
	assert.NoDirExists(t, filepath.Join(wtDir, "dry"))
	ws, _ := env.state.GetWorktree(filepath.Join(wtDir, "dry"))
	assert.Nil(t, ws)

	assert.Contains(t, env.err.String(), "DRY-RUN")
}

func TestDryRun_Open(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().RepoName().Return("myrepo", nil)
	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)

	err := openRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.err.String(), "DRY-RUN")
}

func TestDryRun_Switch(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)

	env.state.SetWorktree(wtPath, &state.WorktreeState{
		ClaudeSessionID: "c-123",
	})

	err := switchRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.err.String(), "DRY-RUN")
}

// ─── Prune Tests ─────────────────────────────────────────────────────────────

func TestPrune_CleansStaleState(t *testing.T) {
	env := setupTest(t)

	// Add state for a non-existent path
	env.state.SetWorktree("/nonexistent/path", &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "stale",
	})

	wtDir := filepath.Join(env.dir, "repo.worktrees")
	env.git.EXPECT().WorktreesDir().Return(wtDir, nil)
	env.git.EXPECT().WorktreePrune().Return(nil)

	err := pruneRun()
	require.NoError(t, err)

	// Verify stale entry was pruned
	ws, _ := env.state.GetWorktree("/nonexistent/path")
	assert.Nil(t, ws)
	assert.Contains(t, env.out.String(), "Pruned 1 stale")
}

func TestPrune_NothingToClean(t *testing.T) {
	env := setupTest(t)

	wtDir := filepath.Join(env.dir, "repo.worktrees")
	env.git.EXPECT().WorktreesDir().Return(wtDir, nil)
	env.git.EXPECT().WorktreePrune().Return(nil)

	err := pruneRun()
	require.NoError(t, err)

	assert.Contains(t, env.out.String(), "clean")
}

func TestPrune_DryRun(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	env.state.SetWorktree("/nonexistent/path", &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "stale",
	})

	wtDir := filepath.Join(env.dir, "repo.worktrees")
	env.git.EXPECT().WorktreesDir().Return(wtDir, nil)

	// WorktreePrune should NOT be called in dry-run

	err := pruneRun()
	require.NoError(t, err)

	assert.Contains(t, env.err.String(), "DRY-RUN")
}

func TestDryRun_Delete(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(false, nil)

	// Should not call WorktreeRemove
	_ = mock.Anything

	err := deleteRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.err.String(), "DRY-RUN")
	assert.DirExists(t, wtPath) // dir not removed
}

// --- Claude Trust Tests ---

func TestCreate_SetsClaudeTrust(t *testing.T) {
	env := setupTest(t)
	wtDir := filepath.Join(env.dir, "repo.worktrees")
	wtPath := filepath.Join(wtDir, "auth")

	env.git.EXPECT().RepoName().Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir().Return(wtDir, nil)
	env.git.EXPECT().BranchExists("feature/auth").Return(false, nil)
	env.git.EXPECT().WorktreeAdd(wtPath, "feature/auth", "main", true).
		Run(func(path, branch, base string, newBranch bool) {
			os.MkdirAll(path, 0755)
		}).Return(nil)

	env.iterm.EXPECT().CreateWorktreeWindow(wtPath, "wt:myrepo:auth", false).
		Return(&iterm.SessionIDs{ClaudeSessionID: "c-123", ShellSessionID: "s-456"}, nil)

	err := createRun("feature/auth")
	require.NoError(t, err)

	// Verify Claude trust was set
	added, err := env.claude.TrustProject(wtPath)
	require.NoError(t, err)
	assert.False(t, added, "should already be trusted")
}

func TestCreate_DryRun_DoesNotSetTrust(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	wtDir := filepath.Join(env.dir, "repo.worktrees")

	env.git.EXPECT().RepoName().Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir().Return(wtDir, nil)
	env.git.EXPECT().BranchExists("feature/dry").Return(false, nil)

	err := createRun("feature/dry")
	require.NoError(t, err)

	// Verify no Claude trust was set (file should not exist)
	_, statErr := os.Stat(env.claude.Path())
	assert.True(t, os.IsNotExist(statErr), "claude.json should not be created in dry-run")
}

func TestDelete_RemovesClaudeTrust(t *testing.T) {
	env := setupTest(t)

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	// Pre-set Claude trust
	added, err := env.claude.TrustProject(wtPath)
	require.NoError(t, err)
	require.True(t, added)

	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(false, nil)
	env.git.EXPECT().WorktreeRemove(wtPath, false).
		Run(func(path string, force bool) { os.RemoveAll(path) }).Return(nil)

	err = deleteRun("auth")
	require.NoError(t, err)

	// Verify Claude trust was removed - re-trusting should return true (added)
	added, err = env.claude.TrustProject(wtPath)
	require.NoError(t, err)
	assert.True(t, added, "trust should have been removed by delete")
}

func TestPrune_CleansOrphanedTrust(t *testing.T) {
	env := setupTest(t)

	wtDir := filepath.Join(env.dir, "repo.worktrees")
	existingPath := filepath.Join(wtDir, "auth")
	stalePath := filepath.Join(wtDir, "stale-branch")

	// Create only the existing path
	os.MkdirAll(existingPath, 0755)

	// Trust both
	_, err := env.claude.TrustProject(existingPath)
	require.NoError(t, err)
	_, err = env.claude.TrustProject(stalePath)
	require.NoError(t, err)

	env.git.EXPECT().WorktreesDir().Return(wtDir, nil)
	env.git.EXPECT().WorktreePrune().Return(nil)

	err = pruneRun()
	require.NoError(t, err)

	// Verify stale entry was pruned - re-trusting should return true (added)
	added, err := env.claude.TrustProject(stalePath)
	require.NoError(t, err)
	assert.True(t, added, "stale trust should have been pruned")

	// Verify existing entry preserved
	added, err = env.claude.TrustProject(existingPath)
	require.NoError(t, err)
	assert.False(t, added, "existing trust should be preserved")

	assert.Contains(t, env.out.String(), "Pruned 1 stale Claude trust")
}
