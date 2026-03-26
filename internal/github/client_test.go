package github_test

import (
	"context"
	"os"
	"strings"
	"testing"

	stackgh "github.com/hack-dance/stack/internal/github"
	"github.com/hack-dance/stack/internal/testutil"
)

func TestViewPRMapsGitHubOIDsIntoCachedState(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	ghStub := testutil.SetupGHStub(t, "hack-dance/stack", "main")
	t.Setenv("STACK_TEST_GH_STATE", ghStub.StatePath)
	t.Setenv("STACK_TEST_GH_LOG", ghStub.LogPath)
	t.Setenv("PATH", ghStub.Dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	testutil.WriteFile(t, ghStub.StatePath, `{
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
      "headRefOid": "abc123",
      "baseRefOid": "def456",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 8
}`)

	client := stackgh.NewClient(repo)
	pr, err := client.ViewPR(context.Background(), 7)
	if err != nil {
		t.Fatalf("view pr: %v", err)
	}

	if pr.LastSeenHeadOID != "abc123" {
		t.Fatalf("expected head oid abc123, got %q", pr.LastSeenHeadOID)
	}
	if pr.LastSeenBaseOID != "def456" {
		t.Fatalf("expected base oid def456, got %q", pr.LastSeenBaseOID)
	}
}

func TestFindPRByHeadPrefersSingleOpenMatch(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	ghStub := testutil.SetupGHStub(t, "hack-dance/stack", "main")
	t.Setenv("STACK_TEST_GH_STATE", ghStub.StatePath)
	t.Setenv("STACK_TEST_GH_LOG", ghStub.LogPath)
	t.Setenv("PATH", ghStub.Dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	testutil.WriteFile(t, ghStub.StatePath, `{
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
      "headRefName": "feature/a",
      "baseRefName": "main",
      "state": "MERGED",
      "isDraft": false
    },
    "7": {
      "id": "PR_7",
      "number": 7,
      "url": "https://example.com/hack-dance/stack/pull/7",
      "repo": "hack-dance/stack",
      "headRefName": "feature/a",
      "baseRefName": "main",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 8
}`)

	client := stackgh.NewClient(repo)
	pr, err := client.FindPRByHead(context.Background(), "feature/a")
	if err != nil {
		t.Fatalf("find pr by head: %v", err)
	}
	if pr.Number != 7 {
		t.Fatalf("expected open PR #7, got %+v", pr)
	}
}

func TestFindPRByHeadIgnoresHistoricalMatchesWithoutOpenPR(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	ghStub := testutil.SetupGHStub(t, "hack-dance/stack", "main")
	t.Setenv("STACK_TEST_GH_STATE", ghStub.StatePath)
	t.Setenv("STACK_TEST_GH_LOG", ghStub.LogPath)
	t.Setenv("PATH", ghStub.Dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	testutil.WriteFile(t, ghStub.StatePath, `{
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
      "headRefName": "feature/a",
      "baseRefName": "main",
      "state": "MERGED",
      "isDraft": false
    },
    "4": {
      "id": "PR_4",
      "number": 4,
      "url": "https://example.com/hack-dance/stack/pull/4",
      "repo": "hack-dance/stack",
      "headRefName": "feature/a",
      "baseRefName": "main",
      "state": "CLOSED",
      "isDraft": false
    }
  },
  "next_number": 5
}`)

	client := stackgh.NewClient(repo)
	pr, err := client.FindPRByHead(context.Background(), "feature/a")
	if err != nil {
		t.Fatalf("find pr by head: %v", err)
	}
	if pr.Number != 0 {
		t.Fatalf("expected no live PR match, got %+v", pr)
	}
}

func TestFindPRByHeadRejectsAmbiguousOpenMatches(t *testing.T) {
	repo := testutil.SetupGitRepo(t)
	ghStub := testutil.SetupGHStub(t, "hack-dance/stack", "main")
	t.Setenv("STACK_TEST_GH_STATE", ghStub.StatePath)
	t.Setenv("STACK_TEST_GH_LOG", ghStub.LogPath)
	t.Setenv("PATH", ghStub.Dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	testutil.WriteFile(t, ghStub.StatePath, `{
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
      "state": "OPEN",
      "isDraft": false
    },
    "8": {
      "id": "PR_8",
      "number": 8,
      "url": "https://example.com/hack-dance/stack/pull/8",
      "repo": "hack-dance/stack",
      "headRefName": "feature/a",
      "baseRefName": "feature/base",
      "state": "OPEN",
      "isDraft": false
    }
  },
  "next_number": 9
}`)

	client := stackgh.NewClient(repo)
	_, err := client.FindPRByHead(context.Background(), "feature/a")
	if err == nil || !strings.Contains(err.Error(), "multiple open pull requests match head") {
		t.Fatalf("expected ambiguous open-pr error, got %v", err)
	}
}
