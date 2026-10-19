# Delivery: turning a turn's end into a report

Split from [delivery.md](delivery.md) by subject on September 27, 2026: that document
carries how a message is sent, queued and read; this one carries what happens once the
reader's turn ends — how it becomes a report, how a report notes that
the reader's conversation changed underneath it, and where the turn ends that are not an
ordinary report are described.

### The end of a turn

Callbacks with a nonempty `agent_id` belong to a nested agent and are ignored
before touching the turn's records or waits. `agent_type` alone
does not imply nesting: a root session can select an agent profile.

The harness's adapter hands each turn's end to its wrapper, which passes it to the
internal reporting function (`ReportCompletion`, or `ConfirmCompletion` for an end the
adapter can hold open, in `internal/cli/completion.go`) with explicit session epoch,
thread identity and the read boundary it captured. Until S9, October 11, 2026, Claude
Code's Stop and StopFailure hooks ran the hidden `rewake turn-ended` with the last
reply in their payload; that path left with the hooks, and Claude Code reports no turn
end in this build. Under the mailbox lock, for
every eligible run in `awaiting/<own epoch>/` it leaves a `finished` message whose text
is that reply, and whose `inReplyTo` lists the messages read from that run since its
last report, addressed to that run — not to whoever holds the name now. The rules this
follows are 7 and 8 of [mail-bridge-cli.md](archive/mail-bridge-cli.md); how each record is
recovered, record by record, is [turn-end-recovery.md](turn-end-recovery.md), and this
section is the path through the code.

A turn end is one operation, named by its cause: a hash of the run, the event's id, its
kind and its start and end on the boot clock (`turnOp` in
`internal/core/mail/turn_reports.go`). Its scope is fixed by what the event carries. An
end that names an event carries a read boundary, a position on the run's read clock
(`internal/core/mail/read_boundary.go`): what was read or kept at or below it is this
end's, and a task read or an answer kept above it belongs to a later end, however late a
retry comes. An end that names an event and carries no boundary reports nothing, on any
attempt, and its waits stay owed. An end heard once — one that names no event — has no
retry, and its one attempt under the lock is its scope. The pending mark that makes the
end interim is found in the turn's window: from the later of the turn's start and the
latest earlier end of the run a done journal records (`TurnWindowStart`), to its end. No end
removes a mark: marks stay for the run's life ([turn-outcomes.md](turn-outcomes.md)).

Decided by the owner, October 18, 2026: the turn-end journals with their plan and seam,
the stops, the once-marks of reports, the barrier and `rewake decide` are replaced by the
simple sequence below. No live loss, duplicate, stopped mailbox or decision was ever seen
in real work, and the mechanics itself stopped a mailbox on a file it did not know. A
report lost when a process is killed in the milliseconds between two steps is asked for
again by main.

Under the sender's mailbox lock the end runs its steps in one order
(`publishTurnContext` in `internal/core/mail/turn_reports.go`): it reads the scoped
waits, the window's pending mark and the hold as above; for each waiter whose run is
still the live run of its name (read without the name lock's cleanup) it writes the report
into the recipient's mailbox unless a letter of that id is there in any stage
(`writeTurnReport`); then it clears the recorded message ids of each wait — later messages
remain owed to the next result — drops the kept answer only when it is still the version
carried, and records the interim end, never over a record of the same run that ended
later, nor over the record of another run that holds the name
(`internal/core/mail/interim.go`). Last it writes `journal/<op>.done`
(`internal/core/mail/journal.go`): the operation, its end time and the held end it took.
A retry of the same event finds it and answers from it; the next end's window opens after
it. A done journal is kept while its run lives; the live run's sweep removes those of
ended runs a day later.

Writing the report before clearing the wait is the order that matters: a cleared wait
counts as answered (`internal/core/mail/awaited.go`), so the other order, cut short,
would leave an answer on record and no letter. A failure returns at the step it met, and
a retry of the same event runs the steps again from what the disk says — the wait still
names what is owed, the report's id is fixed by the wait, and a report found written is
not written again. Killed before the report is written, the end leaves the wait standing,
and the sender reads the task as owed or gone (`rewake inbox --awaited`).
Killed after, the letter is with the sender already. Two narrow holes are closed in the
code. The sweep of a recipient keeps a report whose writer's wait still names what it
answers (`reportStillAwaited` in `internal/core/mail/sweep.go`), so a retry that comes
after a day still finds the letter. A report is not written
when every task of its wait already has a finished or failed report of the name in the
sender's mailbox (`internal/core/mail/answered.go`): the run died between writing the report and
clearing the wait. A failure report to main, which answers no wait, is found again by
its operation in its id's tail on a retry (`reportOfOp`).
The report's id is derived from the wait — the reporting run, the waiting run,
when the wait began, and its message ids — and a report whose id is already in the recipient's
mailbox, in any stage, is not written again. An end cut off between writing
and forgetting therefore costs nothing at the next turn: its retry forgets the same wait. A read retried after its last step failed does not record the
wait a second time: the `read` status, written after the wait, says it is a
retry. Their wrappers announce it:
`Rewake: cx finished, 1 new message`.

The turn end only records; waking is the recipient wrapper's job, through the same
notice as any message.

Two rules keep this from turning into a loop:

- reading finished, error, stopped, pending or notify asks for nothing back;
- each waiting run is told once; a new run of the name starts with no waiters.

So an exchange ends: A writes to B; B reads, answers, ends its turn and reports
to A; A reads both, ends its turn and reports to B; B reads the report, which
asks for nothing, and the exchange is over. The cost is that last wake of B. An
earlier version spared it by treating any message to a waiting run as the
answer, and lost the report whenever that message was a new request or a note
sent mid-turn.

### Reports after a thread change

A harness can change conversations inside one wrapper run, as a person's `/new` or
`/clear` does. Before an owed message is made readable, the wrapper records the selected
thread in `inbox/<name>/threads/<message id>` under the mailbox lock, as its adapter
names it (a `ThreadSource`, `internal/adapter/thread.go`); an adapter that names none
records nothing. Claude Code named its conversation through its hooks until S9,
October 11, 2026, and names none in this build
([delivery-adapters.md](delivery-adapters.md#claude-code-adapter)). The context survives reads and status rewrites, and is retained
while the message is queued, unread or included in an unsettled wait.

At the end of a turn the current thread comes from the harness, in the completion its
adapter hands the wrapper.
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
