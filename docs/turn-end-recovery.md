# Recovering a turn end

Rules 7 and 8 of [rules/effects.md](rules/effects.md) say what recovery may do; this
document says how, for every record the code reads (`internal/core/mail/turn_reports.go`,
`journal.go`, `interim.go`, `kept.go`, `read_boundary.go`);
[delivery-turn-end.md](delivery-turn-end.md) walks the same path through the code. It was
written on September 30, 2026 for the turn-end journals, and rewritten on October 18,
2026, when the owner replaced the journals, the barrier, the stops and the operator
decision with a simple turn end: under the sender's mailbox lock it writes its reports,
then clears the waits they answer, takes the kept answer and records the interim end, and
last records itself done. Rule clauses are cited as 7-identity, 7-scope, 7-clock and 8.
An owner decision of September 30 still shapes it: a hold takes a place on the read clock
so that a stop after an Esc keeps its held answer (7-clock). How a mailbox passed from the builds before the
journal to it — the cutover, the conversion of their receipts and `rewake settle` — was
removed in stage 3 of 2.0 and is kept in
[archive-1.x/protocol-cutover.md](archive/1.x/protocol-cutover.md).

## The operation

A turn end is named by its run and its event (7-identity). Its done record,
`journal/<op>.done`, is named from that, so a retry that finds it finds its own; its
reports are named by the obligations they answer and, for a report that closes nothing,
by the operation in the id's tail, found again in the recipient's mailbox on a retry.

Its scope (7-scope) comes from the event, fixed by the event's form:

