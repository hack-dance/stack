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

func TestRestackStepsIncludeDescendantsWhenAncestorMoves(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	runtime := newTestRuntime(repo)

	mainHead, err := runtime.Git.ResolveRef(runtime.Context, "main")
	if err != nil {
		t.Fatalf("resolve main head: %v", err)
	}

	testutil.Run(t, repo, "git", "switch", "-c", "feature/base")
	testutil.WriteFile(t, filepath.Join(repo, "feature-base.txt"), "feature base\n")
	testutil.Run(t, repo, "git", "add", "feature-base.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature base")
	featureBaseHead, err := runtime.Git.ResolveRef(runtime.Context, "feature/base")
	if err != nil {
		t.Fatalf("resolve feature/base head: %v", err)
	}

	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature a")
	featureAHead, err := runtime.Git.ResolveRef(runtime.Context, "feature/a")
	if err != nil {
		t.Fatalf("resolve feature/a head: %v", err)
	}

	testutil.Run(t, repo, "git", "switch", "-c", "feature/b")
	testutil.WriteFile(t, filepath.Join(repo, "feature-b.txt"), "feature b\n")
	testutil.Run(t, repo, "git", "add", "feature-b.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature b")
	featureBHead, err := runtime.Git.ResolveRef(runtime.Context, "feature/b")
	if err != nil {
		t.Fatalf("resolve feature/b head: %v", err)
	}

	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches: map[string]store.BranchRecord{
			"feature/base": {
				ParentBranch: "main",
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
			"feature/a": {
				ParentBranch: "feature/base",
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
			"feature/b": {
				ParentBranch: "feature/a",
				Restack: store.RestackMetadata{
					LastParentHeadOID: featureAHead,
				},
			},
		},
	}

	steps, err := restackStepsForTargets(runtime, state, []string{"feature/a"})
	if err != nil {
		t.Fatalf("restack steps: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 restack steps, got %d", len(steps))
	}
	if steps[0].Branch != "feature/a" || steps[0].Parent != "feature/base" {
		t.Fatalf("unexpected first step: %+v", steps[0])
	}
	if steps[0].PreviousParentHead != mainHead {
		t.Fatalf("expected feature/a previous parent head %q, got %q", mainHead, steps[0].PreviousParentHead)
	}
	if steps[0].PreviousBranchHead != featureAHead {
		t.Fatalf("expected feature/a previous branch head %q, got %q", featureAHead, steps[0].PreviousBranchHead)
	}
	if featureBaseHead == featureAHead {
		t.Fatalf("expected feature/base head to differ from feature/a head")
	}
	if steps[1].Branch != "feature/b" || steps[1].Parent != "feature/a" {
		t.Fatalf("unexpected second step: %+v", steps[1])
	}
	if steps[1].PreviousParentHead != featureAHead {
		t.Fatalf("expected feature/b previous parent head %q, got %q", featureAHead, steps[1].PreviousParentHead)
	}
	if steps[1].PreviousBranchHead != featureBHead {
		t.Fatalf("expected feature/b previous branch head %q, got %q", featureBHead, steps[1].PreviousBranchHead)
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
    },
    "2": {
      "id": "PR_2",
      "number": 2,
      "url": "https://example.com/hack-dance/stack/pull/2",
      "repo": "hack-dance/stack",
      "headRefName": "feature/b",
      "baseRefName": "feature/a",
      "lastSeenHeadOid": "",
      "lastSeenBaseOid": "",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 3
}`)

	executeCommand(t, runtime, "sync", "--apply")

	state, err := runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if got := state.Branches["feature/b"].ParentBranch; got != "main" {
		t.Fatalf("expected feature/b parent to be main, got %q", got)
	}
	if got := state.Branches["feature/b"].PR.BaseRefName; got != "main" {
		t.Fatalf("expected feature/b PR base to be main, got %q", got)
	}

	log := readFile(t, ghStub.LogPath)
	if !strings.Contains(log, "pr edit 2 --base main") {
		t.Fatalf("expected child PR retarget, got %q", log)
	}

	_ = repo
}

func TestMoveRestacksDescendantSubtree(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testutil.Run(t, repo, "git", "init", "--bare", remote)
	testutil.Run(t, repo, "git", "remote", "add", "origin", remote)
	testutil.Run(t, repo, "git", "push", "-u", "origin", "main")

	testutil.Run(t, repo, "git", "switch", "-c", "feature/base")
	testutil.WriteFile(t, filepath.Join(repo, "feature-base.txt"), "feature base\n")
	testutil.Run(t, repo, "git", "add", "feature-base.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature base")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "feature/base")
	featureBaseHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/base"))

	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature a")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "feature/a")
	oldFeatureAHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/a"))

	testutil.Run(t, repo, "git", "switch", "-c", "feature/b")
	testutil.WriteFile(t, filepath.Join(repo, "feature-b.txt"), "feature b\n")
	testutil.Run(t, repo, "git", "add", "feature-b.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature b")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "feature/b")
	oldFeatureBHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/b"))

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
			"feature/base": {
				ParentBranch: "main",
				RemoteName:   "origin",
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
			"feature/a": {
				ParentBranch: "main",
				RemoteName:   "origin",
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
			"feature/b": {
				ParentBranch: "feature/a",
				RemoteName:   "origin",
				Restack: store.RestackMetadata{
					LastParentHeadOID: oldFeatureAHead,
				},
			},
		},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	output := executeCommand(t, runtime, "move", "feature/a", "--parent", "feature/base", "--yes")
	if !strings.Contains(output, "feature/b: restack on top of rewritten feature/a") {
		t.Fatalf("expected descendant preview, got %q", output)
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if got := state.Branches["feature/a"].ParentBranch; got != "feature/base" {
		t.Fatalf("expected feature/a parent to be feature/base, got %q", got)
	}
	if got := state.Branches["feature/b"].ParentBranch; got != "feature/a" {
		t.Fatalf("expected feature/b parent to stay feature/a, got %q", got)
	}

	newFeatureAHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/a"))
	newFeatureBHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/b"))
	if featureBaseHead == newFeatureAHead {
		t.Fatalf("expected moved feature/a head to remain distinct from feature/base head")
	}
	if newFeatureAHead == oldFeatureAHead {
		t.Fatalf("expected feature/a head to change after move")
	}
	if newFeatureBHead == oldFeatureBHead {
		t.Fatalf("expected feature/b head to change after move")
	}
	if got := state.Branches["feature/b"].Restack.LastParentHeadOID; got != newFeatureAHead {
		t.Fatalf("expected feature/b restack parent head %q, got %q", newFeatureAHead, got)
	}

	mergeBase := strings.TrimSpace(testutil.Run(t, repo, "git", "merge-base", "feature/a", "feature/b"))
	if mergeBase != newFeatureAHead {
		t.Fatalf("expected feature/b to be restacked onto new feature/a head %q, got merge-base %q", newFeatureAHead, mergeBase)
	}
}

func TestMoveDoesNotPersistParentWhenDescendantAnchorMissing(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testutil.Run(t, repo, "git", "init", "--bare", remote)
	testutil.Run(t, repo, "git", "remote", "add", "origin", remote)
	testutil.Run(t, repo, "git", "push", "-u", "origin", "main")

	testutil.Run(t, repo, "git", "switch", "-c", "feature/base")
	testutil.WriteFile(t, filepath.Join(repo, "feature-base.txt"), "feature base\n")
	testutil.Run(t, repo, "git", "add", "feature-base.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature base")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "feature/base")

	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature a")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "feature/a")
	featureAHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/a"))

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
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches: map[string]store.BranchRecord{
			"feature/base": {
				ParentBranch: "main",
				RemoteName:   "origin",
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
			"feature/a": {
				ParentBranch: "main",
				RemoteName:   "origin",
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
			"feature/b": {
				ParentBranch: "feature/a",
				RemoteName:   "origin",
				Restack:      store.RestackMetadata{},
			},
		},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	err = executeCommandExpectError(runtime, "move", "feature/a", "--parent", "feature/base", "--yes")
	if err == nil || !strings.Contains(err.Error(), "feature/b") || !strings.Contains(err.Error(), "no recorded restack anchor") {
		t.Fatalf("expected descendant anchor error, got %v", err)
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if got := state.Branches["feature/a"].ParentBranch; got != "main" {
		t.Fatalf("expected feature/a parent to stay main, got %q", got)
	}
	if got := state.Branches["feature/b"].Restack.LastParentHeadOID; got != "" {
		t.Fatalf("expected missing feature/b anchor to remain empty, got %q", got)
	}
	if got := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/a")); got != featureAHead {
		t.Fatalf("expected feature/a head to stay %q, got %q", featureAHead, got)
	}
}

func TestMoveRepairsDescendantWithStaleRecordedAnchor(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testutil.Run(t, repo, "git", "init", "--bare", remote)
	testutil.Run(t, repo, "git", "remote", "add", "origin", remote)
	testutil.Run(t, repo, "git", "push", "-u", "origin", "main")

	testutil.Run(t, repo, "git", "switch", "-c", "feature/base")
	testutil.WriteFile(t, filepath.Join(repo, "feature-base.txt"), "feature base\n")
	testutil.Run(t, repo, "git", "add", "feature-base.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature base")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "feature/base")

	testutil.Run(t, repo, "git", "switch", "-c", "unrelated")
	testutil.WriteFile(t, filepath.Join(repo, "unrelated.txt"), "unrelated\n")
	testutil.Run(t, repo, "git", "add", "unrelated.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add unrelated")
	unrelatedHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	testutil.Run(t, repo, "git", "switch", "main")
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
	oldFeatureBHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/b"))

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
			"feature/base": {
				ParentBranch: "main",
				RemoteName:   "origin",
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
			"feature/a": {
				ParentBranch: "main",
				RemoteName:   "origin",
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
			"feature/b": {
				ParentBranch: "feature/a",
				RemoteName:   "origin",
				Restack: store.RestackMetadata{
					LastParentHeadOID: unrelatedHead,
				},
			},
		},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	output := executeCommand(t, runtime, "move", "feature/a", "--parent", "feature/base", "--yes")
	if !strings.Contains(output, "feature/b: restack on top of rewritten feature/a") {
		t.Fatalf("expected descendant preview, got %q", output)
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if got := state.Branches["feature/a"].ParentBranch; got != "feature/base" {
		t.Fatalf("expected feature/a parent to be feature/base, got %q", got)
	}
	newFeatureAHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/a"))
	if got := state.Branches["feature/b"].Restack.LastParentHeadOID; got != newFeatureAHead {
		t.Fatalf("expected feature/b anchor to be repaired to %q, got %q", newFeatureAHead, got)
	}
	if got := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/b")); got == oldFeatureBHead {
		t.Fatalf("expected feature/b head to change after repaired move")
	}
}

func TestMoveAbortRestoresOriginalParentAfterConflict(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testutil.Run(t, repo, "git", "init", "--bare", remote)
	testutil.Run(t, repo, "git", "remote", "add", "origin", remote)
	testutil.Run(t, repo, "git", "push", "-u", "origin", "main")

	testutil.WriteFile(t, filepath.Join(repo, "shared.txt"), "base\n")
	testutil.Run(t, repo, "git", "add", "shared.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add shared file")
	testutil.Run(t, repo, "git", "push", "origin", "main")

	testutil.Run(t, repo, "git", "switch", "-c", "feature/base")
	testutil.WriteFile(t, filepath.Join(repo, "shared.txt"), "base branch\n")
	testutil.Run(t, repo, "git", "add", "shared.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "change shared on feature base")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "feature/base")

	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "shared.txt"), "feature a branch\n")
	testutil.Run(t, repo, "git", "add", "shared.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "change shared on feature a")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "feature/a")

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
			"feature/base": {
				ParentBranch: "main",
				RemoteName:   "origin",
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
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

	err = executeCommandExpectError(runtime, "move", "feature/a", "--parent", "feature/base", "--yes")
	if err == nil {
		t.Fatalf("expected conflict during move")
	}

	rebaseInProgress, err := runtime.Git.RebaseInProgress(runtime.Context)
	if err != nil {
		t.Fatalf("rebase in progress: %v", err)
	}
	if !rebaseInProgress {
		t.Fatalf("expected rebase to be in progress after conflicting move")
	}

	executeCommand(t, runtime, "abort")

	rebaseInProgress, err = runtime.Git.RebaseInProgress(runtime.Context)
	if err != nil {
		t.Fatalf("rebase in progress after abort: %v", err)
	}
	if rebaseInProgress {
		t.Fatalf("expected rebase to be cleared after abort")
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if got := state.Branches["feature/a"].ParentBranch; got != "main" {
		t.Fatalf("expected feature/a parent to stay main after abort, got %q", got)
	}
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
    },
    "2": {
      "id": "PR_2",
      "number": 2,
      "url": "https://example.com/hack-dance/stack/pull/2",
      "repo": "hack-dance/stack",
      "headRefName": "feature/b",
      "baseRefName": "feature/a",
      "lastSeenHeadOid": "",
      "lastSeenBaseOid": "",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 3
}`)

	executeCommand(t, runtime, "sync", "--apply")

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if got := state.Branches["feature/b"].ParentBranch; got != "feature/a" {
		t.Fatalf("expected feature/b parent to stay feature/a, got %q", got)
	}

	log := readFile(t, ghStub.LogPath)
	if strings.Contains(log, "pr edit 2 --base main") {
		t.Fatalf("expected no child PR retarget for ambiguous merged parent, got %q", log)
	}
}

