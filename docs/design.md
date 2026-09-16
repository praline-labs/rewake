# rewake: first-version design

The facts behind this design live in `docs/research.md`. This document covers what we're
building and how.

## Goal

`rewake` lets interactive coding-agent harnesses on one machine talk to each
other. A human launches a harness through the tool — `rewake claude`, `rewake
codex` — and gets an ordinary program in their terminal. The session registers
itself, and any other session, or a human from a shell, can write to it with
`rewake send <name> "text"`. The recipient is told in one line that a message is
waiting — `Rewake: api notify, 1 new message` — which wakes it if it is idle,
and fetches the text itself with `rewake inbox`.

The primary user of these commands is an agent calling them from its own shell.
That's where the requirements on output and failure messages come from (see the
"Interface" section).

## Scope of the first version

In scope: Claude Code and Codex, the commands `claude`, `codex`, `list`, `send`,
`inbox`, `whoami`, the no-argument overview, `--json`. Linux.

Out of scope: pi, opencode, grok (their delivery paths are covered in research);
macOS (needs replacements for `/proc`); delivery receipts from the recipient;
message history; a UI.

Never doing: a pseudo-terminal proxy or typing text into someone else's screen;
editing the user's harness configs; reading transcript contents.

## Design

### Processes

**Wrapper** — the `rewake claude|codex` process. It stays the harness's parent
for its whole life: launches it with an inherited terminal, registers the
session, services its inbox, and on exit deregisters the session and returns the
harness's exit code. There's no daemon: each wrapper is the delivery point for
its own session only.

**Sender** — any process that calls `rewake send`. It doesn't deliver anything
itself: it drops a file into the recipient's inbox and waits for a status. This
way sending works the same from a plain shell, from Claude Code's Bash, and from
a Codex sandbox, which can write to `/tmp` but is denied sockets and `~/.codex`.

There's no separate daemon: nothing to start, stop, or recover, and there's no
state where "the sessions exist but the daemon is dead". Exactly as many wrapper
processes run as there are open sessions; a session without a wrapper doesn't
take part in messaging.

### State directory

`$REWAKE_DIR`, defaulting to `/tmp/rewake-<uid>`. It has to be `/tmp`: the Codex
sandbox writes there by default, and `$XDG_RUNTIME_DIR` isn't reachable from it.
The wrapper passes `REWAKE_DIR` to children explicitly, so a sandbox with a
different `TMPDIR` still finds the same directory.

Checked on every open: it's a directory, not a symlink, owned by the current
uid, with no group or other access; otherwise it fails with an explanation.
Created with 0700.

```
/tmp/rewake-<uid>/
  sessions/<name>.json       session record
  inbox/<name>/<id>.json     message whose notice has not gone out yet
  inbox/<name>/<id>.status   status: pending, delivered (notice sent), read, failed
  inbox/<name>/unread/       announced, waiting for the agent to read it
  inbox/<name>/done/         read and failed, for diagnostics (cleaned up by age)
  inbox/<name>/answering/<id>   renewable reservation for a question
  inbox/<name>/received/<id>    successful answer output for a question
  inbox/<name>/awaiting/<epoch>/<peer>   per run: who waits for its turn to end, and which run of them
  sock/<name>.<epoch>.sock   Claude Code inbound socket, one path per run
```

The socket path is deliberately short: the limit is 103 bytes.

### Session record

```json
{
  "name": "claude-2",
  "harness": "claude",
  "managed": true,
  "servicePid": 12345,
  "serviceStart": 1671399,
  "harnessPid": 12346,
  "harnessStart": 1671402,
  "cwd": "/home/u/code/x",
  "startedAt": "2026-09-16T00:08:03Z",
  "claude": { "socket": "/tmp/rewake-1000/sock/claude-2.sock" },
  "codex": { "home": "/home/u/.codex" }
}
```

- `serviceStart`, `harnessStart` — field 22 of `/proc/<pid>/stat` (start time in
  ticks). Liveness = the process exists and the start time matches: pids get
  reused.
- A session is alive as long as both the servicing process and the harness are
  alive. Any reader of the registry deletes a dead record.
- Publishing a record is atomic and exclusive: write a temp file, then `link()`
  it to the final name — `link` fails if the name is taken. If the existing
  record belongs to a dead session, it's removed and the attempt retried; for a
  live one, the name stays taken.
- Updating one's own record (for example, the harness pid after launch) uses a
  temp file and `rename()`.

