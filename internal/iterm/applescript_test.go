package iterm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScriptCreateWorktreeWindow(t *testing.T) {
	script := ScriptCreateWorktreeWindow("/Users/joe/repo.worktrees/auth", "wt:repo:auth", false)

	assert.Contains(t, script, `cd '/Users/joe/repo.worktrees/auth' && claude`)
	assert.Contains(t, script, `"wt:repo:auth:claude"`)
	assert.Contains(t, script, `"wt:repo:auth:shell"`)
	assert.Contains(t, script, `split horizontally`)
	assert.Contains(t, script, `claudeID & "\t" & shellID`)
}

func TestScriptCreateWorktreeWindow_NoClaude(t *testing.T) {
	script := ScriptCreateWorktreeWindow("/Users/joe/repo.worktrees/auth", "wt:repo:auth", true)

	assert.NotContains(t, script, "&& claude")
	assert.Contains(t, script, `cd '/Users/joe/repo.worktrees/auth'`)
}

func TestScriptSessionExists(t *testing.T) {
	script := ScriptSessionExists("session-123")
	assert.Contains(t, script, `"session-123"`)
	assert.Contains(t, script, `return "true"`)
	assert.Contains(t, script, `return "false"`)
}

func TestScriptFocusWindow(t *testing.T) {
	script := ScriptFocusWindow("session-456")
	assert.Contains(t, script, `"session-456"`)
	assert.Contains(t, script, "activate")
}

func TestScriptCloseWindow(t *testing.T) {
	script := ScriptCloseWindow("session-789")
	assert.Contains(t, script, `"session-789"`)
	assert.Contains(t, script, "close w")
}

func TestEscapeAppleScript(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`normal`, `normal`},
		{`has "quotes"`, `has \"quotes\"`},
		{`has \backslash`, `has \\backslash`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeAppleScript(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestScriptIsRunning(t *testing.T) {
	script := ScriptIsRunning()
	assert.True(t, strings.Contains(script, "iTerm2"))
}
