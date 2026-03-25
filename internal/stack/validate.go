package stack

import (
	"fmt"
	"sort"

	"github.com/hack-dance/stack/internal/store"
)

type ValidationResult struct {
	RepoIssues   []HealthIssue
	BranchIssues map[string][]HealthIssue
}

func ValidateState(state store.RepoState) ValidationResult {
	result := ValidationResult{
		RepoIssues:   make([]HealthIssue, 0),
		BranchIssues: map[string][]HealthIssue{},
	}

	if state.Trunk == "" {
		result.RepoIssues = append(result.RepoIssues, HealthIssue{
			Severity: SeverityError,
			Message:  "missing configured trunk branch",
		})
	}

	if state.DefaultRemote == "" {
		result.RepoIssues = append(result.RepoIssues, HealthIssue{
			Severity: SeverityError,
			Message:  "missing configured default remote",
		})
	}

	branches := make([]string, 0, len(state.Branches))
	for branchName := range state.Branches {
		branches = append(branches, branchName)
	}
	sort.Strings(branches)

	for _, branchName := range branches {
		record := state.Branches[branchName]
		if record.ParentBranch == branchName {
			result.BranchIssues[branchName] = append(result.BranchIssues[branchName], HealthIssue{
				Severity: SeverityError,
				Message:  "branch cannot parent itself",
			})
		}

		if record.ParentBranch == "" {
			result.BranchIssues[branchName] = append(result.BranchIssues[branchName], HealthIssue{
				Severity: SeverityError,
				Message:  "branch has no configured parent",
			})
		}

		if record.ParentBranch != "" && record.ParentBranch != state.Trunk {
			if _, ok := state.Branches[record.ParentBranch]; !ok {
				result.BranchIssues[branchName] = append(result.BranchIssues[branchName], HealthIssue{
					Severity: SeverityError,
					Message:  fmt.Sprintf("parent %q is not tracked in local metadata", record.ParentBranch),
				})
			}
		}
	}

	prOwners := map[int]string{}
	for _, branchName := range branches {
		record := state.Branches[branchName]
		if record.PR.Number == 0 {
			continue
		}

		if owner, ok := prOwners[record.PR.Number]; ok && owner != branchName {
			message := fmt.Sprintf("pull request #%d is linked to both %q and %q", record.PR.Number, owner, branchName)
			result.BranchIssues[branchName] = append(result.BranchIssues[branchName], HealthIssue{
				Severity: SeverityError,
				Message:  message,
			})
			result.BranchIssues[owner] = append(result.BranchIssues[owner], HealthIssue{
				Severity: SeverityError,
				Message:  message,
			})
			continue
		}

		prOwners[record.PR.Number] = branchName
	}

	for _, cycle := range findCycles(state) {
		if len(cycle) == 0 {
			continue
		}

		message := fmt.Sprintf("cycle detected in stack metadata: %s", cycleString(cycle))
		result.RepoIssues = append(result.RepoIssues, HealthIssue{
			Severity: SeverityError,
			Message:  message,
		})

		for _, branchName := range cycle {
			result.BranchIssues[branchName] = append(result.BranchIssues[branchName], HealthIssue{
				Severity: SeverityError,
				Message:  message,
			})
		}
	}

	return result
}

func EnsureBranchCanParent(state store.RepoState, branch string, parent string) error {
	if branch == parent {
		return fmt.Errorf("branch %q cannot parent itself", branch)
	}

	current := parent
	visited := map[string]bool{}
	for current != "" && current != state.Trunk {
		if current == branch {
			return fmt.Errorf("changing %q to parent %q would create a cycle", branch, parent)
		}
		if visited[current] {
			return fmt.Errorf("parent chain for %q is already cyclic; repair metadata before moving branches", branch)
		}
		visited[current] = true

		record, ok := state.Branches[current]
		if !ok {
			return nil
		}
		current = record.ParentBranch
	}

	return nil
}

func HasErrors(issues []HealthIssue) bool {
	for _, issue := range issues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}

func findCycles(state store.RepoState) [][]string {
	visited := map[string]bool{}
	onStack := map[string]int{}
	stackPath := make([]string, 0)
	cycles := make([][]string, 0)
	recorded := map[string]bool{}

	var visit func(string)
	visit = func(branch string) {
		if branch == "" || branch == state.Trunk {
			return
		}
		if _, ok := state.Branches[branch]; !ok {
			return
		}
		if visited[branch] {
			return
		}

		onStack[branch] = len(stackPath)
		stackPath = append(stackPath, branch)

		parent := state.Branches[branch].ParentBranch
		if index, ok := onStack[parent]; ok {
			cycle := append([]string(nil), stackPath[index:]...)
			key := cycleString(cycle)
			if !recorded[key] {
				recorded[key] = true
				cycles = append(cycles, cycle)
			}
		} else {
			visit(parent)
		}

		stackPath = stackPath[:len(stackPath)-1]
		delete(onStack, branch)
		visited[branch] = true
	}

	branches := make([]string, 0, len(state.Branches))
	for branch := range state.Branches {
		branches = append(branches, branch)
	}
	sort.Strings(branches)

	for _, branch := range branches {
		visit(branch)
	}

	return cycles
}

func cycleString(cycle []string) string {
	ordered := normalizeCycle(cycle)
	if len(ordered) == 0 {
		return ""
	}
	return fmt.Sprintf("%s -> %s", joinCycle(ordered), ordered[0])
}

func normalizeCycle(cycle []string) []string {
	if len(cycle) == 0 {
		return nil
	}

	best := append([]string(nil), cycle...)
	for index := 1; index < len(cycle); index += 1 {
		candidate := append([]string(nil), cycle[index:]...)
		candidate = append(candidate, cycle[:index]...)
		if lessCycle(candidate, best) {
			best = candidate
		}
	}

	return best
}

func lessCycle(a []string, b []string) bool {
	for index := range a {
		if a[index] == b[index] {
			continue
		}
		return a[index] < b[index]
	}
	return false
}

func joinCycle(cycle []string) string {
	result := ""
	for index, branch := range cycle {
		if index > 0 {
			result += " -> "
		}
		result += branch
	}
	return result
}
