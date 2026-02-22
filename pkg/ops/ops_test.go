package ops

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joescharf/wt/pkg/gitops"
	"github.com/joescharf/wt/pkg/gitops/mocks"
)

// testLogger captures log output for assertions.
type testLogger struct {
	infos    []string
	successes []string
	warnings []string
	verboses []string
}

func (l *testLogger) Info(format string, args ...interface{})    { l.infos = append(l.infos, fmt.Sprintf(format, args...)) }
func (l *testLogger) Success(format string, args ...interface{}) { l.successes = append(l.successes, fmt.Sprintf(format, args...)) }
func (l *testLogger) Warning(format string, args ...interface{}) { l.warnings = append(l.warnings, fmt.Sprintf(format, args...)) }
func (l *testLogger) Verbose(format string, args ...interface{}) { l.verboses = append(l.verboses, fmt.Sprintf(format, args...)) }

// --- FormatSyncStatus ---

func TestFormatSyncStatus(t *testing.T) {
	tests := []struct {
		ahead, behind int
		want          string
	}{
		{0, 0, "clean"},
		{3, 0, "↑3"},
		{0, 5, "↓5"},
		{2, 4, "↑2 ↓4"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, FormatSyncStatus(tt.ahead, tt.behind))
	}
}

// --- Sync Tests ---

func TestSync_AlreadyInSync(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().IsMergeInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().HasRemote("/repo").Return(false, nil)
	mg.EXPECT().CommitsAhead("/wt/auth", "main").Return(0, nil)
	mg.EXPECT().CommitsBehind("/wt/auth", "main").Return(0, nil)

	result, err := Sync(mg, log, SyncOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
	})

	require.NoError(t, err)
	assert.True(t, result.AlreadySynced)
	assert.True(t, result.Success)
	assert.Equal(t, 0, result.Behind)
}

func TestSync_MergeBehind(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().IsMergeInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().HasRemote("/repo").Return(false, nil)
	mg.EXPECT().CommitsAhead("/wt/auth", "main").Return(1, nil)
	mg.EXPECT().CommitsBehind("/wt/auth", "main").Return(3, nil)
	mg.EXPECT().Merge("/wt/auth", "main").Return(nil)

	result, err := Sync(mg, log, SyncOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
	})

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.False(t, result.AlreadySynced)
	assert.Equal(t, 3, result.Behind)
	assert.Equal(t, 1, result.Ahead)
}

func TestSync_RebaseBehind(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().IsMergeInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().HasRemote("/repo").Return(false, nil)
	mg.EXPECT().CommitsAhead("/wt/auth", "main").Return(2, nil)
	mg.EXPECT().CommitsBehind("/wt/auth", "main").Return(1, nil)
	mg.EXPECT().Rebase("/wt/auth", "main").Return(nil)

	result, err := Sync(mg, log, SyncOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "rebase",
	})

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "rebase", result.Strategy)
}

func TestSync_DirtyWorktreeBlocked(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(true, nil)

	_, err := Sync(mg, log, SyncOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "uncommitted changes")
}

func TestSync_DirtyWorktreeForced(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	// With Force=true, dirty check is skipped entirely
	mg.EXPECT().IsMergeInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().HasRemote("/repo").Return(false, nil)
	mg.EXPECT().CommitsAhead("/wt/auth", "main").Return(0, nil)
	mg.EXPECT().CommitsBehind("/wt/auth", "main").Return(0, nil)

	result, err := Sync(mg, log, SyncOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
		Force:      true,
	})

	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestSync_MergeConflict(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().IsMergeInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().HasRemote("/repo").Return(false, nil)
	mg.EXPECT().CommitsAhead("/wt/auth", "main").Return(0, nil)
	mg.EXPECT().CommitsBehind("/wt/auth", "main").Return(2, nil)
	mg.EXPECT().Merge("/wt/auth", "main").Return(fmt.Errorf("conflict"))

	result, err := Sync(mg, log, SyncOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "merge conflict")
	assert.True(t, result.Conflict)
	assert.False(t, result.Success)
}

