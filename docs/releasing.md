# Releasing

This repo uses conventional commits plus `release-please` and `goreleaser`.

## Release model

- Pull request titles should follow Conventional Commits.
- Release-bearing changes should land on `main` via squash or rebase merges so the final commit subject stays conventional.
- `release-please` watches `main`, opens a release PR, updates `CHANGELOG.md`, and bumps the version manifest.
- Merging the release PR creates the Git tag.
- Tag pushes trigger `goreleaser`, which builds artifacts, creates the GitHub Release, and updates `hack-dance/homebrew-tap`.

## Required secrets

- `RELEASE_PLEASE_TOKEN`
  Use this if you want CI to run on release PRs opened by `release-please`. If omitted, the workflow falls back to the default GitHub token.
- `HOMEBREW_TAP_GITHUB_TOKEN`
  Token with write access to `hack-dance/homebrew-tap`.

## Maintainer flow

1. Merge conventional-commit changes into `main`.
2. Wait for the `release-please` workflow to open or update the release PR.
3. Review the generated version and changelog.
4. Merge the release PR.
5. Verify the `release` workflow published artifacts and updated the Homebrew tap.

## Local dry run

```bash
mise install
mise exec -- go test ./...
mise exec -- go build ./...
goreleaser release --snapshot --clean
```

Use snapshot mode locally so you can verify archives and formulas without creating a tag.