### Roles

A session has a role, from the catalogue in `internal/role`: one value per role
and a line in its list, the way a harness is added. The launch flag `--<id>`,
the help line, the sentence the role adds to the intro and whether its turns
are reported all come from that value. The record keeps the role's id.

| role | flag | turns reported | Git metadata writes requested for the sandbox | intro adds |
|---|---|---|---|---|
| `worker` | none (default) | yes | no | end your turn with the result |
| `main` | `--main` | no | yes | you get reports, yours go to nobody |
| `write` | `--write` | yes | yes | end your turn with the result; you can commit |

Git writes are a separate role capability from reporting. The sandbox adapter
appends `--add-dir` for the discovered metadata directories. Ordinary repos,
worktrees and submodules are supported; configuration and existing roots remain
intact. Unresolved or symlinked metadata is skipped with a reason. See
[launch permissions](launch.md). A role does not revoke permissions the user
already granted; worker receives no extra Git access from rewake.

The main session exists to stop a loop: it reads the reports of its workers,
and if its own turns were reported to them, each report would wake the other
side for good. So a silent role gets no end-of-turn hook at launch (no Stop
hook, no `notify`), records no waits when it reads, and `turn-ended` does
nothing for it. The default and zero value report turns without requesting extra Git access.
The write role reports like a worker; main stays silent whether its Git grant
was applied or skipped.

### Names

`[a-z0-9][a-z0-9._-]{0,31}`. The default is the harness name; if that's taken,
`claude-2`, `claude-3`. Explicit: `rewake --name api claude`. A session's name is
its address, so a live name is never reused.

### Environment the harness receives

- `REWAKE_SESSION=<name>` — who I am; `send` uses it to sign the sender.
- `REWAKE_EPOCH=<epoch>` — which run of that name I am. A process left over
  from an ended run keeps its environment; with this, it can neither read the
  next run's mail nor sign or report for it.
- `REWAKE_DIR=<directory>`.
- Inherited Claude Code markers are stripped (list in research): otherwise
  `rewake claude` launched from inside another session would inherit that
  session's socket and have transcript saving disabled.

None of the variable names contain `KEY`, `SECRET`, or `TOKEN`: Codex strips
those from the agent's command environment.

## Launch and signals

See [launching a harness and handling signals](launch.md).

## Delivery

See the [delivery protocol](delivery.md) for sending, reading, reports and
mailbox locking. A question reserves its answer before publication. A fresh
reservation keeps the report queued and excludes it from ordinary inbox reads;
only successful output records receipt. Every question sharing a report must
receive it before it is archived. Missing or stale reservations return unread
reports to ordinary notification. Silent roles refuse questions before sending.

## A session without a wrapper

Not supported: an owner decision. Only what's launched through `rewake` takes
part. A plain `claude` in another terminal doesn't show up in `list` and doesn't
receive messages — it has to be restarted through the wrapper.

## Interface

The conventions are carried over from i-plane, where agents have already
proven them out.

### Commands

```
rewake                                  overview (command map, workflow, behavior notes)
rewake [--name N] [--main] claude [args...]   launch a Claude Code session under rewake
rewake [--name N] [--main] codex [args...]    launch a Codex session under rewake
rewake list [--json]                    live sessions
rewake send <name> <text|-> [--question] [--wait S] [--json]
rewake inbox [--json]                   read the messages waiting for this session
rewake whoami [--json]                  this session's name and directory
rewake <command> --help
```

The tool's own flags only work before the harness name; everything after it
belongs to the harness.

### Output

- With no arguments — the overview, exit code 0: command groups, the workflow as
  real invocations, and behavior notes (exit codes, the refusal to ever prompt
  interactively, the Codex delay).
- One line per object, aligned columns, empty values are omitted.
  ```
  claude-2  claude  idle  /home/u/code/api  started 12m ago
  codex     codex         /home/u/code/web  started 3m ago
  ```
  Busy/idle state is shown only where the harness reports it itself: for Claude
  Code, the `status` field from `~/.claude/sessions/<pid>.json` (private format;
  a missing field means an empty column). Codex has no such signal without
  screen scraping.
- `send` prints the result as one line:
  ```
  delivered to claude-2 via socket
  delivered to codex via codex queue; codex checks its queue about every ten seconds
  pending for codex: the codex session has no conversation yet; delivers after its first turn
  ```
- `--json` on every command prints the full model; the text form is deliberately
  trimmed down. No colors, no TTY-dependent behavior.
