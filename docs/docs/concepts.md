# Concepts

## Worktree Layout

Worktrees are created as **siblings** to the main repo in a `<repo>.worktrees/` directory:

```
~/projects/
  my-repo/                        # Main repo
  my-repo.worktrees/
    auth/                         # feature/auth worktree
    bugfix-login/                 # bugfix/login worktree
```

This convention keeps worktrees visually grouped with their repo while avoiding nesting inside the repo itself.

**Branch-to-dirname mapping:** The last path segment of the branch name becomes the directory name. `feature/auth` becomes `auth`, `bugfix/login` becomes `login`.

## iTerm2 Integration

Each worktree gets a dedicated iTerm2 **window** (not tab) with two panes:

```
+------------------------------------------+
|  wt:my-repo:auth:claude                  |
|  $ claude                                |
|                                          |
|------------------------------------------|
|  wt:my-repo:auth:shell                   |
|  $ _                                     |
+------------------------------------------+
```

Sessions are named `wt:<repo>:<dirname>:<pane>` for visual identification. The tool tracks sessions by their unique IDs in the state file rather than by name.

**Auto-launch:** If iTerm2 isn't running when you use `wt`, it automatically launches iTerm2 and waits up to 10 seconds for it to be ready.

**Window management:**

- `wt create` / `wt open` — creates or reopens the iTerm2 window
- `wt switch` — focuses an existing window
- `wt delete` / `wt merge` (with cleanup) — closes the window

## Claude Code Integration

By default, `wt create` and `wt open` launch Claude Code (`claude`) in the top pane. This provides an AI coding assistant in every worktree session.

To disable Claude auto-launch:

- Per-command: `wt create feature/auth --no-claude`
- Config file: set `no_claude: true`
- Environment: `export WT_NO_CLAUDE=true`

## Sync and Merge Strategies

`wt` supports two strategies for integrating changes: **merge** and **rebase**.

### Merge (default)

- **`sync`**: Merges the base branch into the feature branch, creating a merge commit
- **`merge`**: Merges the feature branch into the base branch with a merge commit

### Rebase

- **`sync --rebase`**: Rebases the feature branch onto the base branch (replays commits on top)
- **`merge --rebase`**: Rebases the feature branch onto base, then fast-forward merges the result into the base branch

Rebase produces a linear commit history without merge commits.

### Strategy Resolution

Strategy is resolved in this order:

1. `--rebase` or `--merge` flag on the command
2. `WT_REBASE` environment variable
3. `rebase` key in config file
4. Default: merge

## Idempotent Operations

Most `wt` commands are designed to recover gracefully if run repeatedly:

| Command | Idempotent behavior |
|---------|-------------------|
| `create` | If worktree exists, delegates to `open` |
| `open` | If window is already open, focuses it |
| `sync` | If already in sync (0 behind), exits early. If merge/rebase in progress, continues it |
| `merge` | If merge/rebase in progress from a prior conflict, continues it |

## Safety Checks

`wt` protects against accidental data loss:

- **Dirty worktree protection** — `sync`, `merge`, and `delete` refuse to operate on worktrees with uncommitted changes. Use `--force` to override.
- **Unpushed commit warnings** — `delete` checks for commits that haven't been pushed and prompts for confirmation.
- **Base branch verification** — `merge` (local mode) verifies the main repo is checked out to the target base branch before merging.
- **Create prompts** — If the target worktree directory doesn't exist yet, `create` prompts for confirmation (defaults to yes).

## State Management

`wt` tracks worktree sessions in a state file at `~/.config/wt/state.json`:

```json
{
  "worktrees": {
    "/path/to/repo.worktrees/auth": {
      "repo": "my-repo",
      "branch": "feature/auth",
      "claude_session_id": "...",
      "shell_session_id": "...",
      "created_at": "2026-02-08T12:00:00Z"
    }
  }
}
```

State is automatically pruned when running `wt list`. Use `wt prune` for explicit cleanup of stale entries (worktree paths that no longer exist on disk).

## Repo Detection

`wt` works from **any directory** inside a repo or worktree. It uses `git rev-parse --git-common-dir` to find the shared `.git` directory and derive the main repo root.

This means you can run `wt list` from within a worktree and it will show all worktrees for that repo.
