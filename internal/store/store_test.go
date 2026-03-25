package store_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
