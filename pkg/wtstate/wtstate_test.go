package wtstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	mgr := NewManager(statePath)

	now := time.Now().UTC().Truncate(time.Second)
	ws := &WorktreeState{
		Repo:            "myrepo",
		Branch:          "feature/auth",
		ClaudeSessionID: "session-123",
		ShellSessionID:  "session-456",
		CreatedAt:       FlexTime{Time: now},
	}

	// Set a worktree entry
	err := mgr.SetWorktree("/tmp/myrepo.worktrees/auth", ws)
	require.NoError(t, err)

	// Read it back
	got, err := mgr.GetWorktree("/tmp/myrepo.worktrees/auth")
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "myrepo", got.Repo)
	assert.Equal(t, "feature/auth", got.Branch)
	assert.Equal(t, "session-123", got.ClaudeSessionID)
	assert.Equal(t, "session-456", got.ShellSessionID)
	assert.Equal(t, now.Unix(), got.CreatedAt.Time.Unix())

	// Verify nil for missing entry
	missing, err := mgr.GetWorktree("/nonexistent")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestStateRemoveWorktree(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	mgr := NewManager(statePath)

	ws := &WorktreeState{
		Repo:   "myrepo",
		Branch: "feature/auth",
	}
	err := mgr.SetWorktree("/tmp/path", ws)
	require.NoError(t, err)

	err = mgr.RemoveWorktree("/tmp/path")
	require.NoError(t, err)

	got, err := mgr.GetWorktree("/tmp/path")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStatePrune(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	mgr := NewManager(statePath)

	// Create a real directory for one entry
	realDir := filepath.Join(dir, "real-worktree")
	err := os.MkdirAll(realDir, 0755)
	require.NoError(t, err)

	// Add two entries: one with a real dir, one with a fake dir
	err = mgr.SetWorktree(realDir, &WorktreeState{Repo: "repo", Branch: "real"})
	require.NoError(t, err)
	err = mgr.SetWorktree("/tmp/nonexistent-worktree-path-12345", &WorktreeState{Repo: "repo", Branch: "stale"})
	require.NoError(t, err)

	pruned, err := mgr.Prune()
	require.NoError(t, err)
	assert.Equal(t, 1, pruned)

	// Real entry should survive
	s, err := mgr.Load()
	require.NoError(t, err)
	assert.Len(t, s.Worktrees, 1)
	assert.NotNil(t, s.Worktrees[realDir])
}

func TestLoadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	mgr := NewManager(statePath)

	// No file exists yet
	s, err := mgr.Load()
	require.NoError(t, err)
	assert.NotNil(t, s.Worktrees)
	assert.Empty(t, s.Worktrees)
}
