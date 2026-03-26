# stack_queue

Generated from the current Cobra command tree.

## stack queue

Hand one verified trunk-bound PR or landing PR to GitHub auto-merge or merge queue

### Synopsis

Validate that one tracked trunk branch or recorded landing branch is ready for handoff, then ask GitHub to auto-merge or enqueue the PR using the current head commit.

```
stack queue <branch> [flags]
```

### Examples

```
stack queue feature/a
stack queue stack/discovery-core
stack queue feature/a --strategy squash
stack queue feature/a --yes
```

### Options

```
  -h, --help              help for queue
      --strategy string   Merge strategy: merge, squash, or rebase (default "merge")
      --yes               Skip confirmation
```

### SEE ALSO

* [stack](stack.md)	 - Manage explicit stacked PR workflows with Git and GitHub

