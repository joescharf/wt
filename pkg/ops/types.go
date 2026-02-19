package ops

// SyncOptions configures a sync operation.
type SyncOptions struct {
	BaseBranch string // Base branch to sync from (e.g., "main")
	Strategy   string // "merge" or "rebase"
	Force      bool   // Skip dirty worktree check
	DryRun     bool   // Show what would happen without changes
}

// SyncResult holds the result of syncing a single worktree.
type SyncResult struct {
	Branch        string   // Feature branch name
	BaseBranch    string   // Base branch synced from
	WorktreePath  string   // Absolute path to worktree
	Strategy      string   // "merge" or "rebase"
	Success       bool     // Whether sync completed successfully
	HasConflicts  bool     // Whether conflicts were encountered
	AlreadySynced bool     // True if already up-to-date
	DryRun        bool     // True if this was a dry-run
	Ahead         int      // Commits ahead of base
	Behind        int      // Commits behind base
	ConflictFiles []string // List of conflicting files (if any)
	Error         error    // Error if sync failed
}

// MergeOptions configures a merge operation.
type MergeOptions struct {
	BaseBranch string // Target branch to merge into
	Strategy   string // "merge" or "rebase"
	Force      bool   // Skip safety checks
	DryRun     bool   // Show what would happen without changes
	CreatePR   bool   // Create a PR instead of local merge
	NoCleanup  bool   // Keep worktree after merge
	PRTitle    string // PR title (CreatePR only)
	PRBody     string // PR body (CreatePR only)
	PRDraft    bool   // Create draft PR (CreatePR only)
}

// PRCreateFunc is a function that creates a PR via external tooling (e.g., gh CLI).
// It receives the command arguments and returns the output and any error.
type PRCreateFunc func(args []string) (string, error)

// MergeResult holds the result of a merge operation.
type MergeResult struct {
	Branch        string   // Feature branch that was merged
	BaseBranch    string   // Target branch merged into
	WorktreePath  string   // Absolute path to worktree
	Strategy      string   // "merge" or "rebase"
	Success       bool     // Whether merge completed successfully
	HasConflicts  bool     // Whether conflicts were encountered
	DryRun        bool     // True if this was a dry-run
	ConflictFiles []string // List of conflicting files (if any)
	PRCreated     bool     // Whether a PR was created
	PRURL         string   // URL of created PR (if any)
	Cleaned       bool     // Whether worktree was cleaned up
	Error         error    // Error if merge failed
}

// DeleteOptions configures a delete operation.
type DeleteOptions struct {
	Force        bool // Force removal, skip safety checks
	DeleteBranch bool // Also delete the git branch
	DryRun       bool // Show what would happen without changes
}

// SafetyCheckFunc is called to confirm deletion when there are uncommitted/unpushed changes.
// Returns true to proceed, false to abort.
type SafetyCheckFunc func(worktreePath, dirname string) bool

// CleanupFunc is called after worktree removal for wt-specific cleanup (iTerm2, Claude trust, etc.).
type CleanupFunc func(worktreePath, branchName string) error

// PruneResult holds the result of a prune operation.
type PruneResult struct {
	StatePruned int  // Number of stale state entries removed
	GitPruned   bool // Whether git worktree prune was run
	DryRun      bool // True if this was a dry-run
}
