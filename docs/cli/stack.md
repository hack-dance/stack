# stack

Generated from the current Cobra command tree.

## stack

Manage explicit stacked PR workflows with Git and GitHub

### Synopsis

Use normal Git branches, normal GitHub pull requests, and explicit local stack
metadata. The CLI favors visible state, repairable workflows, and safe handoff
to GitHub merge queue via the gh CLI.

```
stack [flags]
```

### Options

```
  -h, --help   help for stack
```

### SEE ALSO

* [stack abort](stack_abort.md)	 - Abort an interrupted restack and clear the operation journal
* [stack adopt](stack_adopt.md)	 - Adopt existing pull requests into explicit stack metadata
* [stack closeout](stack_closeout.md)	 - Plan read-only post-merge closeout for a landing branch
* [stack compose](stack_compose.md)	 - Compose a strict landing branch from selected tracked branches
* [stack continue](stack_continue.md)	 - Continue an interrupted restack after conflicts are resolved
* [stack create](stack_create.md)	 - Create a tracked branch on top of the current branch
* [stack init](stack_init.md)	 - Initialize stack metadata for this repository
* [stack move](stack_move.md)	 - Change a branch parent and restack the affected subtree
* [stack queue](stack_queue.md)	 - Hand one verified trunk-bound PR or landing PR to GitHub auto-merge or merge queue
* [stack restack](stack_restack.md)	 - Rebase a branch or subtree onto its configured parent
* [stack status](stack_status.md)	 - Show stack health, hierarchy, and cached PR state
* [stack submit](stack_submit.md)	 - Push tracked branches and create or update one normal PR per branch
* [stack supersede](stack_supersede.md)	 - Mark original PRs as superseded by a landing PR
* [stack sync](stack_sync.md)	 - Refresh cached PR metadata and inspect or apply safe repairs
* [stack track](stack_track.md)	 - Adopt an existing branch into the explicit stack graph
* [stack tui](stack_tui.md)	 - Open the read-only stack dashboard
* [stack verify](stack_verify.md)	 - Attach and inspect lightweight verification records
* [stack version](stack_version.md)	 - Print version, commit, and build date

