package stack_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	stackgit "github.com/hack-dance/stack/internal/git"
	"github.com/hack-dance/stack/internal/stack"
	"github.com/hack-dance/stack/internal/store"
)

func TestBuildSummaryDetectsMissingAnchor(t *testing.T) {
	t.Parallel()

	repo := setupRepo(t)
	client := stackgit.NewClient(repo)
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches: map[string]store.BranchRecord{
			"feature/a": {
				ParentBranch: "main",
			},
		},
	}

	run(t, repo, "git", "switch", "-c", "feature/a")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run(t, repo, "git", "add", "a.txt")
	run(t, repo, "git", "commit", "-m", "add a")

	summary, err := stack.BuildSummary(context.Background(), client, state)
	if err != nil {
		t.Fatalf("build summary: %v", err)
	}

	if len(summary.Branches) != 1 {
		t.Fatalf("expected 1 branch summary, got %d", len(summary.Branches))
	}

	if len(summary.Branches[0].Issues) == 0 {
		t.Fatalf("expected health issues for missing anchor")
	}
}

func setupRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	run(t, dir, "git", "init", "-b", "main")
	run(t, dir, "git", "config", "user.email", "stack@example.com")
	run(t, dir, "git", "config", "user.name", "Stack Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run(t, dir, "git", "add", "README.md")
	run(t, dir, "git", "commit", "-m", "init")
	return dir
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, string(output))
	}
}
