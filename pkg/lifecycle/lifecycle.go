package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/joescharf/wt/pkg/claude"
	"github.com/joescharf/wt/pkg/gitops"
	"github.com/joescharf/wt/pkg/iterm"
	"github.com/joescharf/wt/pkg/ops"
	state "github.com/joescharf/wt/pkg/wtstate"
)

// Manager orchestrates worktree lifecycle operations: create, open, and delete.
// It combines git, iTerm, state, and trust management into cohesive workflows.
type Manager struct {
	git   gitops.Client
	iterm iterm.Client
	state *state.Manager
	trust *claude.TrustManager // nil-safe
	log   ops.Logger
}

// NewManager creates a lifecycle Manager with the given dependencies.
// trust may be nil if Claude trust management is not configured.
func NewManager(git gitops.Client, it iterm.Client, sm *state.Manager, trust *claude.TrustManager, log ops.Logger) *Manager {
	return &Manager{
		git:   git,
		iterm: it,
		state: sm,
		trust: trust,
		log:   log,
	}
}

// CreateOptions configures a worktree create operation.
type CreateOptions struct {
	RepoPath   string // root of the main repository
	Branch     string // branch name to create
	BaseBranch string // base branch (e.g., "main")
	NoClaude   bool   // don't auto-launch claude in top pane
	Existing   bool   // use existing branch instead of creating new
	DryRun     bool
}

// CreateResult describes the outcome of a create operation.
type CreateResult struct {
	WtPath    string
	Branch    string
	RepoName  string
	Created   bool // false if delegated to Open (already existed)
	SessionID string
}

// Create creates a new worktree with an iTerm2 window.
// If the worktree already exists, it delegates to Open (idempotent).
func (m *Manager) Create(opts CreateOptions) (*CreateResult, error) {
	repoName, err := m.git.RepoName(opts.RepoPath)
	if err != nil {
		return nil, err
	}

	wtDir, err := m.git.WorktreesDir(opts.RepoPath)
	if err != nil {
		return nil, err
	}

	dirname := gitops.BranchToDirname(opts.Branch)
	wtPath := filepath.Join(wtDir, dirname)

	m.log.Info("Creating worktree for branch '%s' in repo '%s'", opts.Branch, repoName)
	m.log.Verbose("Worktrees dir: %s", wtDir)
	m.log.Verbose("Worktree path: %s", wtPath)
	m.log.Verbose("Base branch: %s", opts.BaseBranch)

	// If worktree already exists, delegate to open
	if isDirectory(wtPath) {
		m.log.Info("Worktree already exists, opening iTerm2 window")
		openResult, err := m.Open(OpenOptions{
			RepoPath: opts.RepoPath,
			WtPath:   wtPath,
			Branch:   opts.Branch,
			NoClaude: opts.NoClaude,
			DryRun:   opts.DryRun,
		})
		if err != nil {
			return nil, err
		}
		return &CreateResult{
			WtPath:    openResult.WtPath,
			Branch:    openResult.Branch,
			RepoName:  repoName,
			Created:   false,
			SessionID: openResult.SessionID,
		}, nil
	}

	// Create worktrees directory if needed
	if !isDirectory(wtDir) {
		if opts.DryRun {
			m.log.Info("Would create worktrees directory: %s", wtDir)
		} else {
			if err := os.MkdirAll(wtDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create worktrees directory: %w", err)
			}
			m.log.Success("Created worktrees directory")
		}
	}

	// Check if branch already exists (auto-detect)
	branchExists, err := m.git.BranchExists(opts.RepoPath, opts.Branch)
	if err != nil {
		return nil, err
	}

	useExisting := opts.Existing || branchExists
	if branchExists && !opts.Existing {
		m.log.Info("Branch '%s' already exists, using it", opts.Branch)
	}

	if opts.DryRun {
		if useExisting {
			m.log.Info("Would create worktree from existing branch '%s'", opts.Branch)
		} else {
			m.log.Info("Would create worktree with new branch '%s' from '%s'", opts.Branch, opts.BaseBranch)
		}
		m.log.Info("Would create iTerm2 window for %s", wtPath)
		m.log.Info("Would save state")
		return &CreateResult{WtPath: wtPath, Branch: opts.Branch, RepoName: repoName}, nil
	}

	// Create worktree
	if useExisting {
		m.log.Info("Creating worktree from existing branch '%s'", opts.Branch)
		err = m.git.WorktreeAdd(opts.RepoPath, wtPath, opts.Branch, "", false)
	} else {
		m.log.Info("Creating worktree with new branch '%s' from '%s'", opts.Branch, opts.BaseBranch)
		err = m.git.WorktreeAdd(opts.RepoPath, wtPath, opts.Branch, opts.BaseBranch, true)
	}
	if err != nil {
		return nil, err
	}
	m.log.Success("Git worktree created")

	// Pre-approve Claude Code trust
	m.trustProject(wtPath)

	// Create iTerm2 window
	sessionName := fmt.Sprintf("wt:%s:%s", repoName, dirname)
	m.log.Info("Creating iTerm2 window (session: %s)", sessionName)

	sessions, err := m.iterm.CreateWorktreeWindow(wtPath, sessionName, opts.NoClaude)
	if err != nil {
		m.log.Warning("Worktree created but failed to open iTerm2 window: %v", err)
		m.log.Info("Use 'wt open %s' to try again", opts.Branch)
		return &CreateResult{WtPath: wtPath, Branch: opts.Branch, RepoName: repoName, Created: true}, nil
	}

	m.log.Verbose("Claude session: %s", sessions.ClaudeSessionID)
	m.log.Verbose("Shell session:  %s", sessions.ShellSessionID)

	// Save state
	if err := m.state.SetWorktree(wtPath, &state.WorktreeState{
		Repo:            repoName,
		Branch:          opts.Branch,
		ClaudeSessionID: sessions.ClaudeSessionID,
		ShellSessionID:  sessions.ShellSessionID,
		CreatedAt:       state.FlexTime{Time: time.Now().UTC()},
	}); err != nil {
		m.log.Warning("Failed to save state: %v", err)
	}

	m.log.Success("Worktree ready: %s", wtPath)
	m.log.Success("iTerm2 window opened with Claude + shell panes")

	return &CreateResult{
		WtPath:    wtPath,
		Branch:    opts.Branch,
		RepoName:  repoName,
		Created:   true,
		SessionID: sessions.ClaudeSessionID,
	}, nil
}

