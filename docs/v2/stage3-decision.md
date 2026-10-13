# Stage 3: the operator decision

Part B of stage 3 ([stage3.md](stage3.md#two-acceptances-part-a-and-part-b)): the design
of answer 4 of [design.md](design.md#the-owners-answers), a command that lets the
report's recipient or the person record "arrived" or "did not arrive" for a report whose
outcome is unknown. The contract is accepted
([design-rules.md](design-rules.md#an-operator-decision-on-an-unknown-outcome)), with two
corrections: 8, below under [authority](#authority), and 9,
[in the model](stage3-decision-model.md#correction-9-in-its-normative-form). The model —
the world, its outcomes and what a decision is answerable for — is in
[stage3-decision-model.md](stage3-decision-model.md); this file designs what can be
decided, who decides, the command and its records, and what every reader consults; the
protocol, the rules and the tests are in
[stage3-decision-recovery.md](stage3-decision-recovery.md). Built in S18, on S3's
occurrences.

The running example: the worker `api` owes main `lead` a report. `api`'s turn end writes
its journal and publishes report R to `lead`'s run; whether R landed cannot be proven —
R's `published` mark does not read, or `lead`'s mailbox cannot be searched. A stop
occurrence about R's operation is recorded in `api`'s mailbox (S3). `lead` can look at its own
inbox and say whether R is there.

## What can be decided

A decision has a **subject**: one report and its operation, identified from a journal
that reads — the report id, the sender's run, the recipient's name and run, the waits and
obligations it answers, the journal operation (run and event, E7) that carries it. A
resend is an ordinary journal operation (`journal/decided-<D>`), so an unknown resend is
a subject of the same kind, read from its own journal; its decision names the first as
its parent ([the model](stage3-decision-model.md#the-transitions),
[composition](stage3-decision-recovery.md#composition)).

Every report kind an ordinary turn journal carries can be a subject — a finished report,
an interim or stopped one, a failure report to the sender's own name with no waits — and a
decision closes that report's step only: the journal's tail keeps its own `Clear`, and an
interim decided is never turned into the work's completion. A subject is named by its
full report id and checked against the sender's run, the recipient's run and the journal
operation — the report's own sender run included, which must be the journal's: a report
the journal carries with another sender run, or none, is no subject; two journals naming one report, or a report whose step is already published,
moot or decided, are no subject (Q5 of
[stage3-decision-build.md](stage3-decision-build.md#q5-subject-scope)). A subject that
reads is listed and decidable even while other records of the mailbox do not read.

A subject is tied to the open stop occurrences about it: an occurrence names the
operation it is about when one is known (S3), and the decision disposes of exactly those
occurrences, by their ids. An occurrence of the same key found after the decision is a new
one, which this decision does not dispose of.

**Several unknowns, several subjects.** A recipient mailbox that cannot be searched
leaves every report published into it unknown; each is its own subject, decided on its
own, and deciding one disposes of no occurrence about another. A journal that carries
several reports gives one subject per report.

**No subject, nothing to decide.** A cause whose only source of identity is the
unreadable record itself — a journal that does not read, a file of no known kind — has
no subject, whatever number of reports it may carry. **Decided by the owner on
October 5, 2026** (answer 3 of [stage3.md](stage3.md#the-owners-answers)): no command
lifts such a stop; `decide` lists it as undecidable, naming each record and why nothing
can decide it, and the person repairs the file. The diagnostic does not suggest that a
new name or room settles the old obligations, because it does not.

## Authority

**Decided by the owner on October 5, 2026** (answers 4 and 5): verified authority is
bound to the subject's recipient run; a restarted main under the same name, or the
room's main for another session's report, does not decide — the person does; the
person's path stays, unverified, with every failed or unreadable identity check
refusing.

**Verified: the subject's recipient run — correction 8.** The accepted contract names
"the report's recipient — main"
([design-rules.md](design-rules.md#an-operator-decision-on-an-unknown-outcome)); read
literally, the run the report was addressed to, verified through its own wrapper,
whatever its role. Decided by the owner as above, with the condition below. The caller must run below the wrapper of the run the report
was addressed to — the subject's recipient name and epoch — and that wrapper, not the
caller, makes the decision's final transaction (Q2 of
[stage3-decision-build.md](stage3-decision-build.md#q2-authority-at-the-linearization-point)).

1. The subject gives the recipient run: name and epoch, the epoch encoding the
   wrapper's pid, start time and boot (`registry.ParseRun`, `core/registry/run.go`).
2. `decide` connects to that run's authority address (`state.AuthorityAddress(roomDir,
   run)`, `infra/state/paths.go`, which hashes the canonical room directory and the run;
   the grant scheme's, [grants-authority.md](../grants-authority.md#the-scheme)). Before
   S18 only a main's wrapper listens there (`host/wrap.go`, set up in `host/grants.go`);
   from S18 every wrapper does, offering a non-main run the decision request only, never
   a grant's registration, carry or confirmation (`authority.ListenDecisions`). A run
   whose wrapper has ended has no listener: refused.
3. The wrapper checks the caller as the grant scheme's step 1 does: the caller's uid and
   pid from `SO_PEERCRED`, its chain of parents up to the wrapper, each no younger than
   its child, the same mount, user and PID namespaces. The caller checks the wrapper in
   turn — its uid, pid, process start, boot and namespaces from the connection's peer
   credentials, never only the run string it answers — and that it is the subject's
   recipient run.
4. The wrapper then runs the decision's final transaction itself, through a handler the
   host supplies, under its own lease and lifetime: the locked plan, the record and its
   effects (Q1, Q4). The shell holds no mailbox lock while it asks. A wrapper that is
   shutting down refuses new requests — from the moment its harness ends, before any of
   its teardown — and a transaction it took finishes before its last write, as every
   host callback does under S17. So no verified decision is written after the recipient
   run ended: the run that vouches is the process that writes.
   The asker is `authority.Decide`, the wrapper's side `host/decide.go` over
   `mail.Decide`; a wrapper runs at most four decisions at once and refuses the rest
   busy, the answer carries no observations — the record on disk does — and a question
   that went out with no answer back is uncertain, naming the record that may exist.
   `test/decisionrig` holds this between real processes: the recipient's command is
   answered whatever its environment says; another worker's, with the recipient's
   variables or without, through the client or straight at the address, the
   recipient's own in a user namespace of its own, a listener the run does not name, a
   grant at a worker's address and the run's successor under its name are refused with
   nothing written; and a wrapper killed at the client's step after it checked the
   wrapper, at `decision admitted`, waiting for the sender's mailbox, or at
   `decision answered` leaves the asker uncertain and a record only if it wrote one
   before it died. A harness that ends during an admitted decision gets a later
   question refused busy, and the run keeps the room's lease until the admitted one is
   done.

**A compaction does not take the authority away** — the owner's condition. The run is
the wrapper's: `registry.Session.Epoch()` is the wrapper's pid, its process start and
the boot (`core/registry/run.go`), not a turn, a context window, the harness's model
process or a tool call. A compaction replaces none of these, so the run that compacted
is the same run and its shells and tool children still decide, verified. Nor does a
capability lost and reconnected (S11): liveness is a property of the run, not its
identity. Only the wrapper's end ends the run; a later launch of the name is a new run.
`test/decisionrig` asks after a compaction the fixture's program reported and the
wrapper counted, and after the program's link dropped and came back, and the run
decides both, verified.

`REWAKE_SESSION` is never trusted: a worker that sets it to `lead` runs below its own
wrapper and is refused, where 1.x `settle`, removed in S2, trusted the variable.
A later run of the recipient's name, or the room's main for a report addressed to
someone else, is not the recipient run and is refused, naming the person.

**The person.** A caller with no verified authority may decide as the person only when
every check positively finds it outside every session:

- no ancestor in its `/proc` chain, up to pid 1, has `REWAKE_SESSION` in its initial
  environment (`/proc/<pid>/environ`), so a process that unset the variable in its own
  shell is still seen below its session;
- no ancestor is the wrapper of any session registered under the base, any room — that
  is, the v2 registrations of every room of the selected v2 root, once S17's supported
  upgrade has run; the environment check above stays independent of them;
- each of these reads succeeds: an ancestor's `stat` or `environ` that cannot be read, a
  registry that cannot be listed, a session record that cannot be read — each refuses,
  exit 1, naming what could not be read. Failing to find a session is not proof of the
  person.

Decided by main on October 12, 2026, for the processes of another user: the kernel keeps
their `environ` and namespace links from this user — PID 1 is root's on every ordinary
machine, and so are a login daemon and `sudo` — so read literally the rule above would
refuse every caller and leave no person's path at all. An ancestor of the caller's user
has its `stat`, its namespaces and its initial environment read, and one that does not
read refuses. An ancestor of another user has its `stat` and its real and effective user
(`/proc/<pid>/status`) read positively, and one that does not read refuses; its
environment and namespaces are not required. The variable and the wrapper reach a chain
only through processes of the user who started the session — the wrapper runs as that
user and is matched by its pid and start, the harness it starts is given the variable —
and those are read: a root process below a session, `sudo` say, still has the session's
harness above it. The limit: a session started wholly under another user keeps another
state directory and is not in the selected root. The scan is `person.Classify`
(`core/person`).

The checks read a strict inventory, not the forgiving views (Q3 of
[stage3-decision-build.md](stage3-decision-build.md#q3-the-persons-path)):
`registry.ListReadOnly` skips a record that does not load and `WorkDirs` discards its
errors, so neither may answer. Only a registry or a rooms directory missing for real is
empty: each is listed without following a link at its own level or the room's
(`state.ReadDirBelow`, `state.ReadPlainDir`), so one put behind a link, dangling or to a
directory elsewhere, is unreadable — for the person's path and for the main a decision
freezes its notes from alike. The chain is read from the caller itself up to PID 1, each
ancestor's identity kept apart as live, ended or unknown; a pid whose start changed
between reads, or namespaces and parents that disagree between reads, make the scan
restart, and a scan that does not settle within its bound refuses. It takes no lock or
lease in another room and runs no repair of 1.x state. A failed request to a wrapper is
never proof of being outside the sessions. Unreadable identity is exit 1; a caller found
positively below a session that may not decide is exit 2.

The decision is then recorded as the person's, **unverified**, and both the recipient
run and the room's main are told once. The accepted limit: a process that left a
session's tree through `setsid` and a double fork, with a clean environment, passes these
checks and is taken for the person; without the path a room whose recipient run is gone
would have no way out. `test/decisionrig` shows both sides on real processes: a
session's command is below it with its variables unset, in a group of its own, and with
a clean environment there too — its harness places it — while the double fork with a
clean environment classified as the person on October 12, 2026, adopted by a process
outside every session; the rig logs what it classifies as, since that depends on the
adopter.

Every process of the user can write the state directory, so a decision record written by
hand is no more prevented than a forged letter. The authority keeps an honest worker —
or one following a note's line — from deciding its own stop; it is not a defence against
a process set on forging records.

A caller who may not decide is refused before anything is read under the lock, exit 2,
naming who may: "report R of api is decided by lead (run 4242.…) or by the person; this
process runs below the wrapper of api."

## The command

**Decided by the owner on October 5, 2026** (answer 2): `rewake decide <name> <report>
--arrived | --not-arrived`, and `rewake decide <name>` to list.

- **Its place.** A group of its own after the mail commands, so the guide shows it with
  its notes; not on the tool surface (answer 2's set is `inbox`, `send`, `pending`,
  `whoami`, `retry`, `list`). The note that tells of a stop occurrence ends with the two
  lines to run, addressed to whoever may decide.
- **Forms.** The list form is a view, the deciding form a change, classified in the
  command table when S18 adds them ([stage3-state.md](stage3-state.md#what-is-a-writer)).
- **Naming.** By the stopped session and the report id, as the note prints them; the
  answer adds the operation, so two reports are never confused.
- **Listing.** One line per subject: the report, its recipient and run, what it answers,
  the path whose reading failed and why, whether decided, and the parent decision for a
  resend; then the undecidable occurrences. It reads only the sender's mailbox — a decision included after its
  journal's retention — and takes no lock, lease or wrapper request, installs nothing and
  runs no barrier: what a decided subject's recipient holds now is not looked up. A
  decision seen before its effects shows as recorded with its resend, tail and notes
  pending; a journal missed while it is renamed is looked for again under both names, and
  otherwise shown in progress, never as not installed or delivered (Q10).
- **Answer.** One line: `recorded: report R of api to lead did not arrive — decided by
  lead (verified), unproven; sent again as R'; api goes on.` When other causes remain:
  `...; api stays stopped on R2, decide each the same way.` Same words again: `recorded
  before as D at <time>; ...`, and the barrier runs. A resend that has completed since is
  not sent again: the answer says what its form keeps
  ([the completion summary](stage3-decision-recovery.md#installing-the-effects)) —
  `sent again as R' (published)`, `(moot: lead's run ended; whether it landed is not
  known)`, `(unknown, decided by lead as D2)`; `(in progress)` from an open form;
  `(completed, outcome not kept)` from a done form without a readable summary. A
  completed name alone never reads as published. On a stopped mailbox the answer adds
  that the resend and `api`'s tail run once the stop is settled.
- **`--json`.** `{session, report, operation, decision: {id, choice, by: {kind, name,
  run, verified}, at, delivery: "unproven", parent}, recorded, resent, stopped,
  remaining, undecidable}`; the list form `{session, subjects: [...], undecidable: [...]}`.
- **Exit codes.** 0: recorded, or recorded before with the same words — also when the
  mailbox stays stopped on unrelated causes, which the answer names under `stopped`. 1:
  the mailbox is busy, and no decision was written; a decision with the opposite words
  exists; the outcome was proven since; an identity check could not be read; an effect
  failed after the record (`recorded: true`, the failed effect named, the next barrier
  does it). Notes still owed are named as deferred, never as told, and fail nothing. 2: a
  wrong call — no or both switches, a report that is no subject, a caller found
  positively below a session that may not decide.

## The records

All in the stopped mailbox, `<base>/v2/rooms/<room>/inbox/<name>/`, **write-once**
unless the row says otherwise: written to a temporary name and linked into place with
`state.PublishExclusive` (`infra/state/state.go`), which fails if the name exists, through
S17's write seam under the room's build lease. Each kind joins the one list of record
kinds (`core/mail/records.go`), and one strict reader per kind serves every reader below
— recovery, `stopState`, retention and the views (Q6 of
[stage3-decision-build.md](stage3-decision-build.md#q6-validity-disposal-and-unknown-records)).
A strict reader takes one JSON value, refuses a field its record has no place for, and
refuses anything after the value but JSON's own whitespace — a closing delimiter left
there included (`strictJSON`, `core/mail/decision_records.go`). Every record of the
mailbox is read as a regular file and nothing else (`state.ReadRegular`): the
installation runs before the reading that refuses other types, so a link in a record's
place is not followed, a FIFO is not waited on under the mailbox lock, and the type is
checked on the file opened, so a name swapped after its listing is read as it is then;
`decisions/` itself is looked at with `lstat` and never followed, and every directory of the
mailbox — the stops a decision disposes of among them — is listed as a directory of its
own (`state.ReadPlainDir`), so a link in its place is unreadable, not followed to records
that are not the mailbox's and not read as absent when it dangles. A decision's observation
of a listing keeps its entries even when there are none: an empty listing is kept apart
from none.

A journal named `decided-*` is read by the same strict rule, open and done
(`parseResend`, `core/mail/decision_resend.go`): its envelope is the operation its name
fixes on a run, no turn end's tail, and either the decision's one report with progress
about that report alone, or done with its steps emptied and only then a summary. What the
installation, the plan and the views find under a resend's name is held to the decision as
well — its sender's run, the report it holds while open, that report in a readable summary
once done. Bytes of another kind or another resend there are an unknown, kept as they
are: the name stays installed and its bytes are never completed, written over or taken for
the resend. The summary alone is apart from the envelope: one that does not read leaves a
done resend done — "completed, outcome not kept", `ResendView.SummaryErr` — and stops
nothing.

| Record | Path | Written when | Holds |
|---|---|---|---|
| a stop occurrence | `stops/<key>/<occurrence>` | a plan finds a key with no open occurrence (S3) | kind, the paths it rests on, the operation, the paths present and absent, the time |
| its resolution | `stops/<key>/<occurrence>.resolved` | the plan decides its effect from evidence (S3) | the evidence |
| a decision | `decisions/<report-id>` | at its linearization point, before any effect | id, choice, by, at, `delivery: "unproven"`, the subject, the parent decision if any, the exact occurrences it disposes as (key, occurrence) pairs, the notes to tell whole, the observations, and for "did not arrive" the resend journal whole |
| a note told | `decisions/<report-id>.told-recipient`, `.told-main` | after the note's publication answered written, written before, or the run proven ended | the run told and the answer |
| a decision made durable | `decisions/<report-id>.durable` | by the installation, after D's file, `decisions/` and the mailbox flushed, before D's first effect | the decision and when: the evidence that D may authorize its effects, which the disposal, the installation's evidence and the plan go by |
| a closure | `decisions/<report-id>.closed` | by J, after its tail's effects and before its done rewrite | the decision, J's name and operation, the report, and the waits J's tail cleared that the report answers: closure, never delivery |
| the resend | `journal/decided-<decision-id>`, later `.done` | the decision's first effect; saved by its own steps afterwards | an ordinary journal (one report, `resends`, `decision`, no tail of its own); its done form keeps the completion summary — the id of R', its disposition, D, a child decision's id — and is kept while the mailbox lives |

An occurrence named by a valid decision is disposed: no separate record says so, and no
`.resolved` record is made for it. A decision that does not read is an undecidable cause
of its own and disposes nothing. Stop occurrences, their resolutions, decisions, their
`told` and closure records and the resend journals stay while the mailbox lives; no
sweep removes them.

**What a decision keeps of the originals** — the journal that names the subject, the
subject's marks and letter, every occurrence about it and every path one names — is the
observations its final plan used: bytes and hashes where a read returned, the error and
`lstat` where it failed ([the model](stage3-decision-model.md#correction-9-in-its-normative-form),
[the record](stage3-decision-recovery.md#the-decisions-record)). After the record, the
live paths go on with their ordinary writers in both mailboxes.

**What every reader consults:**

| Reader | Consults |
|---|---|
| the barrier (`core/mail/reconcile.go`) | first, before the plan and whatever the plan will meet, each decision with an effect not installed — its resend's name in neither form, a note with no `told` record — and installs it, an installation it cannot read being an occurrence of operation `.install`; then the plan, which runs the installation again read-only, so the gate predicts it, each journal reading the decisions for its reports and the open occurrences about them before the liveness check; then, only when the plan meets no unknown, the unfinished journals and their tails; then `stopState` |
| `stopState` and every caller of it (S3's list) | occurrences without a resolution and not disposed by a valid decision: one that names the occurrence exactly and is about the occurrence's own report, the occurrence's record and any resolution beside it readable (`disposals`, `core/mail/decision_view.go`) |
| the mailbox's sweeps | skip the paths open occurrences name; a disposed occurrence protects nothing |
| the retention of journals (`sweepTurnRecords`) | keeps every journal named `decided-*` in every form, whoever's run ended |
| a journal's recovery — publishing a report, or writing a letter again for an intent with none (`core/mail/journal_held.go`, `publishMarked`) | the decision for that report first: a decided report's step is closed, never published |
| `inbox`, `awaited`, `owed`, `list`, through one read-only model (`Decisions`, `core/mail/decision_view.go`) that takes no lock and changes nothing | decisions, closures, resend fields and the completion summary, to say "decided, still owed" while J's tail has not run, "closed by decision D, delivery unproven" once its closure is recorded, "sent again by decision of lead" and the resend's disposition as its form keeps it — moot never as "not delivered", a missing wait never as delivered |
| `decide` | journals, occurrences, decisions |
