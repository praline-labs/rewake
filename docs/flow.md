# The whole flow, end to end

What happens from the moment a person types `rewake claude` to the moment a
report lands back in the sender's mailbox — every process, every file and every
status on the way. The pieces are specified in [design.md](design.md),
[launch.md](launch.md) and [delivery.md](delivery.md); this page strings them
together so the process can be read in one sitting instead of being recovered
from the code.

## Cast

| who | what it is | lives |
|---|---|---|
| **wrapper** | the `rewake claude` / `rewake codex` process, the harness's parent | as long as its harness |
| **harness** | Claude Code or Codex, the ordinary program in the terminal | the session |
| **agent** | the model inside the harness, calling `rewake send`, `rewake inbox`, `rewake whoami` from its shell | a turn at a time |
| **sender** | any `rewake send` process: an agent's shell, a person's shell | until the status arrives |
| **hook** | `rewake turn-ended`, run by the harness at the end of every turn | a moment |

There is no daemon. Each wrapper services exactly one mailbox: its own
session's. Everything the processes share is files under the state directory,
`REWAKE_DIR`, 0700, checked on every open. Each room has its own subdirectory;
the paths below are relative to `<REWAKE_DIR>/rooms/<room>/`:

```
sessions/<name>.json             who is running: room, role, reason, pids, cwd, harness details
.launch.lock                    role selection and name publication
inbox/<name>/<id>.json           a message the wrapper still has to announce
inbox/<name>/<id>.status         pending | held | delivered | read | failed
inbox/<name>/unread/<id>.json    announced, not yet read by the agent
inbox/<name>/done/<id>.json      read or failed; swept after a day
inbox/<name>/awaiting/<epoch>/<peer>   who is owed a report by this run
inbox/<name>/answering/<id>      a send --question is waiting for this answer
inbox/<name>/received/<id>       id of the report printed for this question
inbox/<name>/retention/<id>      fixed release time for a reserved report
inbox/<name>/turns/<id>          completion retry receipt
inbox/<name>/threads/<id>        selected delivery thread, when supported
inbox/<name>/.lock               the mailbox lock, one flock for every state change
sock/<name>.<epoch>.sock         Claude Code's inbound socket for this run
sock/<name>.<epoch>.reply.sock   where that run's wrapper hears what the socket did with a line
```

## Act 1. A session starts

`rewake --write codex` starts write-codex. The sender in this example starts
with `rewake --main --name lead claude`, becoming lead-claude.

1. **The directory.** The wrapper opens the `REWAKE_DIR` root and the room
   selected by `--room`, or `default` when omitted. Both are 0700; a symlink,
   foreign owner or loose permissions is refused. Legacy root-level records
   are ignored.
2. **The name and role.** Under the room lock, first select the role (step 4),
   then append the harness ID to the role prefix or explicit `--name` prefix.
   Automatic collisions add -2, -3 after the harness suffix; explicit conflicts
   refuse with the full address. Prefix and final name must fit the name syntax
   and 32-character limit. Publication uses `link()`: live names remain taken,
   dead records can be replaced, and other rooms may use the same address.
3. **The run.** The wrapper's pid and start time make the **epoch**
   (`<pid>.<ticks>`). A name can be started many times; the epoch says which
   start this is.
4. **The role.** An unflagged launch always becomes general, even in an empty
   room or after main exits. Only explicit `--main` creates main, and it refuses
   under the room lock if a live main already occupies the room. Explicit
   `--general` and `--write` remain unchanged; a --name prefix never selects a
   role. Main stays silent; write reports like general. Main and write are eligible
   recipients of explicit Git metadata grants requested by main; selecting a role
   does not request access. The record and intro say default general for an omitted
   role flag, or identify the explicit flag.
