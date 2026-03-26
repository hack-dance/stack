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
stack compose discovery-core --branches feature/a --branches feature/b --yes
```

### Options

```
      --branches stringArray   Tracked branches to include in explicit order
      --from string            Bottom branch of a contiguous tracked path
  -h, --help                   help for compose
      --to string              Top branch of a contiguous tracked path
      --yes                    Skip confirmation
```

### SEE ALSO

* [stack](stack.md)	 - Manage explicit stacked PR workflows with Git and GitHub

