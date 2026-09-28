# rewake

Let the coding agents running on your machine message each other.

Start an agent through rewake and it runs in your terminal as usual — same
flags, same login, same screen. rewake only registers the session and carries
messages:

```bash
rewake --main --name lead claude   # terminal one: lead-claude hands out work
rewake --write codex              # terminal two: write-codex takes tasks
rewake list                       # exact addresses in this room
rewake send write-codex "the migration is merged, pull and rerun the smoke"
```

Run the send command from lead-claude. The receiver sees
`Rewake: lead-claude task, 1 new message(s)` and reads the text with `rewake inbox`.
An idle session wakes up; the server transport can steer an active turn. The
receiver ends its turn with the result, which returns as `Rewake: write-codex finished`.

Without a role flag, every session starts as general, even in an empty room.
`rewake codex` starts general-codex, then general-codex-2. Only `--main` creates
an orchestrator; `--name main` changes the address, not the role.

Names use the selected role as a prefix, followed by the harness ID. `--name`
replaces only the prefix: `rewake --write --name megamozg codex` starts
megamozg-codex. Automatic collisions add -2, -3; explicit conflicts refuse.
The complete address must fit 32 characters. Existing sessions keep their names.

Delivery uses what each harness already offers — the session inbox socket of
Claude Code, the owned app-server of Codex — so nothing is typed into anyone's
screen and no terminal is proxied.

Run `rewake` with no arguments for the map of commands, the usual order of work,
and the exit codes.

## Works with

- **Claude Code** — `rewake claude`
- **Codex** — `rewake codex`

Each harness is installed and logged in on its own; rewake starts the one you name,
passes your arguments on — the few exceptions are on `rewake claude --help` and
`rewake codex --help` — and never edits its configuration. Sessions of both
kinds talk to each other in one room.

## Install

```bash
npm install -g @praline-labs/rewake
rewake --version
```

Linux on x64 or arm64. npm installs a small launcher and the binary for your
platform; Node and npm are needed only to install it — rewake itself is a single
static binary and does not run on Node.

From source, with Go 1.25 or newer:

```bash
git clone https://github.com/praline-labs/rewake.git && cd rewake
go build -ldflags "-X github.com/praline-labs/rewake/internal/cli.built=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o rewake ./cmd/rewake
```

The `-ldflags` is optional: it passes in the build time, which `rewake --version` shows.
[docs/install.md](docs/install.md) covers replacing an installed binary safely and how
a release is made.

## Status

See [docs/roadmap/README.md](docs/roadmap/README.md) for what works today and what
comes next.

## License

MIT — see [LICENSE](LICENSE).
