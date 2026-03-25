# stack_restack

Generated from the current Cobra command tree.

## stack restack

Rebase a branch or subtree onto its configured parent

### Synopsis

Preview and restack one tracked branch or subtree. The command refuses invalid anchors and stops instead of guessing.

```
stack restack [branch] [flags]
```

### Examples

```
stack restack
stack restack feature/a
stack restack --all --yes
```

### Options

```
      --all    Restack all tracked branches
  -h, --help   help for restack
      --yes    Skip confirmation
```

### SEE ALSO

* [stack](stack.md)	 - Manage explicit stacked PR workflows with Git and GitHub

