package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/hack-dance/stack/internal/store"
)

type Client struct {
	cwd string
}

type RepoDetails struct {
	NameWithOwner    string `json:"nameWithOwner"`
	URL              string `json:"url"`
	DefaultBranchRef struct {
		Name string `json:"name"`
	} `json:"defaultBranchRef"`
}

func NewClient(cwd string) *Client {
	return &Client{cwd: cwd}
}

func (c *Client) RepoView(ctx context.Context) (RepoDetails, error) {
	output, err := c.run(ctx, "repo", "view", "--json", "nameWithOwner,url,defaultBranchRef")
	if err != nil {
		return RepoDetails{}, err
	}

	var details RepoDetails
	if err := json.Unmarshal([]byte(output), &details); err != nil {
		return RepoDetails{}, err
	}

	return details, nil
}

func (c *Client) ViewPR(ctx context.Context, number int) (store.PullRequest, error) {
	output, err := c.run(
		ctx,
		"pr",
		"view",
		fmt.Sprintf("%d", number),
		"--json",
		"id,number,url,baseRefName,baseRefOid,headRefName,headRefOid,state,isDraft",
	)
	if err != nil {
		return store.PullRequest{}, err
	}

	return decodePR(output)
}

func (c *Client) FindPRByHead(ctx context.Context, branch string) (store.PullRequest, error) {
	output, err := c.run(
		ctx,
		"pr",
		"list",
		"--head",
		branch,
		"--state",
		"all",
		"--json",
		"id,number,url,baseRefName,baseRefOid,headRefName,headRefOid,state,isDraft",
	)
	if err != nil {
		return store.PullRequest{}, err
	}

	var prs []store.PullRequest
	if err := json.Unmarshal([]byte(output), &prs); err != nil {
		return store.PullRequest{}, err
	}

	if len(prs) == 0 {
		return store.PullRequest{}, nil
	}

	return prs[0], nil
}

func (c *Client) CreatePR(ctx context.Context, base string, head string, title string, body string, draft bool) (store.PullRequest, error) {
	args := []string{
		"pr",
		"create",
		"--base",
		base,
		"--head",
		head,
		"--title",
		title,
		"--body",
		body,
	}
	if draft {
		args = append(args, "--draft")
	}

	if _, err := c.run(ctx, args...); err != nil {
		return store.PullRequest{}, err
	}

	return c.FindPRByHead(ctx, head)
}

func (c *Client) EditPRBase(ctx context.Context, number int, base string) error {
	_, err := c.run(ctx, "pr", "edit", fmt.Sprintf("%d", number), "--base", base)
	return err
}

func (c *Client) MergePR(ctx context.Context, number int, headOID string) error {
	_, err := c.run(
		ctx,
		"pr",
		"merge",
		fmt.Sprintf("%d", number),
		"--auto",
		"--match-head-commit",
		headOID,
	)
	return err
}

func decodePR(raw string) (store.PullRequest, error) {
	var pr store.PullRequest
	if err := json.Unmarshal([]byte(raw), &pr); err != nil {
		return store.PullRequest{}, err
	}
	return pr, nil
}

func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = c.cwd

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("gh %s: %s", strings.Join(args, " "), message)
	}

	return stdout.String(), nil
}
