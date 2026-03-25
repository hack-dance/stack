# stack

`stack` is a GitHub stacked PR CLI built with Go and Charm.

It manages explicit local branch stacks using normal Git branches and normal
GitHub pull requests. The tool keeps stack intent in local metadata, surfaces
drift clearly, and favors repairable workflows over hidden automation.

## Current Scope

- explicit local stack metadata stored under shared Git state
- per-worktree operation journals for interrupted rebases
- command-first UX with polished terminal output
- read-only Bubble Tea dashboard for stack inspection
- deterministic restack and recovery scaffolding
- explicit submit, sync, move, and queue command flows
- `git` and `gh` CLI only
- no GraphQL, no backend, no synthetic refs

## Implemented Commands

- `stack init`
- `stack create <branch>`
- `stack track <branch> --parent <parent>`
- `stack status`
- `stack tui`
- `stack restack`
- `stack continue`
- `stack abort`
- `stack submit`
- `stack sync`
- `stack move`
- `stack queue`

## Current Caveats

- restack and sync safety are implemented, but still need deeper fixture coverage
- GitHub sandbox coverage for submit, sync, and queue still needs expansion
- the TUI is intentionally read-only in the current alpha
- ambiguous merged-parent repair cases stop for review instead of guessing

## Tooling

- Go `1.25.x` via `mise`
- `cobra`
- `bubbletea`, `bubbles`, `lipgloss`
- `huh`
- `glamour`
- `charmbracelet/log`
- `vhs` for demos and regression recordings

## Development

```bash
mise install
mise exec -- go test ./...
mise exec -- go build ./...
mise exec -- go run ./cmd/stack status
```

## Design

The reduced V1 design lives at [docs/v1.md](docs/v1.md).
