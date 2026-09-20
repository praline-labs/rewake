# Roadmap

Work for `docs/design.md` closes only after its acceptance criterion passes.
September 20: [native mailbox delivery accepted](native-mailbox-acceptance.md)
in fresh owner checks and installed sessions; permission/timing limits stay explicit.

Open [intermittent bugs](intermittent-bugs.md): the unexplained two-root-thread
refusal on September 17, 2026 cleared after recipient restart.

## How the work is run

- One step, one commit; commit message short, subject line only, in English.
- Before every commit: `gofumpt -l .` is empty; `go vet ./...`,
  `staticcheck ./...`, `golangci-lint run ./...`, and
  `go test -race -shuffle=on ./...` are green. No failing check is waived.
- Live runs happen in a separate `/tmp` directory, with their own `REWAKE_DIR`.
  The user's working harness sessions are left untouched.
- Live Codex runs spend subscription quota: cheap model, short messages, no
  more than one turn per check. Warn the owner before a run that spends quota.
- After milestones 3 and 5, a separate review agent passes over the diff:
  looking for defects and for checks that stay green on broken code. Findings get
  fixed before the next milestone.
- A milestone marked **live criterion** is accepted by running the real
  harnesses, not by tests.

## Milestone 1. Skeleton and interface — done, September 16, 2026

The CLI scaffold without delivery: command table, parsing, overview, help,
failures.

- `go.mod`, the package layout from design, `cmd/rewake`.
- `internal/cli`: the command table (name, arguments, flags, summary, examples,
  next, notes), argument parsing, the no-argument overview, per-command
  `--help`, failure formatting (reason, syntax, examples, `full help:`), "did
  you mean" by edit distance, exit codes 0/1/2/3.
- `internal/cli`: model printing — columns and `--json` from one function.
- Commands print stubs for now.

Tests: argument parsing; an unknown flag or an extra positional argument yields
exit code 2; the command table matches the handlers in both directions; the
table's examples parse without errors.

Acceptance: `rewake` with no arguments prints the overview; `rewake send` with
no arguments prints a failure with syntax and examples.

## Milestone 2. State and registry — done, September 16, 2026

- `internal/state`: the `$REWAKE_DIR` or `/tmp/rewake-<uid>` directory, owner
  and permission checks, creation with 0700, paths, atomic writes (tmp +
  rename).
- `internal/proc`: process start time from `/proc/<pid>/stat`, the process tree,
  `/proc/<pid>/fd` links. The `/proc` root is parameterized for tests.
- `internal/registry`: the session record, name publishing via `link()`,
  picking a free name, liveness by the pid-plus-start-time pair, evicting dead
  records, listing.
- `rewake list` and `rewake whoami` are for real now.

Tests: two concurrent publishes of the same name — one wins; a dead record gets
evicted, a live one doesn't; a reused pid with a different start time counts as
dead; an unsafe directory fails with exit code 2.

Acceptance: a record created by hand shows up in `list`; after killing the
process it disappears from `list`.

## Milestone 3. Claude Code: launch and delivery — done, September 16, 2026

- `internal/wrap`: launching the harness with an inherited terminal, signals
  (ignore `SIGINT`/`SIGQUIT`, forward `SIGTERM`/`SIGHUP`), stripping the parent
  session's markers, publishing and removing the record, the harness's exit
  code.
- `internal/inbox`: the message and status format, sender-side writes, the
  servicing loop with `pending` retries, TTL, moving to `done/`.
- `internal/harness/claude`: launch arguments (`--messaging-socket-path`),
  delivery as a line to the socket, the header `[rewake] from <name> · <id>`.
- `rewake send` end to end: waiting for the status, the result line, exit codes
  0/1/3.

Tests: delivery order by `id`; `pending` retries and becomes `failed` on TTL
expiry; the status is written atomically; delivery to a nonexistent socket gives
`pending`, and after the session dies, `failed`.

**Live criterion:** `rewake claude --model haiku` in one terminal, `rewake send
<name> "reply pong"` from another — a reply on screen within seconds; `list`
shows the session; harness exit removes the record and socket, and pending
messages get `failed`.

Met on September 16, 2026: delivery took 0.25 s and the agent answered within
three seconds, its reply landing in the sender's mailbox as a message of its own;
killing the terminal removed both the record and the socket. The run found two
defects that no test had:

- the agent was told to answer with `rewake send` and could not: the binary was
  not on its PATH. The wrapper now puts its own directory there;
- the first message header, `[rewake] from <name> · <id>`, was copied whole into
  the reply, which then addressed a session called `shell · 33f2`. The header now
  keeps the name on a line of its own and spells out the command to answer with.

