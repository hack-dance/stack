# Install

`stack` is a normal single-binary Go CLI.

## Homebrew

After the first tagged release is published, install from the hack-dance tap:

```bash
brew tap hack-dance/homebrew-tap
brew install hack-dance/tap/stack
```

Upgrade with:

```bash
brew update
brew upgrade hack-dance/tap/stack
```

## Download a release artifact

Tagged releases publish tarballs and checksums to GitHub Releases.

```bash
curl -L https://github.com/hack-dance/stack/releases/latest/download/stack_<version>_darwin_arm64.tar.gz | tar -xz
mv stack /usr/local/bin/stack
```

Replace the archive name with the OS and architecture you need.

## Build from source

```bash
mise install
mise exec -- go build -o ./bin/stack ./cmd/stack
./bin/stack version
```

## Requirements

- `git`
- `gh`
- a GitHub auth session with repo access for PR operations