func TestSyncApplySkipsWhenLocalParentMovedPastMergedPRHead(t *testing.T) {
	repo, runtime, featureAHead := setupTrackedStackRepo(t)
	ghStub := testutil.SetupGHStub(t, "hack-dance/stack", "main")
	t.Setenv("STACK_TEST_GH_STATE", ghStub.StatePath)
	t.Setenv("STACK_TEST_GH_LOG", ghStub.LogPath)
	t.Setenv("PATH", ghStub.Dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	testutil.Run(t, repo, "git", "switch", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a moved locally\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "advance feature a locally")

	movedHead, err := runtime.Git.ResolveRef(runtime.Context, "feature/a")
	if err != nil {
		t.Fatalf("resolve moved feature/a head: %v", err)
	}

	state, err := runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	childRecord := state.Branches["feature/b"]
	childRecord.Restack.LastParentHeadOID = movedHead
	state.Branches["feature/b"] = childRecord
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
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
    },
    "2": {
      "id": "PR_2",
      "number": 2,
      "url": "https://example.com/hack-dance/stack/pull/2",
      "repo": "hack-dance/stack",
      "headRefName": "feature/b",
      "baseRefName": "feature/a",
      "lastSeenHeadOid": "",
      "lastSeenBaseOid": "",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 3
}`)

	executeCommand(t, runtime, "sync", "--apply")

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if got := state.Branches["feature/b"].ParentBranch; got != "feature/a" {
		t.Fatalf("expected feature/b parent to stay feature/a, got %q", got)
	}

	log := readFile(t, ghStub.LogPath)
	if strings.Contains(log, "pr edit 2 --base main") {
		t.Fatalf("expected no child PR retarget when local parent moved past merged PR head, got %q", log)
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

	output := executeCommand(t, runtime, "submit", "feature/a", "--yes", "--no-restack")

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

	if !strings.Contains(output, `PR title: "add feature a" (commit subject)`) {
		t.Fatalf("expected submit preview to include commit-subject title source, got %q", output)
	}
	if !strings.Contains(output, "PR body: generated default") {
		t.Fatalf("expected submit preview to include generated body source, got %q", output)
	}

	log := readFile(t, ghStub.LogPath)
	if !strings.Contains(log, "pr create --base main --head feature/a") {
		t.Fatalf("expected gh create log entry, got %q", log)
	}

	ghState := readFile(t, ghStub.StatePath)
	if !strings.Contains(ghState, `"title": "add feature a"`) {
		t.Fatalf("expected gh state to capture commit-subject title, got %q", ghState)
	}
	if !strings.Contains(ghState, "Generated by `stack submit` because the tip commit body was empty.") {
		t.Fatalf("expected gh state to capture generated default body, got %q", ghState)
	}
}

func TestSubmitFallsBackToBranchNameWhenCommitSubjectIsEmpty(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testutil.Run(t, repo, "git", "init", "--bare", remote)
	testutil.Run(t, repo, "git", "remote", "add", "origin", remote)
	testutil.Run(t, repo, "git", "push", "-u", "origin", "main")

	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.Run(t, repo, "git", "commit", "--allow-empty", "--allow-empty-message", "-m", "")

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

	output := executeCommand(t, runtime, "submit", "feature/a", "--yes", "--no-restack")
	if !strings.Contains(output, `PR title: "feature/a" (branch name fallback)`) {
		t.Fatalf("expected branch-name title fallback in preview, got %q", output)
	}
	if !strings.Contains(output, "PR body: generated default") {
		t.Fatalf("expected generated default body in preview, got %q", output)
	}

	ghState := readFile(t, ghStub.StatePath)
	if !strings.Contains(ghState, `"title": "feature/a"`) {
		t.Fatalf("expected gh state to capture branch-name fallback title, got %q", ghState)
	}
	if !strings.Contains(ghState, "Stack branch `feature/a` targeting `main`.") || !strings.Contains(ghState, "Generated by `stack submit` because the tip commit body was empty.") {
		t.Fatalf("expected gh state to capture generated body fallback, got %q", ghState)
	}
}

func TestCreateRejectsDetachedHEAD(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	testutil.Run(t, repo, "git", "checkout", "--detach")

	runtime := newTestRuntime(repo)
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches:      map[string]store.BranchRecord{},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	err := executeCommandExpectError(runtime, "create", "feature/a")
	if err == nil || !strings.Contains(err.Error(), "detached HEAD") {
		t.Fatalf("expected detached HEAD error, got %v", err)
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if len(state.Branches) != 0 {
		t.Fatalf("expected no tracked branches, got %+v", state.Branches)
	}
	if runtime.Git.BranchExists(runtime.Context, "feature/a") {
		t.Fatalf("expected feature/a branch to not be created")
	}
}

func TestCreateRejectsUntrackedCurrentParent(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	testutil.Run(t, repo, "git", "switch", "-c", "feature/base")

	runtime := newTestRuntime(repo)
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches:      map[string]store.BranchRecord{},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	err := executeCommandExpectError(runtime, "create", "feature/child")
	if err == nil || !strings.Contains(err.Error(), "not tracked in local metadata") {
		t.Fatalf("expected untracked parent error, got %v", err)
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if len(state.Branches) != 0 {
		t.Fatalf("expected no tracked branches, got %+v", state.Branches)
	}
	if runtime.Git.BranchExists(runtime.Context, "feature/child") {
		t.Fatalf("expected feature/child branch to not be created")
	}
}

func TestTrackRejectsUntrackedParent(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	testutil.Run(t, repo, "git", "switch", "-c", "feature/base")
	testutil.Run(t, repo, "git", "switch", "-c", "feature/child")

	runtime := newTestRuntime(repo)
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches:      map[string]store.BranchRecord{},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	err := executeCommandExpectError(runtime, "track", "feature/child", "--parent", "feature/base")
	if err == nil || !strings.Contains(err.Error(), "parent branch \"feature/base\" is not tracked in local metadata") {
		t.Fatalf("expected untracked parent error, got %v", err)
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if len(state.Branches) != 0 {
		t.Fatalf("expected no tracked branches, got %+v", state.Branches)
	}
}

func TestTrackUsesMergeBaseAnchorForStaleAdoption(t *testing.T) {
	repo := testutil.SetupGitRepo(t)

	testutil.Run(t, repo, "git", "switch", "-c", "feature/base")
	testutil.WriteFile(t, filepath.Join(repo, "feature-base.txt"), "feature base\n")
	testutil.Run(t, repo, "git", "add", "feature-base.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature base")
	originalFeatureBaseHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	testutil.Run(t, repo, "git", "switch", "-c", "feature/child")
	testutil.WriteFile(t, filepath.Join(repo, "feature-child.txt"), "feature child\n")
	testutil.Run(t, repo, "git", "add", "feature-child.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature child")
	originalFeatureChildHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	testutil.Run(t, repo, "git", "switch", "feature/base")
	testutil.WriteFile(t, filepath.Join(repo, "feature-base.txt"), "feature base advanced\n")
	testutil.Run(t, repo, "git", "add", "feature-base.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "advance feature base")
	advancedFeatureBaseHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

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
		Branches:      map[string]store.BranchRecord{},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	executeCommand(t, runtime, "track", "feature/base", "--parent", "main")
	executeCommand(t, runtime, "track", "feature/child", "--parent", "feature/base")

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state after track: %v", err)
	}
	if got := state.Branches["feature/base"].Restack.LastParentHeadOID; got != mainHead {
		t.Fatalf("expected feature/base anchor %q, got %q", mainHead, got)
	}
	if got := state.Branches["feature/child"].Restack.LastParentHeadOID; got != originalFeatureBaseHead {
		t.Fatalf("expected feature/child merge-base anchor %q, got %q", originalFeatureBaseHead, got)
	}
	if originalFeatureBaseHead == advancedFeatureBaseHead {
		t.Fatalf("expected feature/base head to advance")
	}

	executeCommand(t, runtime, "restack", "--all", "--yes")

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state after restack: %v", err)
	}
	if got := state.Branches["feature/child"].Restack.LastParentHeadOID; got != advancedFeatureBaseHead {
		t.Fatalf("expected feature/child anchor to update to %q, got %q", advancedFeatureBaseHead, got)
	}
	if got := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/child")); got == originalFeatureChildHead {
		t.Fatalf("expected feature/child head to change after restack")
	}

	mergeBase := strings.TrimSpace(testutil.Run(t, repo, "git", "merge-base", "feature/base", "feature/child"))
	if mergeBase != advancedFeatureBaseHead {
		t.Fatalf("expected feature/child merge-base %q after restack, got %q", advancedFeatureBaseHead, mergeBase)
	}
}

func TestAdoptPRFetchesMissingLocalBranchAndTracksPR(t *testing.T) {
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
	featureAHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/a"))

	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "branch", "-D", "feature/a")

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
		Branches:      map[string]store.BranchRecord{},
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
    "7": {
      "id": "PR_7",
      "number": 7,
      "url": "https://example.com/hack-dance/stack/pull/7",
      "repo": "hack-dance/stack",
      "headRefName": "feature/a",
      "baseRefName": "main",
      "headRefOid": "`+featureAHead+`",
      "baseRefOid": "`+mainHead+`",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 8
}`)

	output := executeCommand(t, runtime, "adopt", "pr", "7", "--parent", "main", "--yes")
	if !strings.Contains(output, "fetched: origin/feature/a") {
		t.Fatalf("expected fetch output, got %q", output)
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	record := state.Branches["feature/a"]
	if record.ParentBranch != "main" {
		t.Fatalf("expected feature/a parent main, got %q", record.ParentBranch)
	}
	if record.PR.Number != 7 {
		t.Fatalf("expected tracked PR #7, got %+v", record.PR)
	}
	if record.Restack.LastParentHeadOID != mainHead {
		t.Fatalf("expected restack anchor %q, got %q", mainHead, record.Restack.LastParentHeadOID)
	}
	if !runtime.Git.BranchExists(runtime.Context, "feature/a") {
		t.Fatalf("expected feature/a to exist locally after adopt")
	}
}

