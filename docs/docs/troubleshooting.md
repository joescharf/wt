# Troubleshooting

## iTerm2 Issues

### iTerm2 auto-launch

If iTerm2 isn't running, `wt` automatically launches it and waits up to 10 seconds for it to be ready. If you see timeout errors, open iTerm2 manually and try again.

### Window status shows "stale"

The iTerm2 window was closed but state still exists. Two options:

- Run `wt open <branch>` to create a new window and update the state
- Run `wt prune` to clean up stale entries without reopening

## Branch Resolution

### Branch name not found

You can use the full branch name (`feature/auth`) or just the dirname (`auth`). The tool tries both when resolving. If neither matches, check `wt list` for the exact branch name.

## Merge and Sync Conflicts

### Merge conflict during `wt merge`

If `merge` encounters a conflict, it stops without cleaning up the worktree:

1. Resolve the conflicts in the worktree
2. Stage the resolved files: `git add <files>`
3. Run `wt merge <branch>` again — it detects the in-progress merge and continues automatically

For rebase conflicts, you can also abort with `git rebase --abort` in the worktree.

### Sync conflict during `wt sync`

If `sync` encounters a conflict:

1. Resolve the conflicts in the worktree
2. Stage the resolved files: `git add <files>`
3. Run `wt sync <branch>` again — it detects the in-progress operation and continues

For rebase conflicts, you can abort with `git rebase --abort`.

### Main repo not on base branch

`merge` (local mode) requires the main repo to be on the target base branch. If you see this error:

```bash
cd /path/to/main-repo
git checkout main          # or your configured base branch
wt merge feature/auth      # try again
```

## GitHub CLI

### `gh` CLI not found

The `--pr` flag requires the [GitHub CLI](https://cli.github.com). Install and authenticate:

```bash
brew install gh
gh auth login
```

## Delete Safety

### Worktree has uncommitted changes

`delete` checks for uncommitted changes and unpushed commits before removing a worktree. If there's risk of data loss, it prompts for confirmation.

To force deletion without checks:

```bash
wt delete feature/auth --force
```

## General

### Verbose output

For debugging any command, add `--verbose` to see detailed output including git commands, paths, and session IDs:

```bash
wt create feature/auth --verbose
wt sync feature/auth -v
```

### Dry-run

To see what a command would do without making changes:

```bash
wt merge feature/auth --dry-run
wt delete --all -n
```
