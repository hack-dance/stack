# stack_submit

Generated from the current Cobra command tree.

## stack submit

Push tracked branches and create or update one normal PR per branch

### Synopsis

Fetch, optionally restack, preview the push plan, then push tracked branches and create or refresh one normal GitHub PR per branch.

```
stack submit [branch] [flags]
```

### Examples

```
stack submit
stack submit feature/a --draft
stack submit --all --yes
```

### Options

```
      --all          Submit all tracked branches
      --draft        Create PRs as drafts when needed
  -h, --help         help for submit
      --no-restack   Skip restack before submit
      --yes          Skip confirmation
```

### SEE ALSO

* [stack](stack.md)	 - Manage explicit stacked PR workflows with Git and GitHub

