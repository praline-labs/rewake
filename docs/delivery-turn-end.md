---
description: The end of a turn: its steps under the sender's lock, its scope and the read clock, pending marks, the interim record, a held end, and what a retry finds.
---

# Delivery: turning a turn's end into a report

How the end of a reader's turn becomes its report: the steps it takes under the sender's
mailbox lock, the records it reads and writes, and what a retry finds for each. The rules it
keeps are E7 and E8 of [rules/effects.md](rules/effects.md); the turn ends that are not an
ordinary report — a failure, a stop, a `rewake pending` — are in
[turn-outcomes.md](turn-outcomes.md). The code is `internal/core/mail/turn_reports.go`,
`journal.go`, `interim.go`, `kept.go` and `read_boundary.go`.

## The end of a turn

The harness's adapter hands each turn's end to its wrapper, which passes it to the internal
reporting function (`ReportCompletion`, or `ConfirmCompletion` for an end the adapter can hold
open, in `internal/cli/completion.go`) with the session's epoch, the thread the adapter
names and the read boundary it captured. A subagent's turn is not reported. Under the
mailbox lock, for every eligible run in `awaiting/<own epoch>/`, the end leaves a `finished`
message whose text is the turn's last reply and whose `inReplyTo` lists the messages read
from that run since its last report, addressed to that run — not to whoever holds the name
now.

