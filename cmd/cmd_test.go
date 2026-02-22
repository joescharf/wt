package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/joescharf/wt/pkg/claude"
	"github.com/joescharf/wt/pkg/gitops"
	gitmocks "github.com/joescharf/wt/pkg/gitops/mocks"
	"github.com/joescharf/wt/pkg/iterm"
	itermmocks "github.com/joescharf/wt/pkg/iterm/mocks"
	"github.com/joescharf/wt/pkg/lifecycle"
	state "github.com/joescharf/wt/pkg/wtstate"
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
	repoRoot = dir
	opsLogger = &uiLogger{u: u}
	lcMgr = lifecycle.NewManager(mockGit, mockIterm, mgr, trust, &uiLogger{u: u})

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
	mergePR = false
	mergeNoCleanup = false
	mergeBase = ""
	mergeTitle = ""
	mergeBody = ""
	mergeDraft = false
	mergeForce = false
	syncBase = ""
	syncForce = false
	syncAll = false
	syncRebase = false
	syncMerge = false
	mergeRebase = false
	mergeMerge = false
	discoverAdopt = false
	configForce = false
	configDirFunc = defaultConfigDir
	promptFunc = func(msg string) bool { return false } // default deny in tests

	// Set viper defaults for tests
	viper.Reset()
	viper.SetDefault("base_branch", "main")
	viper.SetDefault("no_claude", false)
	viper.SetDefault("rebase", false)

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

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
	env.git.EXPECT().BranchExists(mock.Anything, "feature/auth").Return(false, nil)
	env.git.EXPECT().WorktreeAdd(mock.Anything, wtPath, "feature/auth", "main", true).
		Run(func(repoPath, path, branch, base string, newBranch bool) {
			_ = os.MkdirAll(path, 0755) // simulate worktree creation
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
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	// lifecycle.Create calls: RepoName, WorktreesDir, detects dir exists, delegates to Open
	// lifecycle.Open calls: RepoName, then CreateWorktreeWindow (no existing session)
	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil).Times(2) // once for create, once for open
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
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

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
	env.git.EXPECT().BranchExists(mock.Anything, "feature/auth").Return(true, nil)
	env.git.EXPECT().WorktreeAdd(mock.Anything, wtPath, "feature/auth", "", false).
		Run(func(repoPath, path, branch, base string, newBranch bool) {
			_ = os.MkdirAll(path, 0755)
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
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(filepath.Join(env.dir, "repo.worktrees"), nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main", HEAD: "abc123"},
		{Path: wtPath, Branch: "feature/auth", HEAD: "def456"},
	}, nil)

	// Set up state so we can check window status
	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "c-123",
		CreatedAt:       state.FlexTime{Time: time.Now().UTC().Add(-2 * time.Hour)},
	}))

	env.iterm.EXPECT().IsRunning().Return(true)
	env.iterm.EXPECT().SessionExists("c-123").Return(true)

	// Git status checks
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(2, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(0, nil)

	err := listRun()
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "feature/auth")
	assert.Contains(t, out, "open")
	assert.Contains(t, out, "↑2")
	assert.Contains(t, out, "2h")
}

func TestList_StatusDirty(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(filepath.Join(env.dir, "repo.worktrees"), nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main", HEAD: "abc123"},
		{Path: wtPath, Branch: "feature/auth", HEAD: "def456"},
	}, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(true, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(0, nil)

	err := listRun()
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "dirty")
}

func TestList_StatusDirtyAndBehind(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(filepath.Join(env.dir, "repo.worktrees"), nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main", HEAD: "abc123"},
		{Path: wtPath, Branch: "feature/auth", HEAD: "def456"},
	}, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(true, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(4, nil)

	err := listRun()
	require.NoError(t, err)
	out := env.out.String()
	assert.Contains(t, out, "dirty")
	assert.Contains(t, out, "↓4")
}

func TestList_StatusClean(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(filepath.Join(env.dir, "repo.worktrees"), nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main", HEAD: "abc123"},
		{Path: wtPath, Branch: "feature/auth", HEAD: "def456"},
	}, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(0, nil)

	err := listRun()
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "clean")
}

func TestList_StatusBehind(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(filepath.Join(env.dir, "repo.worktrees"), nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main", HEAD: "abc123"},
		{Path: wtPath, Branch: "feature/auth", HEAD: "def456"},
	}, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(5, nil)

	err := listRun()
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "↓5")
}

func TestList_StatusAheadAndBehind(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(filepath.Join(env.dir, "repo.worktrees"), nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main", HEAD: "abc123"},
		{Path: wtPath, Branch: "feature/auth", HEAD: "def456"},
	}, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(2, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(3, nil)

	err := listRun()
	require.NoError(t, err)
	out := env.out.String()
	assert.Contains(t, out, "↑2")
	assert.Contains(t, out, "↓3")
}

func TestList_PrunesStaleState(t *testing.T) {
	env := setupTest(t)

	// Add state for a non-existent path
	require.NoError(t, env.state.SetWorktree("/nonexistent/path", &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "stale",
	}))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(filepath.Join(env.dir, "repo.worktrees"), nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main", HEAD: "abc123"},
	}, nil)

	err := listRun()
	require.NoError(t, err)

	// Verify stale entry was pruned
	ws, _ := env.state.GetWorktree("/nonexistent/path")
	assert.Nil(t, ws)

	assert.Contains(t, env.out.String(), "Pruned 1 stale")
}

