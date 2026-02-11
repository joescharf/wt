# wt-create.tape — Show creating a new worktree (dry-run)
# Usage: vhs wt-create.tape

Output ../docs/assets/screenshots/wt-create.gif
Output ../docs/assets/screenshots/wt-create.webm

Set Shell "zsh"
Set Theme "Catppuccin Mocha"
Set Width 1000
Set Height 450
Set FontSize 16
Set Padding 20
Set TypingSpeed 40ms
Set CursorBlink false

# Show create help first
Type "wt create --help"
Sleep 300ms
Enter
Sleep 3s

# Show dry-run of creating a worktree
Type "wt create feature/auth --dry-run"
Sleep 300ms
Enter
Sleep 3s
