package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/joescharf/wt/pkg/claude"
	"github.com/joescharf/wt/pkg/gitops"
	"github.com/joescharf/wt/pkg/iterm"
	"github.com/joescharf/wt/pkg/ops"
	"github.com/joescharf/wt/pkg/wtstate"
)

// Manager orchestrates worktree lifecycle operations:
// create (git worktree + trust + iTerm), open (focus/create iTerm),
// and delete (close iTerm + remove worktree + untrust + cleanup state).
type Manager struct {
	git    gitops.Client
	iterm  iterm.Client
	state  *wtstate.Manager
	trust  *claude.TrustManager
	logger ops.Logger
}

// NewManager creates a lifecycle manager. Any dependency can be nil
// (operations requiring it will be skipped or return an error).
func NewManager(git gitops.Client, itermClient iterm.Client, state *wtstate.Manager, trust *claude.TrustManager, logger ops.Logger) *Manager {
	if logger == nil {
		logger = &nopLogger{}
	}
	return &Manager{
		git:    git,
		iterm:  itermClient,
		state:  state,
		trust:  trust,
		logger: logger,
	}
}

// Accessor methods for dependency injection by consumers.
func (m *Manager) ITerm() iterm.Client         { return m.iterm }
func (m *Manager) State() *wtstate.Manager     { return m.state }
func (m *Manager) Trust() *claude.TrustManager { return m.trust }

// CreateOptions configures a Create operation.
type CreateOptions struct {
	Branch   string // Required: branch name
	Base     string // Base branch (default: "main")
	NoClaude bool   // Don't auto-launch claude in top pane
	Existing bool   // Use existing branch instead of creating new
}

// CreateResult holds the result of creating a worktree.
type CreateResult struct {
	WorktreePath string
	Branch       string
	RepoName     string
	SessionIDs   *iterm.SessionIDs
	Existed      bool // true if worktree already existed, just opened window
}

// Create creates a worktree with iTerm window, trust, and state management.
// If the worktree already exists, delegates to Open.
func (m *Manager) Create(ctx context.Context, opts CreateOptions) (*CreateResult, error) {
	if m.git == nil {
		return nil, fmt.Errorf("git client required for Create")
	}
	if opts.Branch == "" {
		return nil, fmt.Errorf("branch is required")
	}

	repoName, err := m.git.RepoName()
	if err != nil {
		return nil, fmt.Errorf("get repo name: %w", err)
	}

	wtDir, err := m.git.WorktreesDir()
	if err != nil {
		return nil, fmt.Errorf("get worktrees dir: %w", err)
	}

	dirname := gitops.BranchToDirname(opts.Branch)
	wtPath := filepath.Join(wtDir, dirname)

	// If worktree already exists, delegate to Open
	if info, err := os.Stat(wtPath); err == nil && info.IsDir() {
		m.logger.Info("Worktree already exists, opening window")
		openResult, err := m.Open(ctx, wtPath, OpenOptions{NoClaude: opts.NoClaude})
		if err != nil {
			return nil, err
		}
		return &CreateResult{
			WorktreePath: wtPath,
			Branch:       opts.Branch,
			RepoName:     repoName,
			SessionIDs:   openResult.SessionIDs,
			Existed:      true,
		}, nil
	}

	// Create worktrees directory if needed
	if _, err := os.Stat(wtDir); os.IsNotExist(err) {
		if err := os.MkdirAll(wtDir, 0755); err != nil {
			return nil, fmt.Errorf("create worktrees directory: %w", err)
		}
	}

	// Check if branch already exists
	branchExists, err := m.git.BranchExists(opts.Branch)
	if err != nil {
		return nil, fmt.Errorf("check branch: %w", err)
	}

	useExisting := opts.Existing || branchExists

	// Create git worktree
	base := opts.Base
	if base == "" {
		base = "main"
	}

	if useExisting {
		m.logger.Info("Creating worktree from existing branch '%s'", opts.Branch)
		err = m.git.WorktreeAdd(wtPath, opts.Branch, "", false)
	} else {
		m.logger.Info("Creating worktree with new branch '%s' from '%s'", opts.Branch, base)
		err = m.git.WorktreeAdd(wtPath, opts.Branch, base, true)
	}
	if err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}
	m.logger.Success("Git worktree created")

	// Pre-approve Claude Code trust
	if m.trust != nil {
		if added, trustErr := m.trust.TrustProject(wtPath); trustErr != nil {
			m.logger.Warning("Failed to set Claude trust: %v", trustErr)
		} else if added {
			m.logger.Verbose("Claude trust set for %s", wtPath)
		}
	}

	// Create iTerm2 window
	var sessionIDs *iterm.SessionIDs
	if m.iterm != nil {
		sessionName := fmt.Sprintf("wt:%s:%s", repoName, dirname)
		m.logger.Info("Creating iTerm2 window (session: %s)", sessionName)

		sessionIDs, err = m.iterm.CreateWorktreeWindow(wtPath, sessionName, opts.NoClaude)
		if err != nil {
			m.logger.Warning("Worktree created but failed to open iTerm2 window: %v", err)
		}
	}

	// Save wt state
	if m.state != nil && sessionIDs != nil {
		stateErr := m.state.SetWorktree(wtPath, &wtstate.WorktreeState{
			Repo:            repoName,
			Branch:          opts.Branch,
			ClaudeSessionID: sessionIDs.ClaudeSessionID,
			ShellSessionID:  sessionIDs.ShellSessionID,
			CreatedAt:       wtstate.FlexTime{Time: time.Now().UTC()},
		})
		if stateErr != nil {
			m.logger.Warning("Failed to save state: %v", stateErr)
		}
	}

	return &CreateResult{
		WorktreePath: wtPath,
		Branch:       opts.Branch,
		RepoName:     repoName,
		SessionIDs:   sessionIDs,
	}, nil
}

