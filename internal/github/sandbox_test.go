package github_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	stackgh "github.com/hack-dance/stack/internal/github"
)

func TestSandboxRepoView(t *testing.T) {
	if os.Getenv("STACK_RUN_GITHUB_SANDBOX") != "1" {
		t.Skip("set STACK_RUN_GITHUB_SANDBOX=1 to run against the real hack-dance/stack sandbox")
	}

	client := stackgh.NewClient(sandboxRepoRoot(t))
	repo, err := client.RepoView(context.Background())
	if err != nil {
		t.Fatalf("repo view: %v", err)
	}

	if repo.NameWithOwner != "hack-dance/stack" {
		t.Fatalf("expected hack-dance/stack, got %q", repo.NameWithOwner)
	}
	if repo.DefaultBranchRef.Name == "" {
		t.Fatalf("expected default branch name")
	}
}

func TestSandboxViewExistingPR(t *testing.T) {
	numberText := os.Getenv("STACK_GITHUB_SANDBOX_PR_NUMBER")
	if numberText == "" {
		t.Skip("set STACK_GITHUB_SANDBOX_PR_NUMBER to a real PR number for non-destructive GitHub sandbox verification")
	}

	number, err := strconv.Atoi(numberText)
	if err != nil {
		t.Fatalf("invalid STACK_GITHUB_SANDBOX_PR_NUMBER: %v", err)
	}

	client := stackgh.NewClient(sandboxRepoRoot(t))
	pr, err := client.ViewPR(context.Background(), number)
	if err != nil {
		t.Fatalf("view pr: %v", err)
	}

	if pr.Number != number {
		t.Fatalf("expected PR number %d, got %d", number, pr.Number)
	}
	if pr.HeadRefName == "" {
		t.Fatalf("expected PR head ref name")
	}
	if pr.BaseRefName == "" {
		t.Fatalf("expected PR base ref name")
	}
}

func sandboxRepoRoot(t *testing.T) string {
	t.Helper()

	if root := os.Getenv("STACK_GITHUB_SANDBOX_REPO_ROOT"); root != "" {
		return root
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	return filepath.Clean(filepath.Join(wd, "..", ".."))
}
