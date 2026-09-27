# Owed reports

What a session owes and what it is owed, read back from the records rewake already
keeps: `rewake inbox --owed` shows a session again the tasks it has read and not
reported on, and `rewake inbox --awaited` shows a run what it sent and still waits on.
Both only read. The records themselves — the `awaiting/` wait records, statuses and
reports — are described in [delivery.md](delivery.md).

[Back to the delivery document](delivery.md).

## Reading again what is owed (`rewake inbox --owed`)

What a session has read and not reported on is exactly what its
`awaiting/<own epoch>/` records name: a report clears the ids it settles, and an
interim turn end (`rewake pending`) or a stop keeps them. `--owed` prints those
messages again, in full — under `Rewake: owed a report for <n> message(s):`, each with
the sender, kind and time of an ordinary read and its text, from `done/` (or
`unread/`, for a read whose last step failed) — oldest read first; `--json` adds each
id. An addendum (`rewake send --to`, [delivery-sent.md](delivery-sent.md)) is
placed right under its task, headed `+ addendum from <sender> · HH:MM:SS`, so the
corrections are re-read with the brief they correct; one whose task is not owed any
more stands alone, its heading naming the task (`· addendum to <id>`). The task is
the one as it is now: an addendum to a task `rewake edit` replaced goes under the
replacement, and `addendumTo` names it (`inbox.CurrentTask`, [delivery-sent.md](delivery-sent.md#edit-rewake-edit-id-text)).
Only the rendering nests: `--json` keeps the flat list, each addendum with its `addendumTo`.
It is the task a session is working on, from the mailbox rather than from memory,
which is what a session needs after a context compaction: a summary retells the brief
and can drop an item. The write and general playbooks tell it to re-read
there, to read new mail with `rewake inbox`, and to say so in its report.

It only reads. No lock is taken — every record it reads is written whole or not at
all — and nothing is marked read, recorded, announced or given a status, so a second
read is not a second obligation. A message whose text is gone is still named, by id
and sender (`text no longer kept`), with `kept: false`; the age sweep of `done/` and
`unread/` keeps what the current run still owes, however old. Nothing owed is one
line, `Rewake: nothing owed a report.`, and exit 0. Only work from another session is
owed: a task sent from a plain shell has no run to report to, records no wait, and so
is not listed and cannot be shown again this way — `--help` says so, the one-line
answer does not. Mail not yet read — unread, pending or held — is not owed and does
not appear. What of it asks for work — tasks and questions, and a kind this build does
not know, by kind alone (`inbox.AsksForWork`), so a task from a plain shell counts too —
is counted after the list, `Rewake: <n> unread task(s) or question(s) — run rewake
inbox.`, and nothing is said when there is none; `--json` carries the count as
`unread`, 0 included. The count says work is waiting, not that a report is. Unread mail
is looked at before what is owed, and a message in both is counted once, as owed: a
parallel `rewake inbox` writes the wait record before it moves the message out of
`unread/`, so a message read between the two looks is found at least once — the other
order could miss it in both and answer "nothing owed" with nothing unread. "Nothing owed" is not "no work": a Codex worker that asked
`--owed` after a compaction read that line and skipped a new unread task twice, which
is why the count is there and why the playbooks tell a session to read new mail with
`rewake inbox` as well (both dated September 25, 2026).

`--owed` is used alone: `--peek` and `--message` look at unread mail, and a
combination is refused. A main session is refused too, naming why: its reads record no
obligation, so it owes nothing to show; the refusal points to `--awaited`, which answers
what main does need.

## What others owe you (`rewake inbox --awaited`)

The opposite list: the tasks and questions this run sent that have no report yet, by
recipient. A main session needs it after its own compaction, when it would otherwise
rebuild from the summary what it handed out and still waits on; the main playbook says
to run it then. It works in any role — a worker that delegates is owed reports too.

It is assembled from files other sessions already keep. Every mailbox in the room is
searched in the order a message moves — `inbox/`, `unread/`, `done/`, as `PutOnce`
looks — so a message that moves on during the walk is found further along rather than
missed in both, and a copy found later replaces one seen before. The search looks for messages whose `from` and `fromEpoch`
are this run and whose kind owes a report; notes, mail from a plain shell and mail from
an earlier run of this name are not this run's to wait on and are never listed. Each one
is then placed:

| state | what it rests on |
|---|---|
| `undelivered` — not delivered yet | waiting in `inbox/`, status absent or `pending`; a `pending` status's detail says why it waits, a compaction for one |
| `held` | status `held`; the detail says why |
| `unread` — delivered, unread | status `delivered` |
| `owed` — read, being worked on | status `read` (or in `done/` with the status swept), and the recipient's `awaiting/<its run>/<this name>` names this run and the id |
| `pending` | read and owed, and the latest interim report on it came after any stop |
| `stopped` | read and owed, and the latest report on it is a stop; the text form prints the stop's own words after `stopped:`, which say whether the person at the keyboard or a main with `rewake interrupt` stopped it |

"Latest" is by the report's `createdAt`, the id only breaking a tie: every report on one
wait carries the wait's time prefix, and what follows it is a hash.
| `failed` — no report coming | status `failed` |

A `finished` or `error` report in this run's own mailbox that names the id in
`inReplyTo` settles it — from any run of the recipient, since a run that resumed the
conversation reports on what it took over — and so does a wait record that no longer
names it; the record is cleared only once the report is written. A message whose wait a
resumed run took over is followed in that run's records
([delivery.md](delivery.md#a-resumed-conversation)), and listed as that run's. Settled messages are not listed.

A message read by a run that then ended, still named by its wait record, and delivered
into a conversation is not lost yet: a resume of that conversation takes the wait over and
reports on it. It is listed as `<name> ended; a resume of its conversation may still
report` (`"resumable": true`) and counted with the reports still expected, and main is told
not to send it again yet: sending it would get the work done twice. That holds before any
new run of the name, and after a new one has started but not yet learned its conversation
and swept the old run's waits. It stops holding when a new run in another conversation
sweeps the wait, or a day after the wait was recorded (`resumeWindow`, as long as
finished mail is kept): from then on no resume takes it over, main's wrapper lets its grant
go, and it is lost for good. Any other unsettled message of a recipient whose run has
ended, or whose name a new run has taken, is lost — listed as `no report coming: <name>
ended` or `… was replaced by a new run`, whatever stage it had reached, and counted apart
from the reports still expected. Only that line means the task may be sent again.

Whether a missing wait record can be trusted depends on who could have removed it. Only
the report removes one, and only a new run of the name sweeps the old run's directory
(`sweepAwaiting`). So while the recipient's run lives, and after it ended with nobody
taking the name — its `awaiting/<run>/` directory is still there — a record that no
longer names the message means answered, even once this run's own server has swept the
report after a day (`keepFinished`), while the ended recipient's mailbox, with no server
left, keeps the read task in `done/`. Once a new run has taken the name, the old
directory is gone and only a `finished` or `error` report naming the id tells answered
from lost; without one the message is listed as having no report coming. A live
recipient whose role reports nothing is skipped: its reads owe no report.

The plain form opens with `Rewake: waiting on <n> report(s):` — with `; <m> more will
not come` when some will not — then, per recipient, `to <name>` and, per message,
`<id> · <kind> · <time> · <state>` above the first line of its text. An addendum
follows its task, indented: `  + <id> · addendum to <tail of the task's id> · <time> · <state>` — the tail
is a reference `withdraw`, `edit` and `--to` accept. One whose task is not listed — reported
on already, or no longer kept — stands alone as `<id> · task, addendum to <tail> · …`. The task is
the current one, an edit's replacement included, as for `--owed`. Nothing awaited is
`Rewake: nobody owes you a report.` `--json` carries each message whole: `id`, `kind`,
`createdAt`, `state`, `detail` (the hold or failure reason, or the interim or stop
report's text), `gone` (`ended` or `replaced`, only when lost), `resumable`, `addendumTo`
for an addendum, and the
full `text`. A withdrawn message is a note from then on and is not listed.

It only reads, like `--owed`: no lock, no status, nothing marked, nothing sent, and a
registry lookup that does not prune a dead record. A message caught between two
directories is counted once, at the stage further along. `--awaited` is used alone.