func TestList_StatusRebasing(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(filepath.Join(env.dir, "repo.worktrees"), nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main", HEAD: "abc123"},
		{Path: wtPath, Branch: "feature/auth", HEAD: "def456"},
	}, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(true, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(1, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(2, nil)

	err := listRun()
	require.NoError(t, err)
	out := env.out.String()
	assert.Contains(t, out, "rebasing")
	assert.Contains(t, out, "dirty")
	assert.Contains(t, out, "↑1")
	assert.Contains(t, out, "↓2")
}

func TestList_StatusMerging(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(filepath.Join(env.dir, "repo.worktrees"), nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main", HEAD: "abc123"},
		{Path: wtPath, Branch: "feature/auth", HEAD: "def456"},
	}, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(true, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(true, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(3, nil)

	err := listRun()
	require.NoError(t, err)
	out := env.out.String()
	assert.Contains(t, out, "merging")
	assert.Contains(t, out, "dirty")
	assert.Contains(t, out, "↓3")
}

func TestList_StatusRebasingOnly(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(filepath.Join(env.dir, "repo.worktrees"), nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main", HEAD: "abc123"},
		{Path: wtPath, Branch: "feature/auth", HEAD: "def456"},
	}, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(0, nil)

	err := listRun()
	require.NoError(t, err)
	out := env.out.String()
	assert.Contains(t, out, "rebasing")
	assert.NotContains(t, out, "clean")
}

// ─── Switch Tests ────────────────────────────────────────────────────────────

func TestSwitch_FocusesWindow(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.iterm.EXPECT().EnsureRunning().Return(nil)
	env.iterm.EXPECT().SessionExists("c-123").Return(true)
	env.iterm.EXPECT().FocusWindow("c-123").Return(nil)

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "c-123",
	}))

	err := switchRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Focused")
}

func TestSwitch_StaleSession(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.iterm.EXPECT().EnsureRunning().Return(nil)
	env.iterm.EXPECT().SessionExists("c-123").Return(false)

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "c-123",
	}))

	err := switchRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.err.String(), "no longer exists")
}

// ─── Open Tests ──────────────────────────────────────────────────────────────

func TestOpen_AlreadyOpen(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.iterm.EXPECT().IsRunning().Return(true)
	env.iterm.EXPECT().SessionExists("c-123").Return(true)
	env.iterm.EXPECT().FocusWindow("c-123").Return(nil)

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "c-123",
	}))

	err := openRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "already open")
}

func TestOpen_NewWindow(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
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
	promptDefaultYes = func(msg string) bool { return true }

	wtDir := filepath.Join(env.dir, "repo.worktrees")
	wtPath := filepath.Join(wtDir, "feat-mkdocs")

	// openRun calls ResolveWorktree (returns error), then createRun -> lcMgr.Create calls RepoName + WorktreesDir
	env.git.EXPECT().ResolveWorktree(mock.Anything, "feat-mkdocs").Return("", fmt.Errorf("worktree not found: feat-mkdocs"))
	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
	env.git.EXPECT().BranchExists(mock.Anything, "feat-mkdocs").Return(false, nil)
	env.git.EXPECT().WorktreeAdd(mock.Anything, wtPath, "feat-mkdocs", "main", true).
		Run(func(repoPath, path, branch, base string, newBranch bool) {
			_ = os.MkdirAll(path, 0755)
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
	promptDefaultYes = func(msg string) bool { return false }

	// openRun calls ResolveWorktree (fails), warns, prompts (denied), returns nil
	env.git.EXPECT().ResolveWorktree(mock.Anything, "feat-mkdocs").Return("", fmt.Errorf("worktree not found: feat-mkdocs"))

	err := openRun("feat-mkdocs")
	require.NoError(t, err)
	assert.Contains(t, env.err.String(), "not found")
}

func TestOpen_BareShorthand(t *testing.T) {
	// wt <branch> delegates to openRun
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
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
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(false, nil)
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, false).
		Run(func(repoPath, path string, force bool) {
			_ = os.RemoveAll(path)
		}).Return(nil)
	env.git.EXPECT().BranchDelete(mock.Anything, "feature/auth", false).Return(nil)

	env.iterm.EXPECT().IsRunning().Return(true)
	env.iterm.EXPECT().SessionExists("c-123").Return(true)
	env.iterm.EXPECT().CloseWindow("c-123").Return(nil)

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "c-123",
	}))

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
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	// --force skips safety checks (no IsWorktreeDirty/HasUnpushedCommits calls)
	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, true).
		Run(func(repoPath, path string, force bool) {
			_ = os.RemoveAll(path)
		}).Return(nil)

	err := deleteRun("auth")
	require.NoError(t, err)
}

func TestDelete_SafeCleanWorktree(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(false, nil)
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, false).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)

	err := deleteRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "removed")
}

func TestDelete_DirtyWorktreePromptDenied(t *testing.T) {
	env := setupTest(t)
	promptFunc = func(msg string) bool { return false }

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(true, nil)

	err := deleteRun("auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aborted")
}

func TestDelete_DirtyWorktreePromptAccepted(t *testing.T) {
	env := setupTest(t)
	promptFunc = func(msg string) bool { return true }

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(true, nil)
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, false).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)

	err := deleteRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "removed")
}

