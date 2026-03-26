# stack_closeout

Generated from the current Cobra command tree.

## stack closeout

Plan read-only post-merge closeout for a landing branch

### Synopsis

Use recorded landing composition, pull request state, and verification records to show which original PRs and explicit tickets are safe to close now versus still blocked on deploy checks.

```
stack closeout <landing-branch> [flags]
```

### Examples

```
stack closeout stack/discovery-core
stack closeout stack/discovery-core --apply --yes
```

### Options

```
      --apply   Close superseded PRs that are explicitly marked safe to close after landing merge
  -h, --help    help for closeout
      --yes     Skip confirmation when used with --apply
```

### SEE ALSO

* [stack](stack.md)	 - Manage explicit stacked PR workflows with Git and GitHub

