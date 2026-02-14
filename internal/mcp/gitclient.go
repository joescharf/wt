package mcp

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/joescharf/wt/internal/git"
)

// GitClient defines git operations where every method takes an explicit repoPath.
// This allows the MCP server (which runs via stdio with no CWD context) to operate
// on any repository by path.
type GitClient interface {
	RepoRoot(repoPath string) (string, error)
	RepoName(repoPath string) (string, error)
	WorktreesDir(repoPath string) (string, error)
	WorktreeList(repoPath string) ([]git.WorktreeInfo, error)
	WorktreeAdd(repoPath, wtPath, branch, base string, newBranch bool) error
	WorktreeRemove(repoPath, wtPath string, force bool) error
	BranchExists(repoPath, branch string) (bool, error)
	BranchDelete(repoPath, branch string, force bool) error
	CurrentBranch(worktreePath string) (string, error)
	IsWorktreeDirty(worktreePath string) (bool, error)
	HasUnpushedCommits(worktreePath, baseBranch string) (bool, error)
	HasRemote(repoPath string) (bool, error)
	Fetch(repoPath string) error
	Merge(repoPath, branch string) error
	Rebase(repoPath, branch string) error
	Push(worktreePath, branch string, setUpstream bool) error
	Pull(repoPath string) error
	CommitsAhead(worktreePath, baseBranch string) (int, error)
	CommitsBehind(worktreePath, baseBranch string) (int, error)
	WorktreePrune(repoPath string) error
}

// RealGitClient implements GitClient using real git commands with -C <path>.
type RealGitClient struct{}

// NewGitClient returns a new RealGitClient.
func NewGitClient() *RealGitClient {
	return &RealGitClient{}
}

func (c *RealGitClient) RepoRoot(repoPath string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository: %w", err)
	}

	gitCommonDir := strings.TrimSpace(string(out))

	var root string
	if gitCommonDir == ".git" {
		absPath, err := filepath.Abs(repoPath)
		if err != nil {
			return "", err
		}
		root = absPath
	} else {
		absGitDir, err := filepath.Abs(gitCommonDir)
		if err != nil {
			return "", err
		}
		root = filepath.Dir(absGitDir)
	}

	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return root, nil
	}
	return resolved, nil
}

func (c *RealGitClient) RepoName(repoPath string) (string, error) {
	root, err := c.RepoRoot(repoPath)
	if err != nil {
		return "", err
	}
	return filepath.Base(root), nil
}

func (c *RealGitClient) WorktreesDir(repoPath string) (string, error) {
	root, err := c.RepoRoot(repoPath)
	if err != nil {
		return "", err
	}
	return root + ".worktrees", nil
}

func (c *RealGitClient) WorktreeList(repoPath string) ([]git.WorktreeInfo, error) {
	root, err := c.RepoRoot(repoPath)
	if err != nil {
		return nil, err
	}

	out, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	return git.ParseWorktreeListPorcelain(string(out)), nil
}

func (c *RealGitClient) WorktreeAdd(repoPath, wtPath, branch, base string, newBranch bool) error {
	root, err := c.RepoRoot(repoPath)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	if newBranch {
		cmd = exec.Command("git", "-C", root, "worktree", "add", "-b", branch, wtPath, base)
	} else {
		cmd = exec.Command("git", "-C", root, "worktree", "add", wtPath, branch)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (c *RealGitClient) WorktreeRemove(repoPath, wtPath string, force bool) error {
	root, err := c.RepoRoot(repoPath)
	if err != nil {
		return err
	}

	args := []string{"-C", root, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, wtPath)

	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (c *RealGitClient) BranchExists(repoPath, branch string) (bool, error) {
	root, err := c.RepoRoot(repoPath)
	if err != nil {
		return false, err
	}

	err = exec.Command("git", "-C", root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch).Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *RealGitClient) BranchDelete(repoPath, branch string, force bool) error {
	root, err := c.RepoRoot(repoPath)
	if err != nil {
		return err
	}

	flag := "-d"
	if force {
		flag = "-D"
	}

	out, err := exec.Command("git", "-C", root, "branch", flag, branch).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git branch delete failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (c *RealGitClient) CurrentBranch(worktreePath string) (string, error) {
	out, err := exec.Command("git", "-C", worktreePath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *RealGitClient) IsWorktreeDirty(worktreePath string) (bool, error) {
	out, err := exec.Command("git", "-C", worktreePath, "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("failed to check worktree status: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (c *RealGitClient) HasUnpushedCommits(worktreePath, baseBranch string) (bool, error) {
	out, err := exec.Command("git", "-C", worktreePath, "log", "@{upstream}..HEAD", "--oneline").Output()
	if err == nil {
		return strings.TrimSpace(string(out)) != "", nil
	}

	out, err = exec.Command("git", "-C", worktreePath, "log", baseBranch+"..HEAD", "--oneline").Output()
	if err != nil {
		return false, fmt.Errorf("failed to check unpushed commits: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (c *RealGitClient) HasRemote(repoPath string) (bool, error) {
	root, err := c.RepoRoot(repoPath)
	if err != nil {
		return false, err
	}
	out, err := exec.Command("git", "-C", root, "remote").Output()
	if err != nil {
		return false, fmt.Errorf("failed to check remotes: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (c *RealGitClient) Fetch(repoPath string) error {
	out, err := exec.Command("git", "-C", repoPath, "fetch").CombinedOutput()
	if err != nil {
		return fmt.Errorf("git fetch failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (c *RealGitClient) Merge(repoPath, branch string) error {
	out, err := exec.Command("git", "-C", repoPath, "merge", branch, "--no-edit").CombinedOutput()
	if err != nil {
		return fmt.Errorf("git merge failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (c *RealGitClient) Rebase(repoPath, branch string) error {
	out, err := exec.Command("git", "-C", repoPath, "rebase", branch).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git rebase failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (c *RealGitClient) Push(worktreePath, branch string, setUpstream bool) error {
	args := []string{"-C", worktreePath, "push"}
	if setUpstream {
		args = append(args, "-u", "origin", branch)
	}
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (c *RealGitClient) Pull(repoPath string) error {
	out, err := exec.Command("git", "-C", repoPath, "pull").CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (c *RealGitClient) CommitsAhead(worktreePath, baseBranch string) (int, error) {
	out, err := exec.Command("git", "-C", worktreePath, "rev-list", "--count", baseBranch+"..HEAD").Output()
	if err != nil {
		return 0, fmt.Errorf("failed to count commits ahead: %w", err)
	}
	var count int
	_, err = fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &count)
	if err != nil {
		return 0, fmt.Errorf("failed to parse commit count: %w", err)
	}
	return count, nil
}

func (c *RealGitClient) CommitsBehind(worktreePath, baseBranch string) (int, error) {
	out, err := exec.Command("git", "-C", worktreePath, "rev-list", "--count", "HEAD.."+baseBranch).Output()
	if err != nil {
		return 0, fmt.Errorf("failed to count commits behind: %w", err)
	}
	var count int
	_, err = fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &count)
	if err != nil {
		return 0, fmt.Errorf("failed to parse commit count: %w", err)
	}
	return count, nil
}

func (c *RealGitClient) WorktreePrune(repoPath string) error {
	root, err := c.RepoRoot(repoPath)
	if err != nil {
		return err
	}

	out, err := exec.Command("git", "-C", root, "worktree", "prune").CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree prune failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
