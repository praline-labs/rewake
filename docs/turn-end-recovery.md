# Recovering a turn end

Rules 7 and 8 of [mail-bridge-cli.md](mail-bridge-cli.md) say what recovery may do; this
document says how, for every record the code reads. It was written on September 30,
2026, before the code that follows it, which landed the same day
(`internal/inbox/journal.go`, `reconcile.go`, `interim.go`, `kept.go`,
`read_boundary.go`, `internal/cli/turn_reports.go`);
[delivery-turn-end.md](delivery-turn-end.md) walks the same path through the code. Rule
clauses are cited as 7-identity, 7-scope, 7-clock, 7-journal, 8-evidence, 8-stop,
8-proof, 8-retention and 8-reconcile, in the order the rules state them. Two owner
decisions of the same day shape it: an unknown that no look can settle stops the whole
mailbox (8-stop), and a hold takes a place on the read clock so that a stop after an
Esc keeps its held answer (7-clock). How a mailbox passed from the builds before the
journal to it — the cutover, the conversion of their receipts and `rewake settle` — was
removed in stage 3 of 2.0 and is kept in
[archive-1.x/protocol-cutover.md](archive-1.x/protocol-cutover.md).

## The operation

A turn end is named by its run and its event (7-identity). Its journal is named from
that, so a retry that finds it finds its own; its reports are named by the obligations
they answer and, for a report that closes nothing, by the operation; a notice about it
is named by the operation.

Its scope (7-scope) comes from the event, fixed by the event's form:

