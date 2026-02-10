package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigInit_CreatesFile(t *testing.T) {
	env := setupTest(t)
	configDirFunc = func() (string, error) { return env.dir, nil }

	err := configInitRun()
	require.NoError(t, err)

	cfgPath := filepath.Join(env.dir, "config.yaml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "base_branch: main")
	assert.Contains(t, content, "rebase: false")
	assert.Contains(t, content, "no_claude: false")
	assert.Contains(t, content, "# state_dir:")
	assert.Contains(t, env.out.String(), "Config file created")
}

func TestConfigInit_ExistingFile_NoForce(t *testing.T) {
	env := setupTest(t)
	configDirFunc = func() (string, error) { return env.dir, nil }

	// Create existing file
	cfgPath := filepath.Join(env.dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("existing"), 0644))

	err := configInitRun()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file already exists")
	assert.Contains(t, err.Error(), "--force")

	// File should be untouched
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "existing", string(data))
}

func TestConfigInit_ExistingFile_WithForce(t *testing.T) {
	env := setupTest(t)
	configDirFunc = func() (string, error) { return env.dir, nil }

	// Create existing file
	cfgPath := filepath.Join(env.dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("existing"), 0644))

	configForce = true
	err := configInitRun()
	require.NoError(t, err)

	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "base_branch: main")
	assert.Contains(t, env.err.String(), "Overwriting existing config file")
}

func TestConfigInit_DryRun(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	output.DryRun = true
	configDirFunc = func() (string, error) { return env.dir, nil }

	err := configInitRun()
	require.NoError(t, err)

	// File should NOT be created
	cfgPath := filepath.Join(env.dir, "config.yaml")
	_, err = os.Stat(cfgPath)
	assert.True(t, os.IsNotExist(err))

	// Output should contain the template contents
	assert.Contains(t, env.out.String(), "base_branch: main")
	assert.Contains(t, env.err.String(), "DRY-RUN")
}

func TestConfigInit_ReflectsCurrentValues(t *testing.T) {
	env := setupTest(t)
	configDirFunc = func() (string, error) { return env.dir, nil }

	// Override viper values
	viper.Set("base_branch", "develop")
	viper.Set("rebase", true)
	viper.Set("no_claude", true)

	err := configInitRun()
	require.NoError(t, err)

	cfgPath := filepath.Join(env.dir, "config.yaml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "base_branch: develop")
	assert.Contains(t, content, "rebase: true")
	assert.Contains(t, content, "no_claude: true")
}

func TestConfigShow_DefaultsOnly(t *testing.T) {
	env := setupTest(t)
	configDirFunc = func() (string, error) { return env.dir, nil }

	err := configShowRun()
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Config file: (none)")
	assert.Contains(t, out, "base_branch")
	assert.Contains(t, out, "(default)")
}

func TestConfigShow_WithConfigFile(t *testing.T) {
	env := setupTest(t)
	configDirFunc = func() (string, error) { return env.dir, nil }

	// Create a YAML config file with one key
	cfgPath := filepath.Join(env.dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("base_branch: develop\n"), 0644))

	err := configShowRun()
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "Config file:")
	assert.Contains(t, out, cfgPath)
	assert.Contains(t, out, "base_branch")
	assert.Contains(t, out, "(file)")
	// Other keys should still show default
	assert.Contains(t, out, "(default)")
}

func TestConfigShow_WithEnvVar(t *testing.T) {
	env := setupTest(t)
	configDirFunc = func() (string, error) { return env.dir, nil }

	t.Setenv("WT_REBASE", "true")

	err := configShowRun()
	require.NoError(t, err)

	out := env.out.String()
	assert.Contains(t, out, "(env: WT_REBASE)")
}

func TestConfigEdit_NoEditor(t *testing.T) {
	setupTest(t)

	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	err := configEditRun()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "$EDITOR is not set")
}

func TestConfigEdit_NoConfigFile(t *testing.T) {
	env := setupTest(t)
	configDirFunc = func() (string, error) { return env.dir, nil }

	t.Setenv("EDITOR", "vim")

	err := configEditRun()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file not found")
	assert.Contains(t, err.Error(), "wt config init")
}
