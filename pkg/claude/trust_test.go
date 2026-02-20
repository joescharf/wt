package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrustProject_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	mgr := NewTrustManager(path)

	added, err := mgr.TrustProject("/Users/joe/worktrees/auth")
	require.NoError(t, err)
	assert.True(t, added)

	// Verify file contents
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &top))

	var projects map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["projects"], &projects))

	key := ",Users,joe,worktrees,auth"
	var project map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(projects[key], &project))

	var trusted bool
	require.NoError(t, json.Unmarshal(project["hasTrustDialogAccepted"], &trusted))
	assert.True(t, trusted)

	require.NoError(t, json.Unmarshal(project["hasTrustDialogHooksAccepted"], &trusted))
	assert.True(t, trusted)
}

func TestTrustProject_PreservesTopLevelFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")

	// Write an existing file with other fields
	existing := `{
  "numStartups": 42,
  "hasCompletedOnboarding": true,
  "projects": {}
}`
	require.NoError(t, os.WriteFile(path, []byte(existing), 0644))

	mgr := NewTrustManager(path)
	added, err := mgr.TrustProject("/Users/joe/worktrees/auth")
	require.NoError(t, err)
	assert.True(t, added)

	// Verify top-level fields preserved
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &top))

	var numStartups int
	require.NoError(t, json.Unmarshal(top["numStartups"], &numStartups))
	assert.Equal(t, 42, numStartups)

	var onboarded bool
	require.NoError(t, json.Unmarshal(top["hasCompletedOnboarding"], &onboarded))
	assert.True(t, onboarded)
}

func TestTrustProject_PreservesExistingProjectFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")

	key := ",Users,joe,worktrees,auth"
	existing := `{
  "projects": {
    "` + key + `": {
      "allowedTools": ["Edit", "Read"],
      "hasTrustDialogAccepted": false
    }
  }
}`
	require.NoError(t, os.WriteFile(path, []byte(existing), 0644))

	mgr := NewTrustManager(path)
	added, err := mgr.TrustProject("/Users/joe/worktrees/auth")
	require.NoError(t, err)
	assert.True(t, added)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &top))

	var projects map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["projects"], &projects))

	var project map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(projects[key], &project))

	// Trust flags set
	var trusted bool
	require.NoError(t, json.Unmarshal(project["hasTrustDialogAccepted"], &trusted))
	assert.True(t, trusted)

	// allowedTools preserved
	var tools []string
	require.NoError(t, json.Unmarshal(project["allowedTools"], &tools))
	assert.Equal(t, []string{"Edit", "Read"}, tools)
}

func TestTrustProject_AlreadyTrusted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")

	key := ",Users,joe,worktrees,auth"
	existing := `{
  "projects": {
    "` + key + `": {
      "hasTrustDialogAccepted": true,
      "hasTrustDialogHooksAccepted": true
    }
  }
}`
	require.NoError(t, os.WriteFile(path, []byte(existing), 0644))

	mgr := NewTrustManager(path)
	added, err := mgr.TrustProject("/Users/joe/worktrees/auth")
	require.NoError(t, err)
	assert.False(t, added, "should return false when already trusted")
}

func TestTrustProject_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	require.NoError(t, os.WriteFile(path, []byte(""), 0644))

	mgr := NewTrustManager(path)
	added, err := mgr.TrustProject("/Users/joe/worktrees/auth")
	require.NoError(t, err)
	assert.True(t, added)

	// Verify file is valid JSON
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &top))
	assert.Contains(t, string(top["projects"]), "hasTrustDialogAccepted")
}

func TestUntrustProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	mgr := NewTrustManager(path)

	// First trust it
	_, err := mgr.TrustProject("/Users/joe/worktrees/auth")
	require.NoError(t, err)

	// Then untrust
	err = mgr.UntrustProject("/Users/joe/worktrees/auth")
	require.NoError(t, err)

	// Verify entry is gone
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &top))

	var projects map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["projects"], &projects))

	key := ",Users,joe,worktrees,auth"
	_, exists := projects[key]
	assert.False(t, exists, "project entry should be removed")
}

func TestUntrustProject_NonExistent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	mgr := NewTrustManager(path)

	// Untrust on non-existent file should not error
	err := mgr.UntrustProject("/Users/joe/worktrees/auth")
	require.NoError(t, err)
}

func TestPruneProjects(t *testing.T) {
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)

	path := filepath.Join(dir, ".claude.json")
	mgr := NewTrustManager(path)

	worktreesDir := filepath.Join(dir, "repo.worktrees")
	existingPath := filepath.Join(worktreesDir, "auth")
	stalePath := filepath.Join(worktreesDir, "stale-branch")
	outsidePath := "/other/project"

	// Create the existing worktree dir
	require.NoError(t, os.MkdirAll(existingPath, 0755))

	// Trust all three
	_, err := mgr.TrustProject(existingPath)
	require.NoError(t, err)
	_, err = mgr.TrustProject(stalePath)
	require.NoError(t, err)
	_, err = mgr.TrustProject(outsidePath)
	require.NoError(t, err)

	// Prune
	pruned, err := mgr.PruneProjects(worktreesDir)
	require.NoError(t, err)
	assert.Equal(t, 1, pruned, "should prune only the stale entry under worktreesDir")

	// Verify: existing still present, stale gone, outside preserved
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &top))

	var projects map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["projects"], &projects))

	assert.Contains(t, projects, encodeProjectKey(existingPath))
	assert.NotContains(t, projects, encodeProjectKey(stalePath))
	assert.Contains(t, projects, encodeProjectKey(outsidePath))
}

func TestPruneProjects_NothingToPrune(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	mgr := NewTrustManager(path)

	pruned, err := mgr.PruneProjects("/some/worktrees")
	require.NoError(t, err)
	assert.Equal(t, 0, pruned)
}

func TestDefaultPath(t *testing.T) {
	p, err := DefaultPath()
	require.NoError(t, err)
	assert.Contains(t, p, ".claude.json")
}