func TestDelete_UnpushedCommitsPromptDenied(t *testing.T) {
	env := setupTest(t)
	promptFunc = func(msg string) bool { return false }

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
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
	require.NoError(t, os.MkdirAll(wtPath1, 0755))
	require.NoError(t, os.MkdirAll(wtPath2, 0755))

	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
		{Path: wtPath1, Branch: "feature/auth"},
		{Path: wtPath2, Branch: "feature/api"},
	}, nil)
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath1, true).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath2, true).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().WorktreePrune(mock.Anything).Return(nil)

	err := deleteAllRun()
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Deleted 2 worktrees")
}

func TestDelete_All_NoneFound(t *testing.T) {
	env := setupTest(t)
	deleteAll = true

	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
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

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
	env.git.EXPECT().BranchExists(mock.Anything, "feature/dry").Return(false, nil)

	err := createRun("feature/dry")
	require.NoError(t, err)

	// Verify no side effects
	assert.NoDirExists(t, filepath.Join(wtDir, "dry"))
	ws, _ := env.state.GetWorktree(filepath.Join(wtDir, "dry"))
	assert.Nil(t, ws)

	assert.Contains(t, env.out.String(), "Would create worktree")
}

func TestDryRun_Open(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)

	err := openRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Would open iTerm2 window")
}

func TestDryRun_Switch(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		ClaudeSessionID: "c-123",
	}))

	err := switchRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.err.String(), "DRY-RUN")
}

// ─── Prune Tests ─────────────────────────────────────────────────────────────

func TestPrune_CleansStaleState(t *testing.T) {
	env := setupTest(t)

	// Add state for a non-existent path
	require.NoError(t, env.state.SetWorktree("/nonexistent/path", &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "stale",
	}))

	wtDir := filepath.Join(env.dir, "repo.worktrees")
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
	env.git.EXPECT().WorktreePrune(mock.Anything).Return(nil)

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
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
	env.git.EXPECT().WorktreePrune(mock.Anything).Return(nil)

	err := pruneRun()
	require.NoError(t, err)

	assert.Contains(t, env.out.String(), "clean")
}

func TestPrune_DryRun(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	require.NoError(t, env.state.SetWorktree("/nonexistent/path", &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "stale",
	}))

	wtDir := filepath.Join(env.dir, "repo.worktrees")
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)

	// WorktreePrune should NOT be called in dry-run

	err := pruneRun()
	require.NoError(t, err)

	assert.Contains(t, env.out.String(), "Would run git worktree prune")
}

func TestDryRun_Delete(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(false, nil)

	// Should not call WorktreeRemove
	_ = mock.Anything

	err := deleteRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Would remove git worktree")
	assert.DirExists(t, wtPath) // dir not removed
}

// --- Claude Trust Tests ---

func TestCreate_SetsClaudeTrust(t *testing.T) {
	env := setupTest(t)
	wtDir := filepath.Join(env.dir, "repo.worktrees")
	wtPath := filepath.Join(wtDir, "auth")

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
	env.git.EXPECT().BranchExists(mock.Anything, "feature/auth").Return(false, nil)
	env.git.EXPECT().WorktreeAdd(mock.Anything, wtPath, "feature/auth", "main", true).
		Run(func(repoPath, path, branch, base string, newBranch bool) {
			_ = os.MkdirAll(path, 0755)
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

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
	env.git.EXPECT().BranchExists(mock.Anything, "feature/dry").Return(false, nil)

	err := createRun("feature/dry")
	require.NoError(t, err)

	// Verify no Claude trust was set (file should not exist)
	_, statErr := os.Stat(env.claude.Path())
	assert.True(t, os.IsNotExist(statErr), "claude.json should not be created in dry-run")
}

func TestDelete_RemovesClaudeTrust(t *testing.T) {
	env := setupTest(t)

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	// Pre-set Claude trust
	added, err := env.claude.TrustProject(wtPath)
	require.NoError(t, err)
	require.True(t, added)

	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(false, nil)
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, false).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)

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
	require.NoError(t, os.MkdirAll(existingPath, 0755))

	// Trust both
	_, err := env.claude.TrustProject(existingPath)
	require.NoError(t, err)
	_, err = env.claude.TrustProject(stalePath)
	require.NoError(t, err)

	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
	env.git.EXPECT().WorktreePrune(mock.Anything).Return(nil)

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

	assert.Contains(t, env.out.String(), "Pruned 1 stale trust entries")
}

// ─── Merge Tests ─────────────────────────────────────────────────────────────

func TestMerge_LocalSuccess_WithRemote(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CurrentBranch(env.dir).Return("main", nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(true, nil).Times(2) // once in merge, once in finish
	env.git.EXPECT().Pull(env.dir).Return(nil)
	env.git.EXPECT().Merge(env.dir, "feature/auth").Return(nil)
	env.git.EXPECT().Push(env.dir, "main", false).Return(nil)

	// Cleanup expectations: remove worktree + delete branch
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, true).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().BranchDelete(mock.Anything, "feature/auth", false).Return(nil)

	err := mergeRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Merged")
	assert.Contains(t, out, "Merge complete")
}

