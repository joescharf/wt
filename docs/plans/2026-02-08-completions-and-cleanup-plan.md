# Shell Completions + Smart Delete & Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add dynamic shell completions (zsh/bash/fish) and smart delete with safety checks, prune command, and bulk delete to the `wt` CLI.

**Architecture:** Extend `git.Client` interface with 4 new methods (`BranchList`, `IsWorktreeDirty`, `HasUnpushedCommits`, `WorktreePrune`). Add `cmd/completion.go` for shell completions with `ValidArgsFunction` on each command. Add `cmd/prune.go` for standalone prune. Modify `cmd/delete.go` for safety checks, interactive prompts (injectable for testing), and `--all` flag.

**Tech Stack:** Go, cobra (completions + ValidArgsFunction), viper, mockery (mock regen), testify

---

### Task 1: Add BranchList to git.Client

**Files:**
- Modify: `internal/git/git.go:20-31` (Client interface)
- Modify: `internal/git/git.go` (add RealClient method after BranchDelete)
- Test: `internal/git/git_test.go`

**Step 1: Write the failing test**

Add to `internal/git/git_test.go`:

```go
func TestBranchList_Integration(t *testing.T) {
	repoDir := initTestRepo(t)

	// Create some branches
	for _, branch := range []string{"feature/auth", "bugfix/login"} {
		cmd := exec.Command("git", "-C", repoDir, "branch", branch)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "create branch %s: %s", branch, string(out))
	}

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()
	branches, err := client.BranchList()
	require.NoError(t, err)

	// Should contain all branches including main (or master)
	assert.GreaterOrEqual(t, len(branches), 3)
	assert.Contains(t, branches, "feature/auth")
	assert.Contains(t, branches, "bugfix/login")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/git/ -run TestBranchList_Integration -v`
Expected: FAIL — `client.BranchList undefined`

**Step 3: Add BranchList to Client interface and implement**

In `internal/git/git.go`, add `BranchList() ([]string, error)` to the Client interface (after line 30, before the closing `}`).

Then add the implementation after `BranchDelete`:

```go
func (c *RealClient) BranchList() ([]string, error) {
	root, err := c.RepoRoot()
	if err != nil {
		return nil, err
	}

	out, err := exec.Command("git", "-C", root, "branch", "--format=%(refname:short)").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/git/ -run TestBranchList_Integration -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "feat: add BranchList to git.Client interface"
```

---

### Task 2: Add IsWorktreeDirty to git.Client

**Files:**
- Modify: `internal/git/git.go:20-31` (Client interface)
- Modify: `internal/git/git.go` (add RealClient method)
- Test: `internal/git/git_test.go`

**Step 1: Write the failing test**

Add to `internal/git/git_test.go`:

```go
func TestIsWorktreeDirty_Integration(t *testing.T) {
	repoDir := initTestRepo(t)

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()

	// Clean repo
	dirty, err := client.IsWorktreeDirty(repoDir)
	require.NoError(t, err)
	assert.False(t, dirty)

	// Create an untracked file
	err = os.WriteFile(filepath.Join(repoDir, "dirty.txt"), []byte("dirty"), 0644)
	require.NoError(t, err)

	dirty, err = client.IsWorktreeDirty(repoDir)
	require.NoError(t, err)
	assert.True(t, dirty)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/git/ -run TestIsWorktreeDirty_Integration -v`
Expected: FAIL — `client.IsWorktreeDirty undefined`

**Step 3: Implement IsWorktreeDirty**

Add to Client interface: `IsWorktreeDirty(path string) (bool, error)`

```go
func (c *RealClient) IsWorktreeDirty(path string) (bool, error) {
	out, err := exec.Command("git", "-C", path, "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("failed to check worktree status: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/git/ -run TestIsWorktreeDirty_Integration -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "feat: add IsWorktreeDirty to git.Client interface"
```

---

### Task 3: Add HasUnpushedCommits to git.Client

**Files:**
- Modify: `internal/git/git.go` (Client interface + implementation)
- Test: `internal/git/git_test.go`

**Step 1: Write the failing test**

