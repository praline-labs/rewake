---
description: How each harness carries a notice once the wrapper sends it — Claude Code's socket line, its inbound gate, reply socket and late receipts; pi's extension; the fixture — and where each adapter's turn ends come from.
---

# Delivery adapters

How a notice reaches each harness once the wrapper's inbox server has decided to send it
([delivery.md](delivery.md#servicing-process-the-wrapper)): the line written to a Claude Code
session and what its receipts say, the pi extension's frame, and the fixture. The mailbox, the
lock, the notice texts and reading are in [delivery.md](delivery.md).

## Claude Code

Connect to the session's inbound socket, `sock/<name>.<epoch>.sock`, with a 2-second timeout,
write the line
`{"type":"user","message":{"role":"user","content":<notification>},"priority":"next"}`
followed by `\n`, then close. The content is

```
<task-notification>
<task-id>rewake-<short id></task-id>
<status>completed</status>
<summary><the notice></summary>
</task-notification>
```

Claude Code picks how to draw a user message from its text, and draws this one as
`● <summary>`, with the preview on the next line. The status colours the circle: `completed`
is green and `failed` is red; the kind is named in the summary. The short id keeps two
identical notices apart: Claude Code drops identical text from the same sender within 30
seconds ([research.md](research.md#how-a-socket-message-is-drawn)).

**The inbound gate and its receipts.** A successful write says only that the line reached the
session. Its inbound gate then accepts the line, holds it, or refuses it
([research-launch.md](research-launch.md#claude-codes-cross-session-inbound-gate)), and tells
a sender that asks. So every line asks: it carries `"from":"uds:<reply socket>"` and a UUID
`"msg_id"`, and the wrapper listens on that reply socket (`internal/adapter/claude/lane.go`).
The receiver sends a receipt only to a `.sock` in the same directory as its own socket, and
only after checking that the listener is the process that wrote the line — the same pid, uid
and start time. That is why the wrapper both writes and listens: `rewake send` only puts a
file in a mailbox and has exited long before a hold ends. The reply socket is the session
socket's name with `.reply` before `.sock`, mode 0600; beside a socket the caller named with
`--messaging-socket-path` it is `rewake-<hash>.reply.sock` in that socket's directory. That
directory may lie outside the room, where no lease of the state reaches, so the lane binds
there under a `state.SocketClaim`: the run's authority over that one path — clearing a dead
run's socket, binding, the close-time unlink — while this process holds the room's lease
([rules/state.md](rules/state.md#views-and-changes)). A directory in a state, of this base or
another, is no such place: there the room the path lies in decides, as written and where its
directory's links lead.

Each receipt is one line on a new connection, quoting the line's id in `orig_msg_id`. After a
write the adapter waits 300 ms for the first one (`receiptWindow`) — a held line was answered
within 30 ms live, and an accepted one gets none, so each accepted line costs the whole
window:

| receipt | result |
|---|---|
| none within the window | `delivered` — the gate accepts silently |
| `held` | `held`, with the receiver's reason |
| `delivered` | `delivered`, "released after being held" |
| `expired` with `status_detail: "refused"` | `failed`, the session refuses messages from other sessions |
| `expired` | `failed`, the hold expired unreleased |
| `denied` | `failed`, the person declined it |
| `dropped` | `failed`, with `drop_reason`; every id in `dropped_msg_ids` too |

**A late word.** A later receipt about a held line goes to the inbox server, which settles the
message as above. A delivery counted from silence is counted only for now: nothing bounds how
soon a loaded receiver speaks, and a hold must never stay counted as delivered. So the adapter
keeps each such line, and the inbox server the members of its notice, for a minute or the last
128 notices (`mail.LateWordWindow`, `mail.LateWordKeep`), and a late word takes the delivery
back:

| late word | result |
|---|---|
| `held` | `held` again, unless the agent has read the message already; the hold then ends through a later receipt or the session's end, as any other |
| `expired`, `denied`, `dropped` with no `held` before it | `failed` — the line never reached the agent |
| `delivered` | nothing changes |

Either way a task or question that ends `failed` sends its sender the undelivered note: the
sender may have exited 0 on `delivered` long before. A word after the minute is ignored.
Receipts come on separate connections, so two that follow closely may be read in either order;
the final one wins either way, since a `held` read after it finds the line forgotten. An
unknown status says nothing. Without a reply socket the line goes without `from` and `msg_id`,
the wrapper says so on stderr, and a successful write means `delivered`.

**The first notice.** The gate also holds whatever arrives in the first moments after the
socket appears, until the session has drawn its interface
([research.md](research.md#the-inbound-gate-on-rewakes-line)). The wrapper therefore keeps the
first notice back for three seconds from the start of its lane (`openingLimit`).

**A wrapper killed outright** closes nothing: its reply socket stays in `sock/`, and what its
session held stays `held` on disk. The next lane to start in that directory removes every
`*.reply.sock` older than five seconds that a dial is refused on — a file that exists, or a
dial that fails otherwise, proves nothing, and a younger socket may be bound by a wrapper
starting beside it and not listening yet; those stay (`replysweep.go`). The next session with
the name fails what the dead one held and tells each sender, as the dead one would have on a
clean exit.

`ENOENT` and `ECONNREFUSED` mean `pending` as long as the harness is alive (the socket has not
been created yet, or is being recreated); otherwise `failed`.

**Turn ends and the conversation.** Claude Code's turns come through rewake's mod, not
through the socket: the mod sends `turn/start` and `turn/end` to the host over the mod↔host
protocol, the end carrying its reason, its answer and the conversation it ran in
([claude-turns.md](claude-turns.md)). A delivery records no thread on Claude Code: the
adapter names none when a message is made readable, so a report there never carries
`threadChanged`.

## pi

A per-run extension connects to the wrapper's unix socket
(`internal/adapter/pi/extension.ts`). A protocol-2 hello with the current conversation and
native state, from the exact process the wrapper started, makes Wake, the turn boundary and
Telemetry live, plus Control when the run has its control directory. The extension reconnects
after a lost socket, `/new` or `/reload`; a replaced socket cannot withdraw its successor's
capabilities.

The wrapper sends a `wake` frame with the notice and the expected conversation. The extension
checks the conversation and calls `pi.sendMessage` with `triggerTurn: true` and
`deliverAs: "steer"`. An idle session starts work; a busy one takes the notice after its tool
batch. An `answer` means the native call accepted the notice, not that a model request or
inbox read succeeded. Before a connection exists the delivery is pending; after a write whose
answer is lost it fails without an automatic replay. During compaction the extension refuses
queue acceptance with `pending`, so delivery waits and retries after compaction ends.

`agent_start` is acknowledged before model work. `agent_end` supplies the last assistant text,
while `agent_settled` reports the end after automatic work. An aborted settled event reports
aborted; a final `stop` reports answered; other stop reasons report failed. The reader captures
the end's read boundary before publication, which runs separately so it cannot block answers
to notices. Up to three publication attempts use the same event and capture. There is no
durable outbox, transcript scan, or replay across a reconnect; a lost end requires main to ask
again. The adapter cannot hold an end for an interim confirmation. A delivery records the
conversation it went into, so a report after a `/new` carries `threadChanged`.

The protocol, bounds and probes are in [research-pi.md](research-pi.md).

## The fixture

The workflow suite's stand-in harness (`internal/adapter/fixture`, built only with the
`rewaketest` tag) takes a notice as a frame on its socket and answers with how it took it; it
reports turn ends with an end frame, and is the one adapter that can hold an end open for the
confirmation ([turn-outcomes.md](turn-outcomes.md#the-confirmation)). A test can withhold any
of its capabilities ([testing.md](testing.md#the-fixtures-readiness-exchange)).
