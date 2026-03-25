# Demo assets

The README GIFs are rendered from the tape files in this directory.

## Render the demos

```bash
scripts/demo/render.sh
```

The render script prepares the sandbox scenarios on your machine, then runs VHS
locally so the output inherits your host fonts and terminal rendering.

If `vhs`, `ffmpeg`, and `ttyd` are not already installed, the script falls back
to `nix-shell -p vhs ffmpeg ttyd`.

## Files

- `status.tape` records a stack inspection flow in a real sandbox clone
- `queue.tape` records a real submit and queue handoff flow against the GitHub sandbox
- `status.gif` and `queue.gif` are the generated README assets
