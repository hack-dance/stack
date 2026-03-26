package stack_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestBuildSummaryFlagsUnlinkedRemoteBranchWithSubmitGuidance(t *testing.T) {
	t.Parallel()

	repo := setupRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	run(t, repo, "git", "init", "--bare", remote)
	run(t, repo, "git", "remote", "add", "origin", remote)
	run(t, repo, "git", "push", "-u", "origin", "main")

	run(t, repo, "git", "switch", "-c", "feature/a")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run(t, repo, "git", "add", "a.txt")
	run(t, repo, "git", "commit", "-m", "add a")
	run(t, repo, "git", "push", "-u", "origin", "feature/a")

	client := stackgit.NewClient(repo)
	mainHead := strings.TrimSpace(runOutput(t, repo, "git", "rev-parse", "main"))
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches: map[string]store.BranchRecord{
			"feature/a": {
				ParentBranch: "main",
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
		},
	}

	summary, err := stack.BuildSummary(context.Background(), client, state)
	if err != nil {
		t.Fatalf("build summary: %v", err)
	}

	if len(summary.Branches) != 1 {
		t.Fatalf("expected 1 branch summary, got %d", len(summary.Branches))
	}

	found := false
	for _, issue := range summary.Branches[0].Issues {
		if strings.Contains(issue.Message, "run `stack submit` to relink or create one") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected actionable relink guidance in summary, got %+v", summary.Branches[0].Issues)
	}
}

func TestBuildSummaryIncludesLatestVerificationAndStaleWarning(t *testing.T) {
	t.Parallel()

	repo := setupRepo(t)
	client := stackgit.NewClient(repo)

	run(t, repo, "git", "switch", "-c", "feature/a")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run(t, repo, "git", "add", "a.txt")
	run(t, repo, "git", "commit", "-m", "add a")
	verifiedHead := strings.TrimSpace(runOutput(t, repo, "git", "rev-parse", "HEAD"))

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a changed\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run(t, repo, "git", "add", "a.txt")
	run(t, repo, "git", "commit", "-m", "advance a")

	mainHead := strings.TrimSpace(runOutput(t, repo, "git", "rev-parse", "main"))
	score := 100
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches: map[string]store.BranchRecord{
			"feature/a": {
				ParentBranch: "main",
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
		},
		Verifications: map[string][]store.VerificationRecord{
			"feature/a": {
				{
					CheckType:  "sim",
					Identifier: "run-123",
					Passed:     true,
					HeadOID:    verifiedHead,
					RecordedAt: "2026-03-26T17:30:00Z",
					Score:      &score,
				},
			},
		},
	}

	summary, err := stack.BuildSummary(context.Background(), client, state)
	if err != nil {
		t.Fatalf("build summary: %v", err)
	}

	if summary.Branches[0].Verification == nil {
		t.Fatalf("expected verification summary")
	}
	if summary.Branches[0].Verification.Latest.Identifier != "run-123" {
		t.Fatalf("expected latest verification identifier run-123, got %+v", summary.Branches[0].Verification)
	}
	if summary.Branches[0].Verification.HeadMatchesCurrent {
		t.Fatalf("expected verification to be stale after branch head moved")
	}

	found := false
	for _, issue := range summary.Branches[0].Issues {
		if strings.Contains(issue.Message, "branch head moved since the latest recorded verification") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected stale verification issue, got %+v", summary.Branches[0].Issues)
	}
}

func TestBuildSummaryIncludesLandingBranchesFromVerificationRecords(t *testing.T) {
	t.Parallel()

	repo := setupRepo(t)
	client := stackgit.NewClient(repo)

	run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	if err := os.WriteFile(filepath.Join(repo, "landing.txt"), []byte("landing\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run(t, repo, "git", "add", "landing.txt")
	run(t, repo, "git", "commit", "-m", "landing")
	landingHead := strings.TrimSpace(runOutput(t, repo, "git", "rev-parse", "HEAD"))

	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches:      map[string]store.BranchRecord{},
		Verifications: map[string][]store.VerificationRecord{
			"stack/discovery-core": {
				{
					CheckType:  "manual",
					Identifier: "deploy-check",
					Passed:     false,
					HeadOID:    landingHead,
					RecordedAt: "2026-03-26T17:45:00Z",
					Note:       "post-deploy pending",
				},
			},
		},
	}

	summary, err := stack.BuildSummary(context.Background(), client, state)
	if err != nil {
		t.Fatalf("build summary: %v", err)
	}

	if len(summary.LandingBranches) != 1 {
		t.Fatalf("expected 1 landing branch summary, got %+v", summary.LandingBranches)
	}
	landing := summary.LandingBranches[0]
	if landing.Name != "stack/discovery-core" {
		t.Fatalf("unexpected landing branch summary %+v", landing)
	}
	if landing.Verification == nil || landing.Verification.Latest.Identifier != "deploy-check" {
		t.Fatalf("expected landing verification summary, got %+v", landing.Verification)
	}
	found := false
	for _, issue := range landing.Issues {
		if strings.Contains(issue.Message, "latest manual verification failed") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected failed verification guidance on landing branch, got %+v", landing.Issues)
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
