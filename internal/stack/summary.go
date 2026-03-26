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
	Name            string               `json:"name"`
	Parent          string               `json:"parent"`
	Depth           int                  `json:"depth"`
	CurrentHeadOID  string               `json:"currentHeadOid,omitempty"`
	ParentHeadOID   string               `json:"parentHeadOid,omitempty"`
	RemoteExists    bool                 `json:"remoteExists"`
	ParentAncestor  bool                 `json:"parentAncestor"`
	LocalExists     bool                 `json:"localExists"`
	ParentExists    bool                 `json:"parentExists"`
	IsCurrentBranch bool                 `json:"isCurrentBranch"`
	Verification    *VerificationSummary `json:"verification,omitempty"`
	Issues          []HealthIssue        `json:"issues"`
	Record          store.BranchRecord   `json:"record"`
}

type LandingBranchSummary struct {
	Name            string               `json:"name"`
	CurrentHeadOID  string               `json:"currentHeadOid,omitempty"`
	RemoteExists    bool                 `json:"remoteExists"`
	LocalExists     bool                 `json:"localExists"`
	IsCurrentBranch bool                 `json:"isCurrentBranch"`
	Verification    *VerificationSummary `json:"verification,omitempty"`
	Issues          []HealthIssue        `json:"issues"`
}

type VerificationSummary struct {
	Count              int                      `json:"count"`
	Latest             store.VerificationRecord `json:"latest"`
	HeadMatchesCurrent bool                     `json:"headMatchesCurrent"`
}

type Summary struct {
	RepoRoot        string                 `json:"repoRoot"`
	Trunk           string                 `json:"trunk"`
	DefaultRemote   string                 `json:"defaultRemote"`
	CurrentBranch   string                 `json:"currentBranch,omitempty"`
	RepoIssues      []HealthIssue          `json:"repoIssues,omitempty"`
	UntrackedHeads  []string               `json:"untrackedHeads,omitempty"`
	Branches        []BranchSummary        `json:"branches"`
	LandingBranches []LandingBranchSummary `json:"landingBranches,omitempty"`
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
		summary.Verification = buildVerificationSummary(state.Verifications[branchName], summary.CurrentHeadOID)

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
						Message:  "branch is not currently based on parent; run `stack restack` or repair it manually",
					})
				}
			}
		}

		if record.Restack.LastParentHeadOID == "" {
			summary.Issues = append(summary.Issues, HealthIssue{
				Severity: SeverityWarn,
				Message:  "missing restack anchor; re-track or repair metadata before restacking",
			})
		} else if summary.ParentHeadOID != "" && record.Restack.LastParentHeadOID != summary.ParentHeadOID {
			summary.Issues = append(summary.Issues, HealthIssue{
				Severity: SeverityInfo,
				Message:  "parent moved since last recorded restack; run `stack restack` before submitting",
			})
		}

		if !summary.RemoteExists {
			summary.Issues = append(summary.Issues, HealthIssue{
				Severity: SeverityInfo,
				Message:  "remote branch has not been pushed yet; run `stack submit` when ready",
			})
		} else if record.PR.Number == 0 {
			summary.Issues = append(summary.Issues, HealthIssue{
				Severity: SeverityWarn,
				Message:  "remote branch exists but no PR is linked; run `stack submit` to relink or create one",
			})
		}

		if record.PR.Number > 0 {
			if record.PR.State == "CLOSED" {
				summary.Issues = append(summary.Issues, HealthIssue{
					Severity: SeverityWarn,
					Message:  "tracked PR is closed; repair or relink it before submitting again",
				})
			}
			if record.PR.State == "MERGED" {
				summary.Issues = append(summary.Issues, HealthIssue{
					Severity: SeverityInfo,
					Message:  "tracked PR is merged; run `stack sync` before moving descendants",
				})
			}
			if record.PR.BaseRefName != "" && record.PR.BaseRefName != record.ParentBranch {
				summary.Issues = append(summary.Issues, HealthIssue{
					Severity: SeverityWarn,
					Message:  fmt.Sprintf("cached PR base is %q, expected %q; run `stack submit` to retarget it", record.PR.BaseRefName, record.ParentBranch),
				})
			}
			if record.PR.HeadRefName != "" && record.PR.HeadRefName != branchName {
				summary.Issues = append(summary.Issues, HealthIssue{
					Severity: SeverityWarn,
					Message:  fmt.Sprintf("cached PR head is %q, expected %q; relink the correct PR before continuing", record.PR.HeadRefName, branchName),
				})
			}
			if summary.CurrentHeadOID != "" && record.PR.LastSeenHeadOID != "" && record.PR.LastSeenHeadOID != summary.CurrentHeadOID {
				summary.Issues = append(summary.Issues, HealthIssue{
					Severity: SeverityInfo,
					Message:  "local branch head differs from the last synced PR head; run `stack submit` to refresh it",
				})
			}
		}
		appendVerificationIssues(branchName, &summary.Issues, summary.Verification)

		branches = append(branches, summary)
	}

	landingBranches := buildLandingSummaries(ctx, git, state, strings.TrimSpace(currentBranch))

	return Summary{
		RepoRoot:        paths.Root,
		Trunk:           state.Trunk,
		DefaultRemote:   state.DefaultRemote,
		CurrentBranch:   strings.TrimSpace(currentBranch),
		RepoIssues:      validation.RepoIssues,
		Branches:        branches,
		LandingBranches: landingBranches,
	}, nil
}

