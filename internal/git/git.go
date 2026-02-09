package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeInfo holds parsed worktree metadata from `git worktree list --porcelain`.
type WorktreeInfo struct {
	Path   string
	Branch string
	HEAD   string
}

// Client defines the interface for git operations.
// Pure utility functions (BranchToDirname, ResolveWorktreePath) are package-level functions.
type Client interface {
	RepoRoot() (string, error)
	RepoName() (string, error)
	WorktreesDir() (string, error)
	WorktreeList() ([]WorktreeInfo, error)
	WorktreeAdd(path, branch, base string, newBranch bool) error
	WorktreeRemove(path string, force bool) error
	BranchExists(branch string) (bool, error)
	BranchDelete(branch string, force bool) error
	CurrentBranch(worktreePath string) (string, error)
	ResolveWorktree(input string) (string, error)
	BranchList() ([]string, error)
	IsWorktreeDirty(path string) (bool, error)
	HasUnpushedCommits(path, baseBranch string) (bool, error)
	WorktreePrune() error
}

// RealClient implements Client using real git commands.
type RealClient struct{}

// NewClient returns a new RealClient.
func NewClient() *RealClient {
	return &RealClient{}
}

func (c *RealClient) RepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository: %w", err)
	}

	gitCommonDir := strings.TrimSpace(string(out))

	// For a main repo: .git (relative) or /abs/path/.git
	// For a worktree: /abs/path/to/main/.git
	var root string
	if gitCommonDir == ".git" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root = cwd
	} else {
		absGitDir, err := filepath.Abs(gitCommonDir)
		if err != nil {
			return "", err
		}
		root = filepath.Dir(absGitDir)
	}

	// Resolve symlinks for consistent paths (e.g., macOS /var -> /private/var)
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return root, nil
	}
	return resolved, nil
}

func (c *RealClient) RepoName() (string, error) {
	root, err := c.RepoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Base(root), nil
}

func (c *RealClient) WorktreesDir() (string, error) {
	root, err := c.RepoRoot()
	if err != nil {
		return "", err
	}
	return root + ".worktrees", nil
}

func (c *RealClient) WorktreeList() ([]WorktreeInfo, error) {
	root, err := c.RepoRoot()
	if err != nil {
		return nil, err
	}

	out, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	return ParseWorktreeListPorcelain(string(out)), nil
}

// ParseWorktreeListPorcelain parses the output of `git worktree list --porcelain`.
func ParseWorktreeListPorcelain(output string) []WorktreeInfo {
	var worktrees []WorktreeInfo
	var current WorktreeInfo

	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			current.HEAD = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			branch := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(branch, "refs/heads/")
		case line == "":
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = WorktreeInfo{}
			}
		}
	}
	// Handle last entry if no trailing newline
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees
}

func (c *RealClient) WorktreeAdd(path, branch, base string, newBranch bool) error {
	root, err := c.RepoRoot()
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	if newBranch {
		cmd = exec.Command("git", "-C", root, "worktree", "add", "-b", branch, path, base)
	} else {
		cmd = exec.Command("git", "-C", root, "worktree", "add", path, branch)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (c *RealClient) WorktreeRemove(path string, force bool) error {
	root, err := c.RepoRoot()
	if err != nil {
		return err
	}

	args := []string{"-C", root, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)

	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (c *RealClient) BranchExists(branch string) (bool, error) {
	root, err := c.RepoRoot()
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

func (c *RealClient) BranchDelete(branch string, force bool) error {
	root, err := c.RepoRoot()
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

func (c *RealClient) IsWorktreeDirty(path string) (bool, error) {
	out, err := exec.Command("git", "-C", path, "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("failed to check worktree status: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

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

func (c *RealClient) CurrentBranch(worktreePath string) (string, error) {
	out, err := exec.Command("git", "-C", worktreePath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *RealClient) ResolveWorktree(input string) (string, error) {
	wtDir, err := c.WorktreesDir()
	if err != nil {
		return "", err
	}
	return ResolveWorktreePath(input, wtDir)
}

// ResolveWorktreePath resolves a branch name, dirname, or full path to a worktree path.
// This is a pure function for testability.
func ResolveWorktreePath(input, worktreesDir string) (string, error) {
	// Full path
	if filepath.IsAbs(input) {
		return input, nil
	}

	// Try as dirname first
	candidate := filepath.Join(worktreesDir, input)
	if isDir(candidate) {
		return candidate, nil
	}

	// Try converting from branch name
	dirname := BranchToDirname(input)
	candidate = filepath.Join(worktreesDir, dirname)
	return candidate, nil
}

// BranchToDirname converts a branch name to a directory name by extracting the last path segment.
func BranchToDirname(branch string) string {
	parts := strings.Split(branch, "/")
	return parts[len(parts)-1]
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

