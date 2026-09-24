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
id. It is the task a session is working on, from the mailbox rather than from
memory, which is what a session needs after a context compaction: a summary retells
the brief and can drop an item. The write and general playbooks tell it to re-read
there and to say so in its report.

It only reads. No lock is taken — every record it reads is written whole or not at
all — and nothing is marked read, recorded, announced or given a status, so a second
read is not a second obligation. A message whose text is gone is still named, by id
and sender (`text no longer kept`), with `kept: false`; the age sweep of `done/` and
`unread/` keeps what the current run still owes, however old. Nothing owed is one
line, `Rewake: nothing owed a report.`, and exit 0. Only work from another session is
owed: a task sent from a plain shell has no run to report to, records no wait, and so
is not listed and cannot be shown again this way — `--help` says so, the one-line
answer does not. Mail not yet read — unread, pending or held — is not owed and does
not appear.

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
`inReplyTo` settles it, and so does a wait record that no longer names it — the record
is cleared only once the report is written. Settled messages are not listed. A recipient
whose run has ended, or whose name a new run has taken, owes nothing any more: its
unsettled messages are listed as `no report coming: <name> ended` or `… was replaced by
a new run`, whatever stage they had reached, and counted apart from the reports still
expected.

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
`<id> · <kind> · <time> · <state>` above the first line of its text. Nothing awaited is
`Rewake: nobody owes you a report.` `--json` carries each message whole: `id`, `kind`,
`createdAt`, `state`, `detail` (the hold or failure reason, or the interim or stop
report's text), `gone` (`ended` or `replaced`) and the full `text`.

It only reads, like `--owed`: no lock, no status, nothing marked, nothing sent, and a
registry lookup that does not prune a dead record. A message caught between two
directories is counted once, at the stage further along. `--awaited` is used alone.
