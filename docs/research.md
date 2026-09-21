# Research: how to deliver a message to a live harness session

Gathered September 15-16, 2026. Versions: Claude Code 2.1.270, Codex CLI 0.154.0.
Tags: **[verified live]** — live on these versions; **[source]** — source;
**[docs]** — official pages. Recheck facts before changing the adapter.

**[verified live; Codex CLI 0.155.1 and Claude Code 2.1.270; September 21, 2026]**
How each harness takes a model and a reasoning effort for one launch, read from the
installed binaries and checked by running them.

**Read this before trusting a `--help` probe.** Appending `--help` to a command is the
cheap way to ask "does this CLI accept that spelling", and on these two harnesses it
answers different questions — or none:

| Command | Exit | What it actually tells you |
| --- | --- | --- |
| `claude --definitely-not-a-flag x --help` | 0 | nothing: Claude Code takes any unknown flag beside `--help` and prints the help |
| `codex --unknown --help` | 2 | the spelling is rejected — the useful form |
| `codex --help --unknown` | 0 | nothing: `--help` before the flag short-circuits |
| `codex -c not-a-setting --help` | 0 | nothing about the *setting*: only the flag's spelling was checked |

So on Codex the probe works in exactly one arrangement — the flag first, `--help` last
— and only for how a flag is written, never for what a configuration setting contains.
On Claude Code it does not work at all, and a form has to be checked by running the
command without `--help`: `claude -m x` answers "unknown option '-m'", which is how it
is known that Claude Code has no short form for either flag. Claude Code takes both
as flags: `--model <model>` and `--effort <level>`, where the levels it lists are
low, medium, high, xhigh and max. Codex takes the model as `-m`/`--model <MODEL>`,
but has no flag for the reasoning effort: it is the configuration key
`model_reasoning_effort`, set for a single launch through the repeatable
`-c key=value` override. The key name is confirmed in the reference tree at
`e29eceb75` (`codex-rs/core/src/config/edit.rs:227`). Codex also accepts the model as
the configuration key `model`, so a person can state that choice either way. Its
override parser splits a setting on the first `=` and trims both halves
(`codex-rs/utils/cli/src/config_override.rs`), which is why `-c 'model = "x"'` and
`-c model="x"` are the same setting — verified live on 0.155.1, both accepted. Neither harness needs its
configuration file touched for either setting.

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

**[verified live; Codex CLI 0.155.1; September 21, 2026]** The installed CLI moved to
0.155.1 and the adapter's pin moved with it. Observed on that version: launch with
registration, a task accepted through the app-server, a `finished` report back and
telemetry collection ([claude-parity-2026-09-21.md](claude-parity-2026-09-21.md));
and, in a second probe, steer — a message sent inside the second of four sleep-10
calls, with the worker observed `working` in a snapshot taken just before the send,
reached the recipient at the next boundary between tools, the series continued and the
original task returned its own result. Not observed on this version: conversation
selection after `/new`, refusal of a stale target, `stopped` from a keyboard
interruption, and a multi-message group arriving during an active turn. One probe
each, no repeats and no race coverage. Every fact below carries the version it was
taken on; a fact tagged 0.154.0 has not been re-checked on 0.155.1.
September 20: [native mailbox contract](native-mailbox.md) and
[owner/installed acceptance with evidence limits](native-mailbox-acceptance.md).

## Claude Code

### Session inbound socket

- Every interactive session listens on a unix socket. The default path is
  `$XDG_RUNTIME_DIR/cc-socks/<pid>.sock`; the hidden flag
  `--messaging-socket-path <path>` sets it explicitly. **[verified live]**
- Requirements for an explicit path: absolute, no `..`, no longer than 103 bytes, the
  parent directory owned by the user with 0700 permissions. If a live socket is
  already listening on that path, startup fails. **[binary source 2.1.270]**
- The socket is created before hooks run; the session exports `CLAUDE_CODE_MESSAGING_SOCKET`
  and `CLAUDE_CODE_MESSAGING_TOKEN` into the environment of its children (Bash, hooks). **[verified live]**
- On shutdown (including via SIGHUP) the socket and the registry entry are removed. **[verified live]**
- Protocol: one JSON line terminated by `\n`, then close the connection — no reply.
  ```json
  {"type":"user","message":{"role":"user","content":"text"},"priority":"next"}
  ```
  `priority`: `now` interrupts the turn, `next` (default) — after the current
  tool's result, `later` — at the end of the turn. Line limit is 1 MiB. **[binary
  source]** The `priority` field is not officially documented.
