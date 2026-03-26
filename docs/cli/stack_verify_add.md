# stack_verify_add

Generated from the current Cobra command tree.

## stack verify add

Record one verification result for a branch

### Synopsis

Record local verification evidence against the current head of a tracked branch or composed landing branch.

```
stack verify add <branch> [flags]
```

### Examples

```
stack verify add stack/discovery-core --type sim --run-id abc123 --passed --score 100
stack verify add feature/a --type manual --identifier smoke-check-42 --failed --note "deploy blocked"
```

### Options

```
      --failed              Mark the verification as failed
  -h, --help                help for add
      --identifier string   Verification identifier such as a run id, URL, or external check reference
      --note string         Optional operator note
      --passed              Mark the verification as passed
      --run-id string       Convenience alias for --identifier when recording a run id
      --score int           Optional numeric score
      --type string         Verification type such as sim, unit, integration, manual, deploy, or smoke
```

### SEE ALSO

* [stack verify](stack_verify.md)	 - Attach and inspect lightweight verification records

