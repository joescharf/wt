package iterm

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SessionIDs holds the iTerm2 session IDs for a worktree window.
type SessionIDs struct {
	ClaudeSessionID string
	ShellSessionID  string
}

// Client defines the interface for iTerm2 operations.
type Client interface {
	IsRunning() bool
	EnsureRunning() error
	CreateWorktreeWindow(path, name string, noClaude bool) (*SessionIDs, error)
	SessionExists(sessionID string) bool
	FocusWindow(sessionID string) error
	CloseWindow(sessionID string) error
}

// RealClient implements Client using osascript.
type RealClient struct{}

// NewClient returns a new RealClient.
func NewClient() *RealClient {
	return &RealClient{}
}

func (c *RealClient) IsRunning() bool {
	out, err := exec.Command("osascript", "-e", ScriptIsRunning()).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func (c *RealClient) EnsureRunning() error {
	if c.IsRunning() {
		return nil
	}

	if err := exec.Command("open", "-a", "iTerm").Start(); err != nil {
		return fmt.Errorf("failed to launch iTerm2: %w", err)
	}

	// Wait up to 10 seconds
	for i := 0; i < 20; i++ {
		if c.IsRunning() {
			time.Sleep(1 * time.Second) // extra settle time
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timed out waiting for iTerm2 to start")
}

func (c *RealClient) CreateWorktreeWindow(path, name string, noClaude bool) (*SessionIDs, error) {
	if err := c.EnsureRunning(); err != nil {
		return nil, err
	}

	script := ScriptCreateWorktreeWindow(path, name, noClaude)
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to create iTerm2 window: %w", err)
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), "\t", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("unexpected osascript output: %s", string(out))
	}

	return &SessionIDs{
		ClaudeSessionID: parts[0],
		ShellSessionID:  parts[1],
	}, nil
}

func (c *RealClient) SessionExists(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	out, err := exec.Command("osascript", "-e", ScriptSessionExists(sessionID)).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func (c *RealClient) FocusWindow(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("empty session ID")
	}
	_, err := exec.Command("osascript", "-e", ScriptFocusWindow(sessionID)).Output()
	return err
}

func (c *RealClient) CloseWindow(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("empty session ID")
	}
	_, err := exec.Command("osascript", "-e", ScriptCloseWindow(sessionID)).Output()
	return err
}
