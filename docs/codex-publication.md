# Which Codex turn outcomes are published

How the Codex gateway decides which turn outcomes reach the waiters, and why the second
and third safety properties of remote control hold under it. The properties, the rest
of the mechanism and its known limits are in
[remote-control-codex.md](remote-control-codex.md).

## The rule

The publication rule was decided by main on September 25, 2026, replacing the
withholding by marks of the round before. The outcome of a turn — finished, failed,
stopped, or `completion not observed` when its end passed unseen — is published as it
is only with proof that the turn is work: a successful reply to `turn/start`, `turn/steer` or `review/start` named it (a
detached review's reply names its turn in the review's own conversation), or it had an
item other than `contextCompaction`. A turn with neither settles nothing, whoever
started it, whatever the marks say, on any connection. Main decided on September 25,
2026, in round 8, that it is still reported, as advisory: a `stopped` report with the
text "a turn of this conversation ended without rewake seeing what it did", followed by
what is known of its end — the turn's error text, or that it was stopped — which goes to
the current waiters, settles nothing and keeps their waits, as a gap does. A turn whose
`contextCompaction` item shows it to be a compaction, and that has no proof of work, is
the one exception: it reports nothing, having nothing to report. A compaction stopped
before its item shows nothing of what it was, and reports as advisory.

When the proof comes after the end — a reply travels apart from the turn's events and
may follow its `turn/completed` — the outcome waits for it half a second, then goes as
advisory and still waits: a proof that comes later publishes the turn's own outcome, of
any kind, a stop included, with its start and end, and it is handled as any turn end —
a finish or an error settles the task, and a pending mark made during the turn is
taken by it. The advisory has an identity of its own, the turn's id with `/advisory`
after it, so neither the gateway's publication nor the turn receipts take the two for
one report; the advisory carries no start or end, and takes no pending mark. The waiting outcomes are kept per connection, 16 at most; one
pushed out, or left when the connection ends, whose reply is lost with it, goes as
advisory if it has not yet. The gateway keeps the proofs for all its connections
(`internal/harness/codex/gateway/proof.go`), so a turn named on one connection reports
when it ends on the next, after the terminal reconnects. The rule reads no mark: marks
serve only main's answer, the telemetry's author and holding deliveries.

A gap — a run the gateway sees only as an active status and then idle, its turn never
named — may have been work or a compaction, and nothing tells which. Main decided on
September 25, 2026 that it is always published, but only as advisory: a `stopped`
report with the text "a run of this conversation passed unseen by rewake; whether it did
your task is not known here", which goes to the current waiters, settles nothing and
keeps their waits, as a keyboard stop does
([turn-outcomes.md](turn-outcomes.md#keyboard-stops)). Silence would leave the sender
owed with no word; a settling report could be a compaction's. The next turn end that
finishes settles the task. A turn without proof of work is reported the same way, for
the same reason.

Why the third property, that a compaction's turn never settles a task, holds: a manual compaction's turn gives neither proof. `thread/compact/start`
answers `{}` and input into a running compaction is refused, so no reply names it; the
local, remote and token-budget paths emit the one `contextCompaction` item and nothing
else, and the compaction's hooks come as `hook/*`, not items (sources in
[remote-control-codex.md](remote-control-codex.md#what-the-gateway-knows-a-conversation-is-doing),
under why the item ties the mark). So its turn never settles a task — running after a goal's turn
took its mark, or ending on the next connection, it publishes nothing, its item showing
what it is; stopped by Esc or by its PreCompact hook before its item, it shows nothing
and is reported as advisory — with no mark having to recognize it.

Why the second, that no outcome is silently dropped, holds: a turn the terminal types or a delivery sends is named by the reply to
its `turn/start`, which always carries the turn, the started one or the one steered into
([research-protocol.md](research-protocol.md#what-a-turn-shows-of-its-input)); a steer's
reply names the running turn, a review's its own. Such a turn also records its input as
a `userMessage` item, though not always — a `UserPromptSubmit` hook that blocks the input
records none — so the reply is the proof that comes on a live connection; it is lost
when the connection ends before it. A goal's turn that works has items. A turn with no
reply and no item may still have something to report — a goal's turn failing at its
first model call, on a usage limit or a network error, before any item
(`ext/goal/runtime.rs`; its input is a contextual fragment the item mapping drops,
`event_mapping.rs`), or a turn whose reply was lost with the connection and whose input
a hook blocked (`core/src/session/turn.rs`) — and reports as advisory, with its error
text. A gap reports as advisory, whether it was work or not. Nothing reported as
advisory settles a task, so (3) holds with it.
