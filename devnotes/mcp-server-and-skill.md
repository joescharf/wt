# MCP Server + Claude Skill for WT

*2026-02-14T17:04:11Z*

## What Changed

Added an MCP (Model Context Protocol) server to the wt project and a Claude Skill in the joescharf marketplace. Together these let Claude Code manage git worktrees programmatically — creating, opening, syncing, merging, and deleting worktrees without touching the CLI directly.

### Part 1: MCP Server

The MCP server exposes 6 tools over stdio that Claude Code can call natively:

| Tool | Description |
|------|-------------|
| `wt_list` | List worktrees with window status, git status, and age |
| `wt_create` | Create worktree + branch + iTerm2 window |
| `wt_open` | Open or focus existing worktree window |
| `wt_delete` | Close window + remove worktree (safety checks) |
| `wt_sync` | Sync worktree with base branch (merge or rebase) |
| `wt_merge` | Merge into base branch or push for PR |

**Architecture decision:** The existing `git.Client` interface uses CWD context via `RepoRoot()`. Since the MCP server runs as stdio with no CWD, a parallel `GitClient` interface was created in `internal/mcp/` where every method takes an explicit `repoPath` parameter. This avoids refactoring the existing CLI interface.

### Part 2: Claude Skill

A skill was added to the `cc-joescharf-marketplace` at `plugins/wt/skills/manage-worktrees/SKILL.md`. It documents all 6 MCP tools with parameters, provides CLI fallback commands, configuration reference, and workflow examples (new feature, review, cleanup).

## New Files

| File | Purpose |
|------|---------|
| `internal/mcp/gitclient.go` | Path-based `GitClient` interface + `RealGitClient` implementation |
| `internal/mcp/gitclient_test.go` | 11 integration tests using real git repos in `t.TempDir()` |
| `internal/mcp/server.go` | MCP server with 6 tool handlers |
| `internal/mcp/server_test.go` | 30 unit tests with mock dependencies + integration test |
| `cmd/mcp.go` | CLI commands: `wt mcp`, `wt mcp install`, `wt mcp status` |

### Modified Files

| File | Change |
|------|--------|
| `go.mod` / `go.sum` | Added `github.com/mark3labs/mcp-go v0.43.2` |
| `README.md` | Added `mcp` command documentation |

### Marketplace Files (cc-joescharf-marketplace)

| File | Purpose |
|------|---------|
| `plugins/wt/.claude-plugin/plugin.json` | Plugin metadata |
| `plugins/wt/skills/manage-worktrees/SKILL.md` | Skill definition with MCP tools, CLI fallback, workflows |
| `.claude-plugin/marketplace.json` | Updated to include `wt` plugin |

## Verification

```bash
go test ./internal/mcp/... -count=1 2>&1 | tail -5
```

```output
ok  	github.com/joescharf/wt/internal/mcp	0.878s
```

```bash
go test ./... -count=1 2>&1 | grep -E '^(ok|FAIL|---)\s'
```

```output
ok  	github.com/joescharf/wt/cmd	1.316s
ok  	github.com/joescharf/wt/internal/claude	0.410s
ok  	github.com/joescharf/wt/internal/git	1.505s
ok  	github.com/joescharf/wt/internal/iterm	0.715s
ok  	github.com/joescharf/wt/internal/mcp	1.503s
ok  	github.com/joescharf/wt/internal/state	1.015s
```

```bash
go build -o /tmp/wt-test ./. && /tmp/wt-test mcp --help 2>&1 | head -6
```

```output
Start an MCP (Model Context Protocol) server on stdio.

This allows Claude Code to manage worktrees natively via MCP tools.
Configure in Claude Code with:

  wt mcp install
```

```bash
/tmp/wt-test mcp status 2>&1
```

```output
i ~/.claude.json: wt not configured (other servers present)
i .mcp.json (cwd): not found
```

## Installation

To install the MCP server in Claude Code:

```bash
wt mcp install    # Writes to ~/.claude.json
wt mcp status     # Verify configuration
# Restart Claude Code to pick up the change
```

After restart, the 6 `wt_*` tools will be available to Claude Code. The skill in the marketplace teaches Claude when and how to use them.
