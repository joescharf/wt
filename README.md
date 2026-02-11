# wt

Git Worktree Manager with iTerm2 Integration.

Creates git worktrees with dedicated iTerm2 windows — Claude Code on top, shell on bottom — and tracks them across repos. Full lifecycle management: `create` → work → `sync` → `merge` → auto-cleanup.

## Quick Start

```bash
# Create a worktree with a new branch (opens iTerm2 window)
wt create feature/auth

# Open a worktree (shorthand — same as: wt open auth)
wt auth

# List active worktrees
wt list

# Focus an existing worktree's window
wt switch feature/auth

# Sync worktree with latest main
wt sync feature/auth

# Merge branch into main + auto-cleanup
wt merge feature/auth

# Or create a PR instead
wt merge feature/auth --pr

# Tear down everything
wt delete feature/auth --delete-branch

# Clean up stale state
wt prune
```

## Installation

```bash
# Via Homebrew tap
brew install joescharf/tap/wt

# Or build from source
go install github.com/joescharf/wt@latest
```

## Shell Completions

Enable tab completion for worktree names, branch names, and flags:

```bash
# Zsh (add to ~/.zshrc)
source <(wt completion zsh)

# Bash (add to ~/.bashrc)
source <(wt completion bash)

# Fish
wt completion fish | source
```

For persistent completions:

```bash
# Zsh
wt completion zsh > "${fpath[1]}/_wt"

# Bash (macOS)
wt completion bash > $(brew --prefix)/etc/bash_completion.d/wt

# Fish
wt completion fish > ~/.config/fish/completions/wt.fish
```

## Commands

### `<branch>` (shorthand)

Opens the iTerm2 window for an existing worktree. Any unrecognized command is treated as a branch name.

```bash
wt auth                  # Opens worktree for feature/auth (dirname match)
wt feature/auth          # Also works with full branch name
```

Equivalent to `wt open <branch>`.

### `create <branch>`

Creates a git worktree, checks out a new branch, and opens an iTerm2 window with two panes. **Idempotent** — if the worktree already exists, opens it instead (same as `open`).

```bash
wt create feature/auth                          # New branch from main
wt create feature/auth --base develop            # New branch from develop
wt create feature/auth --no-claude               # Don't auto-launch Claude
wt create feature/existing-work --existing       # Use existing branch
wt create feature/auth                          # Safe to re-run — opens existing
```

**What happens:**

1. If the worktree already exists, delegates to `open`
2. Otherwise, creates `<repo>.worktrees/<dirname>/` as a sibling to the main repo
3. Opens a new iTerm2 window split horizontally:
   - **Top pane**: `cd <worktree> && claude`
   - **Bottom pane**: `cd <worktree>` (shell for testing)
4. Saves session IDs to the state file for later tracking

**Branch-to-dirname mapping:** `feature/foo` becomes `foo` (last path segment).

### `list`

Shows all worktrees for the current repo with their iTerm2 window status and git status.

