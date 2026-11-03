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

To update, run `rewake update`, which shows a newer release and what installing it restarts, and
`rewake update <version>` to install it ([below](#updating-from-inside-rewake)); the same
`npm install -g` again does it too. To remove, `npm uninstall -g @praline-labs/rewake`, and the
build goes with the entry. A running room keeps the build it was launched with: its
sessions run the room's own copy of it, a `rewake` command typed in a terminal there is handed to
that copy with one line saying so, which points at `rewake restart` for what the room must restart:
only the shell, when only it changed, and else main — its workers end with it — which moves the
room to the new build ([rules/state.md](rules/state.md#the-rooms-own-copy)). A pi
worker raised in an old room after an update that changed the bundled pi is refused until then.

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
  edit them. rewake's face, `face/` in the checkout, is compiled into the shell and needs no step
  of its own; `REWAKE_FACE=<checkout>/face/src/worker.ts` has any shell run the face from those
  sources, reloaded as you edit them ([face.md](face.md#the-faces-own-code)).
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

## The parts and what an update restarts

Owner decision, November 1, 2026: every part of rewake ships in one release, any part can be
restarted on its own for a quick reload, and an update says what it will restart. A build writes
a manifest of its parts, `manifest.json` beside the installed `rewake`: `bin/manifest.json` in
each platform package, written by `tools/release` after `scripts/pack.sh`, and
`<install>/manifest.json` for the owner's install, written by `tools/land`
([testing.md](testing.md)). It is `{"manifest":1,"parts":[…]}`, each part with its name, its
restart class, a `digest` of what it is made of, a `file` (the SHA-256 of what it ships as) where
it ships as one, `within` when it is compiled into another, and the contracts it `speaks` and
`requires`. It is not a version of the room's state: one build per room stays as
[rules/state.md](rules/state.md) says. The code is `internal/infra/parts`.

| part | digest | class |
|---|---|---|
| `rewake` | the Go and embedded files of the module that `go list -deps ./cmd/rewake` names, `go.mod`, `go.sum`, the toolchain's version and the platform, with the release's line `var Version = "…"` of `internal/cli/registry.go` read as `var Version = ""`; `file` is the build id | switch: `rewake restart rewake`, the running room switched to it while every session runs on; room — end main and start it again, which ends its workers — when the two builds differ on a contract |
| `shell` | `shell/` without `node_modules`, tests and hidden files; in a release also `file`, of `bin/rewake-shell` | shell: `rewake restart shell`, or Ctrl+\ then `r` |
| `face` | `face/` the same way; in a release `within: shell` | live: it follows its files; `rewake restart face` reloads it by hand |
| `pi` | `npm:@earendil-works/pi-coding-agent@<version>` | sessions: each new session takes it |

The build time stays out of every digest, so two builds of one tree have one, and so does the
release's version: every release changes that line, and a release that changes only the shell
must not ask for a room restart (main's decision, November 1, 2026). The line is the gate's
contract, `parts.VersionLine`, which the gate checks too; a `registry.go` with none or with two
fails the digest. A build carries its digest: `tools/release` takes it for each platform and
hands it to `scripts/pack.sh` (`REWAKE_DIGEST_LINUX_X64`, `_ARM64`), `tools/land` the same, as
`-X …/internal/cli.digest`, so the running `rewake` says its digest beside its file. The contracts,
each said by the part itself (`rewake --version --json`, `rewake-shell --contracts`), never read
from the sources: `host`, the host channel's version, spoken by rewake and required by the shell;
`face`, the face's `contract`, spoken by the face and required by the shell; `mirror`, spoken by
rewake and required by the face; `pi`, the pinned version, spoken by pi and required by
rewake; and those a switch of the running room compares
([rules/state.md](rules/state.md#switching-a-running-room)), all spoken by rewake: `handover`,
the shape of a wrapper's handed run; `handover.<adapter>`, an adapter's handed state
(`handover.fixture` in a test build); `switch`, the switch's mark and messages; `mod`, the
protocol between Claude Code's mod and its host; and `extension`, pi's extension protocol. The room view and the space stream's mode bytes have no number: they change only with
rewake, which is a room restart anyway, so after a landing that changes both rewake and the shell
`restart shell` may load a shell the room's older view does not suit, while the plan says room
(decided by main, November 1, 2026).

Which parts `rewake restart <part>` restarts in place — the shell and the face — is one answer in
`internal/infra/parts` (`InPlace`, with the words of a refusal, `NotInPlace`): the command and the
advice of a plan's step come from it. The shell's banner reads only each step's class from the
room view and words the person's key for it itself, so it holds no list of parts.

The handover contracts of the space host and of main's host (`handover.space`, `handover.mainhost`)
are made by `stream.Contract` from the handover's version and the screen file's: the handover's own
number while the screen file is at its first version, `<handover>.<screen>` after. A raise of either
is a contract a switch refuses, and today's value is the one before the screen was named.

### The running set

Nothing records it in the room. `rewake` is the CLI's own build: a room's command runs as the
room's own copy. The shell is what main's host started: the host takes its digest with the
manifest's function when it starts it (the sources' digest for a checkout's `rewake-shell`, else
the file's, with the manifest's `digest` added when the installed manifest describes that file),
and its contracts from the shell's hello ([shell.md](shell.md#the-hosts-channel)), and serves
both at its door; a compiled shell's hello says its face's contracts too, served as a face
`within` the shell. The face from sources and pi are not recorded: the face follows its files
unless the shell's contract refuses it, and a pi session runs the version rewake requires.

### The plan

`parts.Plan` compares the running parts with the installed manifest: a part is the same when its
digests match where both have one and its files where both have one, so a compiled shell whose
embedded face alone changed is a change; `rewake` alone is judged by its digest where both have
one, since a release rebuilds it with its version inside and its file always differs, while its
file stays the build's identity for the room's lease and hand-off; one that changed restarts by its class, an unknown class
as room, and a running shell that said no contracts gets its own step. A part `within` another
moves with it: its contracts count as its container's, so a restart of a compiled shell is judged
by its face's as well (found in review, November 1, 2026). Then every requirer must
find its contract spoken at its value once the steps are taken; where it would not, the part
that must move is raised to its class, and the reason names the contract (a face on disk that
speaks 6 to a shell that requires 5 is a shell restart; a pi on disk that rewake does not pin is
a room restart). `rewake restart` with no part prints the plan, one line per step, or `nothing to
restart`; an install without a manifest — `go run`, a build by hand — has no plan, and
`restart shell` works there as before. `rewake restart shell` and `restart face` refuse when the
installed part would require what the running rewake does not speak; `restart rewake` switches the
running room to the installed rewake when every session and host can hand its run over and the
installed build speaks and requires the room's contracts at the same values; otherwise it is
refused, naming who cannot or which contract, and the room keeps its build
([rules/state.md](rules/state.md#switching-a-running-room)). The plan says the same: a changed
`rewake` is of class `switch`, `rewake restart rewake`, when the running and the installed build
speak and require every contract at the same values (`parts.Admits`, which the switch itself
checks), and of class `room` otherwise, its reason naming the contract; a rewake older than the
class reads it as `room` (November 2, 2026). A harness rewake ships
refuses, naming the room's restart. The shell shows the plan's shell, switch and room
steps in its hints row ([shell.md](shell.md#the-prefix-and-the-room)).

## Updating from inside rewake

Owner decision, November 1, 2026: rewake updates from inside, shows a banner when npm has a newer
release, keeps a changelog, and says what an update restarts. Only a build from npm does:
`scripts/pack.sh` builds it with `-X …/internal/infra/update.Channel=npm`, and any other — a
`tools/land` install such as the owner's, a build by hand, `go run` — refuses `rewake update`,
naming its rebuild, and keeps only the plan's banner. The code is `internal/infra/update`.

- **The check.** `npm view @praline-labs/rewake@latest version --json`, run as a child, 30 s at
  most. Every npm call of the feature — `view`, `pack` and `install` — takes `--global --prefix
  <prefix>`, the prefix being the install's own (the origin's path above `lib/node_modules/`), so
  all three read one configuration — the user's `.npmrc`, the prefix's `etc/npmrc`, the
  environment and its proxy — and never a project's `.npmrc`, which a global install ignores:
  the preview cannot come from another registry than the install. When the latest release is
  another than the last check's, `npm pack` fetches its entry into a scratch directory of the
  system's, removed after, and `CHANGELOG.md` and `manifests/<platform>.json` are read from it: a
  few KB, never the platform build. Main's host checks 30 s after it starts and then whenever the
  record is 6 h old, read every 10 minutes; `rewake update` checks afresh. A view only reads.
- **The record.** `<base>/v2/update.json` — `checked`, `latest`, the target's `manifest` and
  parsed `changelog`, and the last `error` — one per state base, so the rooms of a machine share
  it, written by rename through `state.WriteUpdateRecord` with no room's lease, being no room's
  state ([rules/state.md](rules/state.md)); an unreadable record is a missing one. A failed check
  keeps the last good answer and records the error and the time, so the host does not retry every
  poll; offline, the banner shows the last good answer or nothing.
- **`rewake update`** prints `running · installed · latest`, the changelog's entries after the
  running release up to the latest, by release and part, and the plan from what the room runs to
  the latest release's manifest, one line per step as `rewake restart` prints it — computed, never
  read from the changelog; only a release is offered, never a prerelease, so a stable build is never
  moved onto one. `rewake update <version>` takes the exact release only — no range, no `latest`,
  not a platform build's version; a prerelease such as `3.1.0-rc.1` only so — which the registry
  must have (npm's `E404` is "no such release"); then `npm install -g @praline-labs/rewake@<version> --prefix <prefix>`, npm's
  output on stderr so `--json` stays clean, a check that the install the command came from now
  holds that release, and the plan of the new install. The plan reads the install the command
  came from, taken before npm ran: npm 11.17 moves the running directory aside to replace it (seen
  on November 1, 2026). Installing restarts nothing: the room runs its own copy, and
  `running` stays the old release until main restarts or the room switches (`rewake restart rewake`). An older release is a rollback. Main or
  the person runs it; a worker is refused.
- **The banner.** The room view's `available` is `{version, restarts}` when the record's latest is
  a release newer than both the installed and the running one; `restarts` is `room` when the plan to it
  has a room step, else `switch` when it has a switch, else `shell`, else `nothing`
  ([shell.md](shell.md#the-prefix-and-the-room)). Once it is installed, the plan's banner takes
  over.
- **The changelog.** `CHANGELOG.md` at the repository root, shipped in the entry: `## <version>`
  newest first by semver's precedence, a prerelease below the release it leads to and kept there,
  `### <part>` a part of the manifest or `docs`, and `- ` lines, a line continued by two spaces. It is written by hand at release time from `git log` since the last release;
  the gate refuses a release without its entry.

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
the binary, with the face the shell carries, installs each other build as npm would choose it
for that cpu, and installs the
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
   holds the version a build from source reports, `var Version = "2.11.0"` today; the
   packages get theirs from `scripts/pack.sh`, and the gate refuses before the build
   when the line at HEAD names another version than the one released, so the tagged
   source and the npm binary cannot disagree (owner decision, September 28, 2026). The
   same commit adds the release's entry to `CHANGELOG.md`, which the gate requires at HEAD
   ([above](#updating-from-inside-rewake)). It also wants a clean tree whose HEAD
   `origin/main` contains, as `git push` left the remote-tracking ref.
2. The dry run, which publishes nothing:

   ```bash
   go run ./tools/release 2.0.0
   ```

   It prints the changelog's entry of the release, refusing a release without one, and asks
   the registry for each of the three versions exactly — `2.0.0`,
   `2.0.0-linux-x64`, `2.0.0-linux-arm64` — and wants none of them there, builds them
   with `scripts/pack.sh`, each with its rewake's digest, writes each platform package's
   `bin/manifest.json` ([above](#the-parts-and-what-an-update-restarts)) and the same bytes into
   the entry as `manifests/<platform>.json`, checks each one's `package.json` and what
   `npm pack --dry-run --json` would upload against an allowlist in both directions,
   and wants the entry's `optionalDependencies` to be exactly the aliases to this
   release's builds. It reads the architecture of each platform binary and of the shell beside
   it, runs this machine's
   binary for `--version` — the version, built from HEAD with no local changes — and has
   npm install the release from a registry it serves on the loopback interface: this
   machine's build alone, whose command then runs, with the pinned pi beside it running
   under node, the command launched on a terminal as a person's launch is, a stand-in pi named
   by `--command` as main, whose output and hints row the shell beside the binary draws under
   the launch's host, and that shell starting its own face; each other build alone for its cpu,
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
