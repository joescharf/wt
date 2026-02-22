package ops

import (
	"fmt"
	"os"

	"github.com/joescharf/wt/pkg/gitops"
)

// Logger is the output interface for ops functions.
// Implementations can route to CLI UI, MCP responses, or discard output.
type Logger interface {
	Info(format string, args ...interface{})
	Success(format string, args ...interface{})
	Warning(format string, args ...interface{})
	Verbose(format string, args ...interface{})
}

// SafetyCheckFunc checks whether a worktree is safe to delete.
// Returns true if safe (or user confirmed), false to skip.
type SafetyCheckFunc func(wtPath string) (safe bool, err error)

// CleanupFunc performs cleanup after worktree deletion (close iTerm, remove state, trust, etc.).
type CleanupFunc func(wtPath, branch string) error

// PRCreateFunc creates a pull request via external tooling (e.g., gh CLI).
// Takes a list of arguments and returns the PR output (URL) and any error.
type PRCreateFunc func(args []string) (string, error)

// StateChecker checks whether a worktree path is already managed in state.
type StateChecker func(path string) (managed bool, err error)

// StateAdopter adopts an unmanaged worktree into state.
type StateAdopter func(path, repo, branch string) error

// StatePruner prunes stale state entries, returning the count pruned.
type StatePruner func() (int, error)

// TrustPruner prunes stale trust entries under a directory, returning the count pruned.
// May be nil if trust management is not configured.
type TrustPruner func(dir string) (int, error)

// SyncOptions configures a single worktree sync operation.
type SyncOptions struct {
	RepoPath   string // root of the main repository
	BaseBranch string // base branch to sync from (e.g., "main")
	Branch     string // resolved branch name of the worktree
	WtPath     string // resolved worktree filesystem path
	Strategy   string // "merge" or "rebase"
	Force      bool   // skip dirty worktree safety check
	DryRun     bool
}

// SyncResult describes the outcome of a single sync operation.
type SyncResult struct {
	Branch        string
	Ahead         int
	Behind        int
	AlreadySynced bool
	Strategy      string
	Conflict      bool
	Skipped       bool
	SkipReason    string
	Success       bool
}

// MergeOptions configures a merge operation.
type MergeOptions struct {
	RepoPath   string // root of the main repository
	BaseBranch string // target branch (e.g., "main")
	Branch     string // resolved feature branch name
	WtPath     string // resolved worktree filesystem path
	Strategy   string // "merge" or "rebase"
	Force      bool   // skip safety checks
	DryRun     bool
	CreatePR   bool   // create PR instead of local merge
	NoCleanup  bool   // keep worktree after merge
	PRTitle    string // PR title (--pr only)
	PRBody     string // PR body (--pr only)
	PRDraft    bool   // create draft PR
}

// MergeResult describes the outcome of a merge operation.
type MergeResult struct {
	Branch    string
	Success   bool
	PRCreated bool
	PRURL     string
}

// DeleteOptions configures a single worktree delete operation.
type DeleteOptions struct {
	RepoPath     string // root of the main repository
	WtPath       string // resolved worktree filesystem path
	Branch       string // resolved branch name
	Force        bool   // force removal, skip safety checks
	DeleteBranch bool   // also delete the git branch
	DryRun       bool
}

// PruneOptions configures a prune operation.
type PruneOptions struct {
	RepoPath string // root of the main repository
	DryRun   bool
}

// PruneResult describes the outcome of a prune operation.
type PruneResult struct {
	StatePruned int
	TrustPruned int
	GitPruned   bool
}

// DiscoverOptions configures a discover operation.
type DiscoverOptions struct {
	RepoPath string // root of the main repository
	Adopt    bool   // create state entries for discovered worktrees
	DryRun   bool
}

// DiscoverResult describes the outcome of a discover operation.
type DiscoverResult struct {
	RepoName  string
	Unmanaged []UnmanagedWorktree
	Adopted   int
}

// UnmanagedWorktree represents a worktree found by discover that is not in state.
type UnmanagedWorktree struct {
	Path   string
	Branch string
	Source string // "wt" (standard dir) or "external"
}

// FormatSyncStatus returns a human-readable ahead/behind status string.
func FormatSyncStatus(ahead, behind int) string {
	switch {
	case ahead > 0 && behind > 0:
		return fmt.Sprintf("↑%d ↓%d", ahead, behind)
	case ahead > 0:
		return fmt.Sprintf("↑%d", ahead)
	case behind > 0:
		return fmt.Sprintf("↓%d", behind)
	default:
		return "clean"
	}
}

// classifySource determines whether a worktree is in the standard dir or external.
func classifySource(wtPath, standardDir string) string {
	if len(wtPath) > len(standardDir) && wtPath[:len(standardDir)] == standardDir {
		return "wt"
	}
	return "external"
}

// isDirectory checks if a path exists and is a directory.
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// resolveMergeSource determines the merge source (local or remote) and fetches if needed.
func resolveMergeSource(git gitops.Client, log Logger, repoPath, baseBranch string, dryRun bool) (mergeSource string, hasRemote bool) {
	hasRemote, err := git.HasRemote(repoPath)
	if err != nil {
		log.Verbose("Could not check for remote: %v", err)
	}

	mergeSource = baseBranch
	if hasRemote {
		if dryRun {
			log.Info("Would fetch from remote")
		} else {
			log.Info("Fetching latest changes")
			if err := git.Fetch(repoPath); err != nil {
				log.Warning("Fetch failed: %v (continuing with local state)", err)
			}
		}
		mergeSource = "origin/" + baseBranch
	}
	return mergeSource, hasRemote
}

// resolveEffectiveMergeSource checks both remote and local base branch and picks
// whichever has more commits behind — catching unpushed commits on base.
func resolveEffectiveMergeSource(git gitops.Client, log Logger, wtPath, baseBranch, mergeSource string) (effectiveSource string, ahead, behind int) {
	ahead, err := git.CommitsAhead(wtPath, mergeSource)
	if err != nil {
		log.Verbose("Could not check ahead status: %v", err)
	}
	behind, err = git.CommitsBehind(wtPath, mergeSource)
	if err != nil {
		log.Verbose("Could not check behind status: %v", err)
	}

	effectiveSource = mergeSource
	if mergeSource != baseBranch {
		localBehind, err := git.CommitsBehind(wtPath, baseBranch)
		if err != nil {
			log.Verbose("Could not check local behind status: %v", err)
		}
		if localBehind > behind {
			behind = localBehind
			effectiveSource = baseBranch
			ahead, _ = git.CommitsAhead(wtPath, effectiveSource)
		}
	}
	return effectiveSource, ahead, behind
}
