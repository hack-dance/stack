package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func SetupGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	Run(t, dir, "git", "init", "-b", "main")
	Run(t, dir, "git", "config", "user.email", "stack@example.com")
	Run(t, dir, "git", "config", "user.name", "Stack Test")
	WriteFile(t, filepath.Join(dir, "README.md"), "hello\n")
	Run(t, dir, "git", "add", "README.md")
	Run(t, dir, "git", "commit", "-m", "init")
	return dir
}

func WriteFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func Run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, string(output))
	}
	return string(output)
}

func RunContext(t *testing.T, ctx context.Context, dir string, name string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, string(output))
	}
	return string(output)
}
