# pi: the adapter

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
  explicitly passed `-e` / `--extension` entries still load. The adapter adds an embedded
  TypeScript entry and its management module, written only in the run directory. Runtime
  imports are Node built-ins and that sibling module; package imports supply types and
  need no installed dependency beside pi.
- `-na` / `--no-approve` ignores project-local resources requiring trust. It does not
  edit a trust record. These two flags intentionally limit discovery for this launch.
- `--append-system-prompt` is repeatable. The adapter appends its briefing even when the
  caller already supplied one. `--no-intro` suppresses only rewake's briefing.
- `--session-id` opens or creates the named conversation. The native CLI rejects it with
  `--session`, `--continue` or `--resume`. The adapter also rejects a fork or
  `--no-session` when rewake names a new conversation. The id does not add a user message.
  Rechecked October 18, 2026 on installed 1.2.0 in `docs/cli.md`, `docs/sessions.md` and
  `dist/main.js` (`validateSessionIdFlags`, `createSessionManager`): an exact id searches
  the current project's session directory and creates the session if absent. Thus a space's
  fresh random `--session-id` must not be classified as continuation beside `--worktree`;
  that mistaken classification caused the pi worker's exit 2. Continue, resume, session and
  fork flags still refuse a new checkout. Session storage can be shared through
  `--session-dir`, the environment or native settings; the fresh random id, not an
  assumption about that storage, keeps a space independent of existing conversations. These are source checks, not
  a new live acceptance run.
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

## Telemetry and remote control

Read on the same installed 1.2.0 in `dist/core/extensions/types.d.ts`,
`dist/core/extensions/runner.js`, `dist/core/agent-session.js` and
`dist/modes/interactive/interactive-mode.js`:

- `ctx.model` and `ctx.thinkingLevel` are live getters. `ctx.getContextUsage()` returns
  native token usage, context window and percent; model context window remains available
  before usage is known. Immediately after compaction, usage and percent are null until a
  later assistant usage observation. The adapter preserves those unknowns; it does not
  substitute the estimated compaction result or apply another harness's token baseline.
- State is pushed at turn and compaction boundaries and settings changes, and polled
  through that same extension once a second. A sample sequence orders pushes and poll
  answers. A disconnected connection or three seconds without a valid observation makes
  the cached sample stale. The wrapper's snapshot publisher never waits on a native call.
- `ctx.compact({customInstructions, onComplete, onError})` returns void. Native manual
  compaction calls abort before preparing its input; the adapter therefore refuses a busy
  session before calling it and holds notices pending while compaction is in progress.
  Disabling automatic compaction in native settings does not disable this manual command.
- `session_before_compact` proves a start; before it, native preparation can refuse with
  `Already compacted` or `Nothing to compact (session too small)`. These map to
  `nothing to compact`. A request with no observed start after one second returns
  `requested`; its final outcome still follows. Other failures report a fixed explanation,
  not an error that could contain conversation text.
- `session_compact` counts a completed manual or automatic compaction. The completion
  callback separately supplies `tokensBefore` and optional `estimatedTokensAfter` for a
  remote request's outcome. Native summary text is never sent to the wrapper. Counts are
  observed since wrapper start, deduplicated by event sequence on each connection; gaps
  make coverage partial. No missed event or outcome is recovered from history.
- `ctx.abort()` also returns void. Interrupt waits up to three seconds for the matching
  `agent_settled` with an aborted outcome; no settled event means an open failure, not
  proof that the turn stopped. The stopped report names its initiator. A native custom
  message, appended without starting a turn, leaves that same actor in the next turn's
  history. An idle session refuses interrupt as `no turn running`.

A third mock probe loaded both shipped modules and verified on 1.2.0:

1. Protocol 2 hello and requested state exposed the selected model, thinking level,
   window and native usage; an empty conversation refused compact and interrupt.
2. An idle compaction accepted a focus in its native summary request, acknowledged
   start, emitted one completion and a final outcome with both token counts. A notice
   during that compaction was explicitly left pending. The native compactor made two
   summary requests; the focus appeared in the first.
