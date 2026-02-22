# Rebuild pkg/lifecycle for orchestrated worktree management

*2026-02-22T16:55:38Z*

Rebuilt pkg/lifecycle as a Manager that orchestrates git+iterm+state+trust for worktree create/open/delete. Extracted from cmd/create.go, cmd/open.go, and cmd/delete.go (cleanupWorktree). Create delegates to Open when worktree exists (idempotent). Open checks IsRunning before SessionExists. Delete closes iTerm with 500ms delay, removes worktree, optionally deletes branch with force fallback, and cleans state+trust. 18 tests pass with race detector.

```bash
go test -v -race -count=1 ./pkg/lifecycle/ 2>&1 | tail -25
```

```output
--- PASS: TestCreate_WithTrust (0.00s)
=== RUN   TestOpen_NewWindow
--- PASS: TestOpen_NewWindow (0.00s)
=== RUN   TestOpen_FocusExistingWindow
--- PASS: TestOpen_FocusExistingWindow (0.00s)
=== RUN   TestOpen_StaleSession_CreatesNew
--- PASS: TestOpen_StaleSession_CreatesNew (0.00s)
=== RUN   TestOpen_DryRun
--- PASS: TestOpen_DryRun (0.00s)
=== RUN   TestDelete_FullCleanup
--- PASS: TestDelete_FullCleanup (0.50s)
=== RUN   TestDelete_NoBranchDelete
--- PASS: TestDelete_NoBranchDelete (0.00s)
=== RUN   TestDelete_Force_BranchFallback
--- PASS: TestDelete_Force_BranchFallback (0.00s)
=== RUN   TestDelete_BranchFromState
--- PASS: TestDelete_BranchFromState (0.00s)
=== RUN   TestDelete_DryRun
--- PASS: TestDelete_DryRun (0.00s)
=== RUN   TestDelete_WithTrust
--- PASS: TestDelete_WithTrust (0.00s)
=== RUN   TestDelete_ITermNotRunning
--- PASS: TestDelete_ITermNotRunning (0.00s)
PASS
ok  	github.com/joescharf/wt/pkg/lifecycle	1.703s
```
