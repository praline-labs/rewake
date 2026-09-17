# rewake: first-version design

The facts behind this design live in `docs/research.md`. This document covers what we're
building and how.

## Goal

`rewake` lets interactive coding-agent harnesses on one machine talk to each
other. A human launches a harness through the tool — `rewake claude`, `rewake
codex` — and gets an ordinary program in their terminal. The session registers
itself, and any other session, or a human from a shell, can write to it with
`rewake send <name> "text"`. The recipient sees a notice with a first-line preview when a message is
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

The server adapter has three processes: wrapper, app-server and TUI. The wrapper
starts and stops both children, owns the RPC client and routes reports; the server
owns thread execution and sandboxing; the TUI keeps the inherited terminal.
An optional Backend in LaunchPlan encapsulates this lifecycle, delivery, thread
identity and completion events. A new harness can implement it independently.
The socket adapter continues using its existing direct delivery and hook path.

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
<REWAKE_DIR>/
  rooms/<room>/
    .launch.lock              serializes role choice and name publication
    sessions/<name>.json       session record
    inbox/<name>/<id>.json     waiting for delivery
    inbox/<name>/<id>.status   pending, delivered, read or failed
    inbox/<name>/unread/       readable messages
    inbox/<name>/done/         read and failed messages
    inbox/<name>/answering/<id> renewable question reservation
    inbox/<name>/received/<id> id of the report successfully printed
    inbox/<name>/retention/<id> reservation release time for reports
    inbox/<name>/turns/<id>    completion retry receipts
    inbox/<name>/threads/<id>  selected delivery thread, when supported
    inbox/<name>/awaiting/<epoch>/<peer> reports owed by this run
    sock/<name>.<epoch>.sock   one inbound socket per run