func TestMerge_LocalSuccess_NoRemote(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CurrentBranch(env.dir).Return("main", nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil).Times(2) // once in merge, once in finish
	env.git.EXPECT().Merge(env.dir, "feature/auth").Return(nil)

	// Cleanup: no push, but still remove worktree + delete branch
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, true).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().BranchDelete(mock.Anything, "feature/auth", false).Return(nil)

	err := mergeRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Merged")
	assert.Contains(t, out, "Merge complete")
}

func TestMerge_NothingToMerge(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(false, nil)

	err := mergeRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "No commits to merge")
}

func TestMerge_DirtyWorktree(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(true, nil)

	err := mergeRun("auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uncommitted changes")
}

func TestMerge_DirtyWorktree_Force(t *testing.T) {
	env := setupTest(t)
	mergeForce = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	// --force skips IsWorktreeDirty
	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CurrentBranch(env.dir).Return("main", nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil).Times(2)
	env.git.EXPECT().Merge(env.dir, "feature/auth").Return(nil)
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, true).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().BranchDelete(mock.Anything, "feature/auth", false).Return(nil)

	err := mergeRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Merge complete")
}

func TestMerge_MergeConflict_NoCleanup(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CurrentBranch(env.dir).Return("main", nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)
	env.git.EXPECT().Merge(env.dir, "feature/auth").Return(assert.AnError)

	err := mergeRun("feature/auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "merge conflict")

	// Worktree should NOT be cleaned up
	assert.DirExists(t, wtPath)
}

func TestMerge_NoCleanup(t *testing.T) {
	env := setupTest(t)
	mergeNoCleanup = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CurrentBranch(env.dir).Return("main", nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil).Times(2)
	env.git.EXPECT().Merge(env.dir, "feature/auth").Return(nil)

	// No WorktreeRemove or BranchDelete expected

	err := mergeRun("feature/auth")
	require.NoError(t, err)
	assert.DirExists(t, wtPath) // worktree preserved
	assert.Contains(t, env.out.String(), "Merge complete")
}

func TestMerge_MainRepoNotOnBaseBranch(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CurrentBranch(env.dir).Return("develop", nil)

	err := mergeRun("auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 'main'")
}

func TestMerge_PR_Success(t *testing.T) {
	env := setupTest(t)
	mergePR = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().Push(wtPath, "feature/auth", true).Return(nil)

	// Mock gh pr create
	ghPRCreateFunc = func(args []string) (string, error) {
		assert.Contains(t, args, "--base")
		assert.Contains(t, args, "--fill")
		return "https://github.com/owner/repo/pull/42", nil
	}

	err := mergeRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Pull request created")
	assert.Contains(t, out, "https://github.com/owner/repo/pull/42")
}

func TestMerge_PR_Draft(t *testing.T) {
	env := setupTest(t)
	mergePR = true
	mergeDraft = true
	mergeTitle = "My PR"

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().Push(wtPath, "feature/auth", true).Return(nil)

	ghPRCreateFunc = func(args []string) (string, error) {
		assert.Contains(t, args, "--draft")
		assert.Contains(t, args, "--title")
		assert.Contains(t, args, "My PR")
		return "https://github.com/owner/repo/pull/43", nil
	}

	err := mergeRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Pull request created")
}

func TestMerge_DryRun_Local(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CurrentBranch(env.dir).Return("main", nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(true, nil).Times(2)

	err := mergeRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Would merge")
	assert.Contains(t, out, "Would pull")
	assert.Contains(t, out, "Would push")
	assert.DirExists(t, wtPath) // not removed in dry run
}

func TestMerge_DryRun_PR(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true
	mergePR = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)

	err := mergeRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Would push")
	assert.Contains(t, out, "Would run: gh")
}

func TestMerge_WorktreeNotFound(t *testing.T) {
	env := setupTest(t)
	_ = env

	env.git.EXPECT().ResolveWorktree(mock.Anything, "nonexistent").Return("", fmt.Errorf("worktree not found: nonexistent"))

	err := mergeRun("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree not found")
}

func TestMerge_CustomBase(t *testing.T) {
	env := setupTest(t)
	mergeBase = "develop"

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	// Note: uses "develop" as base branch
	env.git.EXPECT().HasUnpushedCommits(wtPath, "develop").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CurrentBranch(env.dir).Return("develop", nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil).Times(2)
	env.git.EXPECT().Merge(env.dir, "feature/auth").Return(nil)
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, true).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().BranchDelete(mock.Anything, "feature/auth", false).Return(nil)

	err := mergeRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Merge complete")
}

func TestMerge_Continue_Success(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(true, nil)
	env.git.EXPECT().HasConflicts(env.dir).Return(false, nil)
	env.git.EXPECT().MergeContinue(env.dir).Return(nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)

	// Cleanup
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, true).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().BranchDelete(mock.Anything, "feature/auth", false).Return(nil)

	err := mergeRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Merge in progress")
	assert.Contains(t, out, "Merge complete")
}

func TestMerge_Continue_WithRemote(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(true, nil)
	env.git.EXPECT().HasConflicts(env.dir).Return(false, nil)
	env.git.EXPECT().MergeContinue(env.dir).Return(nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(true, nil)
	env.git.EXPECT().Push(env.dir, "main", false).Return(nil)

	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, true).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().BranchDelete(mock.Anything, "feature/auth", false).Return(nil)

	err := mergeRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Merge complete")
}

