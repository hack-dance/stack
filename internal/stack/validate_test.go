package stack_test

import (
	"testing"

	"github.com/hack-dance/stack/internal/stack"
	"github.com/hack-dance/stack/internal/store"
)

func TestValidateStateDetectsCyclesAndDuplicatePRs(t *testing.T) {
	t.Parallel()

	state := store.RepoState{
		Trunk:         "main",
		DefaultRemote: "origin",
		Branches: map[string]store.BranchRecord{
			"feature/a": {
				ParentBranch: "feature/b",
				PR:           store.PullRequest{Number: 10},
			},
			"feature/b": {
				ParentBranch: "feature/a",
				PR:           store.PullRequest{Number: 10},
			},
		},
	}

	validation := stack.ValidateState(state)
	if len(validation.RepoIssues) == 0 {
		t.Fatalf("expected repository-level validation issues")
	}
	if !stack.HasErrors(validation.BranchIssues["feature/a"]) {
		t.Fatalf("expected feature/a to have validation errors")
	}
	if !stack.HasErrors(validation.BranchIssues["feature/b"]) {
		t.Fatalf("expected feature/b to have validation errors")
	}
}

func TestEnsureBranchCanParentRejectsCycles(t *testing.T) {
	t.Parallel()

	state := store.RepoState{
		Trunk: "main",
		Branches: map[string]store.BranchRecord{
			"feature/a": {ParentBranch: "main"},
			"feature/b": {ParentBranch: "feature/a"},
			"feature/c": {ParentBranch: "feature/b"},
		},
	}

	if err := stack.EnsureBranchCanParent(state, "feature/a", "feature/c"); err == nil {
		t.Fatalf("expected cycle detection when moving feature/a under feature/c")
	}
}
