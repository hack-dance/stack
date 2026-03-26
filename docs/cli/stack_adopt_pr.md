# stack_adopt_pr

Generated from the current Cobra command tree.

## stack adopt pr

Adopt one open pull request into the stack graph

### Synopsis

Look up one open pull request, optionally fetch its head branch locally, then track it under an explicit parent branch.

```
stack adopt pr <number> [flags]
```

### Examples

```
stack adopt pr 353 --parent main
stack adopt pr 354 --parent pr/353 --yes
```

### Options

```
  -h, --help            help for pr
      --parent string   Parent branch or trunk
      --yes             Skip confirmation
```

### SEE ALSO

* [stack adopt](stack_adopt.md)	 - Adopt existing pull requests into explicit stack metadata

