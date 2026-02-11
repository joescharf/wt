# Command Reference

## Branch Shorthand

Any unrecognized command is treated as a branch name and opens the corresponding worktree window.

```bash
wt auth                  # Opens worktree for feature/auth (dirname match)
wt feature/auth          # Also works with full branch name
```

Equivalent to `wt open <branch>`.

---

## `create`

Creates a git worktree, checks out a new branch, and opens an iTerm2 window with two panes.

**Idempotent** — if the worktree already exists, opens it instead (same as `open`).

```bash
wt create feature/auth                        # New branch from main
wt create feature/auth --base develop         # New branch from develop
wt create feature/auth --no-claude            # Skip auto-launching Claude
wt create feature/existing-work --existing    # Use an existing branch
```

**What happens:**

1. If the worktree already exists, delegates to `open`
2. Creates `<repo>.worktrees/<dirname>/` as a sibling to the main repo
3. Opens a new iTerm2 window split horizontally:
    - **Top pane**: `cd <worktree> && claude`
    - **Bottom pane**: `cd <worktree>` (shell)
4. Saves session IDs to the state file

| Flag | Default | Description |
|------|---------|-------------|
| `--base` | config `base_branch` | Base branch to create from |
| `--existing` | `false` | Use an existing branch instead of creating a new one |
| `--no-claude` | config `no_claude` | Skip launching Claude in the top pane |

**Branch-to-dirname mapping:** `feature/foo` becomes `foo` (last path segment).

---

## `open`

Re-opens an iTerm2 window for an existing worktree (after the window was closed).

```bash
wt open feature/auth
wt open auth             # dirname also works
```

If the window is already open, focuses it instead.

---

## `list`

Shows all worktrees for the current repo with iTerm2 window status and git status.

**Aliases:** `ls`

```bash
wt list
wt ls
```

Example output:

```
Worktrees for myrepo

  BRANCH          PATH                         WINDOW   STATUS              AGE
  feature/auth    .../myrepo.worktrees/auth    open     ↑2                  2h
  bugfix/login    .../myrepo.worktrees/login   stale    dirty ↓3            1d
  feature/api     .../myrepo.worktrees/api     closed   clean               3d
  feature/sync    .../myrepo.worktrees/sync    open     ↑1 ↓5              4h
  feature/rebase  .../myrepo.worktrees/rebase  open     rebasing dirty ↓2   1h
```

**Output columns:**

| Column | Description |
|--------|-------------|
| **BRANCH** | Git branch name |
| **PATH** | Worktree directory path |
| **WINDOW** | `open` (green), `stale` (yellow — window closed but state exists), or `closed` (red) |
| **STATUS** | Git working state (see below) |
| **AGE** | Time since creation |

**Status indicators:**

| Indicator | Meaning |
|-----------|---------|
| `clean` | No uncommitted changes, in sync with base |
| `dirty` | Has uncommitted changes |
| `rebasing` | Rebase in progress (needs conflict resolution) |
| `merging` | Merge in progress (needs conflict resolution) |
| `↑N` | N commits ahead of base branch |
| `↓N` | N commits behind base branch (needs `wt sync`) |
| `↑N ↓M` | Diverged — N ahead and M behind |

Indicators combine, e.g. `rebasing dirty ↑N ↓M`.

Automatically prunes stale state entries for worktrees that no longer exist on disk.

---

## `switch`

Focuses the iTerm2 window for an existing worktree.

**Aliases:** `go`

```bash
wt switch feature/auth
wt go feature/auth
wt switch auth           # dirname also works
```

If the window was closed, suggests using `open` instead.

---

## `sync`

Syncs a worktree with the base branch by merging (or rebasing onto) the latest base branch. Reports ahead/behind status before syncing.

**Aliases:** `sy`

**Idempotent** — if a merge or rebase has conflicts, resolve them and run `wt sync` again to continue.

```bash
wt sync feature/auth                   # Sync with main
wt sync feature/auth --rebase          # Rebase onto main instead
wt sync feature/auth --base develop    # Sync with develop
wt sync feature/auth --force           # Skip dirty worktree check
wt sync --all                          # Sync all worktrees
wt sync --all --rebase                 # Rebase all worktrees
wt sync -n feature/auth                # Dry-run
```

**What happens:**

1. Safety checks (dirty worktree → error, use `--force` to skip)
2. If a merge or rebase is already in progress, picks up where it left off
3. Fetches latest changes (if remote exists)
4. Checks behind count against both remote and local base branch, using whichever is further ahead
5. Reports status (`↑2 ↓3` means 2 ahead, 3 behind)
6. If already in sync (0 behind), exits early
7. Merges base into feature (default) or rebases feature onto base (`--rebase`)

