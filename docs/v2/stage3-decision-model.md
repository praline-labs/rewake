# Stage 3: the operator decision — its model

Part B of stage 3 ([stage3.md](stage3.md#two-acceptances-part-a-and-part-b)), built in
S18. The decision is stated here as an abstract model before any record: what is true in
the world, what can be observed of it, what the operator chooses, how long the runs live,
and what outcome each world must have. The scope of what a decision is answerable for and
correction 9 follow from the model. The record protocol derived from it, the rules D1–D8
and the tests whose oracle it is are in [stage3-decision-recovery.md](stage3-decision-recovery.md);
the subject, the authority, the command and the records in
[stage3-decision.md](stage3-decision.md).

The running example: the worker `api` owes main `lead` a report. `api`'s turn-end
operation J publishes report R to `lead`'s run r; whether R landed cannot be proven.

## Why a model first

Each review round found the recovery wrong in a different composition: a resend that
waited on its child, writers frozen out of their own records, a resend state lost, a
decision that could not recognise its own resend once it had progressed, an oracle that
took the operator's choice for the truth. Each was a record protocol checked against its
own reading. The model below has no file in it; the protocol is derived from it and the
oracle predicts from it, so a defect of the protocol shows as a difference from the model
rather than as agreement with itself.

## The world

Five dimensions, kept apart. A test sets every one of them; no code reads the first.

**1. Physical truth.** For each copy of the report — R, and each resend R', R'', … — the
copy landed for run r, or it did not. Landing is an event: once landed, always landed,
whatever happens to the letter afterwards (read, moved to `done/`, swept). A copy
published for r is pinned to r; a letter written after r ended counts as nothing landed.

**2. Observable evidence.** What a plan of the sender can read of one copy at one moment,
by E8:

| Observation | What it reads | Consistent with |
|---|---|---|
| none | the step open, no intent mark | not landed |
| absence proven | an intent mark without its letter, which the sweep never leaves behind for a written letter (`inbox/once.go:14-21`) | not landed, at that read |
| published | the letter, or its published mark | landed |
| moot | r ended before the copy was published | not landed for r |
| unknown | a read that failed, a mark of no known meaning, a mailbox that cannot be searched | either |

The observation changes with time — by ordinary activity in r's mailbox, by repair, by a
read that fails and later succeeds — always within what the truth allows. Unknown is the
only observation that hides the truth, and the only one on which anyone may decide.

**3. The operator's choice.** On an unknown copy, the recipient run or the person chooses
"arrived" or "did not arrive". Either can be wrong: a wrong "did not arrive" duplicates
(the copy landed, and its resend lands too); a wrong "arrived" loses (the copy did not
land, and nothing replaces it). The model never derives the truth from the choice nor the
choice from the truth; a test scripts both.

**4. Run lifetimes.** The sender run s, the recipient run r and main's run each live or
end at any point relative to the rest: before the decision, after it and before its
effects are installed, after a resend is installed and before it publishes, after a
resend's outcome turned unknown. A later launch of a name is a different run.

**5. Progress and retention of effects.** What a decision causes is progressed and
retired by ordinary owners: the resend saves its steps, completes, takes its done form
and meets the retention of journals; a note's once record is swept with the run it was
written for (`once.go:205-216`). These transitions change no outcome.

**Excluded combinations** — never built by a test, never accepted by the oracle:

- a copy observed published that did not land; a copy observed none, absence proven or
  moot that had landed by that read;
- a decision on a copy whose observation at the decision's linearization point is not
  unknown;
- a verified decision by any run but r, or by r after r ended;
- a resend after "arrived"; a decision on a resend whose observation is not unknown;
- a copy landed for r after r ended.

## The transitions

Over the world, in any interleaving; these are the oracle's schedules.

| | Transition | When | Result |
|---|---|---|---|
| M1 | decide "arrived" | R observed unknown at the linearization point | R's step closed by D, delivery unproven |
| M2 | decide "did not arrive" | the same | R's step closed by D; a resend R' installed — R's text, kind and waits answered, a fresh id, naming R and D, to r |
| M3 | a resend runs | R' installed | R' by E8 alone: published, moot or unknown; an unknown R' may be decided by M1 or M2, its decision naming D as parent |
| M4 | J's tail | every step of J published, moot or closed | the tail runs once; what it does concerning R names D |
| M5 | evidence about R returns | after D | nothing on the sender's side |
| M6 | ordinary activity in r's mailbox | any time | r reads R, the sweep promotes R's intent to published and removes its letter, a new run of the name drops old once marks, r proves a publication; nothing on the sender's side, D unchanged |
| M7 | an effect progresses or is retired | after it was installed | no outcome changes; it is never installed again |
| M8 | a run ends | any time | publishing to it is moot from then; r's authority ends with it, and only the person decides |
| M9 | a second command | D recorded | the same words answered by D; the opposite words refused |

## The expected outcome of a world

Derived from the five dimensions only:

- **Copies.** r holds one letter of the report per copy that landed: `[R landed] + [R'
  landed] + …`, over the copies that exist — R' exists only if R was decided "did not
  arrive", R'' only if R' was. A copy that ran cleanly to a live r lands; a moot copy
  does not; an unknown copy counts by its truth.
- **Right and wrong.** A choice is right when it matches its copy's truth. With every
  choice right and r alive for the last publication, r holds exactly one copy; each wrong
  "did not arrive" adds one; a wrong "arrived" over copies none of which landed leaves none.
- **The obligation** R answers is closed exactly once, by J's tail, marked closed by D
  with delivery unproven when R's step was closed by D. No later evidence and no copy
  reopens it or marks it delivered.
- **Notes.** Each of r and main's run that did not decide learns of D once while it lives;
  an ended run is owed nothing.
- **Records.** D holds the subject's identity, the choice, who decided and whether that was
  verified, "delivery unproven", and the observations it was made on. Nothing on the
  sender's side says R delivered after D; nothing anywhere derives delivery from D. A
  proof r establishes by its own activity may say R was published — genuine evidence —
  and changes nothing on the sender's side.
- **Convergence.** Every schedule ends with each decision's effects installed once, each
  resend at an E8 outcome or stopped on its own cause, each J finished or stopped on a
  report not yet decided.

**Two generations**, R decided "did not arrive" and R' unknown and decided — all four
combinations of R''s truth and the second choice, with x = 1 when R landed, 0 otherwise:

| R''s truth | second choice | R'' | copies | |
|---|---|---|---|---|
| landed | arrived | — | x + 1 | right |
| landed | did not arrive | runs cleanly, lands | x + 2 | a duplicate |
| not landed | arrived | — | x | the resend lost |
| not landed | did not arrive | runs cleanly, lands | x + 1 | right |

With r ended before R'' publishes, R'' is moot and counts nothing.

**The resend's own states**, E8's, independent of how it is recorded: none (nothing
written: published when its turn comes); absence proven (an intent, no letter: published,
once, under the same id); published; unknown — a read failed at the intent, at the
letter or at the published mark, or r's mailbox could not be searched — a cause about the
resend's operation and a subject of its own. r ended before the resend wrote its intent:
moot. r ended after the resend turned unknown: the cause stays, since a run's end resolves
nothing (S3), and only the person can decide it.

**The invariants** the outcome implies, which the rules restate:

- **I1.** A step closed by a decision is never published afterwards by the sender's side.
- **I2.** Every effect taken on D's authority names D, and nothing derives delivery from D
  ([the scope below](#what-a-decision-is-answerable-for)).
- **I3.** One decision per report: the same words answer it, the opposite are refused.
- **I4.** Each copy is published at most once, only to r, or is moot.
- **I5.** A decision's effects follow from its record alone and wait on no other
  operation, decision or barrier.
- **I6.** J's tail waits on J's own steps only.
- **I7.** Every id is bounded; ancestry is a field, never a longer name.
- **I8.** D's observations are the ones its final plan used.
- **I9.** Each effect of D is installed at most once while D lives: its progress, its
  completion, its retention and the end of a run never make it missing.

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
live paths change. For review with part B.

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

Considered and refused: a decision that republishes R under R's own id instead of a
resend operation. It would publish R after D, against I1, and make the decision wait on a
delivery.