- **Wakes an idle session**: the turn starts on its own. Checked twice — a message
  to the socket of a session that had been idle for 90 seconds, and a message to a
  session started with an explicit `--messaging-socket-path`: response after 3
  seconds. **[verified live]**
  Officially: "When the receiving session is idle, Claude Code starts a new turn
  with the message". **[docs: code.claude.com/docs/en/cross-session-messaging]**
- The model sees text headed "Another Claude session sent a message", followed by
  a paragraph noting that the peer session cannot grant elevated permissions. **[verified live]**
- The token is optional on Linux and macOS; trust rests on the 0700 directory, the
  0600 socket, and checking the sender's uid. On Windows it's a named pipe with a
  mandatory token. **[binary source, docs]**
- Limits:
  - identical text from the same sender within 30 seconds is dropped;
  - the recipient's queue holds at most 50 messages, and a sender gets about 30 in a row;
  - `crossSessionInbound` (`accept|hold|refuse`) in settings can delay or refuse
    incoming messages; without it, a session in `bypassPermissions` holds the
    message as a dialog until the sender declares the same mode;
  - the mechanism can be disabled remotely (the `tengu_harbor_kite` flag) or via an
    environment variable; it disables itself silently if creating the socket
    directory fails;
  - slash commands from incoming messages are not executed. **[binary source, docs]**

### How a socket message is drawn

- The interface picks the drawing of a user message from its text, not from a
  protocol field. Content that contains `<task-notification>` with a
  `<summary>` is drawn as one line, `● <summary>`, coloured by `<status>`
  (`completed` green, `failed` red, `killed` yellow, anything else plain) — the
  line a finished background task gets. The model still receives the peer
  wrapper around it ("Another Claude session sent a message…"); only the screen
  is clean. **[verified live, 2.1.270; source: `UserTextMessage.tsx` →
  `UserAgentNotificationMessage` in an older published build]**
- The protocol has `type: "control"` frames too: `rename`,
  `peer_message_status`, `notify_when_idle` and `peer_idle_notice`, among
  others. `peer_idle_notice` also draws as a "finished" line, but it is accepted
  only as the answer to a `notify_when_idle` the receiving session sent itself
  through its own `SendMessage`; an unsolicited one is dropped as
  "uncorrelated". An outside process cannot use it. **[binary source 2.1.270]**
- `ListAgents`/`SendMessage` see every interactive Claude Code session on the
  machine through its socket, not only subagents. **[verified live]**

### Hooks for one launch

- `--settings <file-or-json>` adds a settings layer for the run. Layers are
  merged (`mergeWith`), so hooks in it run next to the user's own. Only one
  `--settings` is read. **[source, older published build; verified live that the
  hook runs]**
- The Stop hook receives `last_assistant_message` on stdin. **[source; verified
  live]**
- A hook runs while its session is awake; it cannot wake anything from deep idle.
  Waking goes through the socket. **[owner]**

### Claude Code's own session registry

`~/.claude/sessions/<pid>.json`: `pid`, `sessionId`, `cwd`, `messagingSocketPath`,
`status` (`busy|idle|waiting`), `kind`, `version`, `peerProtocol`, `procStart`.
**[verified live]** The format is private and versioned — read it as a hint, not
a contract.

### Child process environment