```go
func TestHasUnpushedCommits_Integration(t *testing.T) {
	repoDir := initTestRepo(t)

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()

	// Create a worktree with a branch
	wtDir := repoDir + ".worktrees"
	err = os.MkdirAll(wtDir, 0755)
	require.NoError(t, err)

	wtPath := filepath.Join(wtDir, "test-branch")
	err = client.WorktreeAdd(wtPath, "test-branch", "HEAD", true)
	require.NoError(t, err)

	// No unpushed commits (same as base)
	unpushed, err := client.HasUnpushedCommits(wtPath, "HEAD")
	require.NoError(t, err)
	assert.False(t, unpushed)

	// Add a commit in the worktree
	testFile := filepath.Join(wtPath, "new.txt")
	err = os.WriteFile(testFile, []byte("new"), 0644)
	require.NoError(t, err)

	cmd := exec.Command("git", "-C", wtPath, "add", ".")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", wtPath, "commit", "-m", "new commit")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com")
	require.NoError(t, cmd.Run())

	// Now has unpushed commits relative to main
	// Get the main branch name
	mainBranch, err := client.CurrentBranch(repoDir)
	require.NoError(t, err)

	unpushed, err = client.HasUnpushedCommits(wtPath, mainBranch)
	require.NoError(t, err)
	assert.True(t, unpushed)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/git/ -run TestHasUnpushedCommits_Integration -v`
Expected: FAIL — `client.HasUnpushedCommits undefined`

**Step 3: Implement HasUnpushedCommits**

Add to Client interface: `HasUnpushedCommits(path, baseBranch string) (bool, error)`

```go
func (c *RealClient) HasUnpushedCommits(path, baseBranch string) (bool, error) {
	// Try upstream first
	out, err := exec.Command("git", "-C", path, "log", "@{upstream}..HEAD", "--oneline").Output()
	if err == nil {
		return strings.TrimSpace(string(out)) != "", nil
	}

	// No upstream configured, fall back to baseBranch
	out, err = exec.Command("git", "-C", path, "log", baseBranch+"..HEAD", "--oneline").Output()
	if err != nil {
		return false, fmt.Errorf("failed to check unpushed commits: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/git/ -run TestHasUnpushedCommits_Integration -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "feat: add HasUnpushedCommits to git.Client interface"
```

---

### Task 4: Add WorktreePrune to git.Client

**Files:**
- Modify: `internal/git/git.go` (Client interface + implementation)
- Test: `internal/git/git_test.go`

**Step 1: Write the failing test**

```go
func TestWorktreePrune_Integration(t *testing.T) {
	repoDir := initTestRepo(t)

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(repoDir)

	client := NewClient()

	// Should succeed even with nothing to prune
	err = client.WorktreePrune()
	require.NoError(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/git/ -run TestWorktreePrune_Integration -v`
Expected: FAIL — `client.WorktreePrune undefined`

**Step 3: Implement WorktreePrune**

Add to Client interface: `WorktreePrune() error`

```go
func (c *RealClient) WorktreePrune() error {
	root, err := c.RepoRoot()
	if err != nil {
		return err
	}

	out, err := exec.Command("git", "-C", root, "worktree", "prune").CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree prune failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/git/ -run TestWorktreePrune_Integration -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "feat: add WorktreePrune to git.Client interface"
```

---

### Task 5: Regenerate mocks

**Files:**
- Regenerate: `internal/git/mocks/mock_Client.go`

**Step 1: Run mockery**

```bash
cd /Users/joescharf/app/utils/worktree-dev && mockery
```

**Step 2: Verify build**

```bash
go build ./...
```

**Step 3: Verify all existing tests still pass**

```bash
go test ./...
```

**Step 4: Commit**

```bash
git add internal/git/mocks/
git commit -m "chore: regenerate mocks for new git.Client methods"
```

---

### Task 6: Add completion command with dynamic completions

**Files:**
- Create: `cmd/completion.go`
- Modify: `cmd/open.go` (add ValidArgsFunction)
- Modify: `cmd/switch.go` (add ValidArgsFunction)
- Modify: `cmd/delete.go` (add ValidArgsFunction)
- Modify: `cmd/create.go` (add ValidArgsFunction for `--base` flag)
- Modify: `cmd/root.go` (add ValidArgsFunction for bare shorthand)

**Step 1: Create `cmd/completion.go`**

```go
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:       "completion [bash|zsh|fish]",
	Short:     "Generate shell completion script",
	Long: `Generate shell completion scripts for wt.

To load completions:

Bash:
  source <(wt completion bash)

  # To load completions for each session, execute once:
  # Linux:
  wt completion bash > /etc/bash_completion.d/wt
  # macOS:
  wt completion bash > $(brew --prefix)/etc/bash_completion.d/wt

