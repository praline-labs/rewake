# Rules: the effects of a mail operation, E1–E8

The rules every mail operation keeps — what an effect may be decided by, and in what
order a turn end makes its effects — the current wording of
[design-rules.md](../v2/design-rules.md#effects-e1e8), from `mail-bridge-cli.md` rules 1–8
without the 1.x migration.

Decided by the owner, October 18, 2026: the mechanics frozen on October 17 — the operator
decision D1–D8 and `rewake decide`, the stops of a mailbox, the turn-end journals with
their plan and seam, the once-marks of reports and the barrier that completed unfinished
journals — is removed, and E1, E3, E6 and E8 are simplified to what the simple turn end
keeps ([delivery-turn-end.md](../delivery-turn-end.md#the-end-of-a-turn)). No live loss,
duplicate, stopped mailbox or decision was ever seen in real work; the mechanics itself
stopped a mailbox on a file it did not know, and cost two full mailbox walks, about seven
atomic writes and fifteen flushes on every turn end. A report lost when a process is
killed in the milliseconds between two steps is asked for again by main (the owner,
October 17, 2026). The rules as they stood until then, with D1–D8, are in git history and
in [stage3-decision-recovery.md](../archive/v2/stage3-decision-recovery.md#its-rules).

## The rules

- **E1. An effect is undone or skipped only on evidence, and a retry finds what an
  earlier attempt left.** A receipt goes only when its effect is proven absent or proven
  done; a step not recorded as done is no proof it was not done. A turn end that failed
  part way leaves its wait standing, and a retry of the same event runs its steps again
  from the disk. (M1 rule 1.)
- **E2. A lock is removed only by whoever holds it**, and a call that took a lock checks
  the file at the path is still its own. (M1 rule 2.)
- **E3. Every check that decides an effect runs under the lock that orders it, right
  before the effect, and again on a retry**, which is a new call with its own deadline.
  A turn end holds its own mailbox's lock throughout; a report is written into the
  recipient's mailbox without that mailbox's lock, since a letter is written atomically
  and the recipient's sweep keeps a report its writer's wait still names. (M1 rule 3.)
- **E4. A read's completion is one durable fact every channel shares.** A letter leaves
  `unread/` only by being read, and never comes back; a late acknowledgment finds it read
  and marks nothing. (M1 rule 4.)
- **E5. The size bound holds on the final encoded bytes of every answer, in one place**;
  every text part names its letter, part, count and byte range. (M1 rule 5.) Since
  October 18, 2026 no answer is cut: the bound and its parts were the mail tool's, and the
  shell prints a read whole.
- **E6. A lookup that decides a write has three outcomes: found, proven absent, or an
  error.** Only "no such file" proves absence. An error is not taken for absence: the
  operation that met it fails, writes nothing on it, and is retried; a letter is not
  written when a stage of its mailbox cannot be searched, and a wait record that cannot be
  read is not written over. Nothing else stops: an unknown or unreadable file in a mailbox
  stops no other call. Readers that only inform fold an error into their most cautious
  answer. (M1 rule 6.)
- **E7. Every effect has an immutable identity and a proven scope, and a retry advances it
  only from what is on disk under that identity.** (M1 rule 7, clauses identity, clock and
  scope.)
  - A turn end is one operation — write its reports, clear the waits they answer, take the
    kept answer they carry, record whether the end was interim — named by its run and its
    event, never by an attempt; a report's id is fixed by the wait it answers (`ReportID`),
    and a failure report to main that answers no wait by the operation in its id's tail.
  - Its scope is fixed by the event: the read boundary is a position on the run's read
    clock, which numbers every read and every hold; a hold reserves its position, keeps the
    answer, then commits the clock, and a position is never issued twice. A held end
    confirmed again after a crash before the commit raises the clock to the position its
    kept answer records, allocating none, so its continuation's boundary takes the answer
    (S5).
  - The turn's window on the boot clock picks the pending marks that make it interim,
    opening after the turn's start or the latest earlier end a done journal records,
    whichever is later; no end removes a pending mark, which lives as long as its run.
  - Every 2.0 TurnBoundary names its event (a turn id); an end that names an event and
    carries no boundary takes no effect on any attempt, and its waits stay owed.
  - An adapter whose ends come over a protocol and are retried hands the core every copy of
    one end as its first capture: the event, the read boundary and both times fixed on its
    first arrival, kept when the core's attempt fails; a copy whose content differs from the
    first is refused and hands the core nothing — Claude Code's mod from S4-6
    ([stage4-turns.md](../v2/stage4-turns.md#the-hosts-side)); pi's adapter captures an end
    once and retries that capture, refusing a second end of the same turn. The core answers
    a receipt only for an end it took: a run whose name is proven to have passed on gets no
    receipt and its end is not sent again.
- **E8. A turn end writes before it clears, and records itself done last.** (M1 rule 8,
  simplified.)
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
  - A run that resumes a conversation does not take over a task that already has a finished
    or failed report of its name in the sender's mailbox (`AdoptWaits`): an earlier run died
    between writing the report and clearing the wait.
  - The recipient's sweep keeps a report whose writer's wait still names what it answers,
    so a retry that comes after a day still finds the letter. A done journal is kept while
    its run lives.
  - What a crash in the milliseconds between two steps can cost, and nothing more: a task
    shown once more as owed until the next end, a lost report main asks for again, a kept
    answer after an interruption carried by the next turn's report.
