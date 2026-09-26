# Research: the Codex app-server protocol

Split out of [research.md](research.md) on September 21, 2026, with the same division:
these facts are obtained from the protocol schema the installed binary generates and
from the reference tree, not by watching a session behave. They change when the
protocol changes, which is why they are checked mechanically — the workflow suite
compares the fixture's messages against a freshly generated schema.

What that catches and what it does not: a schema states the shape of a message — which
fields are required, what type each carries. It says nothing about when a message is
sent, whose state it reflects, or how a refusal behaves. Facts of that kind live in
[research.md](research.md), where they are observed rather than generated.

Tags: **[verified live]** — live on the named versions; **[source]** — read in the
source; **[docs]** — official pages. Facts here age with harness versions: recheck
before touching an adapter.

**[verified live; Codex CLI 0.155.1; September 21, 2026]** The installed binary emits
its own protocol schema: `codex app-server generate-json-schema --experimental --out
<DIR>` writes a bundle of about 4 MB, including the summary files
`codex_app_server_protocol.schemas.json` and `…v2.schemas.json`. Both flags matter.
`--out` is required — without it the command refuses. `--experimental` is required for
a schema that describes the protocol rewake actually speaks: the default bundle omits
experimental fields, leaving the conversation type with 28 properties and no
`canAcceptDirectInput` at all, while the adapter uses that field and
`runtimeWorkspaceRoots`, both experimental. Checking a fixture against the default
bundle would therefore report correct answers as invented fields.

The workflow suite checks fixture replies against that schema, and its list of
understood schema keywords is closed: anything outside it fails the run rather than
passing quietly. That list was completed by walking every definition reachable from
the types the fixture answers, on 0.155.1. **Moving the version pin means walking
them again**: a keyword that turns up in a used type and is not in the register makes
every run red until it is implemented or classified. Doing that walk is part of the
pin move, not a surprise afterwards.

**[source: app-server schema of the installed CLI 0.155.1; September 21, 2026]**
`ThreadStartParams` carries no conversation id: a new conversation is named by the
server and the client learns it from the reply. The id appears only in
`thread/resume`, which is therefore the only place a client can disagree with the
server about which conversation it got. `InitializeParams` requires `clientInfo`
with both `name` and `version`, and `capabilities.experimentalApi` opts into
experimental methods and fields — which `runtimeWorkspaceRoots` and the thread's
`canAcceptDirectInput` are, so a client using them without declaring the capability
is asking for something it never negotiated. The adapter reads a conversation id
from both the request parameters and the reply
(`internal/harness/codex/gateway/metadata.go`), so it covers either shape; a fixture
that sends an id on `thread/start` is the part that is wrong.

**[source: app-server schema of the installed CLI 0.155.1; September 21, 2026]** The
delivery path has required fields a fixture is easy to get wrong. A `Turn` requires
`id`, `items` and `status` together — a turn carrying only an id is not a turn, and
`status` is one of `completed`, `interrupted`, `failed`, `inProgress`. That object
appears in the `turn/start` reply, in `turn/started` and in `turn/completed`, so all
three carry the list and the status. `ItemCompletedNotification` additionally requires
`completedAtMs`, a Unix timestamp in milliseconds, beside `item`, `threadId` and
`turnId`. `TurnStartParams` requires the `input` list to be present; a mailbox
delivery is the case where it is present and empty.

**[verified live; Codex CLI 0.155.1; September 21, 2026]** Two things the schema does
not say and a fixture gets wrong by default. A workspace root must be an absolute
path: the type is `AbsolutePathBuf`, which the JSON schema renders as a plain string
with the requirement only in prose, and the server refuses a relative root outright.
And a request repeating a field name is refused as if the field were missing —
JSON allows the repetition and leaves the choice of winner to the reader, so two
readers of the same request disagree: decoding into a Go struct keeps the earlier
object's fields while a map keeps the later one's.

**[source: reference tree `e29eceb75`; September 22, 2026]** What the server answers
when a `turn/start` arrives at a thread that is already working, which is the fork a
mid-turn delivery depends on. `start_or_steer_turn`
(`codex-rs/core/src/codex_thread.rs:332`) tries to steer first: `start_or_steer`
(`codex-rs/core/src/session/turn_input.rs:276`) calls `steer_input`, which fails with
`NoActiveTurn` on an idle thread and otherwise queues the input into the running turn
and answers that turn's id. The app-server builds the same reply either way
(`request_processors/turn_processor.rs:646–712`): a `TurnStartResponse` whose turn
carries `items: []` and `status: inProgress`. So a steered delivery differs from a
fresh one in the id it comes back with and in nothing else.

