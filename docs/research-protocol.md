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
