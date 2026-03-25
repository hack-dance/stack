package stack

import (
	"context"
	"fmt"
	"sort"
	"strings"

	stackgit "github.com/hack-dance/stack/internal/git"
	"github.com/hack-dance/stack/internal/store"
)

type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

type HealthIssue struct {
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

type BranchSummary struct {
	Name            string             `json:"name"`
	Parent          string             `json:"parent"`
	Depth           int                `json:"depth"`
	CurrentHeadOID  string             `json:"currentHeadOid,omitempty"`
	ParentHeadOID   string             `json:"parentHeadOid,omitempty"`
	RemoteExists    bool               `json:"remoteExists"`
	ParentAncestor  bool               `json:"parentAncestor"`
	LocalExists     bool               `json:"localExists"`
	ParentExists    bool               `json:"parentExists"`
	IsCurrentBranch bool               `json:"isCurrentBranch"`
	Issues          []HealthIssue      `json:"issues"`
	Record          store.BranchRecord `json:"record"`
}

type Summary struct {
	RepoRoot       string          `json:"repoRoot"`
	Trunk          string          `json:"trunk"`
	DefaultRemote  string          `json:"defaultRemote"`
	CurrentBranch  string          `json:"currentBranch,omitempty"`
	RepoIssues     []HealthIssue   `json:"repoIssues,omitempty"`
	UntrackedHeads []string        `json:"untrackedHeads,omitempty"`
	Branches       []BranchSummary `json:"branches"`
}

func BuildSummary(ctx context.Context, git *stackgit.Client, state store.RepoState) (Summary, error) {
	paths, err := git.RepoPaths(ctx)
	if err != nil {
		return Summary{}, err
	}

	currentBranch, _ := git.CurrentBranch(ctx)
	validation := ValidateState(state)
	branches := make([]BranchSummary, 0, len(state.Branches))

	for _, branchName := range topoOrder(state) {
		record := state.Branches[branchName]
		summary := BranchSummary{
			Name:            branchName,
			Parent:          record.ParentBranch,
			Depth:           depthFor(state, branchName),
			LocalExists:     git.BranchExists(ctx, branchName),
			ParentExists:    record.ParentBranch == state.Trunk || git.BranchExists(ctx, record.ParentBranch),
			RemoteExists:    git.RemoteBranchExists(ctx, state.DefaultRemote, branchName),
			IsCurrentBranch: strings.TrimSpace(currentBranch) == branchName,
			Record:          record,
		}

		if summary.LocalExists {
			headOID, err := git.ResolveRef(ctx, branchName)
			if err == nil {
				summary.CurrentHeadOID = headOID
			}
		}

		if record.ParentBranch == state.Trunk || summary.ParentExists {
			parentOID, err := git.ResolveRef(ctx, record.ParentBranch)
			if err == nil {
				summary.ParentHeadOID = parentOID
			}
		}

		summary.Issues = append(summary.Issues, validation.BranchIssues[branchName]...)

		if !summary.LocalExists {
			summary.Issues = append(summary.Issues, HealthIssue{
				Severity: SeverityError,
				Message:  "local branch is missing",
			})
		}

		if !summary.ParentExists {
			summary.Issues = append(summary.Issues, HealthIssue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("parent branch %q is missing", record.ParentBranch),
			})
		}

		if summary.LocalExists && summary.ParentExists {
			isAncestor, err := git.IsAncestor(ctx, record.ParentBranch, branchName)
			if err == nil {
				summary.ParentAncestor = isAncestor
				if !isAncestor {
					summary.Issues = append(summary.Issues, HealthIssue{
						Severity: SeverityWarn,
						Message:  "branch is not currently based on parent",
					})
				}
			}
		}

		if record.Restack.LastParentHeadOID == "" {
			summary.Issues = append(summary.Issues, HealthIssue{
				Severity: SeverityWarn,
				Message:  "missing restack anchor",
			})
		} else if summary.ParentHeadOID != "" && record.Restack.LastParentHeadOID != summary.ParentHeadOID {
			summary.Issues = append(summary.Issues, HealthIssue{
				Severity: SeverityInfo,
				Message:  "parent moved since last recorded restack",
			})
		}

		if !summary.RemoteExists {
			summary.Issues = append(summary.Issues, HealthIssue{
				Severity: SeverityInfo,
				Message:  "remote branch has not been pushed yet",
			})
		}

		if record.PR.Number > 0 {
			if record.PR.State == "CLOSED" {
				summary.Issues = append(summary.Issues, HealthIssue{
					Severity: SeverityWarn,
					Message:  "tracked PR is closed and needs repair or relink",
				})
			}
			if record.PR.State == "MERGED" {
				summary.Issues = append(summary.Issues, HealthIssue{
					Severity: SeverityInfo,
					Message:  "tracked PR is merged; descendants may need sync",
				})
			}
			if record.PR.BaseRefName != "" && record.PR.BaseRefName != record.ParentBranch {
				summary.Issues = append(summary.Issues, HealthIssue{
					Severity: SeverityWarn,
					Message:  fmt.Sprintf("cached PR base is %q, expected %q", record.PR.BaseRefName, record.ParentBranch),
				})
			}
			if record.PR.HeadRefName != "" && record.PR.HeadRefName != branchName {
				summary.Issues = append(summary.Issues, HealthIssue{
					Severity: SeverityWarn,
					Message:  fmt.Sprintf("cached PR head is %q, expected %q", record.PR.HeadRefName, branchName),
				})
			}
			if summary.CurrentHeadOID != "" && record.PR.LastSeenHeadOID != "" && record.PR.LastSeenHeadOID != summary.CurrentHeadOID {
				summary.Issues = append(summary.Issues, HealthIssue{
					Severity: SeverityInfo,
					Message:  "local branch head differs from last synced PR head",
				})
			}
		}

		branches = append(branches, summary)
	}

	return Summary{
		RepoRoot:      paths.Root,
		Trunk:         state.Trunk,
		DefaultRemote: state.DefaultRemote,
		CurrentBranch: strings.TrimSpace(currentBranch),
		RepoIssues:    validation.RepoIssues,
		Branches:      branches,
	}, nil
}

