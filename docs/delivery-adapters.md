# Delivery adapters

How a notice reaches each harness once the wrapper's inbox server has decided to send it
([delivery.md](delivery.md#servicing-process-the-wrapper)): the line written to a Claude
Code session and what its receipts say, and the Codex gateway. The mailbox, the lock, the
notice texts and reading are in [delivery.md](delivery.md).

## Claude Code adapter

Connect to `claude.socket` with a 2-second timeout, write the line
`{"type":"user","message":{"role":"user","content":<notification>},"priority":"next"}`
followed by `\n`, then close. The content is

```
<task-notification>
<task-id>rewake-<short id></task-id>
<status>completed</status>
<summary><the notice></summary>
</task-notification>
```

Claude Code picks how to draw a user message from its text, and draws this one
as `● <summary>`, with the preview on the next line. The status colors the
circle: `completed` is green and `failed` is red; the kind is named in the
summary. The short id keeps
two identical notices apart: Claude Code drops identical text from the same
sender within 30 seconds.

A successful write says only that the line reached the session. Its inbound gate
then accepts the line, holds it, or refuses it
([research-launch.md](research-launch.md#claude-codes-cross-session-inbound-gate)),
and tells a sender that asks. So every line asks: it carries `"from":"uds:<reply
socket>"` and a UUID `"msg_id"`, and the wrapper listens on that reply socket. The
receiver sends a receipt only to a `.sock` in the same directory as its own socket,
and only after checking that the listener is the process that wrote the line — the
same pid, uid and start time. That is why the wrapper both writes and listens:
`rewake send` only puts a file in a mailbox and has exited long before a hold ends.
The reply socket is the session socket's name with `.reply` before `.sock`, mode
0600; beside a socket the caller named with `--messaging-socket-path` it is
`rewake-<hash>.reply.sock` in that socket's directory.

Each receipt is one line on a new connection, quoting the line's id in
`orig_msg_id`. After a write the adapter waits 300 ms for the first one — a held line
was answered within 30 ms live, and an accepted one gets none, so each accepted line
costs the whole window:

| receipt | result |
|---|---|
| none within the window | `delivered` — the gate accepts silently |
| `held` | `held`, with the receiver's reason |
| `delivered` | `delivered`, "released after being held" |
| `expired` with `status_detail: "refused"` | `failed`, the session refuses messages from other sessions |
| `expired` | `failed`, the hold expired unreleased |
| `denied` | `failed`, the person declined it |
| `dropped` | `failed`, with `drop_reason`; every id in `dropped_msg_ids` too |

A later receipt about a held line goes to the inbox server, which settles the message
as above. A delivery counted from silence is counted only for now: nothing bounds how
soon a loaded receiver speaks, and a hold must never stay counted as delivered. So the
adapter keeps each such line, and the inbox server the members of its notice, for a
minute or the last 128 notices (`inbox.LateWordWindow`, `inbox.LateWordKeep`), and a
late word takes the delivery back:

| late word | result |
|---|---|
| `held` | `held` again, unless the agent has read the message already; the hold then ends through a later receipt or the session's end, as any other |
| `expired`, `denied`, `dropped` with no `held` before it | `failed` — the line never reached the agent |
| `delivered` | nothing changes |

Either way a task or question that ends `failed` sends its sender the undelivered note:
the sender may have exited 0 on `delivered` long before. A word after the minute is
ignored. Receipts come on separate connections, so two that follow closely may be read
in either order; the final one wins either way, since a `held` read after it finds the
line forgotten. An unknown status says
nothing. Without a reply socket the line goes without `from` and `msg_id`, the
wrapper says so on stderr, and a successful write means `delivered` as before.

The gate also holds whatever arrives in the first moments after the socket appears.
The wrapper therefore keeps the first notice back until the session draws its status
line, which is when it stops holding — not SessionStart, which live runs showed comes
too early ([research.md](research.md#the-inbound-gate-on-rewakes-line)). With no
status line — the settings were refused, or a managed policy owns the status line —
the first notice waits three seconds.

A wrapper killed outright closes nothing: its reply socket stays in `sock/`, and what
its session held stays `held` on disk. The next lane to start in that directory removes
every `*.reply.sock` older than five seconds that a dial is refused on — a file that
exists, or a dial that fails otherwise, proves nothing, and a younger socket may be bound
by a wrapper starting beside it and not listening yet; those stay. The next session with the name fails
what the dead one held and tells each sender, as the dead one would have on a clean exit.

`ENOENT` and `ECONNREFUSED` mean `pending` as long as the harness is alive (the socket
hasn't been created yet, or is being recreated); otherwise `failed`.

**Which conversation a message went into.** Claude Code names its conversation in every
hook and status line as `session_id`, which `/clear` replaces with a new one, `/resume`
with the resumed conversation's, and a compaction keeps
([research-claude-control.md](research-claude-control.md#interrupting-a-turn-and-changing-the-conversation)).
The telemetry collector keeps the last one it heard, and the wrapper asks it when it pins
an owed message to a conversation before making it readable
([delivery.md](delivery.md#reports-after-a-thread-change)). At the end of the turn
`rewake turn-ended` takes the conversation from the Stop or StopFailure payload's own
`session_id`, so the comparison is with where the turn actually ended rather than with a
snapshot of the collector. Either side unknown — no telemetry socket, no event heard yet,
a payload without the field — leaves `threadChanged` out.

One window marks a report that did not need it: a message delivered after a `/clear` but
before that SessionStart hook reached the collector — the hooks run in the background — is
pinned to the old conversation, and its report then reads as changed. The mark is
advisory, so a false one costs the sender a resend it did not need, where a missing one
would let an answer from a fresh conversation pass for an answer to the task. Covered by
the `thread-changed` workflow case on the fixture since September 23, 2026
([testing.md](testing.md)); not yet observed live (HF-10).

**How a turn ends for rewake.** The Stop hook for a turn that answered, StopFailure for
one that failed, each running `rewake turn-ended`, and — since September 23, 2026 — the
plugin's `turn.complete` with reason `aborted` for a turn a person interrupted, which
runs neither hook ([claude-plugin.md](claude-plugin.md)). The last one reaches the
wrapper through the telemetry collector rather than a hook process, and the wrapper
publishes it as `stopped` itself, with the read boundary it holds at that moment. The
plugin's other turn ends are not reported, so an ordinary turn still gives one report;
nor is an `aborted` end after the turn's Stop or StopFailure hook ran, which the module
itself leaves out.
Where the plugin does not load, an interruption is not heard at all, and the session's
telemetry says so (`interruptions: unobserved`).

## Codex adapter

The wrapper's gateway follows the native TUI's own accepted primary intent and
matching direct-input reply. It does not discover a loaded root or attach an
observer through resume. The transport reserves epoch/connection/generation/thread
before inbox readability and holds admission through the delivery ACK. A-B-A never
reuses an old reservation. See [gateway selection and admission](gateway.md).

Readiness waits for ordinary resume backfill outside the mailbox lock, admission
FIFO and global gate. Native reads and approval replies remain able to progress.
The reservation also orders explicit main-authorized additive Git roots and native settings
updates. No permission policy is replaced. A bounded refusal sends no work and
exposes no task; transport uncertainty is never automatically replayed.

The admitted-work ledger preserves matching outcomes after selection changes.
A separate publisher journals callbacks with causal read boundaries and uses the
durable report/turn receipt path asynchronously; later reads cannot join an older
result. See [stable publication](report-publication.md). Socket readers never wait on mailbox locks. Upstream
completion observation is not proof of terminal receipt. The owner-run synthetic
terminal evidence and current compatibility limits are recorded in
[native gateway evidence](gateway-native-evidence.md).
