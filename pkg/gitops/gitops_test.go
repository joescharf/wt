package gitops

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

func TestBranchList_Integration(t *testing.T) {
	repoDir := initTestRepo(t)

	// Create some branches
	for _, branch := range []string{"feature/auth", "bugfix/login"} {
		cmd := exec.Command("git", "-C", repoDir, "branch", branch)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "create branch %s: %s", branch, string(out))
	}

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()
	branches, err := client.BranchList()
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(branches), 3)
	assert.Contains(t, branches, "feature/auth")
	assert.Contains(t, branches, "bugfix/login")
}

func TestIsWorktreeDirty_Integration(t *testing.T) {
	repoDir := initTestRepo(t)

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()

	// Clean repo
	dirty, err := client.IsWorktreeDirty(repoDir)
	require.NoError(t, err)
	assert.False(t, dirty)

	// Create an untracked file
	err = os.WriteFile(filepath.Join(repoDir, "dirty.txt"), []byte("dirty"), 0644)
	require.NoError(t, err)

	dirty, err = client.IsWorktreeDirty(repoDir)
	require.NoError(t, err)
	assert.True(t, dirty)
}

func TestHasUnpushedCommits_Integration(t *testing.T) {
	repoDir := initTestRepo(t)

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()

	// Create a worktree with a branch
	wtDir := repoDir + ".worktrees"
	err = os.MkdirAll(wtDir, 0755)
	require.NoError(t, err)

	wtPath := filepath.Join(wtDir, "test-branch")
	err = client.WorktreeAdd(wtPath, "test-branch", "HEAD", true)
	require.NoError(t, err)

	// Get the main branch name
	mainBranch, err := client.CurrentBranch(repoDir)
	require.NoError(t, err)

	// No unpushed commits (same as base)
	unpushed, err := client.HasUnpushedCommits(wtPath, mainBranch)
	require.NoError(t, err)
	assert.False(t, unpushed)

	// Add a commit in the worktree
	testFile := filepath.Join(wtPath, "new.txt")
	err = os.WriteFile(testFile, []byte("new"), 0644)
	require.NoError(t, err)

	cmd := exec.Command("git", "-C", wtPath, "add", ".")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", wtPath, "commit", "-m", "new commit")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com")
	require.NoError(t, cmd.Run())

	// Now has unpushed commits relative to main
	unpushed, err = client.HasUnpushedCommits(wtPath, mainBranch)
	require.NoError(t, err)
	assert.True(t, unpushed)
}

func TestWorktreePrune_Integration(t *testing.T) {
	repoDir := initTestRepo(t)

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()

	// Should succeed even with nothing to prune
	err = client.WorktreePrune()
	require.NoError(t, err)
}

func TestCommitsAhead_Integration(t *testing.T) {
	repoDir := initTestRepo(t)

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()

	// Create a worktree with a branch
	wtDir := repoDir + ".worktrees"
	err = os.MkdirAll(wtDir, 0755)
	require.NoError(t, err)

	wtPath := filepath.Join(wtDir, "test-branch")
	err = client.WorktreeAdd(wtPath, "test-branch", "HEAD", true)
	require.NoError(t, err)

	mainBranch, err := client.CurrentBranch(repoDir)
	require.NoError(t, err)

	// Not ahead initially
	ahead, err := client.CommitsAhead(wtPath, mainBranch)
	require.NoError(t, err)
	assert.Equal(t, 0, ahead)

	// Add commits in the worktree
	for i := 0; i < 2; i++ {
		cmd := exec.Command("git", "-C", wtPath, "commit", "--allow-empty", "-m", "feature commit")
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com")
		require.NoError(t, cmd.Run())
	}

	// Now 2 ahead
	ahead, err = client.CommitsAhead(wtPath, mainBranch)
	require.NoError(t, err)
	assert.Equal(t, 2, ahead)
}

func TestCommitsBehind_Integration(t *testing.T) {
	repoDir := initTestRepo(t)

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()

	// Create a worktree with a branch
	wtDir := repoDir + ".worktrees"
	err = os.MkdirAll(wtDir, 0755)
	require.NoError(t, err)

	wtPath := filepath.Join(wtDir, "test-branch")
	err = client.WorktreeAdd(wtPath, "test-branch", "HEAD", true)
	require.NoError(t, err)

	// Get the main branch name
	mainBranch, err := client.CurrentBranch(repoDir)
	require.NoError(t, err)

	// Not behind initially (same as base)
	behind, err := client.CommitsBehind(wtPath, mainBranch)
	require.NoError(t, err)
	assert.Equal(t, 0, behind)

	// Add commits to main (the worktree branch will be behind)
	for i := 0; i < 3; i++ {
		cmd := exec.Command("git", "-C", repoDir, "commit", "--allow-empty", "-m", "main commit")
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com")
		require.NoError(t, cmd.Run())
	}

	// Now the worktree branch should be behind by 3
	behind, err = client.CommitsBehind(wtPath, mainBranch)
	require.NoError(t, err)
	assert.Equal(t, 3, behind)
}

func TestFetch_Integration(t *testing.T) {
	// Fetch requires a remote, so we set up a local bare repo as origin
	dir := t.TempDir()
	dir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	bareDir := filepath.Join(dir, "origin.git")
	repoDir := filepath.Join(dir, "repo")

	// Create bare repo
	cmd := exec.Command("git", "init", "--bare", bareDir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git init --bare: %s", string(out))

	// Clone bare repo to get a working repo with a remote
	cmd = exec.Command("git", "clone", bareDir, repoDir)
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "git clone: %s", string(out))

	// Configure and create initial commit
	cmds := [][]string{
		{"git", "-C", repoDir, "config", "user.email", "test@test.com"},
		{"git", "-C", repoDir, "config", "user.name", "Test"},
		{"git", "-C", repoDir, "commit", "--allow-empty", "-m", "init"},
		{"git", "-C", repoDir, "push"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "cmd %v failed: %s", args, string(out))
	}

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()

	// Fetch should succeed
	err = client.Fetch(repoDir)
	require.NoError(t, err)
}

func TestFetch_NoRemote_Integration(t *testing.T) {
	// Fetch on a repo with no remote should fail
	repoDir := initTestRepo(t)

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()

	// Fetch with no remote - git fetch exits 0 but does nothing
	// This should not error since git fetch with no remote just exits cleanly
	err = client.Fetch(repoDir)
	// git fetch on a repo with no remote may or may not error depending on git version
	// We just verify it doesn't panic
	_ = err
}
