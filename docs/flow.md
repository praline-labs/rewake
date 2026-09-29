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
with `rewake --main --name lead claude`, becoming lead-claude. The owner runs both, from
terminals outside any session: a launch from a shell inside a rewake session is refused
with exit 2 before any step below ([launch.md](launch.md#no-session-inside-a-session)).

1. **The directory.** The wrapper opens the `REWAKE_DIR` root and the room
   selected by `--room`, or `default` when omitted. Both are 0700; a symlink,
   foreign owner or loose permissions is refused. Legacy root-level records
   are ignored. A launch with `--worktree` first gets a checkout of the launch
   directory's HEAD on a new branch under rewake's worktree directory, and the wrapper
   moves into it, at the same place in the repository, before anything else
   ([launch.md](launch.md#a-worktree-for-a-launch)).
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
   role. Main stays silent; write reports like general. A write session on Codex is the
   one recipient of an explicit Git metadata grant requested by main — Claude Code takes
   none, and main cannot send to itself — and every role of a directory grant
   ([grants.md](grants.md)); selecting a role does not request access. The record and intro say default general for an omitted
   role flag, or identify the explicit flag.
5. **The harness command line.** The user's arguments go through as written, save the
   exceptions each launch page's notes list: a `--help` written first, rewake's own
   `--worktree`, a flag the line types again in place of an alias's copy, and the Codex
   flags that name another server or configuration, which are refused. rewake adds, for one launch only and never into a config file:
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
     remote/profile/local-provider launches refuse; `--worktree` never reaches it.
6. **The environment.** `REWAKE_SESSION=<name>`, `REWAKE_EPOCH=<epoch>`,
   `REWAKE_DIR=<root>` and `REWAKE_ROOM=<room>`; inherited Claude Code markers are stripped, so the harness
   never borrows another session's socket. A launch from inside a session never gets
   here; the stripping stays for a shell that carries such markers without rewake's.
7. **Launch.** The harness starts with the wrapper's terminal and process
   group. Its pid and start time are added to the record. From now on the
   session is alive only while both processes are.
8. **The intro.** The agent's first context names its session, room, selected
   role and the reason for that role, then the role's whole playbook — its steps and
   its titled sections of rules, the text `rewake guide` prints again in that session —
   ending with MAIL: the shared rule of a first line stating the point, how a waiting
   message is announced with `Rewake:`, and that `rewake guide` has the complete rules
   ([roles.md](roles.md#what-a-session-is-told)).
   The golden copies are in `internal/brief/testdata/`.
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
   by default; a question's delivery 5 at most) and prints one line: `Rewake: delivered to write-codex via app-server`,
   `Rewake: pending for write-codex: …` or `Rewake: held for write-codex: …` (exit 3),
   or `Rewake: failed for write-codex: …` (exit 1). A task or a question is announced
   at once; a `--notify` waits up to four seconds for company (Act 3), and the send
   waits with it — inside its five, so it still answers delivered.
6. **The id.** Unless the send failed, a second line gives the message's id,
   `id <id>`. It is how the sender acts on the message later, while it is its own
   run: `rewake withdraw <id>` takes it back unread, leaving a withdrawn note under
   the same id where a notice went out; `rewake edit <id> "..."` does that and sends
   the new text as a new letter; `rewake send write-codex "..." --to <id>` adds to the
   task, and one report settles both. A short tail of the id is enough
   ([delivery-sent.md](delivery-sent.md)).

A task or a question from main may carry a grant: `--grant-git` for Git metadata, or
`--grant-dir <dir>` for a directory outside the worker's workspace. A granted directory
is resolved and checked here, before anything is written; a protected one is refused
with exit 2, a missing one with exit 1, and one the worker can write already is named
and not carried ([grants.md](grants.md#sending)). Then `send` registers the grant with its
own session's wrapper, which takes it only from a process running below itself and
holds it in memory; a grant not registered — a Codex main's, a command from outside
main's tree — is refused with exit 1 and nothing is written
([grants.md](grants.md#who-can-grant)).

## Act 3. The wrapper announces it

The recipient's wrapper announces fixed groups. Later mail waits for its own group
as soon as delivery is ready. Native start-or-steer reaches active work or wakes idle
work without a required peek or terminal event. Only new member IDs are delivered. The initial burst window is
150 ms. Mail that asks for nothing — a notify, a report — then waits for company: three
seconds after the latest such arrival, four at most from when the earliest was written, and all of it goes
in one notice. A task or a question does not wait, and takes whatever is waiting with it
([delivery.md](delivery.md#the-notice)). It checks expiry/leases and reserves destinations for the
[grouped inbox](inbox-groups.md) before readability.

1. **Readable first.** With the shared destination reserved, rechecks each message under
   the mailbox lock, records its delivery thread, and hard-links the message into `unread/`. An agent told
   about mail may run `rewake inbox` at once, so the text is there before the
   notice goes out.
2. **The notice.** A single member uses `Rewake: lead-claude task, 1 new message`
   and its first-line preview. A group uses `Rewake: <n> new messages` and one bounded
   indented preview of its latest member, including sender and kind; every correcting letter
   in the group — a recall or an edit's replacement — gets a line of its own before it
   ([delivery-sent.md](delivery-sent.md)). The count covers exactly this group, not older accepted
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
     A task or question carrying a directory grant waits, pending, while the session's
     telemetry says a turn is running, and main's wrapper confirms the grant as on
     Codex; the wrapper then keeps it in memory, and the session's `grant-hook` adds the
     directory at the first file-tool write inside it ([grants-claude.md](grants-claude.md)).
     `--grant-git` to a Claude Code session is refused at send, with exit 1.
   - Codex: call turn/start through the reserved TUI connection/generation with empty
     input and [standalone mailbox output](native-mailbox.md): short notice plus fixed
     member identities, never full task bodies. A task or question carrying a grant
     goes on a notice of its own and waits, pending, while the thread is active, so the
     grant holds from its first turn; its directories are checked again first, and the
     wrapper of the main that sent it is asked to confirm it — answered only by that
     wrapper's own process, in this wrapper's namespaces. A grant that no longer passes,
     or that main does not confirm, fails the task and tells its sender; a main that is
     alive and does not answer yet keeps it pending. On an idle thread the
     adapter reads the current local roots without history and sends them with the
     granted directories, and with --grant-git (a write session only) the missing Git
     metadata of the thread's repository. If roots cannot be read, --grant-git alone
     goes without the field and says so in delivery status; a directory grant waits
     and is read again.
     No-flag tasks and report-only groups never get a grant. What rewake granted for
     tasks since reported on is taken out of the roots at the same time
     ([grants.md](grants.md#taking-a-grant-back)). Other notices start idle work or
     steer the active turn. A successful RPC result means delivered. A stale
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
main observer queues [compaction-complete and known-departure notices](session-activity.md),
and the letter that ends a compaction main asked for with `rewake compact`: the command
returns once the compaction has started, and the outcome — the tokens and the count, or
why it was refused or failed — comes later as a notify from the worker
([remote-control.md](remote-control-letter.md)).

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
4. **The rule the agent follows**, from the guide: do the work, then end the turn
   with the result as the final message and stop. Do not answer with `rewake send`;
   do not answer a `notify` at all. After a context compaction, re-read the task
   with `rewake inbox --owed` rather than from the summary, and read new mail with
   `rewake inbox`: `--owed` prints again, in full, every message whose `awaiting/`
   record is still there, and changes nothing; tasks and questions still unread it
   only counts, in a last line naming `rewake inbox`. The sender's side of the same
   record: `rewake inbox --awaited` in the sending run lists, by recipient, what it
   sent and has no report on yet — unread, read and being worked on, pending,
   stopped — and what will get none because its recipient ended
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
3. **A grant ends.** A directory granted with a task lives at least until this
   report; a stopped turn or a pending mark keeps it, and an error report or a
   withdrawal ends it as a report does. On Codex rewake takes it back at the next
   delivery into the conversation it was granted in, not at the report itself, and a person's own turn
   in the terminal drops it at once. On Claude Code the hook takes it back at the first
   read or `rewake` command after the report in the `default` and `acceptEdits` modes;
   in `plan`, `bypassPermissions` and auto it stays until the session ends
   ([grants-claude.md](grants-claude.md)). A cold resume before the report starts a new
   run, which takes over the task's wait and has main confirm the grant again — only
   while that main still runs, in the same conversation, within a day of the read; a
   restarted main restores nothing, and a resume after the report gets nothing back
   ([grants-resume.md](grants-resume.md)). The Git metadata of a Codex worker's own
   checkout, opened by `--grant-git` alone, is not journaled and rewake never takes it
   back: it stays for the rest of the thread unless a typed turn replaces the roots
   ([git-grants.md](git-grants.md)).
4. **Forget the reported messages** after all reports are written and the turn
   receipt is marked done. Cleanup matches the original run and wait; newly
   read messages remain owed to the next result.
5. **The sender is woken** by its own wrapper, through Act 3, with
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

A worker that ends a turn before the work is done — waiting on anything outside the
turn: background work still running, the owner, a refusal to be cleared — runs
`rewake pending "<what it waits for>"` first. That one turn end then reaches the
waiters as a `pending` message with that text first and the turn's own answer after it,
owing nothing and settling nothing, and the next turn end without a mark is the report
([turn-outcomes.md](turn-outcomes.md#interim-turn-ends-rewake-pending)) — on Claude Code
after the Stop hook has held it once to ask whether the work is done
([the confirmation](turn-outcomes.md#the-confirmation-on-claude-code)).

The rest of the flow — a blocking question (Act 6), a notify (Act 7), the session's
end (Act 8), one exchange traced through its files, where the flow can stall, and
how a report notes that the reader's conversation changed underneath it — is in
[flow-endings.md](flow-endings.md).
