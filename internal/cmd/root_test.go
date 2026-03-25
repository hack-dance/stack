package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	charmlog "github.com/charmbracelet/log"
	stackgit "github.com/hack-dance/stack/internal/git"
	stackgh "github.com/hack-dance/stack/internal/github"
	stackruntime "github.com/hack-dance/stack/internal/runtime"
	"github.com/hack-dance/stack/internal/store"
	"github.com/hack-dance/stack/internal/testutil"
)

func TestRestackStepsRejectInvalidAnchor(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	runtime := newTestRuntime(repo)

	mainHead, err := runtime.Git.ResolveRef(runtime.Context, "main")
	if err != nil {
		t.Fatalf("resolve main head: %v", err)
	}

	testutil.Run(t, repo, "git", "switch", "-c", "unrelated")
	testutil.WriteFile(t, filepath.Join(repo, "unrelated.txt"), "unrelated\n")
	testutil.Run(t, repo, "git", "add", "unrelated.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add unrelated")
	unrelatedHead, err := runtime.Git.ResolveRef(runtime.Context, "HEAD")
	if err != nil {
		t.Fatalf("resolve unrelated head: %v", err)
	}

	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature a")

	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches: map[string]store.BranchRecord{
			"feature/a": {
				ParentBranch: "main",
				Restack: store.RestackMetadata{
					LastParentHeadOID: unrelatedHead,
				},
			},
		},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	if _, err := restackStepsForTargets(runtime, state, []string{"feature/a"}); err == nil {
		t.Fatalf("expected invalid anchor error")
	} else if !strings.Contains(err.Error(), "refusing to guess a merge-base fallback") {
		t.Fatalf("unexpected error: %v", err)
	}

	if mainHead == unrelatedHead {
		t.Fatalf("expected unrelated head to differ from main head")
	}
}

