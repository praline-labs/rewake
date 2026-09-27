# The flow: questions, notifies, session end, and the reference tables

Split from [flow.md](flow.md) by subject on September 27, 2026: that document carries
the ordinary task exchange end to end, Acts 1 through 5; this one carries the acts that
depart from that path — a blocking question, a notify nobody answers, the session's
end — and the reference material that describes the flow as a whole: one exchange traced
through its files, where the flow can stall, and how a report notes that the reader's
conversation changed underneath it.

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
| a notify or a report with nothing else arriving | it waits three seconds for company, then goes alone; a task arriving meanwhile takes it along at once | delivered after about three seconds, pending meanwhile with why |
| a turn interrupted with Esc or Ctrl+C | stopped advises the waiters to wait, and goes to nobody when none waits; original work stays owed | yellow notice; human continuation reports its result |
| a compaction main asked for with `rewake compact` | the command returns once it has started, or says requested when its start was not seen in 3 seconds; the worker's module or wrapper waits for its end ([remote-control.md](remote-control.md)) | exit 0, then a notify with the tokens and the count, or the refusal or failure |
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
prefixed by an indented ↳, and each recall or replacement on a line of its own before it. They never substitute the preview for inbox content.
Empty first lines stay empty; long ones are clipped with an ellipsis to about
100 columns. Errors use failed/red status; keyboard stops use killed/yellow. Active question reservations
are excluded from ordinary counts and previews.