func TestAdoptPRUsesMergeBaseAnchorForStaleLocalBranch(t *testing.T) {
	repo := testutil.SetupGitRepo(t)

	testutil.Run(t, repo, "git", "switch", "-c", "feature/base")
	testutil.WriteFile(t, filepath.Join(repo, "feature-base.txt"), "feature base\n")
	testutil.Run(t, repo, "git", "add", "feature-base.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature base")
	originalFeatureBaseHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	testutil.Run(t, repo, "git", "switch", "-c", "feature/child")
	testutil.WriteFile(t, filepath.Join(repo, "feature-child.txt"), "feature child\n")
	testutil.Run(t, repo, "git", "add", "feature-child.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature child")

	testutil.Run(t, repo, "git", "switch", "feature/base")
	testutil.WriteFile(t, filepath.Join(repo, "feature-base.txt"), "feature base advanced\n")
	testutil.Run(t, repo, "git", "add", "feature-base.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "advance feature base")

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
			"feature/base": {
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

	writeGHState(t, ghStub.StatePath, `{
  "repo": {
    "nameWithOwner": "hack-dance/stack",
    "url": "https://github.com/hack-dance/stack",
    "defaultBranchRef": { "name": "main" }
  },
  "prs": {
    "9": {
      "id": "PR_9",
      "number": 9,
      "url": "https://example.com/hack-dance/stack/pull/9",
      "repo": "hack-dance/stack",
      "headRefName": "feature/child",
      "baseRefName": "feature/base",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 10
}`)

	output := executeCommand(t, runtime, "adopt", "pr", "9", "--parent", "feature/base", "--yes")
	if !strings.Contains(output, "restack anchor: merge-base") {
		t.Fatalf("expected merge-base note, got %q", output)
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if got := state.Branches["feature/child"].Restack.LastParentHeadOID; got != originalFeatureBaseHead {
		t.Fatalf("expected feature/child anchor %q, got %q", originalFeatureBaseHead, got)
	}
}

func TestAdoptPRRefreshesStaleLocalBranchToMatchPRHead(t *testing.T) {
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
	staleHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/a"))
	testutil.Run(t, repo, "git", "switch", "main")

	otherClone := filepath.Join(t.TempDir(), "other-clone")
	testutil.Run(t, "", "git", "clone", remote, otherClone)
	testutil.Run(t, otherClone, "git", "config", "user.email", "stack@example.com")
	testutil.Run(t, otherClone, "git", "config", "user.name", "Stack Test")
	testutil.Run(t, otherClone, "git", "switch", "feature/a")
	testutil.WriteFile(t, filepath.Join(otherClone, "feature-a.txt"), "feature a advanced\n")
	testutil.Run(t, otherClone, "git", "add", "feature-a.txt")
	testutil.Run(t, otherClone, "git", "commit", "-m", "advance feature a")
	testutil.Run(t, otherClone, "git", "push", "origin", "feature/a")
	freshHead := strings.TrimSpace(testutil.Run(t, otherClone, "git", "rev-parse", "feature/a"))

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
		Branches:      map[string]store.BranchRecord{},
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
    "7": {
      "id": "PR_7",
      "number": 7,
      "url": "https://example.com/hack-dance/stack/pull/7",
      "repo": "hack-dance/stack",
      "headRefName": "feature/a",
      "baseRefName": "main",
      "headRefOid": "`+freshHead+`",
      "baseRefOid": "`+mainHead+`",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 8
}`)

	output := executeCommand(t, runtime, "adopt", "pr", "7", "--parent", "main", "--yes")
	if !strings.Contains(output, "refreshed: origin/feature/a") {
		t.Fatalf("expected refresh output, got %q", output)
	}

	currentHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/a"))
	if currentHead != freshHead {
		t.Fatalf("expected local feature/a head %q after adopt refresh, got %q (stale was %q)", freshHead, currentHead, staleHead)
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	record := state.Branches["feature/a"]
	if record.PR.Number != 7 {
		t.Fatalf("expected tracked PR #7, got %+v", record.PR)
	}
	if record.Restack.LastParentHeadOID != mainHead {
		t.Fatalf("expected restack anchor %q, got %q", mainHead, record.Restack.LastParentHeadOID)
	}
}

func TestComposeCreatesLandingBranchFromTrackedRange(t *testing.T) {
	repo := testutil.SetupGitRepo(t)

	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature a")
	featureAHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/a"))

	testutil.Run(t, repo, "git", "switch", "-c", "feature/b")
	testutil.WriteFile(t, filepath.Join(repo, "feature-b.txt"), "feature b\n")
	testutil.Run(t, repo, "git", "add", "feature-b.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature b")

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

	output := executeCommand(t, runtime, "compose", "discovery-core", "--from", "feature/a", "--to", "feature/b", "--yes")
	if !strings.Contains(output, "branch: stack/discovery-core") {
		t.Fatalf("expected compose output branch name, got %q", output)
	}

	if !runtime.Git.BranchExists(runtime.Context, "stack/discovery-core") {
		t.Fatalf("expected composed branch to exist")
	}
	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state after compose: %v", err)
	}
	landing, ok := state.Landings["stack/discovery-core"]
	if !ok {
		t.Fatalf("expected landing metadata to be recorded")
	}
	if got := strings.Join(landing.SourceBranches, ","); got != "feature/a,feature/b" {
		t.Fatalf("unexpected landing source branches %q", got)
	}
	currentBranch, err := runtime.Git.CurrentBranch(runtime.Context)
	if err != nil {
		t.Fatalf("current branch: %v", err)
	}
	if strings.TrimSpace(currentBranch) != "stack/discovery-core" {
		t.Fatalf("expected to end on stack/discovery-core, got %q", currentBranch)
	}

	logOutput := strings.TrimSpace(testutil.Run(t, repo, "git", "log", "--format=%s", "--reverse", "main..stack/discovery-core"))
	if logOutput != "add feature a\nadd feature b" {
		t.Fatalf("unexpected composed commit order: %q", logOutput)
	}
}

func TestComposeRejectsNonContiguousExplicitBranches(t *testing.T) {
	repo := testutil.SetupGitRepo(t)

	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature a")

	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "feature/c")
	testutil.WriteFile(t, filepath.Join(repo, "feature-c.txt"), "feature c\n")
	testutil.Run(t, repo, "git", "add", "feature-c.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature c")

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
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
			"feature/c": {
				ParentBranch: "main",
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
		},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	err = executeCommandExpectError(runtime, "compose", "discovery-core", "--branches", "feature/a", "--branches", "feature/c", "--yes")
	if err == nil || !strings.Contains(err.Error(), "contiguous parent chain") {
		t.Fatalf("expected contiguous-chain error, got %v", err)
	}
}

func TestComposeOpenPRCreatesLandingPRAndStoresTickets(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testutil.Run(t, repo, "git", "init", "--bare", remote)
	testutil.Run(t, repo, "git", "remote", "add", "origin", remote)
	testutil.Run(t, repo, "git", "push", "-u", "origin", "main")

	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature a")
	featureAHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/a"))

	testutil.Run(t, repo, "git", "switch", "-c", "feature/b")
	testutil.WriteFile(t, filepath.Join(repo, "feature-b.txt"), "feature b\n")
	testutil.Run(t, repo, "git", "add", "feature-b.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature b")

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

	ghStub := testutil.SetupGHStub(t, "hack-dance/stack", "main")
	t.Setenv("STACK_TEST_GH_STATE", ghStub.StatePath)
	t.Setenv("STACK_TEST_GH_LOG", ghStub.LogPath)
	t.Setenv("PATH", ghStub.Dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	output := executeCommand(t, runtime, "compose", "discovery-core", "--from", "feature/a", "--to", "feature/b", "--ticket", "lnhack-66", "--ticket", "LNHACK-67", "--open-pr", "--yes")
	if !strings.Contains(output, "landing PR: #1") {
		t.Fatalf("expected compose output to mention landing PR, got %q", output)
	}
	if !strings.Contains(output, "tickets: LNHACK-66, LNHACK-67") {
		t.Fatalf("expected compose output to mention tickets, got %q", output)
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state after compose: %v", err)
	}
	landing, ok := state.Landings["stack/discovery-core"]
	if !ok {
		t.Fatalf("expected landing metadata to be recorded")
	}
	if got := strings.Join(landing.Tickets, ","); got != "LNHACK-66,LNHACK-67" {
		t.Fatalf("unexpected landing tickets %q", got)
	}
	if landing.LandingPRNumber != 1 {
		t.Fatalf("expected landing PR number 1, got %+v", landing)
	}
	if !runtime.Git.RemoteBranchExists(runtime.Context, "origin", "stack/discovery-core") {
		t.Fatalf("expected landing branch to be pushed to origin")
	}

	ghState := readFile(t, ghStub.StatePath)
	if !strings.Contains(ghState, `"headRefName": "stack/discovery-core"`) || !strings.Contains(ghState, `"title": "Landing: LNHACK-66, LNHACK-67"`) {
		t.Fatalf("expected landing PR to be created in gh state, got %q", ghState)
	}
}

func TestComposeOpenPRPersistsLandingMetadataBeforePROperations(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testutil.Run(t, repo, "git", "init", "--bare", remote)
	testutil.Run(t, repo, "git", "remote", "add", "origin", remote)
	testutil.Run(t, repo, "git", "push", "-u", "origin", "main")

	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature a")
	featureAHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/a"))

	testutil.Run(t, repo, "git", "switch", "-c", "feature/b")
	testutil.WriteFile(t, filepath.Join(repo, "feature-b.txt"), "feature b\n")
	testutil.Run(t, repo, "git", "add", "feature-b.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature b")

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

	failDir := t.TempDir()
	failGH := filepath.Join(failDir, "gh")
	testutil.WriteFile(t, failGH, "#!/bin/sh\necho \"forced gh failure\" >&2\nexit 1\n")
	testutil.Run(t, "", "chmod", "+x", failGH)
	t.Setenv("PATH", failDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err = executeCommandExpectError(runtime, "compose", "discovery-core", "--from", "feature/a", "--to", "feature/b", "--ticket", "LNHACK-66", "--open-pr", "--yes")
	if err == nil || !strings.Contains(err.Error(), "forced gh failure") {
		t.Fatalf("expected gh failure during compose --open-pr, got %v", err)
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state after failed compose: %v", err)
	}
	landing, ok := state.Landings["stack/discovery-core"]
	if !ok {
		t.Fatalf("expected landing metadata to be recorded before PR operations fail")
	}
	if got := strings.Join(landing.Tickets, ","); got != "LNHACK-66" {
		t.Fatalf("unexpected landing tickets %q", got)
	}
	if landing.LandingPRNumber != 0 {
		t.Fatalf("expected landing PR number to remain unset after failure, got %+v", landing)
	}
	if !runtime.Git.BranchExists(runtime.Context, "stack/discovery-core") {
		t.Fatalf("expected landing branch to exist locally after failed PR open")
	}
	if !runtime.Git.RemoteBranchExists(runtime.Context, "origin", "stack/discovery-core") {
		t.Fatalf("expected landing branch to be pushed before gh failure")
	}
}

func TestCloseoutClassifiesSupersededPRsAndDeployGatedTickets(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testutil.Run(t, repo, "git", "init", "--bare", remote)
	testutil.Run(t, repo, "git", "remote", "add", "origin", remote)
	testutil.Run(t, repo, "git", "push", "-u", "origin", "main")

	testutil.Run(t, repo, "git", "switch", "-c", "hack-agent/lnhack-66-feature-a")
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "a\n")
	testutil.Run(t, repo, "git", "add", "a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add a")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "hack-agent/lnhack-66-feature-a")
	featureAHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	testutil.Run(t, repo, "git", "switch", "-c", "hack-agent/lnhack-67-feature-b")
	testutil.WriteFile(t, filepath.Join(repo, "b.txt"), "b\n")
	testutil.Run(t, repo, "git", "add", "b.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add b")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "hack-agent/lnhack-67-feature-b")
	featureBHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing")
	landingHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

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
			"hack-agent/lnhack-66-feature-a": {
				ParentBranch: "main",
				PR: store.PullRequest{
					Number:          1,
					HeadRefName:     "hack-agent/lnhack-66-feature-a",
					BaseRefName:     "main",
					LastSeenHeadOID: featureAHead,
					State:           "OPEN",
				},
				Restack: store.RestackMetadata{LastParentHeadOID: mainHead},
			},
			"hack-agent/lnhack-67-feature-b": {
				ParentBranch: "hack-agent/lnhack-66-feature-a",
				PR: store.PullRequest{
					Number:          2,
					HeadRefName:     "hack-agent/lnhack-67-feature-b",
					BaseRefName:     "hack-agent/lnhack-66-feature-a",
					LastSeenHeadOID: featureBHead,
					State:           "OPEN",
				},
				Restack: store.RestackMetadata{LastParentHeadOID: featureAHead},
			},
		},
		Landings: map[string]store.LandingRecord{
			"stack/discovery-core": {
				BaseBranch:     "main",
				SourceBranches: []string{"hack-agent/lnhack-66-feature-a", "hack-agent/lnhack-67-feature-b"},
				Tickets:        []string{"LNHACK-66", "LNHACK-67"},
				CreatedAt:      "2026-03-26T18:30:00Z",
			},
		},
		Verifications: map[string][]store.VerificationRecord{
			"stack/discovery-core": {
				{
					CheckType:  "deploy",
					Identifier: "deploy-1",
					Passed:     true,
					HeadOID:    landingHead,
					RecordedAt: "2026-03-26T18:31:00Z",
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
      "headRefName": "hack-agent/lnhack-66-feature-a",
      "baseRefName": "main",
      "headRefOid": "`+featureAHead+`",
      "state": "OPEN",
      "isDraft": false
    },
    "2": {
      "id": "PR_2",
      "number": 2,
      "url": "https://example.com/hack-dance/stack/pull/2",
      "repo": "hack-dance/stack",
      "headRefName": "hack-agent/lnhack-67-feature-b",
      "baseRefName": "hack-agent/lnhack-66-feature-a",
      "headRefOid": "`+featureBHead+`",
      "state": "OPEN",
      "isDraft": false
    },
    "9": {
      "id": "PR_9",
      "number": 9,
      "url": "https://example.com/hack-dance/stack/pull/9",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+landingHead+`",
      "state": "MERGED",
      "isDraft": false
    }
  },
  "next_number": 10
}`)

	output := executeCommand(t, runtime, "closeout", "stack/discovery-core")
	if !strings.Contains(output, "landing PR: #9 MERGED") {
		t.Fatalf("expected merged landing PR in closeout output, got %q", output)
	}
	if !strings.Contains(output, "#1 hack-agent/lnhack-66-feature-a") || !strings.Contains(output, "#2 hack-agent/lnhack-67-feature-b") {
		t.Fatalf("expected superseded PRs in closeout output, got %q", output)
	}
	if !strings.Contains(output, "LNHACK-66") || !strings.Contains(output, "LNHACK-67") {
		t.Fatalf("expected explicit ticket refs in closeout output, got %q", output)
	}
	if strings.Contains(output, "record a passed deploy or smoke verification") {
		t.Fatalf("expected deploy follow-up to be cleared, got %q", output)
	}
}

func TestCloseoutApplyClosesSupersededPRsWhenOptedIn(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "a\n")
	testutil.Run(t, repo, "git", "add", "a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add a")
	featureAHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing")
	landingHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

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
				PR: store.PullRequest{
					Number:          1,
					HeadRefName:     "feature/a",
					BaseRefName:     "main",
					LastSeenHeadOID: featureAHead,
					State:           "OPEN",
				},
				Restack: store.RestackMetadata{LastParentHeadOID: mainHead},
			},
		},
		Landings: map[string]store.LandingRecord{
			"stack/discovery-core": {
				BaseBranch:                "main",
				SourceBranches:            []string{"feature/a"},
				Tickets:                   []string{"LNHACK-66"},
				LandingPRNumber:           9,
				SupersededPRs:             []int{1},
				CloseSupersededAfterMerge: true,
				CreatedAt:                 "2026-03-26T18:35:00Z",
			},
		},
		Verifications: map[string][]store.VerificationRecord{
			"stack/discovery-core": {
				{
					CheckType:  "deploy",
					Identifier: "deploy-1",
					Passed:     true,
					HeadOID:    landingHead,
					RecordedAt: "2026-03-26T18:36:00Z",
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
      "headRefOid": "`+featureAHead+`",
      "state": "OPEN",
      "isDraft": false
    },
    "9": {
      "id": "PR_9",
      "number": 9,
      "url": "https://example.com/hack-dance/stack/pull/9",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+landingHead+`",
      "state": "MERGED",
      "isDraft": false
    }
  },
  "next_number": 10
}`)

	output := executeCommand(t, runtime, "closeout", "stack/discovery-core", "--apply", "--yes")
	if !strings.Contains(output, "closed superseded PRs: #1 feature/a") {
		t.Fatalf("expected closeout apply output to mention closed PR, got %q", output)
	}

	log := readFile(t, ghStub.LogPath)
	if !strings.Contains(log, "pr close 1 --comment Closing as superseded by merged landing PR #9.") {
		t.Fatalf("expected GitHub close log, got %q", log)
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state after closeout apply: %v", err)
	}
	if state.Branches["feature/a"].PR.State != "CLOSED" {
		t.Fatalf("expected tracked PR state to be updated locally, got %+v", state.Branches["feature/a"].PR)
	}
}

func TestCloseoutRequiresExplicitResolutionForAmbiguousLandingPRs(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing")
	landingHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	runtime := newTestRuntime(repo)
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches:      map[string]store.BranchRecord{},
		Landings: map[string]store.LandingRecord{
			"stack/discovery-core": {
				BaseBranch:     "main",
				SourceBranches: []string{},
				CreatedAt:      "2026-03-26T18:40:00Z",
			},
		},
		Verifications: map[string][]store.VerificationRecord{
			"stack/discovery-core": {
				{
					CheckType:  "manual",
					Identifier: "check",
					Passed:     true,
					HeadOID:    landingHead,
					RecordedAt: "2026-03-26T18:41:00Z",
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
    "9": {
      "id": "PR_9",
      "number": 9,
      "url": "https://example.com/hack-dance/stack/pull/9",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+landingHead+`",
      "state": "OPEN",
      "isDraft": false
    },
    "10": {
      "id": "PR_10",
      "number": 10,
      "url": "https://example.com/hack-dance/stack/pull/10",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+landingHead+`",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 11
}`)

	output := executeCommand(t, runtime, "closeout", "stack/discovery-core")
	if !strings.Contains(output, "landing PR: ambiguous") || !strings.Contains(output, "resolve ambiguous landing PR ownership") {
		t.Fatalf("expected ambiguous landing PR guidance, got %q", output)
	}
}

