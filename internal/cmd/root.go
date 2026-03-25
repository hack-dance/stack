package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hack-dance/stack/internal/buildinfo"
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
		newVersionCommand(),
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

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, and build date",
		Long:  "Print the stack CLI version information embedded at build time.",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), buildinfo.Summary())
			return nil
		},
	}
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
			if err := ensureStateWritable(state); err != nil {
				return err
			}
			if err := ensureNoPendingOperation(runtime); err != nil {
				return err
			}
			if _, exists := state.Branches[args[0]]; exists {
				return fmt.Errorf("branch %q is already tracked", args[0])
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
			if err := ensureStateWritable(state); err != nil {
				return err
			}

			branch := args[0]
			if !runtime.Git.BranchExists(runtime.Context, branch) {
				return fmt.Errorf("branch %q does not exist locally", branch)
			}

			if parent != state.Trunk && !runtime.Git.BranchExists(runtime.Context, parent) {
				return fmt.Errorf("parent branch %q does not exist locally", parent)
			}
			if err := stack.EnsureBranchCanParent(state, branch, parent); err != nil {
				return err
			}

			parentOID, _ := runtime.Git.ResolveRef(runtime.Context, parent)
			state.Branches[branch] = store.BranchRecord{
				ParentBranch: parent,
				RemoteName:   state.DefaultRemote,
				Restack: store.RestackMetadata{
					LastParentHeadOID: parentOID,
				},
			}

			if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Tracked branch", []string{
				fmt.Sprintf("branch: %s", branch),
				fmt.Sprintf("parent: %s", parent),
			}))
			return nil
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
		Long:  "Inspect the current stack graph, branch health, cached PR state, and repo-level metadata issues.",
		Example: strings.TrimSpace(`
stack status
stack status --json
		`),
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
		Long:  "Open a read-only Bubble Tea dashboard for browsing the stack tree, health details, and cached PR state.",
		Example: strings.TrimSpace(`
stack tui
		`),
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
		Long:  "Preview and restack one tracked branch or subtree. The command refuses invalid anchors and stops instead of guessing.",
		Example: strings.TrimSpace(`
stack restack
stack restack feature/a
stack restack --all --yes
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}
			if err := ensureStateWritable(state); err != nil {
				return err
			}
			if err := ensureNoPendingOperation(runtime); err != nil {
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

			return runRestackPlan(runtime, state, steps, nil)
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
		Long:  "Resume a recorded restack only when the current worktree and git rebase state match the saved operation journal.",
		Example: strings.TrimSpace(`
stack continue
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			op, err := runtime.Store.ReadOperation(runtime.Context)
			if err != nil {
				return fmt.Errorf("no interrupted operation found")
			}
			if err := validateOperationContext(runtime, op); err != nil {
				return err
			}
			rebaseInProgress, err := runtime.Git.RebaseInProgress(runtime.Context)
			if err != nil {
				return err
			}
			if !rebaseInProgress {
				return fmt.Errorf("no git rebase is currently in progress for this worktree")
			}

			if err := runtime.Git.RebaseContinue(runtime.Context); err != nil {
				return err
			}

			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}
			if err := ensureStateWritable(state); err != nil {
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
				if err := runtime.Store.ClearOperation(runtime.Context); err != nil {
					return err
				}
				return restoreOriginalBranch(runtime, op)
			}

			return runRestackPlan(runtime, state, op.Pending, &op)
		},
	}
}

