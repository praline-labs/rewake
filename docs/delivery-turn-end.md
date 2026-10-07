# Delivery: turning a turn's end into a report

Split from [delivery.md](delivery.md) by subject on September 27, 2026: that document
carries how a message is sent, queued and read; this one carries what happens once the
reader's turn ends — how `turn-ended` turns it into a report, how a report notes that
the reader's conversation changed underneath it, and where the turn ends that are not an
ordinary report are described.

### The end of a turn

Callbacks with a nonempty `agent_id` belong to a nested agent and are ignored
before touching turn journals or waits. `agent_type` alone
does not imply nesting: a root session can select an agent profile.

A hook runs `rewake turn-ended` with the last reply in its payload. The owned
server backend sends completion events directly to the same internal reporting
function, with explicit session epoch and thread identity. Under the mailbox lock, for
every eligible run in `awaiting/<own epoch>/` it leaves a `finished` message whose text
is that reply, and whose `inReplyTo` lists the messages read from that run since its
last report, addressed to that run — not to whoever holds the name now. The rules this
follows are 7 and 8 of [mail-bridge-cli.md](mail-bridge-cli.md); how each record is
recovered, record by record, is [turn-end-recovery.md](turn-end-recovery.md), and this
section is the path through the code.

A turn end is one operation, named by its cause: a hash of the run, the event's id, its
kind and its start and end on the boot clock (`turnOp` in `internal/cli/turn_reports.go`).
Its scope is fixed by what the event carries. An end that names an event carries a read
boundary, a position on the run's read clock (`internal/inbox/read_boundary.go`): what
was read or kept at or below it is this end's, and a task read or an answer kept above
it belongs to a later end, however late a retry comes. An end that names an event and
carries no boundary reports nothing, on any attempt, and its waits stay owed. An end
heard once — a hook that names no event — has no retry, and its one attempt under the
lock is its scope. The pending mark that makes the end interim is found in the turn's
window: from the later of the turn's start and the latest earlier end of the run a
journal records (`TurnWindowStart`), to its end. No end removes a mark: marks stay for
the run's life ([turn-outcomes.md](turn-outcomes.md)).

The journal, `journal/<op>` (`internal/inbox/journal.go`), is written before the first
effect and is the one source of what the end does, and nothing but a journal publishes:
the complete report batch — recipient epochs, ids, kind, text, timestamps and
thread-change annotations — then the version of the kept answer the reports carry with
the operation of the end that was held, the waits to clear, the mark used, and the end's word for the interim record: the pending
line, or that the end settled the work. Completing publishes each report not yet
published and records its id, drops the kept answer only when it is still the version
published, clears only the recorded message ids — later messages remain owed to the next
result — and records the interim end, never over a record of the same run that ended
later, nor over the record of another run that holds the name (`internal/inbox/interim.go`). It then empties the journal to its
operation, its end time and the held end it took, marks it done and renames it `<op>.done`, so the barrier
does not read it again while a retry of the same end still finds it completed. A done
journal is kept while its run lives: a retry comes only from that run, and the next
end's window opens after it. The live run's sweep looks at them again a day later and
removes those of ended runs; an unfinished journal is never swept, and a write of one
that never finished (a dot name) is skipped and swept.

