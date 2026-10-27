---
description: What each turn end reports — finished, error, stopped or pending — who it goes to, and the hold that keeps an interim end from closing a task.
---

# Turn outcomes other than a report

What reaches the sessions waiting on a turn when that turn does not simply finish: it
failed, it was stopped, or the session marked it as not the end of the work. The ordinary
path — a turn end, its `finished` report, and how waits are recorded and settled — is in
[delivery-turn-end.md](delivery-turn-end.md#the-end-of-a-turn); the rules are O1 and O2 of
[rules/turns.md](rules/turns.md).

## Failed turns

After a failed or interrupted turn, a session is woken only by new mail: what was already
announced is never announced again, and new mail needs neither a manual continuation nor a
peek to reach it. The same holds for the stops below, and for an interim end. Only a finished
turn that leaves a task announced in it unread brings one reminder of it
([delivery.md](delivery.md#the-notice)); a task announced in a failed, stopped or interim
turn is not reminded of later.

The adapter that heard the turn end hands the core its reason (`adapter.TurnEnd`): a failed
end and a refused one — the harness declined the turn — both report `error` with the
harness's words unchanged, an aborted one `stopped`. An empty answer after the turn read work
produces an error whose text says the turn ended with no final answer and no reason. Successful turns produce `finished`. `error` is a kind of
message with no send flag — `send --error` is refused — and creates no reply obligation.

Errors go to all live waiters of the run; with none, they go to the room's main. A failing
main, or a room with no main, keeps the error in the failing session's own unread mailbox
without announcing it to itself. A waiting question takes the error and exits 1.

Every adapter of this build reports failures: Claude Code's mod sends `turn/end` with the
reason `failed` or `refused`, pi's extension a stop reason other than `stop`, and the
fixture's program its end frame.

## Interim turn ends (`rewake pending`)

A turn end is the report by default. A session about to end a turn that has not finished the
work — waiting on anything outside the turn: background work still running, the owner, a
refusal to be cleared — runs `rewake pending "<what it waits for>"` first; the playbooks of
the roles that report say so. That turn end then reaches every waiter as a `pending` message
instead of the report: that text on its first line, then a blank line and whatever the turn
itself said, so findings written into the answer are not lost behind the mark. It owes
nothing, settles nothing, and the waits stay open. The command itself answers one line,
`Rewake: marked pending; at this turn's end <senders> will read that the work goes on.` The
next turn end without a mark is the report. A forgotten `pending` gives the behaviour without
it, never worse; a `finished` marker the worker had to remember would leave the obligation
open for ever when forgotten.

- **The marks** are `inbox/<name>/pending/marks/<epoch>/<time>-<id>`, one file per
  `rewake pending` call, written only under the mailbox lock. A mark belongs to the turn it
  was made in and to no other, and the tie is time on the machine's boot clock
  (`CLOCK_BOOTTIME`, `internal/infra/boottime`), one clock for every process, which a
  wall-clock jump after a suspend does not move back. A turn end honours a mark only if its
  time lies in the turn's window; of several marks in the window the latest decides, so a
  second `pending` in one turn replaces the text. A mark older than the window was made in
  an earlier turn whose end never reached rewake and decides nothing; a mark newer than the
  turn's end was made in a later turn while this one was being published, and is left for it
  ([delivery-turn-end.md](delivery-turn-end.md#pending-marks)). A turn whose start is not known
  cannot be tied to a mark and ends as a report. Every failure falls to that side: the
  behaviour without `pending`.
- **The turn's start and end.** The adapter times both as its harness reports them and
  carries them with the end, so a turn published late — queued, or retried when the mailbox
  was busy — cannot take a mark made in the turn after it. The start is also recorded for the
  host (`mail.Box.RecordTurnStart`, from the boundary's `TurnHandler.Started`): each reading a
  file of its own under the run's `awaiting/<epoch>/.turn-starts/`, named by the reading and
  never overwritten, the latest being the largest name; a start recorded after a mark's time
  proves the mark's turn over when its end was lost. A directory of readings that cannot be
  read refuses `rewake pending`.
- **The gap that remains.** If a turn end with a mark is lost and the harness then starts a
  turn by itself, reporting no start, and that turn's end is the first one heard, its final
  answer leaves as an interim message and the report does not come.
- **A turn background work woke.** A worker that marked a turn and was then woken by a
  finished subagent or background task must mark that turn too, or its end is the report;
  the briefing says so. An adapter that can hold an end open lets the core ask once when the
  mark is missing ([below](#the-confirmation)); elsewhere the briefing is all there is.
- **Only a normal finish is softened.** A failed or stopped turn reports `error` or `stopped`
  as it would have, with the mark in its window all the same.
- **The message** has its own report id (the wait's id plus `-pending-<turn>`), like
  `stopped`, so the report that follows is not taken for a copy. A blocked `--question` does
  not take it for its answer: it is a turn outcome for being kept readable when its notice
  fails (`mail.IsReport`), but not one that settles (`mail.Settles`), and a question waiting
  in the sender's session leaves it to be announced. The notice reads
  `Rewake: <session> pending, …` with the mark's text as its preview.
- **Refused** outside a session, for main — whose turns are reported to nobody — without
  text, and when no read task or question in this run waits for a report; each refusal says
  why.

## The confirmation

After an interim turn end, rewake holds the next turn end that carries no mark, once, and
asks the session whether the work is done. It catches the mark forgotten on a turn a finished
subagent woke, without a marker the worker must remember. The check runs for an adapter that
can hold an end open; in this build that is the fixture's alone, and on Claude Code and pi the
briefing alone asks for the mark (O1's known limit).

- **How.** The adapter calls the completion handler's `Confirm` in place of `Publish`, and
  `ConfirmCompletion` answers what the end said (`mail.Outcome`): its reason in `Held` when
  it was held, which the wrapper hands the adapter, `""` when it was published;
  the adapter continues the turn with that reason and confirms the next end again. Such an
  end names its event and may be retried, so it is refused without its event id and read
  boundary.
- **When.** An end is held only when all of these are true: the adapter says it may be held;
  it did not fail; this run's last published turn end was interim; a sender whose session
  still runs waits on a task; the ending turn made no mark; and no answer is kept from an
  earlier hold. A sender proven gone — no record, or one of another run — is left out of the
  senders; a sender whose record could not be read fails the check, and the end is published
  to every sender. Then it publishes nothing and keeps the turn's answer. The reason quotes
  the pending line, names the senders, and says both ways on: still waiting — run
  `rewake pending` and end the turn; done — end the turn, and the answer just given goes as
  the report with whatever is added after it.
- **Once.** A turn end while an answer is still kept is never held, so a turn is held at most
  once. Each later unmarked turn end after an interim one is asked again: every interim end
  starts a new episode.
- **The records**, beside the marks in `inbox/<name>/pending/` and changed only under the
  mailbox lock: `interim.json` holds the run's last word on the work
  ([delivery-turn-end.md](delivery-turn-end.md#the-interim-record)); `kept.json` holds the
  held answer, the held end's operation and its reason.
- **The same end confirmed again** — its answer lost — gets the same reason, and nothing is
  published or kept again; once a continuation took the kept answer, its done record names
  the held end (`HeldEndTaken`), and the held end confirmed again is answered as published
  ([delivery-turn-end.md](delivery-turn-end.md#a-held-end-confirmed-again)).
- **The report keeps the held answer.** The continuation's end carries only what the model
  said after the hold, so the next turn end heard puts the kept answer into its own outcome: a
  finish gives the kept answer, then the continuation; a pending mark made in the continuation
  gives the mark's line, the kept answer, then the continuation; a failure gives its error,
  then the kept answer; a stop gives its line, then the kept answer, with the waits open as
  after any stop. The answer is dropped once that outcome is published, as a step of the turn
  end; each kept answer has its own version, and the step drops the version the outcome
  carried and never a later one.
- **A hold is not an end.** A held end publishes nothing and journals no end of the run, so a
  mark the continuation makes falls within the turn its end takes it from.

## Stops

A stopped outcome is advisory and comes from the adapter that heard the turn end: its reason
is `aborted`. It goes to the current waiters — the senders of read, unsettled tasks and
questions — with the text "the person at the keyboard stopped this turn". With no waiter it
goes nowhere: unlike an error, it does not fall back to main, and main's own interruption
puts nothing into its own inbox. A person interrupting a turn no rewake task depends on is
their own business, and the session turning idle says all there is to say; the routing
(`internal/core/mail/turn_reports.go`) is shared by every adapter. A waiter whose session has
ended, or runs under another epoch, is no waiter any more: a `stopped` meant only for it is
dropped. Its journal is kept all the same, so a repeated event publishes nothing either.

A turn a main aborted with `rewake interrupt` is stopped the same way, with the text
"<main> interrupted this turn with rewake interrupt" instead, and the same routing
([remote-control.md](remote-control.md)). The stop owes no reply and has its own report id,
separate from the eventual result. Its own operation keeps the original waits intact. A
finished or error outcome for the same turn is an operation of its own and settles those
waits, so a person's continuation can publish `finished` with the same `inReplyTo`. Main
waits for the person instead of resending. A waiting question prints stopped and exits 1; the
later result remains an ordinary inbox report.

A question's answer receipt stores the report id in `received/<question id>`. Only an exact
match confirms that report; a `stopped` does not acknowledge a later `finished` with the same
`inReplyTo`.
