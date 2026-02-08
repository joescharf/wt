#!/bin/zsh

# worktree-dev.zsh - Git Worktree Manager with iTerm2 Integration
# Author: Joe Scharf <joe@joescharf.com>
#
# Creates git worktrees with dedicated iTerm2 windows (Claude on top, shell on bottom).
# Tracks active worktrees and their iTerm2 sessions in a state file.

set -euo pipefail

# Color codes for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly CYAN='\033[0;36m'
readonly NC='\033[0m' # No Color

# Default values
DRY_RUN=false
VERBOSE=false
SUBCOMMAND=""
BRANCH=""
BASE_BRANCH="main"
NO_CLAUDE=false
USE_EXISTING_BRANCH=false
FORCE=false
DELETE_BRANCH=false

# State file location
readonly STATE_DIR="$HOME/.config/worktree-dev"
readonly STATE_FILE="$STATE_DIR/state.json"
readonly SCRIPT_NAME="${ZSH_ARGZERO:t}"

# ─── Cleanup & Logging ────────────────────────────────────────────────────────

cleanup() {
    local exit_code=$?
    if [[ $exit_code -ne 0 ]] && [[ $exit_code -ne 2 ]]; then
        echo "${RED}✗ Script failed with exit code $exit_code${NC}" >&2
    fi
    exit $exit_code
}

trap cleanup EXIT INT TERM

log_info() {
    echo "${BLUE}ℹ${NC} $1"
}

log_success() {
    echo "${GREEN}✓${NC} $1"
}

log_warning() {
    echo "${YELLOW}⚠${NC} $1" >&2
}

log_error() {
    echo "${RED}✗${NC} $1" >&2
}

log_verbose() {
    if [[ "$VERBOSE" == true ]]; then
        echo "${BLUE}  →${NC} $1"
    fi
}

# Execute command with optional dry-run
execute() {
    local cmd="$1"
    local description="$2"

    log_info "$description"
    log_verbose "Command: $cmd"

    if [[ "$DRY_RUN" == true ]]; then
        log_warning "[DRY-RUN] Would execute: $cmd"
        return 0
    fi

    if eval "$cmd"; then
        log_success "$description - Done"
        return 0
    else
        log_error "$description - Failed"
        return 1
    fi
}

# ─── Usage ─────────────────────────────────────────────────────────────────────

usage() {
    local script_name="$SCRIPT_NAME"
    cat << EOF
Usage: $script_name [OPTIONS] <command> [args]

Git Worktree Manager with iTerm2 Integration.
Creates worktrees with dedicated iTerm2 windows (Claude on top, shell on bottom).

Commands:
    create <branch>    Create worktree + branch + iTerm2 window
    list               Show worktrees with iTerm2 window status
    switch <branch>    Focus existing worktree's iTerm2 window
    delete <branch>    Close iTerm2 window + remove worktree
    open <branch>      Re-open iTerm2 window for existing worktree
    help               Show this help message

Aliases: new=create, ls=list, go=switch, rm=delete

Global Options:
    -v, --verbose      Verbose output
    -n, --dry-run      Show what would happen
    -h, --help         Show this help message

Create Options:
    --base <branch>    Base branch (default: main)
    --no-claude        Don't auto-launch claude in top pane
    --existing         Use existing branch instead of creating new

Delete Options:
    --force            Force removal with uncommitted changes
    --delete-branch    Also delete the git branch

Examples:
    $script_name create feature/auth
    $script_name create bugfix/login --base develop --no-claude
    $script_name create feature/existing-work --existing
    $script_name list
    $script_name switch feature/auth
    $script_name open feature/auth
    $script_name delete feature/auth --delete-branch
    $script_name -n create dry-test

EOF
}

# ─── Git Worktree Functions ────────────────────────────────────────────────────

