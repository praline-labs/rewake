# Every record of a mailbox

Rule 8 of [mail-bridge-cli.md](mail-bridge-cli.md#the-rules-the-code-holds) stops a
mailbox on an unknown before any effect. That holds only if nothing an effect decides by
is left for the effect to find: an effect that meets an unknown after an earlier effect
of the same barrier went through has let that one through beside it. Acceptance after
acceptance found such an unknown late — an unreadable journal, a wait, a publication
mark, a kept answer, an interim record, then the letter searched for in the recipient's
stages, the mark of a note the barrier itself was about to create, and a read clock
nobody read. Each time the reading before the effects was a second, hand-written list of
what the effects read, and each time it missed one. So there is no such list any more:
the barrier runs its own effects as a plan first, and a test finds what the plan reads
by watching it ([below](#the-plan-and-the-seam)).

## The list

`recordKinds` in `internal/inbox/records.go` names each kind by its path under
`inbox/<name>/`, how a file of it is read, and what one that does not read leaves
unknown. The first pattern that matches a path is its kind.

| Path | Kind | Read as |
|---|---|---|
| `.lock` | the mailbox lock | nothing: it says nothing |
| `stops/<key>/<occurrence>`, `.resolved`, `.told` | an occurrence of a stop's cause; its resolution; that main was told | by the stop itself, below |
| `<id>.json`, `unread/<id>.json`, `done/<id>.json` | a letter | a message |
| `<id>.status` | a letter's delivery status | a status |
| `awaiting/<run>/.read-clock`, `.read-high` | a run's read clock and its highest place | one word, or none yet; a number |
| `awaiting/<run>/.turn-starts/<time>` | a turn start the host heard, one file per reading, the latest the largest | empty, its name a number |
| `awaiting/<run>/<peer>` | who a run owes a report | a wait, under a session's name |
| `answering/<id>`, `received/<id>` | a question waited on; the report printed for it | readable |
| `retention/<id>` | when a reserved report is released | its lifetime |
| `journal/<id>`, `journal/<id>.done` | a turn journal, a completed one | a journal |
| `pending/kept.json`, `pending/interim.json` | a held answer; a run's last word on the work | their records |
| `pending/marks/<run>/<time>` | a pending mark | a mark that says what its name says |
| `once/<run>/<id>` | a publication mark | `intent` or `published` |
| `threads/<id>`, `claims/<id>` | a delivery thread; a read in parts that claimed a letter | readable |
| `receipts/<run>/.<token>.lock`, `key-*`, `<token>.json` | a call's receipt lock, key and receipt | nothing; a token; a receipt that says what its place says |
| `receipts/<run>/calls/<call>` | the operation a tool call holds, by its native call ([mail-bridge-server.md](mail-bridge-server.md)) | a token whose receipt is in the same run |

A file named `.tmp-*`, in any directory, is a write that never finished
(`state.WriteAtomic`), never a record.

## The plan and the seam

The barrier, and every call that would change the mailbox by an effect — pending, a read
and its parts, an acknowledgment, the answer a question takes — first plans (`plan` in
`internal/inbox/reconcile.go`). The plan walks the whole tree: a file
that matches no kind, one that is not a regular file, one that does not read as its kind
says, a directory where a record belongs or where no kind's files lie, and a directory
that cannot be listed each stop the mailbox, named with what they leave unknown; a
directory is allowed only on the way to a kind's files, as each pattern's own
directories say, and a file that left since it was listed moved on and is none of these.
The walk reads every file of a kind it opens, the read clock among them, and hands the
bytes to that kind's parser: no kind can leave its file unread.

Then the plan runs the barrier's own effects, read-only: every unfinished journal, with
every write doing nothing and every
decision kept in memory. Whatever an effect decides by is so read before the first
effect, by the code that decides it: the recipient's publication mark and the letter in
each of the recipient's stages, the session registry that says whether the recipient's
run lives, a kept answer, an interim record, a wait. A plan that meets an unknown records the stop and nothing changes. Only a plan
that meets none lets the effects run, and they run the same code again on what they read
then: the plan is not kept as data for an executor to follow, as decided on October 1,
2026. A decision a race changes between the plan and the effects — a recipient that
ended in between — is taken on the effects' own reads, and an unknown met only
there is the stop of an effect, below.

Every file operation of the barrier and the gates goes through one seam, `fileAccess`
(`internal/inbox/access.go`), in both passes. A read that fails for any reason but a
missing file leaves what the file says unknown (rule 6: only a missing file is an
absence), whichever pass met it; so a path that cannot be followed, through a file where
its directory belongs or a directory closed to reading, is unknown, and so is anything
but a regular file where a letter belongs.

The session registry is not behind the seam, as decided on October 1, 2026: which runs
live is live state, which changes between any two looks. The plan asks it without the
cleanup an ordinary lookup does on the way. A registry that cannot be read in the plan
is a stop the plan found; one that fails only in the effects is a plain failure,
which a retry clears.

## The tests that hold it

A test over the package's writers holds the list complete: every test of `inbox` and
`cli` makes its state directory through a helper that, once the test is over, finds
every file its writers left in any mailbox and fails on one of no kind on the list. A
writer that adds a path without adding its kind fails there, before its first file
stops every mailbox it lands in; a directory no kind names or holds counts as such a
path.

What the plan reads is found by watching it, not by a list
(`TestEveryReadOfThePlanStopsBeforeTheFirstEffect`, `plan_faults_test.go`). Scenes built
as the tests of each effect build them — two journals to two recipients, a report
recorded moot, the steps of a journal, two journals that clear parts of one wait and share the kept answer and the interim
line, and a record of every kind — each run once through a seam that records every read.
Then each read of the plan is failed in turn, in both passes, with every fault that
applies: a path that cannot be followed, one closed to reading, a directory where a file
belongs, and, for a kind whose content is parsed, content that does not parse. Nothing
may change but the stop on record, and the gate before the barrier, the barrier and the
gate after it all answer stopped. Each read the effects make is failed again in the
effects alone: the stop is an effect's, a retry that meets it again keeps it, and a
barrier past it ends where the scene ends without it. Each read the effects make is then
paired with each read of the plan: the first fails the effects, and once they have
failed, the second fails the plan the barrier makes to explain it. The record and the
answer must keep both causes, the effect's as what an effect met; the gate must still
answer stopped once the second is gone; and only a barrier past both ends the stop where
the scene ends. Each pair is run once more with both writes of the stop failing, and the
answer must still name both causes and both failed writes.
Then every write the barrier makes, as the seam records them — in the scene as it is, in
a run where an effect stops, and in one where the plan after it stops too — fails once,
and the next attempt goes through: nothing may be lost or done twice, the stop on record
is never weaker than what the barrier knew, and the gate stays closed until the effects
ran through. Every read of the effects must be one the plan made, and every kind the
walk opens must be read by some scene. A read added to an effect later is on the list
the next run. One that goes around the seam is caught by a last run of each scene with
the room's mailboxes moved aside: the seam follows them, and a read around it meets a
file where they were, so the scene does not end as it does in place.

Three older tests stay beside it (`records_test.go`, `late_unknown_test.go`): a
directory and a link at a sample path of every kind on the list stop the reading; the
recipient's publication marks met through a file and through a closed directory stop
before the first report of an earlier journal; and every durable stop outlives a canceled retry, a
retry stopped by another cause, and a retry whose effect fails. A stop that neither of
the barrier's writes could record is answered with both failures named and the effect's
cause beside any the plan after it found (`effect_stop_test.go`). A gate whose
occurrence could not be recorded still answers the stop, and a resolution whose write
failed reaches its caller, the gate's or the barrier's, and the next call writes it
(`stop_writes_test.go`). The occurrences themselves — a cause removed while stopped,
two causes and one resolved, a recurring cause, main told once, a report held past its
recipient's end, a sweep beside a stop — are in `stop_occurrence_test.go`,
`reconcile_stop_test.go` and `stop_sweep_test.go`; a resolution that does not read, a
path whose presence was unknown or a file on its way, and the settling and the release
beside a stop are in `stop_unknown_test.go`; a server that cannot take its lock settles
nothing, and settles it on a later pass that can (`settle_lock_test.go`); a settling
cut short, by the lock, a stop, a failed archive or a restart, is found again by its
status and copies (`settle_recorded_test.go`), and settled by the status read under the
lock, whatever changed while it was waited for (`settle_recheck_test.go`).

## The stop on record

A stop is records under `inbox/<name>/stops/` (`internal/inbox/stop.go`). Each cause
has a key: a fixed-length hash of its kind — a record that does not read, a path of no
kind, an unknown an effect met, a plan that failed on nothing a path names — the paths
it rests on, relative to the state directory, and the operation it is about, a journal
or a journal and one of its reports. Each time a cause is found while no occurrence of
its key is open is an occurrence: one record, `stops/<key>/<occurrence>`, published
write-once, holding the key's parts, the cause in words, which of its paths were there,
which a look found not there, and the time. Not there is "no such file", or "not a
directory": a component on the way is a file, so nothing stands at the path. A path in
neither could not be looked at — permission, input and output: whether it was there is
unknown, and E6 keeps that apart from absent. A key found again while open is that
occurrence. The stop holds while any
occurrence is open, and `stopState` is its one reader: the barrier, every gate and every
sweep read it. Every call that would change the mailbox answers it first.

An occurrence is resolved only by evidence, written as `<occurrence>.resolved`, naming
it; nothing of a stop is edited or removed, and a resolved occurrence never opens again.
A cause found after its resolution is a new occurrence, with its own evidence to find.
`stopState` reads the resolution before it counts the occurrence closed: one whose bytes
do not parse, that names no evidence or an entry that says nothing — empty, blank or
null — or that cannot be read leaves whether the occurrence was resolved unknown, so it
stays open, the answer says so, nothing resolves it again, and the sweeps keep its
paths; once it reads, the occurrence is closed.

- One the plan found is resolved once a plan, under the mailbox lock, reads every path
  it names as its kind and decides the operation it is about. A path that was there
  when the occurrence was recorded and is gone since was removed while stopped: that is
  no evidence of what it said, and the answer says so. So is one gone since whose
  presence was unknown then: it may have been there. A path a look found not there then
  is read as the operation reads it.
- One only an effect met — an unknown that came after the barrier's plan — is recorded
  as soon as the effect meets it, before the barrier plans again to explain the
  failure: a cause that plan finds is no proof that it is the effect's, so it is an
  occurrence of its own beside the effect's, never in its place. No plan shows the
  effect's cause, so every call answers it from the record. It is resolved once its
  operation is decided by evidence: a barrier has run that journal through to its
  completion, or the journal is on record as completed. A barrier that was canceled,
  whose effect failed, or that met another cause has not.
- An occurrence that could not be recorded is tried once more after the plan that
  explains the failure; the answer names every cause and every failed write, and is
  then the one place the effect's cause is left. Until a later call records it, a
  call's own plan is what stops the mailbox, and it finds every cause still there.
- An occurrence whose record cannot be read stays open until it reads, with its cause
  unknown.
- A report an open occurrence is about stays where it is when its recipient's run
  ends: that the run ended says nothing of whether it landed, so it is not recorded
  moot and its journal stays unfinished until evidence returns.

Main is told of each occurrence once, by a note published once under the occurrence's
id, whatever call found it, and `<occurrence>.told` records that it was; a new
occurrence of the key tells main again. A mailbox that is main's own tells nobody,
since every call it makes answers the stop. Letters from others still arrive. No sweep
of the mailbox removes a path an open occurrence names, and none removes anything while
the stop cannot be read (`inbox/stop_keep.go`). Nothing else in the mailbox removes or
moves such a path either: the server settles no letter an occurrence names, by its own
path or a directory it lies in, until the occurrence is resolved, and the release of an
answer's reservation leaves a mark an occurrence names, its lease no longer refreshed,
for the sweep once nothing names it. Both decide under the mailbox lock, never
without it: a server whose lock cannot be taken writes statuses alone and settles
nothing until a pass holds it. What is still to settle is read from the statuses and
the copies on every pass, not remembered, so a letter a stop kept is settled by the
first pass after the occurrence is resolved, by this run or the next.

A read that met a stop froze nothing: it is not kept as the answer of the read's
receipt, and the same words read once the stop is gone.
