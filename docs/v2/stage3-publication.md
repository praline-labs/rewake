# Stage 3: one landing per copy — publication inside the recipient's lock

A part A behaviour change, built as the first commit of
[S3](stage3-steps.md#s3-a-stop-is-resolved-per-cause-by-evidence-codex) and accepted
Codex-side with it. Review round 5 of part B found it while checking the decision's
invariant I4. It is a defect of the carried core, though, not of the decision: an ordinary
retry with no decision anywhere publishes the same report twice. Citations of code are to
`b6ed4ed`, where `internal/` is unchanged since.

## The race

A turn-end journal J publishes report R to run r of the name `lead`
(`inbox/journal_held.go:145-161`, `publishReport`). In order:

1. J reads `lead`'s live run in the registry. If it is not r, J records R moot and writes
   nothing.
2. J calls `publishMarked` (`:180-212`). It looks for the letter in every stage, reads r's
   once mark and writes intent, letter and `published`.

None of this holds `lead`'s mailbox lock. The comment at `:176-179` says why: the barrier
holds the sender's own lock, and the code avoided waiting on a second one. The proof of
a landing is the letter and r's once mark (`once.go:14-21`). It is retired by `lead`'s
ordinary sweep:

- the letter, a day after it was read, once `settleOnce` has turned an intent into
  `published` (`sweep.go:74-76`, `once.go:137-150`);
- the marks of every run but the sweeper's (`sweep.go:89`, `once.go:206-216`).

That sweep assumes no attempt for an ended run can still pass the liveness check. An
attempt that has already passed it breaks the assumption:

- R lands once. The sender dies before saving `Published`.
- r reads R, and R's age passes the retention cutoff while r still lives.
- A retry of J finds r live (step 1) and pauses before its first evidence read.
- r ends. The next run of `lead` sweeps r's letter and r's marks.
- The retry resumes. It finds no letter and no mark, and writes R again under the same id.

The review's probe runs this schedule on today's core and passes: two successful writes of
one letter id (`TestStage3ReviewRetryAcrossRunEndAndSweep`; main's run, exit 0,
`.scratch/v2-stage3-review5-probes/run.log`).

The heads-up has the same gap. Its operation checks the recipient's run with
`registry.Lookup` outside any lock (`cli/journal_steps.go:41`). It then publishes through
`PublishOnce` (`once.go:61-104`), which reads mark and letter under the recipient's lock
but does not check the run there. The channel keeper's notes check `roomMain` before
`PublishOnce` the same way (`wrap/channel.go:228-246`).

## Which rules it breaks

The race breaks two rules, and no rule's wording changes to fix it:

- **E3.** "Every check that decides an effect runs inside the critical section, right
  before the effect." The liveness check decides the write and runs outside the section.
- **E8.** "Proof lives as long as anything could replay it." The sweep retires the proof
  while an admitted attempt can still replay the write.

The fix makes the carried code do what the accepted rules already say. Exactly one
landing per copy does not depend on S18, so the fix is part A's.

## The contract

A **once-publication** is any write proven by a once mark of its recipient's run:

- a turn-end report (`publishMarked`);
- a heads-up (`PublishOnce` from `cli/journal_steps.go:94`);
- a channel note (`PublishOnce` from `wrap/channel.go:243`);
- from S18, a decision's notes.

For the copy C of such a publication, addressed to run r of name n:

- **P1. One critical section.** The publisher takes n's mailbox lock with a bounded wait.
  Inside it, in this order, it does three things. It reads n's live run. It reads C's
  publication evidence: the letter in every stage and r's mark, each with E6's three
  outcomes. It makes the writes: intent, letter, `published`. A liveness answer read
  before the lock decides nothing. If the run read inside the section is not r, nothing
  is written. The caller then records moot under the sender's rules: S3 consults the open
  occurrences first, and the model's M10 records moot only when nothing else prevents it.
- **P2. The proof is retired only under the same lock, and only by the live run.**
  - A letter's last copy leaves the mailbox only under n's lock. The sweep is the remover
    that matters here; the stage moves of `unread.go` keep a copy in the next stage, and
    S3's review lists every remover with `go/types` and checks each against this rule.
  - A run's marks are removed only by a sweep holding n's lock that has read, inside that
    section, that its own run is n's live run and that r is not.
  - A sweep that cannot take the lock retires nothing. Today `lockWithContext`
    (`inbox/outcome.go:35-42`) runs it unlocked when the lock is unusable, which is the
    E2 gap S3 closes.
  - A sweeper whose run has ended retires nothing at all. Without that check, a stale
    server of an ended run would remove the live run's marks and letters
    (`serve.go:120,177` check ownership only at start).
- **P3. Liveness is read without the name lock.** Inside the mailbox lock the run is read
  with `registry.LookupReadOnly` (`registry/observe.go:5-16`). That read never enters
  name-lock cleanup, which the registry forbids to a holder of a mailbox lock.
- **P4. The sender's own mailbox is not locked twice.** A report to the sender's own name
  (`journal_held.go:156-158`, a failed main's report kept locally) is published inside
  the barrier's own lock. The lock is a `flock` on a fresh descriptor (`state.go:246-273`),
  not reentrant, so a second acquisition by the same process would wait out its bound
  against itself.

The plan runs the same code and writes nothing. Whether its seam takes the recipient's
lock is the implementation's choice: the plan predicts, and only the real pass, inside
P1's section, decides.

## Why it is enough

The claim: under P1 and P2, an attempt that finds r live inside its section also finds
the proof of every earlier landing of C.

1. Take an earlier landing of C. It was written inside some section S1, and S1 left the
   letter beside an intent or a `published` mark of r (`publishMarked`, `PublishOnce`).
   A crash between those writes leaves an intent with its letter, which a sweep promotes
   before removing the letter (P2, `settleOnce`).
2. Take a later attempt whose section S2 reads r live. Retiring the last proof of the
   landing needs a sweep section S3 that read, inside S3, that r is not n's live run.
3. Sections on one lock do not overlap. If S3 came before S2, r had ended before S2, and
   a run that has ended never lives again: a later launch is a new run. Then S2 could not
   have read r live, which contradicts step 2. So S3 came after S2, and S2 found the proof
   and wrote nothing.

A first landing written across r's end stays allowed. If r ends after S2's liveness read
and before its write, the letter is written once, addressed to r. The model counts it as
landed, and no proof of an earlier landing exists to be missed.

A second liveness check outside the section would leave the same gap. Only the section
orders the publisher against the sweep.

## What it costs

- **Two locks held at once.** The barrier holds the sender's lock and waits, bounded, for
  the recipient's — the wait `PublishOnce` already makes (`readerLockWait`,
  `cli/journal_steps.go:89-93`). Two mailboxes publishing to each other at the same moment
  cannot deadlock: at worst both waits expire.
- **A busy recipient.** If the wait expires, the attempt writes nothing. That is a plain
  failure, not an unknown: no stop is recorded (`reconcile.go:62-82` records a stop only
  for an `UnknownRecordError`). The journal stays unfinished, and the next barrier
  publishes once. A turn end can now fail on a busy recipient where today it would have
  written without the lock. It answers as for any busy mailbox, and its waits stay owed.
- **The sweep waits for publishers.** It already takes the lock (`sweep.go:16-21`), so it
  queues behind a publication instead of running beside it.

## Where it goes, and why there

It is **S3's first commit**, green on its own and before the occurrence rules of the same
step.

- **After S2.** S2 deletes the earlier-build branch and its `registry.Successor`
  (`journal_held.go:37-82`, `heldSuccessor`), so the section is written for one publication path, not two.
- **Inside S3.** S3 changes the same function to consult the open occurrences before the
  liveness check, and that order is now: the occurrences, under the sender's lock held
  throughout; then P1's section. S3 also closes E2's gap on the same sweep (the sweep
  removing only under its lock). Writing both in one step builds the order once and gives
  it one Codex-side acceptance.
- **Not a step of its own.** A new step number would renumber S4–S19 across the stage's
  documents and the step list `docs/rules_test.go` reads. A commit inside S3 keeps the
  order, and it lands before everything that relies on it:
  - S3's own moot-and-occurrence test;
  - S5's rebuilt end oracles on the fixture;
  - S7's tool path;
  - part B's I4 (S18).

## What changes

- `inbox/journal_held.go`: `publishReport` and `publishMarked` become one section under
  the recipient's lock (P1, P3), except for the sender's own name (P4). The comment at
  `:176-179`, which explains why it ran unlocked, goes.
- `inbox/once.go`: `PublishOnce` reads the recipient's live run inside its section and
  writes nothing when the run is not the message's `ToEpoch`. Its callers keep their
  outside checks as early answers only: `resolveEndedRun` (`cli/journal_steps.go:36-67`)
  still reads an ended run's mailbox through `PublicationOf`, and `roomMain` still drops
  a note for a main that is gone. `sweepOnce` and `settleOnce` run only under P2's
  conditions.
- `inbox/sweep.go` and `inbox/serve.go`: the sweep reads, under the lock, whether its run
  is the live one, and retires no proof when it is not, or when the lock is unusable.

## Tests

The S3 commit adds these tests. Each fails on today's code or under the mutation named,
except the two outcomes after a race, which hold what the change must keep.

The two lock-race tests share one setup: R lands, the sender dies before `Published`, r reads R,
and R's age passes the cutoff. Each runs for a turn-end report and for a heads-up, the
heads-up through `rewake retry` of its receipt; the expected outcome is the same, with the
receipt marked published in place of the journal's `Published`. The schedules are made
by hooks and channels, never by a sleep.

- **The sweep waits for an attempt's evidence.** A retry reads r live inside its
  section. At its first evidence read, a hook ends r and starts the next run's sweep in
  a goroutine. Expected: the sweep waits until the retry's section ends. The retry finds
  the letter and writes nothing; the seam's log counts one letter write.
- **Admission is inside the section.** A retry stops at its successful liveness read,
  held by a hook. The test ends r and starts the next run's sweep in a goroutine. It
  waits for whichever comes first: the sweep reports, from the lock's seam, that it found
  the lock held and is waiting, or the sweep finishes. Then it resumes the retry.
  - Expected: the sweep finds the lock held and waits; the retry finds the letter and
    writes nothing; the sweep then retires the proof. The seam's log counts one letter
    write.
  - Mutation: the liveness check moved before the lock, with the evidence reads and the
    write still inside it. The paused retry holds no lock, the sweep finishes and retires
    the proof, the resumed retry takes the lock, finds none and writes: two letter
    writes. The evidence-read hook above cannot tell this mutation from P1, since
    that read stays inside the lock.
- **A saved `Published` stays.** For the turn-end report, after the retry of
  either schedule saves `Published`, stop the sender before the journal's
  done rewrite, leaving that save durable. Let the sweep retire the proof,
  then run a later barrier. Expected: the barrier skips R, makes no liveness
  read for it, records no moot for it and writes no letter. The saved
  `Published` is present in the open journal; ordinary completion may then
  replace it with the done form (`inbox/journal.go:164,177-179`).
- **A crash before the save.** For the turn-end report, the retry of the second schedule
  is killed after its section and before `Published` is saved. The sweep finishes and retires the proof.
  Expected: a retry of the still unfinished journal finds r ended, records R moot and
  writes no letter; one letter write in all. The test goes through the journal: the
  publication helper answering ended is not a journal recording moot.
- **A first landing across run end.** There is no earlier landing. Inside
  the still-held section, a hook ends r after the successful liveness read
  and before the first evidence read. The reads find no letter and no mark:
  absence still proves no earlier landing. Expected: one letter written,
  addressed to r; the journal saves `Published`; no second liveness check
  rejects the admitted attempt, and nothing is written again.
- **A stale sweeper.** The server of an ended run sweeps after the next run is live.
  Expected: the live run's marks and letters stay.
- **An unusable lock.** A sweep that cannot take the lock retires no letter and no mark.
- **A busy recipient.** The wait expires. Expected: nothing written, no stop, the journal
  unfinished. The next barrier publishes once.
- **Two mailboxes publishing to each other at once**, each inside its own barrier. Both
  finish within their bounds or answer busy. Expected: no deadlock, each report landed
  once.
- **A failed main's report to itself** completes within the test's deadline: no second
  acquisition of its own lock.
- **Crashes.** A kill at each write of the clean run's log, through the existing fault
  seam (`plan_faults_test.go:35`). Expected: at most one letter write per id across all
  attempts.

The resend of part B is an ordinary journal and publishes through the same code. S18's
test tables carry the admission schedule for a resend R': paused at its liveness read,
then r's end and the sweep, with one landing of R' counted
([the tests](stage3-decision-recovery.md#the-tests)).

## What the rules documents need

E3 and E8 in [design-rules.md](design-rules.md#effects-e1e8) keep their wording.
`docs/rules/effects.md`, written in S1, needs two gap lines, each `closed in S3`:

- under E3: a once-publication checks its recipient's run outside the recipient's
  critical section (`journal_held.go:145-161`, `cli/journal_steps.go:41`,
  `wrap/channel.go:230`);
- under E8, the retention clause: a publication's proof is retired while an attempt
  admitted to its run can still replay it (`sweep.go:74-89`, `once.go:206-216`), and by a
  sweeper that is not the live run.

When S3 lands, both gaps are replaced by the tests above. The same two rows join the gap
table of [stage3-tests.md](stage3-tests.md#gaps-and-their-steps).