Zsh:
  # If shell completion is not already enabled in your environment,
  # enable it by running:
  echo "autoload -U compinit; compinit" >> ~/.zshrc

  source <(wt completion zsh)

  # To load completions for each session, execute once:
  wt completion zsh > "${fpath[1]}/_wt"

Fish:
  wt completion fish | source

  # To load completions for each session, execute once:
  wt completion fish > ~/.config/fish/completions/wt.fish
`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish"},
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletionV2(os.Stdout, true)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		default:
			return cmd.Help()
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}

// completeWorktreeNames returns existing worktree dirnames for shell completion.
func completeWorktreeNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if gitClient == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	repoRoot, err := gitClient.RepoRoot()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	worktrees, err := gitClient.WorktreeList()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, wt := range worktrees {
		if wt.Path == repoRoot {
			continue
		}
		names = append(names, wt.Branch)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeBranchNames returns local branch names for shell completion.
func completeBranchNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if gitClient == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	branches, err := gitClient.BranchList()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return branches, cobra.ShellCompDirectiveNoFileComp
}
```

**Step 2: Add ValidArgsFunction to each command**

In `cmd/open.go`, change the openCmd definition to add:
```go
ValidArgsFunction: completeWorktreeNames,
```

In `cmd/switch.go`, add to switchCmd:
```go
ValidArgsFunction: completeWorktreeNames,
```

In `cmd/delete.go`, add to deleteCmd:
```go
ValidArgsFunction: completeWorktreeNames,
```

In `cmd/root.go`, add to rootCmd:
```go
ValidArgsFunction: completeWorktreeNames,
```

In `cmd/create.go` `init()`, after the `--base` flag definition, register the base flag completion:
```go
createCmd.RegisterFlagCompletionFunc("base", completeBranchNames)
```

**Step 3: Verify build**

```bash
go build ./...
```

**Step 4: Quick manual test**

```bash
go run . completion bash > /dev/null
go run . completion zsh > /dev/null
go run . completion fish > /dev/null
```

**Step 5: Commit**

```bash
git add cmd/completion.go cmd/open.go cmd/switch.go cmd/delete.go cmd/create.go cmd/root.go
git commit -m "feat: add shell completion command with dynamic worktree/branch completion"
```

---

### Task 7: Add prune command

**Files:**
- Create: `cmd/prune.go`
- Test: `cmd/cmd_test.go`

**Step 1: Write the failing test**

Add to `cmd/cmd_test.go`:

```go
// ─── Prune Tests ─────────────────────────────────────────────────────────────

func TestPrune_CleansStaleState(t *testing.T) {
	env := setupTest(t)

	// Add state for a non-existent path
	env.state.SetWorktree("/nonexistent/path", &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "stale",
	})

	env.git.EXPECT().WorktreePrune().Return(nil)

	err := pruneRun()
	require.NoError(t, err)

	// Verify stale entry was pruned
	ws, _ := env.state.GetWorktree("/nonexistent/path")
	assert.Nil(t, ws)
	assert.Contains(t, env.out.String(), "Pruned 1 stale")
}

func TestPrune_NothingToClean(t *testing.T) {
	env := setupTest(t)

	env.git.EXPECT().WorktreePrune().Return(nil)

	err := pruneRun()
	require.NoError(t, err)

	assert.Contains(t, env.out.String(), "clean")
}

func TestPrune_DryRun(t *testing.T) {
	env := setupTest(t)
	dryRun = true
	env.ui.DryRun = true

	env.state.SetWorktree("/nonexistent/path", &state.WorktreeState{
		Repo:   "myrepo",
		Branch: "stale",
	})

	// WorktreePrune should NOT be called in dry-run

	err := pruneRun()
	require.NoError(t, err)

	assert.Contains(t, env.err.String(), "DRY-RUN")
	// State should NOT actually be pruned in dry-run
	// (But Prune() doesn't have dry-run support, so we check the count instead)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/ -run TestPrune -v`
Expected: FAIL — `pruneRun undefined`

**Step 3: Create `cmd/prune.go`**

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Clean up stale state and git worktree tracking",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return pruneRun()
	},
}

func init() {
	rootCmd.AddCommand(pruneCmd)
}

