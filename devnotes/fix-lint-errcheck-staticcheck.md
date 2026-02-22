# Fix all errcheck and staticcheck lint issues

*2026-02-22T18:42:38Z*

Fixed all golangci-lint errcheck and staticcheck issues across 12 files. Production code changes add blank-identifier assignments (`_, _ =`) to fmt.Fprint/Fprintf/Fprintln/Fscanln calls, RegisterFlagCompletionFunc, and table.Bulk/Render calls. Test code wraps os.MkdirAll and state.SetWorktree calls with require.NoError. Three staticcheck QF1008 fixes remove redundant .Time from FlexTime embedded field selectors.

```bash
make lint 2>&1
```

```output
go vet ./...
golangci-lint run ./...
0 issues.
```

```bash
make test 2>&1 | grep -E '^(ok|FAIL)'
```

```output
ok  	github.com/joescharf/wt/cmd	2.753s
ok  	github.com/joescharf/wt/internal/mcp	1.919s
ok  	github.com/joescharf/wt/pkg/claude	1.225s
ok  	github.com/joescharf/wt/pkg/gitops	3.216s
ok  	github.com/joescharf/wt/pkg/iterm	2.046s
ok  	github.com/joescharf/wt/pkg/lifecycle	2.749s
ok  	github.com/joescharf/wt/pkg/ops	2.605s
ok  	github.com/joescharf/wt/pkg/wtstate	2.428s
```