3. Usage and percent became null after compaction, with the window still known. The
   next ordinary turn made usage known again.
4. Compact during a delayed model response was refused as `in a turn`. Interrupt of
   that response settled aborted, reported the actor, and put the actor's interruption
   trace in the next mock model request. That next turn completed normally.

The mock used synthetic usage, so the result proves the native counts were transported,
not that its summaries reduced context. Main owns interactive and real-room acceptance.

## The adapter's wire contract

One JSON object per line, at most 4 MiB, over a private unix socket listened on by the
host. Each side numbers its requests independently with a positive `id`; only
`op: "answer"` resolves a waiter. The adapter checks `SO_PEERCRED` against the announced
harness pid and rechecks its process start time before accepting the first frame.

| Direction | Operation | Fields and acknowledgment |
|---|---|---|
| Extension to host | `hello` | `protocol: 2`, `thread`, initial `state`; success makes Wake, TurnBoundary and Telemetry live, plus Control when its directory exists |
| Host to extension | `wake` | `thread`, `notice`; answer says `ok` and whether it was `steered`, or `pending` while compacting |
| Extension to host | `turn-start` | `thread`, `turn`; acknowledgment precedes model work |
| Extension to host | `turn-end` | `thread`, `turn`, `outcome`, `text`; acknowledgment follows publication |
| Either | `state` | host asks with `thread`; extension answers or pushes `state` with sequence, activity, model, effort, tokens, window, percent and compacting |
| Host to extension | `control` | `thread`, `action`, `request`, `from`, optional `focus`; answer carries outcome, reason, detail, open and token counts where known |
| Extension to host | `compacted` | `thread`, event `sequence`, optional remote `request` and `from`; acknowledgment follows counting |
| Extension to host | `compaction-result` | `thread`, `request`, `from`, outcome and optional token counts; acknowledgment follows retaining the outcome for main's letter |
| Either | `answer` | matching `id`, `ok` or operation-specific result, optional `error` |

Peer announcement, hello, wake and control wait at most five seconds. Each publication
attempt has its own five-second deadline, at most three attempts with 200 ms between them;
the extension waits twenty seconds for an end acknowledgment. The socket reader captures
the read boundary before launching publication separately. Retries reuse that captured
end and event id. Handlers and the control-directory server count among the host's workers
so a publication outlasting close retains its lease.

Disconnect withdraws the live capabilities. Reconnect backs off from 250 ms to eight
seconds while the session runs. A new connection replaces the old one; the old connection
cannot withdraw the new live set or finish a turn in another conversation. There is no
durable outbox, end replay after reconnect or transcript recovery. A notice whose
acknowledgment is lost fails without automatic resend. A control request with a lost
answer has an open outcome and is not repeated. Conversation changes are checked at
delivery, but this adapter does not reserve a conversation across inbox publication.

The current offer is Wake, TurnBoundary with Reports and AfterStart, Telemetry, Control
with compact, focus and interrupt, and Conversation through `--session-id`. HoldsEnd is
false; Limits and a live Spaces offer are absent.

## Live acceptance on a real model (October 18, 2026)

Main's run of `.scratch/process/live-pi.sh` on the branch's build, pi 1.2.0 with the person's own
settings (provider `openai-codex`), a print-mode Claude Code main on haiku, a private
`REWAKE_DIR` (`.scratch/pi-evidence/live/`). **[live]** The boundary went live about 1 s after
the start; a question came back with the word from a file; after 45 s idle a question woke the
worker and was answered; main's status line showed the worker's context (`1% used / 272K`); an
interrupt from main during a 90 s shell sleep was taken, and the next question was answered;
`rewake compact worker-pi '<focus>'` was refused as "nothing to compact" while the conversation
was under pi's `keepRecentTokens` (20 000 by default), and after three reads of about 12 000
tokens it started, the next answer showed `compactions 1` with the context down from 15% to 6%,
and the word the focus named was kept. The compaction's letter goes to main's run, which a
print-mode main has already ended by then — the same trap as a report to it (`docs/traps.md`).
