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
- `git` and `gh` CLI only
- no GraphQL, no backend, no synthetic refs

## First Commands

- `stack init`
- `stack create <branch>`
- `stack track <branch> --parent <parent>`
- `stack status`
- `stack tui`

Later phases add restack, submit, sync, move, queue, and recovery commands on
top of the same state model.

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
mise exec -- go run ./cmd/stack status
```

## Design

The reduced V1 design lives at [docs/v1.md](docs/v1.md).

