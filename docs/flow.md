---
description: The whole path of one message, from a launch through send, notice and read to the report, with every process, file and status on the way, and how main starts a worker in a space.
---

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
| **wrapper** | the `rewake claude` or `rewake pi` process, the harness's parent | as long as its harness |
| **harness** | Claude Code, pi, the ordinary program in the terminal | the session |
| **agent** | the model inside the harness, calling `rewake send`, `rewake inbox`, `rewake whoami` from its shell | a turn at a time |
| **sender** | any `rewake send` process: an agent's shell, a person's shell | until the status arrives |
| **space host** | a detached `rewake` process `rewake space open` starts: owns a worker's terminal, watches main's run, ends the worker after it or when main closes the space (A space, below) | as long as its worker and main |
| **turn boundary** | the harness's word that a turn ended, which its adapter in the wrapper hands the core; on Claude Code the mod says it (Act 5) | a moment |

There is no daemon. Each wrapper services exactly one mailbox: its own
session's. Everything the processes share is files under the state directory,
`REWAKE_DIR`, 0700, checked on every open. Each room has its own subdirectory;
the paths below are relative to `<REWAKE_DIR>/v2/rooms/<room>/`:

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
inbox/<name>/journal/<op>.done   a turn end recorded done, written last, named by the end; kept while its run lives
inbox/<name>/pending/            this run's pending marks, its last word on the work, a held answer
inbox/<name>/once/<epoch>/<id>   a heads-up's publication mark: intent, then published
inbox/<name>/threads/<id>        selected delivery thread, when supported
inbox/<name>/.lock               the mailbox lock, one flock for every state change
                                 (every kind a mailbox holds: docs/mailbox-records.md)
sock/<name>.<epoch>.sock         Claude Code's inbound socket for this run
sock/<name>.<epoch>.reply.sock   where that run's wrapper hears what the socket did with a line
spaces/<id>.json                 a space main opened: its worker, its space host, its end
sock/space.<id>.sock             the worker's screen, served by its space host to the shell as a
                                 raw stream
