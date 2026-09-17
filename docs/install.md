# Local installation without publishing

Two ways, and they answer different questions.

**To use it.** Build beside the installed command, then replace it atomically:

```bash
go build -o ~/.local/bin/rewake.new ./cmd/rewake
mv -f ~/.local/bin/rewake.new ~/.local/bin/rewake
```

Building directly over a running executable can fail with `text file busy`.
Renaming a neighboring file avoids that: existing wrappers keep their old
executable, while new launches use the replacement. Restart existing sessions
separately when their ongoing wrappers need the new behavior.

**To test how it will be installed.** `scripts/pack.sh [version]` builds the npm
packages into `dist/npm` — one per platform plus the entry package — and
publishes nothing. Then:

```bash
cd dist/npm/rewake-linux-x64 && npm pack && npm install -g ./*.tgz
cd ../rewake && npm pack && npm install -g --omit=optional ./*.tgz
rewake --version
```

That exercises everything a registry release would except the registry itself:
the package contents, the platform split, the shim resolving its binary, and the
command landing in PATH. Uninstall with
`npm uninstall -g @iiiokojiadbi/rewake @iiiokojiadbi/rewake-linux-x64`.

The entry package's `bin` is a POSIX script rather than a Node shim: it execs the
binary and disappears, so the terminal, the signals and the exit code belong to
rewake rather than to a process in between. It follows the symlink npm installs
first — resolving the link is the difference between finding the binary and
reporting it missing.