func TestMerge_Continue_UnresolvedConflicts(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(true, nil)
	env.git.EXPECT().HasConflicts(env.dir).Return(true, nil)

	err := mergeRun("auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolved conflicts")
}

func TestMerge_Continue_DryRun(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(true, nil)
	env.git.EXPECT().HasConflicts(env.dir).Return(false, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)

	err := mergeRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Would run: git merge --continue")
}

// ─── Lifecycle Delete Tests ──────────────────────────────────────────────────

func TestLifecycleDelete_FullCleanup(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "c-123",
	}))

	// Trust the project first
	_, err := env.claude.TrustProject(wtPath)
	require.NoError(t, err)

	env.iterm.EXPECT().IsRunning().Return(true)
	env.iterm.EXPECT().SessionExists("c-123").Return(true)
	env.iterm.EXPECT().CloseWindow("c-123").Return(nil)

	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, false).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().BranchDelete(mock.Anything, "feature/auth", false).Return(nil)

	err = lcMgr.Delete(lifecycle.DeleteOptions{
		RepoPath:     env.dir,
		WtPath:       wtPath,
		Branch:       "feature/auth",
		Force:        false,
		DeleteBranch: true,
	})
	require.NoError(t, err)

	// Verify state removed
	ws, _ := env.state.GetWorktree(wtPath)
	assert.Nil(t, ws)

	// Verify trust removed
	added, err := env.claude.TrustProject(wtPath)
	require.NoError(t, err)
	assert.True(t, added, "trust should have been removed by cleanup")

	assert.Contains(t, env.out.String(), "Closed iTerm2 window")
	assert.Contains(t, env.out.String(), "Removed git worktree")
	assert.Contains(t, env.out.String(), "removed")
}

func TestLifecycleDelete_NoBranchDelete(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, true).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)

	// No BranchDelete expected since DeleteBranch=false

	err := lcMgr.Delete(lifecycle.DeleteOptions{
		RepoPath: env.dir,
		WtPath:   wtPath,
		Branch:   "feature/auth",
		Force:    true,
	})
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "removed")
}

// ─── Sync Tests ──────────────────────────────────────────────────────────────

func TestSync_Success_WithRemote(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(true, nil)
	env.git.EXPECT().Fetch(env.dir).Return(nil)
	env.git.EXPECT().CommitsAhead(wtPath, "origin/main").Return(2, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "origin/main").Return(3, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(0, nil) // local main not ahead
	env.git.EXPECT().Merge(wtPath, "origin/main").Return(nil)

	err := syncRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "↑2 ↓3")
	assert.Contains(t, out, "Merging 3 commit(s)")
	assert.Contains(t, out, "Synced")
	assert.Contains(t, out, "feature/auth")
}

func TestSync_Success_NoRemote(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(5, nil)
	env.git.EXPECT().Merge(wtPath, "main").Return(nil)

	err := syncRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "↓5")
	assert.Contains(t, out, "Merging 5 commit(s)")
	assert.Contains(t, out, "Synced")
}

func TestSync_DirtyWorktree(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(true, nil)

	err := syncRun("auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uncommitted changes")
}

func TestSync_DirtyWorktree_Force(t *testing.T) {
	env := setupTest(t)
	syncForce = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	// --force skips IsWorktreeDirty
	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(1, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(2, nil)
	env.git.EXPECT().Merge(wtPath, "main").Return(nil)

	err := syncRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Synced")
}

func TestSync_WorktreeNotFound(t *testing.T) {
	env := setupTest(t)
	_ = env

	env.git.EXPECT().ResolveWorktree(mock.Anything, "nonexistent").Return("", fmt.Errorf("worktree not found: nonexistent"))

	err := syncRun("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree not found")
}

func TestSync_CustomBase(t *testing.T) {
	env := setupTest(t)
	syncBase = "develop"

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "develop").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "develop").Return(1, nil)
	env.git.EXPECT().Merge(wtPath, "develop").Return(nil)

	err := syncRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Synced")
}

func TestSync_Continue_Success(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(true, nil)
	env.git.EXPECT().HasConflicts(wtPath).Return(false, nil)
	env.git.EXPECT().MergeContinue(wtPath).Return(nil)

	err := syncRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Merge in progress")
	assert.Contains(t, out, "Sync continued")
}

func TestSync_Continue_UnresolvedConflicts(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(true, nil)
	env.git.EXPECT().HasConflicts(wtPath).Return(true, nil)

	err := syncRun("auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolved conflicts")
}

func TestSync_DryRun(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(true, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "origin/main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "origin/main").Return(3, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(0, nil) // local main not ahead

	err := syncRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Would fetch")
	assert.Contains(t, out, "Would merge")
}

func TestSync_All_Success(t *testing.T) {
	env := setupTest(t)
	syncAll = true

	wtPath1 := filepath.Join(env.dir, "repo.worktrees", "auth")
	wtPath2 := filepath.Join(env.dir, "repo.worktrees", "api")
	require.NoError(t, os.MkdirAll(wtPath1, 0755))
	require.NoError(t, os.MkdirAll(wtPath2, 0755))

	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
		{Path: wtPath1, Branch: "feature/auth"},
		{Path: wtPath2, Branch: "feature/api"},
	}, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(true, nil)
	env.git.EXPECT().Fetch(env.dir).Return(nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath1).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath1).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath1).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath1, "origin/main").Return(1, nil)
	env.git.EXPECT().CommitsBehind(wtPath1, "origin/main").Return(3, nil)
	env.git.EXPECT().CommitsBehind(wtPath1, "main").Return(0, nil) // local main not ahead
	env.git.EXPECT().Merge(wtPath1, "origin/main").Return(nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath2).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath2).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath2).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath2, "origin/main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath2, "origin/main").Return(1, nil)
	env.git.EXPECT().CommitsBehind(wtPath2, "main").Return(0, nil) // local main not ahead
	env.git.EXPECT().Merge(wtPath2, "origin/main").Return(nil)

	err := syncAllRun()
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Synced")
	assert.Contains(t, out, "2 synced")
}

