# Stage 3: the decision as it is built

S18 builds part B of stage 3 — [the model](stage3-decision-model.md),
[the decision](stage3-decision.md) and [its protocol](stage3-decision-recovery.md) — on
the tree S1–S17 left, which changed much since part B was written at `b6ed4ed`: the v2
root, the build lease, views apart from changes, the write seam and `ErrNoLease` above
all. A readiness review on the Codex side, October 11, 2026, listed what drifted and
fourteen questions the build had to settle. **Decided by main on October 12, 2026**: the
review is adopted in full — its corrections, its recommendations to Q1–Q14, its
acceptance bar and its build sequence — with the choices it left to main written below.
Where part B and this file disagree, this file wins, and part B's text has been corrected
to agree with it.

## What drifted

Part B's citations named 1.x paths. They now name these, under `internal/`:

| Part B named | Now |
|---|---|
| `inbox/…`, `inbox.NewID` | `core/mail/…`, `mail.NewID`; the durable paths still spell `inbox/<name>/…` |
| `state`, `proc`, `registry`, `grantauth`, `grantauthtest` | `infra/state`, `infra/proc`, `core/registry`, `core/grant/authority`, `authoritytest` |
| the wrapper, `wrap/wrap.go` | `host` (`host/wrap.go`, its authority set up in `host/grants.go`), the endpoint `host/endpoint` |
| `bridge/server/fault_test.go` | `test/toolrig/fault_test.go` |
| the room's directory | `<base>/v2/rooms/<room>`: `state.Dir` is the room, `state.Base` the base; `state.AuthorityAddress(roomDir, run)` hashes the canonical room directory and the run, and decisions never go in S17's run directory |
| `turn_records.go`, its conversion-journal exception | `core/mail/journal_retention.go`, with no exception: S2 removed it |
| a typed "moot" answer of `PublishOnce` | `(bool, error)` with `ErrRecipientEnded`, which alone means ended |

What else changed under part B, and what the build does about it:

- **Run identity is not availability.** `core/registry/run.go` keeps `Epoch`, `RunEpoch`,
  `ParseRun`; S11's capability set can drop and reconnect without a new epoch.
  `Session.Alive` weighs the harness as well as the wrapper, and `ObserveRun` tells alive,
  ended and unknown apart: a false is no proof of an end.
- **The forgiving views are no inventory.** `registry.ListReadOnly` skips a record that
  does not load, and `WorkDirs` discards errors; neither serves the person's path (Q3).
- **The forms have a home.** `decide`'s two forms join `Command.Access`
  (`cli/access.go`): the list a view, the choice a change. `decide` stays off the tool
  surface, retries included.
- **The lease is outermost.** A deciding invocation holds its room's lease through
  `Context.within` from before the handler to after its output; every process that
  writes holds its own, and none is taken inside a name, mailbox or receipt lock.
- **S3's section is built.** The recipient run, the evidence and the publication are read
  and written in one section of the recipient's lock (`core/mail/journal_held.go`,
  `core/mail/once.go`); `PublicationOf` takes that lock itself and is never called under
  it.
- **A plan takes no recipient lock**, and **a mailbox-wide plan can stop before any
  report**: `inspectMailbox` returns when `readMailbox` fails. So the plan is neither the
  decision's final observation nor its subject inventory (Q1, Q5).
- **Not every open occurrence stops the effects.** An unknown only an effect met lets a
  clean barrier run its operation through; the model's dimension 6 says so now.
- **Occurrences hold more than a cause**: `Present` and `Absent`, a validated
  resolution, and several per report; `heldReports` keeps one per report, a gate map, not
  the inventory a decision disposes from.
- **S15's owners.** The delivery server is `host/delivery`'s; `SweepFinishedLocked`,
  `SettleLocked`, `SweepAwaitingLocked` stay in `core/mail` with their four permitted
  callers (`internal/layout_callers_test.go`). `decide` calls none of them, widens no
  table, restarts no server and settles no letter. Settlement is deferred while a stop
  protects a path, never lost, and a disposed occurrence protects nothing.
- **Journal completion keeps `Held`** beside `Epoch`, `Op`, `Ended`, `Done`.
- **`PublishExclusive` has one fault point** before its temporary file; a link can succeed
  and its directory flush fail. The finer cuts the crash matrix needs are added to the
  seam, not assumed (Q4).
- **Nothing captures observations yet.** The seam reads but has no `lstat`, and
  `evidenceOf` rereads and keeps hashes, not bytes (Q12).