## Milestone 4. Codex: launch and delivery — done, September 16, 2026

- `internal/harness/codex`: finding the current thread from open lock files,
  chosen by mtime; delivery via `codex queue`; parsing its errors (`no rollout
  found` → `pending`, `No active session` → `failed`); `CODEX_HOME` in the
  session record.
- The Codex wrapper: the intro via `-c developer_instructions`, concatenated
  with the user's value; the state directory added to `writable_roots` under
  `exclude_slash_tmp`.

Tests: thread lookup on a fixture of a process tree and a `/proc` directory;
picking the most recent lock; classifying `codex queue` errors by their text.

**Live criterion:** `rewake codex`, then `rewake send` before the first message
— exit code 3 and `pending`; after the session's first turn, the same message
gets delivered and Codex starts a turn on its own. Separately: `codex sandbox -P
:workspace -- rewake send …` delivers to Claude Code — verifying the inbox path,
without calling the model.

Met on September 16, 2026, in full: the message sent before the first turn came
back as pending with its reason, the thread id was read from the open lock file,
and after one turn of the session the waiting message was delivered on its own,
Codex started a turn and answered through `rewake send` from inside its sandbox.

The run found a defect no test had. A Codex agent runs its commands in a sandbox
with its own pid namespace, where every process but its own is missing — so
`rewake list`, run from there, judged every session dead and deleted the record
of the session that was running it. Records now carry the namespace their pids
belong to, and a reader in a different one neither reports a session gone nor
removes anything.

## Milestone 5. Intro and permissions — done, September 16, 2026

- The intro for both harnesses, plus the `--no-intro` flag.
- `--allowedTools "Bash(rewake:*)"` for Claude Code.
- Verify that the flag adds to the user's rules rather than replacing them; if
  it replaces them, drop the flag and have the overview name the rule to add
  once.

**Live criterion:** an agent launched through the wrapper, asked "who are you
in rewake", answers with its own name, and runs `rewake send` without
confirmation. For Codex, the same question in one cheap turn.

Met on September 16, 2026, as a side effect of the milestone 3 and 4 runs: both
agents answered through `rewake send` without being told how and without a
confirmation prompt, the Codex one from inside its sandbox.

## Milestone 6. Ready for daily use

- Cleaning up `done/` by age; clear failures at the edges (a session dying
  while a wait is in progress, a taken name, an unreachable directory).
- `README.md`: what it is, installation, the three commands, the trust
  boundary.
- Switching the inbox from polling to inotify, with polling as a fallback.
- Building for linux-amd64 and linux-arm64, publishing `@iiiokojiadbi/rewake`
  with platform packages; publish only on the owner's explicit word.

Acceptance: a week of use without manual intervention; not one case of a
message silently getting lost.

## Milestone 7. Notices instead of pasted text — done, September 16, 2026

The owner's call after living with milestone 6 for an hour: a message pasted
into a session looked like something the user typed, drowned the screen in a
block of text plus two lines of rewake hints plus Claude Code's own paragraph,
and gave the agent no sense that a tool was involved.

- A harness is told that mail is waiting — `rewake: api notify, 1 new message` —
  and the agent reads it with the new `rewake inbox`. Claude Code draws the
  notice as a single `● …` line because it is wrapped in `<task-notification>`;
  Codex gets the same line as a plain message.
- Messages have a kind: `notify`, `question` (`send --question`), `finished`.
- The end of a turn is reported to whoever wrote during it, with the last reply:
  a Stop hook passed in `--settings` for Claude Code, `-c notify` for Codex. A
  direct answer replaces that report, and reading an answer or a report asks
  for nothing back.
- The intro shrank to what rewake is and "run `rewake guide`"; the instructions
  moved into the guide.

**Live criterion, met:** two Claude Code sessions on Sonnet — one asked the other
a question on a shell's instruction, both read the guide on their own, the
question and the answer each arrived as one line, and the end of the asking
session's turn came back as `● rewake: web finished`. Then Claude Code and
Codex: a task sent to Codex arrived as a plain `rewake: api notify` line, Codex
read it with `rewake inbox`, answered in its final message only, and the notify
program turned that into `● rewake: cx finished` on the Claude side with `42`
in the inbox.

The run found two things no test had:

- an agent tried to answer a message from `shell` with `rewake send shell`; the
  old message text used to say that cannot work, and nothing said it any more.
  The guide says it now;
- an answer read by the asking session put the answering one on its waiting
  list, so the answering session woke up once more only to read that its answer
  had been read. Messages to a waiting session are now marked as replies.