| Event | Where its scope comes from | Retry |
|---|---|---|
| A hook that names no event (Claude Code's Stop and StopFailure) | the state at its one attempt, under the mailbox lock: this run's waits, this run's kept answer, the pending marks in its turn's window | none: it is heard once |
| A completion with an id and a read boundary (a Codex completion, the plugin's stop after an Esc, an end an adapter confirms) | the event: waits read at or below `Through`, a kept answer held at or below `Through`, pending marks in its window (pending marks), and `Ended` for its interim record | the same event, the same scope |
| An id without a boundary (a Codex notify payload through `turn-ended`; rewake installs none) | none | refused on every attempt: no effect, and its waits stay owed for the next end |

The third row is refused rather than scoped by a receipt: a receipt written on a retry,
after the first attempt's write failed, would fix the scope of the moment it was
written — questions read since, an answer kept since — and nothing in it could tell
that retry from a first attempt.

### A held end confirmed again

A hook's first call is heard once, so any end after a hold is its continuation. An end
an adapter confirms names its event and may be retried: if the answer to its hold is
lost and the same end comes again, it finds no done record, since a hold writes none.
So a hold records, in the one write of `kept.json`, the held end's operation (`held`)
and the reason it was answered (`reason`), and the done record of the end that takes the
kept answer names that operation (`Held`). Under the mailbox lock, before the hold
check, an end that names its event is answered from the record:

- a done record of its own operation — published before;
- the kept answer's `held` naming it — the same reason again, nothing published and
  nothing kept again;
- a done record that took the kept answer naming it — published before, with its
  continuation.

Only an end of another identity takes the kept answer. Done records stay while their
run lives, and retries come only from that run. A done record that cannot be read may be
the one that took the kept answer, so the lookup fails and the end reports nothing on
that attempt. The third check is the one a continuation that marked
pending needs: it leaves the task owed and the run interim, so without it the held end
confirmed again would be held, or published, a second time.

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

A crash after `kept.json` is written and before the commit leaves the hold's position
durable and the word below it, and a clock opened again recovers from the waits only; a
continuation with no read between would then capture a boundary below the kept answer
and leave it out. So a held end confirmed again first raises the word to the position
its kept answer records, when the word is below it: no position is allocated, the answer
is not rewritten, and a word already at or above it stays, so no boundary captured for
another end is widened.

The word is a counter of the state (`state.Counter`): a file of one word mapped shared,
whose stores and flush are writes of the room behind the write seam's guard, and whose
load touches neither the filesystem nor a lock. A store the guard refuses leaves the word
where it was, the same as a crash before the commit.

An end with a boundary takes the kept answer only when its position is at or below
`Through`, and drops it only while it is still that version. A hook heard once takes
this run's kept answer as it finds it: nothing can be kept between its reading and its
effects, which share the lock.

## Pending marks

A mark is one file per `rewake pending` call, named by its run, the boot-clock time
`At` its process started and a random id, so no two calls share a name and a later mark
never writes over an earlier one. A call that cannot read its start time records no
mark and says so: a mark with no `At` would belong to no turn. No turn end removes a
mark (7-scope). Removing was what let a later pending end destroy the evidence an
earlier end's retry needed: E1 is marked P1, its attempt fails; E2 is marked P2 and,
removing P1 as stale, leaves E1's retry to find no mark and close as finished a turn
that said pending. A mark therefore stays for its run's life, since only that run
retries its ends, and goes when the run's records are swept. Being used is no effect.

| Mark | Recovery |
|---|---|
| this run's, `At` in the end's window | the latest by `At`, then by name, gives the pending line; ties do not depend on the order of retries |
| this run's, `At` outside the window | another turn's; not this end's |
| another run's | not this end's; swept with that run's records |
| unreadable, its name in the window | the end fails on that attempt: it may be this turn's |

An end's window runs from just after its start to its `Ended`, inclusive. Its start is
the later of the turn's start the event carries and the latest `Ended` of this run that
a done record keeps below its own; with neither, the run's start. A done record keeps its
`Ended` while it is kept, so moving the start past an end is the last step of that end's
operation, not a later write. No assumption
that windows never overlap is needed. The hook path records the next turn's start only
after an end is published, by a write that can fail silently, and a native completion's
times are when the gateway observed its events: with that stale start alone, E2 would
take P1, already E1's, and so would every end after it. With E1's done record, E2's
window opens after E1. A mark at the very start is the turn before's, since the start comes
before anything its turn runs; one at the very end is this turn's, since the agent's
call precedes the end it announces. The same window decides the hold.

A retry is told from a new end by its event, the same run and event with the same
`Started` and `Ended`. A retry whose done record is there is answered from it; one
without it decides as a first attempt, where a done record written since can only narrow
the window to its own turn. Windows still overlap when an end is heard before an earlier
one of its run is recorded done — a late completion, a lost write. A mark in both makes
both interim, and the end after both is narrowed by both records, so it does not
repeat. That is the safe side: a mark only makes an end
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
| this run's, same operation | done: a retry after the write, before the end was recorded done |
| this run's, ended earlier | write over it (not done) |
| this run's, ended later — a later interim end, or a settling end that came first | superseded: the later end is the last word |
| this run's, the same `Ended`, another operation | fixed without regard to the order of retries: interim wins over settled, since it only makes the next unmarked end ask once, and between two of a kind the operation whose name sorts last |
| unreadable | the end fails on that attempt, and names the path |

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
record, so the record in place stays for the next end heard; and its done record keeps
no `Ended`, so it moves no later window. Without a boundary it is refused as any id
without one is.

## The evidence for each effect

| Effect | Done | Not done | Unreadable |
|---|---|---|---|
| A report | a letter of its id in the recipient's mailbox, in any stage; or every task of its wait answered by a finished or failed report of the name there | neither | the stage that cannot be searched fails the attempt: a second copy is the outcome that cannot be taken back |
| Clearing a wait | the messages gone from the wait record | the messages still there | the wait record fails the attempt |
| Taking the kept answer | absent, or holding another version | the version carried still in place | the record fails the attempt |
| Recording the interim end | by the interim table | by the interim table | the record fails the attempt |
| The end recorded done | `journal/<op>.done` | absent | the record fails the lookup, and the end reports nothing on that attempt |

## Reconciliation

Removed on October 18, 2026, with the journals it completed. Nothing runs before a turn
end reads a wait: an end that failed part way left its wait standing and nothing else to
complete, and its retry, or the next end, runs the steps again (8). Adoption looks for the
answer itself: a run that resumes a conversation does not take over a task that already
has a finished or failed report of the name in the sender's mailbox
(`internal/core/mail/answered.go`), since the run before it may have died between writing
the report and clearing the wait.

## The stop

Removed on October 18, 2026: a record that cannot be read fails the operation that met
it, and stops nothing else.

## Every record and state

| Record | State | Recovery |
|---|---|---|
| Done record | absent | the end runs its steps from the disk |
| Done record | present, its operation | answered from it; kept while the run lives |
| Done record | `Held` naming the end confirmed | answered as published with its continuation |
| Done record | unreadable | the end fails on that attempt |
| `kept.json` | this run's, at or below the boundary | taken, and dropped while still the version carried |
| `kept.json` | this run's, above the boundary | left for a later end |
| `kept.json` | this run's, `held` naming the end confirmed | the same reason again; the word raised to its position |
| `kept.json` | another run's | never taken; written over by the next hold |
| `kept.json` | unreadable | the end fails, and nothing writes over it |
| Pending mark | any | as the pending table |
| `interim.json` | any | as the interim table |
| Wait record | unreadable | the end fails, and nothing writes over it |
| Report letter | read and older than a day, its writer's wait still naming what it answers | kept by the sweep, so a late retry finds it |

## What stays open

- The answer of a held turn whose run ended is never published: if that run's end never
  published it either, the tasks are still answered by the next run's end, without that
  text.
- An end with an id and no boundary reports nothing; its waits stay owed for the next
  end with a boundary or heard once.
- An end heard before an earlier end of its run is recorded done may share a mark with
  it: both are interim, once (pending marks).
- A process killed in the milliseconds between two steps of an end can cost a task shown
  once more as owed until the next end, a lost report main asks for again, or a kept
  answer after an interruption carried by the next turn's report (the owner, October 17
  and 18, 2026).