- **Recovery runs in `mail.EndTurn`, `CompleteTurn` and `Reconcile`**, reached by host
  callbacks and the command line; `rewake retry` retries a receipt, not a journal.

## The decisions

### Q1. The final plan and its lock order

The room's lease, then the sender's mailbox lock with a bounded wait; under it, D first,
then the subject read on its own — J's bytes, R's step and every open occurrence about R.
For a new D, the recipient's mailbox lock with S3's bounded wait, taken once when it is
the sender's own; R's final evidence is captured there and the section is kept until D is
published. The recipient's lock is let go before notes are installed or a barrier runs;
the sender's is kept for them. No name lock, pruning lookup, recipient barrier or lease is
taken inside. A lock not had within its wait is exit 1 and no D. Unrelated journals are
admitted by a separate pass, whose failure never hides a readable subject.

### Q2. Authority at the linearization point

A vouch followed by the wrapper's death and a child's write would record a verified
decision after r ended, which the model excludes. **Chosen**: the verified wrapper makes
the decision's final transaction itself, through a handler the host supplies, after it
has checked the caller's peer credentials and the subject's identity. It checks uid, pid,
start, boot and namespaces, and the caller checks the wrapper's the same way, not only
the run string it answers. The shell holds no mailbox lock while it asks; the wrapper
writes under its own lease and lifetime and refuses new requests while it shuts down; a
non-main wrapper offers the decision and nothing of the grants. This amends part B's
"vouch only" and keeps its stronger rule: no verified decision after r ended. Not taken:
an authorization that may finish after r's end.

### Q3. The person's path

A strict inventory over every room of the selected v2 root, and the caller's real chain
of ancestors, itself and PID 1 included: every candidate record and every `stat` and
`environ` read, unknown kept apart from ended, PID reuse and disagreeing namespace or
ancestry observations detected, an unstable scan restarted within a bound and then
refused. Not `ListReadOnly` or `WorkDirs`, no lock or lease in another room, no 1.x
repair. A failed request to a wrapper is no proof of being outside the sessions. **Chosen**:
unreadable identity is exit 1; positively unauthorized is exit 2; the clean double-fork
escape stays an explicitly unverified, accepted limit.

### Q4. The commit point, durability and retry

An exclusive link names the winner; only completed durability — D's bytes, its link,
its directories — permits effects. On uncertainty the answer is exit 1 naming the path
that may exist, and D is never removed or replaced. A later writer validates that exact D,
makes it durable again and installs its fixed effects: the same words work from D after
J's retention, opposite words never replace it, a same-word retry that would mutate
checks authority again. D's id, observations, resend and notes are never regenerated.
A missing, unreadable or malformed existing D is not a free name.

### Q5. Subject scope

Every report kind an ordinary journal carries, interim and stopped ones and a failure
report to self with no waits included; a decision closes that report's step only, and the
tail keeps its own `Clear`. An interim never becomes the work's completion. A subject is
named by its full report id, checked against the sender's run, the recipient's run and
the journal operation; a duplicate or conflicting subject, or a step already published,
moot or decided, refuses. Readable subjects are listed while other records do not read.
A subject that changed between the early lookup and the locked one refuses. D names exact
(stop key, occurrence id) pairs — never a key, a journal, a recipient or whatever the
gate map holds.

### Q6. Validity, disposal and unknown records

One strict reader per record kind serves recovery, `stopState`, retention and the views:
required fields, the subject agreeing with its path, the choice, the identities, the
observations' hashes, the resend's shape and bounded references are checked before D
gives any authority. A D that does not read or validate is an undecidable stop, never an
absent decision and never permission to publish R again. A valid D disposes only the
occurrences it names; unrelated damaged occurrences or resolutions stay unknown, and no
`.resolved` record is made for a decision. S3's effect-only recovery stays. A malformed
local record beside a self-addressed report stays a cause of its own after the report's
occurrence is disposed; no corrupt mail is exempted from the scan.

### Q7. Installation and its errors

Valid decisions are recovered before the stop gate and the plan, also when s has ended;
each effect is installed on its own, waiting on no admission or other decision. A resend
whose lookup or link failed is a named effect failure, not installed, and no journal runs
in that pass. An owed note is owed, never an E8 unknown that stops mail. A `told` record
that does not read is an undecidable cause, never taken for told. An existing resend name
proves installation only; contents of the wrong kind stop their own operation and are
never written over from D. **Precedence**: D written with unrelated E8 stops answers exit
0 and `stopped`; a failed effect, exit 1 and `recorded: true`; owed notes are named
deferred, never told.

### Q8. The resend's shape and retention

