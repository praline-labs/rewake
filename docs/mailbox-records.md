# Every record of a mailbox

What a mailbox under `inbox/<name>/` holds, by path. Until October 18, 2026 this list was
code (`recordKinds` in `internal/core/mail/records.go`), and a file it did not name, or
one of a named kind that did not read, stopped the mailbox until evidence or an operator
decision settled it. Decided by the owner that day, the stops, the list in code, the plan
and the seam are removed with the rest of the frozen mechanics
([rules/effects.md](rules/effects.md)): an operation that meets a file it cannot read fails
with an error and is retried, and a file nobody knows stops nothing. The table is a
reference for a reader of a mailbox, kept by hand.

## The list

| Path | Kind | Read as |
|---|---|---|
| `.lock` | the mailbox lock | nothing: it says nothing |
| `<id>.json`, `unread/<id>.json`, `done/<id>.json` | a letter | a message |
| `<id>.status` | a letter's delivery status | a status |
| `awaiting/<run>/.read-clock`, `.read-high` | a run's read clock and its highest place | one word, or none yet; a number |
| `awaiting/<run>/.turn-starts/<time>` | a turn start the host heard, one file per reading, the latest the largest | empty, its name a number |
| `awaiting/<run>/<peer>` | who a run owes a report | a wait, under a session's name |
| `answering/<id>`, `received/<id>` | a question waited on; the report printed for it | readable |
| `retention/<id>` | when a reserved report is released | its lifetime |
| `journal/<op>.done` | a turn end recorded done: its run, operation, end time and the held end it took ([delivery-turn-end.md](delivery-turn-end.md#the-end-of-a-turn)) | a journal, by the fields it still needs |
| `pending/kept.json`, `pending/interim.json` | a held answer; a run's last word on the work | their records |
| `pending/marks/<run>/<time>` | a pending mark | a mark that says what its name says |
| `once/<run>/<id>` | the publication mark of a heads-up published once (`mail.PublishOnce`); no report has one since October 18, 2026 | `intent` or `published` |
| `threads/<id>` | a delivery thread | readable |
| `receipts/<run>/.<token>.lock`, `key-*`, `<token>.json` | a call's receipt lock, key and receipt | nothing; a token; a receipt that says what its place says |

A file named `.tmp-*`, in any directory, is a write that never finished
(`state.WriteAtomic`), never a record. A room's mailboxes made by an earlier build may
still hold `stops/`, `decisions/`, `claims/`, unfinished `journal/<op>` files or
`journal/resend-*`: no code reads them now, and a room belongs to one build
([v2/design-state.md](v2/design-state.md#one-build-per-room)).

## The plan and the seam

Removed on October 18, 2026: the barrier no longer runs its effects as a plan first, and
the file seam both passes shared is plain file access (`internal/core/mail/access.go`).

## The tests that hold it

Removed with the list in code on October 18, 2026.

## The stop on record

Removed on October 18, 2026: a mailbox no longer stops, and nothing keeps
`stops/<key>/<occurrence>` records; what a turn end does when a step fails is in
[delivery-turn-end.md](delivery-turn-end.md#the-end-of-a-turn).
