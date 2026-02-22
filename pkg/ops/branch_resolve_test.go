package ops

import (
	"context"
	"testing"

	"github.com/joescharf/wt/pkg/gitops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockGitClient implements gitops.Client for testing branch resolution.
type mockGitClient struct {
	repoRoot      string
	currentBranch string
	dirty         bool
	hasCommits    bool
	mergeIP       bool
	rebaseIP      bool
	hasRemote     bool

	// Track which branch was passed to Merge/Rebase
	mergedBranch  string
	rebasedBranch string
}

func (m *mockGitClient) RepoRoot() (string, error)                              { return m.repoRoot, nil }
func (m *mockGitClient) RepoName() (string, error)                              { return "test-repo", nil }
func (m *mockGitClient) WorktreesDir() (string, error)                          { return m.repoRoot + ".worktrees", nil }
func (m *mockGitClient) WorktreeList() ([]gitops.WorktreeInfo, error)           { return nil, nil }
func (m *mockGitClient) WorktreeAdd(path, branch, base string, newBranch bool) error { return nil }
func (m *mockGitClient) WorktreeRemove(path string, force bool) error           { return nil }
func (m *mockGitClient) BranchExists(branch string) (bool, error)               { return true, nil }
func (m *mockGitClient) BranchDelete(branch string, force bool) error           { return nil }
func (m *mockGitClient) CurrentBranch(wtPath string) (string, error)            { return m.currentBranch, nil }
func (m *mockGitClient) ResolveWorktree(input string) (string, error)           { return input, nil }
func (m *mockGitClient) BranchList() ([]string, error)                          { return nil, nil }
func (m *mockGitClient) IsWorktreeDirty(path string) (bool, error)              { return m.dirty, nil }
func (m *mockGitClient) HasUnpushedCommits(path, baseBranch string) (bool, error) { return m.hasCommits, nil }
func (m *mockGitClient) WorktreePrune() error                                   { return nil }
func (m *mockGitClient) Merge(repoPath, branch string) error {
	m.mergedBranch = branch
	return nil
}
func (m *mockGitClient) MergeContinue(repoPath string) error                    { return nil }
func (m *mockGitClient) IsMergeInProgress(repoPath string) (bool, error)        { return m.mergeIP, nil }
func (m *mockGitClient) HasConflicts(repoPath string) (bool, error)             { return false, nil }
func (m *mockGitClient) Rebase(repoPath, branch string) error {
	m.rebasedBranch = branch
	return nil
}
func (m *mockGitClient) RebaseContinue(repoPath string) error                   { return nil }
func (m *mockGitClient) RebaseAbort(repoPath string) error                      { return nil }
func (m *mockGitClient) IsRebaseInProgress(repoPath string) (bool, error)       { return m.rebaseIP, nil }
func (m *mockGitClient) Pull(repoPath string) error                             { return nil }
func (m *mockGitClient) Push(wtPath, branch string, setUpstream bool) error     { return nil }
func (m *mockGitClient) HasRemote() (bool, error)                               { return m.hasRemote, nil }
func (m *mockGitClient) Fetch(repoPath string) error                            { return nil }
func (m *mockGitClient) CommitsAhead(wtPath, baseBranch string) (int, error)    { return 1, nil }
func (m *mockGitClient) CommitsBehind(wtPath, baseBranch string) (int, error)   { return 1, nil }

func TestMerge_ExplicitBranch_OverridesAutoDetection(t *testing.T) {
	git := &mockGitClient{
		repoRoot:      "/tmp/repo",
		currentBranch: "main",
		hasCommits:    true,
	}

	// Worktree path whose dirname would be "foo-bar" (wrong for feature/foo-bar)
	wtPath := "/tmp/repo.worktrees/foo-bar"

	result, err := Merge(context.Background(), git, nil, NopLogger{}, wtPath, MergeOptions{
		BaseBranch: "main",
		Branch:     "feature/foo-bar", // Explicit branch with slash
	}, nil)

	require.NoError(t, err)
	assert.Equal(t, "feature/foo-bar", result.Branch)
	// Verify the correct branch name was passed to git merge
	assert.Equal(t, "feature/foo-bar", git.mergedBranch)
}

func TestMerge_NoBranch_FallsBackToGitCurrentBranch(t *testing.T) {
	git := &mockGitClient{
		repoRoot:      "/tmp/repo",
		currentBranch: "main",
		hasCommits:    true,
	}

	wtPath := "/tmp/repo.worktrees/some-dir"

	result, err := Merge(context.Background(), git, nil, NopLogger{}, wtPath, MergeOptions{
		BaseBranch: "main",
		// Branch not set — should fall back to git.CurrentBranch
	}, nil)

	require.NoError(t, err)
	// CurrentBranch returns "main" from mock, not the dirname
	assert.Equal(t, "main", result.Branch)
}

func TestSync_ExplicitBranch_OverridesAutoDetection(t *testing.T) {
	git := &mockGitClient{
		repoRoot:      "/tmp/repo",
		currentBranch: "wrong-branch",
	}

	wtPath := "/tmp/repo.worktrees/foo-bar"

	result, err := Sync(context.Background(), git, nil, NopLogger{}, wtPath, SyncOptions{
		BaseBranch: "main",
		Branch:     "feature/foo-bar", // Explicit branch with slash
	})

	require.NoError(t, err)
	assert.Equal(t, "feature/foo-bar", result.Branch)
}

func TestSync_NoBranch_FallsBackToGitCurrentBranch(t *testing.T) {
	git := &mockGitClient{
		repoRoot:      "/tmp/repo",
		currentBranch: "feature/actual-branch",
	}

	wtPath := "/tmp/repo.worktrees/actual-branch"

	result, err := Sync(context.Background(), git, nil, NopLogger{}, wtPath, SyncOptions{
		BaseBranch: "main",
		// Branch not set — dirname "actual-branch" != "feature/actual-branch", so git.CurrentBranch is used
	})

	require.NoError(t, err)
	assert.Equal(t, "feature/actual-branch", result.Branch)
}
