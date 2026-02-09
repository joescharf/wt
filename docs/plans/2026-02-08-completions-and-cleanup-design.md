# Shell Completions + Smart Delete & Cleanup

## Overview

Two features to improve the `wt` CLI workflow:
1. Dynamic shell completions for zsh, bash, and fish
2. Smart delete with safety checks, a standalone prune command, and bulk delete

## Feature 1: Shell Completions

### Completion Command

New `wt completion <shell>` command that writes a completion script to stdout.

```
wt completion bash   # source <(wt completion bash)
wt completion zsh    # source <(wt completion zsh)
wt completion fish   # wt completion fish | source
```

Uses cobra's built-in `GenBashCompletionV2()`, `GenZshCompletion()`, `GenFishCompletion()`.

### Dynamic Completions

Each command that takes a branch/worktree argument gets a `ValidArgsFunction` for context-aware tab completion:

| Command | Completes with |
|---------|---------------|
| `wt open <TAB>` | Existing worktree dirnames |
| `wt switch <TAB>` | Existing worktree dirnames |
| `wt delete <TAB>` | Existing worktree dirnames |
| `wt create --base <TAB>` | Local branch names |
| `wt <TAB>` (bare shorthand) | Existing worktree dirnames |

Two helper functions:
- `completeWorktreeNames()` — calls `gitClient.WorktreeList()`, returns dirnames (excluding main repo)
- `completeBranchNames()` — calls `git branch --format='%(refname:short)'` via a new `git.Client.BranchList()` method

### Files

- `cmd/completion.go` — completion command + helper functions
- `internal/git/git.go` — add `BranchList() ([]string, error)` to Client interface
- Mock regeneration for `BranchList`

## Feature 2: Smart Delete & Cleanup

### Smart Delete Safety Checks

Before removing a worktree, check for data loss risk:

1. **Dirty working tree**: `git -C <path> status --porcelain` — non-empty = uncommitted changes
2. **Unpushed commits**: `git -C <path> log @{upstream}..HEAD --oneline 2>/dev/null` — if that fails (no upstream), fall back to `git -C <path> log <baseBranch>..HEAD --oneline`

Decision matrix:
- **Clean + pushed** -> delete immediately (safe, no prompt)
- **Dirty or unpushed** -> show what's at risk, prompt `Delete worktree with [uncommitted changes / N unpushed commits]? [y/N]`
- **`--force` flag** -> skip all checks (preserves existing behavior)
- **`--dry-run`** -> show what checks would find, don't prompt or delete

### New git.Client Methods

```go
// IsWorktreeDirty returns true if the worktree has uncommitted changes.
IsWorktreeDirty(path string) (bool, error)

// HasUnpushedCommits returns true if the worktree has commits not pushed
// to upstream or ahead of baseBranch.
HasUnpushedCommits(path, baseBranch string) (bool, error)
```

### Interactive Prompt

A `confirmPrompt(msg string) bool` function in `cmd/` that reads from stdin. Extracted to a package-level `var promptFunc` so tests can replace it with a mock.

### Prune Command

New `wt prune` command:

```
wt prune          # clean stale state + git worktree prune
wt prune -n       # dry-run: show what would be cleaned
```

Steps:
1. Call `stateMgr.Prune()` to remove state entries for paths that no longer exist
2. Call `git worktree prune` to clean git's internal worktree tracking
3. Report counts of what was cleaned

New `git.Client` method:
```go
// WorktreePrune runs `git worktree prune` to clean stale internal tracking.
WorktreePrune() error
```

### Bulk Delete

New `--all` flag on `wt delete`:

```
wt delete --all              # delete all worktrees (with safety checks on each)
wt delete --all --force      # delete all worktrees without prompting
```

Steps:
1. List all non-main worktrees via `gitClient.WorktreeList()`
2. For each worktree, apply safety checks
3. Safe ones: delete silently. Unsafe ones: prompt individually
4. `--force`: skip all prompts
5. After all deletions, run `git worktree prune`

### Files Changed

- `cmd/delete.go` — add safety checks, `--all` flag, prompt logic
- `cmd/prune.go` — new prune command
- `internal/git/git.go` — add `IsWorktreeDirty`, `HasUnpushedCommits`, `WorktreePrune`, `BranchList` to interface + implementation
- Mock regeneration
- `cmd/cmd_test.go` — tests for new behaviors

## Implementation Order

1. Add `BranchList`, `IsWorktreeDirty`, `HasUnpushedCommits`, `WorktreePrune` to git.Client + tests
2. Regenerate mocks
3. Add `cmd/completion.go` with completion command + dynamic completions + tests
4. Add `cmd/prune.go` with prune command + tests
5. Update `cmd/delete.go` with safety checks, `--all` flag, prompt + tests
6. Run full test suite, verify `go vet` clean