func topoOrder(state store.RepoState) []string {
	children := map[string][]string{}
	roots := make([]string, 0)

	for branchName, record := range state.Branches {
		children[record.ParentBranch] = append(children[record.ParentBranch], branchName)
		if record.ParentBranch == state.Trunk {
			roots = append(roots, branchName)
		}
	}

	sort.Strings(roots)
	for parent := range children {
		sort.Strings(children[parent])
	}

	ordered := make([]string, 0, len(state.Branches))
	visited := map[string]bool{}

	var walk func(string)
	walk = func(parent string) {
		for _, child := range children[parent] {
			if visited[child] {
				continue
			}
			visited[child] = true
			ordered = append(ordered, child)
			walk(child)
		}
	}

	walk(state.Trunk)

	remaining := make([]string, 0)
	for branchName := range state.Branches {
		if !visited[branchName] {
			remaining = append(remaining, branchName)
		}
	}
	sort.Strings(remaining)
	ordered = append(ordered, remaining...)

	return ordered
}

func Children(state store.RepoState, parent string) []string {
	children := make([]string, 0)
	for branchName, record := range state.Branches {
		if record.ParentBranch == parent {
			children = append(children, branchName)
		}
	}
	sort.Strings(children)
	return children
}

func depthFor(state store.RepoState, branch string) int {
	depth := 0
	current := branch
	seen := map[string]bool{}

	for {
		record, ok := state.Branches[current]
		if !ok {
			return depth
		}
		if record.ParentBranch == state.Trunk {
			return depth
		}
		if seen[current] {
			return depth
		}
		seen[current] = true
		depth += 1
		current = record.ParentBranch
	}
}
