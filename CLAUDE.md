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

**CLI layer** (`cmd/`): Cobra commands with Viper config. Thin wrappers that parse flags, resolve worktree/branch, then delegate to `pkg/ops` or `pkg/lifecycle`. Package-level vars (`gitClient`, `itermClient`, `stateMgr`, `claudeTrust`, `output`, `opsLogger`, `lcMgr`) initialized in `cobra.OnInitialize(initConfig, initDeps)`. `uiLogger` adapter bridges `ui.UI` to `ops.Logger`.

**Business logic** (`pkg/ops/`): Pure functions (`Sync`, `Merge`, `Delete`, `Prune`, `Discover`) that accept interfaces and return result structs. Uses callback types (`SafetyCheckFunc`, `CleanupFunc`, `PRCreateFunc`) for dependency injection. Used by both CLI and lifecycle manager.

**Lifecycle orchestrator** (`pkg/lifecycle/`): `Manager` combines git+iterm+state+trust operations for create/open/delete. Designed for library consumers (e.g., `pm`).

**Git operations** (`pkg/gitops/`): Path-based `Client` interface with `RealClient` that shells out to `git -C repoPath`. Every method takes `repoPath` as first parameter.

**iTerm2 integration** (`pkg/iterm/`): `Client` interface using AppleScript via `osascript`.

**State persistence** (`pkg/wtstate/`): `Manager` for `state.json` with atomic file writes (temp + rename).

**Claude trust** (`pkg/claude/`): Manages `hasTrustDialogAccepted` in `~/.claude.json`.

**MCP server** (`internal/mcp/`): Exposes worktree operations as MCP tools over stdio. Uses `mark3labs/mcp-go` and the shared `pkg/gitops.Client`.

**UI** (`internal/ui/`): `UI` struct with colored output, verbose/dry-run modes, terminal-width-aware tables.

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
