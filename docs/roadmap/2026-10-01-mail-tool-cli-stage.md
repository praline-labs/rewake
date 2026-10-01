# Stage M1 of the mail tool: the CLI side, accepted

The first of the three stages of [mail-bridge.md](../mail-bridge.md) — the CLI that runs
one call of the mail tool — was accepted on October 1, 2026, after seventeen acceptance
rounds on the Codex side. The server (M2) and the injection at launch (M3) come next;
until the wrapper's context endpoint exists, nothing runs under the tool and every ticket
is refused.

## What was built

- **The tool's CLI side** ([mail-bridge-cli.md](../mail-bridge-cli.md)): the worker's own
  CLI words run as one call — the surface the tool allows, the ticket and the receipt of
  a call, reads in parts claimed by the call that showed them, the size cap on the final
  encoded bytes, and a retry that is a new call with its own deadline.
- **Eight rules the code holds**, stated in that document: an operation whose effect is
  unknown is never discarded or bypassed; a lock is removed only by its holder; every
  check that decides an effect is made under the lock and again on retry; a read's
  completion is one durable fact; the cap holds on the bytes sent; a lookup has three
  outcomes and only "no such file" is absence; every effect has an immutable identity
  and a proven scope, and recovery advances it only from durable evidence; and an effect
  is proven done, proven not done or unknown, and an unknown stops every change to its
  mailbox until evidence or a person settles it.
- **Turn-end recovery and the protocol cutover**
  ([turn-end-recovery.md](../turn-end-recovery.md),
  [protocol-cutover.md](../protocol-cutover.md), built as recorded in
  [the entry of September 30](2026-09-30-turn-end-recovery-build.md)): runs named by pid,
  start and boot; one journal per turn end with its read boundary; marks of the run's
  life; the earlier build's receipts converted into one journal and decided together,
  with `rewake settle` for a report nothing can decide; a launch that refuses while an
  earlier-build writer may still run.
- **Every record of a mailbox on one list, and a plan before the effects**
  ([mailbox-records.md](../mailbox-records.md)): the barrier and every call that would
  change the mailbox first walk it against the list of record kinds, then run the
  barrier's own effects read-only through one file seam; only a plan that meets no
  unknown lets them run. A stop is a record of the mailbox, and an effect's stop is
  lifted only by a barrier that ran every effect through.

## How it was accepted

- **Rounds 1–8** found defects in the code against rules that grew with them: eleven in
  the first round, most against five properties; a sixth property in the second, a
  seventh in the fifth and the last in the eighth. Among them a read taken for absence:
  about 21 places in the delivery code older than the stage read an error as "not
  there"; round 4 closed all but one of them.
- **Round 9 was a design-first pass.** Rules 7 and 8 were restated, and the recovery
  rules went through five passes of review as text, with no code changed, until they
  were accepted; only then were they built (round 10).
- **Rounds 10–13** found recovery and cutover defects, then a class that kept coming
  back: an unknown found late, by an effect, after an earlier effect of the same barrier
  had gone through. Each fix extended a hand-written list of what the effects read, and
  each round found the list short.
- **Round 14 answered the class rather than the instance**, again with the design
  written and accepted before the build: the effects run first as a plan through the
  same seam, and an automatic fault test watches the seam to learn what the plan reads,
  then fails each read in turn — in both passes, in the effects alone, paired with a
  second cause, and with each write of the barrier broken once — instead of checking a
  list. A read added to an effect later is on the test's list the next run.
- **Rounds 15–17** each found one way an effect's stop could still be lost — replaced by
  a different cause the plan after it found, recorded weaker after a failed write, or
  missing from the answer when no write went through — and the fault test was extended
  to the dimension each one opened. Round 17 accepted.

Each round's fix came with a test, and with a mutant of the fix killed by it; the probes
of every round were rerun at each later one.

## What stays open

- **A stop that no write of a call could record** lives only in that call's answer,
  which names every cause and every failed write. Until a later barrier records it, the
  mailbox is held only by each gate's own plan, which finds every cause still there but
  not one that only an effect met ([mailbox-records.md](../mailbox-records.md#the-stop-on-record)).
- **Two failures of the same write** are tried only for the stop record; the other
  writes of the barrier are broken one at a time.
- **One place of the older delivery code** still reads an error as absence: `send`
  goes on when the registry lookup before it fails with anything but "not found". Its
  effect is low — the epoch is checked again before the letter goes — and it is queued
  in [work-queue.md](../work-queue.md#now-after-100).
- What stays open of the recovery rules is in
  [turn-end-recovery.md](../turn-end-recovery.md#what-stays-open), and what the live
  stages must still show is in
  [mail-bridge.md](../mail-bridge.md#what-stays-open-before-acceptance).
