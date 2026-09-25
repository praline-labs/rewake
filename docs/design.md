# rewake: first-version design

The facts behind this design live in `docs/research.md` and its two companions,
`docs/research-launch.md` and `docs/research-protocol.md`, split by how a fact is
obtained. This document covers what we're building and how.

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
macOS (needs replacements for `/proc`); delivery receipts from the recipient (added
later for Claude Code's inbound gate, September 23, 2026 — [delivery-adapters.md](delivery-adapters.md#claude-code-adapter));
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
starts and stops both children, owns the inline RPC gateway and routes reports; the server
owns thread execution and sandboxing; the TUI keeps the inherited terminal.
An optional Backend in LaunchPlan encapsulates this lifecycle, delivery, thread
identity and completion events. An optional transport-neutral reservation holds
a destination across inbox readability and ACK; native RPC stays inside its adapter.
The socket adapter continues using its existing direct delivery and hook path, and
its wrapper also listens on a reply socket of its own beside the session's: Claude
Code reports there what its inbound gate did with a line, and only to the process that
wrote the line, so the wrapper — the one writer of its session's notices — is the one
listener ([delivery-adapters.md](delivery-adapters.md#claude-code-adapter)).

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
Created with 0700. [Optional primary observations](session-state.md) are collected asynchronously; only verified main callers see them.

```
<REWAKE_DIR>/
  rooms/<room>/
    .launch.lock              serializes role choice and name publication
    sessions/<name>.json       session record
    observations/<digest>.json latest bounded state for one name/epoch
    inbox/<name>/<id>.json     waiting for delivery
    inbox/<name>/<id>.status   pending, delivered, read or failed
    inbox/<name>/unread/       readable messages
    inbox/<name>/done/         read messages and archived delivery failures
    inbox/<name>/answering/<id> renewable question reservation
    inbox/<name>/received/<id> id of the report successfully printed
    inbox/<name>/retention/<id> reservation release time for reports
    inbox/<name>/turns/<id>    completion retry receipts
    inbox/<name>/threads/<id>  selected delivery thread, when supported
    inbox/<name>/awaiting/<epoch>/<peer> reports owed by this run
    inbox/<name>/pending/      the running turn's `rewake pending` mark
    sock/<name>.<epoch>.sock   one inbound socket per run
    sock/<name>.<epoch>.reply.sock the wrapper's own: Claude Code's receipts for held lines
    sock/<name>.<epoch>.obs    Claude Code telemetry datagrams to the wrapper
    sock/<name>.<epoch>.obs.turn/ when its latest turn started, one file per reading, for `rewake pending`
    sock/<name>.<epoch>.obs.plugin/ rewake's function-hooks plugin for this run (claude-plugin.md)
    control/<name>.<epoch>/    a main's compact or interrupt request and its answer (remote-control.md)
    letters/<name>/<id>.json   a compaction this main asked for, held by the command while it asks, kept on an answer that leaves the outcome open until its letter goes (remote-control-letter.md)
```

All mailbox paths in the delivery specification are relative to the room.
Socket names — the telemetry socket's too — fall back to a digest of name and epoch
when the expanded path would exceed 103 bytes (the reply socket, which has to share the
inbound socket's directory, to `rewake-<digest>.reply.sock` there); an excessively long state root still needs shortening.

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
  "name": "general-claude-2",
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
  "socket": "/tmp/rewake-1000/rooms/default/sock/general-claude-2.12345.1671399.sock"
}
```

- `serviceStart`, `harnessStart` — field 22 of `/proc/<pid>/stat` (start time in
  ticks). Liveness = the process exists and the start time matches: pids get
  reused.
- A session is alive as long as both the servicing process and the harness are
  alive. A listing or a lookup deletes a dead record it reads, but only when the
  record's name lock is free at that moment (`LOCK_NB`): both are reads and never wait
  on a lock — a lookup runs inside `turn-ended`, the foreground Stop hook, where a wait
  would stall the end of a turn. Whoever holds the lock is the run leaving or another
  reader cleaning up, so a record left now goes with the next read, and a dead record
  is reported as no session either way. Publishing still takes the lock and waits,
  replacing a dead record under it. Decided September 23, 2026 over
  a bounded wait, which would still make a read wait on something it cannot see.
- Publishing a record is atomic and exclusive: write a temp file, then `link()`
  it to the final name — `link` fails if the name is taken. If the existing
  record belongs to a dead session, it's removed and the attempt retried; for a
  live one, the name stays taken.
- Updating one's own record (for example, the harness pid after launch) uses a
  temp file and `rename()`.

### Roles and names

A session's role — what it is told, whether its turns are reported, whether it may take
a Git grant — and how its name is built from the role, the prefix and the harness are
in [roles.md](roles.md), with the owner decisions that set them.

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
An accepted report survives a failed notification in unread, with the failed
diagnostic and durable reportAvailable flag. Task/notify failures still archive.

The server backend tracks thread identity separately from its event subscription.
First delivery may precede persistence. Active status triggers bounded resume
attempts; idle without an observed completion produces an explicit observation
error after a grace period. See [ordering evidence](server-observation.md).
Observer cleanup follows established root changes, including fresh idle roots;
it does not resolve [terminal ownership ambiguity](thread-ownership-investigation.md).

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
rewake [--room R] [--name PREFIX] [--main|--general|--write] claude [args...]   launch a Claude Code session under rewake
rewake [--room R] [--name PREFIX] [--main|--general|--write] codex [args...]    launch a Codex session under rewake
rewake list [--json]                    live sessions in this room
rewake send <name> <text|-> [--question] [--wait S] [--json]
rewake inbox [--json]                   read the messages waiting for this session
rewake whoami [--json]                  this session's name, room, role and state root
rewake compact <name> [focus] [--json]  main only: compact an idle session (remote-control.md)
rewake interrupt <name> [--json]        main only: stop a session's running turn
rewake <command> --help
```

The tool's own flags only work before the harness name; everything after it
belongs to the harness.

### Output

- With no arguments — the overview, exit code 0: command groups, the workflow as
  real invocations, and behavior notes (exit codes, the refusal to ever prompt
  interactively, and delivery outcomes).
- The overview opens with the build, and `--version` prints that line alone:
  ```
  rewake 0.0.1 · build 36d6b90 · built 2026-09-25 11:58 UTC · modified
  ```
  The version stays the same between releases, so it cannot tell one build from
  another; the revision and the build time do. The revision comes from the VCS stamp Go
  records when it builds in a git checkout (`runtime/debug.ReadBuildInfo`), with
  `modified` when the working tree differed from it; a binary built from an archive, or
  run with `go run`, carries no stamp and says `build unknown`. Go records no build
  time, so the build command passes it in, `-ldflags "-X
  github.com/iiiokojiadbi/rewake/internal/cli.built=<RFC 3339 UTC>"` — `scripts/pack.sh`
  and the local build in [install.md](install.md) do. A binary built without it shows
  the revision's commit time instead, labeled `committed <time>`; with neither, no
  time. Under `--json` both carry the same fields as an object, `build` in the guide's
  model and the whole answer of `--version`: `version` and `known` always; the full
  `revision` only when known; `built` and `committed`, RFC 3339, each only when known;
  and `modified` only when set. The owner
  asked on September 25, 2026 for the version, the revision and the build date: the
  version alone left comparing file hashes as the only way to know which build a
  session runs.
- One line per object, aligned columns, empty values are omitted.
  ```
  general-claude-2  claude  12m  /workspace/api  room=default  (general)
  write-codex       codex   3m   /workspace/web  room=default  (write)
  ```
  List shows addresses, harnesses, ages, working directories, rooms and roles.
- `send` prints the result as one line:
  ```
  Rewake: delivered to general-claude-2 via socket
  Rewake: delivered to write-codex via app-server
  Rewake: failed for write-codex: delivery thread is unavailable
  ```
- A line rewake says itself in the ordinary flow starts with `Rewake:` and states the
  outcome — a notice, a delivery result, an inbox header, a note of its own. It does
  not explain the mechanism: that is in `--help` and the guide. The exceptions, named
  in the guide too: text from another session keeps its own header —
  `from <session> · <kind> · <time>`, `answer from <session>:` for a question,
  `from <session> · <id> · text no longer kept` when `--owed` has lost the text — and
  is printed as written; `--awaited` names each recipient as `to <session>` and each
  message as `<id> · <kind> · <time> · <state>` above its first line; main's state line reads `<session>: <activity> | context … |
  compactions …`; the availability and departure notices open with `Session
  available.` or `Session is no longer available…` and keep their identity block, by
  owner decision ([session-state.md](session-state.md#availability-notifications)).
  Refusals keep their full form. Every such line is listed in
  [the feed record](roadmap/2026-09-23-quiet-feed.md), and the lines of a view added
  since in that view's own entry — `--awaited`'s in
  [its record](roadmap/2026-09-23-awaited-view.md#the-lines-it-prints).
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

Go 1.25. Dependencies are allowed one deliberate decision at a time, judged on having
no transitive dependencies of their own and on being actively maintained; the standard
library is preferred where it does the job. The one in use is `pelletier/go-toml/v2`,
for the alias file. The source layout, package by package, and the harness
interface every adapter implements are in [code.md](code.md).

## Testing

How to run, read and extend the tests is in [testing.md](testing.md). Three layers,
cheapest first. Unit tests beside the code, run by the five checks in
`AGENTS.md`: name publishing races, liveness with reused pids, inbox order, retries and
expiry, the owned server's framing, correlation and reconnects on a fake socket, and a
parse of every example in the command table. The workflow suite in `test/workflow`,
switched on by `REWAKE_WORKFLOW=1`: a built rewake end to end against a fixture of each
harness, in both columns, with negative controls of two kinds: some build rewake with
one line mutated (batch-arrival's four, three of task-report's, mid-turn's
wait-for-idle), the others change the fixture's world (task-report's other four, the
readiness controls, mid-turn's late and failed-operation); what each runs and why is in
[check-runner-scenarios.md](check-runner-scenarios.md). Under the same switch, the
schema case checks the fixture's messages against the protocol schema of a real Codex,
the installed one or a version named by `REWAKE_CODEX_VERSION` and run in a container.
And live runs with real harnesses in a separate `/tmp` state directory, done by hand and
recorded in the dated acceptance documents; live Codex runs spend subscription quota,
so they use a cheap model and short messages.

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
7. Whatever rewake passes to a harness adds to the user's settings and never replaces them. A replacement-only briefing key is passed only when it cannot overwrite
   the user's instructions. Rewake never installs a notify program.

[Compact session tables and grouped inbox reading](inbox-groups.md) preserve message identities while sharing a bounded collection window and destination admission.
