---
description: E1–E8, what may decide a mail operation's effect and the order of a turn end's steps, the rule a pending mark keeps and how a letter is retired; open before changing how mail is written, read or published.
---

# Rules: the effects of a mail operation, E1–E8

The rules every mail operation keeps: what an effect may be decided by, and in what order a
turn end makes its effects. The path through the code is
[delivery-turn-end.md](../delivery-turn-end.md#the-end-of-a-turn). A report lost when a process
is killed in the milliseconds between two steps is asked for again by main; no mechanism is
built against it (`AGENTS.md`, "Build for failures that were seen").

## The rules

- **E1. An effect is undone or skipped only on evidence, and a retry finds what an
  earlier attempt left.** A step not recorded as done is no proof it was not done. A turn
  end that failed
  part way leaves its wait standing, and a retry of the same event runs its steps again
  from the disk.
- **E2. A lock is removed only by whoever holds it**, and a call that took a lock checks
  the file at the path is still its own.
- **E3. Every check that decides an effect runs under the lock that orders it, right
  before the effect, and again on a retry**, which is a new call with its own deadline.
  A turn end holds its own mailbox's lock throughout; a report is written into the
  recipient's mailbox without that mailbox's lock, since a letter is written atomically
  and the recipient's sweep keeps a report its writer's wait still names.
- **E4. A read's completion is one durable fact every channel shares.** A letter leaves
  `unread/` only by being read, and never comes back; a late acknowledgment finds it read
  and marks nothing.
- **E5. An answer is printed whole.** A read prints every letter it takes in full, and a
  letter counts as read only once its output got through.
- **E6. A lookup that decides a write has three outcomes: found, proven absent, or an
  error.** Only "no such file" proves absence. An error is not taken for absence: the
  operation that met it fails, writes nothing on it, and is retried; a letter is not
  written when a stage of its mailbox cannot be searched, and a wait record that cannot be
  read is not written over. Nothing else stops: an unknown or unreadable file in a mailbox
  stops no other call. Readers that only inform fold an error into their most cautious
  answer.
- **E7. Every effect has an immutable identity and a proven scope, and a retry advances it
  only from what is on disk under that identity.**
  - A turn end is one operation — write its reports, clear the waits they answer, take the
    kept answer they carry, record whether the end was interim — named by its run and its
    event, never by an attempt; a report's id is fixed by the wait it answers (`ReportID`),
    and a failure report to main that answers no wait by the operation in its id's tail.
  - Its scope is fixed by the event: the read boundary is a position on the run's read
    clock, which numbers every read and every hold; a hold reserves its position, keeps the
    answer, then commits the clock, and a position is never issued twice. A held end
    confirmed again after a crash before the commit raises the clock to the position its
    kept answer records, allocating none, so its continuation's boundary takes the answer.
  - The turn's window on the boot clock picks the pending marks that make it interim,
    opening after the turn's start or the latest earlier end a done journal records,
    whichever is later; no end removes a pending mark, which lives as long as its run.
  - Every turn boundary names its event (a turn id); an end that names an event and
    carries no boundary takes no effect on any attempt, and its waits stay owed.
  - An adapter whose ends come over a protocol and are retried hands the core every copy of
    one end as its first capture: the event, the read boundary and both times fixed on its
    first arrival, kept when the core's attempt fails; a copy whose content differs from the
    first is refused and hands the core nothing — Claude Code's mod
    ([claude-turns.md](../claude-turns.md#the-hosts-side)); pi's adapter captures an end
    once and retries that capture, refusing a second end of the same turn. The core answers
    a receipt only for an end it took: a run whose name is proven to have passed on gets no
    receipt and its end is not sent again.
- **E8. A turn end writes before it clears, and records itself done last.**
  - Under the sender's mailbox lock, each report is written unless a letter of its id is in
    the recipient's mailbox in any stage, or every task its wait names already has a
    finished or failed report of the name there; then the waits are cleared, the kept answer
    dropped if it is still the version carried, the interim end recorded, and last
    `journal/<op>.done` written. A cleared wait counts as answered, so the order "write, then
    clear" is what keeps an answer on record from standing without its letter.
  - A retry of the same event finds `journal/<op>.done` and answers from it; one that finds
    none runs the steps again, and finds its report written by its id.
  - A report goes only to the run of its recipient that is still the live run of the name;
    one to a run that ended or was replaced reaches nobody, and what it answered is cleared.
  - The recipient's sweep keeps a report whose writer's wait still names what it answers,
    so a retry that comes after a day still finds the letter. A done journal is kept while
    its run lives.
  - What a crash in the milliseconds between two steps can cost, and nothing more: a task
    shown once more as owed until the next end, a lost report main asks for again, a kept
    answer after an interruption carried by the next turn's report.

## A pending mark

`rewake pending` marks the running turn (`internal/cli/pending.go`), and the mark speaks for
that turn only. The call takes the boot-clock time it started (`At`); under the mailbox lock,
just before the mark, it looks for an end of its run on record at or after `At`, or a turn
start recorded after it (`mail.Box.RecordTurnStart`): either proves the turn ended, and it marks
nothing and says so, exit 1. A recorded start is the harness's own time of the turn's start,
never when the report arrived; a missing one proves nothing. A journal that cannot be read
writes no mark, and the call fails, to be run again. The mark and an end's choice of its mark
share the mailbox lock, so a mark written before the end's journal is that end's to take, and
one after it is never written (`mail.EndedSince`).

## Retiring a letter

A letter is the proof that it landed, and it is guarded by its recipient's mailbox lock. Its
last copy leaves the mailbox only under that lock, by a sweep that has read, inside that
section, that its own run is the name's live run (`Locked.SweepFinished`, the delivery
server's `sweepFinished`). A sweep that cannot take the lock, or whose run has ended, retires
nothing, so a server of an ended run cannot remove the live run's letters. Settling moves a
copy only beside another that stands, so a copy whose archive failed stays for the next pass.
