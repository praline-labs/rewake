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
uploads into `dist/npm` — a build per platform plus the entry, all versions of the one
package `@praline-labs/rewake` — and publishes nothing. The entry reaches its builds
through `npm:` aliases, which npm resolves only against a registry: an install from the
tarballs on disk would look for the builds on the public registry. So the install is
tested by the release gate's dry run, which serves the release from a registry of its
own on the loopback interface and has npm install it there:

```bash
go run ./tools/release 1.0.2
```

It installs the entry globally into a scratch prefix and runs the command, installs each
other build as npm would choose it for that cpu, and installs the entry with
`--omit=optional` to see the command fail naming the reinstall. That last one goes into
a project rather than globally: npm 11.17 installs a global package's optional
dependencies even under `--omit=optional`, seen on September 28, 2026. On a tree with
uncommitted changes or an unreleased version the checks before the build fail, and the
build and the install checks still run and report.

`rewake --version` names the version, the revision the binary was built from
(`build unknown` for one built outside a git checkout) and the build time
`scripts/pack.sh` passes in.

That exercises everything a registry release would except the public registry itself:
the package contents, npm resolving the aliases and choosing a build by `os` and `cpu`,
the shim resolving its binary, and the command landing in PATH. An install from the
registry is removed with `npm uninstall -g @praline-labs/rewake`; the build goes with
the entry.

The entry package's `bin` is a POSIX script rather than a Node shim: it execs the
binary and disappears, so the terminal, the signals and the exit code belong to
rewake rather than to a process in between. It follows the symlink npm installs
first — resolving the link is the difference between finding the binary and
reporting it missing.

## Releasing to npm

Owner decision, September 28, 2026: rewake is released on the public npm registry as
`@praline-labs/rewake`, under the MIT license; the unscoped name `rewake` belongs to
somebody else. Nothing is published without the owner's explicit word, so the tooling
publishes nothing unless told to.

Owner decision, September 28, 2026, from 1.0.2 on: a release is one package name. The
entry is `<version>` under `latest`; each platform build is the version
`<version>-linux-x64` or `<version>-linux-arm64` of the same package, under the dist-tag
`linux-x64` or `linux-arm64`, and the entry names them in `optionalDependencies` as
`npm:@praline-labs/rewake@<version>-linux-x64` behind the aliases
`@praline-labs/rewake-linux-x64` and `-linux-arm64` (the reasons in
[design.md](design.md#distribution)). 1.0.0 and 1.0.1 used separate platform packages of
those names; the owner removed them from the registry on September 28, 2026, so those
two versions now install without a binary, and they are deprecated once 1.0.2 is out.

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

   It asks the registry for each of the three versions exactly — `1.0.2`,
   `1.0.2-linux-x64`, `1.0.2-linux-arm64` — and wants none of them there, builds them
   with `scripts/pack.sh`, checks each one's `package.json` and what
   `npm pack --dry-run --json` would upload against an allowlist in both directions,
   and wants the entry's `optionalDependencies` to be exactly the aliases to this
   release's builds. It reads each platform binary's architecture, runs this machine's
   binary for `--version` — the version, built from HEAD with no local changes — and has
   npm install the release from a registry it serves on the loopback interface: this
   machine's build alone, whose command then runs, each other build alone for its cpu,
   and none with `--omit=optional`, whose command then fails naming the reinstall. It
   runs the shim's own tests, `go test ./scripts/`, too. A dry run goes on past a failed
   check, so one run shows every problem, and exits 1 when any failed.
   `go run ./tools/release --help` lists the checks and the codes.
3. With the owner's word, the same with `--publish`, and `--otp <code>` when the
   account asks for one:

   ```bash
   go run ./tools/release 1.0.0 --publish --otp 123456
   ```

   A failed check before the build stops it there, and nothing is published unless
   every check passed. The platform builds go up first, with `--tag linux-x64` and
   `--tag linux-arm64`, and the entry last with `--tag latest` (`next` for a
   prerelease), so an install between two uploads never finds an entry whose binary is
   missing, and no build ever holds `latest`: npm gives it to an upload named with no
   tag. Each goes with `--access public` to `https://registry.npmjs.org/`, named on the
   command line together with the scope's registry, since a scope setting on the machine
   would outrank `--registry` alone.
4. Tag the release with the command the gate prints; the gate creates no tag.

The version check frees a version only on npm's own `E404`. `npm view` fails the same
way for a version that is not there and for a registry it could not reach, and a gate
that took every failure for the first would publish over a network error; a 403, a
timeout or an answer that does not parse stops the release as unsafe to decide.

The version is semver without build metadata: npm strips a `+build` part when it
publishes, so `1.0.0+build.1` would be checked and tagged as one number and published
as another. A platform build's own version, such as `1.0.2-linux-x64`, is a semver
prerelease too, and the gate refuses it as a release, naming the release it belongs to. `npm publish` gets `--dry-run=false`, which outranks a dry-run setting
inherited from the environment or an npmrc, under which npm skips the upload and
still exits 0.

The first failed upload stops the rest, and the gate prints what was published, what
was not and what is unknown. A refusal from the registry — a 4xx code or `EOTP` — means
the package was not published. Any other failure, a lost connection or a timeout, may
have come after the registry stored it, so the gate asks the registry once: a package
that is there counts as published and the next upload follows; one that is not, or
cannot be asked about, is unknown, and its version is not reused until `npm view`
still answers `E404` a while later. A version any upload already took cannot be reused — the registry
refuses a version once taken, even after an unpublish, and the gate refuses it too —
so a release that stopped halfway continues as the next version. After the uploads
the gate asks the registry for each version and for the dist-tags — `latest` on the
entry, each platform tag on its build — and reports without retrying. A version not
visible right after its upload, or a tag not on it yet, is usually the registry's
delay, but only a later look tells it from an upload npm skipped, so the run then ends
with 1, names the `npm view` to repeat or the `npm dist-tag add` that fixes a tag, and
prints no tag.

`go run` reports every non-zero exit as 1; build the gate first when the codes matter:
0 every check passed (and with `--publish`, every upload went up and is visible under
its tag), 1 a check or an upload failed or a version or tag is not visible yet, 2 the
call was wrong.