- A single command table (name, arguments, flags, summary, examples, next) is
  the source for parsing, the overview, `--help`, and the hint shown on error. A
  test checks the table against the handlers and runs the examples.

### Failures and exit codes

| code | meaning |
|---|---|
| 0 | done; for `send` — delivered |
| 1 | the target failed or is unreachable: delivery `failed`, or the session died while waiting |
| 2 | invalid invocation: unknown command or flag, extra argument, no such session, name taken, unsafe directory |
| 3 | `send`: accepted but not yet delivered (`pending` at the end of `--wait`); the message will still deliver on its own |

An invocation failure shows: the reason, the syntax, an example, the flags, and
`full help: rewake <cmd> --help`. An unknown command gets the closest name ("did
you mean") if the edit distance is small. Never prompt interactively.

## Code

Go 1.25, no external dependencies (standard library and `syscall` only).

```
cmd/rewake/main.go            entry point, top-level parsing
internal/cli/                 command table, parsing, overview, help, failures, printing
internal/state/                directory: checks, paths, atomic writes
internal/registry/             session record, name publishing, liveness, listing
internal/proc/                  /proc: start time, process tree, fd links
internal/inbox/                 message, status, sender-side write, servicing loop
internal/harness/claude/        launch arguments, environment, socket delivery
internal/harness/codex/         launch arguments, thread lookup, delivery via codex queue
internal/wrap/                  wrapper: launch, signals, lifecycle
```

The harness adapter is an interface (`internal/harness/plan.go`):

```go
type Harness interface {
    ID() string
    Title() string
    Summary() string
    Examples() []string
    Notes() []string
    Launch(request LaunchRequest) (LaunchPlan, error)
    Deliver(ctx context.Context, session registry.Session, message inbox.Message) inbox.Result
}
```

`Deliver` sends the notice for `message`, built by `harness.Notice`; it never
sends `message.Text`.

## Testing

Unit tests (`go test`):
- name publishing: a race between two publishes of the same name — one wins; a
  dead record gets evicted; a live one doesn't;
- liveness: a reused pid with a different start time counts as dead;
- inbox: delivery order, `pending` retries, TTL, the status is written
  atomically;
- Codex thread lookup on a fixture of a `/proc`-like tree (the `/proc` root is
  parameterized);
- argument parsing and the command table: the table's examples parse cleanly.

Live tests, scripted under tmux, in a separate `/tmp` directory:
1. `rewake claude --model haiku` and `rewake codex`, `rewake list` sees both.
2. From a shell: `send claude "reply pong"` — a reply shows up on Claude's screen
   within seconds.
3. From the Codex sandbox (`codex sandbox -P :workspace -- rewake send ...`) —
   delivery to Claude through the inbox, without calling the model.
4. `send codex` before the first message — `pending`, exit code 3; after the
   first turn — the turn starts on its own.
5. The Claude agent runs `rewake send` without confirmation (checks
   `--allowedTools`).
6. Harness exit — the record and socket are gone, pending messages got
   `failed`.

Live Codex tests spend subscription quota: run them on a cheap model with short
messages.

## Distribution

- Build: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64|arm64 go build`, a binary a few
  MB in size.
- npm: `@iiiokojiadbi/rewake` with a shim, plus platform packages
  `@iiiokojiadbi/rewake-linux-x64`, `-linux-arm64` in `optionalDependencies` (the
  esbuild pattern). The scoped package is published with `--access public`.
- A non-npm alternative: `go install`.

## Owner decisions, September 16, 2026

1. The intro is injected at launch for both harnesses, on by default; turned off
   with `--no-intro`.
2. Permissions for the tool's own commands are granted for both harnesses, not
   just Claude Code.
3. The state directory in `/tmp` with 0700 permissions — this trust boundary is
   accepted.
4. A session without a wrapper is not supported; `register` is dropped from the
   first version.
5. A harness is told that mail is waiting, never handed the text; the agent reads
   it with `rewake inbox`. The notice pattern is `Rewake: <sender> <kind>, <n> new
   message(s)`, with the kinds `notify`, `question` and `finished`.
6. The intro is minimal — what rewake is, and to run `rewake guide` — and the
   instructions live in the guide. Nothing is added to a notice that the person
   watching the session would not see.
7. Whatever rewake passes to a harness adds to the user's settings and never
   replaces them. Where a key can only replace (`notify`), it is passed only when
   the user certainly has none.
