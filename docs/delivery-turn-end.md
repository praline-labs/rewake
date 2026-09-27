# Delivery: turning a turn's end into a report

Split from [delivery.md](delivery.md) by subject on September 27, 2026: that document
carries how a message is sent, queued and read; this one carries what happens once the
reader's turn ends — how `turn-ended` turns it into a report, how a report notes that
the reader's conversation changed underneath it, and where the turn ends that are not an
ordinary report are described.

### The end of a turn

Callbacks with a nonempty `agent_id` belong to a nested agent and are ignored
before touching turn receipts or waits. `agent_type` alone
does not imply nesting: a root session can select an agent profile.

A hook runs `rewake turn-ended` with the last reply in its payload. The owned
server backend sends completion events directly to the same internal reporting
function, with explicit session epoch and thread identity. Backend completions
carry an observation-time read boundary; only its eligible still-owed messages are
selected. Under the mailbox lock, for every eligible run in `awaiting/<own epoch>/` it
leaves a `finished` message whose text is that reply, and whose `inReplyTo`
lists the messages read from that run since its last report, addressed to that run —
not to whoever holds the name now. Before publishing an identified turn, its
receipt in `turns/` stores the waiter/message snapshot and complete report batch:
recipient epochs, ids, kind, text, timestamps and thread-change annotations.
Retries load this batch, even if the callback payload or current waits changed.
All reports must be published before the receipt is marked done. Cleanup removes
only the recorded message ids from the same run and wait; later messages remain
owed to the next result. A recipient whose run ended is skipped. The fallback
target is also fixed before publication, never selected again on retry.

The report's id is derived from the wait — the reporting run, the waiting run,
when the wait began, and its message ids — and a report whose id is already in the recipient's
mailbox, in any stage, is not written again. A waiter that could not be removed,
or a hook that died between writing and forgetting, therefore costs nothing at
the next turn. A read retried after its last step failed does not record the
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
