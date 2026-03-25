# Repository Agent Instructions

This file adds repo-specific guidance for work in this repository. Follow it in
addition to the global baseline instructions.

## Toolchain

- Use `mise` for Go commands in this repo.
- Prefer `mise exec -- gofmt -w <files>` for formatting Go files.
- Prefer `mise exec -- go test ./...` for the main test suite.
- Prefer `mise exec -- go build ./...` for a repo-wide build check.
- Do not assume `go` or `gofmt` are on `PATH`; use `mise exec -- ...`.

## Verification

- Treat `mise exec -- go test ./...` and `mise exec -- go build ./...` as the
  default verification loop for code changes.
- If you change queue, restack, submit, sync, or recovery behavior, read
  [docs/testing.md](docs/testing.md) and use the strongest relevant loop from
  that doc instead of stopping at unit tests.
- State clearly what was verified and what remains unverified if a stronger loop
  is unavailable.

## Commits And Releases

- Use Conventional Commits for commit subjects.
- Keep pull request titles conventional as well; the release flow depends on
  it.
- Before touching release behavior or release automation, read
  [docs/releasing.md](docs/releasing.md).

## Working Tree Hygiene

- Do not commit local build outputs such as the root `stack` binary.
- If a generated or built artifact appears repeatedly during normal work, add it
  to `.gitignore` in the same change that introduces the behavior.

