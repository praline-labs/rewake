# rewake

Let the coding agents running on your machine message each other.

You have several Claude Code and Codex sessions open, each in its own terminal, and
none of them knows the others exist. With rewake, one session hands out work, the
others take it, and their reports come back by themselves. A session that sits idle
wakes up when mail arrives.

```bash
npm install -g @praline-labs/rewake
rewake guide
```

## Quick start

Install and log in to Claude Code or Codex as usual. Then start each session through
rewake, from its own terminal:

```bash
rewake --main --name lead claude   # terminal 1: lead-claude hands out work
rewake --write codex               # terminal 2: write-codex takes it
```

Each agent is briefed at launch and told to run `rewake guide` on its first task, so
you talk to it in plain words. In lead-claude, type:

> Ask write-codex to run the test suite and tell me what fails.

Here is what happens:

1. lead-claude runs `rewake send write-codex "run the test suite and report what fails"`.
2. write-codex is idle. It wakes with a line like `Rewake: lead-claude task, 1 new message`,
   and reads the task with `rewake inbox`.
3. write-codex does the work and ends its turn. Its final message goes back to
   lead-claude as `Rewake: write-codex finished`, with the report.

Nobody copies text between windows or reminds a worker to reply. Run `rewake list` in
any terminal to see who is up. Either harness can take either role: `rewake --write claude`
works the same way.

## What it can do

**Messages**

- **Tasks.** `rewake send <name> "..."`: the recipient works on it, and its final message
  comes back as the report.
- **Questions.** Add `--question` and the sender blocks until the answer arrives, then
  prints it.
- **Heads-ups.** `--notify` informs the recipient; no report comes back.
- **Waking and steering.** An idle recipient wakes up. A busy one gets the notice
  without waiting for its turn to end.
- **Interim reports.** `rewake pending "suite still running"` marks a turn that ends
  before the work does: the sender hears the work is still going, and a later turn
  reports.
- **Additions and corrections.** `--to <id>` adds to a task you already sent. While a
  message is unread, `rewake withdraw <id>` takes it back and `rewake edit <id> "..."`
  replaces it.
- **Delivery status you can trust.** `send` exits 0 when the message is delivered, 3 when
  it is accepted and lands on its own later, 1 when the target refused or could not be
  reached, and 2 when the call was wrong. `rewake inbox --awaited` lists what you handed
  out and where each item stands. `rewake inbox --owed` shows again what you still owe,
  for example after a context compaction.

**Roles and rooms**

- **Roles.** Pick one with `--main`, `--write` or `--general` (the default). Each room
  has one main: it hands out work and receives reports, and only main can grant access,
  compact a worker or interrupt one.
- **Rooms.** `--room <name>` keeps separate groups of sessions apart. Sessions see and
  address only their own room.
- **Addresses.** A session's address is `<prefix>-<harness>`, for example `lead-claude`
  or `write-codex`. The prefix is the role, or `--name`.

**Control from main**

- **Telemetry.** In main, `rewake list` shows each session's activity, model and
  effort, context fill, usable window and compaction count. Main is also told when a
  session becomes available, compacts or leaves.
- **Compact.** `rewake compact <name> [focus]` compacts an idle worker's conversation,
  optionally telling the summary what to keep.
- **Interrupt.** `rewake interrupt <name>` stops the worker's current turn, as Esc would.

**Write grants**

- **Directories.** `send --grant-dir <dir>` lets the recipient write a directory outside
  its workspace while the task is open; when it is taken back depends on the harness.
- **Git.** `send --grant-git` opens a Codex write session's Git metadata, so that it can
  commit.

**Parallel work**

- **Worktrees.** `rewake claude --worktree=fix-login` starts the session in its own
  checkout, on its own branch. `rewake worktree land fix-login` fast-forwards your
  branch to it. `finish` lands the branch and removes the worktree; `ls` and `rm`
  list and remove worktrees.

**Launch conveniences**

- **Aliases and defaults.** Name a launch in `.rewake.toml` or
  `~/.config/rewake/aliases.toml`, and start it with `rewake <alias>`. Set default
  models and efforts in `~/.config/rewake/settings`.

**For agents**

- **Built for scripts.** Every command takes `--json`, nothing ever prompts, and a
  refusal names the next thing to try.

## Claude Code and Codex side by side

