package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBranchToDirname(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{"feature/auth", "auth"},
		{"bugfix/login-fix", "login-fix"},
		{"main", "main"},
		{"a/b/c/deep", "deep"},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			assert.Equal(t, tt.want, BranchToDirname(tt.branch))
		})
	}
}

func TestParseWorktreeListPorcelain(t *testing.T) {
	input := `worktree /Users/joe/myrepo
HEAD abc123def456
branch refs/heads/main

worktree /Users/joe/myrepo.worktrees/auth
HEAD def789abc012
branch refs/heads/feature/auth

`
	got := ParseWorktreeListPorcelain(input)
	require.Len(t, got, 2)

	assert.Equal(t, "/Users/joe/myrepo", got[0].Path)
	assert.Equal(t, "main", got[0].Branch)
	assert.Equal(t, "abc123def456", got[0].HEAD)

	assert.Equal(t, "/Users/joe/myrepo.worktrees/auth", got[1].Path)
	assert.Equal(t, "feature/auth", got[1].Branch)
	assert.Equal(t, "def789abc012", got[1].HEAD)
}

func TestParseWorktreeListPorcelain_NoTrailingNewline(t *testing.T) {
	input := `worktree /repo
HEAD abc123
branch refs/heads/main`

	got := ParseWorktreeListPorcelain(input)
	require.Len(t, got, 1)
	assert.Equal(t, "/repo", got[0].Path)
	assert.Equal(t, "main", got[0].Branch)
}

func TestResolveWorktreePath(t *testing.T) {
	dir := t.TempDir()
	wtDir := filepath.Join(dir, "repo.worktrees")

	// Create a directory for the "auth" worktree
	authDir := filepath.Join(wtDir, "auth")
	err := os.MkdirAll(authDir, 0755)
	require.NoError(t, err)

	// Full path passthrough
	path, err := ResolveWorktreePath("/some/absolute/path", wtDir)
	require.NoError(t, err)
	assert.Equal(t, "/some/absolute/path", path)

	// Dirname match
	path, err = ResolveWorktreePath("auth", wtDir)
	require.NoError(t, err)
	assert.Equal(t, authDir, path)

	// Branch name conversion
	path, err = ResolveWorktreePath("feature/auth", wtDir)
	require.NoError(t, err)
	assert.Equal(t, authDir, path)

	// Non-existing returns expected path
	path, err = ResolveWorktreePath("feature/new", wtDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(wtDir, "new"), path)
}

// Integration tests that create real git repos

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Resolve symlinks for macOS /var -> /private/var
	dir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "cmd %v failed: %s", args, string(out))
	}
	return dir
}

func TestRepoRoot_Integration(t *testing.T) {
	repoDir := initTestRepo(t)

	// Change to repo dir for the test
	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()
	root, err := client.RepoRoot()
	require.NoError(t, err)
	assert.Equal(t, repoDir, root)
}

func TestWorktreeLifecycle_Integration(t *testing.T) {
	repoDir := initTestRepo(t)
	wtDir := repoDir + ".worktrees"
	err := os.MkdirAll(wtDir, 0755)
	require.NoError(t, err)

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()

	// Create worktree with new branch
	wtPath := filepath.Join(wtDir, "auth")
	err = client.WorktreeAdd(wtPath, "feature/auth", "HEAD", true)
	require.NoError(t, err)
	assert.DirExists(t, wtPath)

	// List worktrees
	list, err := client.WorktreeList()
	require.NoError(t, err)
	require.Len(t, list, 2) // main + new worktree

	// Branch exists
	exists, err := client.BranchExists("feature/auth")
	require.NoError(t, err)
	assert.True(t, exists)

	// Branch doesn't exist
	exists, err = client.BranchExists("nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)

	// Current branch
	branch, err := client.CurrentBranch(wtPath)
	require.NoError(t, err)
	assert.Equal(t, "feature/auth", branch)

	// Remove worktree
	err = client.WorktreeRemove(wtPath, false)
	require.NoError(t, err)
	assert.NoDirExists(t, wtPath)
}