func TestSync_ContinueMerge(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().IsMergeInProgress("/wt/auth").Return(true, nil)
	mg.EXPECT().HasConflicts("/wt/auth").Return(false, nil)
	mg.EXPECT().MergeContinue("/wt/auth").Return(nil)

	result, err := Sync(mg, log, SyncOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
	})

	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestSync_ContinueRebase(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().IsMergeInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/auth").Return(true, nil)
	mg.EXPECT().HasConflicts("/wt/auth").Return(false, nil)
	mg.EXPECT().RebaseContinue("/wt/auth").Return(nil)

	result, err := Sync(mg, log, SyncOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "rebase",
	})

	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestSync_WithRemoteFetches(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().IsMergeInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().HasRemote("/repo").Return(true, nil)
	mg.EXPECT().Fetch("/repo").Return(nil)
	// With remote, merge source becomes "origin/main"
	mg.EXPECT().CommitsAhead("/wt/auth", "origin/main").Return(0, nil)
	mg.EXPECT().CommitsBehind("/wt/auth", "origin/main").Return(2, nil)
	// Also check local base branch
	mg.EXPECT().CommitsBehind("/wt/auth", "main").Return(1, nil)
	mg.EXPECT().Merge("/wt/auth", "origin/main").Return(nil)

	result, err := Sync(mg, log, SyncOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
	})

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 2, result.Behind)
}

func TestSync_DryRun(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().IsMergeInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().HasRemote("/repo").Return(false, nil)
	mg.EXPECT().CommitsAhead("/wt/auth", "main").Return(0, nil)
	mg.EXPECT().CommitsBehind("/wt/auth", "main").Return(3, nil)
	// Should NOT call Merge in dry-run mode

	result, err := Sync(mg, log, SyncOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
		DryRun:     true,
	})

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// --- SyncAll Tests ---

func TestSyncAll_NoWorktrees(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().WorktreeList("/repo").Return([]gitops.WorktreeInfo{
		{Path: "/repo", Branch: "main"},
	}, nil)

	results, err := SyncAll(mg, log, SyncOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Strategy:   "merge",
	})

	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestSyncAll_SkipsDirty(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().WorktreeList("/repo").Return([]gitops.WorktreeInfo{
		{Path: "/repo", Branch: "main"},
		{Path: "/wt/auth", Branch: "feature/auth"},
	}, nil)
	mg.EXPECT().HasRemote("/repo").Return(false, nil)
	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(true, nil)

	results, err := SyncAll(mg, log, SyncOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Strategy:   "merge",
	})

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Skipped)
	assert.Equal(t, "uncommitted changes", results[0].SkipReason)
}

func TestSyncAll_MultipleMixed(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().WorktreeList("/repo").Return([]gitops.WorktreeInfo{
		{Path: "/repo", Branch: "main"},
		{Path: "/wt/auth", Branch: "feature/auth"},
		{Path: "/wt/fix", Branch: "bugfix/login"},
	}, nil)
	mg.EXPECT().HasRemote("/repo").Return(false, nil)

	// auth: up to date
	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().IsMergeInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().CommitsAhead("/wt/auth", "main").Return(1, nil)
	mg.EXPECT().CommitsBehind("/wt/auth", "main").Return(0, nil)

	// fix: behind, needs merge
	mg.EXPECT().IsWorktreeDirty("/wt/fix").Return(false, nil)
	mg.EXPECT().IsMergeInProgress("/wt/fix").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/fix").Return(false, nil)
	mg.EXPECT().CommitsAhead("/wt/fix", "main").Return(0, nil)
	mg.EXPECT().CommitsBehind("/wt/fix", "main").Return(2, nil)
	mg.EXPECT().Merge("/wt/fix", "main").Return(nil)

	results, err := SyncAll(mg, log, SyncOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Strategy:   "merge",
	})

	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.True(t, results[0].AlreadySynced)
	assert.True(t, results[1].Success)
}

// --- Merge Tests ---

