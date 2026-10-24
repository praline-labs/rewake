---
description: The other ends of a message's path — a blocking question, a notify, a session's end — one exchange traced through its files, and where the flow can stall and what the sender sees.
---

# The flow: questions, notifies, session end, and the reference tables

[flow.md](flow.md) carries the ordinary task exchange end to end, Acts 1 through 5; this
document carries the acts that depart from that path — a blocking question, a notify nobody
answers, the session's end — and the reference tables of the flow as a whole: one exchange
traced through its files, and where the flow can stall.

## Act 6. A question, when the sender wants to block

`rewake send write-claude "which port?" --question` is a task whose sender waits for the
answer in the same command.

1. **Reserve before publishing.** The sender creates `answering/<id>` in its own mailbox and
   keeps writing the boot clock into it every second while it lives. Then it publishes the
   question exactly as in Act 2.
2. **The recipient does its part** exactly as in Acts 3–5; it cannot tell a question from a
   task.
3. **The report arrives** in the sender's mailbox. Its wrapper sees a fresh reservation for an
   id in `inReplyTo`, links the report into `unread/`, and leaves it there without a notice and
   without a final status, checking the reservation on every tick. Ordinary `inbox` skips it.
4. **The waiting send takes it.** Under the lock: print the answer, write `received/<id>` with
   the exact report id, remove the mark. The report moves to `done/` only once every question
   it answers has a receipt and no fresh reservation remains. Output failure exits 1 without a
   receipt, so the answer is kept.
5. **When the sender is gone** — timeout (exit 3), kill, or crash — the mark goes stale within
   three seconds and the wrapper announces the report the ordinary way. Nothing is lost, only
   blocked for a while. A send that ends on its own takes its mark away under the lock.

## Act 7. A notify, when nothing is owed

`rewake send write-claude "the migration is merged" --notify` goes through Acts 2–4 and stops
there: reading it records no wait, so no report comes back. It is the right kind for a
heads-up and for a probe.

## Act 8. The session ends

The harness exits, or Ctrl+C reaches the foreground group, or `kill` reaches the wrapper
(which then forwards it to a harness still alive a moment later).

1. The wrapper closes the inbox: waiting mail gets `failed: session ended`. Unexpired reports
   stay readable for their epoch. A reserved answer keeps its queue entry and lease semantics
   so its waiting command can consume it. A task the session had read and not reported on is
   lost: `rewake inbox --awaited` shows its sender "no report coming: <name> ended; check its
   saved work and give the task to a new worker"
   ([delivery-owed.md](delivery-owed.md#what-others-owe-you-rewake-inbox---awaited)).
2. The record and the socket are removed; a later `list` will not show the name, and a `send`
   to it gets exit 2.
3. The wrapper exits with the harness's exit code.

A wrapper that dies without its harness leaves an agent that is alive and no longer
addressable; no view lists the dead record, and the next change form that reads the registry
removes it.

## One exchange, as files

`main-claude` gives `write-claude` a task and gets the result back.

| step | inbox/write-claude | inbox/main-claude |
|---|---|---|
| send | `<id>.json`, `<id>.status: pending` | |
| wrapper announces | `unread/<id>.json`, status `delivered` | |
| write-claude reads | `awaiting/<epoch-w>/main-claude`, status `read`, `done/<id>.json` | |
| write-claude's turn ends | `awaiting/` emptied, `journal/<op>.done` | `<rid>.json` (finished, inReplyTo: id) |
| main-claude's wrapper announces | | `unread/<rid>.json`, status `delivered` |
| main-claude reads | | status `read`, `done/<rid>.json`; no wait recorded |

Exit codes the sender sees along the way: 0 delivered, 3 accepted and still pending or held,
1 failed or unreachable, 2 wrong call — unknown session, bad flag, a question to a silent
role.

## Where the flow can stall, and what it says

| situation | what happens | what the sender sees |
|---|---|---|
| a notify or a report with nothing else arriving | it waits three seconds for company, then goes alone; a task arriving meanwhile takes it along at once | delivered after about three seconds, pending meanwhile with why |
| a turn interrupted with Esc or Ctrl+C | stopped advises the waiters to wait, and goes to nobody when none waits; the original work stays owed | a notice of the stop; the person's continuation reports the result |
| a compaction main asked for with `rewake compact` | the command waits up to 5 seconds for the session to take the request and up to 10 for its answer, and returns once the compaction has started, or says requested when its start was not seen in that time; the outcome comes later as a letter ([remote-control.md](remote-control.md)) | exit 0, then a notify with the tokens and the count, or the refusal or failure |
| a turn interrupted by main's `rewake interrupt` | the same stopped, naming main instead of the person ([remote-control.md](remote-control.md)) | a notice of the stop naming main |
| recipient's wrapper gone | the record is removed by the next change form | exit 2, no such session |
| a reader's stdout blocks | it holds the lock; the server waits, a turn end five seconds, `inbox` ten | delays, then "mailbox is busy" |
| the main session is asked a question | refused before publication | exit 2 with a hint |
| a task to a run whose turn boundary is not live yet — a Claude Code session before its mod's hello | refused before anything is written | exit 1, naming why |
| a process of an ended run | its epoch no longer holds the name | `inbox` and `send` refuse |
| Claude Code's socket not yet up | `ENOENT` while the harness lives | `pending`, then delivered |
| Claude Code up, its lane just started | the first notice waits three seconds from the lane's start | `pending`, then delivered |
| Claude Code's gate holds the line | both copies stay, no retry | `held` (exit 3), then delivered, or failed and a note |
| Claude Code's gate says it holds only after the 300 ms | the delivery is taken back, for up to a minute | `delivered` (exit 0), then `held`; if it fails, a note |
| the wrapper is killed while its session holds a line | the next session with the name fails it | `failed`, the session ended, if send still waits; a note once the name is taken again |
| pi compacting | the extension answers `pending` | `pending`, then delivered once the compaction ends |

How a report notes that the reader's conversation changed underneath it is in
[delivery-turn-end.md](delivery-turn-end.md#reports-after-a-thread-change); how a failed,
stopped or interim turn reaches its waiters, in [turn-outcomes.md](turn-outcomes.md).