A turn end that fails or dies anywhere in the sequence leaves its journal unfinished,
and the barrier completes it under the mailbox lock before a wait is read again
(`inbox.Reconcile`, `internal/inbox/reconcile.go`): every turn end runs it first, and so
does adoption, which the server runs at its start. One it cannot complete stops that
turn end, or that adoption, too. The turn end completes its own journal the same way,
through the barrier, once it is written. Before any effect the barrier reads every file
of the mailbox against the list of record kinds, then runs its remaining effects as a
plan that writes nothing, so whatever they decide by is read first
([mailbox-records.md](mailbox-records.md#the-plan-and-the-seam)), and changes nothing
while one of them cannot be told, so a journal never publishes beside an unknown found
late. The same plan is the stop every other call asks first (`inbox.MailboxStopped`): `rewake inbox` and each
part of a read in parts, `rewake pending`, the acknowledgment of a read, and a question
taking its answer all refuse while a record cannot be told. The stop is kept on record
in the mailbox, and goes once a look finds its cause gone, or, for one only an effect
met, once a barrier has run every effect through. The ids the journal records are its own proof of
publication, and the recipient keeps one that covers the gap between writing a report
and recording its id: each report is written under the marks of
`inbox/<to>/once/<to-epoch>/<id>` that a journaled heads-up uses — `intent` before,
`published` after — and the sweep settles an intent before it removes the report it
names. The marks live as long as the recipient's run, which is as long as a journal can
publish to it, so a journal completed after the recipient has read the report and swept
it does not publish it again: the sweep retires a letter or a run's marks only under the
recipient's lock, and only when it reads there that its own run is the live one and the
marks' run is not; a sweep that cannot take the lock, or a server of a run that has
ended, retires nothing. A report found written without a mark is marked published
before the journal goes on. A mark that says neither is unknown: it stops the mailbox before the journal's
first effect, and stops a heads-up published once, the sweep's removal of the letter and the answer to what a run
was sent. The recipient's run, the report and its marks are read, and the marks and the
report written, in one section of the recipient's lock, which a turn end holding its own
waits for, bounded; a report to the sender's own name is written inside the lock the
turn end already holds. So a sweep never runs between the look and the write, and an
attempt that finds the run live there also finds every earlier landing
([stage3-publication.md](v2/stage3-publication.md#why-it-is-enough)). When the wait
expires the turn end fails as for any busy mailbox: nothing is written, no stop is
recorded, and the next barrier publishes the report once. A report whose recipient is a
run that has ended or was replaced, as read inside that section, is moot: the journal
records it so, and what it answered is cleared.
The fallback target is also fixed before publication, never selected again on retry.

The report's id is derived from the wait — the reporting run, the waiting run,
when the wait began, and its message ids — and a report whose id is already in the recipient's
mailbox, in any stage, is not written again. A hook that died between writing
and forgetting therefore costs nothing at the next turn: its retry, or whoever completes
its journal, forgets the same wait. A read retried after its last step failed does not record the
wait a second time: the `read` status, written after the wait, says it is a
retry. A payload that does not
arrive within three seconds is treated as no payload. Their wrappers announce it:
`Rewake: cx finished, 1 new message`.

The hook only records; waking is the recipient wrapper's job, through the same
notice as any message. A hook cannot wake anything out of deep idle, and it
runs while its own session is still awake anyway.

Two rules keep this from turning into a loop:

- reading finished, error, stopped, pending or notify asks for nothing back;
- each waiting run is told once; a new run of the name starts with no waiters.

So an exchange ends: A writes to B; B reads, answers, ends its turn and reports
to A; A reads both, ends its turn and reports to B; B reads the report, which
asks for nothing, and the exchange is over. The cost is that last wake of B. An
earlier version spared it by treating any message to a waiting run as the
answer, and lost the report whenever that message was a new request or a note
sent mid-turn.

`turn-ended` is hidden from the guide and never fails loudly: it runs inside the
harness's own machinery, where an error is noise at best.


### Reports after a thread change

Both harnesses can change conversations inside one wrapper run: Codex with `/new`,
Claude Code with `/clear` or `/resume`. Before an owed message is made readable, the
wrapper records the selected thread in `inbox/<name>/threads/<message id>` under the
mailbox lock — Codex's from its server, Claude Code's the conversation its telemetry
collector heard last ([delivery-adapters.md](delivery-adapters.md#claude-code-adapter)).
On Codex the server call uses that same thread. The context survives reads and status rewrites, and is retained
while the message is queued, unread or included in an unsettled wait.

At the end of a turn the current thread comes from the harness: Codex's completion
carries it through the gateway, and Claude Code's Stop hook names it as `session_id`.
If any known delivery thread in the wait's message ids differs, the report carries
`threadChanged: true`. Otherwise the field is omitted, including when either identity
is unavailable. This is an advisory comparison, not a change to report routing
or the wrapper epoch. A new conversation does not clear waits or resend tasks.

`rewake inbox` prints this line beneath a marked report:

```
Rewake: the reader's thread changed after delivery; this may not answer it, resend the message
```

A waiting `send --question` also prints the warning and preserves the boolean in
its JSON result. The caller decides whether to resend. Existing idempotent
report publication still applies.

### Turn ends that are not an ordinary report

A failed turn, a keyboard stop and a turn end marked with `rewake pending` each reach
the waiters differently from a `finished` report; they are in
[turn-outcomes.md](turn-outcomes.md).
