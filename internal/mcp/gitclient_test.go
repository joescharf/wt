package mcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initTestRepo creates a temporary git repo with an initial commit and returns its path.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "-C", dir, "init", "--initial-branch=main"},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "initial commit"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "cmd %v failed: %s", args, string(out))
	}

	// Resolve symlinks for consistent paths (macOS /tmp -> /private/tmp)
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	return resolved
}

func TestRealGitClient_RepoRoot(t *testing.T) {
	repo := initTestRepo(t)
	client := NewGitClient()

	root, err := client.RepoRoot(repo)
	require.NoError(t, err)
	assert.Equal(t, repo, root)
}

func TestRealGitClient_RepoName(t *testing.T) {
	repo := initTestRepo(t)
	client := NewGitClient()

	name, err := client.RepoName(repo)
	require.NoError(t, err)
	assert.Equal(t, filepath.Base(repo), name)
}

func TestRealGitClient_WorktreesDir(t *testing.T) {
	repo := initTestRepo(t)
	client := NewGitClient()

	wtDir, err := client.WorktreesDir(repo)
	require.NoError(t, err)
	assert.Equal(t, repo+".worktrees", wtDir)
}

func TestRealGitClient_WorktreeList(t *testing.T) {
	repo := initTestRepo(t)
	client := NewGitClient()

	worktrees, err := client.WorktreeList(repo)
	require.NoError(t, err)
	require.Len(t, worktrees, 1, "should have main worktree")
	assert.Equal(t, "main", worktrees[0].Branch)
}

func TestRealGitClient_BranchExists(t *testing.T) {
	repo := initTestRepo(t)
	client := NewGitClient()

	exists, err := client.BranchExists(repo, "main")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = client.BranchExists(repo, "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRealGitClient_CurrentBranch(t *testing.T) {
	repo := initTestRepo(t)
	client := NewGitClient()

	branch, err := client.CurrentBranch(repo)
	require.NoError(t, err)
	assert.Equal(t, "main", branch)
}

func TestRealGitClient_IsWorktreeDirty(t *testing.T) {
	repo := initTestRepo(t)
	client := NewGitClient()

	dirty, err := client.IsWorktreeDirty(repo)
	require.NoError(t, err)
	assert.False(t, dirty)

	// Make it dirty
	require.NoError(t, os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("dirty"), 0644))

	dirty, err = client.IsWorktreeDirty(repo)
	require.NoError(t, err)
	assert.True(t, dirty)
}

func TestRealGitClient_HasRemote(t *testing.T) {
	repo := initTestRepo(t)
	client := NewGitClient()

	hasRemote, err := client.HasRemote(repo)
	require.NoError(t, err)
	assert.False(t, hasRemote, "fresh repo should have no remote")
}

func TestRealGitClient_WorktreeAddAndRemove(t *testing.T) {
	repo := initTestRepo(t)
	client := NewGitClient()

	wtDir := repo + ".worktrees"
	require.NoError(t, os.MkdirAll(wtDir, 0755))
	wtPath := filepath.Join(wtDir, "test-branch")

	// Add worktree with new branch
	err := client.WorktreeAdd(repo, wtPath, "test-branch", "main", true)
	require.NoError(t, err)

	// Verify it exists
	worktrees, err := client.WorktreeList(repo)
	require.NoError(t, err)
	require.Len(t, worktrees, 2)

	// Verify branch exists
	exists, err := client.BranchExists(repo, "test-branch")
	require.NoError(t, err)
	assert.True(t, exists)

	// Remove worktree
	err = client.WorktreeRemove(repo, wtPath, false)
	require.NoError(t, err)

	worktrees, err = client.WorktreeList(repo)
	require.NoError(t, err)
	assert.Len(t, worktrees, 1)
}

func TestRealGitClient_CommitsAheadBehind(t *testing.T) {
	repo := initTestRepo(t)
	client := NewGitClient()

	wtDir := repo + ".worktrees"
	require.NoError(t, os.MkdirAll(wtDir, 0755))
	wtPath := filepath.Join(wtDir, "feature")

	// Create worktree with new branch
	err := client.WorktreeAdd(repo, wtPath, "feature", "main", true)
	require.NoError(t, err)

	// Add a commit on the feature branch
	cmds := [][]string{
		{"git", "-C", wtPath, "commit", "--allow-empty", "-m", "feature commit"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "cmd %v failed: %s", args, string(out))
	}

	ahead, err := client.CommitsAhead(wtPath, "main")
	require.NoError(t, err)
	assert.Equal(t, 1, ahead)

	behind, err := client.CommitsBehind(wtPath, "main")
	require.NoError(t, err)
	assert.Equal(t, 0, behind)
}

func TestRealGitClient_RepoRoot_InvalidPath(t *testing.T) {
	client := NewGitClient()

	_, err := client.RepoRoot("/nonexistent/path")
	assert.Error(t, err)
}