| Capability | Claude Code | Codex |
|---|---|---|
| Wake an idle session | yes | yes |
| Reach a session mid-turn | yes, at its next tool call | yes, steers the running turn |
| Report when the turn ends, or fails | yes | yes |
| Report a stop by Esc at the keyboard | yes | yes |
| `pending`, `--question`, `withdraw`, `edit` | yes | yes |
| Telemetry in main's `rewake list` | yes | yes |
| `rewake compact` | yes, with a focus | partial: no focus |
| `rewake interrupt` | yes | yes |
| Directory grant | partial: spares prompts, no boundary | yes: a sandbox root, when sandboxed |
| Git grant (`--grant-git`) | no: commits within its own permissions | yes, to a write session |
| Worktree per session (`--worktree`) | yes | yes |
| Follow the conversation across `/clear`, `/new` | yes, not yet seen live | yes |
| Grants back after a resume | partial: `--resume <id>` or `--continue`, not `/resume` | yes |
| Mail held when a resume ends up in another conversation | no | yes, until resumed or `rewake accept` |
| Act as main that grants | yes | no: main, but cannot grant |

## How it works

`rewake claude` or `rewake codex` starts the harness as a child process in your
terminal, with the same flags, login and screen. rewake registers the session under
a name. Messages are files in a private state directory. The session's wrapper
announces each message through a channel the harness offers itself:

- Claude Code: its session inbox socket.
- Codex: a private app-server that starts or steers a turn.

When a turn ends, its final message goes back to whoever is waiting for it.

rewake does not proxy the terminal, type into anyone's screen, or read transcripts.
It runs no daemon: each wrapper lives exactly as long as its session. rewake never
edits your harness configuration; it passes what it needs as flags for that one
launch.

## Commands

| Command | What it does |
|---|---|
| `rewake`, `rewake guide` | The map: every command, the usual order of work, the exit codes |
| `rewake claude [args]`, `rewake codex [args]` | Start a session; your arguments pass through to the harness |
| `rewake list` | The running sessions of this room |
| `rewake send <name> <text>` | Send a task; add `--question` or `--notify` for the other kinds |
| `rewake inbox` | Read waiting mail; `--peek` previews it, `--owed` shows what you owe, `--awaited` what others owe you |
| `rewake pending <text>` | Mark a turn that ends before the work does |
| `rewake withdraw <id>`, `rewake edit <id> <text>` | Take back or replace an unread message |
| `rewake compact <name>`, `rewake interrupt <name>` | Steer a worker (main only) |
| `rewake accept <name> <conversation>` | Let a resumed Codex session take mail in the conversation its terminal went on in (the person, outside any session) |
| `rewake worktree ls \| land \| finish \| rm` | List, land, finish or remove the worktrees made for `--worktree` launches |
| `rewake whoami` | Show this session's name, room and role |

`rewake <command> --help` gives the flags, notes and real examples for each command.

## Requirements

- Linux on x64 or arm64.
- Node and npm, to install only. rewake is a single static binary and does not run on
  Node.
- Claude Code or Codex, installed and logged in on its own. Tested with Claude Code
  2.1.280 and Codex 0.155.1 and 0.157.1. A Codex launch prints a note when the
  installed version differs from the one the transport was last observed with.

## Limits

- Only sessions started through rewake take part. A plain `claude` in another terminal
  cannot be reached; restart it with `rewake claude`.
- One machine, one user. Nothing crosses the network, and messages cannot cross rooms.
- A task sent from a plain shell gets no report back: there is no session to deliver it
  to. `--question` needs one too.
- A directory grant on Claude Code spares prompts but is no boundary: an approved shell
  command writes anywhere anyway. A Codex main cannot grant, and `--grant-git` reaches
  only a Codex write session.
- rewake depends on each harness's behaviour, which can change with a new release.
  What was checked, and on which version, is recorded in `docs/research*.md`.

## Install

```bash
npm install -g @praline-labs/rewake
rewake --version
```

npm installs a small launcher and the binary for your platform.

To build from source, you need Go 1.25 or newer:

```bash
git clone https://github.com/praline-labs/rewake.git && cd rewake
go build -o rewake ./cmd/rewake
```

[docs/install.md](docs/install.md) covers stamping the build time into `--version`,
replacing an installed binary safely, and how a release is made.

## Learn more

- [docs/flow.md](docs/flow.md): the path of one message, from launch to report.
- [docs/design.md](docs/design.md): the design, the state directory and the CLI
  contract.
- [docs/traps.md](docs/traps.md): behaviour that surprises, listed by symptom.
- [docs/README.md](docs/README.md): the full map of the documentation.
- [docs/roadmap/README.md](docs/roadmap/README.md): what works today and what comes
  next.

## License

MIT. See [LICENSE](LICENSE).