// OpenOptions configures a worktree open operation.
type OpenOptions struct {
	RepoPath string // for RepoName fallback
	WtPath   string // resolved worktree filesystem path
	Branch   string // branch name (for state lookup)
	NoClaude bool
	DryRun   bool
}

// OpenResult describes the outcome of an open operation.
type OpenResult struct {
	WtPath    string
	Branch    string
	SessionID string
	Focused   bool // true if an existing window was focused instead of creating new
}

// Open opens or focuses an iTerm2 window for an existing worktree.
func (m *Manager) Open(opts OpenOptions) (*OpenResult, error) {
	repoName, err := m.git.RepoName(opts.RepoPath)
	if err != nil {
		return nil, err
	}

	dirname := filepath.Base(opts.WtPath)

	// Check if window already exists
	ws, err := m.state.GetWorktree(opts.WtPath)
	if err != nil {
		return nil, err
	}
	if ws != nil && ws.ClaudeSessionID != "" {
		if m.iterm.IsRunning() && m.iterm.SessionExists(ws.ClaudeSessionID) {
			m.log.Info("iTerm2 window already open, focusing it")
			if err := m.iterm.FocusWindow(ws.ClaudeSessionID); err != nil {
				return nil, err
			}
			return &OpenResult{WtPath: opts.WtPath, Branch: opts.Branch, SessionID: ws.ClaudeSessionID, Focused: true}, nil
		}
	}

	if opts.DryRun {
		m.log.Info("Would open iTerm2 window for %s", opts.WtPath)
		return &OpenResult{WtPath: opts.WtPath, Branch: opts.Branch}, nil
	}

	// Pre-approve Claude Code trust
	m.trustProject(opts.WtPath)

	sessionName := fmt.Sprintf("wt:%s:%s", repoName, dirname)
	m.log.Info("Opening iTerm2 window for '%s'", dirname)

	sessions, err := m.iterm.CreateWorktreeWindow(opts.WtPath, sessionName, opts.NoClaude)
	if err != nil {
		return nil, fmt.Errorf("failed to create iTerm2 window: %w", err)
	}

	// Get branch from state or git
	branchName := opts.Branch
	if ws != nil && ws.Branch != "" {
		branchName = ws.Branch
	} else {
		if b, err := m.git.CurrentBranch(opts.WtPath); err == nil {
			branchName = b
		}
	}

	if err := m.state.SetWorktree(opts.WtPath, &state.WorktreeState{
		Repo:            repoName,
		Branch:          branchName,
		ClaudeSessionID: sessions.ClaudeSessionID,
		ShellSessionID:  sessions.ShellSessionID,
		CreatedAt:       state.FlexTime{Time: time.Now().UTC()},
	}); err != nil {
		m.log.Warning("Window opened but failed to save state: %v", err)
	}

	m.log.Success("iTerm2 window opened for '%s'", dirname)
	return &OpenResult{WtPath: opts.WtPath, Branch: branchName, SessionID: sessions.ClaudeSessionID}, nil
}