// DeleteOptions configures a Delete operation.
type DeleteOptions struct {
	Force        bool // Skip safety checks (dirty worktree, etc.)
	DeleteBranch bool // Also delete the git branch
	DryRun       bool // Show what would happen without doing it
}

// Delete performs full worktree cleanup: close iTerm -> remove git worktree ->
// untrust Claude -> remove wt state -> optionally delete branch.
func (m *Manager) Delete(ctx context.Context, worktreePath string, opts DeleteOptions) error {
	if m.git == nil {
		return fmt.Errorf("git client required for Delete")
	}

	dirname := filepath.Base(worktreePath)

	// 1. Close iTerm2 window (best-effort, before removing worktree)
	if m.iterm != nil && m.state != nil {
		ws, _ := m.state.GetWorktree(worktreePath)
		if ws != nil && ws.ClaudeSessionID != "" {
			if m.iterm.SessionExists(ws.ClaudeSessionID) {
				if err := m.iterm.CloseWindow(ws.ClaudeSessionID); err != nil {
					m.logger.Warning("Failed to close iTerm2 window: %v", err)
				} else {
					m.logger.Success("Closed iTerm2 window")
				}
			}
		}
	}

	// 2. Get branch name from state before removing
	branchName := dirname
	if m.state != nil {
		ws, _ := m.state.GetWorktree(worktreePath)
		if ws != nil && ws.Branch != "" {
			branchName = ws.Branch
		}
	}

	// 3. Remove git worktree
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		m.logger.Info("Worktree already removed: %s", worktreePath)
	} else if opts.DryRun {
		m.logger.Info("Would remove git worktree: %s", worktreePath)
	} else {
		if err := m.git.WorktreeRemove(worktreePath, opts.Force); err != nil {
			return fmt.Errorf("remove worktree: %w", err)
		}
		m.logger.Success("Removed git worktree '%s'", dirname)
	}

	// 4. Delete branch if requested
	if opts.DeleteBranch && !opts.DryRun {
		err := m.git.BranchDelete(branchName, false)
		if err != nil && opts.Force {
			err = m.git.BranchDelete(branchName, true)
		}
		if err != nil {
			m.logger.Warning("Could not delete branch '%s': %v", branchName, err)
		} else {
			m.logger.Success("Deleted branch '%s'", branchName)
		}
	}

	// 5. Remove wt state entry
	if !opts.DryRun && m.state != nil {
		_ = m.state.RemoveWorktree(worktreePath)
	}

	// 6. Remove Claude trust
	if !opts.DryRun && m.trust != nil {
		if err := m.trust.UntrustProject(worktreePath); err != nil {
			m.logger.Warning("Failed to remove Claude trust: %v", err)
		}
	}

	return nil
}

