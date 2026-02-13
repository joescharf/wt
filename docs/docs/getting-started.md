# Getting Started

## Prerequisites

| Dependency | Purpose | Required |
|------------|---------|----------|
| macOS | AppleScript automation for iTerm2 | Yes |
| [iTerm2](https://iterm2.com) | Terminal emulator with scriptable windows/panes | Yes |
| git | Worktree operations | Yes |
| [GitHub CLI](https://cli.github.com) (`gh`) | Pull request creation via `wt merge --pr` | Optional |

## Installation

=== "Homebrew (recommended)"

    ```bash
    brew install joescharf/tap/wt
    ```

    The Homebrew formula installs a universal binary that is code-signed and notarized by Apple, so macOS Gatekeeper will not show security warnings.

=== "From source"

    ```bash
    go install github.com/joescharf/wt@latest
    ```

    !!! note
        Building from source produces an unsigned binary. macOS may show a Gatekeeper warning on first run. You can allow it in **System Settings > Privacy & Security**.

Verify the installation:

```bash
wt version
```

## Shell Completions

Enable tab completion for worktree names, branch names, and flags.

=== "Zsh"

    Add to `~/.zshrc` for session completions:

    ```bash
    source <(wt completion zsh)
    ```

    Or install persistently:

    ```bash
    wt completion zsh > "${fpath[1]}/_wt"
    ```

=== "Bash"

    Add to `~/.bashrc` for session completions:

    ```bash
    source <(wt completion bash)
    ```

    Or install persistently (macOS with Homebrew):

    ```bash
    wt completion bash > $(brew --prefix)/etc/bash_completion.d/wt
    ```

=== "Fish"

    For session completions:

    ```bash
    wt completion fish | source
    ```

    Or install persistently:

    ```bash
    wt completion fish > ~/.config/fish/completions/wt.fish
    ```

## Your First Worktree

This walkthrough creates a feature branch, does some work, syncs with main, and merges.

### 1. Create a worktree

```bash
wt create feature/my-feature
```

This creates a git worktree with a new `feature/my-feature` branch and opens an iTerm2 window with two panes:

- **Top pane**: Claude Code session (`claude`)
- **Bottom pane**: Shell for running commands

The worktree directory is created as a sibling to your repo:

```
~/projects/
  my-repo/                        # Main repo
  my-repo.worktrees/
    my-feature/                   # Your new worktree
```

### 2. List your worktrees

```bash
wt list
```

```
Worktrees for my-repo

  BRANCH              PATH                                WINDOW   STATUS   AGE
  feature/my-feature  .../my-repo.worktrees/my-feature    open     clean    1m
```

### 3. Sync with main

After working on your feature, pull in the latest changes from main:

```bash
wt sync feature/my-feature
```

This fetches from the remote (if one exists), reports ahead/behind status, and merges main into your feature branch.

!!! tip
    Use `wt sync feature/my-feature --rebase` to rebase onto main instead of merging, producing a linear history.

### 4. Merge and clean up

When your feature is ready:

```bash
# Local merge into main + auto-cleanup
wt merge feature/my-feature

# Or create a pull request instead
wt merge feature/my-feature --pr
```

Local merge performs the merge, pushes (if remote exists), removes the worktree, closes the iTerm2 window, and cleans up state — all in one command.

### 5. Shorthand access

If you close the iTerm2 window and want to reopen it later:

```bash
wt my-feature        # Opens the worktree window (dirname match)
```

Any unrecognized command is treated as a branch name, so `wt my-feature` is equivalent to `wt open my-feature`.