func TestCloseoutSurfacesTrackedSourcePRRefreshFailures(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature a")

	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing")
	landingHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	runtime := newTestRuntime(repo)
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
					LastSeenHeadOID: strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "feature/a")),
					State:           "OPEN",
				},
			},
		},
		Landings: map[string]store.LandingRecord{
			"stack/discovery-core": {
				BaseBranch:     "main",
				SourceBranches: []string{"feature/a"},
				CreatedAt:      "2026-03-26T18:40:00Z",
			},
		},
		Verifications: map[string][]store.VerificationRecord{
			"stack/discovery-core": {
				{
					CheckType:  "manual",
					Identifier: "check",
					Passed:     true,
					HeadOID:    landingHead,
					RecordedAt: "2026-03-26T18:41:00Z",
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
    "9": {
      "id": "PR_9",
      "number": 9,
      "url": "https://example.com/hack-dance/stack/pull/9",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+landingHead+`",
      "state": "MERGED",
      "isDraft": false
    }
  },
  "next_number": 10
}`)

	output := executeCommand(t, runtime, "closeout", "stack/discovery-core")
	if !strings.Contains(output, `source PR for feature/a could not be refreshed: tracked PR #1 for "feature/a" could not be loaded`) {
		t.Fatalf("expected source PR refresh failure guidance, got %q", output)
	}
}

func TestSupersedeRecordsMetadataAndCommentsOriginalPRs(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing")
	landingHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	runtime := newTestRuntime(repo)
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches:      map[string]store.BranchRecord{},
		Landings: map[string]store.LandingRecord{
			"stack/discovery-core": {
				BaseBranch:     "main",
				SourceBranches: []string{"hack-agent/lnhack-66-feature-a", "hack-agent/lnhack-67-feature-b"},
				CreatedAt:      "2026-03-26T19:00:00Z",
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
      "headRefName": "hack-agent/lnhack-66-feature-a",
      "baseRefName": "main",
      "headRefOid": "",
      "state": "OPEN",
      "isDraft": false
    },
    "2": {
      "id": "PR_2",
      "number": 2,
      "url": "https://example.com/hack-dance/stack/pull/2",
      "repo": "hack-dance/stack",
      "headRefName": "hack-agent/lnhack-67-feature-b",
      "baseRefName": "hack-agent/lnhack-66-feature-a",
      "headRefOid": "",
      "state": "OPEN",
      "isDraft": false
    },
    "9": {
      "id": "PR_9",
      "number": 9,
      "url": "https://example.com/hack-dance/stack/pull/9",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+landingHead+`",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 10
}`)

	output := executeCommand(t, runtime, "supersede", "--landing", "stack/discovery-core", "--prs", "1,2", "--close-after-merge", "--yes")
	if !strings.Contains(output, "github comments: posted") {
		t.Fatalf("expected supersede output to mention posted comments, got %q", output)
	}

	state, err := runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	landing, ok := state.Landings["stack/discovery-core"]
	if !ok {
		t.Fatalf("expected landing metadata to remain present")
	}
	if got := strings.TrimSpace(joinPRNumbers(landing.SupersededPRs)); got != "#1, #2" {
		t.Fatalf("expected superseded PR metadata to be recorded, got %q", got)
	}
	if !landing.CloseSupersededAfterMerge {
		t.Fatalf("expected close-after-merge metadata to be recorded")
	}
	if landing.LandingPRNumber != 9 {
		t.Fatalf("expected landing PR number to be recorded, got %+v", landing)
	}

	log := readFile(t, ghStub.LogPath)
	if !strings.Contains(log, "pr comment 1 --body") || !strings.Contains(log, "pr comment 2 --body") {
		t.Fatalf("expected GitHub comments on both original PRs, got %q", log)
	}

	ghState := readFile(t, ghStub.StatePath)
	if !strings.Contains(ghState, "superseded by landing PR #9") {
		t.Fatalf("expected landing PR reference in gh comment state, got %q", ghState)
	}
}

func TestSupersedeRejectsPRsOutsideLandingSourceBranches(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing")
	landingHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	runtime := newTestRuntime(repo)
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches:      map[string]store.BranchRecord{},
		Landings: map[string]store.LandingRecord{
			"stack/discovery-core": {
				BaseBranch:     "main",
				SourceBranches: []string{"hack-agent/lnhack-66-feature-a", "hack-agent/lnhack-67-feature-b"},
				CreatedAt:      "2026-03-26T19:00:00Z",
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
    "3": {
      "id": "PR_3",
      "number": 3,
      "url": "https://example.com/hack-dance/stack/pull/3",
      "repo": "hack-dance/stack",
      "headRefName": "hack-agent/unrelated-follow-up",
      "baseRefName": "main",
      "headRefOid": "",
      "state": "OPEN",
      "isDraft": false
    },
    "9": {
      "id": "PR_9",
      "number": 9,
      "url": "https://example.com/hack-dance/stack/pull/9",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+landingHead+`",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 10
}`)

	err := executeCommandExpectError(runtime, "supersede", "--landing", "stack/discovery-core", "--prs", "3", "--no-comment", "--yes")
	if err == nil || !strings.Contains(err.Error(), `pull request #3 head "hack-agent/unrelated-follow-up" is not part of landing batch "stack/discovery-core"`) {
		t.Fatalf("expected supersede source-branch validation error, got %v", err)
	}
}

func TestVerifyAddAndListForTrackedBranch(t *testing.T) {
	repo := testutil.SetupGitRepo(t)

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
				Restack: store.RestackMetadata{
					LastParentHeadOID: mainHead,
				},
			},
		},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	output := executeCommand(t, runtime, "verify", "add", "feature/a", "--type", "sim", "--run-id", "run-123", "--passed", "--score", "100", "--note", "festival smoke")
	if !strings.Contains(output, "Verification recorded") {
		t.Fatalf("expected verification add output, got %q", output)
	}

	state, err = runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	records := state.Verifications["feature/a"]
	if len(records) != 1 {
		t.Fatalf("expected 1 verification record, got %+v", records)
	}
	if records[0].CheckType != "sim" {
		t.Fatalf("expected sim verification type, got %+v", records[0])
	}
	if records[0].Identifier != "run-123" {
		t.Fatalf("expected run-123 identifier, got %+v", records[0])
	}
	if !records[0].Passed {
		t.Fatalf("expected verification to be marked passed")
	}
	if records[0].Score == nil || *records[0].Score != 100 {
		t.Fatalf("expected score 100, got %+v", records[0].Score)
	}

	listOutput := executeCommand(t, runtime, "verify", "list", "feature/a")
	if !strings.Contains(listOutput, "run-123") || !strings.Contains(listOutput, "festival smoke") {
		t.Fatalf("expected verification list output, got %q", listOutput)
	}
}

func TestVerifyAddWorksForUntrackedLandingBranch(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing branch\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing branch")

	runtime := newTestRuntime(repo)
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches:      map[string]store.BranchRecord{},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	executeCommand(t, runtime, "verify", "add", "stack/discovery-core", "--type", "manual", "--identifier", "deploy-check", "--failed", "--note", "safe to close after deploy")

	state, err := runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	records := state.Verifications["stack/discovery-core"]
	if len(records) != 1 {
		t.Fatalf("expected 1 landing verification record, got %+v", records)
	}
	if records[0].Passed {
		t.Fatalf("expected landing verification to be marked failed")
	}
	if records[0].Identifier != "deploy-check" {
		t.Fatalf("expected identifier deploy-check, got %+v", records[0])
	}
}

func TestVerifyAddRejectsConflictingOutcomeFlags(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	runtime := newTestRuntime(repo)
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches:      map[string]store.BranchRecord{},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	err := executeCommandExpectError(runtime, "verify", "add", "main", "--type", "manual", "--passed", "--failed")
	if err == nil || !strings.Contains(err.Error(), "exactly one of --passed or --failed") {
		t.Fatalf("expected conflicting verification flag error, got %v", err)
	}
}

func TestStatusShowsVerificationAndLandingBranchSections(t *testing.T) {
	repo := testutil.SetupGitRepo(t)

	testutil.Run(t, repo, "git", "switch", "-c", "feature/a")
	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "add feature a")
	verifiedHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	testutil.WriteFile(t, filepath.Join(repo, "feature-a.txt"), "feature a changed\n")
	testutil.Run(t, repo, "git", "add", "feature-a.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "advance feature a")

	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing branch")
	landingHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	runtime := newTestRuntime(repo)
	mainHead, err := runtime.Git.ResolveRef(runtime.Context, "main")
	if err != nil {
		t.Fatalf("resolve main head: %v", err)
	}
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
					RecordedAt: "2026-03-26T18:00:00Z",
					Score:      &score,
				},
			},
			"stack/discovery-core": {
				{
					CheckType:  "manual",
					Identifier: "deploy-check",
					Passed:     false,
					HeadOID:    landingHead,
					RecordedAt: "2026-03-26T18:05:00Z",
				},
			},
		},
	}
	if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	output := executeCommand(t, runtime, "status")
	if !strings.Contains(output, "Verify: sim") || !strings.Contains(output, "run-123") || !strings.Contains(output, "stale") {
		t.Fatalf("expected tracked verification details in status output, got %q", output)
	}
	if !strings.Contains(output, "Landing Branches") || !strings.Contains(output, "stack/discovery-core") || !strings.Contains(output, "deploy-check") {
		t.Fatalf("expected landing branch section in status output, got %q", output)
	}
}

