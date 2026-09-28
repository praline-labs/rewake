# Installing and releasing

Locally, without publishing, there are two ways, and they answer different questions.

**To use it.** Build beside the installed command, then replace it atomically:

```bash
go build -ldflags "-X github.com/praline-labs/rewake/internal/cli.built=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
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
`npm uninstall -g @praline-labs/rewake @praline-labs/rewake-linux-x64`.

The entry package's `bin` is a POSIX script rather than a Node shim: it execs the
binary and disappears, so the terminal, the signals and the exit code belong to
rewake rather than to a process in between. It follows the symlink npm installs
first — resolving the link is the difference between finding the binary and
reporting it missing.

## Releasing to npm

Owner decision, September 28, 2026: rewake is released on the public npm registry as
`@praline-labs/rewake`, with the platform packages `@praline-labs/rewake-linux-x64` and
`@praline-labs/rewake-linux-arm64`, under the MIT license; the unscoped name `rewake`
belongs to somebody else. Nothing is published without the owner's explicit word, so
the tooling publishes nothing unless told to.

Every release goes through the gate, `tools/release`:

1. Set the version in the release commit and push it. `internal/cli/registry.go`
   holds the version a build from source reports, `var Version = "1.0.0"`; the
   packages get theirs from `scripts/pack.sh`, and the gate refuses before the build
   when the line at HEAD names another version than the one released, so the tagged
   source and the npm binary cannot disagree (owner decision, September 28, 2026). It
   also wants a clean tree whose HEAD `origin/main` contains, as `git push` left the
   remote-tracking ref.
2. The dry run, which publishes nothing:

   ```bash
   go run ./tools/release 1.0.0
   ```

   It checks that none of the three packages has the version in the registry, builds
   them with `scripts/pack.sh`, checks each one's `package.json` and what
   `npm pack --dry-run --json` would upload against an allowlist in both directions,
   reads each platform binary's architecture, runs this machine's binary for
   `--version` — the version, built from HEAD with no local changes — and installs the
   entry with its shim into a scratch `node_modules`, with this machine's platform
   package and without it. It runs the shim's own tests, `go test ./scripts/`, too. A
   dry run goes on past a failed check, so one run shows every problem, and exits 1
   when any failed. `go run ./tools/release --help` lists the checks and the codes.
3. With the owner's word, the same with `--publish`, and `--otp <code>` when the
   account asks for one:

   ```bash
   go run ./tools/release 1.0.0 --publish --otp 123456
   ```

   A failed check before the build stops it there, and nothing is published unless
   every check passed. The platform packages go up first and the entry last, so an
   install between two uploads never finds an entry whose binary is missing. Each goes
   with `--access public` to `https://registry.npmjs.org/`, named on the command line
   together with the scope's registry, since a scope setting on the machine would
   outrank `--registry` alone.
4. Tag the release with the command the gate prints; the gate creates no tag.

The version check frees a version only on npm's own `E404`. `npm view` fails the same
way for a version that is not there and for a registry it could not reach, and a gate
that took every failure for the first would publish over a network error; a 403, a
timeout or an answer that does not parse stops the release as unsafe to decide.

The version is semver without build metadata: npm strips a `+build` part when it
publishes, so `1.0.0+build.1` would be checked and tagged as one number and published
as another. `npm publish` gets `--dry-run=false`, which outranks a dry-run setting
inherited from the environment or an npmrc, under which npm skips the upload and
still exits 0.

The first failed upload stops the rest, and the gate prints what was published, what
was not and what is unknown. A refusal from the registry — a 4xx code or `EOTP` — means
the package was not published. Any other failure, a lost connection or a timeout, may
have come after the registry stored it, so the gate asks the registry once: a package
that is there counts as published and the next upload follows; one that is not, or
cannot be asked about, is unknown, and its version is not reused until `npm view`
still answers `E404` a while later. A version any package already has cannot be reused — the registry
refuses a version once taken, even after an unpublish, and the gate refuses it too —
so a release that stopped halfway continues as the next version. After the uploads
the gate asks the registry for each package and for the entry's `latest` tag, and
reports without retrying. A package not visible right after its upload is usually
the registry's delay, but only a later look tells it from an upload npm skipped, so
the run then ends with 1, names the `npm view` to repeat, and prints no tag.

`go run` reports every non-zero exit as 1; build the gate first when the codes matter:
0 every check passed (and with `--publish`, every package went up and is visible), 1 a
check or an upload failed or a package is not visible yet, 2 the call was wrong.
