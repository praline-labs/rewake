# Stage 3: the operator decision — its model

Part B of stage 3 ([stage3.md](stage3.md#two-acceptances-part-a-and-part-b)), built in
S18. The decision is stated here as an abstract model before any record: what is true in
the world, what can be observed of it, what the sender's side records, what the operator
chooses, how long the runs live, when the mailbox lets an operation run, and what outcome
each world must have. The scope of what a decision is answerable for and correction 9
follow from the model. The record protocol derived from it, the rules D1–D8 and the tests
whose oracle it is are in [stage3-decision-recovery.md](stage3-decision-recovery.md); the
subject, the authority, the command and the records in
[stage3-decision.md](stage3-decision.md).

The running example: the worker `api` owes main `lead` a report. `api`'s turn-end
operation J publishes report R to `lead`'s run r; whether R landed cannot be proven.

## Why a model first

Each review round found the recovery wrong in a different composition: a resend that
waited on its child, writers frozen out of their own records, a resend state lost, a
decision that could not recognise its own resend once it had progressed, an oracle that
took the operator's choice for the truth, a moot copy counted as never landed, a parent's
tail promised to run past a mailbox the barrier keeps stopped, a retry outliving the proof
of its own earlier landing, a missing mark read as proof of non-delivery. Each was a record protocol
checked against its own reading. The model below has no file in it; the protocol is
derived from it and the oracle predicts from it, so a defect of the protocol shows as a
difference from the model rather than as agreement with itself.

## The world

Seven dimensions, kept apart. A test sets every one of them; no code reads the first.

**1. Historical landing.** For each copy of the report — R, and each resend R', R'', … —
its letter was written into r's mailbox, addressed to r, or it was not. Landing is an
event: once landed, always landed, whatever happens to the letter afterwards — read,
moved to `done/`, swept, r ended. **The publisher does not hold r alive**: from S3 it
reads r's run, the evidence and writes inside one critical section of r's mailbox lock
([stage3-publication.md](stage3-publication.md#the-contract)), and r can still end
between the read and the write, since a run's end takes no mailbox lock. A letter
written then is addressed to r and is a landing like any other; what r's name does with it
afterwards is ordinary activity. What r's end cannot do inside the section is retire the
proof of an earlier landing: the sweep that retires it waits for the lock, so each copy is
written at most once — part A's guarantee, which I4 restates. The oracle counts landings
from the fault seam's log of writes, never from the files left in r's mailbox.

**2. Evidence.** What a plan of the sender can read of one copy at one moment, by E8:

| Evidence | What it reads | Consistent with |
|---|---|---|
| none | inside the publication's section that admitted the attempt by reading r live, the lock still held: the step open, no letter and no mark — r may have ended since that read | not landed: no sweep can retire the proof of a landing while the section holds the lock, and none retired it before a section that reads r live ([stage3-publication.md](stage3-publication.md#why-it-is-enough)) |
| absence proven | an intent mark without its letter, which the sweep never leaves behind for a written letter (`inbox/once.go:14-21`) | not landed, at that read |
| published | the letter, or its published mark | landed |
| unknown | a read that failed, a mark of no known meaning, a mailbox that cannot be searched; **or every read succeeding with no letter and no mark without that admission** — outside a section that read r live and still holds the lock — the proof of a landing may have been retired once r ended (`PublicationOf`, `once.go:165-200`, answers `PublicationUnknown` there) | either |

`none` is the one row whose absence is evidence, and only where the protocol still proves
it: a step whose earlier attempt may have written, read without a still-held section that
admitted the attempt with r live, is unknown, not none. What decides is that admission, not
r's life at the evidence read: r ending between the section's liveness read and its first
evidence read leaves the absence proof of no earlier landing, and the first write is
allowed. **An open occurrence keeps its unknown** when its named paths vanish — r reads and
sweeps the letter, the next run of the name drops r's marks — and every later read
succeeds with "no such file": that is unknown — the retired-proof row, read without that
admission once r has ended, a named path removed while it lives — never none, and the occurrence stays open (S3: "no
such file" at a named path resolves nothing).

The evidence changes with time — by ordinary activity in r's mailbox, by repair, by a
read that fails and later succeeds, by r's proof being retired after r ends — always within
what the truth allows. Unknown is the only evidence that hides the truth.

**3. The sender's disposition** of a copy: what J's progress records of it, which is not
what happened to it.

| Disposition | Recorded when | What it says of landing |
|---|---|---|
| open | the step not yet run, or stopped | nothing |
| published | evidence "published" was read, or the sender's own write returned and was saved (`Published`) | landed |
| moot | a sender attempt, inside its publication's section, read r ended, with no other outcome recorded and no occurrence about the copy open (below) | **nothing**: no further publication to r is useful |
| closed by D | a decision on an unknown copy | nothing: delivery unproven |

**Moot is a lifetime disposition, not evidence.** An attempt reads r's run before it reads
any evidence and records `Moot` when it finds r ended (`journal_held.go:24-35`,
`:145-161`), whether or not an earlier attempt wrote the letter. Moot is recorded by that
observation, never by r's end alone: an attempt that read r live and wrote across r's end
records `Published` if it survives to save it, and only a later attempt, after that save
was lost, records the landed copy moot. A copy that landed while r lived, with the sender
dead before it saved `Published`, is recorded moot by the next barrier — the review's
probe confirms it on today's core. S18 keeps that behaviour and changes the claim: a moot
copy counts by its truth, which only the oracle knows.

**The stronger rule where an occurrence is open.** An occurrence is resolved only by
evidence about its effect (S3), and r's end is not such evidence. So a copy with an open
occurrence about its operation is not made moot by r's end: the occurrence stays, and
returning evidence or a decision closes it — and only the person may decide it, since r's
authority ended with r. S3 builds the order this needs, with its test
([stage3-steps.md](stage3-steps.md#s3-a-stop-is-resolved-per-cause-by-evidence-codex)): the sender's recovery
consults the open occurrences about a copy before the liveness check, and takes the moot
path only for a copy that has none.

**4. The operator's choice.** On an unknown copy, the recipient run or the person chooses
"arrived" or "did not arrive". Either can be wrong: a wrong "did not arrive" duplicates
(the copy landed, and its resend lands too); a wrong "arrived" loses (the copy did not
land, and nothing replaces it). The model never derives the truth from the choice nor the
choice from the truth; a test scripts both.

**5. Run lifetimes.** The sender run s, the recipient run r and main's run each live or
end at any point relative to the rest: before the decision; after it and before its
effects are installed; after a resend is installed and before it publishes; between the
resend's liveness check and its write; after its letter is written and before each of
the sender's saves; after a resend's outcome turned unknown. A later launch of a name is
a different run.

**6. Admission.** E8 lets an operation's effects run only in a barrier whose plan meets
no unknown anywhere in the mailbox (`reconcile.go:39-49,105-114,138-149`): an unknown
copy, an unrelated unreadable journal or any open occurrence keeps every journal of the
mailbox where it is, ready or not. An operation is **ready** when its own steps let it go
on; it **runs** when it is ready and the mailbox is admitted. The two are kept apart, and
E8's mailbox-wide rule wins: a decision makes a step ready, never the mailbox admitted.

**7. Progress and retention of effects.** What a decision causes is progressed and
retired by ordinary owners: the resend saves its steps, completes, takes its done form
and meets the retention of journals; a note's once record is swept with the run it was
written for (`once.go:205-216`). These transitions change no outcome.

**Excluded combinations** — never built by a test, never accepted by the oracle:

- a copy with evidence "published" that did not land; a copy with evidence "none" or
  "absence proven" that had landed by that read;
- a copy recorded published that did not land;
- a decision on a copy whose evidence at the decision's linearization point is not
  unknown;
- a verified decision by any run but r, or by r after r ended;
- a resend after "arrived"; a decision on a resend whose evidence is not unknown;
- a copy recorded moot while an occurrence about it is open, or by a run's end alone with
  no attempt reading it ended (M8, M10);
- evidence "none" read without the protected admission: outside the publication's
  section, or in a section that did not read r live before that read. "None" read in the
  section that read r live stays allowed when r ends between that liveness read and the
  evidence read; readable absence with no such admission is unknown, the row above.

A landed copy recorded moot is **not** excluded: it is the schedule above, and the
oracle counts it as landed.

## The transitions

Over the world, in any interleaving; these are the oracle's schedules.

| | Transition | When | Result |
|---|---|---|---|
| M1 | decide "arrived" | R's evidence unknown at the linearization point | R's step closed by D, delivery unproven |
| M2 | decide "did not arrive" | the same | R's step closed by D; a resend R' installed — R's text, kind and waits answered, a fresh id, naming R and D, to r |
| M3 | a resend runs | R' installed, the mailbox admitted | R' by E8 alone: published, moot or unknown; an unknown R' may be decided by M1 or M2, its decision naming D as parent |
| M4 | J's tail | every step of J published, moot or closed, and the mailbox admitted | the tail runs once; what it does concerning R names D |
| M5 | evidence about R returns | after D | nothing on the sender's side |
| M6 | ordinary activity in r's mailbox | any time | r reads R, the sweep promotes R's intent to published and removes its letter, a new run of the name drops old once marks, r proves a publication; nothing on the sender's side, D unchanged |
| M7 | an effect progresses or is retired | after it was installed | no outcome changes; it is never installed again |
| M8 | a run ends | any time | r's authority ends with it: from then only the person may decide a copy to r. No disposition is recorded by the end itself |
| M9 | a second command | D recorded | the same words answered by D; the opposite words refused |
| M10 | a sender attempt reads r ended | inside its publication's section, before any write of its own | moot recorded when the copy has no other disposition and no open occurrence about it; otherwise nothing. Zero writes when no earlier attempt landed; after an earlier landing it is the recovery of a lost `Published` save, and the copy counts landed |

M1, M2 and the installation of their effects need the sender's mailbox lock, a readable
subject and its unknown evidence — not admission. M3 and M4 need admission. So a
decision is recordable and its effects installable while the mailbox is stopped, which
is when it is needed, and a parent's tail never waits on a decision that waits on it.

## The expected outcome of a world

Derived from the seven dimensions only:

- **Copies.** r was written one letter per copy that landed: `[R landed] + [R' landed] +
  …`, over the copies that exist — R' exists only if R was decided "did not arrive", R''
  only if R' was. A copy that ran cleanly to a live r lands; a copy with no earlier
  landing whose attempts all found r ended wrote nothing and does not; every other copy —
  unknown, moot after a write whose `Published` save was lost, written across r's end —
  counts by its truth. Reading and sweeping change no count.
- **Right and wrong.** A choice is right when it matches its copy's truth. With every
  choice right and r alive for the last write, exactly one copy landed; each wrong "did
  not arrive" adds one; a wrong "arrived" over copies none of which landed leaves none.
- **The obligation** R answers is closed exactly once, by J's tail, marked closed by D
  with delivery unproven when R's step was closed by D. No later evidence and no copy
  reopens it or marks it delivered.
- **Waits.** R's waits stay while J's tail has not run; they are cleared at the first
  admitted barrier after every step of J is published, moot or closed — not earlier,
  however ready J is, while any unknown in the mailbox stands.
- **Notes.** Each of r and main's run that did not decide learns of D once, if it lives
  until a barrier can search its mailbox; an ended run is owed nothing; a note to a
  mailbox that stays unsearchable stays owed.
- **Records.** D holds the subject's identity, the choice, who decided and whether that
  was verified, "delivery unproven", and the observations it was made on. Nothing on the
  sender's side says R delivered after D, and nothing says a moot copy did not land;
  nothing anywhere derives delivery from D. A proof r establishes by its own activity may
  say R was published — genuine evidence — and changes nothing on the sender's side.
- **Convergence**, under its prerequisites: once every unknown in the mailbox is settled
  — by evidence or by a decision — and every mailbox a note goes to can be searched, every
  schedule ends with each decision's effects installed once, each resend at an E8
  disposition or stopped on its own cause, each J finished, and each note written or
  moot. Without them it ends with every effect installed and the rest stopped on a named
  cause or owed.

**Two generations**, R decided "did not arrive" and R' unknown and decided — all four
combinations of the truth of R' and the second choice, with x = 1 when R landed, 0
otherwise:

| Truth of R' | Second choice | R'' | Copies | |
|---|---|---|---|---|
| landed | arrived | — | x + 1 | right |
| landed | did not arrive | runs cleanly, lands | x + 2 | a duplicate |
| not landed | arrived | — | x | the resend lost |
| not landed | did not arrive | runs cleanly, lands | x + 1 | right |

With r ended before R'''s first attempt reads it, R'' is moot without a write and counts
nothing; with r ended after R'' wrote, R'' counts one — recorded published when the attempt
that wrote it saved that, moot when a later attempt recovered the lost save.

**The resend's own states**, E8's, independent of how it is recorded: none (inside the
section that read r live and still holds the lock, though r may have ended since; nothing
written: published when its turn comes); absence proven (an
intent, no letter: published, once, under the same id); published; unknown — a read failed
at the intent, at the letter or at the published mark, r's mailbox could not be searched,
or r has ended and nothing of its proof is left on a read without that admission — a cause about the resend's operation and
a subject of its own. r ended before the resend's first attempt read it: moot, nothing
written. r ended after its letter was written: published if that attempt saved it, moot
and landed if a later attempt recovered the lost save. r ended after the resend turned
unknown: the occurrence stays, evidence may still close it, and only the person may decide
it.

**The invariants** the outcome implies, which the rules restate:

- **I1.** A step closed by a decision is never published afterwards by the sender's side.
- **I2.** Every effect taken on D's authority names D, and nothing derives delivery from D
  ([the scope below](#what-a-decision-is-answerable-for)).
- **I3.** One decision per report: the same words answer it, the opposite are refused.
- **I4.** Each copy is written at most once, only addressed to r, or not at all. It rests
  on part A's publication contract (S3, [stage3-publication.md](stage3-publication.md)),
  not on the decision: an ordinary retry needs it as much as a resend does.
- **I5.** A decision's record and the installation of its effects follow from the record
  alone and wait on no other operation, decision, tail or admission; the effects' progress
  waits on admission like any operation's.
- **I6.** J's tail runs once every step of J is published, moot or closed and the mailbox
  is admitted; it waits on no decision, and no decision waits on it.
- **I7.** Every id is bounded; ancestry is a field, never a longer name.
- **I8.** D's observations are the ones its final plan used.
- **I9.** Each effect of D is installed at most once while D lives: its progress, its
  completion, its retention and the end of a run never make it missing.
- **I10.** A disposition is not a landing: moot and closed by D say nothing of whether a
  copy landed, and no record or rendering claims either.

## What a decision is answerable for

The third pass promised that every later effect concerning R names D. Effects after D are
of three kinds, and only the first can keep that promise:

| Kind | Examples | Names D |
|---|---|---|
| taken on D's authority | R's step closed in J; the tail clearing R's waits; R' and its operation; the notes; every rendering of these | yes |
| the sender's recovery | J's barrier; a retried turn end | it reads D before R's live evidence and closes R's step by it; it never publishes R, changes D or cancels R' |
| ordinary activity in r's mailbox | r reading R (`inbox/unread.go:181-188`); the sweep promoting R's intent before removing its letter (`once.go:133-149`); a new run dropping old once marks (`once.go:205-216`); r's own proof of publication | no |

The third kind follows its owners exactly as it does without a decision, derives no
authority from D, and need not know it exists. Its records may say R was published: that
is evidence produced by the mailbox holding R, and it is allowed. It does not reach the
sender (M5). Making every writer in r's mailbox name D would need each to discover the
sender's decisions — a cross-mailbox ordering protocol — and no outcome above depends on
it, so the promise is not made.

R' does name D, as an effect on D's authority: it carries `resends: R` and `decision: D`,
so a reader holding R and R' sees that R' is R sent again by decision and can tell a
duplicate.

## Correction 9, in its normative form

The accepted contract keeps "the original records and the stop's evidence untouched, for
good" ([design-rules.md](design-rules.md#an-operator-decision-on-an-unknown-outcome)).
Read literally, every path a decided stop names is frozen against its own writers for the
mailbox's life; the second review found that freezing deadlocking on a child decision,
losing a resend state and growing ids without bound. Proposed, in the form the third
review recommended, and the reading S18 builds:

- **D keeps what it was made on, for good**: the operation, recipient and obligation
  identity, and the exact observations of its final plan — for a read that returned, its
  bytes and their SHA-256, bytes that failed to parse included; for a directory, the
  listing it returned; for a read that failed, the error and whatever `lstat` returned.
  These are the observations the plan used, captured as it used them, not a later reread
  presented as a snapshot. Contents that could not be read are not in D and cannot be
  recovered from it: an error and an `lstat` record a failed observation, not a copy of
  what was not seen.
- **The live originals follow their ordinary owners**: r may read or sweep R and may
  establish a genuine proof of publication; none of this derives authority from D or
  carries its id.
- **D is never proof of delivery.** Every effect authorized by D carries its id. The
  sender's recovery treats R's step as decided before it consults R's live evidence,
  never republishes R, and never changes D or cancels R' on later proof.
- **R' is an independent E8 operation**, and D's installation of each effect is proven
  for as long as D could install it again — ordinary retention of journals is not that
  proof ([installing the effects](stage3-decision-recovery.md#installing-the-effects)).

What it gives up: the original records are not kept untouched for good; what is kept for
good is what the decision saw. The oracle of original paths and hashes unchanged is
retired; the tests check the kept observations and the decision's provenance, and let the
live paths change. Accepted in review round 4.

## The simpler contract the model shows

Taken, each because the model needs no more:

- **The resend's installation is its own name, kept.** Installing R' is one exclusive
  link at a name fixed by D; the name is recognised in every form the journal takes, and
  the journal is retained in its done form for as long as D. Installation and its proof
  are then one atomic act, so there is no crash window between installing and recording
  a witness, and no witness record at all.
- **A note's once is the recipient run's own once record**, idempotent for that run's life
  and moot after it ends; a record in the sender's mailbox only stops the checking.
- **Attribution is scoped** to effects on D's authority — narrower than the third pass's
  promise, and the only one the outcomes need.
- **Moot keeps today's behaviour** with a narrower claim, rather than recovery reading
  publication evidence after r's end. That reading would be a change of behaviour, with
  outcomes still to define for a letter read or swept since, and the sender would still
  not know the truth; no outcome above needs it.
- **E8's admission is kept** rather than a bounded exception letting a ready tail run
  past other unknowns, which would need a preflight of its own; the decision unblocks the
  mailbox by settling the unknown, which is what it is for.

Considered and refused: a decision that republishes R under R's own id instead of a
resend operation. It would publish R after D, against I1, and make the decision wait on a
delivery.
