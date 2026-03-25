package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	stackgit "github.com/hack-dance/stack/internal/git"
)

func TestPushBranchRejectsStaleRemoteLease(t *testing.T) {
	t.Parallel()

	repo := setupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	run(t, repo, "git", "init", "--bare", remote)
	run(t, repo, "git", "remote", "add", "origin", remote)
	run(t, repo, "git", "push", "-u", "origin", "main")

	run(t, repo, "git", "switch", "-c", "feature/a")
	writeFile(t, filepath.Join(repo, "feature.txt"), "one\n")
	run(t, repo, "git", "add", "feature.txt")
	run(t, repo, "git", "commit", "-m", "feature one")
	run(t, repo, "git", "push", "-u", "origin", "feature/a")

	client := stackgit.NewClient(repo)
	initialRemoteOID, exists, err := client.RemoteBranchOID(context.Background(), "origin", "feature/a")
	if err != nil {
		t.Fatalf("read remote branch oid: %v", err)
	}
	if !exists {
		t.Fatalf("expected remote branch to exist")
	}

	otherClone := filepath.Join(t.TempDir(), "other")
	run(t, repo, "git", "clone", remote, otherClone)
	run(t, otherClone, "git", "config", "user.email", "stack@example.com")
	run(t, otherClone, "git", "config", "user.name", "Stack Test")
	run(t, otherClone, "git", "switch", "feature/a")
	writeFile(t, filepath.Join(otherClone, "feature.txt"), "two\n")
	run(t, otherClone, "git", "add", "feature.txt")
	run(t, otherClone, "git", "commit", "-m", "remote update")
	run(t, otherClone, "git", "push", "origin", "feature/a")

	writeFile(t, filepath.Join(repo, "local.txt"), "local\n")
	run(t, repo, "git", "add", "local.txt")
	run(t, repo, "git", "commit", "-m", "local update")

	err = client.PushBranch(context.Background(), "origin", "feature/a", initialRemoteOID)
	if err == nil {
		t.Fatalf("expected explicit lease push to fail after remote moved")
	}
	if !strings.Contains(err.Error(), "force-with-lease") {
		t.Fatalf("expected force-with-lease failure, got %v", err)
	}
}

func TestIsAncestorReturnsFalseForSiblingCommits(t *testing.T) {
	t.Parallel()

	repo := setupGitRepo(t)
	run(t, repo, "git", "switch", "-c", "feature/a")
	writeFile(t, filepath.Join(repo, "feature-a.txt"), "a\n")
	run(t, repo, "git", "add", "feature-a.txt")
	run(t, repo, "git", "commit", "-m", "feature a")
	featureHead := strings.TrimSpace(runOutput(t, repo, "git", "rev-parse", "HEAD"))

	run(t, repo, "git", "switch", "main")
	run(t, repo, "git", "switch", "-c", "feature/b")
	writeFile(t, filepath.Join(repo, "feature-b.txt"), "b\n")
	run(t, repo, "git", "add", "feature-b.txt")
	run(t, repo, "git", "commit", "-m", "feature b")

	client := stackgit.NewClient(repo)
	isAncestor, err := client.IsAncestor(context.Background(), featureHead, "feature/b")
	if err != nil {
		t.Fatalf("is ancestor: %v", err)
	}
	if isAncestor {
		t.Fatalf("expected sibling commit to not be an ancestor")
	}
}

func TestRebaseContinueDoesNotRequireInteractiveEditor(t *testing.T) {
	repo := setupGitRepo(t)
	baseOID := strings.TrimSpace(runOutput(t, repo, "git", "rev-parse", "HEAD"))

	run(t, repo, "git", "switch", "-c", "feature/a")
	writeFile(t, filepath.Join(repo, "shared.txt"), "feature branch\n")
	run(t, repo, "git", "add", "shared.txt")
	run(t, repo, "git", "commit", "-m", "feature change")

	run(t, repo, "git", "switch", "main")
	writeFile(t, filepath.Join(repo, "shared.txt"), "main branch\n")
	run(t, repo, "git", "add", "shared.txt")
	run(t, repo, "git", "commit", "-m", "main change")

	client := stackgit.NewClient(repo)
	err := client.RebaseOnto(context.Background(), "main", baseOID, "feature/a")
	if err == nil {
		t.Fatalf("expected rebase conflict")
	}

	writeFile(t, filepath.Join(repo, "shared.txt"), "resolved during test\n")
	run(t, repo, "git", "add", "shared.txt")
	t.Setenv("GIT_EDITOR", "false")

	if err := client.RebaseContinue(context.Background()); err != nil {
		t.Fatalf("rebase continue: %v", err)
	}

	inProgress, err := client.RebaseInProgress(context.Background())
	if err != nil {
		t.Fatalf("rebase in progress: %v", err)
	}
	if inProgress {
		t.Fatalf("expected rebase to be complete")
	}

	content := runOutput(t, repo, "git", "show", "HEAD:shared.txt")
	if strings.TrimSpace(content) != "resolved during test" {
		t.Fatalf("unexpected rebased content: %q", content)
	}
}

func setupGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	run(t, dir, "git", "init", "-b", "main")
	run(t, dir, "git", "config", "user.email", "stack@example.com")
	run(t, dir, "git", "config", "user.name", "Stack Test")
	writeFile(t, filepath.Join(dir, "README.md"), "hello\n")
	run(t, dir, "git", "add", "README.md")
	run(t, dir, "git", "commit", "-m", "init")
	return dir
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
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

func runOutput(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, string(output))
	}
	return string(output)
}
