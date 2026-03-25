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