A process launched from under a Claude Code session inherits its markers
(`CLAUDECODE`, `CLAUDE_CODE_CHILD_SESSION`, `CLAUDE_CODE_SESSION_ID`,
`CLAUDE_CODE_MESSAGING_SOCKET`, `CLAUDE_CODE_MESSAGING_TOKEN`, `CLAUDE_PID`, and
others). **[verified live]** A child Claude Code with `CLAUDE_CODE_CHILD_SESSION`
set disables transcript saving. In a neighboring project, the adapter strips
twelve variables:
`CLAUDECODE`, `CLAUDE_CODE_CHILD_SESSION`, `CLAUDE_CODE_SESSION_ID`,
`CLAUDE_CODE_BRIDGE_SESSION_ID`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_EXECPATH`,
`CLAUDE_CODE_MESSAGING_SOCKET`, `CLAUDE_CODE_MESSAGING_TOKEN`, `CLAUDE_PID`,
`CLAUDE_PLUGIN_DATA`, `CLAUDE_EFFORT`, `CLAUDE_CODE_SUBAGENT_MODEL`.

### Other useful bits

- `--append-system-prompt <text>` appends text to the system prompt for a single
  run. **[CLI help]**
- The `SessionStart` hook can export variables into the Bash session by appending
  `export K=V` to the file at `$CLAUDE_ENV_FILE` (this is how the official codex
  plugin does it). **[source: plugin]**
- The first run in a new folder shows a trust dialog: default is "No, exit",
  confirming takes down-arrow then Enter. **[verified live]**
- In the default mode, the agent calling Bash requires human confirmation.

## Codex CLI

### Delivery: `codex queue`

- `codex queue --thread <id|exact name> --message <text>`, run from any process,
  places the message in a durable queue (`$CODEX_HOME/queue_1.sqlite`, method
  `thread/queue/add`). Every Codex process, including a plain TUI, polls the
  queue once every 10 seconds and starts a turn on its own if the thread is idle.
  **[source: ext/queue/src/service.rs]**
- An idle TUI wakes up without a daemon and without `--remote`, with a delay of
  about 10 seconds. **[verified live]**
- Mid-turn, the message waits and triggers the next turn once the current one
  finishes; after an explicit interrupt there's no auto-start. **[source]** Not
  verified live.
- **A brand-new thread with no messages yet cannot accept one**: the rollout file
  is created lazily, and `queue` responds with
  `no rollout found for thread id <id> (code -32603)`, exit code 1. **[verified live]**
- Unknown name: `No active session found matching '<name>'`, exit code 1.
  **[verified live]**

### Thread identity and terminal events

The server adapter uses thread/started and thread/closed, not TUI file
descriptors. Locks remain a fact of the standalone TUI, but under --remote the
server owns them. A root thread source/originator and absent parent id distinguish
TUI work from nested agents. The message sidecar still carries the delivery id.

Legacy notify emits normal completions but skips some error branches. The
adapter now consumes turn/completed: completed, failed with error.message, or
interrupted. Intermediate error notifications with willRetry=true are not final.
No transcript is read to obtain the result or infer a missing failure.

### Sandbox (Linux)

See the preserved [permission research](research-permissions.md#sandbox-linux).

### Git metadata writes by role

See the preserved [permission research](research-permissions.md#git-metadata-writes-by-role).

### Managed worktrees and continuation permissions

See the preserved [permission research](research-permissions.md#managed-worktrees-and-continuation-permissions).

### Remote continuation permissions

See the preserved [permission research](research-permissions.md#remote-continuation-permissions).

### The sandbox has its own pid namespace

The commands a Codex agent runs are started under `bwrap --as-pid-1`, so inside
them `/proc` holds only the sandboxed process itself. `/tmp` is the real one —
a file written there appears outside — but every pid from outside is missing.
**[verified live]** Anything judging "is that process alive" from inside the
sandbox therefore concludes "no" about every session but its own. A reader has to
compare `/proc/self/ns/pid` before believing a pid it did not create.

### Environment and instructions

- `shell_environment_policy` inherits the environment by default, excluding names
  matching the patterns `*KEY*`, `*SECRET*`, `*TOKEN*`. **[source]** The tool's
  variable names must not match these patterns.
- The `developer_instructions` config key can be set per run via
  `-c developer_instructions="..."`; it overrides the user's value if one is set.
  **[source]**
- The first run in a new folder shows a trust dialog, confirmed with Enter; the
  decision is written to `~/.codex/config.toml`. **[verified live]**

### Session-owned app-server

**[snapshot 44b9011; CLI 0.154.0; September 17, 2026]** Foreground
app-server --listen unix://PATH supports a TUI via --remote. The listener uses
WebSocket frames, not JSONL; proxy is only a byte relay. turn/start calls
start_or_steer_turn (request_processors/turn_processor.rs:646–677), including
after an interruption. Basic turn/steer is stable and requires expectedTurnId;
queue/start is experimental and is not needed for immediate delivery.

**[owner live probes, 0.154.0]** [Same-turn steering](/tmp/rw13/live-steer-ok.jsonl)
and [fresh /new delivery](/tmp/rw13/live-new-ok.jsonl) reached the TUI and model.
A closed old thread can absorb input without a visible answer, and resuming an
empty thread did not reliably reply. Track closure; never deliver to a cached
id after losing evidence that it remains current.

**[source and no-model probes]** [Research report](/tmp/rw13/report.md) records
RPC initialization, failed and interrupted outcomes, owned locks and the remote
configuration boundary. TUI notify is not forwarded; developer instructions are
conditional on a feature there, so launch configuration reaches the server too.
TUI --add-dir is carried as runtimeWorkspaceRoots; -C and positional input are
forwarded. REWAKE_* survives default shell environment filtering.

The owned server uses CODEX_INTERNAL_APP_SERVER_REMOTE_CONTROL_DISABLED=1
(transport/remote_control/mod.rs:87) to keep this local launch off the persisted
remote-control path. This internal marker and the remote configuration rules
are version-specific. The later [installed native-mailbox acceptance](native-mailbox-acceptance.md)
closes the supported launch/delivery path; raw protocol probes alone did not do so.

## Other harnesses (for later)

- **pi** — no native entry point. A custom extension, loaded with the `-e` flag,
  listens on a socket and calls `pi.sendUserMessage(text, {deliverAs})`: it starts
  a turn from idle, crashes mid-turn without `deliverAs`, and refuses during a
  manual `/compact`. `--session-id` is set up front, `PI_SESSION_ID` is visible in
  the agent's bash, and the end of a turn is the `agent_settled` event. **[source]**
- **opencode** — `POST /session/<id>/prompt_async` with
  `{"parts":[{"type":"text","text":"..."}]}` wakes it from idle, and queues
  mid-turn. The TUI only listens on the network with `--port`; a session can be
  created ahead of time (`POST /session`) and opened with `--session ses_…`;
  protected by `OPENCODE_SERVER_PASSWORD`. **[source]**
- **grok** — entry only in the hidden leader mode (`--leader`,
  `~/.grok/leader.sock`); the default mode has no external entry point. **[source]**

See [prior-art notes](prior-art.md) for other messaging designs.

## Failed-turn observation

In snapshot `44b9011`, `hooks/src/legacy_notify.rs:15–39` emits only thread/turn
ids, cwd, input messages and last-assistant-message. It carries no error field.
`core/src/session/turn.rs:638–702` dispatches it on normal completion; the error
branches at `:742–783` bypass it. `core/src/tasks/mod.rs:798–810` puts the error
on TurnComplete instead. Therefore a rollout task_complete error is not evidence
that legacy notify received that reason. Rewake accepts that event if supplied,
and emits a textless error for a received empty completion after read work, but
legacy notify alone cannot observe a callback that the harness never invokes.
The server transport replaces that dependency for the server-backed harness. It does not
read transcripts or invent a cause.

The [official hook reference](https://code.claude.com/docs/en/hooks#stopfailure)
specifies StopFailure instead of Stop for API failures. Its rendered
last_assistant_message is the error text, with error_details and error as
fallbacks. Rewake installs StopFailure for every role, including main. This is
schema/documentation verification; no live model turn was used for this change.

**[owner socket probe: 2.1.270, September 17, 2026]** A newline inside the
task-notification summary renders an indented second line. Extra sibling fields
are discarded by the interface, so the authored first-line preview belongs in
summary itself. The server transport uses a second line in the same message.

**[hook schema checked September 17, 2026; installed version 2.1.270; no live run]**
[Common hook fields](https://code.claude.com/docs/en/hooks#common-input-fields)
distinguish inherited hooks inside a subagent by `agent_id`. `agent_type` also
appears on a root session launched with an agent profile, so it cannot filter
child callbacks. Rewake ignores completions carrying a child identity.

**[CLI declarations: 2.1.270 source `main.tsx:988`; snapshot `44b9011`]**
`--allowedTools <tools...>` is variadic. The TUI CLI also has a variadic
`--image` (`utils/cli/src/shared_options.rs:11–19`, `num_args = 1..`). A trailing
`--` separates a caller prompt from either option's values.

**[snapshot 44b9011; CLI 0.154.0; September 17, 2026]**
thread/loaded/list starts with an omitted or null cursor. For a nonempty loaded
set, an empty string is an invalid ThreadId, not the first page. Only a returned
nextCursor belongs in the next request (`thread_processor.rs:2732–2777`).
[Gateway native evidence](gateway-native-evidence.md) and [128 MiB transport limits](startup-transport.md#confirmed-native-message-size-failure-and-repair) record
the owner-accepted prototype and integration ordering; [ownership research](thread-ownership-investigation.md) preserves the earlier investigation.
The [native resume matrix](thread-lock-probes.md) verifies conditional recreation on the fingerprinted 0.154.0 build; original live-event attribution remains open.
[Primary state metadata](session-state.md) records the source-backed contract; [September 19 owner acceptance](session-activity.md#evidence-and-acceptance) separates observed activity/notices from unexercised live cases. [Native start-or-steer and outcome evidence](native-terminal-progress.md) separates prompt dispatch from terminal reporting.