func buildLandingSummaries(ctx context.Context, git *stackgit.Client, state store.RepoState, currentBranch string) []LandingBranchSummary {
	names := make([]string, 0)
	for branchName := range state.Verifications {
		if _, tracked := state.Branches[branchName]; tracked {
			continue
		}
		names = append(names, branchName)
	}
	sort.Strings(names)

	landingBranches := make([]LandingBranchSummary, 0, len(names))
	for _, branchName := range names {
		summary := LandingBranchSummary{
			Name:            branchName,
			LocalExists:     git.BranchExists(ctx, branchName),
			RemoteExists:    git.RemoteBranchExists(ctx, state.DefaultRemote, branchName),
			IsCurrentBranch: currentBranch == branchName,
		}
		if summary.LocalExists {
			headOID, err := git.ResolveRef(ctx, branchName)
			if err == nil {
				summary.CurrentHeadOID = headOID
			}
		}
		summary.Verification = buildVerificationSummary(state.Verifications[branchName], summary.CurrentHeadOID)

		if !summary.LocalExists {
			summary.Issues = append(summary.Issues, HealthIssue{
				Severity: SeverityWarn,
				Message:  "landing branch is missing locally; recreate or repair it before relying on stored verification",
			})
		}
		if summary.LocalExists && !summary.RemoteExists {
			summary.Issues = append(summary.Issues, HealthIssue{
				Severity: SeverityInfo,
				Message:  "landing branch has not been pushed yet; push it when you are ready to open or update the landing PR",
			})
		}
		appendVerificationIssues(branchName, &summary.Issues, summary.Verification)
		landingBranches = append(landingBranches, summary)
	}

	return landingBranches
}

func buildVerificationSummary(records []store.VerificationRecord, currentHeadOID string) *VerificationSummary {
	if len(records) == 0 {
		return nil
	}
	latest := records[len(records)-1]
	return &VerificationSummary{
		Count:              len(records),
		Latest:             latest,
		HeadMatchesCurrent: latest.HeadOID != "" && currentHeadOID != "" && latest.HeadOID == currentHeadOID,
	}
}

func appendVerificationIssues(branchName string, issues *[]HealthIssue, verification *VerificationSummary) {
	if verification == nil {
		return
	}
	if !verification.Latest.Passed {
		*issues = append(*issues, HealthIssue{
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("latest %s verification failed; inspect `stack verify list %s` before landing", verification.Latest.CheckType, branchName),
		})
	}
	if !verification.HeadMatchesCurrent {
		*issues = append(*issues, HealthIssue{
			Severity: SeverityWarn,
			Message:  "branch head moved since the latest recorded verification; rerun or record fresh verification before landing",
		})
	}
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
