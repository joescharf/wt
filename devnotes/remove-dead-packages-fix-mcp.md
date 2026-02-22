# Remove dead packages and fix MCP server

*2026-02-22T15:27:45Z*

Removed pkg/ops/ and pkg/lifecycle/ — parallel reimplementations of CLI logic with zero consumers and divergent behavior (-1,694 lines). Fixed the MCP server to use configurable baseBranch from viper config instead of hardcoding 'main', added post-merge cleanup (iTerm close, worktree removal, branch deletion, state cleanup), and added branch deletion to the delete handler. Added tests for custom baseBranch propagation and merge cleanup.

```bash
make test 2>&1 | tail -20
```

```output
=== RUN   TestEscapeAppleScript/has_\backslash
--- PASS: TestEscapeAppleScript (0.00s)
    --- PASS: TestEscapeAppleScript/normal (0.00s)
    --- PASS: TestEscapeAppleScript/has_"quotes" (0.00s)
    --- PASS: TestEscapeAppleScript/has_\backslash (0.00s)
=== RUN   TestScriptIsRunning
--- PASS: TestScriptIsRunning (0.00s)
PASS
ok  	github.com/joescharf/wt/pkg/iterm	1.717s
?   	github.com/joescharf/wt/pkg/iterm/mocks	[no test files]
=== RUN   TestStateRoundTrip
--- PASS: TestStateRoundTrip (0.00s)
=== RUN   TestStateRemoveWorktree
--- PASS: TestStateRemoveWorktree (0.00s)
=== RUN   TestStatePrune
--- PASS: TestStatePrune (0.00s)
=== RUN   TestLoadEmptyFile
--- PASS: TestLoadEmptyFile (0.00s)
PASS
ok  	github.com/joescharf/wt/pkg/wtstate	2.038s
```