# Detect the main repo root (works from within a worktree too)
get_repo_root() {
    local git_common_dir
    git_common_dir=$(git rev-parse --git-common-dir 2>/dev/null) || {
        log_error "Not inside a git repository"
        return 1
    }

    # git-common-dir returns the path to the shared .git directory
    # For a main repo: .git (relative) or /abs/path/.git
    # For a worktree: /abs/path/to/main/.git
    if [[ "$git_common_dir" == ".git" ]]; then
        # We're in the main repo root
        pwd
    else
        # Resolve to absolute path and get parent
        local abs_git_dir
        abs_git_dir=$(cd "$git_common_dir" && pwd)
        dirname "$abs_git_dir"
    fi
}

# Returns the worktrees directory: <repo-root>.worktrees
get_worktrees_dir() {
    local repo_root
    repo_root=$(get_repo_root) || return 1
    echo "${repo_root}.worktrees"
}

# Get repo name from repo root
get_repo_name() {
    local repo_root
    repo_root=$(get_repo_root) || return 1
    basename "$repo_root"
}

# Convert branch name to directory name: feature/foo -> foo
branch_to_dirname() {
    local branch="$1"
    # Take the last path segment
    echo "${branch##*/}"
}

# Resolve a branch name or dirname to the full worktree path
# Accepts: "feature/foo", "foo", or full path
resolve_worktree() {
    local input="$1"
    local worktrees_dir
    worktrees_dir=$(get_worktrees_dir) || return 1

    # If it's already a full path
    if [[ "$input" == /* ]]; then
        echo "$input"
        return 0
    fi

    # Try as dirname first
    local candidate="$worktrees_dir/$input"
    if [[ -d "$candidate" ]]; then
        echo "$candidate"
        return 0
    fi

    # Try converting from branch name
    local dirname
    dirname=$(branch_to_dirname "$input")
    candidate="$worktrees_dir/$dirname"
    if [[ -d "$candidate" ]]; then
        echo "$candidate"
        return 0
    fi

    # Not found - return the expected path (for create)
    echo "$candidate"
    return 0
}

# Parse `git worktree list --porcelain` and return worktree info
# Output: tab-separated lines of "path\tbranch\tHEAD"
get_worktree_list() {
    local repo_root
    repo_root=$(get_repo_root) || return 1

    git -C "$repo_root" worktree list --porcelain 2>/dev/null | awk '
        /^worktree / { path = substr($0, 10) }
        /^HEAD /     { head = substr($0, 6) }
        /^branch /   { branch = substr($0, 8) }
        /^$/ {
            if (path != "") {
                sub(/^refs\/heads\//, "", branch)
                print path "\t" branch "\t" head
                path = ""; branch = ""; head = ""
            }
        }
        END {
            if (path != "") {
                sub(/^refs\/heads\//, "", branch)
                print path "\t" branch "\t" head
            }
        }
    '
}

# ─── State Management ─────────────────────────────────────────────────────────

ensure_state_file() {
    if [[ ! -d "$STATE_DIR" ]]; then
        mkdir -p "$STATE_DIR"
    fi
    if [[ ! -f "$STATE_FILE" ]]; then
        echo '{"worktrees":{}}' > "$STATE_FILE"
    fi
}

read_state() {
    ensure_state_file
    cat "$STATE_FILE"
}

# Write worktree state entry
# Args: worktree_path repo_name branch claude_session_id shell_session_id
write_worktree_state() {
    local wt_path="$1"
    local repo_name="$2"
    local branch="$3"
    local claude_sid="$4"
    local shell_sid="$5"
    local created_at
    created_at=$(date -u +"%Y-%m-%dT%H:%M:%S")

    ensure_state_file

    local tmp_file="${STATE_FILE}.tmp"
    jq --arg path "$wt_path" \
       --arg repo "$repo_name" \
       --arg branch "$branch" \
       --arg csid "$claude_sid" \
       --arg ssid "$shell_sid" \
       --arg created "$created_at" \
       '.worktrees[$path] = {
            repo: $repo,
            branch: $branch,
            claude_session_id: $csid,
            shell_session_id: $ssid,
            created_at: $created
        }' "$STATE_FILE" > "$tmp_file" && mv "$tmp_file" "$STATE_FILE"
}

remove_worktree_state() {
    local wt_path="$1"
    ensure_state_file

    local tmp_file="${STATE_FILE}.tmp"
    jq --arg path "$wt_path" 'del(.worktrees[$path])' "$STATE_FILE" > "$tmp_file" && mv "$tmp_file" "$STATE_FILE"
}

# Get session IDs for a worktree path
# Returns: claude_session_id\tshell_session_id
get_worktree_sessions() {
    local wt_path="$1"
    ensure_state_file

    jq -r --arg path "$wt_path" \
        '.worktrees[$path] // empty | "\(.claude_session_id)\t\(.shell_session_id)"' \
        "$STATE_FILE"
}

# Remove state entries for worktrees that no longer exist
prune_stale_state() {
    ensure_state_file

    local paths
    paths=$(jq -r '.worktrees | keys[]' "$STATE_FILE" 2>/dev/null) || return 0

    local pruned=0
    while IFS= read -r wt_path; do
        [[ -z "$wt_path" ]] && continue
        if [[ ! -d "$wt_path" ]]; then
            log_verbose "Pruning stale entry: $wt_path"
            remove_worktree_state "$wt_path"
            ((pruned++))
        fi
    done <<< "$paths"

    if [[ $pruned -gt 0 ]]; then
        log_info "Pruned $pruned stale state entries"
    fi
}

# ─── iTerm2 AppleScript Functions ─────────────────────────────────────────────

iterm2_is_running() {
    pgrep -x "iTerm2" > /dev/null 2>&1
}

# Create an iTerm2 window with two panes for a worktree
# Args: worktree_path session_name no_claude
# Outputs: claude_session_id\tshell_session_id
iterm2_create_worktree_window() {
    local wt_path="$1"
    local session_name="$2"
    local no_claude="${3:-false}"

    if ! iterm2_is_running; then
        log_error "iTerm2 is not running. Please start iTerm2 first."
        return 1
    fi

    local claude_cmd="cd '$wt_path'"
    if [[ "$no_claude" == false ]]; then
        claude_cmd="cd '$wt_path' && claude"
    fi

    local result
    result=$(osascript << APPLESCRIPT
        tell application "iTerm2"
            -- Create a new window
            set newWindow to (create window with default profile)

            tell newWindow
                tell current session of current tab
                    -- This becomes the top pane (Claude)
                    set name to "${session_name}:claude"
                    write text "${claude_cmd}"
                    set claudeID to unique ID
                end tell

                -- Split horizontally to create bottom pane (shell)
                tell current session of current tab
                    set shellSession to (split horizontally with default profile)
                end tell

                tell shellSession
                    set name to "${session_name}:shell"
                    write text "cd '$wt_path'"
                    set shellID to unique ID
                end tell
            end tell

            -- Return the session IDs
            return claudeID & "	" & shellID
        end tell
APPLESCRIPT
    ) || {
        log_error "Failed to create iTerm2 window"
        return 1
    }

    echo "$result"
}

# Check if an iTerm2 session still exists
iterm2_session_exists() {
    local session_id="$1"
    [[ -z "$session_id" ]] && return 1

    local exists
    exists=$(osascript << APPLESCRIPT
        tell application "iTerm2"
            repeat with w in windows
                repeat with t in tabs of w
                    repeat with s in sessions of t
                        if unique ID of s is "${session_id}" then
                            return "true"
                        end if
                    end repeat
                end repeat
            end repeat
            return "false"
        end tell
APPLESCRIPT
    ) 2>/dev/null || return 1

    [[ "$exists" == "true" ]]
}

# Focus the window containing a session
iterm2_focus_window() {
    local session_id="$1"
    [[ -z "$session_id" ]] && return 1

    osascript << APPLESCRIPT
        tell application "iTerm2"
            repeat with w in windows
                repeat with t in tabs of w
                    repeat with s in sessions of t
                        if unique ID of s is "${session_id}" then
                            select t
                            tell w to select
                            activate
                            return true
                        end if
                    end repeat
                end repeat
            end repeat
            return false
        end tell
APPLESCRIPT
}

# Close the window containing a session
iterm2_close_window() {
    local session_id="$1"
    [[ -z "$session_id" ]] && return 1

    osascript << APPLESCRIPT
        tell application "iTerm2"
            repeat with w in windows
                repeat with t in tabs of w
                    repeat with s in sessions of t
                        if unique ID of s is "${session_id}" then
                            close w
                            return true
                        end if
                    end repeat
                end repeat
            end repeat
            return false
        end tell
APPLESCRIPT
}

# ─── Subcommand Implementations ───────────────────────────────────────────────

cmd_create() {
    if [[ -z "$BRANCH" ]]; then
        log_error "Branch name is required"
        echo "Usage: $SCRIPT_NAME create <branch> [--base <branch>] [--no-claude] [--existing]"
        return 1
    fi

    local repo_root repo_name worktrees_dir dirname wt_path
    repo_root=$(get_repo_root) || return 1
    repo_name=$(get_repo_name) || return 1
    worktrees_dir=$(get_worktrees_dir)
    dirname=$(branch_to_dirname "$BRANCH")
    wt_path="$worktrees_dir/$dirname"

    log_info "Creating worktree for branch '${CYAN}${BRANCH}${NC}' in repo '${CYAN}${repo_name}${NC}'"
    log_verbose "Repo root:     $repo_root"
    log_verbose "Worktrees dir: $worktrees_dir"
    log_verbose "Worktree path: $wt_path"
    log_verbose "Base branch:   $BASE_BRANCH"

    # Check if worktree already exists
    if [[ -d "$wt_path" ]]; then
        log_error "Worktree already exists at: $wt_path"
        log_info "Use '$SCRIPT_NAME open $BRANCH' to open an iTerm2 window for it"
        return 1
    fi

    # Create worktrees directory if needed
    if [[ ! -d "$worktrees_dir" ]]; then
        execute "mkdir -p '$worktrees_dir'" "Creating worktrees directory"
    fi

    # Create worktree
    if [[ "$USE_EXISTING_BRANCH" == true ]]; then
        # Use existing branch
        execute "git -C '$repo_root' worktree add '$wt_path' '$BRANCH'" \
                "Creating worktree from existing branch '$BRANCH'"
    else
        # Create new branch from base
        execute "git -C '$repo_root' worktree add -b '$BRANCH' '$wt_path' '$BASE_BRANCH'" \
                "Creating worktree with new branch '$BRANCH' from '$BASE_BRANCH'"
    fi

    # Create iTerm2 window
    if [[ "$DRY_RUN" == true ]]; then
        log_warning "[DRY-RUN] Would create iTerm2 window for $wt_path"
        log_warning "[DRY-RUN] Would save state to $STATE_FILE"
        return 0
    fi

    local session_name="wt:${repo_name}:${dirname}"
    log_info "Creating iTerm2 window (session: ${CYAN}${session_name}${NC})"

    local session_ids
    session_ids=$(iterm2_create_worktree_window "$wt_path" "$session_name" "$NO_CLAUDE") || {
        log_warning "Worktree created but failed to open iTerm2 window"
        log_info "Use '$SCRIPT_NAME open $BRANCH' to try again"
        return 0
    }

    local claude_sid shell_sid
    claude_sid=$(echo "$session_ids" | cut -f1)
    shell_sid=$(echo "$session_ids" | cut -f2)
    log_verbose "Claude session: $claude_sid"
    log_verbose "Shell session:  $shell_sid"

    # Save state
    write_worktree_state "$wt_path" "$repo_name" "$BRANCH" "$claude_sid" "$shell_sid"

    echo
    log_success "Worktree ready: ${CYAN}${wt_path}${NC}"
    log_success "iTerm2 window opened with Claude + shell panes"
}

cmd_list() {
    local repo_root repo_name worktrees_dir
    repo_root=$(get_repo_root) || return 1
    repo_name=$(get_repo_name) || return 1
    worktrees_dir=$(get_worktrees_dir)

    prune_stale_state

    echo "${BLUE}Worktrees for ${CYAN}${repo_name}${NC}"
    echo

    # Table header
    printf "  ${BLUE}%-20s %-30s %-12s %-10s${NC}\n" "BRANCH" "PATH" "WINDOW" "AGE"
    printf "  %-20s %-30s %-12s %-10s\n" "────────────────────" "──────────────────────────────" "────────────" "──────────"

    local found=false
    local worktree_data
    worktree_data=$(get_worktree_list) || return 1

    while IFS=$'\t' read -r wt_path wt_branch wt_head; do
        [[ -z "$wt_path" ]] && continue

        # Skip the main repo worktree (it's the repo itself)
        if [[ "$wt_path" == "$repo_root" ]]; then
            continue
        fi

        found=true

        # Check iTerm2 window status
        local window_status="${RED}closed${NC}"
        local sessions
        sessions=$(get_worktree_sessions "$wt_path")
        if [[ -n "$sessions" ]]; then
            local claude_sid shell_sid
            claude_sid=$(echo "$sessions" | cut -f1)
            shell_sid=$(echo "$sessions" | cut -f2)

            if iterm2_is_running && iterm2_session_exists "$claude_sid" 2>/dev/null; then
                window_status="${GREEN}open${NC}"
            else
                window_status="${YELLOW}stale${NC}"
            fi
        fi

        # Calculate age from state file
        local age="-"
        local created_at
        created_at=$(jq -r --arg path "$wt_path" '.worktrees[$path].created_at // empty' "$STATE_FILE" 2>/dev/null)
        if [[ -n "$created_at" ]]; then
            local created_ts now_ts diff_s
            created_ts=$(date -j -f "%Y-%m-%dT%H:%M:%S" "$created_at" "+%s" 2>/dev/null) || created_ts=0
            now_ts=$(date "+%s")
            if [[ $created_ts -gt 0 ]]; then
                diff_s=$((now_ts - created_ts))
                if [[ $diff_s -lt 3600 ]]; then
                    age="$((diff_s / 60))m"
                elif [[ $diff_s -lt 86400 ]]; then
                    age="$((diff_s / 3600))h"
                else
                    age="$((diff_s / 86400))d"
                fi
            fi
        fi

        # Shorten path for display
        local display_path
        display_path=$(basename "$wt_path")

        # Strip ANSI codes to get visible length for padding
        local visible_status="${window_status//\033\[[0-9;]*m/}"
        local pad_len=$(( 13 - ${#visible_status} ))
        [[ $pad_len -lt 1 ]] && pad_len=1
        printf -v line "  %-20s %-30s " "$wt_branch" "$display_path"
        echo "${line}${window_status}$(printf '%*s' $pad_len '')${age}"
    done <<< "$worktree_data"

    if [[ "$found" == false ]]; then
        echo "  ${YELLOW}No worktrees found${NC}"
    fi

    echo
}

cmd_switch() {
    if [[ -z "$BRANCH" ]]; then
        log_error "Branch name is required"
        echo "Usage: $SCRIPT_NAME switch <branch>"
        return 1
    fi

    local wt_path
    wt_path=$(resolve_worktree "$BRANCH") || return 1

    if [[ ! -d "$wt_path" ]]; then
        log_error "Worktree not found: $wt_path"
        return 1
    fi

    local sessions
    sessions=$(get_worktree_sessions "$wt_path")

    if [[ -z "$sessions" ]]; then
        log_warning "No iTerm2 session recorded for this worktree"
        log_info "Use '$SCRIPT_NAME open $BRANCH' to create a window"
        return 1
    fi

    local claude_sid
    claude_sid=$(echo "$sessions" | cut -f1)

    if [[ "$DRY_RUN" == true ]]; then
        log_warning "[DRY-RUN] Would focus iTerm2 window for session $claude_sid"
        return 0
    fi

    if ! iterm2_is_running; then
        log_error "iTerm2 is not running"
        return 1
    fi

    if iterm2_session_exists "$claude_sid"; then
        iterm2_focus_window "$claude_sid" > /dev/null
        log_success "Focused iTerm2 window for '${CYAN}$(basename "$wt_path")${NC}'"
    else
        log_warning "iTerm2 window no longer exists"
        log_info "Use '$SCRIPT_NAME open $BRANCH' to create a new window"
    fi
}

cmd_delete() {
    if [[ -z "$BRANCH" ]]; then
        log_error "Branch name is required"
        echo "Usage: $SCRIPT_NAME delete <branch> [--force] [--delete-branch]"
        return 1
    fi

    local repo_root wt_path dirname
    repo_root=$(get_repo_root) || return 1
    wt_path=$(resolve_worktree "$BRANCH") || return 1
    dirname=$(basename "$wt_path")

    if [[ ! -d "$wt_path" ]]; then
        log_error "Worktree not found: $wt_path"
        # Clean up stale state if any
        remove_worktree_state "$wt_path" 2>/dev/null
        return 1
    fi

    log_info "Deleting worktree '${CYAN}${dirname}${NC}'"

    # Close iTerm2 window if it exists
    local sessions
    sessions=$(get_worktree_sessions "$wt_path")
    if [[ -n "$sessions" ]]; then
        local claude_sid
        claude_sid=$(echo "$sessions" | cut -f1)

        if [[ "$DRY_RUN" == true ]]; then
            log_warning "[DRY-RUN] Would close iTerm2 window"
        elif iterm2_is_running && iterm2_session_exists "$claude_sid" 2>/dev/null; then
            iterm2_close_window "$claude_sid" > /dev/null 2>&1
            log_success "Closed iTerm2 window"
            # Small delay for iTerm2 to process
            sleep 0.5
        fi
    fi

    # Remove worktree
    local force_flag=""
    if [[ "$FORCE" == true ]]; then
        force_flag="--force"
    fi
    execute "git -C '$repo_root' worktree remove $force_flag '$wt_path'" \
            "Removing git worktree"

    # Delete branch if requested
    if [[ "$DELETE_BRANCH" == true ]]; then
        # Determine the branch name from state or resolve it
        local branch_name
        branch_name=$(jq -r --arg path "$wt_path" '.worktrees[$path].branch // empty' "$STATE_FILE" 2>/dev/null)
        if [[ -z "$branch_name" ]]; then
            branch_name="$BRANCH"
        fi

        if [[ "$DRY_RUN" == true ]]; then
            log_warning "[DRY-RUN] Would delete branch '$branch_name'"
        else
            if git -C "$repo_root" branch -d "$branch_name" 2>/dev/null; then
                log_success "Deleted branch '$branch_name'"
            elif [[ "$FORCE" == true ]] && git -C "$repo_root" branch -D "$branch_name" 2>/dev/null; then
                log_success "Force-deleted branch '$branch_name'"
            else
                log_warning "Could not delete branch '$branch_name' (may not exist or not fully merged)"
            fi
        fi
    fi

    # Remove state entry
    if [[ "$DRY_RUN" == false ]]; then
        remove_worktree_state "$wt_path"
    fi

    echo
    log_success "Worktree '${CYAN}${dirname}${NC}' removed"
}

cmd_open() {
    if [[ -z "$BRANCH" ]]; then
        log_error "Branch name is required"
        echo "Usage: $SCRIPT_NAME open <branch>"
        return 1
    fi

    local repo_name wt_path dirname
    repo_name=$(get_repo_name) || return 1
    wt_path=$(resolve_worktree "$BRANCH") || return 1
    dirname=$(basename "$wt_path")

    if [[ ! -d "$wt_path" ]]; then
        log_error "Worktree not found: $wt_path"
        log_info "Use '$SCRIPT_NAME create $BRANCH' to create it"
        return 1
    fi

    # Check if window already exists
    local sessions
    sessions=$(get_worktree_sessions "$wt_path")
    if [[ -n "$sessions" ]]; then
        local claude_sid
        claude_sid=$(echo "$sessions" | cut -f1)
        if iterm2_is_running && iterm2_session_exists "$claude_sid" 2>/dev/null; then
            log_info "iTerm2 window already open, focusing it"
            iterm2_focus_window "$claude_sid" > /dev/null
            return 0
        fi
    fi

    if [[ "$DRY_RUN" == true ]]; then
        log_warning "[DRY-RUN] Would open iTerm2 window for $wt_path"
        return 0
    fi

    local session_name="wt:${repo_name}:${dirname}"
    log_info "Opening iTerm2 window for '${CYAN}${dirname}${NC}'"

    local session_ids
    session_ids=$(iterm2_create_worktree_window "$wt_path" "$session_name" "$NO_CLAUDE") || {
        log_error "Failed to create iTerm2 window"
        return 1
    }

    local claude_sid shell_sid branch
    claude_sid=$(echo "$session_ids" | cut -f1)
    shell_sid=$(echo "$session_ids" | cut -f2)

    # Get branch from state or git
    branch=$(jq -r --arg path "$wt_path" '.worktrees[$path].branch // empty' "$STATE_FILE" 2>/dev/null)
    if [[ -z "$branch" ]]; then
        branch=$(git -C "$wt_path" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "$BRANCH")
    fi

    write_worktree_state "$wt_path" "$repo_name" "$branch" "$claude_sid" "$shell_sid"

    log_success "iTerm2 window opened for '${CYAN}${dirname}${NC}'"
}

# ─── Argument Parsing ──────────────────────────────────────────────────────────

parse_args() {
    # Parse global flags first, collect remaining args
    local remaining=()

    while [[ $# -gt 0 ]]; do
        case $1 in
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -n|--dry-run)
                DRY_RUN=true
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                remaining+=("$1")
                shift
                ;;
        esac
    done

    # Need at least a subcommand
    if [[ ${#remaining[@]} -eq 0 ]]; then
        log_error "No command specified"
        echo
        usage
        exit 1
    fi

    # Extract subcommand (with aliases)
    case "${remaining[1]}" in
        create|new)     SUBCOMMAND="create" ;;
        list|ls)        SUBCOMMAND="list" ;;
        switch|go)      SUBCOMMAND="switch" ;;
        delete|rm)      SUBCOMMAND="delete" ;;
        open)           SUBCOMMAND="open" ;;
        help)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown command: ${remaining[1]}"
            echo
            usage
            exit 1
            ;;
    esac

    # Parse subcommand-specific flags and positional args
    local i=2
    while [[ $i -le ${#remaining[@]} ]]; do
        local arg="${remaining[$i]}"
        case "$arg" in
            --base)
                ((i++))
                BASE_BRANCH="${remaining[$i]}"
                ;;
            --no-claude)
                NO_CLAUDE=true
                ;;
            --existing)
                USE_EXISTING_BRANCH=true
                ;;
            --force)
                FORCE=true
                ;;
            --delete-branch)
                DELETE_BRANCH=true
                ;;
            -*)
                log_error "Unknown option for '$SUBCOMMAND': $arg"
                exit 1
                ;;
            *)
                # Positional arg = branch name
                if [[ -z "$BRANCH" ]]; then
                    BRANCH="$arg"
                else
                    log_error "Unexpected argument: $arg"
                    exit 1
                fi
                ;;
        esac
        ((i++))
    done
}

# ─── Dependency Check ─────────────────────────────────────────────────────────

check_dependencies() {
    local missing=()

    if ! command -v jq &> /dev/null; then
        missing+=("jq")
    fi
    if ! command -v git &> /dev/null; then
        missing+=("git")
    fi
    if ! command -v osascript &> /dev/null; then
        missing+=("osascript")
    fi

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required dependencies: ${missing[*]}"
        return 1
    fi
}

# ─── Main ─────────────────────────────────────────────────────────────────────

main() {
    parse_args "$@"
    check_dependencies || return 1

    if [[ "$DRY_RUN" == true ]]; then
        log_warning "DRY-RUN mode enabled"
    fi

    case "$SUBCOMMAND" in
        create)  cmd_create ;;
        list)    cmd_list ;;
        switch)  cmd_switch ;;
        delete)  cmd_delete ;;
        open)    cmd_open ;;
    esac
}

main "$@"