func TestSyncApplyReparentsCleanMergedParent(t *testing.T) {
	repo, runtime, featureAHead := setupTrackedStackRepo(t)
	ghStub := testutil.SetupGHStub(t, "hack-dance/stack", "main")
	t.Setenv("STACK_TEST_GH_STATE", ghStub.StatePath)
	t.Setenv("STACK_TEST_GH_LOG", ghStub.LogPath)
	t.Setenv("PATH", ghStub.Dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeGHState(t, ghStub.StatePath, `{
  "repo": {
    "nameWithOwner": "hack-dance/stack",
    "url": "https://github.com/hack-dance/stack",
    "defaultBranchRef": { "name": "main" }
  },
  "prs": {
    "1": {
      "id": "PR_1",
      "number": 1,
      "url": "https://example.com/hack-dance/stack/pull/1",
      "repo": "hack-dance/stack",
      "headRefName": "feature/a",
      "baseRefName": "main",
      "lastSeenHeadOid": "`+featureAHead+`",
      "lastSeenBaseOid": "",
      "state": "MERGED",
      "isDraft": false
    }
  },
  "next_number": 2
}`)

	executeCommand(t, runtime, "sync", "--apply")

	state, err := runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if got := state.Branches["feature/b"].ParentBranch; got != "main" {
		t.Fatalf("expected feature/b parent to be main, got %q", got)
	}

	_ = repo
}

func TestSyncApplySkipsAmbiguousMergedParent(t *testing.T) {
	_, runtime, _ := setupTrackedStackRepo(t)
	ghStub := testutil.SetupGHStub(t, "hack-dance/stack", "main")
	t.Setenv("STACK_TEST_GH_STATE", ghStub.StatePath)
	t.Setenv("STACK_TEST_GH_LOG", ghStub.LogPath)
	t.Setenv("PATH", ghStub.Dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	state, err := runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	state.Branches["feature/b"] = store.BranchRecord{
		ParentBranch: "feature/a",
		RemoteName:   "origin",
		Restack: store.RestackMetadata{
			LastParentHeadOID: state.Branches["feature/a"].Restack.LastParentHeadOID,
		},
	}
	mainHead, err := runtime.Git.ResolveRef(runtime.Context, "main")
	if err != nil {
		t.Fatalf("resolve main head: %v", err)
	}
	record := state.Branches["feature/b"]
	record.Restack.LastParentHeadOID = mainHead
	state.Branches["feature/b"] = record
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	featureAHead, err := runtime.Git.ResolveRef(runtime.Context, "feature/a")
	if err != nil {
		t.Fatalf("resolve feature/a head: %v", err)
	}
	writeGHState(t, ghStub.StatePath, `{
  "repo": {
    "nameWithOwner": "hack-dance/stack",
    "url": "https://github.com/hack-dance/stack",
    "defaultBranchRef": { "name": "main" }
  },
  "prs": {
    "1": {
      "id": "PR_1",
      "number": 1,
      "url": "https://example.com/hack-dance/stack/pull/1",
      "repo": "hack-dance/stack",
      "headRefName": "feature/a",
      "baseRefName": "main",
      "lastSeenHeadOid": "`+featureAHead+`",
      "lastSeenBaseOid": "",
      "state": "MERGED",
      "isDraft": false
    }
  },
  "next_number": 2
}`)

	executeCommand(t, runtime, "sync", "--apply")

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if got := state.Branches["feature/b"].ParentBranch; got != "feature/a" {
		t.Fatalf("expected feature/b parent to stay feature/a, got %q", got)
	}
}

func TestSubmitCreatesAndTracksPR(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testutil.Run(t, repo, "git", "init", "--bare", remote)
	testutil.Run(t, repo, "git", "remote", "add", "origin", remote)
	testutil.Run(t, repo, "git", "push", "-u", "origin", "main")

	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature a")

	runtime := newTestRuntime(repo)
	mainHead, err := runtime.Git.ResolveRef(runtime.Context, "main")
	if err != nil {
		t.Fatalf("resolve main head: %v", err)
	}
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches: map[string]store.BranchRecord{
			"feature/a": {
				ParentBranch: "main",
				RemoteName:   "origin",
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
		},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	ghStub := testutil.SetupGHStub(t, "hack-dance/stack", "main")
	t.Setenv("STACK_TEST_GH_STATE", ghStub.StatePath)
	t.Setenv("STACK_TEST_GH_LOG", ghStub.LogPath)
	t.Setenv("PATH", ghStub.Dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	executeCommand(t, runtime, "submit", "feature/a", "--yes", "--no-restack")

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state.Branches["feature/a"].PR.Number != 1 {
		t.Fatalf("expected tracked PR number 1, got %d", state.Branches["feature/a"].PR.Number)
	}
	if state.Branches["feature/a"].PR.BaseRefName != "main" {
		t.Fatalf("expected PR base main, got %q", state.Branches["feature/a"].PR.BaseRefName)
	}

	if !runtime.Git.RemoteBranchExists(runtime.Context, "origin", "feature/a") {
		t.Fatalf("expected remote feature/a branch to exist")
	}

	log := readFile(t, ghStub.LogPath)
	if !strings.Contains(log, "pr create --base main --head feature/a") {
		t.Fatalf("expected gh create log entry, got %q", log)
	}
}

func TestQueueMergesHealthyBottomBranch(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testutil.Run(t, repo, "git", "init", "--bare", remote)
	testutil.Run(t, repo, "git", "remote", "add", "origin", remote)
	testutil.Run(t, repo, "git", "push", "-u", "origin", "main")

	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature a")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "feature/a")

	runtime := newTestRuntime(repo)
	head, err := runtime.Git.ResolveRef(runtime.Context, "feature/a")
	if err != nil {
		t.Fatalf("resolve feature/a head: %v", err)
	}
	mainHead, err := runtime.Git.ResolveRef(runtime.Context, "main")
	if err != nil {
		t.Fatalf("resolve main head: %v", err)
	}

	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches: map[string]store.BranchRecord{
			"feature/a": {
				ParentBranch: "main",
				RemoteName:   "origin",
				PR: store.PullRequest{
					Number:          1,
					HeadRefName:     "feature/a",
					BaseRefName:     "main",
					LastSeenHeadOID: head,
					State:           "OPEN",
				},
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
		},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	ghStub := testutil.SetupGHStub(t, "hack-dance/stack", "main")
	t.Setenv("STACK_TEST_GH_STATE", ghStub.StatePath)
	t.Setenv("STACK_TEST_GH_LOG", ghStub.LogPath)
	t.Setenv("PATH", ghStub.Dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeGHState(t, ghStub.StatePath, `{
  "repo": {
    "nameWithOwner": "hack-dance/stack",
    "url": "https://github.com/hack-dance/stack",
    "defaultBranchRef": { "name": "main" }
  },
  "prs": {
    "1": {
      "id": "PR_1",
      "number": 1,
      "url": "https://example.com/hack-dance/stack/pull/1",
      "repo": "hack-dance/stack",
      "headRefName": "feature/a",
      "baseRefName": "main",
      "lastSeenHeadOid": "`+head+`",
      "lastSeenBaseOid": "",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 2
}`)

	executeCommand(t, runtime, "queue", "feature/a", "--yes")

	log := readFile(t, ghStub.LogPath)
	expected := "pr merge 1 --auto --match-head-commit " + head
	if !strings.Contains(log, expected) {
		t.Fatalf("expected gh merge log %q, got %q", expected, log)
	}
}

func executeCommand(t *testing.T, runtime *stackruntime.Runtime, args ...string) string {
	t.Helper()

	root := NewRootCommand(runtime)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute %v: %v\nstderr:\n%s", args, err, stderr.String())
	}

	return stdout.String()
}

func newTestRuntime(repo string) *stackruntime.Runtime {
	gitClient := stackgit.NewClient(repo)
	return &stackruntime.Runtime{
		Context: context.Background(),
		Git:     gitClient,
		GitHub:  stackgh.NewClient(repo),
		Store:   store.New(gitClient),
		Logger:  charmlog.New(io.Discard),
	}
}

func setupTrackedStackRepo(t *testing.T) (string, *stackruntime.Runtime, string) {
	t.Helper()

	repo := testutil.SetupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testutil.Run(t, repo, "git", "init", "--bare", remote)
	testutil.Run(t, repo, "git", "remote", "add", "origin", remote)
	testutil.Run(t, repo, "git", "push", "-u", "origin", "main")

	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature a")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "feature/a")

	testutil.Run(t, repo, "git", "switch", "-c", "feature/b")
	testutil.WriteFile(t, filepath.Join(repo, "feature-b.txt"), "feature b\n")
	testutil.Run(t, repo, "git", "add", "feature-b.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature b")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "feature/b")

	runtime := newTestRuntime(repo)
	mainHead, err := runtime.Git.ResolveRef(runtime.Context, "main")
	if err != nil {
		t.Fatalf("resolve main head: %v", err)
	}
	featureAHead, err := runtime.Git.ResolveRef(runtime.Context, "feature/a")
	if err != nil {
		t.Fatalf("resolve feature/a head: %v", err)
	}

	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches: map[string]store.BranchRecord{
			"feature/a": {
				ParentBranch: "main",
				RemoteName:   "origin",
				PR: store.PullRequest{
					Number:          1,
					HeadRefName:     "feature/a",
					BaseRefName:     "main",
					LastSeenHeadOID: featureAHead,
					State:           "OPEN",
				},
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
			"feature/b": {
				ParentBranch: "feature/a",
				RemoteName:   "origin",
				Restack: store.RestackMetadata{
					LastParentHeadOID: featureAHead,
				},
			},
		},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	return repo, runtime, featureAHead
}

func writeGHState(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write gh state: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return string(data)
}
