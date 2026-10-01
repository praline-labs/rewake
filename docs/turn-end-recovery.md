# Recovering a turn end

Rules 7 and 8 of [mail-bridge-cli.md](mail-bridge-cli.md) say what recovery may do; this
document says how, for every record the code reads. It was written on September 30,
2026, before the code that follows it, which landed the same day
(`internal/inbox/journal.go`, `reconcile.go`, `conversion.go`, `interim.go`, `kept.go`,
`read_boundary.go`, `internal/cli/turn_reports.go`, `settle.go`);
[delivery-turn-end.md](delivery-turn-end.md) walks the same path through the code. Rule
clauses are cited as 7-identity, 7-scope, 7-clock, 7-journal, 8-evidence, 8-stop,
8-settle, 8-withheld, 8-proof, 8-retention, 8-reconcile, 8-cutover and 8-origin, in the
order the rules state them. Three owner decisions of the same day shape it: a session
of an earlier build stays on that build's protocol until it is restarted (8-cutover),
an unknown that no look can settle stops the whole mailbox until `rewake settle`
(8-stop, 8-settle), and a hold takes a place on the read clock so that a stop after an
Esc keeps its held answer (7-clock).

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
without one is. An earlier build's record, which carries no operation or time, is
another run's by the time this build reads it (8-cutover).

## The evidence for each effect

| Effect | Proven done | Proven not done | Unknown |
|---|---|---|---|
| A report of the journal protocol | listed in the journal; `published` mark; letter present | no mark and no letter; `intent` mark and no letter | a mark that cannot be read or says neither (a later look) |
| A report of the earlier build | letter present; the receipt's done mark lists it; recorded in the sender's journal; `published` mark; settled delivered | settled undelivered | no letter, no mark and no entry (irrecoverable) |
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
   One that cannot be read stops the mailbox (8-stop), and so does a wait of a run of
   this build without its place on the read clock, which only an earlier-build writer
   still running could have written (8-cutover). The same plan gates every other call
   that would change the mailbox, and the stop is recorded, so a stop found late lets no
   effect through.
2. If earlier-build receipts are there, convert them all into one conversion journal,
   written at once with a decision for every report, before any of them is removed
   (8-origin). They are all of ended runs (8-cutover), so no writer adds to them. A
   receipt the conversion journal already holds byte for byte was left by a conversion
   that died before removing it, and is removed; one it holds under the same file but
   that reads otherwise, or one it does not hold, was written after the conversion and
   stops the mailbox.
3. For every report, establish its state from the evidence above. A report to a run
   of this build that has ended or was replaced is moot: it can reach nobody. One to a
   run of the earlier build is not moot merely because that run ended: the name's
   successor decides it — held for it, or moot once it has ended (8-cutover).
4. Collect the obligations closed: each message a closing report (finished or failed)
   answered, per sender run, when that report is proven done or settled delivered.
5. Decide every other report:
   - superseded, when others closed all its obligations;
   - published, when proven not done and none of its obligations is closed;
   - held, when proven not done and its recipient is a running earlier-build run
     (8-cutover);
   - withheld for good, when it closes nothing and is unknown (8-withheld);
   - when some of its obligations are closed and the rest are not: if it is proven not
     done, it is not published and the rest stay owed for the next end; if it is
     unknown, the mailbox stops, since whether it answered the rest is unknown too.
     Say R1 on tasks A and B is prepared with its publication unknown, and R2 on A alone
     is done: R2 settles A only, and a new report on B could repeat what R1 delivered;
   - otherwise unknown: the mailbox stops.
6. Write the decisions into the journal, then complete it: publish, take the kept
   answer by version, clear the waits whose obligations are closed, moot or held, write
   the interim record, mark done. A held report hands its obligations to the journal:
   the journal keeps the report whole until it is delivered or reaches a terminal
   outcome it records (moot, below), and the waits are cleared because the journal now
   carries the answer, not because it has arrived.

