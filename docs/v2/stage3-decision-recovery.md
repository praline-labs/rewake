# Stage 3: the decision's protocol, rules and tests

Part B of stage 3, built in S18: the records and steps derived from
[the model](stage3-decision-model.md), how decisions compose, the rules D1–D8, and the
tests whose oracle is the model. The subject, the authority, the command and the record
kinds are in [stage3-decision.md](stage3-decision.md); its running example — `api`'s
report R to `lead`'s run r, outcome unknown — continues here. Citations are to `b6ed4ed`.

The contract: **the resend is an independent, ordinary operation**; **a decision keeps
the observations it was made on**, and the live originals stay with their owners
(correction 9, [in its normative form](stage3-decision-model.md#correction-9-in-its-normative-form));
**each effect's installation is proven by something that lives as long as the decision**;
**E8's admission stays mailbox-wide**: a decision is recorded and its effects installed
while the mailbox is stopped, and everything that publishes or clears — the resend, J's
tail — runs only in a barrier whose plan meets no unknown. No decision waits on any
operation, tail or admission, so the dependencies form no cycle; the resend's every state
is E8's; nothing keeps a live path from its own writers.

## The decision's record

**The linearization point** is D's write. Under the sender's mailbox lock the plan runs
again — reading R's mark and letter under r's mailbox lock with the bounded wait it
already uses — and D is written only if R is still unknown. Other unknowns the plan
meets elsewhere in the mailbox do not stop the write: D needs R's evidence, not
admission. Evidence before that point
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

Each effect is installed by `decide`, right after D's write under the same lock, and by
any barrier that finds it not installed, and each is recognised by an identity that its
progress and retention never change. **Installation does not wait for admission.** The
barrier installs first, under the sender's mailbox lock and before its plan: a link of
bytes D fixed, at a name D fixed, and notes to other mailboxes — recovery writing its own
records and telling, which E8 allows in a stopped mailbox, never a publication of the
stopped mailbox's reports or a wait cleared. Then the plan reads everything, the new
journal included, and only a plan that meets no unknown runs the journals.

**1. The resend J'** ("did not arrive"). Its name is fixed by D: `journal/decided-<D>`.
Under the sender's mailbox lock the barrier looks for `journal/decided-<D>.done` and, not
finding it, links the bytes of J' from D at `journal/decided-<D>` with `PublishExclusive`; the
name existing in either form is installation, whatever the bytes inside it say now. J'
then runs, in an admitted barrier, as every journal does: it saves its steps
(`Published`, `Moot`, `journal_held.go:24-35`), writes its done form in place and is
renamed to `.done` (`journal.go:157-167,223-227`) — under the same lock, so the barrier
never sees it between names. Its `Moot` is the lifetime disposition of
[the model](stage3-decision-model.md#the-world): an attempt of J' read r ended inside its
publication's section (M10), not R' proven absent. **Retention**: `sweepTurnRecords` (`turn_records.go:23-58`) removes done journals of
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

**3. J goes on** at its next admitted barrier, as any unfinished journal does: it reads
`decisions/<R-id>` before R's live evidence, records R in a new list of the journal,
`Decided`, beside `Published` and `Moot`, and runs its tail by M4. This is J's own
progress, not an effect D installs.

No progress marks: each effect is found installed by its own identity, and none depends on
another's being done. "Did not arrive" closes R's step in J at once; R's waits are cleared
by J's tail with the rest, in the first admitted barrier, while J' — an unfinished journal
of the same mailbox — is finished by every admitted barrier before a wait is read again,
the guarantee an unfinished report has today. While the mailbox is not admitted, J is
ready and waits, its waits standing, as every ready journal of a stopped mailbox does.

**The completion summary.** A name proves J' was installed, not what came of R': the done
rewrite keeps only `Epoch`, `Op`, `Ended` and `Done` (`journal.go:164`), and D holds J'
as planned. So the done rewrite of a journal named `decided-*` keeps, beside those, one
bounded summary: the id of R', its disposition — published, moot or closed by a decision —
the installing decision D, and for the last the child decision's id. The open form is
read for its fields as they stand. A reader renders only what the form it finds keeps:
the summary from a done form; "installed, in progress" from an open one; "completed,
outcome not kept" from a done form without a readable summary. A name in done form is
never read as published.

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
- **r ends** before J''s first attempt reads it: R' is moot by M10, nothing written. After
  the letter of R' was written: `Published` if the attempt that wrote it saves it; if that
  save is lost, the next attempt reads r ended and records moot, and R' landed — the
  summary and every rendering say moot, never "not delivered". If R' turned unknown first,
  its occurrence stays, since r's end resolves nothing (S3): returning evidence may still
  close it, and only the person may decide it. r's end records no disposition by itself
  (M8).
- **r ends and its proof is retired** with an occurrence about R' open: the letter read
  and swept, r's marks dropped by the next run of the name, every read now succeeding with
  "no such file". That is unknown, not none ([the model](stage3-decision-model.md#the-world)):
  the occurrence stays open, no moot is recorded, nothing resolves it automatically, and
  the person's decision stays available.
- **A ready parent behind an unknown.** D recorded "did not arrive", J' installed, and J
  ready — every step published, moot or closed; then J' meets an unreadable mark, or an
  unrelated journal of the mailbox does not read. The plan meets an unknown, so nothing
  runs: J's tail waits and R's waits stand, in either order of the journals. The unknown
  about R' is a subject; its decision D2 is recorded and installed while the mailbox is
  stopped, needing only the lock, the readable bytes of J' and the unknown evidence of R'; once D2
  disposes the occurrence the next plan is admitted and J's tail runs. J's tail depends on
  D2, D2 on nothing J's tail does: a scheduling order, not a cycle. The unrelated journal
  is settled by its own evidence, or its own decision when it has a subject; with none,
  the stop stays for the person's repair, and J waits with it, as E8 requires.
- **The sender's run ends** with an effect not installed: the mailbox is the name's, and
  the next barrier on it — a later run of the name's, or `decide`'s — installs it before
  its plan, admitted or not.
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
  resend on later proof. The journal's tail runs, as any tail does, in the first barrier
  whose plan meets no unknown in the mailbox.
- **D6.** A resend is an ordinary report operation with a fresh, bounded id and its
  ancestry in fields: its dispositions are E8's, and when unknown it is a subject decided
  with the first decision as parent. Moot says only that an attempt found its run ended
  before another disposition was recorded, never that it did not land; its done form keeps a bounded
  summary of its disposition and decisions, and nothing renders more than a form keeps.
- **D7.** Each effect of a decision is installed at most once while the decision lives,
  recognised by an identity its progress and retention do not change: the resend by its
  name in either form, kept while the mailbox lives; a note by the told run's once
  record, moot once that run has ended. Installation waits on no admission; a progressed,
  completed or retired effect is never installed again, never a new obligation and never
  a stop.
- **D8.** Evidence that returns after a decision changes nothing taken or recorded on the
  sender's side.

## The tests

**The oracle is the model.** Written in the test from
[the model](stage3-decision-model.md#the-expected-outcome-of-a-world), not from the code,
it takes the world the test built — every copy's truth, which the test fixes by placing
or withholding letters, the observations it makes fail, the choices it scripts, the runs
it ends and when, the unknowns elsewhere in the mailbox — and a schedule, and predicts
the copies written for r, counted from the seam's log of writes so that a letter read or
swept still counts, which steps are closed and by what, the waits, the notes, the exit
codes, the records and their renderings, and that no effect on D's authority lacks D's
id, nothing derives delivery from D and no moot copy is called undelivered. The dimensions, a dimension
that does not apply counted once; the model's excluded combinations are never built:

| Dimension | Values |
|---|---|
| R's truth | landed, not landed |
| R's observation at the decision | unknown: a mark that does not read, a mailbox that cannot be searched, an effect met |
| R's observation later | stays unknown; returns published (landed only); returns absence proven (not landed only); returns before D's write; every read succeeding with no letter and no mark after r ended and its proof was retired, read without a section that admitted it with r live (unknown, either truth) |
| the choice; a second command | arrived, did not arrive; none, the same words, the opposite |
| who decides | r verified; the person, r alive or ended |
| truth of R' and its evidence | every pair the model allows: none then published; absence proven then published; unknown at the intent, at the letter, at the published mark, each with R' landed and not |
| the second choice, R' unknown | arrived, did not arrive |
| the runs | r alive; r ended before D, after D before the liveness check of R', between that check and its first evidence read with the next run's sweep started there ([the race](stage3-publication.md#the-race)), between its evidence read and its intent, between its letter and its published mark, after its letter and before J' saves `Published`, after that save and before J's tail, after R' turned unknown; s killed at each of the same points; s alive, ended before D's effects are installed, ended after |
| the rest of the mailbox | nothing else unknown; R' unknown with J ready; an unrelated journal that does not read, ordered before J and after it; either settled by evidence, by a decision, or left |
| activity in r's mailbox | none; r reads R; the sweep promotes R's intent and removes its letter; a new run of r's name drops old once marks |
| a second report in J | none, undecided, decided arrived, decided not arrived |
| generations | one; two, with all four combinations of the truth of R' and the second choice; a chain of 40, each unknown and decided "did not arrive" |

**The effects' lifetime.** In each world the barrier runs again after each state of each
effect: J' open, after each of its saves, done in place, renamed `.done`, after both s
and r ended, after its retention sweep with every age forced past the cutoff, after a
child decision on R'; a note's once mark swept with its run. Each time: nothing installed
again, no new obligation, no new stop occurrence, the copies unchanged.

**Admission.** A ready parent behind an unknown child and behind an unrelated unknown
journal, each in both orders of the journals, and each with a crash before the parent's
first tail effect: R's waits stand and nothing publishes while the unknown does; the
child's decision is recorded and installed meanwhile, `decide` exiting 0 on a stopped
mailbox; after the unknown is settled the next barrier runs the tail once and the waits
clear; left unsettled, the barrier answers the stop and the waits still stand.

**Moot and landing.** R' lands while r lives, s is killed before it saves `Published`, r
ends, the barrier recovers J': recorded moot by M10, counted landed, rendered moot and
never "not delivered". **The crossing write**, r ending between the liveness read and the
write, in both outcomes of the sender: s survives and saves `Published` — recorded
published, counted landed, no moot; s is killed before that save — the next attempt reads
r ended, records moot, counted landed. In both, one letter write in the seam's log. **A first landing across run end**: R' has
never landed, a retry of J' reads r live inside its section, and r ends before its first
evidence read. The reads find no letter and no mark: the evidence is none, not unknown,
the world is built and not excluded, and the attempt writes once — one letter write,
recorded published, no second liveness check. With
an occurrence about R' open, r's end leaves it open and records no moot; returning evidence
may close it, and only the person may decide it. **Retired proof**, for both truths of R —
landed and not — with an occurrence about R open after a failed evidence read: r ends, the
next run of the name sweeps the letter and drops r's marks, every read then succeeds with
"no such file". Expected: the evidence is unknown, never none; no moot; the occurrence
stays open across barriers and the stop holds; no resolution is written; the person's
`decide` is accepted, and the copies count by the truth. **The race of part A**, for a
resend: R' lands, s is killed before `Published`, r reads R' and its age passes the cutoff,
a retry of J' is paused at its successful liveness read inside its section, r ends and the
next run's sweep starts — the sweep finds the lock held and waits, the retry finds the
letter, one write of R' in the log ([stage3-publication.md](stage3-publication.md#tests)).

**Completion.** For each terminal disposition of R' — published, moot, closed by a child
decision — the rendering after the done rewrite of J', after its rename to `.done`, after its
retention sweep with every age forced past the cutoff, and after r's sweep removed the letter
of R': the disposition from the summary, unchanged; a done form whose summary does not
read renders "completed, outcome not kept"; no form renders published without a summary
saying so.

**Crashes.** Every world runs clean in the five checks; every world also runs with a
crash at each durable write of its clean run's log, as `bridge/server/fault_test.go`
does — a clean run with the seam's log (`rewakefault`), then one case per write, the
process killed there, the barrier run again, the oracle checked: in the five checks for
the worlds covering every pair of dimension values, for every world at S18's commit and
at the stage's acceptance. The writes include D's link and directory sync, the link of J',
each note's once record and each `told` record, each of J's saves, every write of J' as
an ordinary journal, its done rewrite with the summary, and its rename. The chain of 40 asserts every path's length is that
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
