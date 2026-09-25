# Local installation without publishing

Two ways, and they answer different questions.

**To use it.** Build beside the installed command, then replace it atomically:

```bash
go build -ldflags "-X github.com/iiiokojiadbi/rewake/internal/cli.built=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o ~/.local/bin/rewake.new ./cmd/rewake
mv -f ~/.local/bin/rewake.new ~/.local/bin/rewake
```

The `-ldflags` passes in the build time, which `rewake --version` and the guide show:
the binary cannot know it otherwise. Built without it, they show the revision's commit
time instead, labeled `committed`, and two builds of one revision with a modified tree
look alike.

Building directly over a running executable can fail with `text file busy`.
Renaming a neighboring file avoids that: existing wrappers keep their old
executable, while new launches use the replacement. Restart existing sessions
separately when their ongoing wrappers need the new behavior.

For the accepted native mailbox path, use the [owner check recipe](native-mailbox-check.md)
with an explicit permission selection for a fresh disposable workspace. The
[September 20 acceptance](native-mailbox-acceptance.md) includes installation and
restart, but does not establish inbox support under deliberate read-only permissions.

**To test how it will be installed.** `scripts/pack.sh [version]` builds the npm
packages into `dist/npm` — one per platform plus the entry package — and
publishes nothing. Then:

```bash
cd dist/npm/rewake-linux-x64 && npm pack && npm install -g ./*.tgz
cd ../rewake && npm pack && npm install -g --omit=optional ./*.tgz
rewake --version
```

`rewake --version` names the version, the revision the binary was built from
(`build unknown` for one built outside a git checkout) and the build time
`scripts/pack.sh` passes in.

That exercises everything a registry release would except the registry itself:
the package contents, the platform split, the shim resolving its binary, and the
command landing in PATH. Uninstall with
`npm uninstall -g @iiiokojiadbi/rewake @iiiokojiadbi/rewake-linux-x64`.

The entry package's `bin` is a POSIX script rather than a Node shim: it execs the
binary and disappears, so the terminal, the signals and the exit code belong to
rewake rather than to a process in between. It follows the symlink npm installs
first — resolving the link is the difference between finding the binary and
reporting it missing.
