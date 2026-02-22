# Wire CLI Commands to pkg/ops and pkg/lifecycle

*2026-02-22T17:11:36Z*

Rewired all CLI commands (create, open, delete, sync, merge, prune, discover) to delegate to pkg/ops and pkg/lifecycle instead of inline logic. Added uiLogger adapter and lifecycle.Manager to cmd/root.go. Net -826 lines from cmd/ — commands now parse flags, resolve worktree/branch, then call library functions. Regenerated gitops mocks for path-based interface. Updated all cmd tests for new delegation patterns.

```bash
go test -count=1 ./... 2>&1 | tail -20
```

```output
?   	github.com/joescharf/wt	[no test files]
ok  	github.com/joescharf/wt/cmd	3.676s
ok  	github.com/joescharf/wt/internal/mcp	3.237s
?   	github.com/joescharf/wt/internal/ui	[no test files]
ok  	github.com/joescharf/wt/pkg/claude	2.854s
ok  	github.com/joescharf/wt/pkg/gitops	4.708s
?   	github.com/joescharf/wt/pkg/gitops/mocks	[no test files]
ok  	github.com/joescharf/wt/pkg/iterm	3.062s
?   	github.com/joescharf/wt/pkg/iterm/mocks	[no test files]
ok  	github.com/joescharf/wt/pkg/lifecycle	3.471s
ok  	github.com/joescharf/wt/pkg/ops	3.426s
?   	github.com/joescharf/wt/pkg/ops/mocks	[no test files]
ok  	github.com/joescharf/wt/pkg/wtstate	3.305s
```