// OpenOptions configures an Open operation.
type OpenOptions struct {
	NoClaude bool // Don't auto-launch claude in top pane
}

// OpenResult holds the result of opening/focusing a worktree window.
type OpenResult struct {
	WorktreePath string
	SessionIDs   *iterm.SessionIDs
	Action       string // "focused" or "opened"
}

// Open focuses an existing iTerm window or creates a new one for the worktree.
func (m *Manager) Open(ctx context.Context, worktreePath string, opts OpenOptions) (*OpenResult, error) {
	if m.iterm == nil {
		return nil, fmt.Errorf("iTerm client required for Open")
	}

	dirname := filepath.Base(worktreePath)

	// Check if window already exists via state
	if m.state != nil {
		ws, _ := m.state.GetWorktree(worktreePath)
		if ws != nil && ws.ClaudeSessionID != "" {
			if m.iterm.SessionExists(ws.ClaudeSessionID) {
				m.logger.Info("iTerm2 window already open, focusing it")
				if err := m.iterm.FocusWindow(ws.ClaudeSessionID); err != nil {
					return nil, fmt.Errorf("focus window: %w", err)
				}
				return &OpenResult{
					WorktreePath: worktreePath,
					SessionIDs: &iterm.SessionIDs{
						ClaudeSessionID: ws.ClaudeSessionID,
						ShellSessionID:  ws.ShellSessionID,
					},
					Action: "focused",
				}, nil
			}
		}
	}

	// Pre-approve Claude trust
	if m.trust != nil {
		if added, err := m.trust.TrustProject(worktreePath); err != nil {
			m.logger.Warning("Failed to set Claude trust: %v", err)
		} else if added {
			m.logger.Verbose("Claude trust set for %s", worktreePath)
		}
	}

	// Create new iTerm window
	repoName := dirname // fallback
	if m.git != nil {
		if name, err := m.git.RepoName(); err == nil {
			repoName = name
		}
	}
	sessionName := fmt.Sprintf("wt:%s:%s", repoName, dirname)
	m.logger.Info("Opening iTerm2 window for '%s'", dirname)

	sessions, err := m.iterm.CreateWorktreeWindow(worktreePath, sessionName, opts.NoClaude)
	if err != nil {
		return nil, fmt.Errorf("create iTerm2 window: %w", err)
	}

	// Update wt state with new session IDs
	if m.state != nil {
		branchName := dirname
		if m.git != nil {
			if b, bErr := m.git.CurrentBranch(worktreePath); bErr == nil {
				branchName = b
			}
		}

		stateErr := m.state.SetWorktree(worktreePath, &wtstate.WorktreeState{
			Repo:            repoName,
			Branch:          branchName,
			ClaudeSessionID: sessions.ClaudeSessionID,
			ShellSessionID:  sessions.ShellSessionID,
			CreatedAt:       wtstate.FlexTime{Time: time.Now().UTC()},
		})
		if stateErr != nil {
			m.logger.Warning("Failed to save state: %v", stateErr)
		}
	}

	return &OpenResult{
		WorktreePath: worktreePath,
		SessionIDs:   sessions,
		Action:       "opened",
	}, nil
}

// nopLogger discards all log output.
type nopLogger struct{}

func (l *nopLogger) Info(format string, args ...interface{})    {}
func (l *nopLogger) Success(format string, args ...interface{}) {}
func (l *nopLogger) Warning(format string, args ...interface{}) {}
func (l *nopLogger) Error(format string, args ...interface{})   {}
func (l *nopLogger) Verbose(format string, args ...interface{}) {}
