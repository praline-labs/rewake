# Turn outcomes other than a report

What reaches the sessions waiting on a turn when that turn does not simply finish: it
failed, a person stopped it, or the session marked it as not the end of the work. The
ordinary path — a turn end, its `finished` report, and how waits are recorded and
settled — is in [delivery-turn-end.md](delivery-turn-end.md#the-end-of-a-turn).

## Failed turns

After a failed or interrupted turn, a session is woken only by new mail: what was
already announced is never announced again, and new mail needs neither a manual
continuation nor a peek to reach it. Owner decision, September 19, 2026; the evidence
is in [native-terminal-progress.md](native-terminal-progress.md). This holds for the
keyboard stops below as well.

The adapter that heard the turn end emits error with the harness reason unchanged. Since
S10, October 11, 2026, the end names its reason (`adapter.TurnEnd`,
[code.md](code.md#the-adapter-api)): a failed end and a refused one — the harness declined
the turn — both report error, an aborted one stopped. Received empty
completion after read work produces an error with empty text. Successful turns
still produce finished. Error is in the kind catalog but has no send flag;
`send --error` is refused. It creates no reply obligation.

Errors go to all live waiters for this run; with none, they go to the room's
main. A failing main, or a room with no main, retains the error in the failing
session's own unread mailbox without announcing it to itself. Identified turns
are idempotent; callbacks without turn ids are separate invocations. Delivery uses failed task-notification status on the socket path and a red circle on
the server path. A waiting question consumes the error and exits 1.

Failure observation depends on the harness providing a callback: the adapter that
hears a turn's end hears a failed one. Claude Code, whose StopFailure hook reported its
failures until S9, October 11, 2026, reports none in this build.

## Interim turn ends (`rewake pending`)

Owner decision, September 23, 2026: a turn end stays the report by default. A session
about to end a turn that has not finished the work — waiting on anything outside the
turn: background work still running, the owner, a refusal to be cleared — runs
`rewake pending "<what it waits for>"` first. The write and general playbooks say so
since September 25, 2026, when a worker's turn ended waiting for the owner and was
taken as its report; before, only write's did, and for background work alone. That turn
end then reaches every waiter as a `pending` message instead of the report: that text
on its first line, then a blank line and whatever the turn itself said, so findings
written into the answer are not lost behind the mark (since September 26, 2026; before,
the mark's text replaced the turn's). It owes nothing, settles nothing, and the waits
stay open. The command itself
answers one line,
`Rewake: marked pending; at this turn's end <senders> will read that the work goes on.`
The next turn end without a mark is the report, as usual. A forgotten `pending` gives
the behaviour without it, never worse; a `finished` marker the worker had to remember
would have left the obligation open for ever when forgotten, which is why it was not
chosen.

- **The marks** are `inbox/<name>/pending/marks/<epoch>/<time>-<id>`
  (`internal/core/mail/marks.go`), one file per `rewake pending` call, written only under
  the mailbox lock. A mark belongs to the turn it was made in and to no other, and the
  tie is time on the machine's boot clock (`CLOCK_BOOTTIME`, `internal/infra/boottime`), one
  clock for every process that a wall-clock jump after a WSL suspend does not move back.
  The mark records when the `rewake pending` process started; a turn end honors it only
  if that lies in the turn's window: after the later of the turn's start and the latest
  earlier end of the run a journal records, up to the turn's end. Of several marks in the
  window the latest decides, so a second `pending` in one turn replaces the text. A mark
  older than the window was made in an earlier turn whose end never reached rewake — an
  Esc interruption a harness did not report, a mailbox lock not taken in time —
  and decides nothing, so the next turn end is the report. A mark newer than the turn's
  end was made in a later turn while this one was still being published, and is left for
  it. No turn end removes a mark: a retry of an end finds the marks its first attempt
  saw, and the marks go when the run's records are swept after it ends
  ([turn-end-recovery.md](turn-end-recovery.md#pending-marks)). A turn whose start is not
  known cannot be tied to a mark and ends as a report. Every failure falls to that side:
  the behaviour without `pending`.
- **The turn's start and end.** The adapter times both as its harness reports them and
  carries them with the completion, so a turn published late — queued, or retried when
  the mailbox was busy — cannot take a mark made in the turn after it. The start is
  also recorded for the host as the harness reports it (`mail.RecordTurnStart`, from
  the boundary's `TurnHandler.Started`, whether or not the run has a mail tool): each reading a file of its own under the run's
  `awaiting/<epoch>/.turn-starts/`, named by the reading and never overwritten, the
  latest being the largest name, so two writers at once cannot leave the older reading
  on top; a start recorded after a mark's time proves the mark's turn over when its end
  was lost. A directory of readings that cannot be read refuses `rewake pending`. Until
  S9, October 11, 2026, Claude Code's UserPromptSubmit and Stop hooks recorded the same
  through `rewake observe`; in this build no reporting role runs on Claude Code, so no
  mark is made there.
- **The gap that remains.** If a turn end with a mark is lost and the harness then
  starts a turn by itself, reporting no start, and that turn's end is the first one
  heard, its final answer leaves as an interim message and the report does not come.
- **A turn background work woke.** A worker that marked a turn and was then woken by a
  finished subagent or background task must mark that turn too, or its end is the
  report; the briefing says so. An adapter that can hold an end open lets the core ask
  once when the mark is missing ([below](#the-confirmation-on-claude-code)); elsewhere
  the briefing is all there is.
- **Every adapter** takes the same path from there: its completion goes into the one
  function that prepares a turn's reports, which asks for the mark once per turn end.
- **Only a normal finish is softened.** A failed or stopped turn reports `error` or
  `stopped` as it would have, with the mark in its window all the same.
- **The message** has its own report id (the wait's id plus `-pending-<turn>`), like
  `stopped`, so the report that follows is not taken for a copy. A blocked `--question`
  does not take it for its answer: it is a turn outcome for being kept readable when
  its notice fails (`mail.IsReport`), but not one that settles (`mail.Settles`), and
  a question waiting in the sender's session leaves it to be announced. The notice reads
  `Rewake: <session> pending, …` with the mark's text as its preview; Claude Code draws it as
  status `running`, which it has no color for, and Codex marked it with ⏳.
- **Refused** outside a session, for main — whose turns are reported to nobody — without
  text, and when no read task or question in this run waits for a report; each refusal
  says why.

## The confirmation on Claude Code

Owner decision, September 26, 2026: after an interim turn end, rewake holds the next
turn end that carries no mark, once, and asks the session whether the work is done. It
catches the mark forgotten on a turn a finished subagent woke — three tasks were closed
early that way that day — without a marker the worker must remember: the turn end stays
the report by default, as decided on September 23. The section keeps its name from
where it began: Claude Code's Stop hook held the end until S9, October 11, 2026, when
the hooks left ([research-claude-control.md](research-claude-control.md#a-stop-hook-that-holds-the-turn)
has how the harness took the hold). Since S5 the same check runs for any adapter that
can hold an end open, and in this build that is the fixture's; an adapter that cannot
leaves the briefing alone to ask for the mark.

- **How.** The adapter calls the completion handler's `Confirm` in place of `Publish`,
  and `ConfirmCompletion` answers `""` when the end was published, or a reason when it
  was held; the adapter continues the turn with that reason and confirms the next end
  again. Such an end names its event and may be retried, so it is refused without its
  event id and read boundary.
- **When.** An end is held only when all of these are true: the adapter says it may be
  held; it did not fail; this run's last published turn end was interim; a sender whose
  session still runs waits on a task; the ending turn made no mark; and no answer is
  kept from an earlier hold. A sender proven gone — no record, or one of another run —
  is left out of the senders; a sender whose record could not be read fails the check,
  and the end is published to every sender, since the hold would otherwise be asked for
  a set the check never saw. Then it publishes nothing and keeps the turn's answer. The
  reason quotes the pending line, names the senders, and says both ways on: still
  waiting — run `rewake pending` and end the turn; done — end the turn, and the answer
  just given goes as the report with whatever is added after it.
- **Once.** A turn end while an answer is still kept is never held, so a turn is held at
  most once. Each later unmarked turn end after an interim one is asked again: every
  interim end starts a new episode.
- **The records**, beside the mark in `inbox/<name>/pending/` and changed only under
  the mailbox lock: `interim.json` holds the run's last word on the work — the epoch,
  the end's operation and its end time, and either the pending line of an interim end
  or that a finished or failed end settled it; a stop leaves it, as a stop leaves the
  waits. Writing it is a step of the turn end's journal
  ([delivery-turn-end.md](delivery-turn-end.md)), so an end that dies after its journal
  is on record still leaves the interim end on record. Of two ends of one run the later
  by end time wins, whatever order their retries come in; the journal of an ended run,
  completed while the next run takes over, leaves the record of the run holding the name
  alone, and one that cannot be read stops the journal rather than being written over
  ([turn-end-recovery.md](turn-end-recovery.md#the-interim-record)). `kept.json` holds
  the held answer, the held end's operation and its reason.
- **The same end confirmed again** — its answer lost — gets the same reason, and nothing
  is published or kept again; once a continuation took the kept answer, its journal
  names the held end in every form it takes, and the held end confirmed again is
  answered as published ([turn-end-recovery.md](turn-end-recovery.md#a-held-end-confirmed-again)).
- **The report keeps the held answer.** The continuation's end carries only what the
  model said after the hold, so the next turn end heard puts the kept answer into its
  own outcome: a finish gives the kept answer, then the continuation; a pending mark
  made in the continuation gives the mark's line, the kept answer, then the
  continuation; a failure gives its error, then the kept answer; a stop gives its line,
  then the kept answer, with the waits open as after any stop. The answer is dropped
  once that outcome is published, not before, as a step of the turn end's journal
  ([delivery-turn-end.md](delivery-turn-end.md)); each kept answer has its own version,
  and the step drops the version the outcome carried and never a later one.
- **A hold is not an end.** A held end publishes nothing and journals no end of the
  run, so a mark the continuation makes falls within the turn its end takes it from.

## Keyboard stops

A stopped outcome is advisory and comes from the adapter that heard the turn end. It goes to the current waiters
— the senders of read, unsettled tasks and questions — using the text "the person at the
keyboard stopped this turn". With no waiter it goes nowhere: unlike an error, it does not
fall back to main, and main's own interruption puts nothing into its own inbox. A person
interrupting a turn no rewake task depends on is their own business, and the session
turning idle says all there is to say — the owner's decision of
September 23, 2026, the same for every adapter, since the routing
(`internal/cli/turn_reports.go`) is shared. A waiter whose session has ended, or runs
under another epoch, is no waiter any more: a `stopped` meant only for it is dropped too,
where it used to go to main. Its journal is kept all the same, so a repeated event
publishes nothing either.
A turn a main aborted with `rewake interrupt` is stopped the same way,
with the text "<main> interrupted this turn with rewake interrupt" instead, and the same
routing. The interrupted session itself is told nothing more: the line its next notice
carried on Claude Code left with the plugin in S9, October 11, 2026
([remote-control.md](remote-control.md)).
It owes no reply and has its own report id, separate from the eventual result.
Its own operation, and journal, keep the original waits intact. A finished
or error outcome for the same native turn is an operation of its own and can settle
those waits; retries of either outcome remain idempotent. Human continuation can then
publish finished with the same inReplyTo and settle those waits. Main waits for
the person instead of resending. A waiting question prints stopped and exits 1;
the later result remains an ordinary inbox report. Socket notices use killed
status; server-delivered text notices use a yellow circle.

**A run that passed unseen** (Codex, September 25, 2026, main's decision): the gateway
saw the conversation go active and then idle but never learned which turn ran, so it
cannot tell work from a compaction. It publishes a `stopped` with the text "a run of
this conversation passed unseen by rewake; whether it did your task is not known here",
routed and kept like a keyboard stop: current waiters only, waits intact, the next
finished settling the task. Why it cannot settle anything is in
[codex-publication.md](archive-1.x/codex-publication.md).

**A turn that ended unproven** (Codex, September 25, 2026, main's decision in round 8):
a turn with no reply naming it and no item but a compaction's — a goal's turn failing
at its first model call, or one whose reply was lost with the connection — is reported
the same way, with the text "a turn of this conversation ended without rewake seeing
what it did", followed by its error text or that it was stopped, under an identity of
its own: the turn's id with `/advisory` after it. If the proof comes later, the turn's
own outcome follows, of any kind, a stop included, with its start and end, and is
handled as any turn end: a finish or an error settles the task, and a pending mark
made during the turn is taken by it, never by the advisory. A turn whose only item is
`contextCompaction` is a compaction and reports nothing.

**Where a stop comes from.** The adapter that hears the turn end says it was
interrupted, and every adapter's stop reaches the same publication with the same text,
notice and kept waits; the next turn end that finishes settles the task — owner
decision, September 23, 2026: harnesses do not diverge here. In 1.x Codex's gateway saw
`turn/completed` with status `interrupted`, and Claude Code's stop came from rewake's
function-hooks plugin, reporting `turn.complete` with reason `aborted`
([claude-plugin.md](archive-1.x/claude-plugin.md)); the plugin left in S9, October 11,
2026, and in this build the fixture's program is the one that reports a stop.

Answer receipts store the report id in received/<question id>. Only an exact
match confirms that report; stopped does not acknowledge a later finished with
the same inReplyTo. Empty legacy receipts are unqualified and do not confirm a
new outcome. Their existing reference-based retention and expiry still apply.