func TestVersionCommandPrintsBuildInfo(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	runtime := newTestRuntime(repo)

	output := executeCommand(t, runtime, "version")
	if !strings.Contains(output, "dev") {
		t.Fatalf("expected dev version output, got %q", output)
	}
	if !strings.Contains(output, "commit") {
		t.Fatalf("expected commit metadata, got %q", output)
	}
}

func TestInitFallsBackToRemoteRepoWhenGHRepoViewFails(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	testutil.Run(t, repo, "git", "remote", "add", "origin", "git@github.com-ln:acme/new-repo.git")
	runtime := newTestRuntime(repo)

	ghStubDir := t.TempDir()
	ghStubPath := filepath.Join(ghStubDir, "gh")
	if err := os.WriteFile(ghStubPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write gh stub: %v", err)
	}
	t.Setenv("PATH", ghStubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	output := executeCommand(t, runtime, "init", "--remote", "origin", "--trunk", "main")
	if !strings.Contains(output, "repo: acme/new-repo") {
		t.Fatalf("expected init output to include remote repo slug, got %q", output)
	}

	state, err := runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state.Repo != "acme/new-repo" {
		t.Fatalf("expected state repo acme/new-repo, got %q", state.Repo)
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
	expected := "pr merge 1 --auto --merge --match-head-commit " + head
	if !strings.Contains(log, expected) {
		t.Fatalf("expected gh merge log %q, got %q", expected, log)
	}
}

func TestQueueAllowsConfiguredMergeStrategy(t *testing.T) {
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
      "headRefOid": "`+head+`",
      "baseRefOid": "",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 2
}`)

	executeCommand(t, runtime, "queue", "feature/a", "--strategy", "squash", "--yes")

	log := readFile(t, ghStub.LogPath)
	expected := "pr merge 1 --auto --squash --match-head-commit " + head
	if !strings.Contains(log, expected) {
		t.Fatalf("expected gh merge log %q, got %q", expected, log)
	}
}

func TestQueuePrintsNextStepsForDownstackBranch(t *testing.T) {
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
      "headRefOid": "`+featureAHead+`",
      "baseRefOid": "",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 2
}`)

	output := executeCommand(t, runtime, "queue", "feature/a", "--yes")
	if !strings.Contains(output, "wait for GitHub to merge PR #1") {
		t.Fatalf("expected merge guidance, got %q", output)
	}
	if !strings.Contains(output, "then run: stack submit feature/b") {
		t.Fatalf("expected child submit guidance, got %q", output)
	}
	if !strings.Contains(output, "then run: stack queue feature/b") {
		t.Fatalf("expected child queue guidance, got %q", output)
	}

	_ = repo
}

