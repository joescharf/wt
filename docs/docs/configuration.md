# Configuration

`wt` uses an optional YAML configuration file with environment variable overrides.

## Config File

**Location:** `~/.config/wt/config.yaml`

```yaml
base_branch: main    # Default base branch for new worktrees
no_claude: false     # Skip launching Claude in top pane
rebase: false        # Use rebase instead of merge for sync/merge
```

### Config Keys

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `base_branch` | string | `main` | Default base branch for `create`, `sync`, and `merge` |
| `no_claude` | bool | `false` | Skip launching Claude Code in the top pane on `create`/`open` |
| `rebase` | bool | `false` | Use rebase instead of merge as the default strategy for `sync` and `merge` |

## Environment Variables

All config keys can be overridden with environment variables using the `WT_` prefix:

```bash
export WT_BASE_BRANCH=develop
export WT_NO_CLAUDE=true
export WT_REBASE=true
```

## Precedence

Configuration values are resolved in this order (highest priority first):

1. **Command-line flags** (e.g. `--base develop`, `--rebase`, `--merge`)
2. **Environment variables** (`WT_BASE_BRANCH`, `WT_NO_CLAUDE`, `WT_REBASE`)
3. **Config file** (`~/.config/wt/config.yaml`)
4. **Defaults** (`main`, `false`, `false`)

## Managing Configuration

### Initialize config file

```bash
wt config init
```

Creates `~/.config/wt/config.yaml` with commented defaults reflecting current effective values. Refuses to overwrite an existing file unless `--force` is passed.

```bash
wt config init --force    # Overwrite existing config
wt config init -n         # Dry-run: show what would be created
```

### View effective config

```bash
wt config show
```

Displays each config key with its effective value and source:

```
base_branch: main       (default)
no_claude:   false      (file)
rebase:      true       (env: WT_REBASE)
```

### Edit config file

```bash
wt config edit
```

Opens the config file in `$EDITOR` (or `$VISUAL`). Errors if neither is set or if the config file doesn't exist yet (run `wt config init` first).

## Merge Strategy

The `rebase` config key controls the default merge strategy for both `sync` and `merge` commands:

| Config | `sync` behavior | `merge` behavior |
|--------|----------------|-----------------|
| `rebase: false` (default) | Merges base into feature | Merge commit into base |
| `rebase: true` | Rebases feature onto base | Rebase-then-fast-forward into base |

Override per-command with `--rebase` or `--merge` flags:

```bash
# Config says merge, but rebase this one time
wt sync feature/auth --rebase

# Config says rebase, but merge this one time
wt merge feature/auth --merge
```
