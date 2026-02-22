# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`wt` is a Git worktree manager with iTerm2 integration, written in Go. It creates git worktrees with dedicated iTerm2 windows (Claude Code on top, shell on bottom) and tracks them across repos. macOS only.

## Commands

| Task | Command |
|---|---|
| Build | `make build` |
| Test all | `make test` |
| Test single | `go test -v -race -count=1 ./pkg/gitops/` |
| Test single func | `go test -v -race -run TestFunctionName ./cmd/` |
| Lint | `make lint` |
| Format | `make fmt` |
| Vet | `go vet ./...` |
| Regenerate mocks | `mockery` (uses `.mockery.yml`) |

## Architecture

**CLI layer** (`cmd/`): Cobra commands with Viper config. Package-level vars (`gitClient`, `itermClient`, `stateMgr`, `claudeTrust`, `output`) initialized in `cobra.OnInitialize(initConfig, initDeps)`.

**Business logic** (`pkg/ops/`): Pure functions (`Sync`, `Merge`, `Delete`, `Prune`) that accept interfaces and return result structs. Used by both CLI and lifecycle manager.

**Lifecycle orchestrator** (`pkg/lifecycle/`): Combines git+iterm+state+trust operations for create/open/delete. Designed for library consumers.

**Git operations** (`pkg/gitops/`): `Client` interface with `RealClient` that shells out to `git`. CWD-based.

**iTerm2 integration** (`pkg/iterm/`): `Client` interface using AppleScript via `osascript`.

**State persistence** (`pkg/wtstate/`): `Manager` for `state.json` with atomic file writes (temp + rename).

**Claude trust** (`pkg/claude/`): Manages `hasTrustDialogAccepted` in `~/.claude.json`.

**MCP server** (`internal/mcp/`): Path-based `GitClient` interface (no CWD context for stdio). Uses `mark3labs/mcp-go`.

**UI** (`internal/ui/`): `UI` struct with colored output, verbose/dry-run modes, terminal-width-aware tables.

### Two Git Client Variants
- `pkg/gitops.Client` — CWD-based, used by CLI commands
- `internal/mcp.GitClient` — path-based (every method takes `repoPath`), used by MCP server

### Strategy Resolution
`resolveStrategy()` in `cmd/root.go`: `--rebase` flag > `--merge` flag > viper `rebase` config > default "merge".

## Configuration

Viper-based. Config file: `~/.config/wt/config.yaml`. Env prefix: `WT_`.

Use `viper.SetDefault()` for defaults, `viper.GetString()`/`viper.GetBool()` for reading. Precedence: env vars > config file > defaults.

Keys: `base_branch` (default "main"), `no_claude` (false), `rebase` (false), `state_dir` (~/.config/wt).

## Testing Patterns

- Framework: `stretchr/testify` (assert, require, mock)
- Mocks: Mockery-generated with expecters (`mockGit.EXPECT().Method().Return(...)`)
- cmd tests: `setupTest(t)` replaces package-level vars with mocks, resets all flags and viper between tests
- pkg tests: Real git repos via `initTestRepo()` helper, `t.TempDir()` for isolation
- macOS path note: Use `filepath.EvalSymlinks()` for `/var` → `/private/var` consistency
- Replaceable function vars for testing: `configDirFunc`, `promptFunc`, `promptDefaultYes`, `ghPRCreateFunc`

## Key Conventions

- Atomic file writes everywhere (temp file + rename)
- Every command supports `-n/--dry-run` — check `dryRun` before mutating
- `create` delegates to `open` if worktree already exists (idempotent)
- Worktrees placed as siblings: `repo.worktrees/<dirname>/`
- GoReleaser for releases (darwin-only universal binary, code-signed + notarized)
