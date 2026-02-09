# wt

Git Worktree Manager with iTerm2 Integration.

Creates git worktrees with dedicated iTerm2 windows — Claude Code on top, shell on bottom — and tracks them across repos. Full lifecycle management: `create` → work → `merge` → auto-cleanup.

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

  BRANCH          PATH                         WINDOW   STATUS   AGE
  feature/auth    .../myrepo.worktrees/auth    open     ahead    2h
  bugfix/login    .../myrepo.worktrees/login   stale    dirty    1d
  feature/api     .../myrepo.worktrees/api     closed   clean    3d
```

Output columns:

- **BRANCH** — git branch name
- **PATH** — worktree directory path
- **WINDOW** — `open` (green), `stale` (yellow, window closed but state exists), or `closed` (red)
- **STATUS** — git working state:
  - `dirty` (red) — has uncommitted changes
  - `ahead` (yellow) — has commits not yet merged into the base branch
  - `clean` (green) — up to date with the base branch
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
wt merge feature/auth --pr                   # Push + create PR via gh CLI
wt merge feature/auth --pr --draft           # Create draft PR
wt merge feature/auth --pr --title "Add auth" # PR with custom title
wt merge feature/auth --no-cleanup           # Merge but keep worktree
wt merge feature/auth --base develop         # Merge into develop
wt merge feature/auth -n                     # Dry-run
wt mg feature/auth                           # alias
```

**Local merge flow:**

1. Safety checks (dirty worktree → error, use `--force` to skip)
2. Verifies main repo is on the base branch
3. Pulls base branch (if remote exists)
4. Merges feature branch into base branch
5. Pushes base branch (if remote exists)
6. Cleans up worktree (unless `--no-cleanup`)

**PR flow:**

1. Same safety checks
2. Pushes branch to remote
3. Creates PR via `gh pr create`
4. Worktree is kept for PR review

| Flag           | Default | Description                                  |
| -------------- | ------- | -------------------------------------------- |
| `--pr`         | `false` | Create PR instead of local merge             |
| `--no-cleanup` | `false` | Keep worktree after merge                    |
| `--base`       | config  | Target branch (default from `base_branch`)   |
| `--title`      | —       | PR title (`--pr` only)                       |
| `--body`       | —       | PR body (`--pr` only, uses `--fill` if empty)|
| `--draft`      | `false` | Draft PR (`--pr` only)                       |
| `--force`      | `false` | Skip safety checks                          |

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
base_branch: main # Default base branch for new worktrees
no_claude: false # Skip launching Claude in top pane
```

Environment variables (prefix `WT_`):

```bash
export WT_BASE_BRANCH=develop
export WT_NO_CLAUDE=true
```

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

**Merge conflict** — If `merge` encounters a conflict, it stops without cleaning up the worktree. Resolve the conflicts in the main repo, stage the files (`git add`), then run `wt merge <branch>` again — it detects the in-progress merge and continues automatically.

**`gh` CLI not found** — The `--pr` flag requires the GitHub CLI. Install it from [cli.github.com](https://cli.github.com). You must also be authenticated (`gh auth login`).

**Main repo not on base branch** — `merge` (local mode) requires the main repo to be on the target base branch. If you see this error, `cd` to the main repo and `git checkout main` (or your configured base branch) first.