5. **The harness command line.** The user's arguments go through untouched.
   rewake adds, for one launch only and never into a config file:
   - Claude Code: `--messaging-socket-path sock/<name>.<epoch>.sock`,
     `--append-system-prompt <intro>`, `--allowedTools "Bash(rewake:*)"`, and
     one `--settings` layer — merged into the user's own if they passed one — with
     StopFailure for every role and Stop for reporting roles, both running
     `rewake turn-ended`, plus background telemetry hooks and the status-line tap
     that report to the wrapper's `sock/<name>.<epoch>.obs`
     ([claude-telemetry.md](claude-telemetry.md)), and `--plugin-dir` with
     `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1` for rewake's plugin, which reports turn
     starts and ends — an interrupted one included — to the same socket
     ([claude-plugin.md](claude-plugin.md));
   - Codex: an owned foreground app-server on a private socket, initialized before
     the TUI starts with --remote. Explicit configuration and the briefing reach
     the server; an inline gateway follows accepted TUI intent. Launch roles add no
     Git roots, and no notify program is installed. Resume/fork keep caller input
     and omit generated permission flags; incompatible
     remote/profile/managed-worktree/local-provider launches refuse.
6. **The environment.** `REWAKE_SESSION=<name>`, `REWAKE_EPOCH=<epoch>`,
   `REWAKE_DIR=<root>` and `REWAKE_ROOM=<room>`; inherited Claude Code markers are stripped so a session
   started from inside another does not borrow its socket.
7. **Launch.** The harness starts with the wrapper's terminal and process
   group. Its pid and start time are added to the record. From now on the
   session is alive only while both processes are.
8. **The intro.** The agent's first context names its session, room, selected
   role and the reason for that role. A waiting message is announced with
   `Rewake:`, run `rewake guide` before sending or reading. Everything else the
   agent needs is in the guide, which always matches the binary.
9. **Serving.** The wrapper watches `inbox/<name>/` with inotify, polls every
   second as the safety net, sweeps old mail every ten minutes, and waits for
   the harness to exit.

`rewake list` shows only this room: name, harness, age, cwd, room and role
for every session. It reads the records, checks that both pids are alive with
matching start times, and deletes any record whose session is dead.

## Act 2. A task is sent

`rewake send write-codex "run the smoke and report what failed"` — from the main
session's shell, or from a person's shell in the same room. A shell without
`REWAKE_ROOM` uses `default`. Commands cannot name a different room.

1. **Who is sending.** `from` is `REWAKE_SESSION` when the run in
   `REWAKE_EPOCH` still holds that name, else `shell`. A leftover process of an
   ended run cannot sign as the new one.
2. **Who receives.** The recipient must be alive in this room; otherwise exit 2 with the live
   names listed.
3. **The kind.** Plain `send` is a `task`; `--question` and `--notify` are the
   other two. A question to a silent recipient is refused here, before anything
   is written: that role never reports, so it could never answer.
4. **The file.** `inbox/write-codex/<id>.json.tmp`, renamed to `.json`. The id is
   time-sortable. Nothing else is touched: the sender does not deliver.
5. **The wait.** The sender polls `<id>.status` for up to `--wait` seconds (5
   by default) and prints one line: `Rewake: delivered to write-codex via app-server`,
   `Rewake: pending for write-codex: …` or `Rewake: held for write-codex: …` (exit 3),
   or `Rewake: failed for write-codex: …` (exit 1).

## Act 3. The wrapper announces it

The recipient's wrapper announces fixed groups. Later mail waits for its own group
as soon as delivery is ready. Native start-or-steer reaches active work or wakes idle
work without a required peek or terminal event. Only new member IDs are delivered. The initial burst window is
150 ms. It checks expiry/leases and reserves destinations for the
[grouped inbox](inbox-groups.md) before readability.

1. **Readable first.** With the shared destination reserved, rechecks each message under
   the mailbox lock, records its delivery thread, and hard-links the message into `unread/`. An agent told
   about mail may run `rewake inbox` at once, so the text is there before the
   notice goes out.
2. **The notice.** A single member uses `Rewake: lead-claude task, 1 new message`
   and its first-line preview. A group uses `Rewake: <n> new messages` and one bounded
   indented preview of its latest member, including sender and kind. The count covers exactly this group, not older accepted
   unread mail or later arrivals. Messages arriving during destination acquisition join the group.