func newAbortCommand(runtime *stackruntime.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "abort",
		Short: "Abort an interrupted restack and clear the operation journal",
		Long:  "Abort the active git rebase in this worktree, clear the saved operation journal, and switch back to the original branch when possible.",
		Example: strings.TrimSpace(`
stack abort
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			op, _ := runtime.Store.ReadOperation(runtime.Context)
			rebaseInProgress, err := runtime.Git.RebaseInProgress(runtime.Context)
			if err != nil {
				return err
			}
			if rebaseInProgress {
				if err := runtime.Git.RebaseAbort(runtime.Context); err != nil {
					return err
				}
			}
			if err := runtime.Store.ClearOperation(runtime.Context); err != nil {
				return err
			}
			return restoreOriginalBranch(runtime, op)
		},
	}
}

func newMoveCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var parent string
	var yes bool

	cmd := &cobra.Command{
		Use:   "move <branch>",
		Short: "Change a branch parent and restack the affected subtree",
		Long:  "Change one tracked branch to a new parent, preview the rewrite, update metadata, and restack from the recorded anchor.",
		Example: strings.TrimSpace(`
stack move feature/b --parent feature/a
stack move feature/b --parent main --yes
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if parent == "" {
				return fmt.Errorf("--parent is required")
			}

			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}
			if err := ensureStateWritable(state); err != nil {
				return err
			}
			if err := ensureNoPendingOperation(runtime); err != nil {
				return err
			}

			branch := args[0]
			record, ok := state.Branches[branch]
			if !ok {
				return fmt.Errorf("branch %q is not tracked", branch)
			}
			if !runtime.Git.BranchExists(runtime.Context, parent) && parent != state.Trunk {
				return fmt.Errorf("parent branch %q does not exist locally", parent)
			}
			if parent != state.Trunk {
				if _, ok := state.Branches[parent]; !ok {
					return fmt.Errorf("parent branch %q is not tracked in local metadata; track it first or move under %s", parent, state.Trunk)
				}
			}
			if err := stack.EnsureBranchCanParent(state, branch, parent); err != nil {
				return err
			}

			preview := []string{fmt.Sprintf("%s: %s -> %s", branch, record.ParentBranch, parent)}
			for _, descendant := range collectSubtree(state, branch)[1:] {
				preview = append(preview, fmt.Sprintf("%s: restack on top of rewritten %s", descendant, state.Branches[descendant].ParentBranch))
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Move preview", preview))

			if !yes {
				confirmed, err := forms.Confirm("Move branch", "This updates stack metadata and may rewrite descendant history.")
				if err != nil {
					return err
				}
				if !confirmed {
					return fmt.Errorf("move cancelled")
				}
			}

			previousParentHead := record.Restack.LastParentHeadOID
			if previousParentHead == "" {
				return fmt.Errorf("branch %q has no recorded restack anchor; cannot move safely", branch)
			}

			plannedState := cloneRepoState(state)
			record.ParentBranch = parent
			plannedState.Branches[branch] = record

			steps, err := restackStepsForTargets(runtime, plannedState, []string{branch})
			if err != nil {
				return err
			}
			if len(steps) == 0 {
				steps = []store.RestackStep{{
					Branch:             branch,
					Parent:             parent,
					PreviousParentHead: previousParentHead,
					PreviousBranchHead: resolveOID(runtime, branch),
				}}
			}
			return runRestackPlan(runtime, plannedState, steps, nil)
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
	var yes bool

	cmd := &cobra.Command{
		Use:   "submit [branch]",
		Short: "Push tracked branches and create or update one normal PR per branch",
		Long:  "Fetch, optionally restack, preview the push plan, then push tracked branches and create or refresh one normal GitHub PR per branch.",
		Example: strings.TrimSpace(`
stack submit
stack submit feature/a --draft
stack submit --all --yes
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}
			if err := ensureStateWritable(state); err != nil {
				return err
			}
			if err := ensureNoPendingOperation(runtime); err != nil {
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
					if err := runRestackPlan(runtime, state, steps, nil); err != nil {
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

			plans := make([]submitPlan, 0, len(targets))
			for _, branch := range targets {
				plan, err := buildSubmitPlan(runtime, state, branch)
				if err != nil {
					return err
				}
				plans = append(plans, plan)
			}

			previewLines := make([]string, 0, len(plans)*3)
			for _, plan := range plans {
				previewLines = append(previewLines, plan.Preview...)
			}
			if len(previewLines) == 0 {
				previewLines = append(previewLines, "Nothing to submit.")
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Submit preview", previewLines))

			if !yes {
				confirmed, err := forms.Confirm("Submit branches", "This may push new branch tips and update pull request state on GitHub.")
				if err != nil {
					return err
				}
				if !confirmed {
					return fmt.Errorf("submit cancelled")
				}
			}

			for _, plan := range plans {
				record := plan.Record
				if err := runtime.Git.PushBranch(runtime.Context, state.DefaultRemote, plan.Branch, plan.RemoteHead); err != nil {
					return err
				}

				if record.PR.Number == 0 {
					title, body, err := runtime.Git.CommitMessage(runtime.Context, plan.Branch)
					if err != nil {
						return err
					}
					if title == "" {
						title = plan.Branch
					}
					body = chooseString(body, fmt.Sprintf("Stack branch `%s` targeting `%s`.", plan.Branch, record.ParentBranch))
					pr, err := runtime.GitHub.CreatePR(runtime.Context, record.ParentBranch, plan.Branch, title, body, draft)
					if err != nil {
						return err
					}
					record.PR = pr
				} else if err := validateTrackedPR(plan.Branch, record); err != nil {
					return err
				} else if record.PR.BaseRefName != record.ParentBranch {
					if err := runtime.GitHub.EditPRBase(runtime.Context, record.PR.Number, record.ParentBranch); err != nil {
						return err
					}
				}

				record, err = refreshTrackedPR(runtime, state, plan.Branch, record)
				if err != nil {
					return err
				}
				state.Branches[plan.Branch] = record
			}

			return runtime.Store.WriteState(runtime.Context, state)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Submit all tracked branches")
	cmd.Flags().BoolVar(&noRestack, "no-restack", false, "Skip restack before submit")
	cmd.Flags().BoolVar(&draft, "draft", false, "Create PRs as drafts when needed")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation")
	return cmd
}

func newSyncCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var apply bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Refresh cached PR metadata and inspect or apply safe repairs",
		Long:  "Refresh cached PR metadata from GitHub and either report or apply clean, classified repairs. Ambiguous cases stop for review.",
		Example: strings.TrimSpace(`
stack sync
stack sync --apply
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}

			if err := runtime.Git.FetchPrune(runtime.Context, state.DefaultRemote); err != nil {
				return err
			}

			repairs := make([]string, 0)
			for _, branch := range orderedBranches(state) {
				record := state.Branches[branch]
				if record.PR.Number == 0 {
					if runtime.Git.RemoteBranchExists(runtime.Context, state.DefaultRemote, branch) {
						repairs = append(repairs, fmt.Sprintf("%s: remote branch exists but no PR is linked in local metadata", branch))
					}
					continue
				}

				pr, err := runtime.GitHub.ViewPR(runtime.Context, record.PR.Number)
				if err != nil {
					repairs = append(repairs, fmt.Sprintf("%s: tracked PR #%d could not be loaded; repair required", branch, record.PR.Number))
					continue
				}
				record.PR = pr
				state.Branches[branch] = record

				if pr.HeadRefName != "" && pr.HeadRefName != branch {
					repairs = append(repairs, fmt.Sprintf("%s: PR head is %s, expected %s", branch, pr.HeadRefName, branch))
				}
				if pr.BaseRefName != "" && pr.BaseRefName != record.ParentBranch {
					repairs = append(repairs, fmt.Sprintf("%s: PR base is %s, expected %s", branch, pr.BaseRefName, record.ParentBranch))
					if apply && pr.State == "OPEN" && pr.HeadRefName == branch {
						if err := runtime.GitHub.EditPRBase(runtime.Context, pr.Number, record.ParentBranch); err != nil {
							return err
						}
						updatedPR, err := runtime.GitHub.ViewPR(runtime.Context, pr.Number)
						if err != nil {
							return err
						}
						record.PR = updatedPR
						state.Branches[branch] = record
					}
				}
				if !runtime.Git.RemoteBranchExists(runtime.Context, state.DefaultRemote, branch) {
					repairs = append(repairs, fmt.Sprintf("%s: remote branch is missing", branch))
				}

				cleanMergedParent := pr.State == "MERGED" &&
					(pr.HeadRefName == "" || pr.HeadRefName == branch) &&
					(pr.BaseRefName == "" || pr.BaseRefName == record.ParentBranch)

				if pr.State == "MERGED" && !cleanMergedParent {
					repairs = append(repairs, fmt.Sprintf("%s: merged parent has drifted PR metadata; inspect manually before reparenting children", branch))
				}

				if cleanMergedParent {
					parentHeadOID := pr.LastSeenHeadOID
					for _, child := range stack.Children(state, branch) {
						childRecord := state.Branches[child]
						if parentHeadOID != "" && childRecord.Restack.LastParentHeadOID == parentHeadOID {
							repairs = append(repairs, fmt.Sprintf("%s: clean reparent %s -> %s", child, branch, record.ParentBranch))
						} else {
							repairs = append(repairs, fmt.Sprintf("%s: merged parent %s needs manual review before reparenting", child, branch))
							continue
						}
						if apply {
							childRecord.ParentBranch = record.ParentBranch
							state.Branches[child] = childRecord
						}
					}
				} else if pr.State == "CLOSED" {
					repairs = append(repairs, fmt.Sprintf("%s: PR is closed without merge; manual repair required", branch))
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
	var strategy string

	cmd := &cobra.Command{
		Use:   "queue <branch>",
		Short: "Hand one healthy bottom-of-stack PR to GitHub auto-merge or merge queue",
		Long:  "Validate that one tracked branch is ready for trunk handoff, then ask GitHub to auto-merge or enqueue the PR using the current head commit.",
		Example: strings.TrimSpace(`
stack queue feature/a
stack queue feature/a --strategy squash
stack queue feature/a --yes
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isValidQueueStrategy(strategy) {
				return fmt.Errorf("invalid queue strategy %q; expected merge, squash, or rebase", strategy)
			}

			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}
			if err := ensureStateWritable(state); err != nil {
				return err
			}
			if err := runtime.Git.FetchPrune(runtime.Context, state.DefaultRemote); err != nil {
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
			remoteHeadOID, remoteExists, err := runtime.Git.RemoteBranchOID(runtime.Context, state.DefaultRemote, branch)
			if err != nil {
				return err
			}
			if !remoteExists {
				return fmt.Errorf("branch %q has not been pushed to %s", branch, state.DefaultRemote)
			}
			if remoteHeadOID != headOID {
				return fmt.Errorf("remote branch %q is stale; run `stack submit %s` before queue handoff", branch, branch)
			}

			record, err = refreshTrackedPR(runtime, state, branch, record)
			if err != nil {
				return err
			}
			if err := validateTrackedPR(branch, record); err != nil {
				return err
			}
			if record.PR.BaseRefName != "" && record.PR.BaseRefName != state.Trunk {
				return fmt.Errorf("PR for %q currently targets %q; run `stack submit %s` before queue handoff", branch, record.PR.BaseRefName, branch)
			}
			if record.PR.IsDraft {
				return fmt.Errorf("branch %q is still a draft PR", branch)
			}
			if record.PR.LastSeenHeadOID != "" && record.PR.LastSeenHeadOID != headOID {
				return fmt.Errorf("PR head for %q is stale; run `stack submit %s` before queue handoff", branch, branch)
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Queue handoff", []string{
				fmt.Sprintf("branch: %s", branch),
				fmt.Sprintf("pr: #%d", record.PR.Number),
				fmt.Sprintf("strategy: %s", strategy),
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

			if err := runtime.GitHub.MergePR(runtime.Context, record.PR.Number, headOID, strategy); err != nil {
				return err
			}

			nextSteps := []string{
				fmt.Sprintf("wait for GitHub to merge PR #%d", record.PR.Number),
				"then run: stack sync",
			}
			children := stack.Children(state, branch)
			if len(children) > 0 {
				nextSteps = append(nextSteps, fmt.Sprintf("then run: stack submit %s", children[0]))
				nextSteps = append(nextSteps, fmt.Sprintf("then run: stack queue %s", children[0]))
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Next steps", nextSteps))
			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation")
	cmd.Flags().StringVar(&strategy, "strategy", "merge", "Merge strategy: merge, squash, or rebase")
	return cmd
}

func isValidQueueStrategy(strategy string) bool {
	switch strategy {
	case "merge", "squash", "rebase":
		return true
	default:
		return false
	}
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

type submitPlan struct {
	Branch       string
	LocalHead    string
	RemoteHead   string
	RemoteExists bool
	Record       store.BranchRecord
	Preview      []string
}

func buildSubmitPlan(runtime *stackruntime.Runtime, state store.RepoState, branch string) (submitPlan, error) {
	record, ok := state.Branches[branch]
	if !ok {
		return submitPlan{}, fmt.Errorf("branch %q is no longer tracked", branch)
	}

	localHeadOID, err := runtime.Git.ResolveRef(runtime.Context, branch)
	if err != nil {
		return submitPlan{}, err
	}

	record, err = refreshTrackedPR(runtime, state, branch, record)
	if err != nil {
		return submitPlan{}, err
	}

	remoteHeadOID, remoteExists, err := runtime.Git.RemoteBranchOID(runtime.Context, state.DefaultRemote, branch)
	if err != nil {
		return submitPlan{}, err
	}
	if err := validateTrackedPR(branch, record); err != nil && record.PR.Number > 0 {
		return submitPlan{}, err
	}
	if err := validateRemoteAgreement(branch, record, localHeadOID, remoteHeadOID, remoteExists); err != nil {
		return submitPlan{}, err
	}

	preview := []string{fmt.Sprintf("%s -> %s", branch, record.ParentBranch)}
	if !remoteExists {
		preview = append(preview, "push new remote branch")
	} else if remoteHeadOID != localHeadOID {
		preview = append(preview, "force-push remote branch with explicit lease")
	} else {
		preview = append(preview, "remote branch already matches local head")
	}

	if record.PR.Number == 0 {
		preview = append(preview, "create GitHub pull request")
	} else if record.PR.BaseRefName != "" && record.PR.BaseRefName != record.ParentBranch {
		preview = append(preview, fmt.Sprintf("retarget PR #%d base to %s", record.PR.Number, record.ParentBranch))
	} else {
		preview = append(preview, fmt.Sprintf("refresh tracked PR #%d", record.PR.Number))
	}

	return submitPlan{
		Branch:       branch,
		LocalHead:    localHeadOID,
		RemoteHead:   remoteHeadOID,
		RemoteExists: remoteExists,
		Record:       record,
		Preview:      preview,
	}, nil
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
	subtreeOrder := make([]string, 0)
	snapshotHeads := map[string]string{}

	for _, target := range targets {
		for _, branch := range collectSubtree(state, target) {
			if seen[branch] {
				continue
			}
			seen[branch] = true
			subtreeOrder = append(subtreeOrder, branch)
		}
	}

	for _, branch := range subtreeOrder {
		headOID, err := runtime.Git.ResolveRef(runtime.Context, branch)
		if err != nil {
			return nil, err
		}
		snapshotHeads[branch] = headOID
	}

	rewritten := map[string]bool{}
	steps := make([]store.RestackStep, 0, len(subtreeOrder))

	for _, branch := range subtreeOrder {
		record := state.Branches[branch]
		if record.Restack.LastParentHeadOID == "" {
			return nil, fmt.Errorf("branch %q has no recorded restack anchor; use `stack track` or repair metadata first", branch)
		}

		previousParentHead := record.Restack.LastParentHeadOID
		parentWillRewrite := rewritten[record.ParentBranch]
		if parentWillRewrite {
			snapshotHead, ok := snapshotHeads[record.ParentBranch]
			if !ok || snapshotHead == "" {
				return nil, fmt.Errorf("branch %q cannot restack because parent %q has no recorded local head", branch, record.ParentBranch)
			}
			previousParentHead = snapshotHead
		}

		validAnchor, err := runtime.Git.IsAncestor(runtime.Context, previousParentHead, branch)
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

		if parentOID == record.Restack.LastParentHeadOID && !parentWillRewrite {
			continue
		}

		steps = append(steps, store.RestackStep{
			Branch:             branch,
			Parent:             record.ParentBranch,
			PreviousParentHead: previousParentHead,
			PreviousBranchHead: resolveOID(runtime, branch),
		})
		rewritten[branch] = true
	}

	return steps, nil
}

func runRestackPlan(runtime *stackruntime.Runtime, state store.RepoState, steps []store.RestackStep, baseOperation *store.OperationState) error {
	paths, err := runtime.Store.ResolvePaths(runtime.Context)
	if err != nil {
		return err
	}
	if err := ensureStateWritable(state); err != nil {
		return err
	}

	originalBranch := ""
	originalHead := ""
	startedAt := time.Now().UTC().Format(time.RFC3339)
	if baseOperation != nil {
		originalBranch = baseOperation.OriginalBranch
		originalHead = baseOperation.OriginalHEAD
		startedAt = baseOperation.StartedAt
	}
	if originalBranch == "" {
		originalBranch, _ = runtime.Git.CurrentBranch(runtime.Context)
	}
	if originalHead == "" {
		originalHead, _ = runtime.Git.ResolveRef(runtime.Context, "HEAD")
	}

	for index, step := range steps {
		activeHead, _ := runtime.Git.ResolveRef(runtime.Context, step.Branch)
		operation := store.OperationState{
			Type:           "restack",
			Repo:           state.Repo,
			RepositoryRoot: paths.Root,
			WorktreeGitDir: paths.GitDir,
			OriginalBranch: strings.TrimSpace(originalBranch),
			OriginalHEAD:   strings.TrimSpace(originalHead),
			StartedAt:      startedAt,
			ActiveHead:     strings.TrimSpace(activeHead),
			Active:         step,
			Pending:        append([]store.RestackStep(nil), steps[index+1:]...),
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

	if err := runtime.Store.ClearOperation(runtime.Context); err != nil {
		return err
	}
	return restoreOriginalBranch(runtime, store.OperationState{
		OriginalBranch: originalBranch,
		OriginalHEAD:   originalHead,
	})
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

func cloneRepoState(state store.RepoState) store.RepoState {
	cloned := state
	cloned.Branches = make(map[string]store.BranchRecord, len(state.Branches))
	for branch, record := range state.Branches {
		cloned.Branches[branch] = record
	}
	return cloned
}

func chooseString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func ensureStateWritable(state store.RepoState) error {
	validation := stack.ValidateState(state)
	messages := make([]string, 0)
	for _, issue := range validation.RepoIssues {
		if issue.Severity == stack.SeverityError {
			messages = append(messages, issue.Message)
		}
	}

	for branch, issues := range validation.BranchIssues {
		if !stack.HasErrors(issues) {
			continue
		}
		messages = append(messages, fmt.Sprintf("%s: %s", branch, issues[0].Message))
	}

	if len(messages) == 0 {
		return nil
	}

	return fmt.Errorf("stack metadata must be repaired before continuing: %s", strings.Join(messages, "; "))
}

func ensureNoPendingOperation(runtime *stackruntime.Runtime) error {
	if _, err := runtime.Store.ReadOperation(runtime.Context); err == nil {
		return fmt.Errorf("an interrupted operation is already recorded; use `stack continue` or `stack abort` first")
	}

	rebaseInProgress, err := runtime.Git.RebaseInProgress(runtime.Context)
	if err != nil {
		return err
	}
	if rebaseInProgress {
		return fmt.Errorf("git rebase is already in progress; use `stack continue` or `stack abort` first")
	}

	return nil
}

func validateOperationContext(runtime *stackruntime.Runtime, operation store.OperationState) error {
	paths, err := runtime.Store.ResolvePaths(runtime.Context)
	if err != nil {
		return err
	}
	if operation.RepositoryRoot != paths.Root {
		return fmt.Errorf("operation journal belongs to %s, current repo is %s", operation.RepositoryRoot, paths.Root)
	}
	if operation.WorktreeGitDir != paths.GitDir {
		return fmt.Errorf("operation journal belongs to a different worktree; resume from the original worktree")
	}

	rebaseInProgress, err := runtime.Git.RebaseInProgress(runtime.Context)
	if err != nil {
		return err
	}
	if !rebaseInProgress {
		return fmt.Errorf("operation journal exists but git rebase is not in progress")
	}

	currentBranch, err := runtime.Git.CurrentBranch(runtime.Context)
	if err != nil {
		return err
	}
	currentBranch = strings.TrimSpace(currentBranch)
	if currentBranch != "" && currentBranch != operation.Active.Branch {
		return fmt.Errorf("current branch is %q; expected %q for `stack continue`", currentBranch, operation.Active.Branch)
	}

	if operation.ActiveHead != "" && currentBranch != "" {
		activeHead, err := runtime.Git.ResolveRef(runtime.Context, operation.Active.Branch)
		if err != nil {
			return err
		}
		if activeHead != operation.ActiveHead {
			return fmt.Errorf("branch %q moved since the restack stopped; repair manually before continuing", operation.Active.Branch)
		}
	}

	return nil
}

func restoreOriginalBranch(runtime *stackruntime.Runtime, operation store.OperationState) error {
	originalBranch := strings.TrimSpace(operation.OriginalBranch)
	if originalBranch == "" {
		return nil
	}
	currentBranch, err := runtime.Git.CurrentBranch(runtime.Context)
	if err != nil {
		return nil
	}
	if strings.TrimSpace(currentBranch) == originalBranch {
		return nil
	}
	if !runtime.Git.BranchExists(runtime.Context, originalBranch) {
		return nil
	}
	return runtime.Git.Switch(runtime.Context, originalBranch)
}

func refreshTrackedPR(runtime *stackruntime.Runtime, state store.RepoState, branch string, record store.BranchRecord) (store.BranchRecord, error) {
	if record.PR.Number > 0 {
		pr, err := runtime.GitHub.ViewPR(runtime.Context, record.PR.Number)
		if err != nil {
			return record, fmt.Errorf("tracked PR #%d for %q could not be loaded: %w", record.PR.Number, branch, err)
		}
		record.PR = pr
		return record, nil
	}

	pr, err := runtime.GitHub.FindPRByHead(runtime.Context, branch)
	if err != nil {
		return record, err
	}
	if pr.Number > 0 {
		if pr.Repo == "" {
			pr.Repo = state.Repo
		}
		record.PR = pr
	}

	return record, nil
}

func validateRemoteAgreement(branch string, record store.BranchRecord, localHeadOID string, remoteHeadOID string, remoteExists bool) error {
	if !remoteExists {
		return nil
	}
	if remoteHeadOID == localHeadOID {
		return nil
	}
	if record.PR.LastSeenHeadOID != "" && record.PR.LastSeenHeadOID != remoteHeadOID {
		return fmt.Errorf("remote branch %q changed from %s to %s; refusing to overwrite without repair", branch, record.PR.LastSeenHeadOID, remoteHeadOID)
	}
	return fmt.Errorf("remote branch %q is at %s while local is %s; refusing to overwrite stale remote state", branch, remoteHeadOID, localHeadOID)
}

func validateTrackedPR(branch string, record store.BranchRecord) error {
	if record.PR.Number == 0 {
		return nil
	}
	if record.PR.State == "CLOSED" {
		return fmt.Errorf("tracked PR for %q is closed; repair local metadata before submitting again", branch)
	}
	if record.PR.State == "MERGED" {
		return fmt.Errorf("tracked PR for %q is already merged; run `stack sync` before submitting again", branch)
	}
	if record.PR.HeadRefName != "" && record.PR.HeadRefName != branch {
		return fmt.Errorf("tracked PR for %q points at head %q", branch, record.PR.HeadRefName)
	}
	return nil
}

func resolveOID(runtime *stackruntime.Runtime, ref string) string {
	oid, err := runtime.Git.ResolveRef(runtime.Context, ref)
	if err != nil {
		return ""
	}
	return oid
}

func init() {
	pflag.CommandLine.SortFlags = false
}