Under the sender's mailbox lock the end runs its steps in one order (`publishTurnContext` in
`internal/core/mail/turn_reports.go`): it reads the scoped waits, the window's pending mark
and the hold ([the operation](#the-operation)); for each waiter whose run is still the live
run of its name it writes the report into the recipient's mailbox unless a letter of that id
is there in any stage (`writeTurnReport`); then it clears the recorded message ids of each
wait — later messages remain owed to the next result — drops the kept answer only when it is
still the version carried, and records the interim end, never over a record of the same run
that ended later, nor over the record of another run that holds the name
(`internal/core/mail/interim.go`). Last it writes `journal/<op>.done`
(`internal/core/mail/journal.go`): the operation, its end time and the held end it took. A
retry of the same event finds it and answers from it; the next end's window opens after it.
A done journal is kept while its run lives; the live run's sweep removes those of ended runs
a day later.

Writing the report before clearing the wait is the order that matters: a cleared wait counts
as answered (`internal/core/mail/awaited.go`), so the other order, cut short, would leave an
answer on record and no letter. A failure returns at the step it met, and a retry of the
same event runs the steps again from what the disk says — the wait still names what is owed,
the report's id is fixed by the wait, and a report found written is not written again.
Killed before the report is written, the end leaves the wait standing, and the sender reads
the task as owed or gone (`rewake inbox --awaited`). Killed after, the letter is with the
sender already. Two narrow holes are closed in the code:

- the sweep of a recipient keeps a report whose writer's wait still names what it answers
  (`reportStillAwaited` in `internal/core/mail/sweep.go`), so a retry that comes after a day
  still finds the letter;
- a report is not written when every task of its wait already has a finished or failed
  report of the name in the sender's mailbox (`internal/core/mail/answered.go`): the run died
  between writing the report and clearing the wait.

A failure report to main, which answers no wait, is found again by its operation in its id's
tail on a retry (`reportOfOp`). The report's id is derived from the wait — the reporting run,
the waiting run, when the wait began, and its message ids. A read retried after its last step
failed does not record the wait a second time: the `read` status, written after the wait,
says it is a retry. The recipient's wrapper announces the report through the same notice as
any message: `Rewake: write-claude finished, 1 new message`.

Two rules keep this from turning into a loop:

- reading finished, error, stopped, pending or notify asks for nothing back;
- each waiting run is told once; a new run of the name starts with no waiters.

So an exchange ends: A writes to B; B reads, answers, ends its turn and reports to A; A reads
the report, which asks for nothing, and the exchange is over. A message to a waiting run is
never taken for the answer, since it may be a new request or a note sent mid-turn.

## The operation

A turn end is one operation, named by its cause: a hash of the run, the event's id, its kind
and its start and end on the boot clock (`turnOp` in `internal/core/mail/turn_reports.go`).
Its done record, `journal/<op>.done`, is named from that, so a retry that finds it finds its
own; its reports are named by the obligations they answer and, for a report that closes
nothing, by the operation in the id's tail.

Its scope comes from the event:

| Event | Where its scope comes from | Retry |
|---|---|---|
| An end with its event and a read boundary | the event: waits read at or below `Through`, a kept answer held at or below `Through`, pending marks in its window, and `Ended` for its interim record | the same event, the same scope |
| An end without its event, its boundary or a known reason | none | refused on every attempt (`TurnEnd.Check`): no effect, and its waits stay owed for the next end |

The second row is refused rather than scoped later: a scope fixed on a retry, after the first
attempt's write failed, would take in questions read and answers kept since, and nothing could
tell that retry from a first attempt.

### A held end confirmed again

An end an adapter confirms names its event and may be retried: if the answer to its hold is
lost and the same end comes again, it finds no done record, since a hold writes none. So a
hold records, in the one write of `kept.json`, the held end's operation (`held`) and the
reason it was answered (`reason`), and the done record of the end that takes the kept answer
names that operation (`Held`). Under the mailbox lock, before the hold check, an end that
names its event is answered from the record:

- a done record of its own operation — published before;
- the kept answer's `held` naming it — the same reason again, nothing published and nothing
  kept again;
- a done record that took the kept answer naming it — published before, with its
  continuation.

Only an end of another identity takes the kept answer. A done record that cannot be read may
be the one that took the kept answer, so the lookup fails and the end reports nothing on that
attempt. The third check is the one a continuation that marked pending needs: it leaves the
task owed and the run interim, so without it the held end confirmed again would be held, or
published, a second time.

## The read clock

Every read and every hold of a run takes the next position on the run's read clock, under the
mailbox lock and in one order: reserve the position in the durable high-water record, write
the record that carries it (the wait with its `Seq`, or `kept.json` with its `Seq` beside the
version), then commit the position to the word that completions capture their boundary from.
So:

- a boundary never covers a position whose record is not yet written, since the commit comes
  after the write;
- a position is never issued twice: a reserve followed by a failed write leaves a gap, and a
  restart resumes above the high-water;
- a kept answer held after a boundary was captured is above it, and belongs to a later end
  even on the first attempt of this one.

A crash after `kept.json` is written and before the commit leaves the hold's position durable
and the word below it, and a clock opened again recovers from the waits only; a continuation
with no read between would then capture a boundary below the kept answer and leave it out.
So a held end confirmed again first raises the word to the position its kept answer records,
when the word is below it: no position is allocated, the answer is not rewritten, and a word
already at or above it stays, so no boundary captured for another end is widened.

The word is a counter of the state (`state.Counter`): a file of one word mapped shared, whose
stores and flush are writes of the room behind the write seam's guard, and whose load touches
neither the filesystem nor a lock. A store the guard refuses leaves the word where it was,
the same as a crash before the commit.

An end with a boundary takes the kept answer only when its position is at or below `Through`,
and drops it only while it is still that version.

## Pending marks

A mark is one file per `rewake pending` call, `pending/marks/<run>/<time>-<id>`
(`internal/core/mail/marks.go`), named by its run, the boot-clock time `At` its process
started and a random id, so no two calls share a name and a later mark never writes over an
earlier one. A call that cannot read its start time records no mark and says so: a mark with
no `At` would belong to no turn. No turn end removes a mark: removing one would let a later
pending end destroy the evidence an earlier end's retry needs. A mark stays for its run's
life, since only that run retries its ends, and goes when the run's records are swept.

| Mark | Recovery |
|---|---|
| this run's, `At` in the end's window | the latest by `At`, then by name, gives the pending line; ties do not depend on the order of retries |
| this run's, `At` outside the window | another turn's; not this end's |
| another run's | not this end's; swept with that run's records |
| unreadable, its name in the window | the end fails on that attempt: it may be this turn's |

An end's window runs from just after its start to its `Ended`, inclusive. Its start is the
later of the turn's start the event carries and the latest `Ended` of this run that a done
record keeps below its own (`TurnWindowStart`); with neither, the run's start. A done record
keeps its `Ended` while it is kept, so moving the start past an end is the last step of that
end's operation, not a later write. A mark at the very start is the turn before's, since the
start comes before anything its turn runs; one at the very end is this turn's, since the
agent's call precedes the end it announces. The same window decides the hold.

A retry is told from a new end by its event: the same run and event with the same `Started`
and `Ended`. A retry whose done record is there is answered from it; one without it decides
as a first attempt, where a done record written since can only narrow the window to its own
turn. Windows still overlap when an end is heard before an earlier one of its run is recorded
done. A mark in both makes both interim, and the end after both is narrowed by both records,
so it does not repeat. That is the safe side: a mark only makes an end interim, and taking one
twice delays a close, where missing it closes a task the agent said goes on.

## The interim record

`interim.json` holds this run's last word on whether the work goes on: the operation that
wrote it, its `Ended` on the boot clock, and either the pending line or `settled`. A settling
end (finished or failed) writes `settled` rather than removing the record, so a late end
cannot bring back a line an end after it settled; a stop writes nothing, as it clears no
waits. An end writes its record only when the one in place is absent, another run's, or ended
earlier than it did:

| In place | Recovery |
|---|---|
| absent, or another run's | write (not done) |
| this run's, same operation | done: a retry after the write, before the end was recorded done |
| this run's, ended earlier | write over it (not done) |
| this run's, ended later — a later interim end, or a settling end that came first | superseded: the later end is the last word |
| this run's, the same `Ended`, another operation | fixed without regard to the order of retries: interim wins over settled, since it only makes the next unmarked end ask once, and between two of a kind the operation whose name sorts last |
| unreadable | the end fails on that attempt, and names the path |

`Ended` is compared only within one run: a boot clock says nothing across runs or restarts of
the machine, and a record of another run is written over.

## What a retry finds

| Effect | Done | Not done | Unreadable |
|---|---|---|---|
| A report | a letter of its id in the recipient's mailbox, in any stage; or every task of its wait answered by a finished or failed report of the name there | neither | the stage that cannot be searched fails the attempt: a second copy is the outcome that cannot be taken back |
| Clearing a wait | the messages gone from the wait record | the messages still there | the wait record fails the attempt |
| Taking the kept answer | absent, or holding another version | the version carried still in place | the record fails the attempt |
| Recording the interim end | by the interim table | by the interim table | the record fails the attempt |
| The end recorded done | `journal/<op>.done` | absent | the record fails the lookup, and the end reports nothing on that attempt |

By record:

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
| Wait record | unreadable | the end fails, and nothing writes over it |
| Report letter | read and older than a day, its writer's wait still naming what it answers | kept by the sweep, so a late retry finds it |

## Reports after a thread change

A harness can change conversations inside one wrapper run, as a person's `/new` or `/clear`
does. Before an owed message is made readable, the wrapper records the selected thread in
`inbox/<name>/threads/<message id>` under the mailbox lock, as its adapter names it (a
`ThreadSource`, `internal/adapter/thread.go`); pi's names it, and an adapter that names none —
Claude Code's — records nothing. The context survives reads and status rewrites, and is kept
while the message is queued, unread or included in an unsettled wait.

At the end of a turn the current thread comes from the end the adapter hands the wrapper. If
any known delivery thread in the wait's message ids differs, the report carries
`threadChanged: true`; otherwise the field is omitted, including when either identity is
unknown. This is advisory, not a change to report routing or the wrapper epoch: a new
conversation does not clear waits or resend tasks. `rewake inbox` prints this line beneath a
marked report:

```
Rewake: the reader's thread changed after delivery; this may not answer it, resend the message
```

A waiting `send --question` also prints the warning and keeps the boolean in its JSON result.
The caller decides whether to resend.

## Known limits

- The answer of a held turn whose run ended is never published. A later turn of the same run
  answers its tasks, without that text; a new run of the name does not, since it takes over
  none of an earlier run's waits, and main sees those tasks as lost
  ([delivery-owed.md](delivery-owed.md)).
- An end with an id and no boundary reports nothing; its waits stay owed for the next end.
- An end heard before an earlier end of its run is recorded done may share a mark with it:
  both are interim, once.
- A process killed in the milliseconds between two steps of an end can cost a task shown once
  more as owed until the next end, a lost report main asks for again, or a kept answer after
  an interruption carried by the next turn's report.