J''s report carries R's text, kind, addressed runs and `InReplyTo`, with a fresh
fixed-length id, `resends` and `decision`; J' has a fresh operation and no copy of J's
tail — no `Clear`, `Kept`, `Held`, interim or pending effect, no turn end time. Its
summary survives the done-in-place and `.done` forms, its installation identity and
retention survive a child decision. A done form without a readable summary renders
"completed, outcome not kept"; one whose envelope does not read stays unknown. A stat
success is no summary, and moot is no non-landing.

### Q9. Closure provenance

J's tail closing R's obligation must outlive J. **Chosen**: a bounded, write-once closure
per decided report, `decisions/<R-id>.closed`, naming D, J and the exact waits its tail
cleared that R answers, written by J after its tail's effects and before its done
rewrite, kept with D. It records closure, never delivery, and is no second installation
marker for J'. A crash replays the tail idempotently and finds it; no new wait is
cleared. Views say decided and still owed before it, closed by D and unproven after; a
missing wait is never read as delivered. A blocking `send --question` whose answer was
closed by a decision ends with a structured closed-by-decision, unproven refusal; no
report text is made up and no decision note is printed as the answer.

### Q10. The listing and truthful answers

`decide <name>` reads the sender's mailbox only, D after J's retention included; no lock,
lease, request to a wrapper, recipient lookup, installation or barrier. A D seen before
its effects shows recorded, with resend, tail and notes pending, apart from the resend's
disposition. A journal missed around its rename is looked for under both names, or shown
in progress — never as not installed, never as delivered. A corrupt record is named
undecidable. The text and JSON model gains fields of its own rather than overloading
`resent` or `stopped`; "resend installed" until it publishes, and "api goes on" only when
it does.

### Q11. Notes and their recipients

The recipient's and main's runs, and each note's content and id, are frozen in D when it
is written; equal destination runs get one note, and no later role or main retargets one.
A main inventory that cannot be read refuses a new D; a room positively without a live
main owes no main's note. The deciding run is skipped only when verified; the person's
path owes both. Notes are `Note`s; one to the sender's own mailbox goes through the lock
already held. Told is recorded only after written, written before or a proven end, and a
once mark swept after its run ended never reinstalls a note. "Arrived" authorizes no
change of a report's status or published marks.

### Q12. Observations and their bounds

Captured in order as the final subject plan consumes them, through the seam, never a
second snapshot or the plan's audit strings: repeated reads stay distinct; bytes that
failed to parse, directory entries, structured errors and `lstat`'s own outcome are
kept. The no-mixed-state promise covers the subject's evidence, ordered by its mailbox
lock. Bounds: 16 MiB a read, 64 MiB in all, 65,536 entries a listing; past one the plan
refuses before D, and nothing accepted is truncated. Capture and every durable step run
through S17's seam under a real lease, with no raw write or test-global bypass. A letter
counts as landed by its publication in the write log, not by a pre-write hook or a copy.

### Q13. The running host and what triggers progress

`Reconcile` stays the recovery owner. `decide` installs and then runs the normal barrier
once under the sender's lock; every barrier recovers decisions first; the wrapper's turn,
read and retry work serializes there. The delivery sweep and settlement never become a
second executor, nor take a sender's lock inside a recipient's section. With s dead or no
further event nothing progresses by itself: the same deciding command, repeated after a
repair, runs a barrier. A view never recovers. Every host-owned transaction and callback
joins S17's lifetime tracking through its last write.

### Q14. Acceptance in three parts

**Chosen**: three parts, each accepted on the Codex side before the next; S18 starts only
on accepted S17.

1. **Records, readers and recovery, no public decision writer**: these documents and the
   [manifest](stage3-decision-manifest.md); the strict decision, closure, told and
   summary records with their read capture, record kinds, stop disposal, retention and
   read-only models; installation before the plan, the journal's decision precedence, the
   resend's completion, the tail's attribution and the error policy, with the finer
   durability cuts they need. No production code writes a decision; tests build them
   under a real lease.
2. **Authority and the host's lifetime**: the strict person classifier and the all-role
   authority protocol, main-only grants kept; the verified commit owner, peer identity,
   bounded requests and S17's teardown; process and fixture tests of compaction,
   reconnection, run replacement, teardown and authority races. The command stays
   unavailable.
3. **The command, every view and the end-to-end gate**: the group, table, flags, examples
   and access classification, both forms, errors and JSON, every promised rendering and
   stop hint; then the complete world and crash manifest, the mutants, the real-process
   smoke, the suite and its controls, and the acceptance record.

## The acceptance bar