func TestSync_All_SkipsDirty(t *testing.T) {
	env := setupTest(t)
	syncAll = true

	wtPath1 := filepath.Join(env.dir, "repo.worktrees", "auth")
	wtPath2 := filepath.Join(env.dir, "repo.worktrees", "api")
	require.NoError(t, os.MkdirAll(wtPath1, 0755))
	require.NoError(t, os.MkdirAll(wtPath2, 0755))

	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
		{Path: wtPath1, Branch: "feature/auth"},
		{Path: wtPath2, Branch: "feature/api"},
	}, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath1).Return(true, nil) // dirty, will skip
	env.git.EXPECT().IsWorktreeDirty(wtPath2).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath2).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath2).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath2, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath2, "main").Return(2, nil)
	env.git.EXPECT().Merge(wtPath2, "main").Return(nil)

	err := syncAllRun()
	require.NoError(t, err)

	out := env.out.String()
	errOut := env.err.String()
	assert.Contains(t, out+errOut, "Skipping")
	assert.Contains(t, out, "1 synced")
	assert.Contains(t, out, "1 skipped")
}

func TestSync_All_NoneFound(t *testing.T) {
	env := setupTest(t)
	syncAll = true

	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
	}, nil)

	err := syncAllRun()
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "No worktrees to sync")
}

// ─── Sync Rebase Tests ───────────────────────────────────────────────────────

func TestSync_Rebase_Success_WithRemote(t *testing.T) {
	env := setupTest(t)
	syncRebase = true
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(true, nil)
	env.git.EXPECT().Fetch(env.dir).Return(nil)
	env.git.EXPECT().CommitsAhead(wtPath, "origin/main").Return(2, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "origin/main").Return(3, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(0, nil) // local main not ahead
	env.git.EXPECT().Rebase(wtPath, "origin/main").Return(nil)

	err := syncRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Rebasing")
	assert.Contains(t, out, "Rebased")
}

func TestSync_Rebase_Success_NoRemote(t *testing.T) {
	env := setupTest(t)
	syncRebase = true
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(5, nil)
	env.git.EXPECT().Rebase(wtPath, "main").Return(nil)

	err := syncRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Rebased")
}

func TestSync_Rebase_Conflict(t *testing.T) {
	env := setupTest(t)
	syncRebase = true
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(2, nil)
	env.git.EXPECT().Rebase(wtPath, "main").Return(assert.AnError)

	err := syncRun("feature/auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rebase conflict")

	errOut := env.err.String()
	assert.Contains(t, errOut, "rebase --abort")
}

func TestSync_Rebase_Continue_Success(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(true, nil)
	env.git.EXPECT().HasConflicts(wtPath).Return(false, nil)
	env.git.EXPECT().RebaseContinue(wtPath).Return(nil)

	err := syncRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Rebase in progress")
	assert.Contains(t, out, "Sync continued")
}

func TestSync_Rebase_Continue_UnresolvedConflicts(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(true, nil)
	env.git.EXPECT().HasConflicts(wtPath).Return(true, nil)

	err := syncRun("auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolved conflicts")
	assert.Contains(t, err.Error(), "rebase --abort")
}

func TestSync_Rebase_All(t *testing.T) {
	env := setupTest(t)
	syncAll = true
	syncRebase = true

	wtPath1 := filepath.Join(env.dir, "repo.worktrees", "auth")
	wtPath2 := filepath.Join(env.dir, "repo.worktrees", "api")
	require.NoError(t, os.MkdirAll(wtPath1, 0755))
	require.NoError(t, os.MkdirAll(wtPath2, 0755))

	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
		{Path: wtPath1, Branch: "feature/auth"},
		{Path: wtPath2, Branch: "feature/api"},
	}, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath1).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath1).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath1).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath1, "main").Return(1, nil)
	env.git.EXPECT().CommitsBehind(wtPath1, "main").Return(3, nil)
	env.git.EXPECT().Rebase(wtPath1, "main").Return(nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath2).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath2).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath2).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath2, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath2, "main").Return(1, nil)
	env.git.EXPECT().Rebase(wtPath2, "main").Return(nil)

	err := syncAllRun()
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Rebased")
	assert.Contains(t, out, "2 synced")
}

func TestSync_Rebase_ConfigDefault(t *testing.T) {
	env := setupTest(t)
	viper.Set("rebase", true) // config sets rebase as default
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(2, nil)
	env.git.EXPECT().Rebase(wtPath, "main").Return(nil)

	err := syncRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Rebased")
}

