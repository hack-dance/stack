package github_test

import (
	"context"
	"os"
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