Each test asserts the model's outcome, not a replay of the code, and each has a mutant
it must kill; a compile failure, an unsupported capability or a skipped case is no kill.
"The tail once" means one logical closure: retries may repeat idempotent steps, never
close a later read or duplicate a report.

| Test | What it requires |
|---|---|
| `TestDecisionFormAccessAndToolExclusion` | the list and help change nothing, a foreign build warns once, choices need the lease, the tool surface and retry refuse |
| `TestDecisionLeasePrecedesAllLocks` | an incompatible build refuses with no write; an active decision holds the lease through its last effect |
| `TestDecisionFinalEvidenceIsLockedThroughCommit` | evidence, reads and sweeps on both sides of D; one protected final observation or a proven refusal; self and opposing mailboxes finish bounded |
| `TestDecisionFindsSubjectsBehindOtherUnknowns` | several reports and occurrences, an unrelated unreadable journal in both orders, interim and self-error subjects; only the named occurrences disposed |
| `TestDecisionAuthorityWithProcesses` | the recipient's child verified; others, spoofed or unset environments, restarted runs, wrong roles and namespaces refused; compaction and reconnection keep authority |
| `TestDecisionAuthorityEndsAtCommitBoundary` | the wrapper killed or stopped at every step to the commit; no verified D after its end |
| `TestPersonDecisionFailsClosed` | every room, every unreadable record, ancestor and namespace, PID reuse; real and fixture processes, their outcomes logged apart |
| `TestDecisionDurabilityAtEveryCut` | every cut through D, its effects, the told, closure and done records, after a failed flush as after a kill; D whole or absent, effects fixed |
| `TestDecisionPrecedesAllRecoveryEvidence` | every recovery path reads D before R's evidence; later proof never republishes R or changes D or J' |
| `TestDecisionInstallationBeforeAdmission` | ready J behind an unknown J' or unrelated journal, both orders; installation goes, the tail and waits wait, owed notes stop nothing |
| `TestDecisionEffectsSurviveAllRetention` | J' in every form and after every sweep and child decision, notes after their once marks go; nothing installed again |
| `TestDecisionCopiesFollowWorldTruth` | both truths against both choices, two generations, the chain of 40 at fixed lengths, landings counted from the write log |
| `TestDecisionResendKeepsPublicationContract` | S3's races, a first write across r's end, a lost save then moot, open occurrences kept, unknown at every read |
| `TestDecisionClosureKeepsProvenance` | old tails retried, J gone and still closed by D, interims never close a task, a blocked question's truthful end |
| `TestDecisionViewsDuringEveryPhase` | every phase paused; the views show only what is established and change nothing |
| `TestDecisionObservationsAreTheFinalReads` | D equals the plan's log; later changes leave its hashes valid; failures kept; oversize refuses |
| `TestDecisionCorruptionNeverAuthorizesEffects` | D, told, summary or J' of the wrong kind, unreadable or conflicting; installation recognised apart from parsing |
| `TestDecisionRacesWithWrapperAndDelivery` | concurrent choices, turn ends, retries, adoption, settlement and sweeps; no unlocked retirement, extra announcement or lock cycle |

The oracles already carried stay, among them `TestAdmissionIsInsideTheSection`,
`TestTheSweepWaitsForAnAttemptsEvidence`, `TestAKillAtAnyWriteLandsTheReportAtMostOnce`,
`TestAnUnknownOnlyAnEffectMetStopsUntilTheBarrier`, `TestTheLockedEntryPointsKeepTheirCallers`,
`TestAStaleSweeperRetiresNothing`, `TestASweepWithoutTheLockRetiresNothing` and
`TestADeferredSettlementGetsAnotherPass`. The stop readers and journal writers are
inventoried by typed references at the implementation, not by a grep.

**Evidence for each part**: the five checks green and the workflow suite on its last
commit, the crosswise check after a control changed; pair-covering crash worlds in the
ordinary checks, the full matrix before S18's final acceptance; counts, selection, exits,
logs and each mutant's failing assertion in the report. Real-process authority tests are
required; a main-run smoke of the installed binary in a private `/tmp` base exercises an
unknown made with the fault-enabled fixture, the list, a decision from the recipient's
child and from a detached person's process, recovery after the deciding process is
killed, and a refused restart of the same name. A person's chain that cannot be read is
kept as refusal evidence, and the successful path is then required on complete process
fixtures, never waived.

## Commits

The intermediate commits of a part build and vet, the tagged variants included; the last
commit of each part runs the five checks and the workflow suite
([stage3.md](stage3.md#what-holds-in-every-step)). Each commit carries its documents.