func TestQueueRejectsSourceBranchWhenLandingPRExists(t *testing.T) {
	repo, runtime, featureAHead := setupTrackedStackRepo(t)

	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "stack/discovery-core")
	landingHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	state, err := runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	state.Landings = map[string]store.LandingRecord{
		"stack/discovery-core": {
			BaseBranch:     "main",
			SourceBranches: []string{"feature/a", "feature/b"},
			CreatedAt:      "2026-03-26T20:00:00Z",
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
      "headRefOid": "`+featureAHead+`",
      "baseRefOid": "",
      "state": "OPEN",
      "isDraft": false
    },
    "9": {
      "id": "PR_9",
      "number": 9,
      "url": "https://example.com/hack-dance/stack/pull/9",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+landingHead+`",
      "baseRefOid": "",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 10
}`)

	err = executeCommandExpectError(runtime, "queue", "feature/a", "--yes")
	if err == nil || !strings.Contains(err.Error(), "queue landing PR #9 instead") || !strings.Contains(err.Error(), "keep source PRs out of the merge queue") {
		t.Fatalf("expected landing queue guidance error, got %v", err)
	}
}

func TestQueueAllowsLandingBranchAndPrintsCloseoutGuidance(t *testing.T) {
	repo, runtime, _ := setupTrackedStackRepo(t)

	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "stack/discovery-core")
	landingHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	state, err := runtime.Store.ReadState(runtime.Context)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	state.Landings = map[string]store.LandingRecord{
		"stack/discovery-core": {
			BaseBranch:     "main",
			SourceBranches: []string{"feature/a", "feature/b"},
			SupersededPRs:  []int{1, 2},
			CreatedAt:      "2026-03-26T20:10:00Z",
		},
	}
	state.Verifications = map[string][]store.VerificationRecord{
		"stack/discovery-core": {
			{
				CheckType:  "manual",
				Identifier: "discovery-batch",
				Passed:     true,
				HeadOID:    landingHead,
				RecordedAt: "2026-03-26T20:11:00Z",
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
    "9": {
      "id": "PR_9",
      "number": 9,
      "url": "https://example.com/hack-dance/stack/pull/9",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+landingHead+`",
      "baseRefOid": "",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 10
}`)

	output := executeCommand(t, runtime, "queue", "stack/discovery-core", "--yes")
	if !strings.Contains(output, "keep out of queue: #1, #2") {
		t.Fatalf("expected source PR exclusion guidance, got %q", output)
	}
	if !strings.Contains(output, "verification: manual passed discovery-batch") {
		t.Fatalf("expected verification summary in queue output, got %q", output)
	}
	if !strings.Contains(output, "then run: stack closeout stack/discovery-core") {
		t.Fatalf("expected closeout next-step guidance, got %q", output)
	}

	log := readFile(t, ghStub.LogPath)
	expected := "pr merge 9 --auto --merge --match-head-commit " + landingHead
	if !strings.Contains(log, expected) {
		t.Fatalf("expected gh merge log %q, got %q", expected, log)
	}
}

func TestQueueLandingBranchPrefersUniqueOpenPRWhenHistoryExists(t *testing.T) {
	repo, runtime, _ := setupTrackedStackRepo(t)
	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "stack/discovery-core")
	landingHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches:      map[string]store.BranchRecord{},
		Landings: map[string]store.LandingRecord{
			"stack/discovery-core": {
				BaseBranch:     "main",
				SourceBranches: []string{},
				CreatedAt:      "2026-03-26T18:40:00Z",
			},
		},
		Verifications: map[string][]store.VerificationRecord{
			"stack/discovery-core": {
				{
					CheckType:  "manual",
					Identifier: "check",
					Passed:     true,
					HeadOID:    landingHead,
					RecordedAt: "2026-03-26T18:41:00Z",
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
    "9": {
      "id": "PR_9",
      "number": 9,
      "url": "https://example.com/hack-dance/stack/pull/9",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+landingHead+`",
      "state": "OPEN",
      "isDraft": false
    },
    "10": {
      "id": "PR_10",
      "number": 10,
      "url": "https://example.com/hack-dance/stack/pull/10",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+landingHead+`",
      "state": "MERGED",
      "isDraft": false
    }
  },
  "next_number": 11
}`)

	output := executeCommand(t, runtime, "queue", "stack/discovery-core", "--yes")
	if !strings.Contains(output, "PR #9") {
		t.Fatalf("expected queue output to use the unique open landing PR, got %q", output)
	}

	log := readFile(t, ghStub.LogPath)
	if !strings.Contains(log, "pr merge 9 --auto --merge --match-head-commit "+landingHead) {
		t.Fatalf("expected queue to merge open landing PR #9, got %q", log)
	}
}

func TestQueueLandingBranchIgnoresStaleRecordedClosedLandingPRWhenReplacementExists(t *testing.T) {
	repo, runtime, _ := setupTrackedStackRepo(t)
	testutil.Run(t, repo, "git", "switch", "main")
	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "stack/discovery-core")
	landingHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches:      map[string]store.BranchRecord{},
		Landings: map[string]store.LandingRecord{
			"stack/discovery-core": {
				BaseBranch:     "main",
				SourceBranches: []string{},
				LandingPRNumber: 8,
				CreatedAt:      "2026-03-26T18:40:00Z",
			},
		},
		Verifications: map[string][]store.VerificationRecord{
			"stack/discovery-core": {
				{
					CheckType:  "manual",
					Identifier: "check",
					Passed:     true,
					HeadOID:    landingHead,
					RecordedAt: "2026-03-26T18:41:00Z",
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
    "8": {
      "id": "PR_8",
      "number": 8,
      "url": "https://example.com/hack-dance/stack/pull/8",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+landingHead+`",
      "state": "CLOSED",
      "isDraft": false
    },
    "9": {
      "id": "PR_9",
      "number": 9,
      "url": "https://example.com/hack-dance/stack/pull/9",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+landingHead+`",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 10
}`)

	output := executeCommand(t, runtime, "queue", "stack/discovery-core", "--yes")
	if !strings.Contains(output, "PR #9") {
		t.Fatalf("expected queue output to use replacement open landing PR, got %q", output)
	}

	log := readFile(t, ghStub.LogPath)
	if !strings.Contains(log, "pr merge 9 --auto --merge --match-head-commit "+landingHead) {
		t.Fatalf("expected queue to merge replacement open landing PR #9, got %q", log)
	}
}

func TestParseTicketRefsRejectsPartialMatches(t *testing.T) {
	tests := []string{
		"ABC-123/extra",
		"prefix/ABC-1",
		"ABC-123 extra",
	}
	for _, value := range tests {
		tickets, err := parseTicketRefs([]string{value})
		if err == nil {
			t.Fatalf("expected parseTicketRefs to reject %q, got tickets %+v", value, tickets)
		}
	}
}

func TestQueueRejectsLandingBranchWithStaleVerification(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testutil.Run(t, repo, "git", "init", "--bare", remote)
	testutil.Run(t, repo, "git", "remote", "add", "origin", remote)
	testutil.Run(t, repo, "git", "push", "-u", "origin", "main")

	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing one\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing one")
	verifiedHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing two\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing two")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "stack/discovery-core")
	currentHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	runtime := newTestRuntime(repo)
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches:      map[string]store.BranchRecord{},
		Landings: map[string]store.LandingRecord{
			"stack/discovery-core": {
				BaseBranch:     "main",
				SourceBranches: []string{"feature/a"},
				CreatedAt:      "2026-03-26T20:20:00Z",
			},
		},
		Verifications: map[string][]store.VerificationRecord{
			"stack/discovery-core": {
				{
					CheckType:  "sim",
					Identifier: "run-123",
					Passed:     true,
					HeadOID:    verifiedHead,
					RecordedAt: "2026-03-26T20:21:00Z",
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
    "9": {
      "id": "PR_9",
      "number": 9,
      "url": "https://example.com/hack-dance/stack/pull/9",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+currentHead+`",
      "baseRefOid": "",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 10
}`)

	err := executeCommandExpectError(runtime, "queue", "stack/discovery-core", "--yes")
	if err == nil || !strings.Contains(err.Error(), "head moved since the latest recorded verification") {
		t.Fatalf("expected stale verification error, got %v", err)
	}
}

func TestQueueRejectsLandingBranchWithFailedVerification(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testutil.Run(t, repo, "git", "init", "--bare", remote)
	testutil.Run(t, repo, "git", "remote", "add", "origin", remote)
	testutil.Run(t, repo, "git", "push", "-u", "origin", "main")

	testutil.Run(t, repo, "git", "switch", "-c", "stack/discovery-core")
	testutil.WriteFile(t, filepath.Join(repo, "landing.txt"), "landing\n")
	testutil.Run(t, repo, "git", "add", "landing.txt")
	testutil.Run(t, repo, "git", "commit", "-m", "landing")
	testutil.Run(t, repo, "git", "push", "-u", "origin", "stack/discovery-core")
	currentHead := strings.TrimSpace(testutil.Run(t, repo, "git", "rev-parse", "HEAD"))

	runtime := newTestRuntime(repo)
	state := store.RepoState{
		Version:       1,
		Repo:          "hack-dance/stack",
		DefaultRemote: "origin",
		Trunk:         "main",
		Branches:      map[string]store.BranchRecord{},
		Landings: map[string]store.LandingRecord{
			"stack/discovery-core": {
				BaseBranch:     "main",
				SourceBranches: []string{"feature/a"},
				CreatedAt:      "2026-03-26T20:30:00Z",
			},
		},
		Verifications: map[string][]store.VerificationRecord{
			"stack/discovery-core": {
				{
					CheckType:  "sim",
					Identifier: "run-456",
					Passed:     false,
					HeadOID:    currentHead,
					RecordedAt: "2026-03-26T20:31:00Z",
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
    "9": {
      "id": "PR_9",
      "number": 9,
      "url": "https://example.com/hack-dance/stack/pull/9",
      "repo": "hack-dance/stack",
      "headRefName": "stack/discovery-core",
      "baseRefName": "main",
      "headRefOid": "`+currentHead+`",
      "baseRefOid": "",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 10
}`)

	err := executeCommandExpectError(runtime, "queue", "stack/discovery-core", "--yes")
	if err == nil || !strings.Contains(err.Error(), "latest sim verification") {
		t.Fatalf("expected failed verification error, got %v", err)
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

func executeCommandExpectError(runtime *stackruntime.Runtime, args ...string) error {
	root := NewRootCommand(runtime)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	if err := root.ExecuteContext(context.Background()); err != nil {
		return err
	}
	return nil
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
				PR: store.PullRequest{
					Number:      2,
					HeadRefName: "feature/b",
					BaseRefName: "feature/a",
					State:       "OPEN",
				},
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