```

## Act 1. A session starts

`rewake claude` starts main-claude, the sender in this example, in the room of its directory;
the owner runs it from a terminal outside any session, and a launch from a shell inside a
rewake session is refused with exit 2 before any step below
([launch.md](launch.md#no-session-inside-a-session)). Main raises the receiver, api-claude, with
`rewake space open api --alias claude-write` ([A space](#a-space-main-starts-a-worker)); that worker's launch goes through the
same steps, with its name, role and room from the space's record.

1. **The directory.** The wrapper resolves the `REWAKE_DIR` base, its root `v2/` and
   the room of its directory — `<slug>-<hash>` of the working directory resolved with symlinks
   followed ([launch.md](launch.md#main-in-the-room-of-its-directory)), or main's room for a
   worker — creating nothing. Each
   that exists is 0700; a symlink, foreign owner or loose permissions is refused.
   Before anything of the room is made or
   locked, it takes the room's build lease for its whole life — the lease's bootstrap
   makes whatever level is missing, and nothing else makes any; a main's launch then writes the
   resolved directory into the room's file `directory` once, which `rewake list` shows — the room's `build` names the content of the rewake
   executable whose sessions write there, a launch of another build switches it only
   while nothing holds the room, and every change form of a shell or a tool call holds the
   lease too, refused in a room of another build
   ([state.md](rules/state.md#the-rooms-lease)). A view form takes no
   lease, writes nothing and fails if it tries; in a room of another build it says so
   once on stderr. The state rewake 1.x left beside the root is never read
   and is left as it is.
   A launch with `--worktree`
   then gets a checkout of the launch
   directory's HEAD on a new branch under rewake's worktree directory, and the wrapper
   moves into it, at the same place in the repository, before anything else
   ([launch.md](launch.md#a-worktree-for-a-launch)).
   Then, before any name is claimed, the bounded read of Claude Code's version: one
   older than 2.1.287, or a read that cannot show its process ended, refuses the launch
   with nothing published.
2. **The name and role.** Under the room lock, append the harness ID to the role's prefix,
   `main`, or to the space's name for a worker.
   Automatic collisions add -2, -3 after the harness suffix; explicit conflicts
   refuse with the full address. Prefix and final name must fit the name syntax
   and 32-character limit. Publication uses `link()`: live names remain taken,
   dead records can be replaced, and other rooms may use the same address.
3. **The run.** The wrapper's pid, its start time and the machine's boot id make
   the **epoch** (`<pid>.<ticks>.<boot>`). A name can be started many times; the epoch
   says which start this is, and the boot keeps it from recurring after a restart.
4. **The role.** A person's launch is main; under the room lock it is refused, exit 1, if a
   live main already occupies the room — one main per directory. A worker's role is the
   space's. Main stays silent; write reports like general. A role that owes reports is refused,
   exit 1, on a harness whose contract reports no turn's end, and on Claude Code when a cause
   known to keep the mod from loading stands; the mod proves
   the boundary after the start ([claude-turns.md](claude-turns.md#readiness-and-admission)).
   Selecting a role requests no access. The record and intro say why the role was taken:
   "rewake <harness> always starts main, one per directory", or "raised by main with rewake
   space open as <role>".
5. **The harness command line.** The user's arguments go through as written, save the
   exceptions each launch page's notes list: a `--help` written first, rewake's own
   `--worktree`. rewake adds, for one launch only and never into a config file:
   - Claude Code: `--messaging-socket-path sock/<name>.<epoch>.sock`,
     `--append-system-prompt <intro>`, `--allowedTools "Bash(rewake:*)"` unless the
     line gives its own, `--plugin-dir` naming the mod written into the run's directory
     ([claude.md](claude.md#loading-the-mod-for-one-run)), and
     `--session-id` naming the new conversation unless the line continues one; a worker
     also gets `CLAUDE_CONFIG_DIR` naming rewake's worker folder and a `--settings` of its
     run ([launch.md](launch.md#claude-code)). The mod tells the wrapper when a turn starts
     and ends (Act 5).
6. **The environment.** `REWAKE_SESSION=<name>`, `REWAKE_EPOCH=<epoch>`,
   `REWAKE_DIR=<base>` and `REWAKE_ROOM=<room>`; inherited Claude Code markers are stripped, so the harness
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
   The golden copies are in `internal/core/brief/testdata/`.
9. **Serving.** The wrapper watches `inbox/<name>/` with inotify, polls every
   second as the safety net, sweeps old mail every ten minutes, and waits for
   the harness to exit.

`rewake list` shows only this room: name, harness, age, cwd, room and role
for every session. It reads the records and checks that both pids are alive with
matching start times; a view, it writes nothing, so a dead session's record is left
for a change form or a later launch to prune
([state.md](rules/state.md#what-is-a-writer)). A record it cannot read is
shown as a session whose state is unknown.

## Act 2. A task is sent

`rewake send write-claude "run the smoke and report what failed"` — from the main
session's shell, or from a person's shell in the same room. A shell without
`REWAKE_ROOM` takes the room of its working directory. Commands cannot name a different room.

1. **Who is sending.** `from` is `REWAKE_SESSION` when the run in
   `REWAKE_EPOCH` still holds that name, else `shell`. A leftover process of an
   ended run cannot sign as the new one.
2. **Who receives.** The recipient must be alive in this room; otherwise exit 2 with the live
   names listed.
3. **The kind.** Plain `send` is a `task`; `--question` and `--notify` are the
   other two. A task or a question to a session whose TurnBoundary is not live — its
   record's live set; on Claude Code, until the mod has proven it — is refused
   here, before anything is written, exit 1: nothing would carry its answer back. A note
   still goes. Where the boundary is live, a question to a silent recipient is refused
   next, exit 2: that role never reports, so it could never answer. A letter to a run
   whose Wake is not live is written and reported accepted, exit 3, at once: no notice
   can reach it until its Wake is live, and it reads the letter with `rewake inbox`.
4. **The file.** `inbox/write-claude/<id>.json.tmp`, renamed to `.json`. The id is
   time-sortable. Nothing else is touched: the sender does not deliver. A `--notify`
   is first journaled under a receipt in the sender's mailbox and then written once
   under the recipient's lock, so a repeat or `rewake retry` cannot publish it twice
   ([rules/effects.md](rules/effects.md#receipts-retries-and-once-marks)).
5. **The wait.** The sender polls `<id>.status` for up to `--wait` seconds (5
   by default; a question's delivery 5 at most) and prints one line: `Rewake: delivered to write-claude via socket`,
   `Rewake: pending for write-claude: …` or `Rewake: held for write-claude: …` (exit 3),
   or `Rewake: failed for write-claude: …` (exit 1). A task or a question is announced
   at once; a `--notify` waits up to four seconds for company (Act 3), and the send
   waits with it — inside its five, so it still answers delivered.
6. **The id.** Unless the send failed, a second line gives the message's id,
   `id <id>`. It is how the sender acts on the message later, while it is its own
   run: `rewake withdraw <id>` takes it back unread, leaving a withdrawn note under
   the same id where a notice went out; `rewake edit <id> "..."` does that and sends
   the new text as a new letter; `rewake send write-claude "..." --to <id>` adds to the
   task, and one report settles both. A short tail of the id is enough
   ([delivery-sent.md](delivery-sent.md)).

## Act 3. The wrapper announces it

The recipient's wrapper announces fixed groups. Later mail waits for its own group as soon
as delivery is ready. The notice reaches active work where its harness allows, or wakes idle
work, without a peek or a terminal event. Only new member ids are announced. The first
collection is 150 ms. Mail that asks for nothing — a notify, a report — then waits for
company: three seconds after the latest such arrival, four at most from when the earliest was
written, and all of it goes in one notice. A task or a question does not wait, and takes
whatever is waiting with it. Each member's expiry and reservation are checked again before
it is made readable ([delivery.md](delivery.md#the-notice)).

1. **Readable first.** With the shared destination reserved, rechecks each message under
   the mailbox lock, records its delivery thread, and hard-links the message into `unread/`. An agent told
   about mail may run `rewake inbox` at once, so the text is there before the
   notice goes out.
2. **The notice.** A single member uses `Rewake: lead-claude task, 1 new message`
   and its first-line preview, ending `(a preview only: run rewake inbox)` for a task or a
   question. A group uses `Rewake: <n> new messages` and one bounded indented preview of its
   latest member, with sender and kind; each correcting letter in it — a recall, an edit's
   replacement — gets a line before it ([delivery-sent.md](delivery-sent.md)). The count covers
   this group alone, not older unread mail or later arrivals; mail arriving meanwhile joins it.
3. **The adapter**, outside the lock because it can take seconds:
   - Claude Code: connect to the session's socket and write one JSON line
     whose content is a `<task-notification>` block with that summary. The
     interface draws it as a single green `● Rewake: lead-claude task, 1 new
     message(s)` line — the same line its own background tasks get — and the
     model wakes if it was idle. The line names the wrapper's reply socket and
     an id, and the session's inbound gate answers there if it holds or refuses
     the line; silence for 300 ms counts as taken, and a word after that for a
     minute takes the delivery back. The first notice to a new
     session waits three seconds from the launch, since until the session
     has drawn its interface the gate holds everything
     ([delivery-adapters.md](delivery-adapters.md#claude-code)).
   - pi: send its extension a `wake` frame with the notice and the conversation it is for;
     the extension steers it into a running turn after the tool batch, or starts a turn
     ([delivery-adapters.md](delivery-adapters.md#pi)).

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
[availability notifications](session-state.md#availability-notices) when peers
become ready; a later main learns which peers were already available. The same
main observer queues [compaction-complete and known-departure notices](session-state.md#compaction-notices),
and the letter that ends a compaction main asked for with `rewake compact`: the command
returns once the compaction has started, and the outcome — the tokens and the count, or
why it was refused or failed — comes later as a notify from the worker
([remote-control.md](remote-control.md#the-letter)).

1. **Whose mailbox.** `REWAKE_SESSION` names it, `REWAKE_EPOCH` proves the run:
   a stale run, or a process with a name and no epoch, is refused. Mail an
   earlier run of the name was told of and never read is shown too, marked `for an
   earlier run` (`earlierRun`); a task or a question in it records no wait.
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

The harness itself says when a turn is over, through its adapter in the wrapper: the
fixture's program with an end frame, Claude Code through the mod's `turn/end`
([claude-turns.md](claude-turns.md)), pi through its extension's `agent_settled`. Every adapter's end goes through the same internal
reporting function. A subagent's turn is not reported and changes none of the
session's waits.

1. **Who is owed.** Under the lock, the turn end reads
   `awaiting/<own epoch>/`. A silent role emits no successful reports;
   failure callbacks still route errors. An identified turn carries its read boundary,
   and only waits read at or below it are this end's
   ([delivery-turn-end.md](delivery-turn-end.md#the-end-of-a-turn)). A step that fails
   returns an error, and a retry or the next end runs the steps again.
2. **The report.** For each waiting run, a `finished` message into that
   sender's mailbox: text is the final reply, `inReplyTo` lists the messages
   read from that run since the last report, `toEpoch` is that run — not
   whoever holds the name now. The id is derived from the wait, so a retried
   end finds the report written and does not write it again, and answers nothing read
   above its boundary even when new work arrived between attempts.
3. **Forget the reported messages** after all reports are written — never before, since
   a cleared wait counts as answered; then the end is recorded done
   (`journal/<op>.done`), which a retry of the same event is answered from. Cleanup
   matches the original run and wait; newly read messages remain owed to the next
   result.
4. **The sender is woken** by its own wrapper, through Act 3, with
   `Rewake: write-claude finished, 1 new message(s)`. The sender reads it with
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
([turn-outcomes.md](turn-outcomes.md#interim-turn-ends-rewake-pending)); an adapter that can
hold an end lets the core ask once, before that report, whether the work is done.

## A space: main starts a worker

Main can read `rewake workers` and run `rewake space open api --alias claude-write --worktree
-- <harness args>` from its shell, while its run has `Spaces` live. The alias supplies its
harness, worker wrapper, role and applied settings; an alias is no launch of its own, and a
worker is raised only from one. Listing is a view for any role, not a launch. Opening writes the record, `spaces/<id>.json`, opening,
and re-executes rewake as a launcher in a session of its own; the launcher starts the
**space host** and exits, so the space host is nobody's child in main's tree. The space host
records its own run, checks that its chain of parents does not meet main's wrapper — or closes
the record, the worker never started — and starts in a pseudo-terminal it owns an ordinary
`rewake claude --worktree <harness args>` — the worktree resolved once from the typed flag and
the template's `worktree = true` ([worktree-tools.md](worktree-tools.md)) —, `--command <wrapper>` before the harness when the
alias names one, with main's base, `REWAKE_SPACE=<room>/<space id>` — and
`REWAKE_SPACE_ENTER=1` for a template's `enter_worktree = true`, whose worker may claim its one
checkout later, which the host then captures as a launch's, and `REWAKE_SPACE_TYPE` with its
text for a worker raised for a kind of work, its briefing's last section
([worker-types.md](worker-types.md)) — and none of main's session in its environment; the launch believes the variable only from its parent, that space's host. From there the worker is any session: Act
1 for it, its record `sessions/api-claude.json`. The space host records the worker's run, open;
the command answers once that run is registered, within 30 s. A harness whose workers need
something only the person gives in the worker's pane adds it to the answer: a Claude Code
worker whose configuration directory has no login yet opens all the same, and the answer tells
main the person is to type `/login` once in that pane ([launch.md](launch.md#claude-code)).

Main sends `api-claude` a task, and its report comes back as in Acts 2 to 5. When main was
started in the shell — at a terminal, always — main runs on the shell's own pty and each worker's pane is
fed the raw bytes from its space host's socket; Ctrl+\\ then a key chooses the worker shown
beside main, and everything else typed goes to the focused program ([shell.md](shell.md)).
Without the shell the worker runs unseen.

The space host reads main's run once a second. When it reads ended — main exited, crashed or
its wrapper was killed — it sends the worker's process group SIGHUP, SIGTERM after a second and
SIGKILL after five more, closes the record with what it saw — the worker ended, its checkout
there or gone — and exits. A worker that ends by itself closes the space the same way, and so
does one main closes with `rewake space close api`: the command moves the record to closing
with main's reason, the space host sees it at its next tick and ends the worker by that
sequence, and the command answers once the record is closed, within 20 s; what the worker still
owed reads as lost in `--awaited`. A worker is never restarted in place: main closes it and
opens a new space. Once the worker's process group is gone, every close removes the worker's
checkout and its branch when nothing would be lost — no changes, no edits hidden by an
assume-unchanged or skip-worktree mark, no lock or submodule, every commit on another local
branch; files git ignores go with it — and otherwise keeps it, adding why to the record's reason, which the close's answer
shows. A checkout left behind is listed as kept by `rewake space list` until it is gone
([space.md](rules/space.md)).

The rest of the flow — a blocking question (Act 6), a notify (Act 7), the session's
end (Act 8), one exchange traced through its files, where the flow can stall, and
how a report notes that the reader's conversation changed underneath it — is in
[flow-endings.md](flow-endings.md).
