---
description: The system as built — scope, processes, the state directory and the session record, rooms, the environment a run gets, the command-line contract, distribution, and the decisions in force.
---

# rewake: the design

What rewake is built as and the decisions that hold it. The facts behind it live in
[research.md](research.md) and its companions [research-launch.md](research-launch.md) and
[research-claude-mod.md](research-claude-mod.md), split by how a fact is obtained.

## Goal

`rewake` lets interactive coding-agent harnesses on one machine talk to each other. A person
launches a harness through the tool — `rewake claude` or `rewake pi` — and gets an ordinary
program in their terminal, inside rewake's shell. The session registers itself, and any other
session, or a person from a shell, can write to it with `rewake send <name> "text"`. The
recipient sees a notice with a first-line preview when a message is waiting —
`Rewake: api notify, 1 new message` — which wakes it if it is idle, and fetches the text itself
with `rewake inbox`.

The primary user of these commands is an agent calling them from its own shell. That's where
the requirements on output and failure messages come from (see "Interface").

## Scope

In scope: Claude Code and pi; Linux; the command line, `--json` on every command; the shell,
which main runs in at a terminal ([shell.md](shell.md)).

Out of scope: Codex, which does not return soon; opencode, grok; macOS (needs replacements for
`/proc`); delivery receipts from the recipient beyond what Claude Code's inbound gate reports
([delivery-adapters.md](delivery-adapters.md#claude-code)); message history.

Never doing: typing text into someone else's screen; editing the user's harness configs; reading
transcript contents. rewake holds a pseudo-terminal only where it draws the program: main's pane
in the shell, and each worker main starts in a space.

## Design

### Processes

**Wrapper** — the `rewake claude` or `rewake pi` process. It stays the harness's parent for its
whole life: launches it with an inherited terminal, registers the session, services its inbox,
and on exit deregisters the session and returns the harness's exit code. There's no daemon: each
wrapper is the delivery point for its own session only.

A launch plan carries one run's capabilities as separate interfaces — a Service for this
lifecycle, Wake for delivery, TurnBoundary for its turn ends, Telemetry, Control and Spaces — and
what an adapter offers before any launch is its `Offers()` value
([rules/adapter-api.md](rules/adapter-api.md)). The Claude Code adapter delivers straight to the
session's socket, reports its turns through its mod ([claude-turns.md](claude-turns.md)), and
its wrapper also listens on a reply socket of its own beside the session's: Claude Code reports
there what its inbound gate did with a line, and only to the process that wrote the line, so the
wrapper — the one writer of its session's notices — is the one listener
([delivery-adapters.md](delivery-adapters.md#claude-code)). pi's extension speaks one socket both
ways ([research-pi.md](research-pi.md)).

**Sender** — any process that calls `rewake send`. It doesn't deliver anything itself: it drops a
file into the recipient's inbox and waits for a status. This way sending works the same from a
plain shell and from an agent's shell.

**Space host** — a process `rewake space open` starts outside main's process tree, which runs a
worker's launch in a pseudo-terminal, serves it to the shell and ends it with main's run
([space.md](space.md)).

There's no separate daemon: nothing to start, stop, or recover, and there's no state where "the
sessions exist but the daemon is dead". Exactly as many wrapper processes run as there are open
sessions; a session without a wrapper doesn't take part in messaging.

### State directory

`$REWAKE_DIR`, defaulting to `rewake-<uid>` in the system's temporary directory. The wrapper
passes `REWAKE_DIR` to children explicitly, so a child with a different `TMPDIR` still finds the
same directory.

Checked on every open: it's a directory, not a symlink, owned by the current uid, with no group
or other access; otherwise it fails with an explanation. Created with 0700, by a launch only:
resolving the directory creates nothing, and the base, the root `v2/` and the room are made by
the bootstrap of the room's build lease ([rules/state.md](rules/state.md#the-bootstrap)), so a
command that only looks leaves no state behind. 1.x's tree beside `v2/` is neither read nor
written. [Optional primary observations](session-state.md) are collected asynchronously; only
verified main callers see them.

```
<REWAKE_DIR>/                   the base
  v2/rooms/<room>/              2.0's root, then the room
    build.lock                 the room's build lease, flock(2): writers shared, a launch changing the build exclusive
    build                      the room's build id, the SHA-256 of the executable a launch established
    directory                  the launch directory the room is named for
    .launch.lock               serializes role choice and name publication
    sessions/<name>.json       session record
    observations/<digest>.json latest bounded state for one name/epoch
    inbox/<name>/<id>.json     waiting for delivery
    inbox/<name>/<id>.status   pending, delivered, read or failed
    inbox/<name>/unread/       readable messages
    inbox/<name>/done/         read messages and archived delivery failures
    inbox/<name>/answering/<id> renewable question reservation
    inbox/<name>/received/<id> id of the report successfully printed
    inbox/<name>/retention/<id> reservation release time for reports
    inbox/<name>/journal/<id>.done the record a turn end writes last, named by the end (delivery-turn-end.md#the-end-of-a-turn)
    inbox/<name>/threads/<id>  selected delivery thread, when an adapter follows one
    inbox/<name>/awaiting/<epoch>/<peer> reports owed by this run
    inbox/<name>/pending/      this run's `rewake pending` marks; its last word on the work and a held answer (turn-outcomes.md)
    inbox/<name>/receipts/<epoch>/ the records of a notify's and `rewake pending`'s retries (rules/effects.md#receipts-and-once-marks)
    inbox/<name>/once/<epoch>/<id> a notify's heads-up, so a retry writes it once
    sock/<name>.<epoch>.sock   one inbound socket per run
    sock/<name>.<epoch>.reply.sock the wrapper's own: Claude Code's receipts for held lines
    sock/space.<id>.sock       a space host's: the worker's raw stream for the shell (space.md)
    control/<name>.<epoch>/    a main's compact or interrupt request and its answer (remote-control.md)
    run/<name>.<epoch>/        what an adapter writes for one run, removed with the run or, after a kill, by a later launch once the run is proven dead (rules/launch.md, L3)
    letters/<name>/<id>.json   a compaction this main asked for, held by the command while it asks, kept on an answer that leaves the outcome open until its letter goes (remote-control.md#the-letter)
    spaces/<id>.json           a space a main opened: its worker, checkout, space host, state and observed end, kept after it closes (space.md#the-record)
```

All mailbox paths in the delivery specification are relative to the room. Socket names fall back
to a digest of name and epoch when the expanded path would exceed 103 bytes (the reply socket,
which has to share the inbound socket's directory, to `rewake-<digest>.reply.sock` there); an
excessively long state root still needs shortening. Since an epoch carries the machine's boot id,
36 characters, the digest is the usual form under an ordinary state root, and the expanded one the
exception.

One build holds a room at a time ([rules/state.md](rules/state.md#the-builds-identity)): an npm
upgrade under running sessions meets a new binary at the install path that refuses to change the
room until its sessions end, instead of writing records the running wrappers' build does not
expect.

One thing rewake keeps outside the base: the checkouts it makes for a launch with `--worktree`,
and their records. A checkout holds work that must outlive a restart, so they live in the user's
data directory, `~/.local/share/rewake/worktrees` by default ([worktree.md](worktree.md)).

### Rooms

The room is the directory rewake is started in, resolved with symlinks followed, and the person
never names one. Its name on disk is `<slug>-<hash>` — the directory's last element reduced to
`[a-z0-9-]` and cut to 19 characters, then 12 hex digits of the SHA-256 of the resolved path — and
the room's file `directory` keeps the resolved path for `rewake list`
([launch.md](launch.md#main-in-the-room-of-its-directory)). One main per directory, so a room is
one main with its workers; the room stays a name of its own in the core, for communication between
two mains later. Workers main raises are in main's room whatever their checkout. Commands inherit
`REWAKE_ROOM`; a shell without it takes the room of its working directory. A room `default` of
older builds is left as it is, unmigrated. `REWAKE_DIR` always remains the base passed to
children, never the root or the room below it.

Room names use the session-name syntax. Names are unique within a room and can repeat in
different rooms. List, send, inbox, whoami, turn reports and delivery open only that room's state
tree below `v2/rooms/`. There is no cross-room address and no `--room` option on messaging or
identity commands. List shows the room above its table and each session's role; its JSON records
carry both. Whoami includes the room and verifies its session epoch before describing a
registered identity.

The `v2/rooms/` namespace keeps even rooms named `inbox`, `sessions` or `sock` separate from old
state. The inverse from a room to its root is checked and refuses a 1.x room
([rules/state.md](rules/state.md#the-roots)). Registry readers also reject JSON without a process
pid and start time; such files are neither treated as sessions nor pruned. There is no
compatibility scan or fallback: an old record must not appear in a new room by accident.

### Session record

`sessions/<name>.json`:

```json
{
  "name": "general-claude-2",
  "room": "x-4b7a42d73500",
  "role": "general",
  "roleReason": "raised by main with rewake space open as general",
  "harness": "claude",
  "servicePid": 12345,
  "serviceStart": 1671399,
  "harnessPid": 12346,
  "harnessStart": 1671402,
  "boot": "0f0e0d0c-0b0a-4908-8706-050403020100",
  "cwd": "/home/u/code/x",
  "startedAt": "2026-09-16T00:08:03Z",
  "messagingReadyAt": "2026-09-16T00:08:05Z",
  "socket": "/tmp/rewake-1000/v2/rooms/x-4b7a42d73500/sock/3f5a9c0e7d21b84c6a0f93e2.sock",
  "ownsSocket": true,
  "pidNamespace": "pid:[4026531836]"
}
```

- `serviceStart`, `harnessStart` — field 22 of `/proc/<pid>/stat` (start time in ticks).
  Liveness = the process exists and the start time matches: pids get reused.
- `boot` — the machine's boot id, part of the epoch `<pid>.<ticks>.<boot>`: a pid and its start
  recur after a restart of the machine, the boot id does not.
- `messagingReadyAt` — when the session first became ready to take messages; it marks that the
  start succeeded, not that delivery works now. Absent until then.
- `ownsSocket` — this session created the socket path, so it removes it when it ends; absent for
  a harness without a socket, and for one whose socket path the caller named.
- `cwd` — the directory rewake was launched from, taken when the name is claimed. A harness that
  moves itself afterwards is not followed: Claude Code launched with `-w <name>` runs in
  `.claude/worktrees/<name>`, while the record, `rewake list` and the availability notices show
  the launch directory ([research-launch.md](research-launch.md#a-worktree-at-launch)).
- `pidNamespace` — the pid namespace the two pids belong to: a reader in another one cannot judge
  whether they are alive, and does not try.

A session is alive as long as both the servicing process and the harness are alive. A view never
deletes a record: it reports a dead record as no session and leaves it
([rules/state.md](rules/state.md#what-is-a-writer)). A dead record is pruned only in a change
form, under its room's lease: a lookup or a listing there deletes one it reads when the record's
name lock is free at that moment (`LOCK_NB`), and never waits on the lock, since a lookup runs
inside a turn's end, where a wait would stall it. Whoever holds the lock is the run leaving or
another change form cleaning up, so a record left now goes with the next one. Publishing takes
the lock and waits, replacing a dead record under it.

Publishing a record is atomic and exclusive: write a temp file, then `link()` it to the final
name — `link` fails if the name is taken. If the existing record belongs to a dead session, it's
removed and the attempt retried; for a live one, the name stays taken. Updating one's own record
(for example, the harness pid after launch) uses a temp file and `rename()`.

### Roles and names

A session's role — what it is told, whether its turns are reported — and how its name is built
from the role and the harness are in [roles.md](roles.md).

### Environment the harness receives

- `REWAKE_SESSION=<name>` — who I am; `send` uses it to sign the sender.
- `REWAKE_EPOCH=<epoch>` — which run of that name I am. A process left over from an ended run
  keeps its environment; with this, it can neither read the next run's mail nor sign or report
  for it.
- `REWAKE_DIR=<base>` — the shared base, not its root `v2/` nor the room below it.
- `REWAKE_ROOM=<room>` — the room for every child command. Inherited room, session and epoch
  markers are replaced with this launch's values.
- `REWAKE_EXECUTABLE=<path>` — the rewake that started the run, over an inherited value: the
  harness's `worktree_status` tool runs it by argv rather than whichever rewake PATH finds first
  ([worktree-tools.md](worktree-tools.md)). A probe gets none.
- `REWAKE_ROLE=<id>` — the run's role, over an inherited value: main's `worktree_list` and
  `worktree_land` are registered only where it is `main`; a choice of what is shown, not an
  authorization. A probe gets none.
- `REWAKE_WORKTREE_ENTER=1` — set by the adapter only for a worker whose template says
  `enter_worktree = true`, once its run can enter a checkout; never inherited, none for a probe.
  The mod registers `worktree_enter` and guards `EnterWorktree` by it, and
  `rewake worktree enter` refuses without it; not a boundary
  ([worktree-tools.md](worktree-tools.md#a-workers-entry-enter_worktree--true-and-worktree_enter)).
- Inherited Claude Code markers are stripped (list in research): otherwise `rewake claude`
  launched from inside another session would inherit that session's socket and have transcript
  saving disabled.

None of the variable names contain `KEY`, `SECRET`, or `TOKEN`: a harness may strip those from the
agent's command environment.

## Launch and signals

See [launching a harness and handling signals](launch.md).

## Delivery

See the [delivery protocol](delivery.md) for sending, reading, reports and mailbox locking. A
question reserves its answer before publication. A fresh reservation keeps the report queued and
excludes it from ordinary inbox reads; only successful output records receipt. Every question
sharing a report must receive it before it is archived. Missing or stale reservations return
unread reports to ordinary notification. Silent roles refuse questions before sending. An accepted
report survives a failed notification in unread, with the failed diagnostic and durable
reportAvailable flag. Task/notify failures still archive.

## A session without a wrapper

Not supported. Only what's launched through `rewake` takes part. A plain `claude` in another
terminal doesn't show up in `list` and doesn't receive messages — it has to be restarted through
the wrapper.

## Interface

The CLI is the interface: every mail operation is a command, and a session works through its shell
and the guide; there are no mail tools beside the command line.

### Commands

The command list is not repeated here: it is `internal/cli/registry.go`, and the tool prints it
itself — `rewake` or `rewake guide` for the overview with every command, `rewake <command> --help`
for one command's syntax, flags and examples. A copy here would teach a stale call with the same
confidence.

The tool's own flags only work before the harness name; everything after it belongs to the
harness.

### Output

- With no arguments — the overview, exit code 0: command groups, the workflow as real
  invocations, and behavior notes (exit codes, the refusal to ever prompt interactively, and
  delivery outcomes).
- The overview opens with the build, and `--version` prints that line alone:
  ```
  rewake 0.0.1 · build 36d6b90 · built 2026-09-25 11:58 UTC · modified
  ```
  The version stays the same between releases, so it cannot tell one build from another; the
  revision and the build time do. The revision comes from the VCS stamp Go records when it builds
  in a git checkout (`runtime/debug.ReadBuildInfo`), with `modified` when the working tree
  differed from it; a binary built from an archive, or run with `go run`, carries no stamp and
  says `build unknown`. Go records no build time, so the build command passes it in, `-ldflags
  "-X github.com/praline-labs/rewake/internal/cli.built=<RFC 3339 UTC>"` — `scripts/pack.sh` and
  the local build in [install.md](install.md) do. A binary built without it shows the revision's
  commit time instead, labeled `committed <time>`; with neither, no time. Under `--json` both
  carry the same fields as an object, `build` in the guide's model and the whole answer of
  `--version`: `version` and `known` always; the full `revision` only when known; `built` and
  `committed`, RFC 3339, each only when known; `modified` only when set; and the build id.
- `rewake list` prints a table under `Room: <room> (<directory>)`, with the directory and harness
  above it when every session shares them, and a row per session
  ([session-state.md](session-state.md#the-session-table)).
- `send` prints the result as one line:
  ```
  Rewake: delivered to general-claude-2 via socket
  Rewake: failed for write-claude: delivery thread is unavailable
  ```
- A line rewake says itself in the ordinary flow starts with `Rewake:` and states the outcome — a
  notice, a delivery result, an inbox header, a note of its own. It does not explain the
  mechanism: that is in `--help` and the guide. The exceptions, named in the guide too: text from
  another session keeps its own header — `from <session> · <kind> · <time>`,
  `answer from <session>:` for a question, `from <session> · <id> · text no longer kept` when
  `--owed` has lost the text — and is printed as written; `--awaited` names each recipient as
  `to <session>` and each message as `<id> · <kind> · <time> · <state>` above its first line;
  main's state line reads `<session>: <activity> | context … | compactions …`, above a letter
  from a session, never one from a person's shell; the availability and departure notices open
  with `Session available.` or `Session is no longer available…` and keep their identity block
  ([session-state.md](session-state.md#availability-notices)). Refusals keep their full form.
  The lines themselves are in the code and in `--help`.
- `--json` on every command prints the full model; the text form is deliberately trimmed down. No
  colors, no TTY-dependent behavior.
- A single command table (name, arguments, flags, summary, examples, next) is the source for
  parsing, the overview, `--help`, and the hint shown on error. A test checks the table against
  the handlers and runs the examples.

### Failures and exit codes

| code | meaning |
|---|---|
| 0 | done; for `send` — delivered |
| 1 | the target failed or is unreachable: delivery `failed`, or the session died while waiting |
| 2 | invalid invocation: unknown command or flag, extra argument, no such session, name taken, unsafe directory |
| 3 | `send`: accepted but not yet delivered (`pending` at the end of `--wait`, or at once when the recipient's Wake is not live); the message will still deliver on its own |

An invocation failure shows: the reason, the syntax, an example, the flags, and
`full help: rewake <cmd> --help`. An unknown command gets the closest name ("did you mean") if the
edit distance is small. Never prompt interactively.

## Code

Go for the core and the host side of every adapter; TypeScript only where a harness requires it —
the Claude Code mod, the pi extension — and in the shell. A part inside a harness is small,
embedded by the binary for one run and versioned with it; the protocol between it and the host is
described once, as a JSON schema, and the types are generated from it. Dependencies are allowed
one deliberate decision at a time, judged on having no transitive dependencies of their own and
on being actively maintained; the standard library is preferred where it does the job. In use:
`pelletier/go-toml/v2`, for the alias file, and the terminal emulator `charmbracelet/x/vt` with
`x/ansi` and `ultraviolet`, an exception confined to `internal/host/terminal`
([shell.md](shell.md#the-space-hosts-emulator)). The source layout, package by package, is in
[code.md](code.md), the import rule in [rules/layout.md](rules/layout.md).

## Testing

How to run, read and extend the tests is in [testing.md](testing.md); the commands and which tests
to write are in `AGENTS.md`, "Checks". Three layers, cheapest first: unit tests beside the code;
the workflow suite in `test/workflow`, switched on by `REWAKE_WORKFLOW=1`, a built rewake end to
end against the fixture harness; and live runs with real harnesses in a separate state directory,
done by hand.

## Distribution

- Build: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64|arm64 go build`, a binary a few MB in size.
- npm: one package, `@praline-labs/rewake`, published with `--access public`. Each platform build
  is a version of the same package, `<version>-linux-x64` and `<version>-linux-arm64`, under a
  dist-tag of its own (`linux-x64`, `linux-arm64`) so it never takes `latest`, and carries the
  `os` and `cpu` npm filters. The entry, `<version>` under `latest`, holds the POSIX shim and
  names the builds in `optionalDependencies` through npm aliases,
  `"@praline-labs/rewake-linux-x64": "npm:@praline-labs/rewake@<version>-linux-x64"`; npm installs
  only the build that matches the machine, under the alias, where the shim finds it. The aliases
  are scoped so nothing from outside the organization can sit where the shim looks.
- pi ships with rewake: the entry names `@earendil-works/pi-coding-agent` in `dependencies` at the
  exact version the binary pins, so npm installs and updates it with rewake, and a launch starts
  that copy with `node` ([launch.md](launch.md#pi)). Claude Code is not shipped; its license leaves
  it to the person.
- The shell ships in each platform build as `bin/rewake-shell`, compiled by Bun with its runtime
  inside, about 99 MB; rewake finds it beside its own executable
  ([shell.md](shell.md#how-rewake-starts-the-shell-and-how-it-ships)).
- A non-npm alternative: `go install`, with pi installed and the shell compiled beside the binary
  by hand ([install.md](install.md#trying-20-beside-103)).
- The repository lives in the GitHub organization `praline-labs`, the module path is
  `github.com/praline-labs/rewake` and the npm scope `@praline-labs` (the unscoped name `rewake`
  belongs to someone else); the license is MIT, and every package carries `LICENSE`.

## Decisions in force

1. The briefing is injected at launch for every harness, on by default, turned off with
   `--no-intro`. It is the role's playbook ([roles.md](roles.md)); the full contract lives in the
   guide.
2. Permissions for the tool's own commands are granted for every harness.
3. The state directory in `/tmp` with 0700 permissions — this trust boundary is accepted.
4. A session without a wrapper is not supported.
5. A harness is told that mail is waiting, never handed the text; the agent reads it with
   `rewake inbox`. The notice pattern is `Rewake: <sender> <kind>, <n> new message(s)`. Nothing is
   added to a notice that the person watching the session would not see.
6. Whatever rewake passes to a harness adds to the user's settings and never replaces them. A
   replacement-only briefing key is passed only when it cannot overwrite the user's instructions.
7. A session whose mod did not load has no live TurnBoundary: a role that must report refuses to
   launch, and a task or question to such a session exits 1, naming why.
8. A harness version that cannot be read refuses the launch, saying how it was asked: a mod
   against an unknown API would fail silently ([rules/adapter-api.md](rules/adapter-api.md#minimum-versions)).
9. Workers close with their main: no worker outlives the main that raised it. What brings them
   back is the record: each worker's session id is kept in its space's record beside its harness,
   checkout, role and name, for a restore ([space.md](space.md#restore)).
10. The CLI keeps its promises to a caller: one command table, no arguments prints the guide,
    `--json` on every command, an unknown flag is a refusal, exit codes 0/1/2/3.

[Compact session tables](session-state.md#the-session-table) and [grouped inbox reading](delivery.md#reading-rewake-inbox) preserve message identities while sharing a bounded collection window and destination admission.
