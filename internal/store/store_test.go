package store_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	stackgit "github.com/hack-dance/stack/internal/git"
	"github.com/hack-dance/stack/internal/store"
)

func TestInitAndReadState(t *testing.T) {
	t.Parallel()

	repo := setupGitRepo(t)
	client := stackgit.NewClient(repo)
	stateStore := store.New(client)

	state, err := stateStore.InitState(context.Background(), "main", "origin", "hack-dance/stack")
	if err != nil {
		t.Fatalf("init state: %v", err)
	}

	if state.Trunk != "main" {
		t.Fatalf("expected trunk main, got %q", state.Trunk)
	}

	readState, err := stateStore.ReadState(context.Background())
	if err != nil {
		t.Fatalf("read state: %v", err)
	}

	if readState.DefaultRemote != "origin" {
		t.Fatalf("expected remote origin, got %q", readState.DefaultRemote)
	}
}

func TestResolvePathsUsesCommonDirForStateAndGitDirForOperationInWorktree(t *testing.T) {
	t.Parallel()

	repo := setupGitRepo(t)
	worktreeDir := filepath.Join(t.TempDir(), "worktree")
	run(t, repo, "git", "worktree", "add", "-b", "feature/worktree", worktreeDir, "main")

	client := stackgit.NewClient(worktreeDir)
	stateStore := store.New(client)

	paths, err := stateStore.ResolvePaths(context.Background())
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	expectedStateFile := normalizePath(t, filepath.Join(repo, ".git", "stack", "state.json"))
	actualStateFile := normalizePath(t, paths.StateFile)
	if actualStateFile != expectedStateFile {
		t.Fatalf("expected shared state file %q, got %q", expectedStateFile, paths.StateFile)
	}

	if filepath.Dir(paths.OpFile) == filepath.Dir(paths.StateFile) {
		t.Fatalf("expected worktree op file to differ from shared state path")
	}

	if filepath.Base(paths.CommonDir) != ".git" {
		t.Fatalf("expected common dir to point at repo .git, got %q", paths.CommonDir)
	}
}

func setupGitRepo(t *testing.T) string {
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

func normalizePath(t *testing.T, path string) string {
	t.Helper()
	return strings.TrimPrefix(filepath.Clean(path), "/private")
}