func TestSync_MergeOverridesConfigRebase(t *testing.T) {
	env := setupTest(t)
	viper.Set("rebase", true) // config says rebase
	syncMerge = true          // but --merge flag overrides

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(1, nil)
	env.git.EXPECT().Merge(wtPath, "main").Return(nil)

	err := syncRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Synced")
}

func TestSync_All_SkipsRebaseInProgress(t *testing.T) {
	env := setupTest(t)
	syncAll = true

	wtPath1 := filepath.Join(env.dir, "repo.worktrees", "auth")
	wtPath2 := filepath.Join(env.dir, "repo.worktrees", "api")
	require.NoError(t, os.MkdirAll(wtPath1, 0755))
	require.NoError(t, os.MkdirAll(wtPath2, 0755))

	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
		{Path: wtPath1, Branch: "feature/auth"},
		{Path: wtPath2, Branch: "feature/api"},
	}, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath1).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath1).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath1).Return(true, nil) // rebase in progress, will skip
	env.git.EXPECT().IsWorktreeDirty(wtPath2).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath2).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath2).Return(false, nil)
	env.git.EXPECT().CommitsAhead(wtPath2, "main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath2, "main").Return(0, nil)

	err := syncAllRun()
	require.NoError(t, err)

	out := env.out.String()
	errOut := env.err.String()
	assert.Contains(t, out+errOut, "rebase in progress")
	assert.Contains(t, out, "1 skipped")
}

func TestSync_WithRemote_LocalMainAhead(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(true, nil)
	env.git.EXPECT().Fetch(env.dir).Return(nil)
	// Remote is in sync but local main has unpushed commits
	env.git.EXPECT().CommitsAhead(wtPath, "origin/main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "origin/main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath, "main").Return(2, nil) // local main has 2 unpushed commits
	env.git.EXPECT().CommitsAhead(wtPath, "main").Return(0, nil)  // re-check ahead against local
	env.git.EXPECT().Merge(wtPath, "main").Return(nil)            // merges from local main

	err := syncRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "↓2")
	assert.Contains(t, out, "Merging 2 commit(s)")
	assert.Contains(t, out, "Synced")
}

func TestSync_All_WithRemote_LocalMainAhead(t *testing.T) {
	env := setupTest(t)
	syncAll = true

	wtPath1 := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath1, 0755))

	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
		{Path: wtPath1, Branch: "feature/auth"},
	}, nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(true, nil)
	env.git.EXPECT().Fetch(env.dir).Return(nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath1).Return(false, nil)
	env.git.EXPECT().IsMergeInProgress(wtPath1).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath1).Return(false, nil)
	// Remote is in sync but local main has unpushed commits
	env.git.EXPECT().CommitsAhead(wtPath1, "origin/main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath1, "origin/main").Return(0, nil)
	env.git.EXPECT().CommitsBehind(wtPath1, "main").Return(3, nil) // local main has 3 unpushed commits
	env.git.EXPECT().CommitsAhead(wtPath1, "main").Return(0, nil)  // re-check ahead against local
	env.git.EXPECT().Merge(wtPath1, "main").Return(nil)            // merges from local main

	err := syncAllRun()
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "1 synced")
	assert.Contains(t, out, "Synced")
}

// ─── Merge Rebase Tests ──────────────────────────────────────────────────────

func TestMerge_Rebase_LocalSuccess(t *testing.T) {
	env := setupTest(t)
	mergeRebase = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CurrentBranch(env.dir).Return("main", nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil).Times(2) // once in merge, once in finish
	env.git.EXPECT().Rebase(wtPath, "main").Return(nil)
	env.git.EXPECT().Merge(env.dir, "feature/auth").Return(nil) // ff merge

	// Cleanup
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, true).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().BranchDelete(mock.Anything, "feature/auth", false).Return(nil)

	err := mergeRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Rebasing")
	assert.Contains(t, out, "Rebased")
	assert.Contains(t, out, "Fast-forward")
	assert.Contains(t, out, "Merge complete")
}

func TestMerge_Rebase_Conflict(t *testing.T) {
	env := setupTest(t)
	mergeRebase = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CurrentBranch(env.dir).Return("main", nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)
	env.git.EXPECT().Rebase(wtPath, "main").Return(assert.AnError)

	err := mergeRun("feature/auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rebase conflict")

	errOut := env.err.String()
	assert.Contains(t, errOut, "rebase --abort")
	assert.DirExists(t, wtPath) // worktree kept
}

func TestMerge_Rebase_Continue_Success(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(true, nil)
	env.git.EXPECT().HasConflicts(wtPath).Return(false, nil)
	env.git.EXPECT().RebaseContinue(wtPath).Return(nil)
	env.git.EXPECT().Merge(env.dir, "feature/auth").Return(nil) // ff merge
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil)

	// Cleanup
	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, true).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().BranchDelete(mock.Anything, "feature/auth", false).Return(nil)

	err := mergeRun("feature/auth")
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Rebase in progress")
	assert.Contains(t, out, "Merge complete")
}

func TestMerge_Rebase_ConfigDefault(t *testing.T) {
	env := setupTest(t)
	viper.Set("rebase", true) // config sets rebase as default

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CurrentBranch(env.dir).Return("main", nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil).Times(2)
	env.git.EXPECT().Rebase(wtPath, "main").Return(nil)
	env.git.EXPECT().Merge(env.dir, "feature/auth").Return(nil) // ff merge

	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, true).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().BranchDelete(mock.Anything, "feature/auth", false).Return(nil)

	err := mergeRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Rebased")
}