The review round five findings (see below) are still open.

## Milestone 9. Roles — done, September 16, 2026

The owner, after restarting under the new build: the session handing out work
got a report of its own turn back at its worker, and the two would wake each
other forever. Roles now live in a catalogue, `internal/role`, and the one that
hands out work has role `main`: it gets every report and reports
nothing. Main now requires explicit `--main`; omitted flags and explicit
`--general` keep reporting behavior. The `write` role now reports like a worker
and is eligible for explicit main-authorized Git metadata access, independently
of reporting. The September 19 decision removed automatic role-based grants.

## Milestone 8. Three kinds of message — done, September 16, 2026

The owner's call after the first real rounds with Codex: a heads-up should not
wake anybody back, a question should block until it is answered, and work is
reported by ending the turn with the result rather than by another send.

- `task` (default): the reader owes a report; the report is its final message.
- `question` (`--question`): the same, and `send` blocks until the answer and
  prints it; the answer is not announced to the asking agent a second time.
- `notify` (`--notify`): owes nothing.
- The intro and the guide say how to answer: end the turn with the result.
- Each kind is its own file in `internal/cli`, listed in one table.

In Codex the notice keeps the 🟢: Codex strips control characters from a user
message, so a coloured `●` like Claude Code's is not possible there.

Earlier findings and fixes are in the [review history](reviews.md) and [its later part](reviews-later.md).

## Later, as needed


- pi, opencode, grok — their delivery paths are already covered in
  `docs/research.md`.
- A busy/idle signal for the recipient, and choosing delivery priority from it.
- A status column in `list` sourced from the Claude Code registry.
- macOS: replacements for `/proc` (`lsof`, `ps -o lstart`).
- Read receipts: `rewake inbox` already writes a `read` status; `send` does not
  report it yet.
- Permissions on request (owner idea, September 17, 2026): "grant permissions
  for actions on request — say review cannot reach a folder in /tmp", and
  "restrict the worker and grant it rights dynamically". The mechanism already
  exists for Git metadata: `turn/start.runtimeWorkspaceRoots` travels with a
  delivered task. A general form would be `rewake send <name> --grant <path>`,
  accepted only from the main role, with paths checked (existing, no symlink
  escape) and the thread's roots only ever extended; a general session would
  start with its working directory alone. Limits: Codex through its app-server
  only (Claude Code needs its own research), a turn the person starts in the
  TUI uses the stored roots, and the sandbox has to be workspace-write.

## Risks

| risk | mitigation |
|---|---|
| Claude Code's private socket protocol changes or gets disabled remotely | delivery is isolated in the adapter; on socket failure, a clear `failed`, not silence |
| owned-server thread identity is unavailable or ambiguous | fail explicitly; retain accepted reports when their notification fails; terminal ownership remains open |
| a fresh thread finishes before observer subscription | first input can start immediately; an observation gap reports an error without inventing a result |
| the wrapper died, the harness is alive | the session is treated as dead; delivery fails with a reason instead of going silent |
| identical text in a row gets dropped by the recipient | a short id in every message's header |

## Local installation

See [local installation without publishing](install.md) for source and package checks.

## Milestone 6 progress, September 16, 2026

- **The mailbox is watched, not polled.** A message arrives as a rename into the
  directory and the kernel says so, so delivery no longer waits for a tick. The
  poll stays at one second as the safety net: it retries pending messages and
  covers a watch the kernel would not give.
- **Finished messages are swept by age.** Delivered and refused ones, and their
  statuses, are kept a day and then let go; a message still waiting is answered
  by the TTL rather than by the sweep.

Two things the live run caught that the tests did not:

- the watch descriptor was closed by two goroutines, and once the number was
  reused that closed somebody else's file — the test framework's own directory,
  as it happened. It is now waited on rather than blindly read, and closed once;
- after the server's tick was slowed to a second, every delivery took exactly
  that long. The server was immediate; the *sender* was polling for its answer at
  the same slow interval. Measured on a live session: 0.03 s instead of 1.0 s.


## Milestone 10. Rooms — done, September 16, 2026

**Owner decision:** rooms isolate session discovery, addressing and delivery.
`--room <name>` is launch-only and defaults to `default`; agent commands inherit
`REWAKE_ROOM`. `REWAKE_DIR` remains the shared root; `rooms/<room>/` holds each room's
sessions, mailboxes and sockets. Old root-level records are ignored.

