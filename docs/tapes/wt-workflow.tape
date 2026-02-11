# wt-workflow.tape — Complete workflow demo using dry-run
# Usage: vhs wt-workflow.tape
# Shows the typical create → list → sync → merge lifecycle

Output ../docs/assets/screenshots/wt-workflow.gif
Output ../docs/assets/screenshots/wt-workflow.webm

Set Shell "zsh"
Set Theme "Catppuccin Mocha"
Set Width 1100
Set Height 600
Set FontSize 16
Set Padding 20
Set TypingSpeed 40ms
Set CursorBlink false

# Step 1: Create a worktree
Type "# Step 1: Create a worktree for your feature"
Enter
Sleep 500ms
Type "wt create feature/auth --dry-run"
Sleep 300ms
Enter
Sleep 3s

# Step 2: List worktrees
Type "# Step 2: See all active worktrees"
Enter
Sleep 500ms
Type "wt list"
Sleep 300ms
Enter
Sleep 3s

# Step 3: Sync with base branch
Type "# Step 3: Sync with latest main"
Enter
Sleep 500ms
Type "wt sync --all --dry-run"
Sleep 300ms
Enter
Sleep 3s

# Step 4: Merge back
Type "# Step 4: Merge back to main when done"
Enter
Sleep 500ms
Type "wt merge feat-create-screenshots --dry-run"
Sleep 300ms
Enter
Sleep 4s