func TestMerge_MergeOverridesConfigRebase(t *testing.T) {
	env := setupTest(t)
	viper.Set("rebase", true) // config says rebase
	mergeMerge = true          // but --merge flag overrides

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().IsMergeInProgress(env.dir).Return(false, nil)
	env.git.EXPECT().IsRebaseInProgress(wtPath).Return(false, nil)
	env.git.EXPECT().CurrentBranch(env.dir).Return("main", nil)
	env.git.EXPECT().HasRemote(mock.Anything).Return(false, nil).Times(2)
	env.git.EXPECT().Merge(env.dir, "feature/auth").Return(nil)

	env.git.EXPECT().WorktreeRemove(mock.Anything, wtPath, true).
		Run(func(repoPath, path string, force bool) { _ = os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().BranchDelete(mock.Anything, "feature/auth", false).Return(nil)

	err := mergeRun("feature/auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Merged")
	assert.Contains(t, env.out.String(), "Merge complete")
}

func TestMerge_PR_RebaseWarning(t *testing.T) {
	env := setupTest(t)
	mergePR = true
	mergeRebase = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().ResolveWorktree(mock.Anything, "feature/auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)
	env.git.EXPECT().Push(wtPath, "feature/auth", true).Return(nil)

	ghPRCreateFunc = func(args []string) (string, error) {
		return "https://github.com/owner/repo/pull/99", nil
	}

	err := mergeRun("feature/auth")
	require.NoError(t, err)

	errOut := env.err.String()
	assert.Contains(t, errOut, "--rebase is ignored")
}

// ─── Discover Tests ──────────────────────────────────────────────────────────

func TestDiscover_FindsUnmanaged(t *testing.T) {
	env := setupTest(t)
	wtDir := filepath.Join(env.dir, "repo.worktrees")
	wtPath := filepath.Join(wtDir, "auth")
	externalPath := filepath.Join(env.dir, ".claude", "worktrees", "glittery-pebble")

	// auth is in state, glittery-pebble is NOT
	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
		{Path: wtPath, Branch: "feature/auth"},
		{Path: externalPath, Branch: "worktree-glittery-pebble"},
	}, nil)

	err := discoverRun()
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "1 unmanaged")
	assert.Contains(t, out, "worktree-glittery-pebble")
	assert.Contains(t, out, "external")
	assert.NotContains(t, out, "feature/auth") // managed, should not appear
}

func TestDiscover_NoneFound(t *testing.T) {
	env := setupTest(t)
	wtDir := filepath.Join(env.dir, "repo.worktrees")
	wtPath := filepath.Join(wtDir, "auth")

	require.NoError(t, env.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}))

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
		{Path: wtPath, Branch: "feature/auth"},
	}, nil)

	err := discoverRun()
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "No unmanaged")
}

func TestDiscover_Adopt(t *testing.T) {
	env := setupTest(t)
	discoverAdopt = true

	wtDir := filepath.Join(env.dir, "repo.worktrees")
	externalPath := filepath.Join(env.dir, ".claude", "worktrees", "glittery-pebble")

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
		{Path: externalPath, Branch: "worktree-glittery-pebble"},
	}, nil)

	err := discoverRun()
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Adopted")
	assert.Contains(t, out, "worktree-glittery-pebble")

	// Verify state was written
	ws, err := env.state.GetWorktree(externalPath)
	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, "worktree-glittery-pebble", ws.Branch)
	assert.Equal(t, "myrepo", ws.Repo)
}

func TestDiscover_DryRun(t *testing.T) {
	env := setupTest(t)
	discoverAdopt = true
	dryRun = true
	env.ui.DryRun = true

	wtDir := filepath.Join(env.dir, "repo.worktrees")
	externalPath := filepath.Join(env.dir, ".claude", "worktrees", "glittery-pebble")

	env.git.EXPECT().RepoName(mock.Anything).Return("myrepo", nil)
	env.git.EXPECT().WorktreesDir(mock.Anything).Return(wtDir, nil)
	env.git.EXPECT().WorktreeList(mock.Anything).Return([]gitops.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
		{Path: externalPath, Branch: "worktree-glittery-pebble"},
	}, nil)

	err := discoverRun()
	require.NoError(t, err)

	assert.Contains(t, env.out.String(), "Would adopt")

	// Verify state was NOT written
	ws, _ := env.state.GetWorktree(externalPath)
	assert.Nil(t, ws)
}

// ─── Source Classification Tests ─────────────────────────────────────────────

func TestWorktreeSource(t *testing.T) {
	standardDir := "/repo.worktrees"

	// Path in standard dir => "wt"
	assert.Equal(t, "wt", worktreeSource("/repo.worktrees/auth", standardDir, &state.WorktreeState{}))

	// External path with state => "adopted"
	assert.Equal(t, "adopted", worktreeSource("/home/.claude/worktrees/x", standardDir, &state.WorktreeState{}))

	// External path without state => "external"
	assert.Equal(t, "external", worktreeSource("/home/.claude/worktrees/x", standardDir, nil))

	// Standard dir with no state still "wt"
	assert.Equal(t, "wt", worktreeSource("/repo.worktrees/auth", standardDir, nil))
}