No second `turn/started` follows a steer: that event is emitted where a task begins
(`codex-rs/core/src/tasks/regular.rs:51`), and a steered input starts no task. The
turn ends once, with one `turn/completed` for the id both deliveries share.

A mailbox delivery can steer even though its `input` list is empty. `steer_input`
refuses empty *user* input, but a delivery carrying a tool output is submitted as a
response item instead — `turn_processor.rs` builds `TurnInput::ResponseItem` when
`toolOutput` is present — and `start_or_steer` accepts a standalone function-call
output as explicit input. The core names this case itself: the queue call is
`extend_pending_input_and_accept_mailbox_delivery_for_turn_state`.

**[source: app-server schema of the installed CLI 0.155.1; September 22, 2026]** A
client learns that a thread is working from `ThreadStatusChangedNotification`, which
requires `threadId` and `status`. `ThreadStatus` is a tagged union, and its `active`
variant requires `activeFlags` beside `type` while `idle` requires the type alone —
so a fixture that reported working with the type alone would be sending a status the
server cannot produce. The adapter reads exactly this notification for its telemetry
(`internal/harness/codex/gateway/telemetry_activity.go`), which is why a fixture that
never sent one left every session looking idle while it worked.

**[source: reference tree `e29eceb75`; September 21, 2026]** `canAcceptDirectInput`
is a field of the thread object, not of the reply. It is declared inside `ThreadData`
(`codex-rs/app-server-protocol/src/protocol/v2/thread_data.rs:274`), beside `source`
and `thread_source`. The adapter reads `result.thread.canAcceptDirectInput` and only
falls back to a top-level copy
(`internal/harness/codex/gateway/metadata.go`), so a fixture that answers on the top
level exercises the fallback and leaves the main branch untested. Two further fields
there are distinct: `source` is where the conversation came from (cli, vscode, exec,
app-server), `thread_source` is an analytics classification — the client sends the
latter, the server answers with both.

## What a turn shows of its input

**[source: reference tree `e29eceb75`; September 25, 2026; write-claude]** A turn
started by `turn/start` records its user input as a `userMessage` item when its task
runs (`core/src/session/mod.rs`, `record_user_prompt_and_emit_turn_item`, reached from
`record_pending_input` in `core/src/hook_runtime.rs`). The input is recorded also when a
pre-turn compaction fails or is aborted, and when finding the input's MCP servers is
cancelled: `run_turn` calls `run_hooks_and_record_inputs` on those paths before it
returns (`core/src/session/turn.rs`). Not always, though: a `UserPromptSubmit` hook that
stops the input keeps it from being recorded, and no item is sent
(`run_hooks_and_record_inputs`, `inspect_pending_input`); an input submitted as a
response item — a delivery carrying a tool output — is recorded without a
`userMessage` item. The reply to `turn/start` names the turn in every case, as above. A
goal's turn starts with no request (`codex_thread.rs`, `start_turn_if_idle`), so no
reply names it.

**[source: reference tree `e29eceb75`; September 25, 2026; review-claude and
review-codex, round 7 of stage 2 part B]** A goal's turn takes its input as a response
item carrying a contextual fragment (`ext/goal/runtime.rs`), which the item mapping
drops (`event_mapping.rs`): one that fails at its first model call — a usage limit, a
network error, a server error — ends with no item at all. A turn whose input a
`UserPromptSubmit` hook blocks has none either (`core/src/session/turn.rs`). When the
terminal disconnects, the server drops the connection's subscription and defers
unloading the conversation (`thread_processor.rs`, `thread_lifecycle.rs`); a review it
has queued is not cancelled and runs later (`handlers.rs`).

## Compaction and the terminal's commands over the protocol

