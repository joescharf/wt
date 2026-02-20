package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// TrustManager manages Claude Code project trust entries in ~/.claude.json.
type TrustManager struct {
	path string
}

// NewTrustManager creates a TrustManager that reads/writes the given path.
func NewTrustManager(path string) *TrustManager {
	return &TrustManager{path: path}
}

// DefaultPath returns the default path to ~/.claude.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude.json"), nil
}

// Path returns the file path this manager operates on.
func (m *TrustManager) Path() string {
	return m.path
}

// TrustProject ensures the given project path has trust flags set in the config.
// Returns true if the entry was added or modified, false if already trusted.
func (m *TrustManager) TrustProject(projectPath string) (bool, error) {
	top, err := m.loadRaw()
	if err != nil {
		return false, err
	}

	key := encodeProjectKey(projectPath)

	// Get or create the projects map
	projects, err := getOrCreateMap(top, "projects")
	if err != nil {
		return false, err
	}

	// Get or create the project entry
	project, err := getOrCreateMap(projects, key)
	if err != nil {
		return false, err
	}

	// Check if already trusted
	if hasTrust(project, "hasTrustDialogAccepted") && hasTrust(project, "hasTrustDialogHooksAccepted") {
		return false, nil
	}

	// Set trust flags
	trueVal, _ := json.Marshal(true)
	project["hasTrustDialogAccepted"] = trueVal
	project["hasTrustDialogHooksAccepted"] = trueVal

	// Write back
	projectData, err := json.Marshal(project)
	if err != nil {
		return false, err
	}
	projects[key] = projectData

	projectsData, err := json.Marshal(projects)
	if err != nil {
		return false, err
	}
	top["projects"] = projectsData

	return true, m.saveRaw(top)
}

// UntrustProject removes the project entry from the config.
func (m *TrustManager) UntrustProject(projectPath string) error {
	top, err := m.loadRaw()
	if err != nil {
		return err
	}

	key := encodeProjectKey(projectPath)

	projectsRaw, ok := top["projects"]
	if !ok {
		return nil
	}

	var projects map[string]json.RawMessage
	if err := json.Unmarshal(projectsRaw, &projects); err != nil {
		return nil // not a valid projects map, nothing to do
	}

	if _, exists := projects[key]; !exists {
		return nil
	}

	delete(projects, key)

	projectsData, err := json.Marshal(projects)
	if err != nil {
		return err
	}
	top["projects"] = projectsData

	return m.saveRaw(top)
}

// PruneProjects removes project entries whose paths are under worktreesDir
// and whose directories no longer exist on disk. Returns the number pruned.
func (m *TrustManager) PruneProjects(worktreesDir string) (int, error) {
	top, err := m.loadRaw()
	if err != nil {
		return 0, err
	}

	projectsRaw, ok := top["projects"]
	if !ok {
		return 0, nil
	}

	var projects map[string]json.RawMessage
	if err := json.Unmarshal(projectsRaw, &projects); err != nil {
		return 0, nil
	}

	pruned := 0
	for key := range projects {
		projectPath := decodeProjectKey(key)
		if !strings.HasPrefix(projectPath, worktreesDir+string(filepath.Separator)) {
			continue
		}
		if _, err := os.Stat(projectPath); os.IsNotExist(err) {
			delete(projects, key)
			pruned++
		}
	}

	if pruned > 0 {
		projectsData, err := json.Marshal(projects)
		if err != nil {
			return pruned, err
		}
		top["projects"] = projectsData
		return pruned, m.saveRaw(top)
	}

	return 0, nil
}

// loadRaw reads the config file into a top-level map preserving all fields.
func (m *TrustManager) loadRaw() (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]json.RawMessage), nil
		}
		return nil, err
	}

	// Handle empty file
	if len(data) == 0 {
		return make(map[string]json.RawMessage), nil
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	return top, nil
}

// saveRaw writes the config map to disk using atomic write (temp + rename).
func (m *TrustManager) saveRaw(top map[string]json.RawMessage) error {
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return err
	}

	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

// encodeProjectKey converts an absolute path to the Claude config key format.
// Claude uses the path with "/" replaced by "," as the key under "projects".
func encodeProjectKey(path string) string {
	return strings.ReplaceAll(path, "/", ",")
}

// decodeProjectKey reverses encodeProjectKey.
func decodeProjectKey(key string) string {
	return strings.ReplaceAll(key, ",", "/")
}

// getOrCreateMap retrieves a nested map from a RawMessage map, creating it if absent.
func getOrCreateMap(parent map[string]json.RawMessage, key string) (map[string]json.RawMessage, error) {
	raw, ok := parent[key]
	if !ok || string(raw) == "null" {
		return make(map[string]json.RawMessage), nil
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// hasTrust checks if a project entry has a specific boolean field set to true.
func hasTrust(project map[string]json.RawMessage, field string) bool {
	raw, ok := project[field]
	if !ok {
		return false
	}
	var val bool
	if err := json.Unmarshal(raw, &val); err != nil {
		return false
	}
	return val
}
