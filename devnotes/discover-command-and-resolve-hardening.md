# Discover Command & ResolveWorktree Hardening

*2026-02-22T05:20:16Z*

Added 'wt discover' command to find worktrees not managed by wt (e.g. those created by Claude Code's EnterWorktree). Supports --adopt to register them in state. Hardened ResolveWorktree to return errors for non-existent paths instead of optimistically returning unverified paths, and extracted ResolveWorktreeFromList as a shared pure function used by both CLI and MCP server. Added SOURCE column to 'wt list' showing wt/adopted/external classification.

```bash
go test -v -race -count=1 -run 'TestDiscover|TestWorktreeSource|TestResolveWorktree' ./cmd/ ./pkg/gitops/ 2>&1 | tail -30
```

```output
=== RUN   TestDiscover_FindsUnmanaged
--- PASS: TestDiscover_FindsUnmanaged (0.00s)
=== RUN   TestDiscover_NoneFound
--- PASS: TestDiscover_NoneFound (0.00s)
=== RUN   TestDiscover_Adopt
--- PASS: TestDiscover_Adopt (0.00s)
=== RUN   TestDiscover_DryRun
--- PASS: TestDiscover_DryRun (0.00s)
=== RUN   TestWorktreeSource
--- PASS: TestWorktreeSource (0.00s)
PASS
ok  	github.com/joescharf/wt/cmd	2.143s
=== RUN   TestResolveWorktreePath
--- PASS: TestResolveWorktreePath (0.00s)
=== RUN   TestResolveWorktreeFromList
--- PASS: TestResolveWorktreeFromList (0.00s)
PASS
ok  	github.com/joescharf/wt/pkg/gitops	1.524s
```