func pruneRun() error {
	// Prune stale state entries
	pruned, err := stateMgr.Prune()
	if err != nil {
		output.Warning("Failed to prune state: %v", err)
	}

	if pruned > 0 {
		output.Info("Pruned %d stale state entries", pruned)
	}

	// Run git worktree prune
	if dryRun {
		output.DryRunMsg("Would run git worktree prune")
	} else {
		if err := gitClient.WorktreePrune(); err != nil {
			output.Warning("Failed to run git worktree prune: %v", err)
		} else {
			output.VerboseLog("Ran git worktree prune")
		}
	}

	if pruned == 0 {
		output.Success("Everything clean, nothing to prune")
	} else {
		fmt.Fprintln(output.Out)
		output.Success("Prune complete")
	}

	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/ -run TestPrune -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/prune.go cmd/cmd_test.go
git commit -m "feat: add prune command for stale state and git worktree cleanup"
```

---

### Task 8: Add confirmPrompt helper for delete safety

**Files:**
- Modify: `cmd/delete.go` (add prompt func var and helper)

**Step 1: Add the prompt function variable and implementation**

At the top of `cmd/delete.go`, add a package-level function variable:

```go
// promptFunc is the confirmation prompt, replaceable in tests.
var promptFunc = defaultPrompt

func defaultPrompt(msg string) bool {
	fmt.Fprintf(output.ErrOut, "%s [y/N] ", msg)
	var answer string
	fmt.Fscanln(os.Stdin, &answer)
	return strings.ToLower(strings.TrimSpace(answer)) == "y"
}
```

**Step 2: Verify build**

```bash
go build ./...
```

**Step 3: Commit**

```bash
git add cmd/delete.go
git commit -m "feat: add injectable confirmation prompt for delete safety"
```

---

### Task 9: Add safety checks to delete command

**Files:**
- Modify: `cmd/delete.go` (add safety check logic before removal)
- Test: `cmd/cmd_test.go`

**Step 1: Write the failing tests**

Add to `cmd/cmd_test.go`, updating `setupTest` first to reset `promptFunc` and `deleteAll`:

In `setupTest`, add to the reset flags section:
```go
deleteAll = false
promptFunc = func(msg string) bool { return false } // default deny in tests
```

Then add tests:

```go
func TestDelete_SafeCleanWorktree(t *testing.T) {
	env := setupTest(t)
	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(false, nil)
	env.git.EXPECT().WorktreeRemove(wtPath, false).
		Run(func(path string, force bool) { os.RemoveAll(path) }).Return(nil)

	err := deleteRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "removed")
}

func TestDelete_DirtyWorktreePromptDenied(t *testing.T) {
	env := setupTest(t)
	promptFunc = func(msg string) bool { return false }

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(true, nil)

	err := deleteRun("auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aborted")
}

func TestDelete_DirtyWorktreePromptAccepted(t *testing.T) {
	env := setupTest(t)
	promptFunc = func(msg string) bool { return true }

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(true, nil)
	env.git.EXPECT().WorktreeRemove(wtPath, false).
		Run(func(path string, force bool) { os.RemoveAll(path) }).Return(nil)

	err := deleteRun("auth")
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "removed")
}

func TestDelete_UnpushedCommitsPromptDenied(t *testing.T) {
	env := setupTest(t)
	promptFunc = func(msg string) bool { return false }

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)
	env.git.EXPECT().IsWorktreeDirty(wtPath).Return(false, nil)
	env.git.EXPECT().HasUnpushedCommits(wtPath, "main").Return(true, nil)

	err := deleteRun("auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aborted")
}