**Sync all** (`--all`) fetches once, then syncs each worktree. Skips dirty worktrees and those with in-progress operations.

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | `false` | Sync all worktrees |
| `--rebase` | config `rebase` | Rebase onto base instead of merging |
| `--merge` | `false` | Use merge (overrides config `rebase` default) |
| `--base` | config `base_branch` | Base branch |
| `--force` | `false` | Skip dirty worktree safety check |

---

## `merge`

Merges a worktree's branch into the base branch (local merge by default) or creates a pull request (`--pr`). After a successful local merge, the worktree is automatically cleaned up.

**Aliases:** `mg`

**Idempotent** — if a merge has conflicts, resolve them and run `wt merge` again to continue.

```bash
wt merge feature/auth                          # Local merge into main + cleanup
wt merge feature/auth --rebase                 # Rebase-then-fast-forward merge
wt merge feature/auth --pr                     # Push + create PR via gh CLI
wt merge feature/auth --pr --draft             # Create draft PR
wt merge feature/auth --pr --title "Add auth"  # PR with custom title
wt merge feature/auth --no-cleanup             # Merge but keep worktree
wt merge feature/auth --base develop           # Merge into develop
wt merge feature/auth -n                       # Dry-run
```

### Local merge flow (default)

1. Safety checks (dirty worktree → error, use `--force` to skip)
2. Verifies main repo is on the base branch
3. Pulls base branch (if remote exists)
4. Merges feature branch into base branch
5. Pushes base branch (if remote exists)
6. Cleans up worktree (unless `--no-cleanup`)

### Rebase-then-fast-forward flow (`--rebase`)

1. Same safety checks
2. Rebases the feature branch onto the base branch (in the worktree)
3. Fast-forward merges the rebased feature tip into the base branch (in the main repo)
4. Pushes and cleans up as normal

This produces a linear commit history without merge commits.

### PR flow (`--pr`)

1. Same safety checks
2. Pushes branch to remote
3. Creates PR via `gh pr create`
4. Worktree is kept for PR review
5. `--rebase` is ignored (merge strategy is configured on GitHub)

| Flag | Default | Description |
|------|---------|-------------|
| `--pr` | `false` | Create PR instead of local merge |
| `--rebase` | config `rebase` | Use rebase-then-fast-forward instead of merge |
| `--merge` | `false` | Use merge (overrides config `rebase` default) |
| `--no-cleanup` | `false` | Keep worktree after merge |
| `--base` | config `base_branch` | Target branch |
| `--title` | — | PR title (`--pr` only) |
| `--body` | — | PR body (`--pr` only, uses `--fill` if empty) |
| `--draft` | `false` | Draft PR (`--pr` only) |
| `--force` | `false` | Skip safety checks |

---

## `delete`

Closes the iTerm2 window, removes the git worktree, and cleans up state.

**Aliases:** `rm`

**Safety checks:** Before deleting, `wt` checks for uncommitted changes and unpushed commits. If there's risk of data loss, it prompts for confirmation.

```bash
wt delete feature/auth                   # Remove worktree (with safety checks)
wt delete feature/auth --delete-branch   # Also delete the git branch
wt delete feature/auth --force           # Skip safety checks
wt delete --all                          # Delete all worktrees (checks each)
wt delete --all --force                  # Delete all without prompting
```

| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Skip safety checks (dirty/unpushed), force removal |
| `--delete-branch` | `false` | Also delete the git branch |
| `--all` | `false` | Delete all worktrees (excludes main repo) |

---

## `config`

Show or manage `wt` configuration. Running bare `wt config` is the same as `wt config show`.

```bash
wt config                    # Show effective config (same as show)
wt config init               # Create config.yaml with commented defaults
wt config init --force       # Overwrite existing config file
wt config show               # Show all keys with values and sources
wt config edit               # Open config file in $EDITOR
```

### Subcommands

**`init`** — Creates `~/.config/wt/config.yaml` with commented defaults reflecting current effective values. Refuses to overwrite unless `--force` is passed. Respects `--dry-run`.

**`show`** — Displays each config key with its effective value and source: `(default)`, `(file)`, or `(env: WT_*)`.

**`edit`** — Opens the config file in `$EDITOR` (or `$VISUAL`). Errors if neither is set or if the config file doesn't exist yet.

---

## `prune`

Cleans up stale state and git worktree tracking.

```bash
wt prune          # Clean stale state + run git worktree prune
wt prune -n       # Dry-run: show what would be cleaned
```

Removes state entries for worktree paths that no longer exist on disk and runs `git worktree prune` to clean git's internal tracking.

---

## `completion`

Generates shell completion scripts. See [Shell Completions](getting-started.md#shell-completions).

```bash
wt completion bash
wt completion zsh
wt completion fish
```

---

## `version`

Prints version, commit hash, and build date.

```bash
wt version
```

---

## Global Flags

These flags are available on all commands:

| Flag | Description |
|------|-------------|
| `-v, --verbose` | Show detailed output (commands, paths, session IDs) |
| `-n, --dry-run` | Show what would happen without making changes |
| `-h, --help` | Show usage |