// DeleteOptions configures a worktree delete operation.
type DeleteOptions struct {
	RepoPath     string // root of the main repository
	WtPath       string // resolved worktree filesystem path
	Branch       string // resolved branch name
	Force        bool   // force removal
	DeleteBranch bool   // also delete the git branch
	DryRun       bool
}

// Delete performs full worktree cleanup: close iTerm window, remove worktree,
// optionally delete branch (with force fallback), remove state and trust entries.
func (m *Manager) Delete(opts DeleteOptions) error {
	dirname := filepath.Base(opts.WtPath)

	// Close iTerm2 window if it exists
	ws, _ := m.state.GetWorktree(opts.WtPath)
	if ws != nil && ws.ClaudeSessionID != "" {
		if opts.DryRun {
			m.log.Info("Would close iTerm2 window")
		} else if m.iterm.IsRunning() && m.iterm.SessionExists(ws.ClaudeSessionID) {
			if err := m.iterm.CloseWindow(ws.ClaudeSessionID); err != nil {
				m.log.Warning("Failed to close iTerm2 window: %v", err)
			} else {
				m.log.Success("Closed iTerm2 window")
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

	// Remove worktree
	if opts.DryRun {
		m.log.Info("Would remove git worktree: %s", opts.WtPath)
	} else {
		m.log.Info("Removing git worktree")
		if err := m.git.WorktreeRemove(opts.RepoPath, opts.WtPath, opts.Force); err != nil {
			return err
		}
		m.log.Success("Removed git worktree")
	}

	// Delete branch if requested
	if opts.DeleteBranch {
		branchName := opts.Branch
		if ws != nil && ws.Branch != "" {
			branchName = ws.Branch
		}

		if opts.DryRun {
			m.log.Info("Would delete branch '%s'", branchName)
		} else {
			err := m.git.BranchDelete(opts.RepoPath, branchName, false)
			if err != nil {
				if opts.Force {
					err = m.git.BranchDelete(opts.RepoPath, branchName, true)
					if err == nil {
						m.log.Success("Force-deleted branch '%s'", branchName)
					} else {
						m.log.Warning("Could not delete branch '%s': %v", branchName, err)
					}
				} else {
					m.log.Warning("Could not delete branch '%s' (may not exist or not fully merged)", branchName)
				}
			} else {
				m.log.Success("Deleted branch '%s'", branchName)
			}
		}
	}

	// Remove state and trust entries
	if !opts.DryRun {
		_ = m.state.RemoveWorktree(opts.WtPath)

		if m.trust != nil {
			if err := m.trust.UntrustProject(opts.WtPath); err != nil {
				m.log.Warning("Failed to remove Claude trust: %v", err)
			}
		}
	}

	m.log.Success("Worktree '%s' removed", dirname)
	return nil
}

// trustProject pre-approves Claude Code trust for a worktree directory.
func (m *Manager) trustProject(wtPath string) {
	if m.trust == nil {
		return
	}
	added, err := m.trust.TrustProject(wtPath)
	if err != nil {
		m.log.Warning("Failed to set Claude trust: %v", err)
	} else if added {
		m.log.Verbose("Claude trust set for %s", wtPath)
	}
}

// isDirectory checks if a path exists and is a directory.
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
