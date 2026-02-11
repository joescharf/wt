# wt

**Git Worktree Manager with iTerm2 + Claude Code integration.**

`wt` creates git worktrees with dedicated iTerm2 windows — Claude Code on top, shell on bottom — and tracks them across repos. Full lifecycle management from branch creation to merge.

## Quick Start

```bash
wt create feature/auth       # Create worktree + open iTerm2 window
wt list                      # See all active worktrees
wt sync feature/auth         # Pull latest main into your branch
wt merge feature/auth        # Merge into main + auto-cleanup
```

## Documentation

| Section | Description |
|---------|-------------|
| [Getting Started](getting-started.md) | Installation, shell completions, first worktree walkthrough |
| [Commands](commands.md) | Complete reference for every command, flag, and alias |
| [Configuration](configuration.md) | Config file, environment variables, merge strategy |
| [Concepts](concepts.md) | Worktree layout, iTerm2 integration, sync strategies |
| [Troubleshooting](troubleshooting.md) | Common issues and solutions |

!!! info "Prerequisites"
    `wt` requires **macOS**, **iTerm2**, and **git**. Optionally install the [GitHub CLI](https://cli.github.com) (`gh`) for pull request support.