Role choice and name publication share a room lock. Without a role flag,
launches now always use general; the September 17 decision supersedes automatic
main selection. Explicit --main refuses with the live main's name when occupied.
General and write can start first. Main/write are eligible for explicit grants;
list, whoami, the launch note and intro identify the room and resolved role.

**Acceptance, verified:** two rooms with identical session names cannot see or
message each other; task notices and final reports stay in their originating
room. The original first-session election checks are historical and superseded.
Current checks require every unflagged launch to use general and concurrent
explicit --main claims to have one winner. They use isolated state and fake
harnesses, plus regression and mutation tests.


## Earlier transport and launch milestones

[September 17 history](transport-milestones-2026-09-17.md) preserves owned-server
acceptance, continuation permissions, lost reports, naming and explicit main.

## Gateway integration — verified paths, September 19, 2026

[Integration and review history](reviews-later.md#round-fourteen-and-gateway-integration--september-18-2026)
records the installed startup and primary/side acceptance, with its scope unchanged.

## Session state and service notices — accepted scope, September 19, 2026

State/activity passed scoped owner acceptance in `ab3a86c`; [the record](session-activity.md#evidence-and-acceptance)
keeps unexercised live cases explicit. The readiness/missing-notice incident stays open.

## Compact tables and grouped inbox — accepted, September 19, 2026

Revision 6 passed independent review and [installed live acceptance](inbox-acceptance.md).
Ready mail uses native start-or-steer without peek/terminal gating; every notice
keeps fixed members. Busy input was read between tool calls, the original report
survived, and a new idle task woke and returned its report. [The contract](inbox-groups.md)
retains independent receipts and explicit main-authorized grants; roles add none.
The P3 launch-role wording was corrected. Runtime matched the reviewed archive.

## Native mailbox output — accepted, September 20, 2026

Independent integration-1 review found no blocker; integration-2 closed its guard
coverage and documentation findings and proved both native directions by identity.
The 121 non-test runtime files stayed unchanged through the follow-up. The owner
accepted ordinary-briefing idle work and automatic report reading, plus a normal
sleep/notify scenario without visible user-message bubbles. After installation and
restart, the short task returned NATIVE-INSTALLED-OK through native output and inbox.
[Acceptance and limits](native-mailbox-acceptance.md) distinguish owner timing from
the deterministic standalone active control. The initial read-only failure required
explicit per-launch workspace-write in the fresh test directory, not automatic
permission expansion. Deliberately read-only inbox support is not established.
Both review rounds, synthetic/mutation coverage and final checks are part of this
stage. The socket/hook adapter retains its existing behavior.

## Native arrival UI — accepted, September 20, 2026

Two independently reviewed prototypes established downstream-only display and the
fixture correction needed for remote /resume. The owner observed idle/exec/stream
rows and later accepted the lifecycle check without a reported picker anomaly.
Transient display is explicitly sufficient; persistence/reconstruction/replay of UI
rows is deferred, while durable mail and report accounting remain mandatory.

The integration review reproduced the five checks and native synthetic cases with
no defect. The owner accepted the new `Ran rewake notice --display-only` label,
installed the reviewed binary and restarted main/write. UI-INSTALLED-OK returned as
an automatic native finished report and was read; the owner confirmed visible rows.
[The accepted contract](native-mailbox-ui.md) retains native Ran, deferred streaming
and independent delivery/read/report truth. [Review history](reviews-native-ui-2026-09-20.md)
keeps the initial permission-override refusal and limited owner timing evidence.
Final closeout changes only documentation after review; no new runtime behavior or
UI persistence mechanism is added. Other harness behavior is unchanged.

## Remaining work — owner decisions, September 19–21, 2026

The native-notification priority is complete, and so is the Claude Code handoff it
pointed to: orchestration moved to Claude Code on September 21, 2026. The first three
entries of the [parity queue](harness-features.md) closed the same day — HF-12, HF-07
with HF-20, and HF-21 — each on one observed run
([claude-parity-2026-09-21.md](claude-parity-2026-09-21.md)). Queue entries are not
milestones; the remaining ones are listed in the feature map.

The next focus is [check automation](check-runner.md): research observable workflows
before implementing a thin runner, with short console output and summary.json and
paid/manual tiers opt-in. Its research answer is
[check-runner-proposal.md](check-runner-proposal.md), September 21, 2026; no runner is
implemented.

Persistent Git permissions remain a separate future item: explicit orchestrator
event, scope to run and repository, preservation across later owner turns, and revocation. A running turn keeps its prior permission context.
Only validated Git metadata roots are in scope, not tools, credentials or arbitrary
folders. Do not implement that persistence in the current batch feature.
The main-readiness/missing-notice incident stays open separately.
