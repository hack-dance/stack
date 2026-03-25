package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hack-dance/stack/internal/docs"
	"github.com/hack-dance/stack/internal/forms"
	stackruntime "github.com/hack-dance/stack/internal/runtime"
	"github.com/hack-dance/stack/internal/stack"
	"github.com/hack-dance/stack/internal/store"
	"github.com/hack-dance/stack/internal/tui"
	"github.com/hack-dance/stack/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewRootCommand(runtime *stackruntime.Runtime) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "stack",
		Short: "Manage explicit stacked PR workflows with Git and GitHub",
		Long: strings.TrimSpace(`
Use normal Git branches, normal GitHub pull requests, and explicit local stack
metadata. The CLI favors visible state, repairable workflows, and safe handoff
to GitHub merge queue via the gh CLI.
		`),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(runtime, false)
		},
	}

	rootCmd.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.RenderMarkdown(docs.CommandMarkdown(cmd)))
	})

	rootCmd.AddCommand(
		newInitCommand(runtime),
		newCreateCommand(runtime),
		newTrackCommand(runtime),
		newStatusCommand(runtime),
		newTUICommand(runtime),
		newRestackCommand(runtime),
		newContinueCommand(runtime),
		newAbortCommand(runtime),
		newMoveCommand(runtime),
		newSubmitCommand(runtime),
		newSyncCommand(runtime),
		newQueueCommand(runtime),
	)

	rootCmd.AddCommand(&cobra.Command{
		Use:    "log",
		Short:  "Alias for `stack status`",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(runtime, false)
		},
	})

	return rootCmd
}

func newInitCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var trunk string
	var remote string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize stack metadata for this repository",
		Long:  "Create the shared stack state file and record the repo trunk and default remote.",
		Example: strings.TrimSpace(`
stack init
stack init --trunk main --remote origin
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := ""
			repoView, err := runtime.GitHub.RepoView(runtime.Context)
			if err == nil {
				repo = repoView.NameWithOwner
				if trunk == "" {
					trunk = repoView.DefaultBranchRef.Name
				}
			}

			if trunk == "" {
				trunk = "main"
			}
			if remote == "" {
				remote = "origin"
			}

			state, err := runtime.Store.InitState(runtime.Context, trunk, remote, repo)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Initialized stack", []string{
				fmt.Sprintf("repo: %s", chooseString(state.Repo, "(unresolved)")),
				fmt.Sprintf("trunk: %s", state.Trunk),
				fmt.Sprintf("remote: %s", state.DefaultRemote),
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&trunk, "trunk", "", "Trunk branch name")
	cmd.Flags().StringVar(&remote, "remote", "", "Default remote name")
	return cmd
}

func newCreateCommand(runtime *stackruntime.Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <branch>",
		Short: "Create a tracked branch on top of the current branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}

			parent, err := runtime.Git.CurrentBranch(runtime.Context)
			if err != nil {
				return err
			}

			if err := runtime.Git.SwitchCreate(runtime.Context, args[0]); err != nil {
				return err
			}

			parentOID, _ := runtime.Git.ResolveRef(runtime.Context, parent)
			state.Branches[args[0]] = store.BranchRecord{
				ParentBranch: parent,
				RemoteName:   state.DefaultRemote,
				Restack: store.RestackMetadata{
					LastParentHeadOID: parentOID,
					LastRestackedAt:   time.Now().UTC().Format(time.RFC3339),
				},
			}

			if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Created tracked branch", []string{
				fmt.Sprintf("branch: %s", args[0]),
				fmt.Sprintf("parent: %s", parent),
			}))
			return nil
		},
	}

	return cmd
}

func newTrackCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var parent string

	cmd := &cobra.Command{
		Use:   "track <branch>",
		Short: "Adopt an existing branch into the explicit stack graph",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if parent == "" {
				return fmt.Errorf("--parent is required")
			}

			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}

			branch := args[0]
			if !runtime.Git.BranchExists(runtime.Context, branch) {
				return fmt.Errorf("branch %q does not exist locally", branch)
			}

			if parent != state.Trunk && !runtime.Git.BranchExists(runtime.Context, parent) {
				return fmt.Errorf("parent branch %q does not exist locally", parent)
			}

			parentOID, _ := runtime.Git.ResolveRef(runtime.Context, parent)
			state.Branches[branch] = store.BranchRecord{
				ParentBranch: parent,
				RemoteName:   state.DefaultRemote,
				Restack: store.RestackMetadata{
					LastParentHeadOID: parentOID,
				},
			}

			return runtime.Store.WriteState(runtime.Context, state)
		},
	}

	cmd.Flags().StringVar(&parent, "parent", "", "Parent branch or trunk")
	return cmd
}

func newStatusCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show stack health, hierarchy, and cached PR state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(runtime, asJSON)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Render status as JSON")
	return cmd
}

func newTUICommand(runtime *stackruntime.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the read-only stack dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}
			summary, err := stack.BuildSummary(runtime.Context, runtime.Git, state)
			if err != nil {
				return err
			}
			return tui.Run(summary)
		},
	}
}

func newRestackCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var all bool
	var yes bool

	cmd := &cobra.Command{
		Use:   "restack [branch]",
		Short: "Rebase a branch or subtree onto its configured parent",
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}

			steps, err := restackSteps(runtime, state, args, all)
			if err != nil {
				return err
			}
			if len(steps) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Restack", []string{"No branches need restacking."}))
				return nil
			}

			lines := make([]string, 0, len(steps))
			for _, step := range steps {
				lines = append(lines, fmt.Sprintf("%s -> %s", step.Branch, step.Parent))
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Restack preview", lines))

			if !yes {
				confirmed, err := forms.Confirm("Restack branches", "This will rewrite branch history for the listed branches.")
				if err != nil {
					return err
				}
				if !confirmed {
					return fmt.Errorf("restack cancelled")
				}
			}

			return runRestackPlan(runtime, state, steps)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Restack all tracked branches")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation")
	return cmd
}

func newContinueCommand(runtime *stackruntime.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "continue",
		Short: "Continue an interrupted restack after conflicts are resolved",
		RunE: func(cmd *cobra.Command, args []string) error {
			op, err := runtime.Store.ReadOperation(runtime.Context)
			if err != nil {
				return fmt.Errorf("no interrupted operation found")
			}

			if err := runtime.Git.RebaseContinue(runtime.Context); err != nil {
				return err
			}

			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}

			if record, ok := state.Branches[op.Active.Branch]; ok {
				parentOID, _ := runtime.Git.ResolveRef(runtime.Context, op.Active.Parent)
				record.Restack.LastParentHeadOID = parentOID
				record.Restack.LastRestackedAt = time.Now().UTC().Format(time.RFC3339)
				state.Branches[op.Active.Branch] = record
				if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
					return err
				}
			}

			if len(op.Pending) == 0 {
				return runtime.Store.ClearOperation(runtime.Context)
			}

			return runRestackPlan(runtime, state, op.Pending)
		},
	}
}

func newAbortCommand(runtime *stackruntime.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "abort",
		Short: "Abort an interrupted restack and clear the operation journal",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runtime.Git.RebaseAbort(runtime.Context); err != nil {
				return err
			}
			return runtime.Store.ClearOperation(runtime.Context)
		},
	}
}

func newMoveCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var parent string
	var yes bool

	cmd := &cobra.Command{
		Use:   "move <branch>",
		Short: "Change a branch parent and restack the affected subtree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if parent == "" {
				return fmt.Errorf("--parent is required")
			}

			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}

			branch := args[0]
			record, ok := state.Branches[branch]
			if !ok {
				return fmt.Errorf("branch %q is not tracked", branch)
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Move preview", []string{
				fmt.Sprintf("%s: %s -> %s", branch, record.ParentBranch, parent),
			}))

			if !yes {
				confirmed, err := forms.Confirm("Move branch", "This updates stack metadata and may rewrite descendant history.")
				if err != nil {
					return err
				}
				if !confirmed {
					return fmt.Errorf("move cancelled")
				}
			}

			parentOID, _ := runtime.Git.ResolveRef(runtime.Context, parent)
			record.ParentBranch = parent
			record.Restack.LastParentHeadOID = parentOID
			state.Branches[branch] = record
			if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
				return err
			}

			return runRestackPlan(runtime, state, []store.RestackStep{{Branch: branch, Parent: parent, PreviousParentHead: parentOID}})
		},
	}

	cmd.Flags().StringVar(&parent, "parent", "", "New parent branch")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation")
	return cmd
}

func newSubmitCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var all bool
	var noRestack bool
	var draft bool

	cmd := &cobra.Command{
		Use:   "submit [branch]",
		Short: "Push tracked branches and create or update one normal PR per branch",
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}

			targets, err := selectTargets(runtime, state, args, all)
			if err != nil {
				return err
			}

			if !noRestack {
				steps, err := restackStepsForTargets(runtime, state, targets)
				if err != nil {
					return err
				}
				if len(steps) > 0 {
					if err := runRestackPlan(runtime, state, steps); err != nil {
						return err
					}
					state, err = runtime.Store.ReadState(runtime.Context)
					if err != nil {
						return err
					}
				}
			}

			if err := runtime.Git.FetchPrune(runtime.Context, state.DefaultRemote); err != nil {
				return err
			}

			for _, branch := range targets {
				if err := runtime.Git.PushForceWithLease(runtime.Context, state.DefaultRemote, branch); err != nil {
					return err
				}

				record := state.Branches[branch]
				if record.PR.Number == 0 {
					found, err := runtime.GitHub.FindPRByHead(runtime.Context, branch)
					if err != nil {
						return err
					}
					if found.Number > 0 {
						record.PR = found
					}
				}

				if record.PR.Number == 0 {
					title, body, err := runtime.Git.CommitMessage(runtime.Context, branch)
					if err != nil {
						return err
					}
					if title == "" {
						title = branch
					}
					body = chooseString(body, fmt.Sprintf("Stack branch `%s` targeting `%s`.", branch, record.ParentBranch))
					pr, err := runtime.GitHub.CreatePR(runtime.Context, record.ParentBranch, branch, title, body, draft)
					if err != nil {
						return err
					}
					record.PR = pr
				} else if record.PR.BaseRefName != record.ParentBranch {
					if err := runtime.GitHub.EditPRBase(runtime.Context, record.PR.Number, record.ParentBranch); err != nil {
						return err
					}
					updatedPR, err := runtime.GitHub.ViewPR(runtime.Context, record.PR.Number)
					if err != nil {
						return err
					}
					record.PR = updatedPR
				}

				state.Branches[branch] = record
			}

			return runtime.Store.WriteState(runtime.Context, state)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Submit all tracked branches")
	cmd.Flags().BoolVar(&noRestack, "no-restack", false, "Skip restack before submit")
	cmd.Flags().BoolVar(&draft, "draft", false, "Create PRs as drafts when needed")
	return cmd
}

func newSyncCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var apply bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Refresh cached PR metadata and inspect or apply safe repairs",
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}

			if err := runtime.Git.FetchPrune(runtime.Context, state.DefaultRemote); err != nil {
				return err
			}

			repairs := make([]string, 0)
			for branch, record := range state.Branches {
				if record.PR.Number == 0 {
					continue
				}

				pr, err := runtime.GitHub.ViewPR(runtime.Context, record.PR.Number)
				if err != nil {
					continue
				}
				record.PR = pr
				state.Branches[branch] = record

				if pr.State == "MERGED" {
					for _, child := range stack.Children(state, branch) {
						childRecord := state.Branches[child]
						repairs = append(repairs, fmt.Sprintf("%s: %s -> %s", child, branch, record.ParentBranch))
						if apply {
							childRecord.ParentBranch = record.ParentBranch
							state.Branches[child] = childRecord
						}
					}
				}
			}

			if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
				return err
			}

			title := "Sync report"
			if apply {
				title = "Sync applied"
			}
			if len(repairs) == 0 {
				repairs = append(repairs, "No clean repairs detected.")
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview(title, repairs))
			return nil
		},
	}

	cmd.Flags().BoolVar(&apply, "apply", false, "Apply clean merged-parent repairs")
	return cmd
}

func newQueueCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "queue <branch>",
		Short: "Hand one healthy bottom-of-stack PR to GitHub auto-merge or merge queue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}

			branch := args[0]
			record, ok := state.Branches[branch]
			if !ok {
				return fmt.Errorf("branch %q is not tracked", branch)
			}
			if record.ParentBranch != state.Trunk {
				return fmt.Errorf("branch %q must target trunk before queue handoff", branch)
			}
			if record.PR.Number == 0 {
				return fmt.Errorf("branch %q has no tracked PR", branch)
			}

			headOID, err := runtime.Git.ResolveRef(runtime.Context, branch)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Queue handoff", []string{
				fmt.Sprintf("branch: %s", branch),
				fmt.Sprintf("pr: #%d", record.PR.Number),
				fmt.Sprintf("head: %s", headOID),
			}))

			if !yes {
				confirmed, err := forms.Confirm("Queue branch", "This hands the current PR head to GitHub auto-merge or merge queue.")
				if err != nil {
					return err
				}
				if !confirmed {
					return fmt.Errorf("queue cancelled")
				}
			}

			return runtime.GitHub.MergePR(runtime.Context, record.PR.Number, headOID)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation")
	return cmd
}

func runStatus(runtime *stackruntime.Runtime, asJSON bool) error {
	state, err := runtime.Store.ReadState(runtime.Context)
	if err != nil {
		return err
	}

	summary, err := stack.BuildSummary(runtime.Context, runtime.Git, state)
	if err != nil {
		return err
	}

	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	}

	_, _ = fmt.Fprintln(os.Stdout, ui.RenderStatus(summary))
	return nil
}

func restackSteps(runtime *stackruntime.Runtime, state store.RepoState, args []string, all bool) ([]store.RestackStep, error) {
	targets, err := selectTargets(runtime, state, args, all)
	if err != nil {
		return nil, err
	}

	return restackStepsForTargets(runtime, state, targets)
}

func restackStepsForTargets(runtime *stackruntime.Runtime, state store.RepoState, targets []string) ([]store.RestackStep, error) {
	seen := map[string]bool{}
	steps := make([]store.RestackStep, 0)

	for _, target := range targets {
		for _, branch := range collectSubtree(state, target) {
			if seen[branch] {
				continue
			}
			seen[branch] = true
			record := state.Branches[branch]
			if record.Restack.LastParentHeadOID == "" {
				return nil, fmt.Errorf("branch %q has no recorded restack anchor; use `stack track` or repair metadata first", branch)
			}

			validAnchor, err := runtime.Git.IsAncestor(runtime.Context, record.Restack.LastParentHeadOID, branch)
			if err != nil {
				return nil, err
			}
			if !validAnchor {
				return nil, fmt.Errorf("branch %q has an invalid restack anchor; refusing to guess a merge-base fallback", branch)
			}

			parentOID, err := runtime.Git.ResolveRef(runtime.Context, record.ParentBranch)
			if err != nil {
				return nil, err
			}
			if parentOID == record.Restack.LastParentHeadOID {
				continue
			}

			steps = append(steps, store.RestackStep{
				Branch:             branch,
				Parent:             record.ParentBranch,
				PreviousParentHead: record.Restack.LastParentHeadOID,
			})
		}
	}

	return steps, nil
}

func runRestackPlan(runtime *stackruntime.Runtime, state store.RepoState, steps []store.RestackStep) error {
	paths, err := runtime.Store.ResolvePaths(runtime.Context)
	if err != nil {
		return err
	}

	for index, step := range steps {
		operation := store.OperationState{
			Type:           "restack",
			RepositoryRoot: paths.Root,
			WorktreeGitDir: paths.GitDir,
			StartedAt:      time.Now().UTC().Format(time.RFC3339),
			Active:         step,
			Pending:        append([]store.RestackStep(nil), steps[index+1:]...),
		}
		if head, err := runtime.Git.ResolveRef(runtime.Context, "HEAD"); err == nil {
			operation.OriginalHEAD = head
		}
		if err := runtime.Store.WriteOperation(runtime.Context, operation); err != nil {
			return err
		}

		if err := runtime.Git.RebaseOnto(runtime.Context, step.Parent, step.PreviousParentHead, step.Branch); err != nil {
			return err
		}

		record := state.Branches[step.Branch]
		parentOID, _ := runtime.Git.ResolveRef(runtime.Context, step.Parent)
		record.Restack.LastParentHeadOID = parentOID
		record.Restack.LastRestackedAt = time.Now().UTC().Format(time.RFC3339)
		state.Branches[step.Branch] = record
		if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
			return err
		}
	}

	return runtime.Store.ClearOperation(runtime.Context)
}

func selectTargets(runtime *stackruntime.Runtime, state store.RepoState, args []string, all bool) ([]string, error) {
	if all {
		targets := make([]string, 0, len(state.Branches))
		for _, branch := range orderedBranches(state) {
			targets = append(targets, branch)
		}
		return targets, nil
	}

	if len(args) > 0 {
		if _, ok := state.Branches[args[0]]; !ok {
			return nil, fmt.Errorf("branch %q is not tracked", args[0])
		}
		return []string{args[0]}, nil
	}

	current, err := runtime.Git.CurrentBranch(runtime.Context)
	if err != nil {
		return nil, err
	}
	if _, ok := state.Branches[current]; !ok {
		return nil, fmt.Errorf("current branch %q is not tracked", current)
	}
	return []string{current}, nil
}

func orderedBranches(state store.RepoState) []string {
	result := make([]string, 0, len(state.Branches))
	for _, branch := range stack.Children(state, state.Trunk) {
		result = append(result, branch)
		appendDescendants(state, branch, &result)
	}
	return result
}

func appendDescendants(state store.RepoState, parent string, ordered *[]string) {
	for _, child := range stack.Children(state, parent) {
		*ordered = append(*ordered, child)
		appendDescendants(state, child, ordered)
	}
}

func collectSubtree(state store.RepoState, root string) []string {
	branches := []string{root}
	for _, child := range stack.Children(state, root) {
		branches = append(branches, collectSubtree(state, child)...)
	}
	return branches
}

func chooseString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func init() {
	pflag.CommandLine.SortFlags = false
}
