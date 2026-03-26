package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
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
			return runStatus(runtime, false, cmd.OutOrStdout())
		},
	}

	rootCmd.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.RenderMarkdown(docs.CommandMarkdown(cmd)))
	})

	rootCmd.AddCommand(
		newInitCommand(runtime),
		newCreateCommand(runtime),
		newTrackCommand(runtime),
		newAdoptCommand(runtime),
		newComposeCommand(runtime),
		newVerifyCommand(runtime),
		newCloseoutCommand(runtime),
		newSupersedeCommand(runtime),
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
			return runStatus(runtime, false, cmd.OutOrStdout())
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
			} else {
				repo, _ = runtime.Git.RemoteRepoSlug(runtime.Context, chooseString(remote, "origin"))
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
			if err := ensureCreateParentAllowed(state, parent); err != nil {
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
			if err := ensureTrackedParentAllowed(state, parent); err != nil {
				return err
			}
			if err := stack.EnsureBranchCanParent(state, branch, parent); err != nil {
				return err
			}

			parentOID, err := trackRestackAnchor(runtime, branch, parent)
			if err != nil {
				return err
			}
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

func newAdoptCommand(runtime *stackruntime.Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Adopt existing pull requests into explicit stack metadata",
		Long:  "Use explicit operator-chosen parents to adopt existing PR heads into the local stack graph.",
	}

	cmd.AddCommand(newAdoptPRCommand(runtime))
	return cmd
}

func newAdoptPRCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var parent string
	var yes bool

	cmd := &cobra.Command{
		Use:   "pr <number>",
		Short: "Adopt one open pull request into the stack graph",
		Long:  "Look up one open pull request, optionally fetch its head branch locally, then track it under an explicit parent branch.",
		Example: strings.TrimSpace(`
stack adopt pr 353 --parent main
stack adopt pr 354 --parent pr/353 --yes
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if parent == "" {
				return fmt.Errorf("--parent is required")
			}

			prNumber, err := strconv.Atoi(args[0])
			if err != nil || prNumber <= 0 {
				return fmt.Errorf("invalid pull request number %q", args[0])
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

			if parent != state.Trunk && !runtime.Git.BranchExists(runtime.Context, parent) {
				return fmt.Errorf("parent branch %q does not exist locally", parent)
			}
			if err := ensureTrackedParentAllowed(state, parent); err != nil {
				return err
			}

			pr, err := runtime.GitHub.ViewPR(runtime.Context, prNumber)
			if err != nil {
				return err
			}
			if pr.State != "" && pr.State != "OPEN" {
				return fmt.Errorf("pull request #%d is %s; only open PRs can be adopted", prNumber, strings.ToLower(pr.State))
			}

			branch := strings.TrimSpace(pr.HeadRefName)
			if branch == "" {
				return fmt.Errorf("pull request #%d has no head branch name", prNumber)
			}
			if _, exists := state.Branches[branch]; exists {
				return fmt.Errorf("branch %q is already tracked", branch)
			}
			if err := stack.EnsureBranchCanParent(state, branch, parent); err != nil {
				return err
			}

			branchExists := runtime.Git.BranchExists(runtime.Context, branch)
			localHeadOID := ""
			if branchExists {
				localHeadOID, err = runtime.Git.ResolveRef(runtime.Context, branch)
				if err != nil {
					return err
				}
				localHeadOID = strings.TrimSpace(localHeadOID)
			}
			prHeadOID := strings.TrimSpace(pr.LastSeenHeadOID)
			needsFetch := !branchExists
			refreshHead := branchExists && localHeadOID != "" && prHeadOID != "" && localHeadOID != prHeadOID
			preview := []string{
				fmt.Sprintf("pr: #%d", pr.Number),
				fmt.Sprintf("branch: %s", branch),
				fmt.Sprintf("parent: %s", parent),
			}
			if needsFetch {
				preview = append(preview, fmt.Sprintf("fetch: %s/%s -> %s", state.DefaultRemote, branch, branch))
			} else if refreshHead {
				preview = append(preview, fmt.Sprintf("refresh: local head %s -> PR head %s", shortOID(localHeadOID), shortOID(prHeadOID)))
			}
			if pr.BaseRefName != "" && pr.BaseRefName != parent {
				preview = append(preview, fmt.Sprintf("note: PR #%d currently targets %s; `stack submit %s` will retarget it later", pr.Number, pr.BaseRefName, branch))
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Adopt PR preview", preview))

			if !yes {
				confirmed, err := forms.Confirm("Adopt pull request", "This may fetch or refresh a remote branch head and will update local stack metadata.")
				if err != nil {
					return err
				}
				if !confirmed {
					return fmt.Errorf("adopt cancelled")
				}
			}

			if needsFetch {
				if err := runtime.Git.FetchBranch(runtime.Context, state.DefaultRemote, branch, branch); err != nil {
					return fmt.Errorf("fetch pull request #%d head %q from %s: %w", pr.Number, branch, state.DefaultRemote, err)
				}
				if prHeadOID != "" {
					fetchedHeadOID, err := runtime.Git.ResolveRef(runtime.Context, branch)
					if err != nil {
						return err
					}
					if strings.TrimSpace(fetchedHeadOID) != prHeadOID {
						return fmt.Errorf("fetched pull request #%d head %q from %s, but local branch %q now points to %s instead of expected PR head %s", pr.Number, branch, state.DefaultRemote, branch, shortOID(strings.TrimSpace(fetchedHeadOID)), shortOID(prHeadOID))
					}
				}
			}
			if refreshHead {
				currentBranch, err := runtime.Git.CurrentBranch(runtime.Context)
				if err != nil {
					return err
				}
				if strings.TrimSpace(currentBranch) == branch {
					return fmt.Errorf("local branch %q points to %s but pull request #%d is at %s; switch away from %s or delete the local branch, then rerun `stack adopt pr %d --parent %s`", branch, shortOID(localHeadOID), pr.Number, shortOID(prHeadOID), branch, pr.Number, parent)
				}
				if err := runtime.Git.FetchBranchForce(runtime.Context, state.DefaultRemote, branch, branch); err != nil {
					return fmt.Errorf("refresh pull request #%d head %q from %s: %w", pr.Number, branch, state.DefaultRemote, err)
				}
				refreshedHeadOID, err := runtime.Git.ResolveRef(runtime.Context, branch)
				if err != nil {
					return err
				}
				if strings.TrimSpace(refreshedHeadOID) != prHeadOID {
					return fmt.Errorf("refreshed pull request #%d head %q from %s, but local branch %q now points to %s instead of expected PR head %s", pr.Number, branch, state.DefaultRemote, branch, shortOID(strings.TrimSpace(refreshedHeadOID)), shortOID(prHeadOID))
				}
			}

			parentOID, usedMergeBase, err := trackRestackAnchorDetail(runtime, branch, parent)
			if err != nil {
				return err
			}

			record := store.BranchRecord{
				ParentBranch: parent,
				RemoteName:   state.DefaultRemote,
				PR:           pr,
				Restack: store.RestackMetadata{
					LastParentHeadOID: parentOID,
				},
			}
			plannedState := cloneRepoState(state)
			plannedState.Branches[branch] = record
			if err := ensureStateWritable(plannedState); err != nil {
				return err
			}
			if err := runtime.Store.WriteState(runtime.Context, plannedState); err != nil {
				return err
			}

			lines := []string{
				fmt.Sprintf("branch: %s", branch),
				fmt.Sprintf("parent: %s", parent),
				fmt.Sprintf("pr: #%d", pr.Number),
			}
			if needsFetch {
				lines = append(lines, fmt.Sprintf("fetched: %s/%s", state.DefaultRemote, branch))
			}
			if refreshHead {
				lines = append(lines, fmt.Sprintf("refreshed: %s/%s -> %s", state.DefaultRemote, branch, shortOID(prHeadOID)))
			}
			if usedMergeBase {
				lines = append(lines, "restack anchor: merge-base with the selected parent")
			} else {
				lines = append(lines, "restack anchor: current parent head")
			}
			if pr.BaseRefName != "" && pr.BaseRefName != parent {
				lines = append(lines, fmt.Sprintf("next: run `stack submit %s` when you want GitHub to target %s", branch, parent))
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Adopted pull request", lines))
			return nil
		},
	}

	cmd.Flags().StringVar(&parent, "parent", "", "Parent branch or trunk")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation")
	return cmd
}

func newComposeCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var branches []string
	var from string
	var to string
	var ticketValues []string
	var openPR bool
	var draft bool
	var title string
	var body string
	var yes bool

	cmd := &cobra.Command{
		Use:   "compose <name>",
		Short: "Compose a strict landing branch from selected tracked branches",
		Long:  "Create one ordinary local landing branch from an explicit linear branch selection and replay only the selected commits in order.",
		Example: strings.TrimSpace(`
stack compose discovery-core --from feature/a --to feature/c
stack compose discovery-core --from feature/a --to feature/c --ticket LNHACK-66 --ticket LNHACK-67 --open-pr
stack compose discovery-core --branches feature/a --branches feature/b --yes
		`),
		Args: cobra.ExactArgs(1),
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

			selectedBranches, err := resolveComposeBranches(state, branches, from, to)
			if err != nil {
				return err
			}
			tickets, err := parseTicketRefs(ticketValues)
			if err != nil {
				return err
			}

			plan, err := buildComposePlan(runtime, state, composeDestinationBranch(args[0]), selectedBranches)
			if err != nil {
				return err
			}

			preview := renderComposePreview(plan)
			if len(tickets) > 0 {
				preview = append(preview, fmt.Sprintf("tickets: %s", strings.Join(tickets, ", ")))
			}
			if openPR {
				preview = append(preview, "push landing branch and open or refresh landing PR")
				if strings.TrimSpace(title) != "" {
					preview = append(preview, fmt.Sprintf("pr title: %q", strings.TrimSpace(title)))
				}
				if strings.TrimSpace(body) != "" {
					preview = append(preview, "pr body: explicit command value")
				}
				if draft {
					preview = append(preview, "pr draft: true")
				}
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Compose preview", preview))

			if !yes {
				confirmText := "This creates a new local branch and cherry-picks the selected commits onto trunk."
				if openPR {
					confirmText = "This creates a new local branch, cherry-picks the selected commits onto trunk, pushes the landing branch, and may create or edit a GitHub PR."
				}
				confirmed, err := forms.Confirm("Compose landing branch", confirmText)
				if err != nil {
					return err
				}
				if !confirmed {
					return fmt.Errorf("compose cancelled")
				}
			}

			if err := runtime.Git.SwitchCreateFrom(runtime.Context, plan.Destination, plan.Base); err != nil {
				return err
			}

			for _, branchPlan := range plan.Branches {
				for _, commit := range branchPlan.Commits {
					if err := runtime.Git.CherryPick(runtime.Context, commit.OID); err != nil {
						return fmt.Errorf("compose stopped while replaying %s from %s onto %s: %w; inspect %s, resolve the cherry-pick, or run `git cherry-pick --abort` manually", shortOID(commit.OID), branchPlan.Name, plan.Destination, err, plan.Destination)
					}
				}
			}

			landing := store.LandingRecord{
				BaseBranch:     plan.Base,
				SourceBranches: append([]string(nil), selectedBranches...),
				Tickets:        tickets,
				CreatedAt:      time.Now().UTC().Format(time.RFC3339),
			}
			state.Landings[plan.Destination] = landing
			if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
				return err
			}

			if openPR {
				metadata := resolveComposePRMetadata(plan, tickets, title, body)
				remoteHeadOID, remoteExists, err := runtime.Git.RemoteBranchOID(runtime.Context, state.DefaultRemote, plan.Destination)
				if err != nil {
					return err
				}
				if err := runtime.Git.PushBranch(runtime.Context, state.DefaultRemote, plan.Destination, remoteHeadOID); err != nil {
					return err
				}

				pr, created, err := openOrUpdateLandingPR(runtime, state, plan.Destination, metadata.Title, metadata.Body, draft)
				if err != nil {
					return err
				}
				landing.LandingPRNumber = pr.Number
				state.Landings[plan.Destination] = landing

				if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
					return err
				}

				lines := []string{
					fmt.Sprintf("branch: %s", plan.Destination),
					fmt.Sprintf("base: %s", plan.Base),
					fmt.Sprintf("replayed commits: %d", plan.CommitCount()),
					fmt.Sprintf("tickets: %s", chooseComposeTicketSummary(tickets)),
					fmt.Sprintf("landing PR: #%d", pr.Number),
				}
				if remoteExists {
					lines = append(lines, fmt.Sprintf("pushed: updated %s/%s", state.DefaultRemote, plan.Destination))
				} else {
					lines = append(lines, fmt.Sprintf("pushed: created %s/%s", state.DefaultRemote, plan.Destination))
				}
				if created {
					lines = append(lines, fmt.Sprintf("landing PR url: %s", pr.URL))
				} else {
					lines = append(lines, "landing PR: refreshed existing GitHub PR")
				}
				lines = append(lines, plan.Warnings...)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Composed landing branch", lines))
				return nil
			}

			lines := []string{
				fmt.Sprintf("branch: %s", plan.Destination),
				fmt.Sprintf("base: %s", plan.Base),
				fmt.Sprintf("replayed commits: %d", plan.CommitCount()),
				fmt.Sprintf("tickets: %s", chooseComposeTicketSummary(tickets)),
			}
			lines = append(lines, plan.Warnings...)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Composed landing branch", lines))
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&branches, "branches", nil, "Tracked branches to include in explicit order")
	cmd.Flags().StringVar(&from, "from", "", "Bottom branch of a contiguous tracked path")
	cmd.Flags().StringVar(&to, "to", "", "Top branch of a contiguous tracked path")
	cmd.Flags().StringArrayVar(&ticketValues, "ticket", nil, "Ticket references to attach to the landing branch, comma-separated or repeated")
	cmd.Flags().BoolVar(&openPR, "open-pr", false, "Push the landing branch and create or refresh its GitHub pull request")
	cmd.Flags().BoolVar(&draft, "draft", false, "Create the landing PR as a draft when used with --open-pr")
	cmd.Flags().StringVar(&title, "title", "", "Explicit landing PR title when used with --open-pr")
	cmd.Flags().StringVar(&body, "body", "", "Explicit landing PR body when used with --open-pr")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation")
	return cmd
}

func newVerifyCommand(runtime *stackruntime.Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Attach and inspect lightweight verification records",
		Long:  "Store local-first verification evidence for tracked branches or composed landing branches.",
	}

	cmd.AddCommand(
		newVerifyAddCommand(runtime),
		newVerifyListCommand(runtime),
	)
	return cmd
}

func newVerifyAddCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var checkType string
	var identifier string
	var runID string
	var note string
	var score int
	var passed bool
	var failed bool

	cmd := &cobra.Command{
		Use:   "add <branch>",
		Short: "Record one verification result for a branch",
		Long:  "Record local verification evidence against the current head of a tracked branch or composed landing branch.",
		Example: strings.TrimSpace(`
stack verify add stack/discovery-core --type sim --run-id abc123 --passed --score 100
stack verify add feature/a --type manual --identifier smoke-check-42 --failed --note "deploy blocked"
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := strings.TrimSpace(args[0])
			if branch == "" {
				return fmt.Errorf("branch is required")
			}
			if strings.TrimSpace(checkType) == "" {
				return fmt.Errorf("--type is required")
			}
			if passed == failed {
				return fmt.Errorf("set exactly one of --passed or --failed")
			}
			if strings.TrimSpace(identifier) != "" && strings.TrimSpace(runID) != "" {
				return fmt.Errorf("use either --identifier or --run-id, not both")
			}

			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}
			if !runtime.Git.BranchExists(runtime.Context, branch) {
				return fmt.Errorf("branch %q does not exist locally", branch)
			}

			headOID, err := runtime.Git.ResolveRef(runtime.Context, branch)
			if err != nil {
				return err
			}

			resolvedIdentifier := strings.TrimSpace(identifier)
			if resolvedIdentifier == "" {
				resolvedIdentifier = strings.TrimSpace(runID)
			}

			record := store.VerificationRecord{
				CheckType:  strings.TrimSpace(checkType),
				Identifier: resolvedIdentifier,
				Passed:     passed,
				HeadOID:    headOID,
				RecordedAt: time.Now().UTC().Format(time.RFC3339),
				Note:       strings.TrimSpace(note),
			}
			if cmd.Flags().Changed("score") {
				record.Score = &score
			}

			state.Verifications[branch] = append(state.Verifications[branch], record)
			if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
				return err
			}

			lines := []string{
				fmt.Sprintf("branch: %s", branch),
				fmt.Sprintf("type: %s", record.CheckType),
				fmt.Sprintf("result: %s", verificationResult(record)),
				fmt.Sprintf("head: %s", shortOID(record.HeadOID)),
			}
			if record.Identifier != "" {
				lines = append(lines, fmt.Sprintf("identifier: %s", record.Identifier))
			}
			if record.Score != nil {
				lines = append(lines, fmt.Sprintf("score: %d", *record.Score))
			}
			if record.Note != "" {
				lines = append(lines, fmt.Sprintf("note: %s", record.Note))
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Verification recorded", lines))
			return nil
		},
	}

	cmd.Flags().StringVar(&checkType, "type", "", "Verification type such as sim, unit, integration, manual, deploy, or smoke")
	cmd.Flags().StringVar(&identifier, "identifier", "", "Verification identifier such as a run id, URL, or external check reference")
	cmd.Flags().StringVar(&runID, "run-id", "", "Convenience alias for --identifier when recording a run id")
	cmd.Flags().StringVar(&note, "note", "", "Optional operator note")
	cmd.Flags().IntVar(&score, "score", 0, "Optional numeric score")
	cmd.Flags().BoolVar(&passed, "passed", false, "Mark the verification as passed")
	cmd.Flags().BoolVar(&failed, "failed", false, "Mark the verification as failed")
	return cmd
}

