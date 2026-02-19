package wtstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// FlexTime wraps time.Time with flexible JSON parsing that accepts
// both RFC3339 ("2006-01-02T15:04:05Z") and bare datetime ("2006-01-02T15:04:05").
type FlexTime struct {
	time.Time
}

func (ft FlexTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(ft.Time.UTC().Format(time.RFC3339))
}

func (ft *FlexTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "" {
		ft.Time = time.Time{}
		return nil
	}
	// Try RFC3339 first, then fall back to bare datetime (assume UTC)
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05", s)
		if err != nil {
			return err
		}
		t = t.UTC()
	}
	ft.Time = t
	return nil
}

// WorktreeState holds the persisted state for a single worktree.
type WorktreeState struct {
	Repo            string   `json:"repo"`
	Branch          string   `json:"branch"`
	ClaudeSessionID string   `json:"claude_session_id"`
	ShellSessionID  string   `json:"shell_session_id"`
	CreatedAt       FlexTime `json:"created_at"`
}

// State is the top-level state file structure.
type State struct {
	Worktrees map[string]*WorktreeState `json:"worktrees"`
}

// Manager handles reading and writing state to disk.
type Manager struct {
	path string
}

// NewManager creates a Manager that reads/writes state at the given path.
func NewManager(path string) *Manager {
	return &Manager{path: path}
}

// Path returns the state file path.
func (m *Manager) Path() string {
	return m.path
}

// Load reads the state file from disk. Returns an empty state if the file does not exist.
func (m *Manager) Load() (*State, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Worktrees: make(map[string]*WorktreeState)}, nil
		}
		return nil, err
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Worktrees == nil {
		s.Worktrees = make(map[string]*WorktreeState)
	}
	return &s, nil
}

// Save writes the state to disk using atomic write (temp file + rename).
func (m *Manager) Save(s *State) error {
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

// SetWorktree adds or updates a worktree entry.
func (m *Manager) SetWorktree(path string, ws *WorktreeState) error {
	s, err := m.Load()
	if err != nil {
		return err
	}
	s.Worktrees[path] = ws
	return m.Save(s)
}

// RemoveWorktree removes a worktree entry by path.
func (m *Manager) RemoveWorktree(path string) error {
	s, err := m.Load()
	if err != nil {
		return err
	}
	delete(s.Worktrees, path)
	return m.Save(s)
}

// GetWorktree returns the state for a worktree path, or nil if not found.
func (m *Manager) GetWorktree(path string) (*WorktreeState, error) {
	s, err := m.Load()
	if err != nil {
		return nil, err
	}
	return s.Worktrees[path], nil
}

// Prune removes entries for worktree paths that no longer exist on disk.
// Returns the number of entries pruned.
func (m *Manager) Prune() (int, error) {
	s, err := m.Load()
	if err != nil {
		return 0, err
	}

	pruned := 0
	for path := range s.Worktrees {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			delete(s.Worktrees, path)
			pruned++
		}
	}

	if pruned > 0 {
		if err := m.Save(s); err != nil {
			return pruned, err
		}
	}
	return pruned, nil
}
