# stack_compose

Generated from the current Cobra command tree.

## stack compose

Compose a strict landing branch from selected tracked branches

### Synopsis

Create one ordinary local landing branch from an explicit linear branch selection and replay only the selected commits in order.

```
stack compose <name> [flags]
```

### Examples

```
stack compose discovery-core --from feature/a --to feature/c
stack compose discovery-core --from feature/a --to feature/c --ticket LNHACK-66 --ticket LNHACK-67 --open-pr
stack compose discovery-core --branches feature/a --branches feature/b --yes
```

### Options

```
      --body string            Explicit landing PR body when used with --open-pr
      --branches stringArray   Tracked branches to include in explicit order
      --draft                  Create the landing PR as a draft when used with --open-pr
      --from string            Bottom branch of a contiguous tracked path
  -h, --help                   help for compose
      --open-pr                Push the landing branch and create or refresh its GitHub pull request
      --ticket stringArray     Ticket references to attach to the landing branch, comma-separated or repeated
      --title string           Explicit landing PR title when used with --open-pr
      --to string              Top branch of a contiguous tracked path
      --yes                    Skip confirmation
```

### SEE ALSO

* [stack](stack.md)	 - Manage explicit stacked PR workflows with Git and GitHub