This build's journals never overlap each other: an end reads only the waits the barrier
left, and the barrier completes every journal before that. Overlap is possible only
among the earlier build's receipts, and step 2 decides those together, in one record,
before any effect; so a done journal of this build keeps no more than its done mark and
its `Ended`, which opens the next end's window (pending marks). The conversion journal
is different: it holds the person's decisions (8-settle), so once done it is kept whole
and for good. There is at most one per name, since no earlier-build receipt appears once
the successor is bound, and one that does stops the mailbox (8-cutover).

## The stop and `rewake settle`

A stopped mailbox (8-stop) keeps every record. Its session reads, reports and clears
nothing, and adoption takes nothing over: each of its hooks and calls that would change
the mailbox answers the exact cause instead, and a hook exits so the harness goes on,
never blocking a stop. Three changes go on, since none decides an effect: letters from
others still arrive and wait unread; recovery writes its own records — the stop, the
conversion journal, a person's decision; and main is told. A stop a later
look may settle names the path that could not be read or parsed; it is kept on record,
and goes as [mailbox-records.md](mailbox-records.md#the-stop-on-record) says. An irrecoverable stop is named to main once, by a note whose id is named by the
journal and published under the journal protocol:

```
Rewake: api stopped changing its mailbox. Its record <state>/inbox/api/journal/<op>
holds 2 reports rewake cannot tell were delivered: an earlier build may have written
them and the sweep removed them since, or never written them.
  report 3f2c… to web (run 4121@…): tasks 9a1…, 7be…
  report 81d0… to db (run 5230@…): task c44…
Until each is settled, api reads, reports and clears nothing. Ask each recipient
whether it has that report, then run one line per report:
  rewake settle api 3f2c… --delivered     it arrived; its tasks count as answered
  rewake settle api 3f2c… --undelivered   it did not; it goes now
```

The unit of a decision is one report: one recipient run and the obligations it answers,
so a report that reached web and one that never reached db are settled apart (8-settle).
The earlier build's kept answer asks for no decision: it belongs to an ended run and is
never published (8-cutover).

`rewake settle` is a shell command for main or the person, not on the tool's surface. A
call from inside a session other than a verified main of this build is refused with
exit 2: a worker settling its own stop would decide the very evidence it is stopped on.
A shell outside every session is the person's. It
takes the mailbox lock and finds the report in the stopped journal; a report that is
not unknown is refused. It writes the decision into the journal before any effect, then
runs the barrier, which reconciles again with that decision among the evidence: a
report settled delivered closes its obligations and may supersede another; one settled
undelivered is published, to its recipient run if it lives, to that run's successor
(8-cutover), or nowhere if its recipient was of this build and has ended. The same
words again answer the recorded decision; the opposite words are refused naming it.
While other reports stay unknown the mailbox stays stopped, and the answer lists them.
The decisions live in the conversion journal, which is kept whole for good once done
(8-retention), so the same words answer the same way after the mailbox has resumed; a
report no journal names is refused as unknown to rewake.

## The cutover

A run of the earlier build keeps its protocol for its whole life (8-cutover): this
build refuses to act for it, holds the reports meant for it, and meets what it left at
the adoption of the name's next run. How a launch proves the earlier writers stopped,
names a run across restarts of the machine and binds the successor is in
[protocol-cutover.md](protocol-cutover.md).

## Every record and state

| Record | State | Proven | Unknown | Recovery |
|---|---|---|---|---|
| Journal | absent | nothing was done | — | a completion prepares from its event; a hook from its one attempt |
| Journal | unfinished | what it names | per effect, by the evidence table | reconcile, then complete |
| Journal | holding a report for an earlier-build run | every other step; the report and its obligations are the journal's | — | published to the successor; moot, with a note to main, if the successor ended first |
| Journal | stopped | the proven parts | the reports it names | note to main once; `rewake settle` |
| Journal | owing main a note of a moot report | the moot decision | — | the note goes before any other effect; the debt is dropped once it is out |
| Journal | done | complete, and its `Ended` | — | nothing; kept while the run lives |
| Journal | unreadable | — | everything | the mailbox stops |
| Earlier-build receipt | not prepared | that end sent nothing and cleared nothing | — | nothing to publish; its waits stay owed |
| Earlier-build receipt | prepared, not done | it cleared no wait | each report without a letter | converted; unknown reports stop the mailbox |
| Earlier-build receipt | done | its reports reached every recipient then live | whether it cleared its waits | its closing reports close their obligations, and their waits are cleared |
| Earlier-build receipt | stopped or interim | as above | as above | its reports close nothing: withheld when unknown; its waits are kept |
| Earlier-build receipt | no reports | — | — | converted; it closes and clears nothing, and is never swept by age |
| Earlier-build receipt | unreadable | — | everything | the mailbox stops |
| Conversion journal | done | every decision, the person's included | — | nothing; kept whole for good |
| `kept.json` | this run's, at or below the boundary | its version and position | — | taken by the version the journal names |
| `kept.json` | this run's, above the boundary | a later hold | — | left for a later end |
| `kept.json` | another run's, any build | an ended run's | whether it was ever published | never taken; written over by the next hold |
| `kept.json` | unreadable | — | its owner | the mailbox stops before any effect, and nothing writes over it |
| Pending mark | any | as the pending table | — | as the pending table |
| `interim.json` | any | as the interim table | — | as the interim table |
| Once mark | none, intent, published | as the evidence table | — | as the evidence table |
| Once mark | other, unreadable, or behind a path that cannot be followed | — | the publication | the mailbox stops before any effect, naming the path |
| Wait record | unreadable | — | its messages | the mailbox stops |
| Run record | present, under the run's boot and epoch | a run of this build | — | its mailbox follows this protocol |
| Run record | none: a run named by its epoch alone | an earlier build's run, by how this build names runs | — | while a writer of it runs, every change to its mailbox is refused and main told; once none is, the name may be taken |
| Run record | unreadable | — | the run's build | whatever depends on it stops, naming the path |
| Successor record | absent | no run of this build bound itself yet | — | the next launch binds |
| Successor record | present, naming a boot and epoch | the successor, for good | — | held reports go to it, or are moot once it has ended |
| Successor record | unreadable | — | the successor | held reports stay held, and a launch of the name refuses |

## What the reviews found

Which clause closes each finding of the earlier acceptances and reviews, and what the
earlier probes now expect of the same bytes, is in
[turn-end-recovery-findings.md](turn-end-recovery-findings.md).

## What stays open

- The earlier build's wrapper runs that build's code until its run stops: its Codex
  completions and the plugin's stop publish by that code, with its races. This build
  bounds that only by not acting for such a run, and by meeting what it left once it
  has stopped.
- A turn end of the earlier build heard from a hook left no record at all. A crash of it
  after publishing and before clearing its waits leaves waits that adoption takes over
  and the next end answers again; nothing is left to recover or stop on.
- The answer of a held turn whose run ended is never published: if that run's end never
  published it either, the tasks are still answered by the next run's end, without that
  text.
- An end with an id and no boundary reports nothing; its waits stay owed for the next
  end with a boundary or heard once.
- A report held for an earlier-build run is lost if the successor ends before taking it:
  it is moot like any report to an ended run of this build, and main is told. The
  successor recorded as its recipient is looked at again on every attempt, so one that
  ended before the publication makes the report moot rather than published.
- The settle gate reads the session the call comes from; a worker that clears its
  session variable from its own shell passes for the person. The gate keeps an honest
  worker from settling by mistake, not a dishonest one.
- Run records and the successor record are never swept: one small file per launch,
  and one per name.
- An end heard before an earlier end of its run has a journal may share a mark with it:
  both are interim, once (pending marks).
- The automatic cutover holds where the upgrade replaces the binary at the paths the
  earlier runs use. An earlier-build binary left at such a path, root, and a run in
  another pid namespace whose session record is gone are beyond the look for writers;
  the barrier stops a mailbox where such a writer left a record, as a signal, not a
  proof ([protocol-cutover.md](protocol-cutover.md)).
