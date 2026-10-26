---
description: Installing, updating and releasing rewake, and running a 2.0 build beside 1.0.3.
---

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

**To test how it will be installed.** `scripts/pack.sh [version]` builds the npm
uploads into `dist/npm` — a build per platform plus the entry, all versions of the one
package `@praline-labs/rewake` — and publishes nothing. It needs [Bun](https://bun.sh), which
compiles rewake's shell into each platform build beside the binary. The entry reaches its builds
through `npm:` aliases, which npm resolves only against a registry: an install from the
tarballs on disk would look for the builds on the public registry. So the install is
tested by the release gate's dry run, which serves the release from a registry of its
own on the loopback interface and has npm install it there:

```bash
go run ./tools/release 1.0.2
```

It installs the entry globally into a scratch prefix and runs the command and the shell beside
the binary, installs each
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

## Trying 2.0 beside 1.0.3

This is for working on 2.0 for a while without touching the installed 1.0.3: the `rewake`
command from npm keeps working as it is, and 2.0 runs under another name, `rewake2`, with
state of its own. No knowledge of Go is needed beyond having it installed.

**Once: what is needed.** Go 1.25 or newer (`go version` prints it; otherwise install it from
<https://go.dev/dl/>), Git, and this repository checked out. `~/.local/bin` must be on `PATH`,
as it already is when 1.0.3 was installed there.

**Build 2.0.** In a terminal, from the checkout:

```bash
git switch v2
git pull
mkdir -p ~/.local/share/rewake2
go build -ldflags "-X github.com/praline-labs/rewake/internal/cli.built=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o ~/.local/share/rewake2/rewake.new ./cmd/rewake
mv -f ~/.local/share/rewake2/rewake.new ~/.local/share/rewake2/rewake
```

To update later, run the same lines again; sessions already running keep the build they
started with.

**Once, for pi: its pinned copy.** pi is part of rewake: the npm package installs the one
version rewake was checked with, and `rewake pi` starts no other (a `pi` on `PATH` is not used).
A build from the checkout has no npm install around it, so put that version beside it, where
the build looks for it — a `node_modules` above its own directory. It needs Node.js 22.19 or
newer (`node --version`):

```bash
npm install --prefix ~/.local/share/rewake2 @earendil-works/pi-coding-agent@1.3.0
```

The version is `PinnedVersion` in `internal/adapter/pi/ship.go`; when an update of the
checkout changes it, run this line again with the new one, or the start of a pi session
refuses, naming both versions. `REWAKE_PI=<pi package directory or entry script>` in the
environment names another copy instead.

**Once, for the shell: its executable.** The npm package carries rewake's shell compiled beside
the binary; a build from the checkout has none, and `rewake2 claude` at a terminal refuses, naming
where it looked. Compile it there with [Bun](https://bun.sh), from the checkout, and again when an update
changes `shell/` (`bun-linux-arm64` on an arm64 machine):

```bash
(cd shell && bun install --frozen-lockfile && bun build.ts bun-linux-x64 ~/.local/share/rewake2/rewake-shell)
```

To run the shell from its sources instead, as you edit them, put
`REWAKE_SHELL=<checkout>/shell/rewake-shell` in the environment: it names the shell to run in
place of the one beside the binary.

**Once, in the first pi session: `/login`.** rewake's pi keeps everything of its own —
login, settings, extensions, skills, prompts, themes, models, packages, trust decisions,
sessions — in `~/.config/rewake/pi/agent`, not in `~/.pi`: your own pi is neither read nor
changed, and nothing is copied from it. Log in there once with `/login`, as in pi. An
extension, skill or theme you want in rewake's pi goes into that directory's
`extensions/`, `skills/` or `themes/`, and a package is installed into it with
`PI_CODING_AGENT_DIR=~/.config/rewake/pi/agent` set for `pi install <source>`. In a
project with `.pi` resources or `.agents/skills`, pi asks once whether to trust the folder
before the session is ready for mail; `/trust` keeps the answer.

**Once: the `rewake2` command.** Create the file `~/.local/bin/rewake2` with these lines:

```sh
#!/bin/sh
export REWAKE_DIR="$HOME/.rewake2"
export REWAKE_WORKTREES="$HOME/.rewake2-worktrees"
exec "$HOME/.local/share/rewake2/rewake" "$@"
```

and make it runnable: `chmod +x ~/.local/bin/rewake2`. Check it: `rewake2 --version` prints a
line with `build` and the time of your build (the version number still reads 1.0.3 until 2.0 is
released; the build is told by `build <commit>`), and `rewake2 list` says no sessions are running.

To see which build a running service is, `readlink /proc/<servicePid>/exe`: a path ending in
`(deleted)` means the file was replaced under it and the service needs a restart.

Why a script and not a binary named `rewake2`: the script gives every 2.0 command its own state
base (`REWAKE_DIR`) and its own place for `--worktree` checkouts (`REWAKE_WORKTREES`), so
nothing of 1.0.3's is read or written. And the binary itself is named `rewake`, in a directory
of its own: inside a session the agent types `rewake inbox`, `rewake send` and so on, and a
launch puts the directory of the binary that started it first on the session's `PATH`, so in a
2.0 session `rewake` is 2.0 and in a 1.0.3 session it stays 1.0.3.

**Log the Claude Code workers in, once.** Claude Code workers have a configuration directory of
their own, rewake's `~/.config/rewake/claude/worker/`, with its own login, so their
conversations stay out of your `claude --resume` and your login is never refreshed by a
worker. The first Claude Code worker main opens shows "Not logged in · Run /login" in its
pane, and main tells you so: type `/login` in that worker's pane and log in with your account.
That once serves every later Claude Code worker.

**Start a room.** Start main in the project's directory, from your own terminal, not a shell an
agent runs (started inside a rewake session, a launch refuses and exits 2):

```bash
rewake2 claude
```

It is `main-claude`, in the room of that directory, and runs in rewake's shell. A second
`rewake2 claude` in the same directory is refused; another directory is another room. Workers
are raised by main alone: ask it for one, and it runs `rewake space open api --alias <alias>` with an alias of
`rewake workers`; the worker `api-claude` joins main's room and ends when main ends. From 120
columns the chosen worker shows beside main, and `Ctrl-\` then `n` or a digit chooses it.
`rewake2 pi` is main on pi, the same way: `main-pi`, in the shell, raising workers. Sessions of 2.0 never see
sessions of 1.0.3, nor 1.0.3 theirs.
`rewake2 space list` shows the spaces and what they left.

**Go back to 1.0.3.** End the 2.0 sessions as usual (`/exit` in each; a space's worker ends
with its main). Then remove what 2.0 added:

```bash
rm ~/.local/bin/rewake2
rm -rf ~/.local/share/rewake2 ~/.rewake2
```

Remove `~/.rewake2-worktrees` too once nothing in the checkouts there is wanted;
`rewake2 worktree ls` lists them while `rewake2` still exists. Nothing of 1.0.3 was changed:
`rewake --version` still says 1.0.3, and its sessions start as before.

**Known limits.** What 2.0 does not do yet, or no longer does:

- Codex is not supported: `rewake2 codex` is an unknown command. Claude Code (2.1.287 or newer)
  and pi are.
- pi runs only at its pinned version (1.3.0), on Node.js 22.19 or newer; another is refused at
  the start, naming the version. A pi named with `--command` (an alias's `command`) runs from
  1.2.0, with a note when it is not the pinned one.
- No grants: main cannot give a worker a directory or Git permissions through rewake. A
  worker has what its own launch gives it, such as the harness's permission flags.
- No mail tool: sessions read and send mail with the `rewake` command in their shell, so a
  harness that may run no shell commands cannot take part.
- A main started in print mode (`claude -p`) gets no report: it exits before a worker's turn
  ends.
- Since October 19, 2026 the aliases (`~/.config/rewake/aliases.toml` and a project's
  `.rewake/aliases.toml`, before October 21 `.rewake.toml`, now refused) are only main's catalogue of workers, opened with `rewake space open
  --alias`; a file of 1.0.3's form, with `rewake` lists or `main = true`, is refused naming
  each field until rewritten ([launch-defaults.md](launch-defaults.md#the-worker-catalogue)).
  rewake writes nothing there.

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
   holds the version a build from source reports, `var Version = "2.0.0"` today; the
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
