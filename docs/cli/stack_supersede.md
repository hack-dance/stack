# stack_supersede

Generated from the current Cobra command tree.

## stack supersede

Mark original PRs as superseded by a landing PR

### Synopsis

Record explicit superseded PR linkage in local landing metadata and optionally comment on the original PRs with the landing PR reference.

```
stack supersede [flags]
```

### Examples

```
stack supersede --landing stack/discovery-core --prs 353,354,363,364
stack supersede --landing stack/discovery-core --prs 353 --prs 354 --no-comment --yes
```

### Options

```
  -h, --help              help for supersede
      --landing string    Landing branch to use as the superseding target
      --no-comment        Record superseded linkage locally without posting GitHub comments
      --prs stringArray   Original PR numbers, comma-separated or repeated
      --yes               Skip confirmation
```

### SEE ALSO

* [stack](stack.md)	 - Manage explicit stacked PR workflows with Git and GitHub

