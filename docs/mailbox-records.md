---
description: The records a mailbox keeps on disk under inbox/<name>/, by path, with what each is read as — a contract between the processes that share a mailbox.
---

# Every record of a mailbox

What a mailbox under `inbox/<name>/` holds, by path. An operation that meets a file it cannot
read fails with an error and is retried, and a file nobody knows stops nothing
([rules/effects.md](rules/effects.md), E6). The table is a reference for a reader of a
mailbox, kept by hand.

## The list

| Path | Kind | Read as |
|---|---|---|
| `.lock` | the mailbox lock | nothing: it says nothing |
| `<id>.json`, `unread/<id>.json`, `done/<id>.json` | a letter | a message; a task's or a question's `brief` names the brief it gives by its id, the text staying the brief's ([roadmap/briefs.md](roadmap/briefs.md)) |
| `<id>.status` | a letter's delivery status | a status |
| `awaiting/<run>/.read-clock`, `.read-high` | a run's read clock and its highest place | one word, or none yet; a number |
| `awaiting/<run>/.turn-starts/<time>` | a turn start the host heard, one file per reading, the latest the largest | empty, its name a number |
| `awaiting/<run>/<peer>` | who a run owes a report | a wait, under a session's name |
| `answering/<id>`, `received/<id>` | a question waited on; the report printed for it | readable |
| `retention/<id>` | when a reserved report is released | its lifetime |
| `journal/<op>.done` | a turn end recorded done: its run, operation, end time and the held end it took ([delivery-turn-end.md](delivery-turn-end.md#the-end-of-a-turn)) | a journal, by the fields it still needs |
| `pending/kept.json`, `pending/interim.json` | a held answer; a run's last word on the work | their records |
| `pending/marks/<run>/<time>` | a pending mark | a mark that says what its name says |
| `threads/<id>` | a delivery thread | readable |

A file named `.tmp-*`, in any directory, is a write that never finished
(`state.WriteAtomic`), never a record. An unfinished `journal/<op>` file is still looked at by
the readers of a run's ends and windows (`JournalRecorded`, `HeldEndTaken`, `TurnWindowStart`,
`LatestEnd`) and by retention, which take only its epoch, end and hold and ignore the rest; a
room belongs to one build ([rules/state.md](rules/state.md#the-rooms-lease)). A mailbox an
earlier build made may still hold `stops/`, `decisions/`, `claims/`, `journal/resend-*`, or
`receipts/<run>/key-*` and `receipts/<run>/calls/`: no code reads them now, and they stop
nothing.

A known limit: a mailbox a build up to 2.0.0 made may keep the `receipts/` and `once/`
directories its `send --notify` and `pending` wrote; nothing reads or sweeps them now, and they
go when the room or the system's temporary directory is cleared.