func TestMerge_LocalMerge(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().HasUnpushedCommits("/wt/auth", "main").Return(true, nil)
	mg.EXPECT().IsMergeInProgress("/repo").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().CurrentBranch("/repo").Return("main", nil)
	mg.EXPECT().HasRemote("/repo").Return(false, nil)
	mg.EXPECT().Merge("/repo", "feature/auth").Return(nil)

	cleanupCalled := false
	cleanup := func(wtPath, branch string) error {
		cleanupCalled = true
		return nil
	}

	result, err := Merge(mg, log, MergeOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
	}, cleanup, nil)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.True(t, cleanupCalled)
}

func TestMerge_NoCommits(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().HasUnpushedCommits("/wt/auth", "main").Return(false, nil)

	result, err := Merge(mg, log, MergeOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
	}, nil, nil)

	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestMerge_DirtyBlocked(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(true, nil)

	_, err := Merge(mg, log, MergeOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
	}, nil, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "uncommitted changes")
}

func TestMerge_WrongBranch(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().HasUnpushedCommits("/wt/auth", "main").Return(true, nil)
	mg.EXPECT().IsMergeInProgress("/repo").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().CurrentBranch("/repo").Return("develop", nil)

	_, err := Merge(mg, log, MergeOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
	}, nil, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "main repo is on 'develop'")
}

func TestMerge_RebaseThenFF(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().HasUnpushedCommits("/wt/auth", "main").Return(true, nil)
	mg.EXPECT().IsMergeInProgress("/repo").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().CurrentBranch("/repo").Return("main", nil)
	mg.EXPECT().HasRemote("/repo").Return(false, nil)
	mg.EXPECT().Rebase("/wt/auth", "main").Return(nil)
	mg.EXPECT().Merge("/repo", "feature/auth").Return(nil)

	result, err := Merge(mg, log, MergeOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "rebase",
	}, nil, nil)

	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestMerge_PR(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().HasUnpushedCommits("/wt/auth", "main").Return(true, nil)
	mg.EXPECT().Push("/wt/auth", "feature/auth", true).Return(nil)

	var capturedArgs []string
	prCreate := func(args []string) (string, error) {
		capturedArgs = args
		return "https://github.com/repo/pull/42", nil
	}

	result, err := Merge(mg, log, MergeOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		CreatePR:   true,
		PRTitle:    "Add auth",
	}, nil, prCreate)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.True(t, result.PRCreated)
	assert.Equal(t, "https://github.com/repo/pull/42", result.PRURL)
	assert.Contains(t, capturedArgs, "--title")
	assert.Contains(t, capturedArgs, "Add auth")
}

func TestMerge_PRDryRun(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().HasUnpushedCommits("/wt/auth", "main").Return(true, nil)
	// Should NOT call Push or PRCreate in dry-run

	result, err := Merge(mg, log, MergeOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		CreatePR:   true,
		DryRun:     true,
	}, nil, nil)

	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestMerge_NoCleanup(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().HasUnpushedCommits("/wt/auth", "main").Return(true, nil)
	mg.EXPECT().IsMergeInProgress("/repo").Return(false, nil)
	mg.EXPECT().IsRebaseInProgress("/wt/auth").Return(false, nil)
	mg.EXPECT().CurrentBranch("/repo").Return("main", nil)
	mg.EXPECT().HasRemote("/repo").Return(false, nil)
	mg.EXPECT().Merge("/repo", "feature/auth").Return(nil)

	cleanupCalled := false
	cleanup := func(wtPath, branch string) error {
		cleanupCalled = true
		return nil
	}

	result, err := Merge(mg, log, MergeOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
		NoCleanup:  true,
	}, cleanup, nil)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.False(t, cleanupCalled)
}