3. **The adapter**, outside the lock because it can take seconds:
   - Claude Code: connect to the session's socket and write one JSON line
     whose content is a `<task-notification>` block with that summary. The
     interface draws it as a single green `● Rewake: lead-claude task, 1 new
     message(s)` line — the same line its own background tasks get — and the
     model wakes if it was idle. The line names the wrapper's reply socket and
     an id, and the session's inbound gate answers there if it holds or refuses
     the line; silence for 300 ms counts as taken, and a word after that for a
     minute takes the delivery back. The first notice to a new
     session waits for its first status line, since until then the gate holds
     everything ([delivery-adapters.md](delivery-adapters.md#claude-code-adapter)).
   - Codex: call turn/start through the reserved TUI connection/generation with empty
     input and [standalone mailbox output](native-mailbox.md): short notice plus fixed
     member identities, never full task bodies. For tasks/questions to
     eligible main/write with explicit --grant-git intent, read current local roots without history and append only
     missing Git metadata from the thread's working repository. If roots cannot
     be read, omit the field and explain that in delivery status. No-flag tasks, general and
     report-only groups never get this grant. It starts idle work or steers the active turn;
     on steer, new roots apply only to subsequent turns. A successful RPC result means delivered. A stale
     or unavailable thread fails; it is not silently retargeted or queued. After ACK,
     a best-effort [display-only row](native-mailbox-ui.md) goes only to the owning
     primary TUI. It does not execute a command, add context or change delivery status;
     native streaming may defer its visible appearance.

4. **The status.** `delivered` (the waiting copy is removed, `unread/` keeps
   the message), `pending` (retried every two seconds), `held` (both copies stay
   until the harness says how the hold ended; not delivered, and not retried),
   or `failed`. A held task or question that ends failed sends its sender a note
   that it never arrived. Failed
   task/notify notices and expired mail move to `done/`. An accepted report
   whose notice fails stays in `unread/`, with `reportAvailable: true` in its
   failed status; the notice is not retried. Written atomically; the sender
   is reading it.

## Act 4. The agent reads

The notice wakes the agent, which runs `rewake inbox` in its shell. Main also gets
[availability notifications](session-state.md#availability-notifications) when peers
become ready; a later main learns which peers were already available. The same
main observer queues [compaction-complete and known-departure notices](session-activity.md).

1. **Whose mailbox.** `REWAKE_SESSION` names it, `REWAKE_EPOCH` proves the run:
   a stale run, or a process with a name and no epoch, is refused. Mail of an
   earlier run is not shown.
2. **Overview or full read.** `rewake inbox --peek` shows IDs and bounded first-line
   metadata without bodies, consumption, waiters or task-boundary changes; successful
   output has no dispatch side effect; new mail does not wait for an overview.
   `--message <id>` selects one available message; the flags are mutually exclusive.
   Plain inbox retains read-all. Under the lock, the selected available messages are
   printed, oldest first, with its sender, kind and time. Reports reserved by a
   waiting `send --question` are skipped (Act 6). A verified main caller first
   sees a [session-state header](session-state.md) and blank line for each message;
   workers see the original output. Stored agent text is unchanged.
3. **Then record.** Only once the output got through: for each `task` or
   `question` from a session, the sender's run is written into
   `awaiting/<own epoch>/<sender>` — this run now owes that run a report. A
   `finished` or a `notify`, or a message from a plain shell, records nothing.
   Then the `read` status, then the move to `done/`. A failure at any step
   leaves the message unread for the next `inbox`.
4. **The rule the agent follows**, from the guide: do the work, then end the
   turn with the result as the final message and stop. Do not answer with
   `rewake send`; do not answer a `notify` at all. After a context compaction,
   re-read the task with `rewake inbox --owed` rather than from the summary: it
   prints again, in full, every message whose `awaiting/` record is still there,
   and changes nothing. The sender's side of the same record: `rewake inbox
   --awaited` in the sending run lists, by recipient, what it sent and has no
   report on yet — unread, read and being worked on, pending, stopped — and what
   will get none because its recipient ended
   ([delivery-owed.md](delivery-owed.md#what-others-owe-you-rewake-inbox---awaited)).

## Act 5. The turn ends and the report goes back

The harness itself says when a turn is over: Claude Code through Stop or StopFailure,
Codex through turn/completed on its own TUI connection. The gateway retains
admitted outcomes across selection changes; no observer attachment is needed.
An asynchronous publisher journals callbacks before the durable report path. An observed active-to-idle interval missing
completion after a short grace produces error with "completion not observed",
without an assistant result, when its turn is known; when its turn was never named it
is a gap, reported as an advisory `stopped` that settles nothing, since it may have been
a compaction. Only a turn with proof that it is work — a reply naming it or an item
other than a compaction's — settles a wait; one without is reported as advisory, and a
turn its `contextCompaction` item shows to be a compaction reports nothing, so a
compaction's turn never settles a wait ([codex-publication.md](codex-publication.md)). The hook and server events use the same internal
reporting function and turn receipts. A callback with `agent_id` is from a nested agent and is
ignored without changing the parent session's waits.

1. **Who is owed.** Under the lock, `turn-ended` reads
   `awaiting/<own epoch>/`. A silent role emits no successful reports;
   failure callbacks still route errors. An identified turn persists its exact
   waiter/message snapshot and full report batch before publishing any report.
2. **The report.** For each waiting run, a `finished` message into that
   sender's mailbox: text is the final reply, `inReplyTo` lists the messages
   read from that run since the last report, `toEpoch` is that run — not
   whoever holds the name now. The id is derived from the wait, so a retried
   hook writes the same report once. Retries use the stored text, recipients
   and message ids even when new work arrived between attempts.
3. **Forget the reported messages** after all reports are written and the turn
   receipt is marked done. Cleanup matches the original run and wait; newly
   read messages remain owed to the next result.
4. **The sender is woken** by its own wrapper, through Act 3, with
   `Rewake: write-codex finished, 1 new message(s)`. The sender reads it with
   `rewake inbox`; a `finished` asks for nothing back, so the exchange ends
   here. The main session never reports successful turns, which is
   what keeps two sessions from waking each other forever.

If that wake-up fails, the report remains available to ordinary `rewake inbox`
in the addressed epoch. Its failed notification status keeps the diagnostic.
Reading it still owes no report; a failure is not converted into another task.

**Nobody sends a report.** The worker finishes its turn and stops; the wrapper turns
the end of that turn into the message above, addressed to whoever was owed one. An
agent that also writes `rewake send` with its result delivers the same text twice —
once by hand, once when the turn ends — and the sender has no way to tell the two
apart. This is easy to get wrong precisely because nothing refuses the extra send: it
is a valid message, it arrives, and the duplication only shows up on the far side.

A message from the worker to the sender is right in one case: when the worker needs an
answer before it can go on — a fork in the task, a question, a finding that changes
what was asked. Send it then, as `--notify` or as an ordinary message, not as a task.
A task creates an obligation to report, and the session most likely to be asked is the
main one, which is silent by role: the obligation would sit there with nothing to
discharge it.

A worker that ends a turn before the work is done — background work still running —
runs `rewake pending "<what it waits for>"` first. That one turn end then reaches the
waiters as a `pending` message with that text, owing nothing and settling nothing, and
the next turn end without a mark is the report
([turn-outcomes.md](turn-outcomes.md#interim-turn-ends-rewake-pending)).

## Act 6. A question, when the sender wants to block

`rewake send write-codex "which port?" --question` is a task whose sender waits for
the answer in the same command.

1. **Reserve before publishing.** The sender creates `answering/<id>` in its
   own mailbox and keeps touching it every second while it lives. Then it
   publishes the question exactly as in Act 2.
2. **The recipient does its part** exactly as in Acts 3–5; it cannot tell a
   question from a task.
3. **The report arrives** in the sender's mailbox. Its wrapper sees a fresh
   reservation for an id in `inReplyTo`, links the report into `unread/`, and
   leaves it there without a notice and without a final status, checking the
   reservation on every tick. Ordinary `inbox` skips it.
4. **The waiting send takes it.** Under the lock: print the answer, write
   `received/<id>` with the exact report id, remove the mark. The report moves to `done/` only once
   every question it answers has a receipt and no fresh reservation remains.
   Output failure exits 1 without a receipt, so the answer is kept.
5. **When the sender is gone** — timeout (exit 3), kill, or crash — the mark
   goes stale within three seconds and the wrapper announces the report the
   ordinary way. Nothing is lost, only blocked for a while.

## Act 7. A notify, when nothing is owed

`rewake send write-codex "the migration is merged" --notify` goes through Acts 2–4
and stops there: reading it records no wait, so no report comes back. It is the
right kind for a heads-up and for a probe.

## Act 8. The session ends

The harness exits, or Ctrl+C reaches the foreground group, or `kill` reaches the
wrapper (which then forwards it to a harness still alive a moment later).

1. The wrapper closes the inbox: waiting mail gets `failed: session ended`.
   Unexpired reports stay readable for their epoch. A reserved answer keeps
   its queue entry and lease semantics so its waiting command can consume it.
2. The record and the socket are removed; a later `list` will not show the
   name, and a `send` to it gets exit 2.
3. The wrapper exits with the harness's exit code.

A wrapper that dies without its harness leaves an agent that is alive and no
longer addressable; the next reader of the registry sees the dead record and
evicts it.

## One exchange, as files

`lead-claude` (main) gives `write-codex` a task and gets the result back.

| step | inbox/write-codex | inbox/lead-claude |
|---|---|---|
| send | `<id>.json`, `<id>.status: pending` | |
| wrapper announces | `unread/<id>.json`, status `delivered` | |
| write-codex reads | `awaiting/<epoch-w>/lead-claude`, status `read`, `done/<id>.json` | |
| write-codex's turn ends | `awaiting/` emptied | `<rid>.json` (finished, inReplyTo: id) |
| lead-claude's wrapper announces | | `unread/<rid>.json`, status `delivered` |
| lead-claude reads | | status `read`, `done/<rid>.json`; no wait recorded |

Exit codes the sender sees along the way: 0 delivered, 3 accepted and still
pending or held, 1 failed or unreachable, 2 wrong call — unknown session, bad flag, a
question to a silent role.

## Where the flow can stall, and what it says

| situation | what happens | what the sender sees |
|---|---|---|
| fresh server thread, no turn yet | turn/start begins its first turn | delivered after RPC acceptance |
| a turn interrupted with Esc or Ctrl+C | stopped advises the waiters to wait, and goes to nobody when none waits; original work stays owed | yellow notice; human continuation reports its result |
| a turn interrupted by main's `rewake interrupt` | the same stopped, naming main instead of the person; on Claude Code the worker's next notice says main interrupted it, once, and on Codex the harness records it in the model's history itself ([remote-control.md](remote-control.md)) | yellow notice naming main |
| recipient's wrapper gone | record evicted on the next read | exit 2, no such session |
| a reader's stdout blocks | it holds the lock; server waits, `turn-ended` five seconds, `inbox` ten | delays, then "mailbox is busy" |
| the main session is asked a question | refused before publication | exit 2 with a hint |
| a process of an ended run | its epoch no longer holds the name | `inbox` and `send` refuse |
| Claude Code's socket not yet up | `ENOENT` while the harness lives | `pending`, then delivered |
| Claude Code up, status line not drawn yet | the first notice waits, three seconds at most | `pending`, then delivered |
| Claude Code's gate holds the line | both copies stay, no retry | `held` (exit 3), then delivered, or failed and a note |
| Claude Code's gate says it holds only after the 300 ms | the delivery is taken back, for up to a minute | `delivered` (exit 0), then `held`; if it fails, a note |
| the wrapper is killed while its session holds a line | the next session with the name fails it | `failed`, the session ended, if send still waits; a note once the name is taken again |

## Reports across conversation changes

A delivery thread is recorded before an owed task becomes readable, then used
for the actual RPC target. This covers readers that fetch mail before the
notice call returns. The context stays alongside the task across read/status
updates and remains while a report is owed.

A `/new` in Codex, or a `/clear` in Claude Code, changes the conversation without
changing the wrapper epoch. At turn
end, the hook compares the current thread with the delivery thread of every
message in `inReplyTo`. Any known mismatch adds `threadChanged: true` to the
report. Inbox prints a warning beneath the result; a waiting question preserves
it too. No message is automatically resent and no wait is discarded because of
the change. Missing context cannot prove a change; harnesses without thread
tracking omit the field.

A failed turn uses error instead of finished and preserves the harness reason.
All live waiters receive it; with none, the room's main receives it. Main's own
error stays in its inbox without a self-notice. Reading error owes no reply.
Empty received completion after read work is a textless error. A waiting
question returns the failure directly. Callback availability is a harness
boundary; rewake never infers a cause from missing callbacks or reads rollouts.

Notifications show the latest available letter's first line beneath the header,
prefixed by an indented ↳. They never substitute the preview for inbox content.
Empty first lines stay empty; long ones are clipped with an ellipsis to about
100 columns. Errors use failed/red status; keyboard stops use killed/yellow. Active question reservations
are excluded from ordinary counts and previews.