```

All mailbox paths in the delivery specification are relative to the room.
Socket names fall back to a digest of name and epoch when the expanded path
would exceed 103 bytes; an excessively long state root still needs shortening.

### Rooms

**Owner decision, September 16, 2026:** `--room <name>` selects a room at launch.
Without the flag, a launch uses `default`, including when invoked from a session
in another room. Commands inherit `REWAKE_ROOM`; a shell without it uses
`default`. `REWAKE_DIR` always remains the shared root passed to children.

Room names use the session-name syntax. Names are unique within a room and can
repeat in different rooms. List, send, inbox, whoami, turn reports and delivery
open only that room's state tree below `rooms/`. There is no cross-room address and no `--room`
option on messaging or identity commands. List shows room and role on every
row; its JSON records include both. Whoami includes the room and verifies its
session epoch before describing a registered identity.

The `rooms/` namespace keeps even rooms named `inbox`, `sessions` or `sock`
separate from old root-level state. Old entries directly in the shared root are
ignored. Registry readers also reject JSON without a process pid and start time;
such files are neither treated as sessions nor pruned. There is no compatibility scan or fallback: the owner restarts those
sessions, and an old record must not appear in a new room by accident.

### Session record

```json
{
  "name": "claude-2",
  "room": "default",
  "role": "general",
  "roleReason": "selected explicitly with --general",
  "harness": "claude",
  "servicePid": 12345,
  "serviceStart": 1671399,
  "harnessPid": 12346,
  "harnessStart": 1671402,
  "cwd": "/home/u/code/x",
  "startedAt": "2026-09-16T00:08:03Z",
  "socket": "/tmp/rewake-1000/rooms/default/sock/claude-2.12345.1671399.sock"
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
the help line and whether its turns are reported come from that value.
System text lives in `internal/brief`, with a reviewed snapshot
for each role; harness adapters only pass the rendered strings. The record keeps the role's id.

| role | flag | turns reported | Git metadata writes requested for the sandbox | intro adds |
|---|---|---|---|---|
| `general` | `--general` | yes | no | end your turn with the result |
| `main` | `--main` | no | yes | you get reports, yours go to nobody |
| `write` | `--write` | yes | yes | end your turn with the result; you can commit |

Without an explicit role, a room with no live main elects the new session main;
otherwise it becomes general. Liveness uses the same pid/start-time and namespace
checks as list. Explicit `--main` refuses with the occupying session's name and
advice to stop/restart it or omit the role flag. `--general` and `--write` are
honored even in an empty room. If only non-main sessions remain, the next automatic launch
becomes main; existing sessions are never promoted in place.

The room's `.launch.lock` covers inspection of live sessions, role choice and
name publication. A starting wrapper is already a live claimant before its
harness starts. Concurrent launches cannot elect two mains. The lock is released
before preparing or running the harness. The record, launch note and intro say
which role was chosen and why.

Git writes are a separate role capability from reporting. The sandbox adapter
appends `--add-dir` for the discovered metadata directories. Ordinary repos,
worktrees and submodules are supported; configuration and existing roots remain
intact. Unresolved or symlinked metadata is skipped with a reason. See
[launch permissions](launch.md). A role does not revoke permissions the user
already granted; general receives no extra Git access from rewake.

The main session exists to stop a loop: it reads the reports of its workers,
and if its own turns were reported to them, each report would wake the other
side for good. A silent role records no waits and emits no successful turn reports. Failure
observation remains installed: StopFailure for the socket harness and terminal
server events for the owned-server harness. An error from main stays in its own unread mailbox
without waking the same failing conversation. Old records with role `worker` are read as `general`; only --general creates
new reporting sessions without Git access. The zero role value remains a reporting fallback inside the
catalogue; an omitted launch role is resolved separately under the room lock.
The write role reports like general; main stays silent whether its Git grant
was applied or skipped.

### Names

`[a-z0-9][a-z0-9._-]{0,31}`. The default is the harness name; if that's taken,
`claude-2`, `claude-3`. Explicit: `rewake --name api claude`. A session's name is
its address within its room, so a live name is never reused there.

### Environment the harness receives

- `REWAKE_SESSION=<name>` — who I am; `send` uses it to sign the sender.
- `REWAKE_EPOCH=<epoch>` — which run of that name I am. A process left over
  from an ended run keeps its environment; with this, it can neither read the
  next run's mail nor sign or report for it.
- `REWAKE_DIR=<root>` — the shared state root, not the room subdirectory.
- `REWAKE_ROOM=<room>` — the room for every child command. Inherited room,
  session and epoch markers are replaced with this launch's values.
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
rewake [--room R] [--name N] [--main|--general|--write] claude [args...]   launch a Claude Code session under rewake
rewake [--room R] [--name N] [--main|--general|--write] codex [args...]    launch a Codex session under rewake
rewake list [--json]                    live sessions in this room
rewake send <name> <text|-> [--question] [--wait S] [--json]
rewake inbox [--json]                   read the messages waiting for this session
rewake whoami [--json]                  this session's name, room, role and state root
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
  delivered to codex via app-server
  failed for codex: delivery thread is unavailable
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
internal/proc/                  /proc: identity, liveness and job-control state
internal/inbox/                 message, status, sender-side write, servicing loop
internal/harness/claude/        launch arguments, environment, socket delivery
internal/harness/codex/         owned app-server, WebSocket RPC, thread events and delivery
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
- owned-server process lifetime, framing, RPC correlation, thread selection,
  reconnects and terminal outcomes on a fake Unix-socket server;
- argument parsing and the command table: the table's examples parse cleanly.

Live tests, scripted under tmux, in a separate `/tmp` directory:
1. `rewake claude --model haiku` and `rewake codex`, `rewake list` sees both.
2. From a shell: `send claude "reply pong"` — a reply shows up on Claude's screen
   within seconds.
3. From the Codex sandbox (`codex sandbox -P :workspace -- rewake send ...`) —
   delivery to Claude through the inbox, without calling the model.
4. `send codex` during work steers that turn; a fresh /new thread accepts its
   first input without prior operator input. Interrupts report stopped.
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
   replaces them. A replacement-only briefing key is passed only when it cannot overwrite
   the user's instructions. Rewake never installs a notify program.