```bash
wt list
wt ls        # alias
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

Output columns:

- **BRANCH** — git branch name
- **PATH** — worktree directory path
- **WINDOW** — `open` (green), `stale` (yellow, window closed but state exists), or `closed` (red)
- **STATUS** — git working state, combining operation, dirty, and ahead/behind indicators:
  - `clean` (green) — no uncommitted changes, in sync with base branch
  - `dirty` (red) — has uncommitted changes
  - `rebasing` (red) — a rebase is in progress (needs conflict resolution)
  - `merging` (red) — a merge is in progress (needs conflict resolution)
  - `↑N` (yellow) — N commits ahead of base branch
  - `↓N` (yellow) — N commits behind base branch (needs `wt sync`)
  - `↑N ↓M` (yellow) — N ahead and M behind (diverged)
  - Combined statuses like `rebasing dirty ↑N ↓M` (red) — multiple indicators shown together
- **AGE** — time since creation

Automatically prunes stale state entries for worktrees that no longer exist on disk.

### `switch <branch>`

Focuses the iTerm2 window for an existing worktree.

```bash
wt switch feature/auth
wt go feature/auth      # alias
wt switch auth           # dirname also works
```

If the window was closed, suggests using `open` instead.

### `merge [branch]`

Merges a worktree's branch into the base branch (local merge by default) or creates a pull request (`--pr`). After a successful local merge, the worktree is automatically cleaned up. **Idempotent** — if a merge has conflicts, resolve them and run `wt merge` again to continue.

```bash
wt merge feature/auth                        # Local merge into main + cleanup
wt merge feature/auth --rebase               # Rebase-then-fast-forward merge
wt merge feature/auth --pr                   # Push + create PR via gh CLI
wt merge feature/auth --pr --draft           # Create draft PR
wt merge feature/auth --pr --title "Add auth" # PR with custom title
wt merge feature/auth --no-cleanup           # Merge but keep worktree
wt merge feature/auth --base develop         # Merge into develop
wt merge feature/auth -n                     # Dry-run
wt mg feature/auth                           # alias
```

**Local merge flow (default):**

1. Safety checks (dirty worktree → error, use `--force` to skip)
2. Verifies main repo is on the base branch
3. Pulls base branch (if remote exists)
4. Merges feature branch into base branch
5. Pushes base branch (if remote exists)
6. Cleans up worktree (unless `--no-cleanup`)

**Rebase-then-fast-forward flow** (`--rebase`):

1. Same safety checks
2. Rebases the feature branch onto the base branch (in the worktree)
3. Fast-forward merges the rebased feature tip into the base branch (in the main repo)
4. Pushes and cleans up as normal

This produces a linear commit history without merge commits.

**PR flow:**

1. Same safety checks
2. Pushes branch to remote
3. Creates PR via `gh pr create`
4. Worktree is kept for PR review
5. `--rebase` is ignored (merge strategy is configured on GitHub)

| Flag           | Default | Description                                  |
| -------------- | ------- | -------------------------------------------- |
| `--pr`         | `false` | Create PR instead of local merge             |
| `--rebase`     | `false` | Use rebase-then-fast-forward instead of merge|
| `--merge`      | `false` | Use merge (overrides config `rebase` default)|
| `--no-cleanup` | `false` | Keep worktree after merge                    |
| `--base`       | config  | Target branch (default from `base_branch`)   |
| `--title`      | —       | PR title (`--pr` only)                       |
| `--body`       | —       | PR body (`--pr` only, uses `--fill` if empty)|
| `--draft`      | `false` | Draft PR (`--pr` only)                       |
| `--force`      | `false` | Skip safety checks                          |

### `sync [branch]`

Syncs a worktree with the base branch by merging (or rebasing onto) the base branch. Reports ahead/behind status before syncing so you know exactly what will happen. **Idempotent** — if a merge or rebase has conflicts, resolve them and run `wt sync` again to continue.

```bash
wt sync feature/auth                   # Sync with main (fetches if remote exists)
wt sync feature/auth --rebase          # Rebase onto main instead of merging
wt sync feature/auth --base develop    # Sync with develop instead
wt sync feature/auth --force           # Skip dirty worktree check
wt sync --all                          # Sync all worktrees at once
wt sync --all --rebase                 # Rebase all worktrees onto base
wt sync -n feature/auth                # Dry-run
wt sy feature/auth                     # alias
```

**What happens:**

1. Safety checks (dirty worktree → error, use `--force` to skip)
2. If a merge or rebase is already in progress, picks up where it left off
3. Fetches latest changes (if remote exists)
4. Checks behind count against both remote (`origin/main`) and local base branch, using whichever is further ahead — catches both upstream changes and local commits on `main` not yet pushed
5. Reports status (`↑2 ↓3` means 2 ahead, 3 behind)
6. If already in sync (0 behind), exits early
7. Merges base branch into feature branch (default) or rebases feature onto base (`--rebase`)

**Sync all** (`--all`) fetches once, then syncs each worktree. Skips dirty worktrees and those with in-progress merges/rebases, reports per-worktree status.

| Flag       | Default | Description                                |
| ---------- | ------- | ------------------------------------------ |
| `--all`    | `false` | Sync all worktrees                         |
| `--rebase` | `false` | Rebase onto base instead of merging        |
| `--merge`  | `false` | Use merge (overrides config `rebase` default) |
| `--base`   | config  | Base branch (default from `base_branch`)   |
| `--force`  | `false` | Skip dirty worktree safety check           |

### `delete [branch]`

Closes the iTerm2 window, removes the git worktree, and cleans up state.

**Safety checks:** Before deleting, `wt` checks for uncommitted changes and unpushed commits. If the worktree is clean and up to date, it deletes immediately. If there's risk of data loss, it prompts for confirmation.

```bash
wt delete feature/auth                   # Remove worktree (with safety checks)
wt delete feature/auth --delete-branch   # Also delete the git branch
wt delete feature/auth --force           # Skip safety checks
wt delete --all                          # Delete all worktrees (checks each one)
wt delete --all --force                  # Delete all worktrees without prompting
wt rm feature/auth                       # alias
```

| Flag              | Description                                            |
| ----------------- | ------------------------------------------------------ |
| `--force`         | Skip safety checks (dirty/unpushed), force removal     |
| `--delete-branch` | Also delete the git branch after removing the worktree |
| `--all`           | Delete all worktrees (excludes main repo)              |

### `open <branch>`

Re-opens an iTerm2 window for an existing worktree (after the window was manually closed).

```bash
wt open feature/auth
wt open auth             # dirname also works
```

If the window is already open, focuses it instead.

### `config`

Show or manage wt configuration. Running bare `wt config` is the same as `wt config show`.

```bash
wt config                    # Show effective config (same as: wt config show)
wt config init               # Create config.yaml with commented defaults
wt config init --force       # Overwrite existing config file
wt config show               # Show all keys with values and sources
wt config edit               # Open config file in $EDITOR
```

**Subcommands:**

- **`init`** — Creates `~/.config/wt/config.yaml` with commented defaults reflecting current effective values. Refuses to overwrite an existing file unless `--force` is passed. Respects `--dry-run`.
- **`show`** — Displays each config key with its effective value and source: `(default)`, `(file)`, or `(env: WT_*)`.
- **`edit`** — Opens the config file in `$EDITOR` (or `$VISUAL`). Errors if neither is set or if the config file doesn't exist yet.

### `prune`

Cleans up stale state and git worktree tracking.

```bash
wt prune          # Clean stale state + run git worktree prune
wt prune -n       # Dry-run: show what would be cleaned
```

This removes state entries for worktree paths that no longer exist on disk and runs `git worktree prune` to clean git's internal tracking.

### `completion <shell>`

Generates shell completion scripts. See [Shell Completions](#shell-completions) above.

```bash
wt completion bash
wt completion zsh
wt completion fish
```

### `version`

Prints version, commit hash, and build date.

```bash
wt version
```

## Global Flags

| Flag            | Description                                         |
| --------------- | --------------------------------------------------- |
| `-v, --verbose` | Show detailed output (commands, paths, session IDs) |
| `-n, --dry-run` | Show what would happen without making changes       |
| `-h, --help`    | Show usage                                          |

## Configuration

Configuration file (optional): `~/.config/wt/config.yaml`

```yaml
base_branch: main  # Default base branch for new worktrees
no_claude: false    # Skip launching Claude in top pane
rebase: false       # Use rebase instead of merge for sync/merge commands
```

Environment variables (prefix `WT_`):

```bash
export WT_BASE_BRANCH=develop
export WT_NO_CLAUDE=true
export WT_REBASE=true          # Make rebase the default strategy
```

When `rebase: true` is set in config (or `WT_REBASE=true`), `sync` and `merge` will use rebase by default. Use `--merge` on any command to override back to merge.

Precedence: environment variables > config file > defaults.

## Worktree Layout

Worktrees are created as siblings to the main repo:

```
~/app/scratch/
  dbsnapper-agent/           # Main repo
  dbsnapper-agent.worktrees/
    auth/                    # feature/auth worktree
    bugfix-login/            # bugfix/login worktree
