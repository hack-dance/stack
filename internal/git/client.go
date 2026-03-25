package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Client struct {
	cwd string
}

type RepoPaths struct {
	Root      string
	GitDir    string
	CommonDir string
}

func NewClient(cwd string) *Client {
	return &Client{cwd: cwd}
}

func (c *Client) RepoPaths(ctx context.Context) (RepoPaths, error) {
	root, err := c.output(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return RepoPaths{}, err
	}

	gitDirRaw, err := c.output(ctx, "rev-parse", "--git-dir")
	if err != nil {
		return RepoPaths{}, err
	}

	commonDirRaw, err := c.output(ctx, "rev-parse", "--git-common-dir")
	if err != nil {
		return RepoPaths{}, err
	}

	root = strings.TrimSpace(root)
	gitDir := strings.TrimSpace(gitDirRaw)
	commonDir := strings.TrimSpace(commonDirRaw)

	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(root, gitDir)
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(root, commonDir)
	}

	return RepoPaths{
		Root:      root,
		GitDir:    gitDir,
		CommonDir: commonDir,
	}, nil
}

func (c *Client) CurrentBranch(ctx context.Context) (string, error) {
	return c.output(ctx, "branch", "--show-current")
}

func (c *Client) Switch(ctx context.Context, branch string) error {
	_, err := c.run(ctx, "switch", branch)
	return err
}

func (c *Client) ResolveRef(ctx context.Context, ref string) (string, error) {
	return c.output(ctx, "rev-parse", ref)
}

func (c *Client) BranchExists(ctx context.Context, branch string) bool {
	_, err := c.output(ctx, "rev-parse", "--verify", branch)
	return err == nil
}

func (c *Client) RemoteBranchExists(ctx context.Context, remote string, branch string) bool {
	_, exists, err := c.RemoteBranchOID(ctx, remote, branch)
	return err == nil && exists
}

func (c *Client) RemoteBranchOID(ctx context.Context, remote string, branch string) (string, bool, error) {
	ref := fmt.Sprintf("refs/remotes/%s/%s", remote, branch)
	oid, err := c.output(ctx, "rev-parse", ref)
	if err == nil {
		return oid, true, nil
	}

	if strings.Contains(err.Error(), "unknown revision") || strings.Contains(err.Error(), "Needed a single revision") {
		return "", false, nil
	}

	return "", false, err
}

func (c *Client) IsAncestor(ctx context.Context, ancestor string, descendant string) (bool, error) {
	_, err := c.run(ctx, "merge-base", "--is-ancestor", ancestor, descendant)
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

func (c *Client) SwitchCreate(ctx context.Context, branch string) error {
	_, err := c.run(ctx, "switch", "-c", branch)
	return err
}

func (c *Client) FetchPrune(ctx context.Context, remote string) error {
	_, err := c.run(ctx, "fetch", "--prune", remote)
	return err
}

func (c *Client) PushBranch(ctx context.Context, remote string, branch string, expectedRemoteOID string) error {
	args := []string{"push"}
	if expectedRemoteOID != "" {
		args = append(args, fmt.Sprintf("--force-with-lease=refs/heads/%s:%s", branch, expectedRemoteOID))
	} else {
		args = append(args, "--set-upstream")
	}
	args = append(args, remote, fmt.Sprintf("%s:refs/heads/%s", branch, branch))

	_, err := c.run(ctx, args...)
	return err
}

func (c *Client) RebaseOnto(ctx context.Context, parent string, oldParentHead string, branch string) error {
	_, err := c.run(ctx, "rebase", "--onto", parent, oldParentHead, branch)
	return err
}

func (c *Client) RebaseContinue(ctx context.Context) error {
	_, err := c.run(ctx, "rebase", "--continue")
	return err
}

func (c *Client) RebaseAbort(ctx context.Context) error {
	_, err := c.run(ctx, "rebase", "--abort")
	return err
}

func (c *Client) RebaseInProgress(ctx context.Context) (bool, error) {
	paths, err := c.RepoPaths(ctx)
	if err != nil {
		return false, err
	}

	for _, candidate := range []string{
		filepath.Join(paths.GitDir, "rebase-merge"),
		filepath.Join(paths.GitDir, "rebase-apply"),
	} {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return true, nil
		}
	}

	return false, nil
}

func (c *Client) CommitMessage(ctx context.Context, ref string) (string, string, error) {
	title, err := c.output(ctx, "log", "-1", "--format=%s", ref)
	if err != nil {
		return "", "", err
	}

	body, err := c.output(ctx, "log", "-1", "--format=%b", ref)
	if err != nil {
		return "", "", err
	}

	return strings.TrimSpace(title), strings.TrimSpace(body), nil
}

func (c *Client) RemoteURL(ctx context.Context, remote string) (string, error) {
	return c.output(ctx, "remote", "get-url", remote)
}

func (c *Client) RangeDiff(ctx context.Context, oldBase string, newBranch string) (string, error) {
	return c.output(ctx, "range-diff", fmt.Sprintf("%s...%s", oldBase, newBranch))
}

func (c *Client) output(ctx context.Context, args ...string) (string, error) {
	stdout, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = c.cwd

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}

	return stdout.String(), nil
}