**[source: app-server schema of the installed CLI 0.155.1 and reference tree `e29eceb75`;
September 23, 2026]** Whether rewake could compact a Codex conversation, or run another
of the terminal's commands, on its own behalf. The Claude Code side is in
[research-claude-control.md](research-claude-control.md#commands-from-outside).

`thread/compact/start` takes `{threadId}` and nothing else, and replies `{}`. The work
shows as events: `turn/started`, an item of kind `contextCompaction`, `thread/compacted`,
a token-usage update, `turn/completed`. The core aborts the running tasks before it
compacts, so a compaction requested mid-turn cuts that turn short; the terminal offers
`/compact` only while idle.

rewake injects the request itself through the gateway's `callReserved`
(`internal/harness/codex/gateway/gateway.go`). The gateway marks a thread as being in
manual compaction when the terminal asks (the `thread/compact/start` branch of its
request loop), and since part B of stage 2 also when `rewake compact` does
([remote-control-codex.md](remote-control-codex.md)).

The terminal's commands and what they send:

| Command | Protocol |
| --- | --- |
| model, effort, plan mode, working directory | a settings update |
| rename | `thread/name/set` |
| goal | the goal methods |
| review | `review/start` |
| a shell command | `thread/shellCommand` |
| background terminals | their own list and clean-up methods |
| interrupt | `turn/interrupt` |
| the read-only lists | their list methods |
| `/new`, `/clear`, `/resume`, `/fork` | none: which conversation is shown is the client's own state |
| commands that act only in the client | none |

## Compaction and interrupt on request

**[source: app-server schemas of 0.155.1 and 0.156.0, generated in a container, and
reference tree `e29eceb75`; September 24, 2026; review-claude]** What `rewake compact`
and `rewake interrupt` would have to send on Codex; their design is in
[remote-control.md](remote-control.md). The live run is below.

- **No focus for one compaction.** `thread/compact/start` takes `{threadId}` in both
  versions and has no field for instructions. What the summary is asked to keep is set
  only by the configuration key `compact_prompt`: in `config` of `thread/start` or
  `thread/resume`, or written into the configuration file by `config/value/write`;
  `thread/settings/update` does not take it. It replaces the whole prompt for the whole
  conversation, so it is no way to pass a focus for one request.
- **Mid-turn it would cut the turn.** The core's `compact()` first calls
  `abort_all_tasks(TurnAbortReason::Replaced)`: a compaction asked during a turn ends
  that turn instead of being refused. rewake has to refuse on its own before it sends.
- **`turn/interrupt`** takes `{threadId, turnId}` in both versions and replies `{}` only
  after the turn is aborted. Idle it is refused with `no active turn to interrupt`, a
  wrong id with `expected active turn id X but found Y`; the turn's status becomes
  `interrupted`. The same shape as the Claude Code plugin's `$.turn.abort`.
- **A turn runs before it is announced.** The reply to `turn/start` and the turn's
  `turn/started` are sent from different code paths (`turn_processor.rs` and
  `bespoke_event_handling.rs`), with no order between them: between the two nothing on
  the connection says a turn runs, and a compaction sent then aborts it **[source,
  reference tree; September 24, 2026; review-claude]**.
- **The model sees the interrupt.** The core records
  `<turn_aborted>The user interrupted the previous turn on purpose…` in the history;
  Claude Code's plugin abort records nothing, which is why rewake adds a line to the
  interrupted session's next notice there.

**[verified live; Codex CLI 0.155.1; September 24, 2026; write-claude]** Part B built,
run in a private state directory with a stand-in main, the cheapest model the CLI
offered at effort `low`, one short turn per check; the gateway's traffic was recorded
by a logger in a throwaway build, not in the tree.

- **The order for a compaction on rewake's request.** The injected
  `thread/compact/start`, its reply `{}` about a millisecond later, then
  `thread/status/changed` active, `turn/started`, `item/started` with the
  `contextCompaction` item, `thread/tokenUsage/updated` for the compaction's turn,
  `item/completed`, `thread/status/changed` idle, `turn/completed` with status
  `completed`. No `thread/compacted` appeared on that connection. It took 4 seconds on
  a conversation that had run no turn and 9 after one short turn.
- **An empty conversation is compacted.** Codex compacts a conversation that has run no
  turn — it spends a model call on it — where Claude Code refuses "Not enough messages
  to compact". rewake answered "compacted … (compaction 1)" with no token counts, since
  no usage had been seen before. After one turn: "17263 tokens before, 4939 after
  (compaction 2)".
- **The terminal during that compaction.** A message typed meanwhile went out as
  `turn/steer`, which the server refused at once; the terminal held the message and
  sent it as `turn/start` right after the compaction's `turn/completed`, where it
  started an ordinary turn. Nothing was lost. The terminal also ran a temporary helper
  conversation of its own for the title, with `thread/start`, `turn/start` and
  `thread/unsubscribe`.
- **What the terminal shows.** For the compaction it did not ask for: "• Context
  compacted · 4s". For an interrupt on rewake's request: "■ Conversation interrupted -
  tell the model what to do differently. Something went wrong? Hit `/feedback` to report
  the issue." — the same as for its own Esc.
- **The order for an interrupt.** The injected `turn/interrupt`, its reply `{}` 9–11 ms
  later, before `thread/status/changed` idle and `turn/completed` with status
  `interrupted`. main's `rewake inbox --awaited` then showed "stopped: lead-claude
  interrupted this turn with rewake interrupt", and main received the stopped message
  with that text. An idle interrupt was refused as no turn running in 0.08 s; a focus
  was refused with exit 2 before anything was sent. main received no compaction notice
  for either of its two compactions.

**[live; Codex CLI 0.155.1; September 24, 2026; test-codex]** A probe of the real
app-server for main's round-5 questions, on an ephemeral conversation of its own, effort
`low`, eight short ordinary turns; times in UTC.

- **Input during a compaction is refused, not queued.** A `turn/start` sent while a
  compaction ran got
  `{"code":-32603,"message":"failed to submit turn input: ActiveTurnNotSteerable { turn_kind: Compact }"}`.
- **The reply to `turn/start` came before `turn/started` in all eight turns**, in the
  same millisecond in five of them and in that order even then (for example 20:03:48.855
  and 20:03:48.855; 20:04:00.693 and 20:04:00.695). The source allows the reverse
  order, so the gateway keeps its guard for it.
- **A manual compaction's turn has one item, and it comes first.** The reply `{}` at
  20:04:13.879; at 20:04:13.889 status active, `turn/started`, and `item/started` of the
  `contextCompaction` item, the turn's first item; `thread/tokenUsage/updated` at
  20:04:18.342; then in the same millisecond `item/completed` of that item, status idle
  and `turn/completed` with status `completed`.
- **An interrupt**: its reply `{}` at 20:04:18.347, then status idle and
  `turn/completed` with status `interrupted` in the same millisecond.

## Codex 0.156.1 against the fixture

**[schemas of 0.155.1 and 0.156.1 generated in a container; workflow suite through the
summarizer with `REWAKE_CODEX_VERSION=0.156.1`; September 24, 2026; write-claude]**
Checked before the owner updates from 0.155.1. Live behaviour was not checked: no
session of 0.156.1 was started.

- **The suite is green against its schema**: 74 scenarios, 95 cases, 92 pass and 3
  unsupported — the Claude Code column's known capabilities — with the schema case's 19
  observations holding.
- **Nothing rewake uses changed.** `thread/compact/start`, `turn/interrupt`,
  `thread/status/changed`, `thread/tokenUsage/updated`, the `Turn` object and its
  statuses, and the `contextCompaction` and `agentMessage` items are the same;
  `turn/steer` changed only a description. No field became required and no type
  changed on these paths.
- **Additions, all optional**: `disabledPluginIds` on `TurnStartParams`, on the
  responses of `thread/start`, `thread/resume` and `thread/fork` and on
  `ThreadSettings`; `daybreakEnabled` on `ThreadStartParams`; `collaborationMode` on
  the `thread/resume` response; `mcpAppUi` on the `mcpToolCall` item; and fields of
  configuration, account, models and MCP servers. A new request `rollout/compress`
  starts a background compression of cold rollout files; it is not a compaction of the
  context.
- **Changes**: `personality` is marked deprecated and stays; an image in `UserInput`
  and `ContentItem` takes a URL or a file id, where the URL was required. rewake sends
  only text.
- **Removed**: the request `thread/rollback` with its params and response, which rewake
  never sends.
- **Server notifications**: no method added or removed; the item and turn
  notifications changed only through the nested definitions above.

## Where a new conversation runs

**[schemas of 0.155.1 and 0.157.1, experimental; source at release tags `rust-v0.155.1`
and `rust-v0.157.1`; September 26, 2026]** Read for Codex's `--worktree`
([research-codex.md](research-codex.md#--worktree-with-a-remote-terminal)).

- **`ThreadStartParams` has no worktree field** in either version. It has an optional,
  nullable `cwd` and `runtimeWorkspaceRoots`, absolute paths.
- **The server applies `cwd` per conversation**, as an override, so the directory the
  server process started in does not fix where a conversation runs. A client that
  created a checkout itself could start a conversation there over the protocol.
- **The checkout is the terminal's work**, not the server's: a local terminal allocates
  it and sends only its path in these two fields.

## The plan over the protocol

**[schemas of 0.155.1 and 0.156.1, experimental; reference tree; September 25,
2026]** What the protocol offers for the model's checklist, the `update_plan` tool
([research-codex.md](research-codex.md#the-plan-tool)).

- **The server announces it** as `turn/plan/updated`, with `threadId`, `turnId`,
  `explanation` and `plan: [{step, status}]`. The notification spells the status
  `inProgress` where the tool's argument has `in_progress`. The schema is the same,
  byte for byte, in both versions.
- **A client has no request that sets it**: `ClientRequest` holds none, and none that
  runs a built-in tool. `mcpServer/tool/call` is for MCP tools, and a `toolOutput` on
  `turn/start` hands in a finished result rather than running anything.
- **Not this checklist**: `ThreadItem::Plan {text}` and `item/plan/delta` carry the
  prose plan of Plan mode.
- **A standing instruction for a turn**: `additionalContext` on `turn/start` and
  `turn/steer` with `kind: application` reaches the model as a developer message, and
  stays in the history; each fragment is cut at about 1000 tokens, and an unchanged
  value under the same key is not necessarily sent again **[source]**. rewake does not
  use it.