```

This convention keeps worktrees visually grouped with their repo while avoiding nesting inside the repo itself.

## State File

Session tracking is stored at `~/.config/wt/state.json`:

```json
{
	"worktrees": {
		"/path/to/repo.worktrees/auth": {
			"repo": "dbsnapper-agent",
			"branch": "feature/auth",
			"claude_session_id": "...",
			"shell_session_id": "...",
			"created_at": "2026-02-08T12:00:00Z"
		}
	}
}
```

State is automatically pruned when running `list`. Use `wt prune` for explicit cleanup.

## iTerm2 Sessions

Each worktree gets a dedicated iTerm2 **window** (not tab) with two panes:

```
+------------------------------------------+
|  wt:dbsnapper-agent:auth:claude          |
|  $ claude                                |
|                                          |
|------------------------------------------|
|  wt:dbsnapper-agent:auth:shell           |
|  $ _                                     |
+------------------------------------------+
```

Sessions are named `wt:<repo>:<dirname>:<pane>` for visual identification. The tool tracks sessions by their unique IDs in the state file rather than by name.

## Repo Detection

`wt` works from any directory inside a repo or worktree. It uses `git rev-parse --git-common-dir` to find the shared `.git` directory and derive the main repo root, so you can run `wt list` from within a worktree and it will show all worktrees for that repo.

## Dependencies

| Dependency  | Purpose                                          | Required |
| ----------- | ------------------------------------------------ | -------- |
| `git`       | Worktree operations                              | Yes      |
| `osascript` | iTerm2 AppleScript automation (built into macOS) | Yes      |
| iTerm2      | Terminal emulator                                | Yes      |
| `gh`        | GitHub CLI for `merge --pr` ([cli.github.com](https://cli.github.com)) | Optional |

## Troubleshooting

**iTerm2 auto-launch** — If iTerm2 isn't running, `wt` automatically launches it and waits up to 10 seconds for it to be ready.

**Window status shows "stale"** — The iTerm2 window was closed but state still exists. Running `open` will create a new window and update the state. Running `wt prune` cleans up stale entries.

**Branch name resolution** — You can use the full branch name (`feature/auth`) or just the dirname (`auth`). The tool tries both when resolving.

**Safety checks on delete** — If a worktree has uncommitted changes or unpushed commits, `delete` will prompt for confirmation. Use `--force` to skip all checks.

**Merge conflict** — If `merge` encounters a conflict (merge or rebase), it stops without cleaning up the worktree. Resolve the conflicts, stage the files (`git add`), then run `wt merge <branch>` again — it detects the in-progress merge/rebase and continues automatically. For rebase conflicts, you can also abort with `git rebase --abort` in the worktree.

**`gh` CLI not found** — The `--pr` flag requires the GitHub CLI. Install it from [cli.github.com](https://cli.github.com). You must also be authenticated (`gh auth login`).

**Sync conflict** — If `sync` encounters a conflict (merge or rebase), it stops. Resolve the conflicts in the worktree, stage the files (`git add`), then run `wt sync <branch>` again — it detects the in-progress merge/rebase and continues automatically. For rebase conflicts, you can also abort with `git rebase --abort` in the worktree.

**Main repo not on base branch** — `merge` (local mode) requires the main repo to be on the target base branch. If you see this error, `cd` to the main repo and `git checkout main` (or your configured base branch) first.
