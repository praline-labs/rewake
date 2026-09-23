# What others owe you — September 23, 2026

The owner approved, on September 23, 2026, the view proposed with `--owed`: a main
session's list of what it handed out and still waits on. `--owed` shows a worker what
it owes; main's reads record no obligation, so main was refused there, and after its
own compaction it had to rebuild from the summary which tasks and questions it had sent
and was still owed a report on.

**What was built.** `rewake inbox --awaited` lists the tasks and questions the current
run sent and has no report on yet, grouped by recipient: id, kind, time, the first line
of the text, and where each stands — not delivered yet, held, delivered and unread,
read and being worked on, pending after an interim report, stopped by a person, or not
delivered with no report coming. `--json` carries the full text and the detail behind a
state: the hold or failure reason, the interim or stop report's text. A recipient whose
run ended, or whose name a new run took, is named as such — no report is coming — and
counted apart from the reports still expected. Nothing awaited is one line, `Rewake:
nobody owes you a report.` Mechanism and the table of states:
[delivery-owed.md](../delivery-owed.md#what-others-owe-you-rewake-inbox---awaited).

## The lines it prints

- `Rewake: waiting on <n> report(s):`, or `Rewake: waiting on <n> report(s); <m> more
  will not come:`, or `Rewake: waiting on no reports; <m> will not come:`.
- `Rewake: nobody owes you a report.` when nothing is awaited.
- Per recipient, `to <name>`; per message, `<id> · <kind> · <time> · <state>` above the
  first line of its text. A state reads `not delivered yet`, `held: <why>`, `delivered,
  unread`, `read, being worked on`, `pending: <the interim's first line>`, `stopped by a
  person`, `not delivered, no report coming: <why>`, `no report coming: <name> ended` or
  `no report coming: <name> was replaced by a new run`.
- `--owed` in a main session: `<name> owes no reports: a main session's reads record no
  obligation, so --owed has nothing to show. rewake inbox --awaited shows what others
  owe it.`

**Decisions.**

- A flag of its own, `--awaited`, not `--owed` answering differently for main. The two
  answer opposite questions, and any role can delegate: a general session that sends
  work is owed reports too. `--owed` in a main session still refuses, and the refusal
  now names `rewake inbox --awaited` as the next step.
- Only the current run's mail: an earlier run of the same name waited, or not, as
  someone else. Notes and mail from a plain shell owe nothing and are never listed.
- It is read from records the other sessions already keep — their mailboxes and
  `awaiting/` records, and the reports in the caller's own mailbox — so nothing new is
  written anywhere, and the view takes no lock, writes nothing and sends nothing. The
  registry lookup it makes is the read-only one, which does not prune a dead record.
- A read message with no wait record is settled while its recipient's run lives, and
  after it ended with nobody taking the name, since a wait record is cleared only once
  the report is written and only a new run sweeps the old run's records. Once a new run
  holds the name, without a `finished` or `error` report naming the id the message is
  listed as having no report coming.
- One line in the main playbook, which feeds the launch briefing and the guide: after a
  context compaction, run `rewake inbox --awaited` instead of rebuilding from the
  summary. The briefing golden was regenerated for it.

**Tests.** Unit tests place a message at every state — undelivered, held, delivered and
unread, read and owed, pending, stopped, failed — and leave out what owes nothing or is
settled: a note, a task from a plain shell, one from an earlier run of the sender, one
already reported on. A recipient that ended and one replaced by a new run are named as
gone, including a read task whose wait record the new run swept, and the new run's own
task is owed as usual. The state directory is identical, file for file and in its times,
after the view in both forms; a combination with another inbox flag, a value on the
switch and a call outside a session are refused. The workflow case `awaited-view` runs
in both columns: main sends a task to each of two workers, one reports and the other
ends its turn pending; the view must list exactly the second task, as pending, and after
a note wakes that worker and it reports, nothing. main runs every command itself, through
a request directory the fixture now serves, so the view is read by the session it is
about. Two mutants on the Claude Code column — one blind to interim reports, one that
never lets a task go — must break exactly the observations they name
([testing.md](../testing.md)).

**Review.** An independent review before the commit found three places where the view
answered wrongly, and all three were fixed with a unit test each, confirmed to fail on
the code before the fix:

- The mailboxes were walked `done/`, `unread/`, `inbox/`, against the way a message
  moves, so one renamed from `unread/` to `done/` between two listings was missed in
  both — a task shown as not sent, or a report missed and an answered task shown as
  owed. The walk now goes `inbox/`, `unread/`, `done/`, a later copy replacing an
  earlier one.
- The latest interim or stop was chosen by id, but every report on one wait shares the
  wait's time prefix and ends in a hash, so a stop could outrank the interim that
  followed it. It is now chosen by `createdAt`, the id breaking a tie.
- An answered task to a recipient that had since ended showed, a day later, as having
  no report coming: the ended session's mailbox is never swept, so the task stays read
  in `done/`, while this run's server sweeps the report after a day. An ended run's wait
  records are swept by nobody until a new run starts, so while its directory is there
  a missing record now means answered; only a replaced run still depends on the report.

**Open.** A wait record holds one run per sender: reading a task from an earlier run of
a sender after one from its current run replaces the current run's wait, and the current
run is then never reported to. Reads go in id order, so the earlier run's mail is
normally read first, and the case needs a selected `--message` read to arise. The
reviewer's proposed fix: `markAwaitingSequence` must not let a message with a smaller id
than every message in the current record replace another run's record, since runs of
one name do not overlap. Not built.

Withdrawing, editing and resending an unread message, the owner's idea of the same day,
starts from this view ([work-queue.md](../work-queue.md#also-queued-not-scheduled)).
