# Rebuild pkg/ops with path-based interface

*2026-02-22T16:37:49Z*

Rebuilt pkg/ops as pure business logic extracted from cmd/ CLI handlers. Operations (Sync, SyncAll, Merge, Delete, DeleteAll, Prune, Discover) accept gitops.Client and Logger interfaces with callback functions for safety checks, cleanup, PR creation, and state management — no UI, colors, or prompts. All 36 tests pass with race detector.

```bash
go test -v -race -count=1 ./pkg/ops/ 2>&1 | tail -45
```

```output
--- PASS: TestMerge_LocalMerge (0.00s)
=== RUN   TestMerge_NoCommits
--- PASS: TestMerge_NoCommits (0.00s)
=== RUN   TestMerge_DirtyBlocked
--- PASS: TestMerge_DirtyBlocked (0.00s)
=== RUN   TestMerge_WrongBranch
--- PASS: TestMerge_WrongBranch (0.00s)
=== RUN   TestMerge_RebaseThenFF
--- PASS: TestMerge_RebaseThenFF (0.00s)
=== RUN   TestMerge_PR
--- PASS: TestMerge_PR (0.00s)
=== RUN   TestMerge_PRDryRun
--- PASS: TestMerge_PRDryRun (0.00s)
=== RUN   TestMerge_NoCleanup
--- PASS: TestMerge_NoCleanup (0.00s)
=== RUN   TestMerge_ContinueMerge
--- PASS: TestMerge_ContinueMerge (0.00s)
=== RUN   TestDelete_Basic
--- PASS: TestDelete_Basic (0.00s)
=== RUN   TestDelete_SafetyCheckAbort
--- PASS: TestDelete_SafetyCheckAbort (0.00s)
=== RUN   TestDelete_SafetyCheckPasses
--- PASS: TestDelete_SafetyCheckPasses (0.00s)
=== RUN   TestDeleteAll_NoWorktrees
--- PASS: TestDeleteAll_NoWorktrees (0.00s)
=== RUN   TestPrune_Clean
--- PASS: TestPrune_Clean (0.00s)
=== RUN   TestPrune_WithStaleEntries
--- PASS: TestPrune_WithStaleEntries (0.00s)
=== RUN   TestPrune_NilTrustPruner
--- PASS: TestPrune_NilTrustPruner (0.00s)
=== RUN   TestPrune_DryRun
--- PASS: TestPrune_DryRun (0.00s)
=== RUN   TestDiscover_NoUnmanaged
--- PASS: TestDiscover_NoUnmanaged (0.00s)
=== RUN   TestDiscover_FindsUnmanaged
--- PASS: TestDiscover_FindsUnmanaged (0.00s)
=== RUN   TestDiscover_Adopt
--- PASS: TestDiscover_Adopt (0.00s)
=== RUN   TestDiscover_AdoptDryRun
--- PASS: TestDiscover_AdoptDryRun (0.00s)
=== RUN   TestClassifySource
--- PASS: TestClassifySource (0.00s)
PASS
ok  	github.com/joescharf/wt/pkg/ops	1.153s
```