| Event | Where its scope comes from | Retry |
|---|---|---|
| A hook that names no event (Claude Code's Stop and StopFailure) | the state at its one attempt, under the mailbox lock: this run's waits, this run's kept answer, the pending marks in its turn's window | none: it is heard once |
| A completion with an id and a read boundary (a Codex completion, the plugin's stop after an Esc) | the event: waits read at or below `Through`, a kept answer held at or below `Through`, pending marks in its window (pending marks), and `Ended` for its interim record | the same event, the same scope |
| An id without a boundary (a Codex notify payload through `turn-ended`; rewake installs none) | none | refused on every attempt: no effect, and its waits stay owed for the next end |

The third row is refused rather than scoped by a receipt: a receipt written on a retry,
after the first attempt's write failed, would fix the scope of the moment it was
written — questions read since, an answer kept since — and nothing in it could tell
that retry from a first attempt.

## The read clock

Every read and every hold of a run takes the next position on the run's read clock
(7-clock), under the mailbox lock and in one order: reserve the position in the durable
high-water record, write the record that carries it (the wait with its `Seq`, or
`kept.json` with its `Seq` beside the version), then commit the position to the word
that completions capture their boundary from. So:

- a boundary never covers a position whose record is not yet written, since the commit
  comes after the write;
- a position is never issued twice: a reserve that was followed by a failed write
  leaves a gap, and a restart resumes above the high-water;
- a kept answer held after a boundary was captured is above it, and belongs to a later
  end even on the first attempt of this one.

An end with a boundary takes the kept answer only when its position is at or below
`Through`, and only by the version it names in the journal. A hook heard once takes
this run's kept answer as it finds it: nothing can be kept between its reading and its
journal, which share the lock.

## Pending marks

A mark is one file per `rewake pending` call, named by its run, the boot-clock time
`At` its process started and a random id, so no two calls share a name and a later mark
never writes over an earlier one. A call that cannot read its start time records no
mark and says so: a mark with no `At` would belong to no turn. No turn end removes a
mark (7-scope). Removing was what let a later pending end destroy the evidence an
earlier end's retry needed: E1 is marked P1, its journal write fails; E2 is marked P2
and, removing P1 as stale, leaves E1's retry to find no mark and close as finished a
turn that said pending. A mark therefore stays for its run's life, since only that run
retries its ends, and goes when the run's records are swept. Being used is no effect:
the journal of the end records the name and line of the mark that decided it.

| Mark | Recovery |
|---|---|
| this run's, `At` in the end's window | the latest by `At`, then by name, gives the pending line; ties do not depend on the order of retries |
| this run's, `At` outside the window | another turn's; not this end's |
| another run's | not this end's; swept with that run's records |
| unreadable, its name in the window | the end stops (8-stop): it may be this turn's |

An end's window runs from just after its start to its `Ended`, inclusive. Its start is
the later of the turn's start the event carries and the latest `Ended` of this run that
a journal records below its own; with neither, the run's start. Every journal of this
build, a done one too, keeps its `Ended` while it is kept (8-retention), so moving the
start past an end is a step of that end's operation, not a later write. No assumption
that windows never overlap is needed. The hook path records the next turn's start only
after an end is published, by a write that can fail silently, and a native completion's
times are when the gateway observed its events: with that stale start alone, E2 would
take P1, already E1's, and so would every end after it. With E1's journal, E2's window
opens after E1. A mark at the very start is the turn before's, since the start comes
before anything its turn runs; one at the very end is this turn's, since the agent's
call precedes the end it announces. The same window decides the hold.

A retry is told from a new end by its event, the same run and event with the same
`Started` and `Ended`. A retry whose journal is there takes the mark its journal names;
one without it took no effect and decides as a first attempt, where a journal written
since can only narrow the window to its own turn. Windows still overlap when an end is
heard before an earlier one of its run has a journal — a late completion, a lost
journal write. A mark in both makes both interim, and the end after both is narrowed by
both journals, so it does not repeat. That is the safe side: a mark only makes an end
interim, and taking one twice delays a close, where missing it closes a task the agent
said goes on.

## The interim record

`interim.json` holds this run's last word on whether the work goes on: the operation
that wrote it, its `Ended` on the boot clock, and either the pending line or `settled`.
A settling end (finished or failed) writes `settled` rather than removing the record,
so a late end cannot bring back a line an end after it settled; a stop writes nothing,
as it clears no waits. An end writes its record only when the one in place is absent,
another run's, or ended earlier than it did:

| In place | Recovery |
|---|---|
| absent, or another run's | write (not done) |
| this run's, same operation | done: a retry after the write, before the journal step was marked |
| this run's, ended earlier | write over it (not done) |
| this run's, ended later — a later interim end, or a settling end that came first | superseded: the later end is the last word |
| this run's, the same `Ended`, another operation | fixed without regard to the order of retries: interim wins over settled, since it only makes the next unmarked end ask once, and between two of a kind the operation whose name sorts last |
| unreadable | the end stops (8-stop), and names the path |

`Ended` is compared only within one run: a boot clock says nothing across runs or
restarts of the machine, and a record of another run is written over. An end whose
`Ended` is not known has no place among its run's ends, and that is an ordinary case:
the gateway reports three outcomes with no times on purpose — a turn that ended
unproven (its id with `/advisory`), a run that passed unseen, a completion not observed
([turn-outcomes.md](turn-outcomes.md)). Such an end keeps what main decided for them:
it is published under its own identity within its read boundary, a stop to the current
waiters with their waits kept, an error clearing the waits it answers, and a waiting
question exits 1 on the stop. What it lacks it does not invent: it takes no pending
mark, since only a finish is softened by one; it neither writes nor settles the interim
record, so the record in place stays for the next end heard; and its journal records no
`Ended`, so it moves no later window. Without a boundary it is refused as any id
without one is.

## The evidence for each effect

| Effect | Proven done | Proven not done | Unknown |
|---|---|---|---|
| A report of the journal protocol | listed in the journal; `published` mark; letter present | no mark and no letter; `intent` mark and no letter | a mark that cannot be read or says neither (a later look) |
| Taking the kept answer | absent, or holding another version, after the journal named this version | the version named still in place | a record that cannot be read |
| Clearing a wait | the messages gone from the wait record | the messages still there | a wait record that cannot be read |
| Recording the interim end | by the interim table | by the interim table | a record that cannot be read |

A report found with its letter and without a mark is recorded in the sender's journal
first, and marked `published` for the recipient after (8-proof). A failure of the
journal write leaves the letter as the only proof, and the attempt stops; if the sweep
takes the letter before the next look, the report is unknown for good, never unsent.
A failure of the mark after the journal write leaves the journal's entry as the proof,
and the sweep changes nothing.

## Reconciliation

The barrier runs under the mailbox lock before a turn end reads a wait and before
adoption takes one over (8-reconcile). In order:

1. Plan before any effect: read every file of the tree against the list of record
   kinds, then run the remaining steps below as a plan that writes nothing, so what each
   decides by is read first ([mailbox-records.md](mailbox-records.md#the-plan-and-the-seam)).
   One that cannot be read stops the mailbox (8-stop). The same plan gates every other
   call that would change the mailbox, and the stop is recorded, so a stop found late
   lets no effect through.
2. Complete every unfinished journal in turn. Each report the journal has not
   settled yet is established from the evidence above and settled: published when its
   recipient's run is the one it was addressed to, recorded moot when that run has ended
   or was replaced, since it can reach nobody. The journal is saved after each report,
   so an earlier report may be out before a later one is found moot.
3. Then the journal's own steps, named when it was written: take the kept answer by
   version, clear the waits it names, write the interim record, mark done.

Journals never overlap each other: an end reads only the waits the barrier left, and
the barrier completes every journal before that. So no report answers an obligation
another journal's report closed, and the barrier collects no closed obligations across
the mailbox and decides no report superseded; the 1.x conversion needed both only
because it turned overlapping records into journals. A done journal keeps no more than
its done mark and its `Ended`, which opens the next end's window (pending marks).

## The stop

A stopped mailbox (8-stop) keeps every record. Its session reads, reports and clears
nothing, and adoption takes nothing over: each of its hooks and calls that would change
the mailbox answers the exact cause instead, and a hook exits so the harness goes on,
never blocking a stop. Letters from others still arrive and wait unread, since none
decides an effect, and recovery writes its own record of the stop. The stop names the
path that could not be read or parsed; it is kept on record, and goes as
[mailbox-records.md](mailbox-records.md#the-stop-on-record) says. Today a stop the
reading found lifts once a later reading finds its cause gone, even when the cause went
by being removed rather than by its evidence returning; a stop only an effect met holds
until a barrier has run every effect through. Main is not told of a stop, and a stop
resolved only by returning evidence is not built yet: both are gaps of E8 that S3 of
stage 3 closes ([rules/effects.md](rules/effects.md)).

## Every record and state

| Record | State | Proven | Unknown | Recovery |
|---|---|---|---|---|
| Journal | absent | nothing was done | — | a completion prepares from its event; a hook from its one attempt |
| Journal | unfinished | what it names | per effect, by the evidence table | reconcile, then complete |
| Journal | done | complete, and its `Ended` | — | nothing; kept while the run lives |
| Journal | unreadable | — | everything | the mailbox stops |
| `kept.json` | this run's, at or below the boundary | its version and position | — | taken by the version the journal names |
| `kept.json` | this run's, above the boundary | a later hold | — | left for a later end |
| `kept.json` | another run's | an ended run's | whether it was ever published | never taken; written over by the next hold |
| `kept.json` | unreadable | — | its owner | the mailbox stops before any effect, and nothing writes over it |
| Pending mark | any | as the pending table | — | as the pending table |
| `interim.json` | any | as the interim table | — | as the interim table |
| Once mark | none, intent, published | as the evidence table | — | as the evidence table |
| Once mark | other, unreadable, or behind a path that cannot be followed | — | the publication | the mailbox stops before any effect, naming the path |
| Wait record | unreadable | — | its messages | the mailbox stops |

## What the reviews found

Which clause closes each finding of the earlier acceptances and reviews, and what the
earlier probes now expect of the same bytes, is in
[turn-end-recovery-findings.md](turn-end-recovery-findings.md).

## What stays open

- The answer of a held turn whose run ended is never published: if that run's end never
  published it either, the tasks are still answered by the next run's end, without that
  text.
- An end with an id and no boundary reports nothing; its waits stay owed for the next
  end with a boundary or heard once.
- An end heard before an earlier end of its run has a journal may share a mark with it:
  both are interim, once (pending marks).