func newVerifyListCommand(runtime *stackruntime.Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <branch>",
		Short: "List stored verification records for a branch",
		Long:  "Show local verification evidence stored for a tracked branch or composed landing branch.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := strings.TrimSpace(args[0])
			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}

			records := append([]store.VerificationRecord(nil), state.Verifications[branch]...)
			if len(records) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Verification records", []string{
					fmt.Sprintf("branch: %s", branch),
					"no verification records stored",
				}))
				return nil
			}

			lines := []string{fmt.Sprintf("branch: %s", branch)}
			for index := len(records) - 1; index >= 0; index-- {
				record := records[index]
				line := fmt.Sprintf("%s  %s  %s", record.RecordedAt, record.CheckType, verificationResult(record))
				if record.Identifier != "" {
					line += fmt.Sprintf("  %s", record.Identifier)
				}
				lines = append(lines, line)
				lines = append(lines, fmt.Sprintf("  head: %s", shortOID(record.HeadOID)))
				if record.Score != nil {
					lines = append(lines, fmt.Sprintf("  score: %d", *record.Score))
				}
				if record.Note != "" {
					lines = append(lines, fmt.Sprintf("  note: %s", record.Note))
				}
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Verification records", lines))
			return nil
		},
	}

	return cmd
}

func newCloseoutCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var apply bool
	var yes bool

	cmd := &cobra.Command{
		Use:   "closeout <landing-branch>",
		Short: "Plan read-only post-merge closeout for a landing branch",
		Long:  "Use recorded landing composition, pull request state, and verification records to show which original PRs and explicit tickets are safe to close now versus still blocked on deploy checks.",
		Example: strings.TrimSpace(`
stack closeout stack/discovery-core
stack closeout stack/discovery-core --apply --yes
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}

			plan, err := buildCloseoutPlan(runtime, state, strings.TrimSpace(args[0]))
			if err != nil {
				return err
			}

			if apply {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Closeout plan", renderCloseoutPlan(plan)))
				if !yes {
					confirmed, err := forms.Confirm("Apply closeout", "This may close superseded GitHub pull requests that are marked safe to close.")
					if err != nil {
						return err
					}
					if !confirmed {
						return fmt.Errorf("closeout cancelled")
					}
				}

				applied, nextState, err := applyCloseoutPlan(runtime, state, plan)
				if err != nil {
					return err
				}
				if err := runtime.Store.WriteState(runtime.Context, nextState); err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Closeout applied", applied))
				return nil
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Closeout plan", renderCloseoutPlan(plan)))
			return nil
		},
	}

	cmd.Flags().BoolVar(&apply, "apply", false, "Close superseded PRs that are explicitly marked safe to close after landing merge")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation when used with --apply")
	return cmd
}

func newSupersedeCommand(runtime *stackruntime.Runtime) *cobra.Command {
	var landingBranch string
	var prValues []string
	var noComment bool
	var closeAfterMerge bool
	var yes bool

	cmd := &cobra.Command{
		Use:   "supersede",
		Short: "Mark original PRs as superseded by a landing PR",
		Long:  "Record explicit superseded PR linkage in local landing metadata and optionally comment on the original PRs with the landing PR reference.",
		Example: strings.TrimSpace(`
stack supersede --landing stack/discovery-core --prs 353,354,363,364
stack supersede --landing stack/discovery-core --prs 353 --prs 354 --no-comment --yes
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(landingBranch) == "" {
				return fmt.Errorf("--landing is required")
			}

			prNumbers, err := parsePRNumbers(prValues)
			if err != nil {
				return err
			}
			if len(prNumbers) == 0 {
				return fmt.Errorf("at least one PR number is required via --prs")
			}

			state, err := runtime.Store.ReadState(runtime.Context)
			if err != nil {
				return err
			}

			plan, err := buildSupersedePlan(runtime, state, strings.TrimSpace(landingBranch), prNumbers)
			if err != nil {
				return err
			}

			lines := []string{
				fmt.Sprintf("landing branch: %s", plan.LandingBranch),
				fmt.Sprintf("landing PR: #%d", plan.LandingPR.Number),
			}
			for _, pr := range plan.SupersededPRs {
				lines = append(lines, fmt.Sprintf("superseded PR: #%d %s", pr.Number, strings.ToLower(pr.State)))
			}
			if noComment {
				lines = append(lines, "comments: skipped by --no-comment")
			} else {
				lines = append(lines, fmt.Sprintf("comments: %d GitHub PR comments will be posted", len(plan.SupersededPRs)))
			}
			if closeAfterMerge {
				lines = append(lines, "close originals: after landing merge via `stack closeout --apply`")
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Supersede preview", lines))

			if !yes {
				confirmed, err := forms.Confirm("Supersede PRs", "This records superseded PR linkage locally and may post comments on GitHub.")
				if err != nil {
					return err
				}
				if !confirmed {
					return fmt.Errorf("supersede cancelled")
				}
			}

			landing := state.Landings[plan.LandingBranch]
			landing.SupersededPRs = append([]int(nil), prNumbers...)
			landing.LandingPRNumber = plan.LandingPR.Number
			landing.CloseSupersededAfterMerge = closeAfterMerge
			landing.SupersededAt = time.Now().UTC().Format(time.RFC3339)
			state.Landings[plan.LandingBranch] = landing
			if err := runtime.Store.WriteState(runtime.Context, state); err != nil {
				return err
			}

			if !noComment {
				body := fmt.Sprintf("This PR is superseded by landing PR #%d (%s) from `%s`.", plan.LandingPR.Number, plan.LandingPR.URL, plan.LandingBranch)
				for _, pr := range plan.SupersededPRs {
					if err := runtime.GitHub.CommentPR(runtime.Context, pr.Number, body); err != nil {
						return err
					}
				}
			}

			result := []string{
				fmt.Sprintf("landing branch: %s", plan.LandingBranch),
				fmt.Sprintf("landing PR: #%d", plan.LandingPR.Number),
				fmt.Sprintf("superseded PRs: %s", joinPRNumbers(prNumbers)),
			}
			if closeAfterMerge {
				result = append(result, "close originals: enabled for post-merge closeout")
			}
			if noComment {
				result = append(result, "github comments: skipped")
			} else {
				result = append(result, "github comments: posted")
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Superseded PRs recorded", result))
			return nil
		},
	}

	cmd.Flags().StringVar(&landingBranch, "landing", "", "Landing branch to use as the superseding target")
	cmd.Flags().StringArrayVar(&prValues, "prs", nil, "Original PR numbers, comma-separated or repeated")
	cmd.Flags().BoolVar(&noComment, "no-comment", false, "Record superseded linkage locally without posting GitHub comments")
	cmd.Flags().BoolVar(&closeAfterMerge, "close-after-merge", false, "Mark superseded PRs for later closure during closeout apply after the landing PR merges")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation")
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
			return runStatus(runtime, asJSON, cmd.OutOrStdout())
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
			if err := ensureTrackedParentAllowed(state, parent); err != nil {
				return err
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
					pr, err := runtime.GitHub.CreatePR(runtime.Context, record.ParentBranch, plan.Branch, plan.Metadata.Title, plan.Metadata.Body, draft)
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
						repairs = append(repairs, fmt.Sprintf("%s: remote branch exists but no PR is linked in local metadata; run `stack submit %s` to relink or create the PR", branch, branch))
					}
					continue
				}

				pr, err := runtime.GitHub.ViewPR(runtime.Context, record.PR.Number)
				if err != nil {
					repairs = append(repairs, fmt.Sprintf("%s: tracked PR #%d could not be loaded; run `stack status` and inspect the PR before resubmitting", branch, record.PR.Number))
					continue
				}
				record.PR = pr
				state.Branches[branch] = record

				if pr.HeadRefName != "" && pr.HeadRefName != branch {
					repairs = append(repairs, fmt.Sprintf("%s: PR head is %s, expected %s; repair or relink the PR before submitting again", branch, pr.HeadRefName, branch))
				}
				if pr.BaseRefName != "" && pr.BaseRefName != record.ParentBranch {
					repairs = append(repairs, fmt.Sprintf("%s: PR base is %s, expected %s; run `stack submit %s` to retarget it", branch, pr.BaseRefName, record.ParentBranch, branch))
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
					repairs = append(repairs, fmt.Sprintf("%s: remote branch is missing; run `stack submit %s` to republish it", branch, branch))
				}

				cleanMergedParent := pr.State == "MERGED" &&
					(pr.HeadRefName == "" || pr.HeadRefName == branch) &&
					(pr.BaseRefName == "" || pr.BaseRefName == record.ParentBranch)

				if pr.State == "MERGED" && !cleanMergedParent {
					repairs = append(repairs, fmt.Sprintf("%s: merged parent has drifted PR metadata; run `stack status` and inspect the merged PR before reparenting children", branch))
				}

				if cleanMergedParent {
					parentHeadOID := pr.LastSeenHeadOID
					for _, child := range stack.Children(state, branch) {
						childRecord := state.Branches[child]
						if parentHeadOID != "" && childRecord.Restack.LastParentHeadOID == parentHeadOID {
							repairs = append(repairs, fmt.Sprintf("%s: clean reparent %s -> %s", child, branch, record.ParentBranch))
						} else {
							repairs = append(repairs, fmt.Sprintf("%s: merged parent %s needs manual review before reparenting; repair the branch graph, then rerun `stack sync --apply`", child, branch))
							continue
						}
						if apply {
							childRecord.ParentBranch = record.ParentBranch
							state.Branches[child] = childRecord
						}
					}
				} else if pr.State == "CLOSED" {
					repairs = append(repairs, fmt.Sprintf("%s: PR is closed without merge; relink or clear local metadata before continuing", branch))
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
		Short: "Hand one verified trunk-bound PR or landing PR to GitHub auto-merge or merge queue",
		Long:  "Validate that one tracked trunk branch or recorded landing branch is ready for handoff, then ask GitHub to auto-merge or enqueue the PR using the current head commit.",
		Example: strings.TrimSpace(`
stack queue feature/a
stack queue stack/discovery-core
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

			plan, err := buildQueuePlan(runtime, state, args[0])
			if err != nil {
				return err
			}

			lines := []string{
				fmt.Sprintf("branch: %s", plan.Branch),
				fmt.Sprintf("pr: #%d", plan.PR.Number),
				fmt.Sprintf("strategy: %s", strategy),
				fmt.Sprintf("head: %s", plan.HeadOID),
			}
			if plan.IsLanding {
				lines = append(lines, "type: landing")
			} else {
				lines = append(lines, "type: tracked")
			}
			if plan.Verification != nil {
				lines = append(lines, fmt.Sprintf("verification: %s", renderQueueVerification(*plan.Verification)))
			}
			if len(plan.ExcludedPRs) > 0 {
				lines = append(lines, fmt.Sprintf("keep out of queue: %s", joinPRNumbers(plan.ExcludedPRs)))
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Queue handoff", lines))

			if !yes {
				confirmed, err := forms.Confirm("Queue branch", "This hands the current PR head to GitHub auto-merge or merge queue.")
				if err != nil {
					return err
				}
				if !confirmed {
					return fmt.Errorf("queue cancelled")
				}
			}

			if err := runtime.GitHub.MergePR(runtime.Context, plan.PR.Number, plan.HeadOID, strategy); err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.RenderPreview("Next steps", plan.NextSteps))
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

func runStatus(runtime *stackruntime.Runtime, asJSON bool, writer io.Writer) error {
	state, err := runtime.Store.ReadState(runtime.Context)
	if err != nil {
		return err
	}

	summary, err := stack.BuildSummary(runtime.Context, runtime.Git, state)
	if err != nil {
		return err
	}

	if asJSON {
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	}

	_, _ = fmt.Fprintln(writer, ui.RenderStatus(summary))
	return nil
}

type submitPlan struct {
	Branch       string
	LocalHead    string
	RemoteHead   string
	RemoteExists bool
	Metadata     submitPRMetadata
	Record       store.BranchRecord
	Preview      []string
}

type submitPRMetadata struct {
	Title       string
	Body        string
	TitleSource string
	BodySource  string
}

type composePlan struct {
	Destination string
	Base        string
	Branches    []composeBranchPlan
	Warnings    []string
}

type composeBranchPlan struct {
	Name    string
	Parent  string
	Commits []composeCommitPlan
}

type composeCommitPlan struct {
	OID     string
	Subject string
}

type composePRMetadata struct {
	Title string
	Body  string
}

type closeoutPlan struct {
	LandingBranch            string
	BaseBranch               string
	LandingPR                *store.PullRequest
	LandingPRAmbiguous       []store.PullRequest
	SourceBranches           []closeoutBranchPlan
	TicketsSafeToClose       []string
	TicketsPendingPostDeploy []string
	SupersededNow            []string
	SupersededPending        []string
	FollowUps                []string
}

type closeoutBranchPlan struct {
	Name string
	PR   *store.PullRequest
}

type supersedePlan struct {
	LandingBranch string
	LandingPR     store.PullRequest
	SupersededPRs []store.PullRequest
}

type queuePlan struct {
	Branch       string
	PR           store.PullRequest
	HeadOID      string
	IsLanding    bool
	Verification *queueVerification
	ExcludedPRs  []int
	NextSteps    []string
}

type queueVerification struct {
	Latest             store.VerificationRecord
	HeadMatchesCurrent bool
}

func (plan composePlan) CommitCount() int {
	total := 0
	for _, branchPlan := range plan.Branches {
		total += len(branchPlan.Commits)
	}
	return total
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
	metadata := submitPRMetadata{}
	if !remoteExists {
		preview = append(preview, "push new remote branch")
	} else if remoteHeadOID != localHeadOID {
		preview = append(preview, "force-push remote branch with explicit lease")
	} else {
		preview = append(preview, "remote branch already matches local head")
	}

	if record.PR.Number == 0 {
		metadata, err = resolveSubmitPRMetadata(runtime, branch, record.ParentBranch)
		if err != nil {
			return submitPlan{}, err
		}
		preview = append(preview, "create GitHub pull request")
		preview = append(preview, fmt.Sprintf("PR title: %q (%s)", metadata.Title, metadata.TitleSource))
		preview = append(preview, fmt.Sprintf("PR body: %s", metadata.BodySource))
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
		Metadata:     metadata,
		Record:       record,
		Preview:      preview,
	}, nil
}

func resolveSubmitPRMetadata(runtime *stackruntime.Runtime, branch string, parent string) (submitPRMetadata, error) {
	title, body, err := runtime.Git.CommitMessage(runtime.Context, branch)
	if err != nil {
		return submitPRMetadata{}, err
	}

	metadata := submitPRMetadata{
		Title:       strings.TrimSpace(title),
		Body:        strings.TrimSpace(body),
		TitleSource: "commit subject",
		BodySource:  "commit body",
	}

	if metadata.Title == "" {
		metadata.Title = branch
		metadata.TitleSource = "branch name fallback"
	}

	if metadata.Body == "" {
		metadata.Body = fmt.Sprintf("Stack branch `%s` targeting `%s`.\n\nGenerated by `stack submit` because the tip commit body was empty.", branch, parent)
		metadata.BodySource = "generated default"
	}

	return metadata, nil
}

func buildComposePlan(runtime *stackruntime.Runtime, state store.RepoState, destination string, branches []string) (composePlan, error) {
	if len(branches) == 0 {
		return composePlan{}, fmt.Errorf("compose needs at least one tracked branch")
	}
	if runtime.Git.BranchExists(runtime.Context, destination) {
		return composePlan{}, fmt.Errorf("branch %q already exists locally", destination)
	}
	if !runtime.Git.BranchExists(runtime.Context, state.Trunk) {
		return composePlan{}, fmt.Errorf("trunk branch %q does not exist locally", state.Trunk)
	}

	plan := composePlan{
		Destination: destination,
		Base:        state.Trunk,
		Branches:    make([]composeBranchPlan, 0, len(branches)),
	}

	for index, branch := range branches {
		record, ok := state.Branches[branch]
		if !ok {
			return composePlan{}, fmt.Errorf("branch %q is not tracked", branch)
		}
		if !runtime.Git.BranchExists(runtime.Context, branch) {
			return composePlan{}, fmt.Errorf("branch %q does not exist locally", branch)
		}
		if record.ParentBranch != state.Trunk && !runtime.Git.BranchExists(runtime.Context, record.ParentBranch) {
			return composePlan{}, fmt.Errorf("parent branch %q for %q does not exist locally", record.ParentBranch, branch)
		}
		if index > 0 && record.ParentBranch != branches[index-1] {
			return composePlan{}, fmt.Errorf("compose branches must form a contiguous parent chain; expected %q to parent %q, got %q", branches[index-1], branch, record.ParentBranch)
		}

		commits, err := runtime.Git.RangeCommits(runtime.Context, record.ParentBranch, branch)
		if err != nil {
			return composePlan{}, err
		}

		branchPlan := composeBranchPlan{
			Name:    branch,
			Parent:  record.ParentBranch,
			Commits: make([]composeCommitPlan, 0, len(commits)),
		}
		for _, commit := range commits {
			branchPlan.Commits = append(branchPlan.Commits, composeCommitPlan{
				OID:     commit.OID,
				Subject: commit.Subject,
			})
		}
		plan.Branches = append(plan.Branches, branchPlan)
	}

	firstParent := plan.Branches[0].Parent
	if firstParent != state.Trunk {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("warning: %s currently targets %s; compose will replay only its unique commits onto %s", plan.Branches[0].Name, firstParent, state.Trunk))
	}
	if plan.CommitCount() == 0 {
		return composePlan{}, fmt.Errorf("selected branches have no unique commits to compose")
	}

	return plan, nil
}

func renderComposePreview(plan composePlan) []string {
	lines := []string{
		fmt.Sprintf("destination: %s", plan.Destination),
		fmt.Sprintf("base: %s", plan.Base),
	}
	lines = append(lines, plan.Warnings...)

	for _, branchPlan := range plan.Branches {
		lines = append(lines, fmt.Sprintf("branch: %s (relative to %s)", branchPlan.Name, branchPlan.Parent))
		if len(branchPlan.Commits) == 0 {
			lines = append(lines, "  no unique commits")
			continue
		}

		for _, commit := range branchPlan.Commits {
			subject := commit.Subject
			if subject == "" {
				subject = "(empty subject)"
			}
			lines = append(lines, fmt.Sprintf("  %s %s", shortOID(commit.OID), subject))
		}
	}

	lines = append(lines, fmt.Sprintf("total commits: %d", plan.CommitCount()))
	return lines
}

func resolveComposePRMetadata(plan composePlan, tickets []string, explicitTitle string, explicitBody string) composePRMetadata {
	title := strings.TrimSpace(explicitTitle)
	if title == "" {
		if len(tickets) > 0 {
			title = fmt.Sprintf("Landing: %s", strings.Join(tickets, ", "))
		} else {
			title = fmt.Sprintf("Landing: %s", strings.TrimPrefix(plan.Destination, "stack/"))
		}
	}

	body := strings.TrimSpace(explicitBody)
	if body == "" {
		lines := []string{
			fmt.Sprintf("Composed landing branch `%s` targeting `%s`.", plan.Destination, plan.Base),
			"",
			"Included branches:",
		}
		for _, branchPlan := range plan.Branches {
			lines = append(lines, fmt.Sprintf("- `%s`", branchPlan.Name))
		}
		if len(tickets) > 0 {
			lines = append(lines, "", "Tickets:")
			for _, ticket := range tickets {
				lines = append(lines, fmt.Sprintf("- `%s`", ticket))
			}
		}
		body = strings.Join(lines, "\n")
	}

	return composePRMetadata{
		Title: title,
		Body:  body,
	}
}

func chooseComposeTicketSummary(tickets []string) string {
	if len(tickets) == 0 {
		return "none recorded"
	}
	return strings.Join(tickets, ", ")
}

func openOrUpdateLandingPR(runtime *stackruntime.Runtime, state store.RepoState, branch string, title string, body string, draft bool) (store.PullRequest, bool, error) {
	pr, err := runtime.GitHub.FindPRByHead(runtime.Context, branch)
	if err != nil {
		return store.PullRequest{}, false, err
	}
	if pr.Number == 0 {
		created, err := runtime.GitHub.CreatePR(runtime.Context, state.Trunk, branch, title, body, draft)
		return created, true, err
	}

	if err := runtime.GitHub.EditPR(runtime.Context, pr.Number, state.Trunk, title, body); err != nil {
		return store.PullRequest{}, false, err
	}
	updated, err := runtime.GitHub.ViewPR(runtime.Context, pr.Number)
	return updated, false, err
}

func resolveComposeBranches(state store.RepoState, explicit []string, from string, to string) ([]string, error) {
	if len(explicit) > 0 {
		if from != "" || to != "" {
			return nil, fmt.Errorf("use either --branches or --from/--to, not both")
		}
		return validateComposeExplicitBranches(state, explicit)
	}

	if from == "" && to == "" {
		return nil, fmt.Errorf("compose requires either repeated --branches or both --from and --to")
	}
	if from == "" || to == "" {
		return nil, fmt.Errorf("compose requires both --from and --to")
	}

	if _, ok := state.Branches[from]; !ok {
		return nil, fmt.Errorf("branch %q is not tracked", from)
	}
	if _, ok := state.Branches[to]; !ok {
		return nil, fmt.Errorf("branch %q is not tracked", to)
	}

	path := make([]string, 0)
	current := to
	for {
		path = append(path, current)
		if current == from {
			break
		}

		record, ok := state.Branches[current]
		if !ok || record.ParentBranch == "" || record.ParentBranch == state.Trunk {
			return nil, fmt.Errorf("branch %q is not an ancestor of %q in tracked metadata", from, to)
		}
		current = record.ParentBranch
	}

	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path, nil
}

func validateComposeExplicitBranches(state store.RepoState, explicit []string) ([]string, error) {
	seen := map[string]bool{}
	ordered := make([]string, 0, len(explicit))

	for index, branch := range explicit {
		if _, ok := state.Branches[branch]; !ok {
			return nil, fmt.Errorf("branch %q is not tracked", branch)
		}
		if seen[branch] {
			return nil, fmt.Errorf("branch %q was selected more than once", branch)
		}
		if index > 0 {
			parent := state.Branches[branch].ParentBranch
			if parent != explicit[index-1] {
				return nil, fmt.Errorf("compose branches must form a contiguous parent chain; expected %q to parent %q, got %q", explicit[index-1], branch, parent)
			}
		}
		seen[branch] = true
		ordered = append(ordered, branch)
	}

	return ordered, nil
}

func composeDestinationBranch(name string) string {
	trimmed := strings.TrimSpace(name)
	if strings.Contains(trimmed, "/") {
		return trimmed
	}
	return "stack/" + trimmed
}

func buildCloseoutPlan(runtime *stackruntime.Runtime, state store.RepoState, landingBranch string) (closeoutPlan, error) {
	landing, ok := state.Landings[landingBranch]
	if !ok {
		return closeoutPlan{}, fmt.Errorf("landing branch %q is not recorded in local compose metadata; compose it first or repair the landing record", landingBranch)
	}

	plan := closeoutPlan{
		LandingBranch:  landingBranch,
		BaseBranch:     landing.BaseBranch,
		SourceBranches: make([]closeoutBranchPlan, 0, len(landing.SourceBranches)),
	}

	landingPRs, err := resolveLandingPRs(runtime, landingBranch, landing)
	if err != nil {
		return closeoutPlan{}, err
	}
	switch len(landingPRs) {
	case 0:
	case 1:
		pr := landingPRs[0]
		landing.LandingPRNumber = pr.Number
		plan.LandingPR = &pr
	default:
		plan.LandingPRAmbiguous = append(plan.LandingPRAmbiguous, landingPRs...)
	}

	for _, branch := range landing.SourceBranches {
		branchPlan := closeoutBranchPlan{Name: branch}
		record, tracked := state.Branches[branch]
		if tracked {
			record, err = refreshTrackedPR(runtime, state, branch, record)
			if err != nil {
				plan.FollowUps = append(plan.FollowUps, fmt.Sprintf("source PR for %s could not be refreshed: %v", branch, err))
			} else if record.PR.Number > 0 {
				pr := record.PR
				branchPlan.PR = &pr
			}
		}
		plan.SourceBranches = append(plan.SourceBranches, branchPlan)
	}
	if len(landing.SupersededPRs) > 0 {
		plan.SourceBranches = plan.SourceBranches[:0]
		for _, number := range landing.SupersededPRs {
			pr, err := runtime.GitHub.ViewPR(runtime.Context, number)
			if err != nil {
				plan.FollowUps = append(plan.FollowUps, fmt.Sprintf("tracked superseded PR #%d could not be loaded; inspect it manually before closeout", number))
				continue
			}
			plan.SourceBranches = append(plan.SourceBranches, closeoutBranchPlan{
				Name: pr.HeadRefName,
				PR:   &pr,
			})
		}
	}

	tickets := append([]string(nil), landing.Tickets...)
	sort.Strings(tickets)

	if plan.LandingPR == nil {
		plan.FollowUps = append(plan.FollowUps, fmt.Sprintf("open or relink the landing PR for %s before treating source PRs as superseded", landingBranch))
	} else if plan.LandingPR.State != "MERGED" {
		plan.FollowUps = append(plan.FollowUps, fmt.Sprintf("wait for landing PR #%d to merge before closing source PRs as superseded", plan.LandingPR.Number))
	}
	if len(plan.LandingPRAmbiguous) > 0 {
		numbers := make([]string, 0, len(plan.LandingPRAmbiguous))
		for _, pr := range plan.LandingPRAmbiguous {
			numbers = append(numbers, fmt.Sprintf("#%d %s", pr.Number, strings.ToLower(pr.State)))
		}
		sort.Strings(numbers)
		plan.FollowUps = append(plan.FollowUps, fmt.Sprintf("resolve ambiguous landing PR ownership for %s: %s", landingBranch, strings.Join(numbers, ", ")))
	}

	deployReady, deployFollowUp := closeoutDeployState(state.Verifications[landingBranch], resolveOID(runtime, landingBranch))
	if deployFollowUp != "" {
		plan.FollowUps = append(plan.FollowUps, deployFollowUp)
	}
	if len(tickets) == 0 {
		plan.FollowUps = append(plan.FollowUps, fmt.Sprintf("no explicit tickets are recorded for %s; compose or repair the landing metadata with ticket refs before using closeout for ticket closure", landingBranch))
	}

	for _, branch := range plan.SourceBranches {
		if branch.PR == nil || branch.PR.Number == 0 {
			continue
		}
		label := fmt.Sprintf("#%d %s", branch.PR.Number, branch.Name)
		if plan.LandingPR != nil && plan.LandingPR.State == "MERGED" && branch.PR.State == "OPEN" {
			plan.SupersededNow = append(plan.SupersededNow, label)
			continue
		}
		if branch.PR.State == "OPEN" {
			plan.SupersededPending = append(plan.SupersededPending, label)
		}
	}

	if deployReady {
		plan.TicketsSafeToClose = append(plan.TicketsSafeToClose, tickets...)
	} else {
		plan.TicketsPendingPostDeploy = append(plan.TicketsPendingPostDeploy, tickets...)
	}

	sort.Strings(plan.SupersededNow)
	sort.Strings(plan.SupersededPending)
	sort.Strings(plan.TicketsSafeToClose)
	sort.Strings(plan.TicketsPendingPostDeploy)
	sort.Strings(plan.FollowUps)

	return plan, nil
}

func buildSupersedePlan(runtime *stackruntime.Runtime, state store.RepoState, landingBranch string, prNumbers []int) (supersedePlan, error) {
	landing, ok := state.Landings[landingBranch]
	if !ok {
		return supersedePlan{}, fmt.Errorf("landing branch %q is not recorded in local compose metadata; compose it first or repair the landing record", landingBranch)
	}
	if len(landing.SourceBranches) == 0 {
		return supersedePlan{}, fmt.Errorf("landing branch %q has no recorded source branches to supersede", landingBranch)
	}

	landingPRs, err := resolveLandingPRs(runtime, landingBranch, landing)
	if err != nil {
		return supersedePlan{}, err
	}
	if len(landingPRs) == 0 {
		return supersedePlan{}, fmt.Errorf("landing branch %q has no pull request yet; open the landing PR before superseding original PRs", landingBranch)
	}
	if len(landingPRs) > 1 {
		numbers := make([]string, 0, len(landingPRs))
		for _, pr := range landingPRs {
			numbers = append(numbers, fmt.Sprintf("#%d %s", pr.Number, strings.ToLower(pr.State)))
		}
		sort.Strings(numbers)
		return supersedePlan{}, fmt.Errorf("landing branch %q matches multiple PRs: %s; resolve the ambiguity before superseding originals", landingBranch, strings.Join(numbers, ", "))
	}

	plan := supersedePlan{
		LandingBranch: landingBranch,
		LandingPR:     landingPRs[0],
		SupersededPRs: make([]store.PullRequest, 0, len(prNumbers)),
	}
	sourceBranches := map[string]bool{}
	for _, branch := range landing.SourceBranches {
		sourceBranches[branch] = true
	}

	for _, number := range prNumbers {
		if number == plan.LandingPR.Number {
			return supersedePlan{}, fmt.Errorf("landing PR #%d cannot supersede itself", number)
		}
		pr, err := runtime.GitHub.ViewPR(runtime.Context, number)
		if err != nil {
			return supersedePlan{}, err
		}
		if !sourceBranches[pr.HeadRefName] {
			return supersedePlan{}, fmt.Errorf("pull request #%d head %q is not part of landing batch %q; expected one of: %s", pr.Number, pr.HeadRefName, landingBranch, strings.Join(landing.SourceBranches, ", "))
		}
		plan.SupersededPRs = append(plan.SupersededPRs, pr)
	}

	return plan, nil
}

func closeoutDeployState(records []store.VerificationRecord, currentHead string) (bool, string) {
	if currentHead == "" {
		return false, "landing branch is missing locally; cannot judge deploy closeout safely"
	}

	for index := len(records) - 1; index >= 0; index-- {
		record := records[index]
		if !isDeployVerification(record.CheckType) {
			continue
		}
		if record.HeadOID != "" && record.HeadOID != currentHead {
			continue
		}
		if record.Passed {
			return true, ""
		}
		return false, fmt.Sprintf("latest %s verification for the current landing head failed; resolve it before closing deploy-gated tickets", record.CheckType)
	}

	return false, "record a passed deploy or smoke verification for the current landing head before closing deploy-gated tickets"
}

func renderCloseoutPlan(plan closeoutPlan) []string {
	lines := []string{
		fmt.Sprintf("landing branch: %s", plan.LandingBranch),
		fmt.Sprintf("base: %s", plan.BaseBranch),
	}

	switch {
	case plan.LandingPR != nil:
		lines = append(lines, fmt.Sprintf("landing PR: #%d %s", plan.LandingPR.Number, plan.LandingPR.State))
	case len(plan.LandingPRAmbiguous) > 0:
		numbers := make([]string, 0, len(plan.LandingPRAmbiguous))
		for _, pr := range plan.LandingPRAmbiguous {
			numbers = append(numbers, fmt.Sprintf("#%d %s", pr.Number, strings.ToLower(pr.State)))
		}
		sort.Strings(numbers)
		lines = append(lines, fmt.Sprintf("landing PR: ambiguous (%s)", strings.Join(numbers, ", ")))
	default:
		lines = append(lines, "landing PR: not found")
	}

	lines = append(lines, "source branches:")
	for _, branch := range plan.SourceBranches {
		line := fmt.Sprintf("  %s", branch.Name)
		if branch.PR != nil {
			line += fmt.Sprintf("  PR #%d %s", branch.PR.Number, branch.PR.State)
		}
		lines = append(lines, line)
	}

	lines = append(lines, "superseded PRs safe to close now:")
	lines = append(lines, renderCloseoutList(plan.SupersededNow)...)

	lines = append(lines, "superseded PRs pending landing merge:")
	lines = append(lines, renderCloseoutList(plan.SupersededPending)...)

	lines = append(lines, "tickets safe to close now:")
	lines = append(lines, renderCloseoutList(plan.TicketsSafeToClose)...)

	lines = append(lines, "tickets pending post-deploy check:")
	lines = append(lines, renderCloseoutList(plan.TicketsPendingPostDeploy)...)

	lines = append(lines, "required follow-up checks:")
	lines = append(lines, renderCloseoutList(plan.FollowUps)...)

	return lines
}

func renderCloseoutList(values []string) []string {
	if len(values) == 0 {
		return []string{"  none"}
	}
	lines := make([]string, 0, len(values))
	for _, value := range values {
		lines = append(lines, fmt.Sprintf("  %s", value))
	}
	return lines
}

func applyCloseoutPlan(runtime *stackruntime.Runtime, state store.RepoState, plan closeoutPlan) ([]string, store.RepoState, error) {
	nextState := cloneRepoState(state)
	landing, ok := nextState.Landings[plan.LandingBranch]
	if !ok {
		return nil, state, fmt.Errorf("landing branch %q is no longer recorded in local metadata", plan.LandingBranch)
	}
	if !landing.CloseSupersededAfterMerge {
		return nil, state, fmt.Errorf("landing branch %q is not marked for post-merge superseded PR closure; rerun `stack supersede --landing %s --prs ... --close-after-merge` or close the PRs manually", plan.LandingBranch, plan.LandingBranch)
	}
	if plan.LandingPR == nil || plan.LandingPR.State != "MERGED" {
		return nil, state, fmt.Errorf("landing PR for %q must be merged before closeout can apply superseded PR closure", plan.LandingBranch)
	}

	closed := make([]string, 0, len(plan.SourceBranches))
	for _, branch := range plan.SourceBranches {
		if branch.PR == nil || branch.PR.Number == 0 || branch.PR.State != "OPEN" {
			continue
		}
		comment := fmt.Sprintf("Closing as superseded by merged landing PR #%d.", plan.LandingPR.Number)
		if err := runtime.GitHub.ClosePR(runtime.Context, branch.PR.Number, comment); err != nil {
			return nil, state, err
		}
		closed = append(closed, fmt.Sprintf("#%d %s", branch.PR.Number, branch.Name))
		for trackedBranch, record := range nextState.Branches {
			if record.PR.Number != branch.PR.Number {
				continue
			}
			record.PR.State = "CLOSED"
			nextState.Branches[trackedBranch] = record
		}
	}

	lines := []string{
		fmt.Sprintf("landing branch: %s", plan.LandingBranch),
		fmt.Sprintf("landing PR: #%d", plan.LandingPR.Number),
	}
	if len(closed) == 0 {
		lines = append(lines, "closed superseded PRs: none")
	} else {
		lines = append(lines, fmt.Sprintf("closed superseded PRs: %s", strings.Join(closed, ", ")))
	}
	if len(plan.TicketsSafeToClose) > 0 {
		lines = append(lines, fmt.Sprintf("tickets safe to close now: %s", strings.Join(plan.TicketsSafeToClose, ", ")))
	}
	return lines, nextState, nil
}

func isDeployVerification(checkType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(checkType))
	return normalized == "deploy" || normalized == "smoke"
}

var ticketRefPattern = regexp.MustCompile(`(?i)\b[a-z][a-z0-9]+-\d+\b`)
var fullTicketRefPattern = regexp.MustCompile(`(?i)^[a-z][a-z0-9]+-\d+$`)

func extractTicketRefs(value string) []string {
	matches := ticketRefPattern.FindAllString(value, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := map[string]bool{}
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		normalized := strings.ToUpper(strings.TrimSpace(match))
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result
}

func mapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func resolveLandingPRs(runtime *stackruntime.Runtime, landingBranch string, landing store.LandingRecord) ([]store.PullRequest, error) {
	var recorded *store.PullRequest
	if landing.LandingPRNumber > 0 {
		pr, err := runtime.GitHub.ViewPR(runtime.Context, landing.LandingPRNumber)
		if err != nil {
			return nil, err
		}
		recorded = &pr
		if pr.State == "OPEN" {
			return []store.PullRequest{pr}, nil
		}
	}
	prs, err := runtime.GitHub.ListPRsByHead(runtime.Context, landingBranch)
	if err != nil {
		return nil, err
	}

	open := make([]store.PullRequest, 0, len(prs))
	for _, pr := range prs {
		if pr.State == "OPEN" {
			open = append(open, pr)
		}
	}
	if len(open) == 1 {
		return open, nil
	}
	if len(open) > 1 {
		return open, nil
	}
	if recorded != nil {
		return []store.PullRequest{*recorded}, nil
	}

	return prs, nil
}

func parsePRNumbers(values []string) ([]int, error) {
	seen := map[int]bool{}
	numbers := make([]int, 0)
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			number, err := strconv.Atoi(trimmed)
			if err != nil || number <= 0 {
				return nil, fmt.Errorf("invalid PR number %q", trimmed)
			}
			if seen[number] {
				continue
			}
			seen[number] = true
			numbers = append(numbers, number)
		}
	}
	sort.Ints(numbers)
	return numbers, nil
}

func parseTicketRefs(values []string) ([]string, error) {
	seen := map[string]bool{}
	tickets := make([]string, 0)
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			if !fullTicketRefPattern.MatchString(trimmed) {
				return nil, fmt.Errorf("invalid ticket reference %q", trimmed)
			}
			normalized := strings.ToUpper(trimmed)
			if seen[normalized] {
				continue
			}
			seen[normalized] = true
			tickets = append(tickets, normalized)
		}
	}
	sort.Strings(tickets)
	return tickets, nil
}

func joinPRNumbers(numbers []int) string {
	parts := make([]string, 0, len(numbers))
	for _, number := range numbers {
		parts = append(parts, fmt.Sprintf("#%d", number))
	}
	return strings.Join(parts, ", ")
}

func buildQueuePlan(runtime *stackruntime.Runtime, state store.RepoState, branch string) (queuePlan, error) {
	if _, ok := state.Landings[branch]; ok {
		return buildLandingQueuePlan(runtime, state, branch)
	}
	return buildTrackedQueuePlan(runtime, state, branch)
}

func buildTrackedQueuePlan(runtime *stackruntime.Runtime, state store.RepoState, branch string) (queuePlan, error) {
	record, ok := state.Branches[branch]
	if !ok {
		if _, landing := state.Landings[branch]; landing {
			return queuePlan{}, fmt.Errorf("landing branch %q could not be resolved for queue handoff", branch)
		}
		return queuePlan{}, fmt.Errorf("branch %q is neither tracked nor recorded as a landing branch", branch)
	}
	if record.ParentBranch != state.Trunk {
		return queuePlan{}, fmt.Errorf("branch %q must target trunk before queue handoff", branch)
	}
	if record.PR.Number == 0 {
		return queuePlan{}, fmt.Errorf("branch %q has no tracked PR", branch)
	}

	if landingPlan, ok, err := buildSourceLandingQueuePlan(runtime, state, branch); err != nil {
		return queuePlan{}, err
	} else if ok {
		return landingPlan, nil
	}

	headOID, err := runtime.Git.ResolveRef(runtime.Context, branch)
	if err != nil {
		return queuePlan{}, err
	}
	remoteHeadOID, remoteExists, err := runtime.Git.RemoteBranchOID(runtime.Context, state.DefaultRemote, branch)
	if err != nil {
		return queuePlan{}, err
	}
	if !remoteExists {
		return queuePlan{}, fmt.Errorf("branch %q has not been pushed to %s", branch, state.DefaultRemote)
	}
	if remoteHeadOID != headOID {
		return queuePlan{}, fmt.Errorf("remote branch %q is stale; run `stack submit %s` before queue handoff", branch, branch)
	}

	record, err = refreshTrackedPR(runtime, state, branch, record)
	if err != nil {
		return queuePlan{}, err
	}
	if err := validateTrackedPR(branch, record); err != nil {
		return queuePlan{}, err
	}
	if record.PR.BaseRefName != "" && record.PR.BaseRefName != state.Trunk {
		return queuePlan{}, fmt.Errorf("PR for %q currently targets %q; run `stack submit %s` before queue handoff", branch, record.PR.BaseRefName, branch)
	}
	if record.PR.IsDraft {
		return queuePlan{}, fmt.Errorf("branch %q is still a draft PR", branch)
	}
	if record.PR.LastSeenHeadOID != "" && record.PR.LastSeenHeadOID != headOID {
		return queuePlan{}, fmt.Errorf("PR head for %q is stale; run `stack submit %s` before queue handoff", branch, branch)
	}

	verification, err := validateQueueVerification(branch, state.Verifications[branch], headOID)
	if err != nil {
		return queuePlan{}, err
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

	return queuePlan{
		Branch:       branch,
		PR:           record.PR,
		HeadOID:      headOID,
		Verification: verification,
		NextSteps:    nextSteps,
	}, nil
}

func buildLandingQueuePlan(runtime *stackruntime.Runtime, state store.RepoState, branch string) (queuePlan, error) {
	landing, ok := state.Landings[branch]
	if !ok {
		return queuePlan{}, fmt.Errorf("landing branch %q is not recorded in local compose metadata", branch)
	}
	if landing.BaseBranch != "" && landing.BaseBranch != state.Trunk {
		return queuePlan{}, fmt.Errorf("landing branch %q targets %q; only trunk-bound landing branches can enter queue", branch, landing.BaseBranch)
	}
	if !runtime.Git.BranchExists(runtime.Context, branch) {
		return queuePlan{}, fmt.Errorf("landing branch %q does not exist locally", branch)
	}

	headOID, err := runtime.Git.ResolveRef(runtime.Context, branch)
	if err != nil {
		return queuePlan{}, err
	}
	remoteHeadOID, remoteExists, err := runtime.Git.RemoteBranchOID(runtime.Context, state.DefaultRemote, branch)
	if err != nil {
		return queuePlan{}, err
	}
	if !remoteExists {
		return queuePlan{}, fmt.Errorf("landing branch %q has not been pushed to %s", branch, state.DefaultRemote)
	}
	if remoteHeadOID != headOID {
		return queuePlan{}, fmt.Errorf("remote landing branch %q is stale; push or refresh it before queue handoff", branch)
	}

	prs, err := resolveLandingPRs(runtime, branch, landing)
	if err != nil {
		return queuePlan{}, err
	}
	if len(prs) == 0 {
		return queuePlan{}, fmt.Errorf("landing branch %q has no pull request yet; open the landing PR before queue handoff", branch)
	}
	if len(prs) > 1 {
		numbers := describePRs(prs)
		return queuePlan{}, fmt.Errorf("landing branch %q matches multiple PRs: %s; resolve the ambiguity before queue handoff", branch, strings.Join(numbers, ", "))
	}

	pr := prs[0]
	if pr.State == "CLOSED" {
		return queuePlan{}, fmt.Errorf("landing PR for %q is closed; repair or reopen it before queue handoff", branch)
	}
	if pr.State == "MERGED" {
		return queuePlan{}, fmt.Errorf("landing PR for %q is already merged; run `stack closeout %s` instead", branch, branch)
	}
	if pr.IsDraft {
		return queuePlan{}, fmt.Errorf("landing branch %q is still a draft PR", branch)
	}
	if pr.BaseRefName != "" && pr.BaseRefName != state.Trunk {
		return queuePlan{}, fmt.Errorf("landing PR for %q currently targets %q; retarget it to %s before queue handoff", branch, pr.BaseRefName, state.Trunk)
	}
	if pr.LastSeenHeadOID != "" && pr.LastSeenHeadOID != headOID {
		return queuePlan{}, fmt.Errorf("landing PR head for %q is stale; push the current landing head before queue handoff", branch)
	}

	verification, err := validateQueueVerification(branch, state.Verifications[branch], headOID)
	if err != nil {
		return queuePlan{}, err
	}

	return queuePlan{
		Branch:       branch,
		PR:           pr,
		HeadOID:      headOID,
		IsLanding:    true,
		Verification: verification,
		ExcludedPRs:  landingExcludedPRs(state, landing),
		NextSteps: []string{
			fmt.Sprintf("wait for GitHub to merge PR #%d", pr.Number),
			fmt.Sprintf("then run: stack closeout %s", branch),
		},
	}, nil
}

func buildSourceLandingQueuePlan(runtime *stackruntime.Runtime, state store.RepoState, branch string) (queuePlan, bool, error) {
	landingBranches := landingBranchesForSourceBranch(state, branch)
	if len(landingBranches) == 0 {
		return queuePlan{}, false, nil
	}
	if len(landingBranches) > 1 {
		return queuePlan{}, false, fmt.Errorf("branch %q appears in multiple landing batches (%s); repair the landing metadata before queue handoff", branch, strings.Join(landingBranches, ", "))
	}

	landingBranch := landingBranches[0]
	prs, err := resolveLandingPRs(runtime, landingBranch, state.Landings[landingBranch])
	if err != nil {
		return queuePlan{}, false, err
	}
	if len(prs) == 0 {
		return queuePlan{}, false, fmt.Errorf("branch %q is part of landing batch %q; open or relink the landing PR before queue handoff, and keep source PRs out of the merge queue", branch, landingBranch)
	}
	if len(prs) > 1 {
		numbers := describePRs(prs)
		return queuePlan{}, false, fmt.Errorf("branch %q is part of landing batch %q; resolve ambiguous landing PR ownership (%s) before queue handoff, and keep source PRs out of the merge queue", branch, landingBranch, strings.Join(numbers, ", "))
	}

	pr := prs[0]
	return queuePlan{}, false, fmt.Errorf("branch %q is part of landing batch %q; queue landing PR #%d instead and keep source PRs out of the merge queue", branch, landingBranch, pr.Number)
}

func validateQueueVerification(branch string, records []store.VerificationRecord, currentHeadOID string) (*queueVerification, error) {
	if len(records) == 0 {
		return nil, nil
	}

	latest := records[len(records)-1]
	verification := &queueVerification{
		Latest:             latest,
		HeadMatchesCurrent: latest.HeadOID != "" && currentHeadOID != "" && latest.HeadOID == currentHeadOID,
	}

	if !latest.Passed {
		return nil, fmt.Errorf("latest %s verification for %q failed; inspect `stack verify list %s` before queue handoff", latest.CheckType, branch, branch)
	}
	if !verification.HeadMatchesCurrent {
		return nil, fmt.Errorf("branch %q head moved since the latest recorded verification; rerun or record fresh verification before queue handoff", branch)
	}
	return verification, nil
}

func renderQueueVerification(verification queueVerification) string {
	parts := []string{
		strings.TrimSpace(verification.Latest.CheckType),
		verificationResult(verification.Latest),
	}
	if verification.Latest.Identifier != "" {
		parts = append(parts, verification.Latest.Identifier)
	}
	return strings.Join(parts, " ")
}

func landingBranchesForSourceBranch(state store.RepoState, branch string) []string {
	names := make([]string, 0)
	for landingBranch, landing := range state.Landings {
		for _, sourceBranch := range landing.SourceBranches {
			if sourceBranch == branch {
				names = append(names, landingBranch)
				break
			}
		}
	}
	sort.Strings(names)
	return names
}

func landingExcludedPRs(state store.RepoState, landing store.LandingRecord) []int {
	if len(landing.SupersededPRs) > 0 {
		return append([]int(nil), landing.SupersededPRs...)
	}

	seen := map[int]bool{}
	numbers := make([]int, 0, len(landing.SourceBranches))
	for _, branch := range landing.SourceBranches {
		record, ok := state.Branches[branch]
		if !ok || record.PR.Number == 0 || seen[record.PR.Number] {
			continue
		}
		seen[record.PR.Number] = true
		numbers = append(numbers, record.PR.Number)
	}
	sort.Ints(numbers)
	return numbers
}

func describePRs(prs []store.PullRequest) []string {
	values := make([]string, 0, len(prs))
	for _, pr := range prs {
		values = append(values, fmt.Sprintf("#%d %s", pr.Number, strings.ToLower(pr.State)))
	}
	sort.Strings(values)
	return values
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
	cloned.Landings = make(map[string]store.LandingRecord, len(state.Landings))
	for branch, record := range state.Landings {
		cloned.Landings[branch] = store.LandingRecord{
			BaseBranch:                record.BaseBranch,
			SourceBranches:            append([]string(nil), record.SourceBranches...),
			Tickets:                   append([]string(nil), record.Tickets...),
			LandingPRNumber:           record.LandingPRNumber,
			SupersededPRs:             append([]int(nil), record.SupersededPRs...),
			CloseSupersededAfterMerge: record.CloseSupersededAfterMerge,
			SupersededAt:              record.SupersededAt,
			CreatedAt:                 record.CreatedAt,
		}
	}
	cloned.Verifications = make(map[string][]store.VerificationRecord, len(state.Verifications))
	for branch, records := range state.Verifications {
		cloned.Verifications[branch] = append([]store.VerificationRecord(nil), records...)
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
		return fmt.Errorf("tracked PR for %q is closed; run `stack status` and repair or relink local metadata before submitting again", branch)
	}
	if record.PR.State == "MERGED" {
		return fmt.Errorf("tracked PR for %q is already merged; run `stack sync` before submitting again", branch)
	}
	if record.PR.HeadRefName != "" && record.PR.HeadRefName != branch {
		return fmt.Errorf("tracked PR for %q points at head %q; inspect with `stack status` and relink the correct PR before submitting again", branch, record.PR.HeadRefName)
	}
	return nil
}

func ensureCreateParentAllowed(state store.RepoState, parent string) error {
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return fmt.Errorf("cannot create a tracked branch from detached HEAD; switch to %s or another tracked branch first", state.Trunk)
	}
	if parent == state.Trunk {
		return nil
	}
	if _, ok := state.Branches[parent]; ok {
		return nil
	}
	return fmt.Errorf("current branch %q is not tracked in local metadata; track it first or switch to %s", parent, state.Trunk)
}

func ensureTrackedParentAllowed(state store.RepoState, parent string) error {
	parent = strings.TrimSpace(parent)
	if parent == "" || parent == state.Trunk {
		return nil
	}
	if _, ok := state.Branches[parent]; ok {
		return nil
	}
	return fmt.Errorf("parent branch %q is not tracked in local metadata; track it first or move under %s", parent, state.Trunk)
}

func trackRestackAnchor(runtime *stackruntime.Runtime, branch string, parent string) (string, error) {
	anchor, _, err := trackRestackAnchorDetail(runtime, branch, parent)
	return anchor, err
}

func trackRestackAnchorDetail(runtime *stackruntime.Runtime, branch string, parent string) (string, bool, error) {
	parentOID, err := runtime.Git.ResolveRef(runtime.Context, parent)
	if err != nil {
		return "", false, err
	}

	validAnchor, err := runtime.Git.IsAncestor(runtime.Context, parentOID, branch)
	if err != nil {
		return "", false, err
	}
	if validAnchor {
		return parentOID, false, nil
	}

	mergeBase, ok, err := runtime.Git.MergeBase(runtime.Context, parent, branch)
	if err != nil {
		return "", false, err
	}
	if !ok || mergeBase == "" {
		return "", false, fmt.Errorf("branch %q does not share a repairable merge base with parent %q; rebase it first or choose a different parent", branch, parent)
	}

	return mergeBase, true, nil
}

func resolveOID(runtime *stackruntime.Runtime, ref string) string {
	oid, err := runtime.Git.ResolveRef(runtime.Context, ref)
	if err != nil {
		return ""
	}
	return oid
}

func shortOID(oid string) string {
	if len(oid) <= 12 {
		return oid
	}
	return oid[:12]
}

func verificationResult(record store.VerificationRecord) string {
	if record.Passed {
		return "passed"
	}
	return "failed"
}

func init() {
	pflag.CommandLine.SortFlags = false
}
