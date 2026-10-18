# pi: the first adapter

Verified October 18, 2026 against installed `@earendil-works/pi-coding-agent` 1.2.0.
This package, not the older reference checkout, is the source for this adapter. The
minimum supported version is **1.2.0**; the launch version gate refuses older or
unreadable versions. Main also compared the 1.3.0 package on October 18: its extension
declarations were byte-identical to 1.2.0 and its changelog contained no extension, event or
`sendMessage` change. It was read as compatible, not live-probed here. Later releases still
need that comparison and the probes below before acceptance.

## Launch and extension lifetime

Read in the package's `docs/cli.md`, `docs/extensions.md`, `CHANGELOG.md` and
`dist/core/extensions/types.d.ts` on that version:

- `-ne` / `--no-extensions` disables discovered, configured and built-in extensions;
  explicitly passed `-e` / `--extension` entries still load. The adapter adds one embedded
  TypeScript file, written only in the run directory. Runtime imports are Node built-ins;
  its package import supplies types and needs no installed dependency beside pi.
- `-na` / `--no-approve` ignores project-local resources requiring trust. It does not
  edit a trust record. These two flags intentionally limit discovery for this launch.
- `--append-system-prompt` is repeatable. The adapter appends its briefing even when the
  caller already supplied one. `--no-intro` suppresses only rewake's briefing.
- `--session-id` opens or creates the named conversation. The native CLI rejects it with
  `--session`, `--continue` or `--resume`. The adapter also rejects a fork or
  `--no-session` when rewake names a new conversation. The id does not add a user message.
- `--model`, `--models`, and `--thinking` name model selection and effort. Thinking levels
  are `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`; a model selector can carry
  a thinking suffix. Defaults from `REWAKE_PI_MODEL` and `REWAKE_PI_EFFORT` yield to those
  explicit choices. They do not inspect or edit pi's own settings.
- Extension registration happens in its factory; connections start in `session_start`,
  when `ctx.sessionManager.getSessionId()` names the active conversation. Shutdown closes
  that connection. The handlers stay registered across the session's lifetime.

Read in `dist/bundle/cli.js`: the executable starts Node and loads the bundled runtime
in that same process. Seen in the private mock probe: the launched pid, extension's
`process.pid`, and unix socket's `SO_PEERCRED` pid were identical. A shell wrapper that
spawns pi as a child therefore fails the peer check; it must `exec` pi.

Seen in that probe: `/new` raises shutdown for the old session and two `session_start`
notifications for the new one. `/reload` raises shutdown and a fresh start in the same
pid and conversation. Connection startup must be idempotent; the shipped extension keeps
one opening promise and one ready connection per conversation.

## Wake and the end of a turn

Read in `dist/core/agent-session.js`, the extension declarations, and the installed
agent-core's `dist/agent-loop.js`:

- From 1.2.0, `sendMessage` requires `deliverAs`. The adapter uses
  `pi.sendMessage(message, {triggerTurn: true, deliverAs: "steer"})`: idle starts a run,
  busy queues a steering message for the next safe point after the tool batch.
- `sendMessage` returns no promise. Its return acknowledges native queue acceptance;
  an asynchronous model failure can follow it. The wire acknowledgment makes no claim
  that the model ran or read the inbox.
- `agent_start` handlers are awaited before the first model request. Automatic retries
  can produce more than one `agent_start` / `agent_end` pair. The extension groups them
  under one random turn id until `agent_settled`.
- `agent_end.messages` contains that run's messages. The adapter takes the last assistant
  message's text blocks and stop reason, without reading any transcript file.
- `agent_settled {aborted}` is emitted after automatic work finishes; its handlers are
  awaited, but it is a notification, not a hook that can hold the end open.
- The installed pi-ai `dist/types.d.ts` lists `pending`, `stop`, `length`, `toolUse`,
  `error`, `aborted`, `deferred`. In `dist/providers/faux.js`, `pending` is the streaming
  placeholder; `deferred` carries a handle to a result that is not yet available. Neither
  proves a completed answer. At settled, aborted wins; `stop` maps to answered; every
  other value, including no assistant message, maps to failed. There is no separate
  refusal classification in this first pass.

The extension reports no native boot-clock timestamp: Node's monotonic clock is not the
host's boot clock. The host's start acknowledgment precedes model work and supplies the
ordinary turn's lower bound. `TurnHandler.Started` receives zero, not an invented native
time, so it cannot prove that a pending mark belongs to an earlier turn whose end was
lost. This is a first-pass limit, as is the lack of held interim ends.

## What the mock probes observed

Private HOME, agent settings, working directory and loopback mock endpoint; no real model
and no personal credentials. The native RPC mode was a probe driver only; the adapter's
launch remains the normal interactive pi command.

The lifecycle probe recorded these results on 1.2.0:

1. An extension-injected idle notice produced `agent_start`, a final assistant message,
   `agent_end`, and one non-aborted `agent_settled`.
2. A notice injected while a bash tool slept appeared in the model request after that
   tool completed. The run produced one final settled event.
3. Abort during a delayed model stream produced an assistant with `stopReason: aborted`
   and `agent_settled.aborted: true`.
4. `/new` and `/reload` reopened the extension's socket; a notice after reload started a
   new run successfully. The process identity remained unchanged.

A second probe loaded the shipped `extension.ts` against a fake protocol peer and the
same mock model. It verified protocol hello, idle delivery and exact final text, a new
conversation's reconnect, busy steering, the aborted end mapping, and reload followed by
a successful wake. Interactive terminal rendering and delivery between real rewake sessions
are main's live acceptance, not claimed by these probes.

## The adapter's wire contract

One JSON object per line, at most 4 MiB, over a private unix socket listened on by the
host. Each side numbers its requests independently with a positive `id`; only
`op: "answer"` resolves a waiter. The adapter checks `SO_PEERCRED` against the announced
harness pid and rechecks its process start time before accepting the first frame.

| Direction | Operation | Fields and acknowledgment |
|---|---|---|
| Extension to host | `hello` | `protocol: 1`, `thread`; success makes Wake and TurnBoundary live |
| Host to extension | `wake` | `thread`, `notice`; answer says `ok` and whether it was `steered` |
| Extension to host | `turn-start` | `thread`, `turn`; acknowledgment precedes model work |
| Extension to host | `turn-end` | `thread`, `turn`, `outcome`, `text`; acknowledgment follows publication |
| Either | `answer` | matching `id`, `ok`, optional `error` |

Peer announcement, hello and wake wait at most five seconds. Each publication attempt has
its own five-second deadline, at most three attempts with 200 ms between them; the extension
waits twenty seconds for an end acknowledgment. The socket reader captures the read boundary
before launching publication separately. Retries reuse that captured end and event id.
Handlers count among the host's workers so a publication outlasting close retains its lease.

Disconnect withdraws the live capabilities. Reconnect backs off from 250 ms to eight
seconds while the session runs. A new connection replaces the old one; the old connection
cannot withdraw the new live set or finish a turn in another conversation. There is no
heartbeat, durable outbox, end replay after reconnect or transcript recovery. A notice whose
acknowledgment is lost fails without automatic resend. Conversation changes are checked at
delivery, but this first pass does not reserve a conversation across inbox publication.

Telemetry, compact, interrupt, and a live Spaces offer belong to the second pass. The
current offer is Wake, TurnBoundary with Reports and AfterStart, and Conversation through
`--session-id`; HoldsEnd is false.
