---
description: Installing rewake with npm or from source, updating it, a second build beside an installed one, testing how a release installs, upgrading from 1.x, and releasing to npm.
---

# Installing and releasing

## Installing with npm

```bash
npm install -g @praline-labs/rewake
rewake --version
```

Linux on x64 or arm64. npm installs a small entry, the build for this machine's cpu — the
binary and rewake's shell beside it — and the pi rewake runs, at the exact version this build
was checked with. pi runs under Node.js 22.19 or newer (`node --version`); a `pi` of your own on
`PATH` is not used. Claude Code is installed and logged in on its own, 2.1.287 or newer.

The first use asks for two logins, each once: `/login` in the first Claude Code worker's pane,
for the workers' configuration directory of their own, and `/login` in the first pi session, for
rewake's pi. What else is left to the person — the worker templates, a project's `.rewake/`, the
shell's keys, where things live — is in [user-guide.md](user-guide.md).

To update, run the same `npm install -g` again; to remove, `npm uninstall -g @praline-labs/rewake`,
and the build goes with the entry. Sessions already running keep the build they started with,
and a room is one build: end a room's main, and its workers with it, before starting main there
on the new build.

**Limits** a person meets:

- The harnesses are Claude Code and pi. pi runs only at the version installed with rewake; a pi
  named with `--command` (a template's `command`) runs from 1.2.0, with a note when it is not
  that version.
- Sessions read and send mail with the `rewake` command in their shell, so a harness that may
  run no shell commands cannot take part. Only sessions started through rewake take part, on one
  machine.
- No grants: main cannot give a worker a directory or Git permissions through rewake. A worker
  has what its own launch gives it, such as the harness's permission flags in its template.
- A main started in print mode (`claude -p`) gets no report: it exits before a worker's turn
  ends.

The entry package's `bin` is a POSIX script rather than a Node shim: it execs the
binary and disappears, so the terminal, the signals and the exit code belong to
rewake rather than to a process in between. It follows the symlink npm installs
first — resolving the link is the difference between finding the binary and
reporting it missing.

## Building from source

Go 1.25 or newer, Git, [Bun](https://bun.sh) for the shell and Node.js 22.19 or newer for pi.
rewake finds its shell and its pi beside its own executable, symlinks followed, so a build lives
in a directory of its own, here `~/.local/share/rewake`, linked onto `PATH`. From the checkout:

```bash
dir=~/.local/share/rewake
mkdir -p "$dir"
go build -ldflags "-X github.com/praline-labs/rewake/internal/cli.built=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o "$dir/rewake.new" ./cmd/rewake
mv -f "$dir/rewake.new" "$dir/rewake"
(cd shell && bun install --frozen-lockfile && bun build.ts bun-linux-x64 "$dir/rewake-shell")
npm install --prefix "$dir" @earendil-works/pi-coding-agent@1.4.0
ln -sf "$dir/rewake" ~/.local/bin/rewake
```

- **The build time.** The `-ldflags` passes it in, and `rewake --version` and the guide show
  it: the binary cannot know it otherwise. Built without it, they show the revision's commit
  time instead, labeled `committed`, and two builds of one revision with a modified tree
  look alike. `rewake --version` names the version, the revision the binary was built from
  (`build unknown` for one built outside a git checkout) and the build time.
- **Replacing a running build.** Building directly over a running executable can fail with
  `text file busy`; renaming a neighboring file avoids that. Existing wrappers keep their old
  executable, while new launches use the replacement.
- **The shell** is compiled again when an update changes `shell/` (`bun-linux-arm64` on an
  arm64 machine). Without it, `rewake claude` at a terminal refuses, naming where it looked.
  `REWAKE_SHELL=<checkout>/shell/rewake-shell` runs the shell from its sources instead, as you
  edit them.
- **pi** is looked for in a `node_modules` above the executable's directory, as node resolves a
  dependency. The version is `PinnedVersion` in `internal/adapter/pi/ship.go`; when an update
  changes it, install that one, or the start of a pi session refuses, naming both versions.
  `REWAKE_PI=<pi package directory or entry script>` names another copy instead.

### A second build beside an installed one

A build under test can run beside the installed `rewake` with state of its own, under another
command name — `rewake-dev` here. Build it as above into a directory of its own,
`~/.local/share/rewake-dev`, without the link, and create `~/.local/bin/rewake-dev`:

```sh
#!/bin/sh
export REWAKE_DIR="$HOME/.rewake-dev"
export REWAKE_WORKTREES="$HOME/.rewake-dev-worktrees"
exec "$HOME/.local/share/rewake-dev/rewake" "$@"
```

and make it runnable: `chmod +x ~/.local/bin/rewake-dev`. The script gives every command of that
build its own state base (`REWAKE_DIR`) and its own place for `--worktree` checkouts
(`REWAKE_WORKTREES`), so nothing of the installed build's is read or written. The binary itself
stays named `rewake`: inside a session the agent types `rewake inbox`, `rewake send` and so on,
and a launch puts the directory of the binary that started it first on the session's `PATH`, so
in a session of either build `rewake` is that build. The logins and the folders under
`~/.config/rewake/` are shared by both.

To see which build a running wrapper is, `readlink /proc/<servicePid>/exe`: a path ending in
`(deleted)` means the file was replaced under it and the session needs a restart. To remove the
second build: end its sessions, then `rm ~/.local/bin/rewake-dev` and
`rm -rf ~/.local/share/rewake-dev ~/.rewake-dev`, and `~/.rewake-dev-worktrees` once nothing in
the checkouts there is wanted (`rewake-dev worktree ls` lists them first).

## Testing how a release installs

`scripts/pack.sh [version]` builds the npm uploads into `dist/npm` — a build per platform plus
the entry, all versions of the one package `@praline-labs/rewake` — and publishes nothing. It
needs Bun, which compiles rewake's shell into each platform build beside the binary. The entry
reaches its builds through `npm:` aliases, which npm resolves only against a registry: an install
from the tarballs on disk would look for the builds on the public registry. So the install is
tested by the release gate's dry run, which serves the release from a registry of its own on the
loopback interface and has npm install it there:

```bash
go run ./tools/release 2.0.0
```

It installs the entry globally into a scratch prefix and runs the command and the shell beside
the binary, installs each other build as npm would choose it for that cpu, and installs the
entry with `--omit=optional` to see the command fail naming the reinstall. That last one goes
into a project rather than globally: npm 11.17 installs a global package's optional
dependencies even under `--omit=optional`, seen on September 28, 2026. On a tree with
uncommitted changes or an unreleased version the checks before the build fail, and the build
and the install checks still run and report.

That exercises everything a registry release would except the public registry itself: the
package contents, npm resolving the aliases and choosing a build by `os` and `cpu`, the shim
resolving its binary, and the command landing in PATH.

## Upgrading from 1.x

rewake 2.0 does not read the state of 1.x and carries none of its mail over. Before the
update, end every rewake 1.x session and run no 1.x command; then install 2.0 and launch.
A 1.x tree under the state base is left as it is: 2.0 lives under its own root `v2/`, reads
nothing of the old tree, and the tree can be removed by hand once no 1.x session runs.

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
two versions now install without a binary and are deprecated, pointing at the latest
release.

A deprecation, or anything else that takes a version range, names exact versions: under
this layout a range such as `<1.0.2` also matches `1.0.2-linux-x64` and
`1.0.2-linux-arm64`, since a build's suffix makes it a prerelease that sorts below its
release, and the mark would land on the builds the current release installs.

Owner decision, October 19, 2026: pi is part of rewake. The entry depends on
`@earendil-works/pi-coding-agent` at the exact version `scripts/pack.sh` reads from
`PinnedVersion` in `internal/adapter/pi/ship.go`, so npm installs and updates pi with rewake;
a launch starts that copy with `node` and refuses any other version
([launch.md](launch.md#pi)). The gate's install serves pi and its dependencies from the public
registry beside the release's own packages, and checks that npm put the pinned pi where the
command looks for it and that it runs. Raising the pin is a change of `PinnedVersion` with the
checks of [research-pi.md](research-pi.md#the-bundled-pi) run again on the new version.

Every release goes through the gate, `tools/release`:

1. Set the version in the release commit and push it. `internal/cli/registry.go`
   holds the version a build from source reports, `var Version = "2.7.1"` today; the
   packages get theirs from `scripts/pack.sh`, and the gate refuses before the build
   when the line at HEAD names another version than the one released, so the tagged
   source and the npm binary cannot disagree (owner decision, September 28, 2026). It
   also wants a clean tree whose HEAD `origin/main` contains, as `git push` left the
   remote-tracking ref.
2. The dry run, which publishes nothing:

   ```bash
   go run ./tools/release 2.0.0
   ```

   It asks the registry for each of the three versions exactly — `2.0.0`,
   `2.0.0-linux-x64`, `2.0.0-linux-arm64` — and wants none of them there, builds them
   with `scripts/pack.sh`, checks each one's `package.json` and what
   `npm pack --dry-run --json` would upload against an allowlist in both directions,
   and wants the entry's `optionalDependencies` to be exactly the aliases to this
   release's builds. It reads the architecture of each platform binary and of the shell beside
   it, runs this machine's
   binary for `--version` — the version, built from HEAD with no local changes — and has
   npm install the release from a registry it serves on the loopback interface: this
   machine's build alone, whose command then runs, with the pinned pi beside it running
   under node and the shell beside the binary drawing a main's output, each other build
   alone for its cpu,
   and none with `--omit=optional`, whose command then fails naming the reinstall. It
   runs the shim's own tests, `go test ./scripts/`, too. A dry run goes on past a failed
   check, so one run shows every problem, and exits 1 when any failed.
   `go run ./tools/release --help` lists the checks and the codes.
3. With the owner's word, the same with `--publish`, and `--otp <code>` when the
   account asks for one:

   ```bash
   go run ./tools/release 2.0.0 --publish --otp 123456
   ```

   A failed check before the build stops it there, and nothing is published unless
   every check passed. The platform builds go up first, with `--tag linux-x64` and
   `--tag linux-arm64`, and the entry last with `--tag latest` (`next` for a
   prerelease), so an install between two uploads never finds an entry whose binary is
   missing, and no build ever holds `latest`: npm gives it to an upload named with no
   tag. Each goes with `--access public` to `https://registry.npmjs.org/`, named on the
   command line together with the scope's registry, since a scope setting on the machine
   would outrank `--registry` alone.
   With `--wait` the gate waits for the registry after the uploads: it asks again every 30
   seconds until every version and dist-tag is there, 40 minutes unless `--wait=<duration>`
   says otherwise (arm64's build of 2.5.0 took 25 minutes to show), and only then reports and
   prints the tag, or ends with 1 as below. It replaces waiting in a shell loop, which an
   agent's tool cuts off after ten minutes.
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
its tag), 1 a check or an upload failed or a version or tag is not visible yet (with
`--wait`, by the end of the wait), 2 the call was wrong.
