# stack_move

Generated from the current Cobra command tree.

## stack move

Change a branch parent and restack the affected subtree

### Synopsis

Change one tracked branch to a new parent, preview the rewrite, update metadata, and restack from the recorded anchor.

```
stack move <branch> [flags]
```

### Examples

```
stack move feature/b --parent feature/a
stack move feature/b --parent main --yes
```

### Options

```
  -h, --help            help for move
      --parent string   New parent branch
      --yes             Skip confirmation
```

### SEE ALSO

* [stack](stack.md)	 - Manage explicit stacked PR workflows with Git and GitHub