func TestMerge_ContinueMerge(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().IsWorktreeDirty("/wt/auth").Return(false, nil)
	mg.EXPECT().HasUnpushedCommits("/wt/auth", "main").Return(true, nil)
	mg.EXPECT().IsMergeInProgress("/repo").Return(true, nil)
	mg.EXPECT().HasConflicts("/repo").Return(false, nil)
	mg.EXPECT().MergeContinue("/repo").Return(nil)
	mg.EXPECT().HasRemote("/repo").Return(false, nil)

	result, err := Merge(mg, log, MergeOptions{
		RepoPath:   "/repo",
		BaseBranch: "main",
		Branch:     "feature/auth",
		WtPath:     "/wt/auth",
		Strategy:   "merge",
		NoCleanup:  true,
	}, nil, nil)

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// --- Delete Tests ---

func TestDelete_Basic(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	cleanupCalled := false
	cleanup := func(wtPath, branch string) error {
		assert.Equal(t, "/wt/auth", wtPath)
		assert.Equal(t, "feature/auth", branch)
		cleanupCalled = true
		return nil
	}

	err := Delete(mg, log, DeleteOptions{
		RepoPath: "/repo",
		WtPath:   "/wt/auth",
		Branch:   "feature/auth",
		Force:    true,
	}, nil, cleanup)

	require.NoError(t, err)
	assert.True(t, cleanupCalled)
}

func TestDelete_SafetyCheckAbort(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	safetyCheck := func(wtPath string) (bool, error) {
		return false, nil // user declined
	}

	err := Delete(mg, log, DeleteOptions{
		RepoPath: "/repo",
		WtPath:   "/wt/auth",
		Branch:   "feature/auth",
	}, safetyCheck, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete aborted")
}

func TestDelete_SafetyCheckPasses(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	safetyCheck := func(wtPath string) (bool, error) {
		return true, nil
	}
	cleanup := func(wtPath, branch string) error {
		return nil
	}

	err := Delete(mg, log, DeleteOptions{
		RepoPath: "/repo",
		WtPath:   "/wt/auth",
		Branch:   "feature/auth",
	}, safetyCheck, cleanup)

	require.NoError(t, err)
}

func TestDeleteAll_NoWorktrees(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().WorktreeList("/repo").Return([]gitops.WorktreeInfo{
		{Path: "/repo", Branch: "main"},
	}, nil)

	count, err := DeleteAll(mg, log, DeleteOptions{
		RepoPath: "/repo",
		Force:    true,
	}, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// --- Prune Tests ---

func TestPrune_Clean(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	statePrune := func() (int, error) { return 0, nil }
	mg.EXPECT().WorktreesDir("/repo").Return("/repo.worktrees", nil)
	trustPrune := func(dir string) (int, error) { return 0, nil }
	mg.EXPECT().WorktreePrune("/repo").Return(nil)

	result, err := Prune(mg, log, PruneOptions{RepoPath: "/repo"}, statePrune, trustPrune)

	require.NoError(t, err)
	assert.Equal(t, 0, result.StatePruned)
	assert.Equal(t, 0, result.TrustPruned)
	assert.True(t, result.GitPruned)
}

func TestPrune_WithStaleEntries(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	statePrune := func() (int, error) { return 3, nil }
	mg.EXPECT().WorktreesDir("/repo").Return("/repo.worktrees", nil)
	trustPrune := func(dir string) (int, error) {
		assert.Equal(t, "/repo.worktrees", dir)
		return 1, nil
	}
	mg.EXPECT().WorktreePrune("/repo").Return(nil)

	result, err := Prune(mg, log, PruneOptions{RepoPath: "/repo"}, statePrune, trustPrune)

	require.NoError(t, err)
	assert.Equal(t, 3, result.StatePruned)
	assert.Equal(t, 1, result.TrustPruned)
}

func TestPrune_NilTrustPruner(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	statePrune := func() (int, error) { return 0, nil }
	mg.EXPECT().WorktreePrune("/repo").Return(nil)

	result, err := Prune(mg, log, PruneOptions{RepoPath: "/repo"}, statePrune, nil)

	require.NoError(t, err)
	assert.Equal(t, 0, result.TrustPruned)
}

func TestPrune_DryRun(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	statePrune := func() (int, error) { return 0, nil }
	mg.EXPECT().WorktreesDir("/repo").Return("/repo.worktrees", nil)
	trustPrune := func(dir string) (int, error) { return 0, nil }
	// Should NOT call WorktreePrune in dry-run

	result, err := Prune(mg, log, PruneOptions{RepoPath: "/repo", DryRun: true}, statePrune, trustPrune)

	require.NoError(t, err)
	assert.False(t, result.GitPruned)
}

// --- Discover Tests ---

func TestDiscover_NoUnmanaged(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().RepoName("/repo").Return("myrepo", nil)
	mg.EXPECT().WorktreesDir("/repo").Return("/repo.worktrees", nil)
	mg.EXPECT().WorktreeList("/repo").Return([]gitops.WorktreeInfo{
		{Path: "/repo", Branch: "main"},
		{Path: "/repo.worktrees/auth", Branch: "feature/auth"},
	}, nil)

	stateCheck := func(path string) (bool, error) {
		return true, nil // all managed
	}

	result, err := Discover(mg, log, DiscoverOptions{RepoPath: "/repo"}, stateCheck, nil)

	require.NoError(t, err)
	assert.Empty(t, result.Unmanaged)
}

func TestDiscover_FindsUnmanaged(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().RepoName("/repo").Return("myrepo", nil)
	mg.EXPECT().WorktreesDir("/repo").Return("/repo.worktrees", nil)
	mg.EXPECT().WorktreeList("/repo").Return([]gitops.WorktreeInfo{
		{Path: "/repo", Branch: "main"},
		{Path: "/repo.worktrees/auth", Branch: "feature/auth"},
		{Path: "/external/fix", Branch: "bugfix/login"},
	}, nil)

	stateCheck := func(path string) (bool, error) {
		return false, nil // none managed
	}

	result, err := Discover(mg, log, DiscoverOptions{RepoPath: "/repo"}, stateCheck, nil)

	require.NoError(t, err)
	require.Len(t, result.Unmanaged, 2)
	assert.Equal(t, "feature/auth", result.Unmanaged[0].Branch)
	assert.Equal(t, "wt", result.Unmanaged[0].Source)
	assert.Equal(t, "bugfix/login", result.Unmanaged[1].Branch)
	assert.Equal(t, "external", result.Unmanaged[1].Source)
}

func TestDiscover_Adopt(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().RepoName("/repo").Return("myrepo", nil)
	mg.EXPECT().WorktreesDir("/repo").Return("/repo.worktrees", nil)
	mg.EXPECT().WorktreeList("/repo").Return([]gitops.WorktreeInfo{
		{Path: "/repo", Branch: "main"},
		{Path: "/repo.worktrees/auth", Branch: "feature/auth"},
	}, nil)

	stateCheck := func(path string) (bool, error) {
		return false, nil
	}

	var adopted []string
	stateAdopt := func(path, repo, branch string) error {
		adopted = append(adopted, branch)
		assert.Equal(t, "myrepo", repo)
		return nil
	}

	result, err := Discover(mg, log, DiscoverOptions{RepoPath: "/repo", Adopt: true}, stateCheck, stateAdopt)

	require.NoError(t, err)
	assert.Equal(t, 1, result.Adopted)
	assert.Equal(t, []string{"feature/auth"}, adopted)
}

func TestDiscover_AdoptDryRun(t *testing.T) {
	mg := mocks.NewMockClient(t)
	log := &testLogger{}

	mg.EXPECT().RepoName("/repo").Return("myrepo", nil)
	mg.EXPECT().WorktreesDir("/repo").Return("/repo.worktrees", nil)
	mg.EXPECT().WorktreeList("/repo").Return([]gitops.WorktreeInfo{
		{Path: "/repo", Branch: "main"},
		{Path: "/repo.worktrees/auth", Branch: "feature/auth"},
	}, nil)

	stateCheck := func(path string) (bool, error) {
		return false, nil
	}

	// stateAdopt should NOT be called in dry-run
	result, err := Discover(mg, log, DiscoverOptions{RepoPath: "/repo", Adopt: true, DryRun: true}, stateCheck, nil)

	require.NoError(t, err)
	require.Len(t, result.Unmanaged, 1)
	assert.Equal(t, 0, result.Adopted)
}

// --- classifySource ---

func TestClassifySource(t *testing.T) {
	assert.Equal(t, "wt", classifySource("/repo.worktrees/auth", "/repo.worktrees"))
	assert.Equal(t, "external", classifySource("/other/path", "/repo.worktrees"))
}
