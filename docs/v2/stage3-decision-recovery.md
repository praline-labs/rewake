# Stage 3: the decision's protocol, rules and tests

Part B of stage 3, built in S18: the records and steps derived from
[the model](stage3-decision-model.md), how decisions compose, the rules D1–D8, and the
tests whose oracle is the model. The subject, the authority, the command and the record
kinds are in [stage3-decision.md](stage3-decision.md); its running example — `api`'s
report R to `lead`'s run r, outcome unknown — continues here. Citations are to `b6ed4ed`.

The contract: **the resend is an independent, ordinary operation**; **a decision keeps
the observations it was made on**, and the live originals stay with their owners
(correction 9, [in its normative form](stage3-decision-model.md#correction-9-in-its-normative-form));
**each effect's installation is proven by something that lives as long as the decision**.
No decision waits on any other operation, so the dependencies form no cycle; the resend's
every state is E8's; nothing keeps a live path from its own writers.

## The decision's record

**The linearization point** is D's write. Under the sender's mailbox lock the plan runs
again — reading R's mark and letter under r's mailbox lock with the bounded wait it
already uses — and D is written only if R is still unknown. Evidence before that point
resolves the occurrence by S3's rule, and `decide` answers exit 1, "proven published" or
"proven not published". Evidence after it is M5: the decision stands, as the accepted
contract trades ([design-rules.md](design-rules.md#an-operator-decision-on-an-unknown-outcome):
decided wrongly, the report reaches r twice, or an obligation closes with nothing
delivered).

**The record**, `decisions/<R-id>` in the sender's mailbox, write-once (`PublishExclusive`,
the link and the directory synced): the id (a fresh `inbox.NewID()`), the choice, who
decided and whether verified, the time, `delivery: "unproven"`, the subject (J's
operation, R's id, the recipient's name and run, the waits R answers), the open
occurrences it disposes (S3, by occurrence id), the parent decision when R is itself a
resend, the runs to tell, the observations below, and for "did not arrive" **J' whole** —
its bytes as they will be written.

**The observations** are the plan's own reads, kept as the plan returned them, not read
again: J's bytes; every occurrence about R and its bytes; for every path an occurrence
names — R's `published` or `intent` mark (`inbox/once.go`), its letter in `unread/` or
`done/`, r's mailbox directory — the bytes or listing when the read returned, or the
error and whatever `lstat` returned when it failed; the SHA-256 of every byte string. A
letter is bounded by the message size cap, a record by its own; the observations by their
sum. Nothing is kept of what could not be read.

## Installing the effects

Each effect is installed by `decide` and by any barrier that finds it not installed, and
each is recognised by an identity that its progress and retention never change.

**1. The resend J'** ("did not arrive"). Its name is fixed by D: `journal/decided-<D>`.
Under the sender's mailbox lock the barrier looks for `journal/decided-<D>.done` and, not
finding it, links J''s bytes from D at `journal/decided-<D>` with `PublishExclusive`; the
name existing in either form is installation, whatever the bytes inside it say now. J'
then runs as every journal does: it saves its steps (`Published`, `Moot`,
`journal_held.go:24-35`), writes its done form in place and is renamed to `.done`
(`journal.go:157-167,223-227`) — under the same lock, so the barrier never sees it between
names. **Retention**: `sweepTurnRecords` (`turn_records.go:23-58`) removes done journals of
ended runs on the premise that only the originating run retries; a decision breaks that
premise, since D can install J' again at any later barrier. So a done journal named
`decided-*` is kept while the mailbox lives, as 1.x kept its conversion journal for good
(`:27-28`). With install and proof one atomic link, a crash leaves J' installed or not, never
installed without its proof. A `stat` of either name that fails with anything but "no
such file" is an occurrence about the effect (S3), never an install.

**2. The notes.** To each run D names to tell — r and main's run, each when it did not
decide — one note, published once through that run's once record keyed by D's id
(`PublishOnce`, `once.go`): idempotent while the run lives, since a live run's once marks
are not swept (`sweepOnce`, `:205-216`), and refused as moot once it has ended. After the
answer — written now, written before, or moot — the barrier records
`decisions/<R-id>.told-recipient` or `.told-main` (write-once), and later barriers stop
asking. A crash before that record repeats a `PublishOnce` that answers "written before",
or moot. A mailbox that cannot be searched leaves the note owed to a later barrier and
stops nothing: a note answers no wait.

**3. J goes on** at its next barrier, as any unfinished journal does: it reads
`decisions/<R-id>` before R's live evidence, records R in a new list of the journal,
`Decided`, beside `Published` and `Moot`, and runs its tail by M4. This is J's own
progress, not an effect D installs.

No progress marks: each effect is found installed by its own identity, and none depends on
another's being done. "Did not arrive" closes R's step in J at once; R's waits are cleared
by J's tail with the rest, while J' — an unfinished journal of the same mailbox — is
finished by every barrier before a wait is read again, the guarantee an unfinished report
has today.

## Composition

- **Several subjects in one mailbox** — several reports of one unsearchable recipient,
  several reports of one journal, a report and its resend — are decided one by one; each
  disposes only its own occurrences; the stop holds while any occurrence is neither
  resolved nor disposed.
- **One journal, two unknown reports**: D1 closes R1 (and installs J1' for "did not
  arrive"); J still waits on R2, stopped on R2's occurrence; D2 closes R2; J's tail runs.
  Neither decision waits on the other or on J.
- **A decision on a resend**: R' unknown is an occurrence about J'; D2 is
  `decisions/<R'-id>`, `parent: D`. "Did not arrive" again installs J'' under D2's own
  id; D's J' stays, open until D2 closes its step, then done and retained; D's barrier
  finds it by name and installs nothing. The chain is the `parent` fields, shown in every
  rendering: "third copy, decided by lead after D".
- **A cause after a decision.** D disposes occurrences, not keys. The sender's recovery
  reads D before R's evidence, so it records no new occurrence about R; an occurrence
  found later about anything else — J itself unreadable, a path of no known kind — is a
  new one, resolved or decided on its own (S3).
- **A turn end retried across a decision**: the retry finds J, reads the decision, and
  closes the step — never publishes R.
- **r ends** before J' publishes: R' is moot by the ordinary rule, main told by J'. If R'
  turned unknown first, its occurrence stays, since r's end resolves nothing (S3), and only
  the person can decide it.
- **The sender's run ends** with an effect not installed: the mailbox is the name's, and
  the next changing call on it — a later run of the name, or `decide` — installs it.
- **Retention**: decision records, their `told` records, stop occurrences and their
  resolutions, and every journal named `decided-*` are kept while the mailbox lives; J
  follows the ordinary retention of journals, which the kept observations make safe.

## Its rules

Written into `docs/rules/effects.md` in S18, each with its tests:

- **D1.** A decision is a choice under an unknown outcome, never evidence: nothing derives
  delivery from it, and every effect taken on its authority carries its id. Ordinary
  activity in the recipient's mailbox follows its own owners and needs no knowledge of it.
- **D2.** Only the subject's recipient run, vouched for by its wrapper — the same run
  after a compaction — or the person, once every identity check has read and found no
  session, decides; the record names who and whether verified; the recipient run and
  main are told of a decision they did not make.
- **D3.** A decision needs a subject read from a readable journal and decides that
  subject only. Its write-once record, kept while the mailbox lives, holds the subject's
  identity and the observations its final plan used — bytes and their hashes for every
  read that returned, the error and metadata for every read that failed — and nothing of
  what could not be read.
- **D4.** A decision is written once, under the sender's mailbox lock, after the plan
  finds the subject still unknown there, before any effect, with the whole resend in it;
  the same words answer it, the opposite words are refused.
- **D5.** "Arrived" closes the report's step; "did not arrive" closes it and installs the
  resend. The sender's recovery reads the decision before the report's live evidence,
  never publishes the report after either, and never changes the decision or cancels the
  resend on later proof.
- **D6.** A resend is an ordinary report operation with a fresh, bounded id and its
  ancestry in fields: its outcomes are E8's, and when unknown it is a subject decided
  with the first decision as parent.
- **D7.** Each effect of a decision is installed at most once while the decision lives,
  recognised by an identity its progress and retention do not change: the resend by its
  name in either form, kept while the mailbox lives; a note by the told run's once
  record, moot once that run has ended. A progressed, completed or retired effect is
  never installed again, never a new obligation and never a stop.
- **D8.** Evidence that returns after a decision changes nothing taken or recorded on the
  sender's side.

## The tests

**The oracle is the model.** Written in the test from
[the model](stage3-decision-model.md#the-expected-outcome-of-a-world), not from the code,
it takes the world the test built — every copy's truth, which the test fixes by placing
or withholding letters, the observations it makes fail, the choices it scripts, the runs
it ends — and a schedule, and predicts the copies r holds, which steps are closed and by
what, the waits, the notes, the exit codes, the records, and that no effect on D's
authority lacks D's id and nothing derives delivery from D. The dimensions, a dimension
that does not apply counted once; the model's excluded combinations are never built:

| Dimension | Values |
|---|---|
| R's truth | landed, not landed |
| R's observation at the decision | unknown: a mark that does not read, a mailbox that cannot be searched, an effect met |
| R's observation later | stays unknown; returns published (landed only); returns absence proven (not landed only); returns before D's write |
| the choice; a second command | arrived, did not arrive; none, the same words, the opposite |
| who decides | r verified; the person, r alive or ended |
| R''s truth and observation | every pair the model allows: none then published; absence proven then published; unknown at the intent, at the letter, at the published mark, each with R' landed and not; moot |
| the second choice, R' unknown | arrived, did not arrive |
| the runs | r alive; r ended before D, after D before R' writes its intent, after R' turned unknown; s alive, ended before D's effects are installed, ended after |
| activity in r's mailbox | none; r reads R; the sweep promotes R's intent and removes its letter; a new run of r's name drops old once marks |
| a second report in J | none, undecided, decided arrived, decided not arrived |
| generations | one; two, with all four combinations of R''s truth and the second choice; a chain of 40, each unknown and decided "did not arrive" |

**The effects' lifetime.** In each world the barrier runs again after each state of each
effect: J' open, after each of its saves, done in place, renamed `.done`, after both s
and r ended, after its retention sweep with every age forced past the cutoff, after a
child decision on R'; a note's once mark swept with its run. Each time: nothing installed
again, no new obligation, no new stop occurrence, the copies unchanged.

**Crashes.** Every world runs clean in the five checks; every world also runs with a
crash at each durable write of its clean run's log, as `bridge/server/fault_test.go`
does — a clean run with the seam's log (`rewakefault`), then one case per write, the
process killed there, the barrier run again, the oracle checked: in the five checks for
the worlds covering every pair of dimension values, for every world at S18's commit and
at the stage's acceptance. The writes include D's link and directory sync, J''s link,
each note's once record and each `told` record, each of J's saves, every write of J' as
an ordinary journal, and its rename. The chain of 40 asserts every path's length is that
of the first generation.

**The observations.** A test seam hands the test the plan's reads as the plan returned
them; D's observations equal them, and agree with the world — a hash for each byte
string the test placed, the error kind for each read it made fail. Then the live paths
change — r reads, its run is replaced, its sweep runs with every age forced past
retention, J reaches its done form and is swept — and D still verifies against its own
hashes. **Capture against r's activity**: r's read and r's sweep racing `decide`, in both
orders — the oracle accepts D made on an unknown, or `decide` refusing as proven because
the sweep promoted the intent first, and nothing else; no observation in D mixes two
states of r's mailbox, which the plan reads under r's lock.

**Authority**, with real processes. A process below the recipient run's wrapper decides,
verified. **After a compaction**: the fixture session compacts through its Control, then
a new shell child of the same wrapper decides, verified, recorded as the same run. **A
lost and reconnected capability**: the fixture's socket drops after a turn
(`RW_SHIM_DROP_AFTER`) and reconnects, and a child still decides, verified, with the run
unchanged. **A wrapper restarted under the same name**: refused, naming the person. **A
real non-session shell**: started through `setsid` and a double fork with a clean
environment, so its real ancestry to PID 1 is read; it decides as the person,
unverified, recipient and main told once — or, where the test's own read of that chain
finds an ancestor it cannot read, it is refused naming it. That refusal proves the path
fails closed, not that it succeeds, so the test logs which outcome ran and S18's review
reports it; the successful person's path is exercised in any case on `grantauthtest`'s
process fixtures with every ancestor readable. Refused, each naming why: a process below
another session's wrapper with `REWAKE_SESSION` set to the recipient; one with the
variable unset in its own shell; the recipient run ended; the room's main for a report to
another session; an ancestor's `stat` or `environ` unreadable, the registry unlistable, a
session record unreadable — each produced on purpose with `grantauthtest`'s fixtures
beside the real-chain case; other namespaces.

**Races.** Two `decide` calls at once, same and opposite words: one record, the other
answered or refused by it. `decide` against a turn end's barrier and against a `retry`,
in both orders. `decide` against the evidence returning: either the decision refuses as
proven, or it stands and the evidence changes nothing — the oracle accepts both and
nothing else.
