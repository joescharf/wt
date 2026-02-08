# worktree-dev.zsh

Git Worktree Manager with iTerm2 Integration.

Creates git worktrees with dedicated iTerm2 windows — Claude Code on top, shell on bottom — and tracks them across repos. Creating a worktree spawns the window; deleting it closes the window too.

## Quick Start

```bash
# Create a worktree with a new branch (opens iTerm2 window)
wt create feature/auth

# List active worktrees
wt list

# Focus an existing worktree's window
wt switch feature/auth

# Re-open a window after closing it
wt open feature/auth

# Tear down everything
wt delete feature/auth --delete-branch
```

## Installation

```bash
# From the source directory:
./worktree-dev.zsh install

# Or reinstall after making changes:
./worktree-dev.zsh install
```

This copies the script to `~/.local/bin/worktree-dev.zsh`, makes it executable, and idempotently adds an `alias wt="worktree-dev.zsh"` to `~/.zshrc`. Run `source ~/.zshrc` or open a new shell to pick up the alias.

## Commands

### `create <branch>`

Creates a git worktree, checks out a new branch, and opens an iTerm2 window with two panes.

```bash
wt create feature/auth                          # New branch from main
wt create feature/auth --base develop            # New branch from develop
wt create feature/auth --no-claude               # Don't auto-launch Claude
wt create feature/existing-work --existing       # Use existing branch
```

**What happens:**
1. Creates `<repo>.worktrees/<dirname>/` as a sibling to the main repo
2. Opens a new iTerm2 window split horizontally:
   - **Top pane**: `cd <worktree> && claude`
   - **Bottom pane**: `cd <worktree>` (shell for testing)
3. Saves session IDs to the state file for later tracking

**Branch-to-dirname mapping:** `feature/foo` becomes `foo` (last path segment).

### `list`

Shows all worktrees for the current repo with their iTerm2 window status.

```bash
wt list
wt ls        # alias
```

Output columns:
- **BRANCH** — git branch name
- **PATH** — worktree directory name
- **WINDOW** — `open` (green), `stale` (yellow, window closed but state exists), or `closed` (red)
- **AGE** — time since creation

Automatically prunes stale state entries for worktrees that no longer exist in git.

### `switch <branch>`

Focuses the iTerm2 window for an existing worktree.

```bash
wt switch feature/auth
wt go feature/auth      # alias
wt switch auth           # dirname also works
```

If the window was closed, suggests using `open` instead.

### `delete <branch>`

Closes the iTerm2 window, removes the git worktree, and cleans up state.

```bash
wt delete feature/auth                   # Remove worktree only
wt delete feature/auth --delete-branch   # Also delete the git branch
wt delete feature/auth --force           # Force removal with uncommitted changes
wt rm feature/auth --delete-branch       # alias
```

### `open <branch>`

Re-opens an iTerm2 window for an existing worktree (after the window was manually closed).

```bash
wt open feature/auth
wt open auth             # dirname also works
```

If the window is already open, focuses it instead.

### `install`

Copies the script to `~/.local/bin/` and adds the `wt` alias to `~/.zshrc`.

```bash
./worktree-dev.zsh install        # From source directory
./worktree-dev.zsh -n install     # Dry-run to see what would happen
```

Safe to run repeatedly — skips the alias if it already exists, and overwrites the installed copy with the current source.

## Global Flags

| Flag | Description |
|------|-------------|
| `-v, --verbose` | Show detailed output (commands, paths, session IDs) |
| `-n, --dry-run` | Show what would happen without making changes |
| `-h, --help` | Show usage |

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

Session tracking is stored at `~/.config/worktree-dev/state.json`:

```json
{
  "worktrees": {
    "/path/to/repo.worktrees/auth": {
      "repo": "dbsnapper-agent",
      "branch": "feature/auth",
      "claude_session_id": "...",
      "shell_session_id": "...",
      "created_at": "2026-02-08T12:00:00"
    }
  }
}
```

State is automatically pruned when running `list` — entries for worktrees that no longer exist in git are removed.

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

Sessions are named `wt:<repo>:<dirname>:<pane>` for visual identification. Since running programs (like Claude) can overwrite session names, the script tracks sessions by their unique IDs in the state file rather than by name.

## Repo Detection

The script works from any directory inside a repo or worktree. It uses `git rev-parse --git-common-dir` to find the shared `.git` directory and derive the main repo root, so you can run `wt list` from within a worktree and it will show all worktrees for that repo.

## Dependencies

| Dependency | Purpose | Install |
|------------|---------|---------|
| `git` | Worktree operations | Xcode CLT / Homebrew |
| `jq` | JSON state file manipulation | `brew install jq` |
| `osascript` | iTerm2 AppleScript automation | Built into macOS |
| iTerm2 | Terminal emulator | `brew install --cask iterm2` |

## Troubleshooting

**"iTerm2 is not running"** — Start iTerm2 before using `create` or `open`. The script doesn't auto-launch iTerm2.

**Window status shows "stale"** — The iTerm2 window was closed but state still exists. Running `open` will create a new window and update the state. Running `list` auto-prunes entries for removed worktrees.

**"Worktree already exists"** — Use `open` to attach a new iTerm2 window to the existing worktree, or `delete` it first.

**Branch name resolution** — You can use the full branch name (`feature/auth`) or just the dirname (`auth`). The script tries both when resolving.

**Force delete** — If a worktree has uncommitted changes, `delete` will fail. Use `--force` to override. Combined with `--delete-branch`, this will also force-delete unmerged branches.
