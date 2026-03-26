# stack_closeout

Generated from the current Cobra command tree.

## stack closeout

Plan read-only post-merge closeout for a landing branch

### Synopsis

Use recorded landing composition, pull request state, and verification records to show which original PRs and inferred tickets are safe to close now versus still blocked on deploy checks.

```
stack closeout <landing-branch> [flags]
```

### Examples

```
stack closeout stack/discovery-core
```

### Options

```
  -h, --help   help for closeout
```

### SEE ALSO

* [stack](stack.md)	 - Manage explicit stacked PR workflows with Git and GitHub

