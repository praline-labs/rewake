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
| `stopped` | the stop this mailbox is in | by the stop itself, below |
| `<id>.json`, `unread/<id>.json`, `done/<id>.json` | a letter | a message |
| `<id>.status` | a letter's delivery status | a status |
| `awaiting/<run>/.read-clock`, `.read-high` | a run's read clock and its highest place | one word, or none yet; a number |
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
cause beside any the plan after it found (`effect_stop_test.go`). A gate whose record of
a stop could not be made or written still answers the stop, and a lift whose remove or
sync failed reaches its caller, the gate's or the barrier's (`stop_writes_test.go`).

## The stop on record

A stop, once found, is written as `inbox/<name>/stopped`, with its cause and whether an
effect met it. Every call that would change the mailbox answers it first. How it goes
depends on what found it:

- One the plan found is found again by the same plan every call makes; the first plan
  that finds its cause gone removes the record, whichever call made it.
- One only an effect met — an unknown that came after the barrier's plan — is recorded
  as soon as the effect meets it, before the barrier plans again to explain the
  failure: a cause that plan finds is no proof that it is the effect's, so it is
  recorded beside the effect's (`Met`), never in its place, and its going does not
  lift the stop. No plan shows the effect's cause, so every call answers it from the
  record, and only the barrier removes it, once it has run every effect through. A
  barrier that was canceled, whose effect failed, or that met another cause
  has not, and keeps it.
- The barrier holds the effect's cause itself until it returns and puts it into every
  record of the stop it writes: once when the effect fails, so a crash cannot lose it,
  and once more after the plan that explains the failure, with what that plan found
  beside it. It never takes the effect's cause from the record, which a write that
  failed in this same call may not hold, so one failed write of the stop leaves the
  next one whole. Its answer names the effect's cause beside any the plan found, as
  the record does. A stop that no write of the call could record is answered so, with
  every failed write named: the answer is then the one place the effect's cause is
  left. Until a later barrier records it, a call's own plan is what stops the mailbox,
  and it finds every cause that is still there.
- A record of the stop that cannot be read is a stop of the second kind, with its cause
  unknown.

A read that met a stop froze nothing: it is not kept as the answer of the read's
receipt, and the same words read once the stop is gone.