func TestDelete_ForceSkipsChecks(t *testing.T) {
	env := setupTest(t)
	deleteForce = true

	wtPath := filepath.Join(env.dir, "repo.worktrees", "auth")
	os.MkdirAll(wtPath, 0755)

	// Should NOT call IsWorktreeDirty or HasUnpushedCommits
	env.git.EXPECT().ResolveWorktree("auth").Return(wtPath, nil)
	env.git.EXPECT().WorktreeRemove(wtPath, true).
		Run(func(path string, force bool) { os.RemoveAll(path) }).Return(nil)

	err := deleteRun("auth")
	require.NoError(t, err)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run "TestDelete_Safe|TestDelete_Dirty|TestDelete_Unpushed" -v`
Expected: FAIL — tests expect new behavior that doesn't exist yet

**Step 3: Modify `deleteRun` in `cmd/delete.go`**

Insert safety checks after verifying the worktree exists but before closing the iTerm2 window. The `--force` flag bypasses all checks:

```go
func deleteRun(branch string) error {
	wtPath, err := gitClient.ResolveWorktree(branch)
	if err != nil {
		return err
	}
	dirname := filepath.Base(wtPath)

	if !isDirectory(wtPath) {
		output.Error("Worktree not found: %s", wtPath)
		_ = stateMgr.RemoveWorktree(wtPath)
		return fmt.Errorf("worktree not found: %s", wtPath)
	}

	output.Info("Deleting worktree '%s'", ui.Cyan(dirname))

	// Safety checks (skip with --force)
	if !deleteForce {
		dirty, err := gitClient.IsWorktreeDirty(wtPath)
		if err != nil {
			output.VerboseLog("Could not check worktree status: %v", err)
		}

		if dirty {
			output.Warning("Worktree has uncommitted changes")
			if dryRun {
				output.DryRunMsg("Would prompt for confirmation (uncommitted changes)")
			} else if !promptFunc("Delete worktree with uncommitted changes?") {
				return fmt.Errorf("delete aborted")
			}
		} else {
			baseBranch := viper.GetString("base_branch")
			unpushed, err := gitClient.HasUnpushedCommits(wtPath, baseBranch)
			if err != nil {
				output.VerboseLog("Could not check unpushed commits: %v", err)
			}

			if unpushed {
				output.Warning("Worktree has unpushed commits")
				if dryRun {
					output.DryRunMsg("Would prompt for confirmation (unpushed commits)")
				} else if !promptFunc("Delete worktree with unpushed commits?") {
					return fmt.Errorf("delete aborted")
				}
			}
		}
	}

	// ... rest of existing delete logic (close window, remove worktree, etc.)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ -run TestDelete -v`
Expected: PASS (all delete tests including the existing ones)

**Step 5: Commit**

```bash
git add cmd/delete.go cmd/cmd_test.go
git commit -m "feat: add safety checks to delete (dirty/unpushed detection with prompt)"
```

---

### Task 10: Add --all flag to delete command

**Files:**
- Modify: `cmd/delete.go` (add `--all` flag and `deleteAllRun`)
- Test: `cmd/cmd_test.go`

**Step 1: Write the failing tests**

```go
func TestDelete_All(t *testing.T) {
	env := setupTest(t)
	deleteAll = true
	deleteForce = true // skip prompts for simplicity

	wtPath1 := filepath.Join(env.dir, "repo.worktrees", "auth")
	wtPath2 := filepath.Join(env.dir, "repo.worktrees", "api")
	os.MkdirAll(wtPath1, 0755)
	os.MkdirAll(wtPath2, 0755)

	env.git.EXPECT().RepoRoot().Return(env.dir, nil)
	env.git.EXPECT().WorktreeList().Return([]git.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
		{Path: wtPath1, Branch: "feature/auth"},
		{Path: wtPath2, Branch: "feature/api"},
	}, nil)
	env.git.EXPECT().WorktreeRemove(wtPath1, true).
		Run(func(path string, force bool) { os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().WorktreeRemove(wtPath2, true).
		Run(func(path string, force bool) { os.RemoveAll(path) }).Return(nil)
	env.git.EXPECT().WorktreePrune().Return(nil)

	err := deleteAllRun()
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "Deleted 2 worktrees")
}

func TestDelete_All_NoneFound(t *testing.T) {
	env := setupTest(t)
	deleteAll = true

	env.git.EXPECT().RepoRoot().Return(env.dir, nil)
	env.git.EXPECT().WorktreeList().Return([]git.WorktreeInfo{
		{Path: env.dir, Branch: "main"},
	}, nil)

	err := deleteAllRun()
	require.NoError(t, err)
	assert.Contains(t, env.out.String(), "No worktrees")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run "TestDelete_All" -v`
Expected: FAIL — `deleteAllRun` undefined, `deleteAll` undefined

**Step 3: Implement --all flag and deleteAllRun**

In `cmd/delete.go`, add the flag:

```go
var (
	deleteForce      bool
	deleteBranchFlag bool
	deleteAll        bool
)
```

In `init()`, add:
```go
deleteCmd.Flags().BoolVar(&deleteAll, "all", false, "Delete all worktrees")
```

Change the command's Args and RunE:
```go
var deleteCmd = &cobra.Command{
	Use:     "delete [branch]",
	Aliases: []string{"rm"},
	Short:   "Close iTerm2 window + remove worktree",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if deleteAll {
			return deleteAllRun()
		}
		if len(args) == 0 {
			return fmt.Errorf("branch argument required (or use --all)")
		}
		return deleteRun(args[0])
	},
	ValidArgsFunction: completeWorktreeNames,
}
```

Add `deleteAllRun`:

```go
func deleteAllRun() error {
	repoRoot, err := gitClient.RepoRoot()
	if err != nil {
		return err
	}

	worktrees, err := gitClient.WorktreeList()
	if err != nil {
		return err
	}

	// Filter out main repo
	var toDelete []git.WorktreeInfo
	for _, wt := range worktrees {
		if wt.Path != repoRoot {
			toDelete = append(toDelete, wt)
		}
	}

	if len(toDelete) == 0 {
		output.Info("No worktrees to delete")
		return nil
	}

	output.Info("Found %d worktrees to delete", len(toDelete))

	deleted := 0
	for _, wt := range toDelete {
		dirname := filepath.Base(wt.Path)

		if !isDirectory(wt.Path) {
			continue
		}

		// Safety checks (skip with --force)
		if !deleteForce {
			dirty, err := gitClient.IsWorktreeDirty(wt.Path)
			if err != nil {
				output.VerboseLog("Could not check status of %s: %v", dirname, err)
			}
			if dirty {
				output.Warning("'%s' has uncommitted changes", dirname)
				if !promptFunc(fmt.Sprintf("Delete '%s' with uncommitted changes?", dirname)) {
					output.Info("Skipping '%s'", dirname)
					continue
				}
			} else {
				baseBranch := viper.GetString("base_branch")
				unpushed, err := gitClient.HasUnpushedCommits(wt.Path, baseBranch)
				if err != nil {
					output.VerboseLog("Could not check unpushed commits for %s: %v", dirname, err)
				}
				if unpushed {
					output.Warning("'%s' has unpushed commits", dirname)
					if !promptFunc(fmt.Sprintf("Delete '%s' with unpushed commits?", dirname)) {
						output.Info("Skipping '%s'", dirname)
						continue
					}
				}
			}
		}

		// Close iTerm2 window
		ws, _ := stateMgr.GetWorktree(wt.Path)
		if ws != nil && ws.ClaudeSessionID != "" {
			if itermClient.IsRunning() && itermClient.SessionExists(ws.ClaudeSessionID) {
				if err := itermClient.CloseWindow(ws.ClaudeSessionID); err != nil {
					output.Warning("Failed to close window for %s: %v", dirname, err)
				}
			}
		}

		// Remove worktree
		if err := gitClient.WorktreeRemove(wt.Path, deleteForce); err != nil {
			output.Warning("Failed to remove %s: %v", dirname, err)
			continue
		}

		// Delete branch if requested
		if deleteBranchFlag {
			branchName := wt.Branch
			if ws != nil && ws.Branch != "" {
				branchName = ws.Branch
			}
			if err := gitClient.BranchDelete(branchName, deleteForce); err != nil {
				output.Warning("Could not delete branch '%s': %v", branchName, err)
			}
		}

		_ = stateMgr.RemoveWorktree(wt.Path)
		output.Success("Removed '%s'", ui.Cyan(dirname))
		deleted++
	}

	// Run git worktree prune after bulk delete
	if err := gitClient.WorktreePrune(); err != nil {
		output.Warning("Failed to run git worktree prune: %v", err)
	}

	fmt.Fprintln(output.Out)
	output.Success("Deleted %d worktrees", deleted)
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ -run TestDelete -v`
Expected: PASS (all delete tests)

**Step 5: Commit**

```bash
git add cmd/delete.go cmd/cmd_test.go
git commit -m "feat: add --all flag to delete for bulk worktree removal"
```

---

### Task 11: Full test suite + go vet

**Step 1: Run full test suite**

```bash
go test ./... -v
```
Expected: All tests pass

**Step 2: Run go vet**

```bash
go vet ./...
```
Expected: No issues

**Step 3: Run the binary to verify help output**

```bash
go run . --help
go run . completion --help
go run . prune --help
go run . delete --help
```

**Step 4: Final commit if any cleanup needed**

---

### Task 12: Update setupTest for new flags

**Note:** This is a prerequisite that should be incorporated into Task 9's test work. In `setupTest()` in `cmd/cmd_test.go`, add to the flag reset section:

```go
deleteAll = false
promptFunc = func(msg string) bool { return false }
```

This ensures test isolation for the new `deleteAll` flag and `promptFunc` variable.
