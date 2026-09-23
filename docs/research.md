# Research: how to deliver a message to a live harness session

Gathered September 15-16, 2026. Versions: Claude Code 2.1.270, Codex CLI 0.154.0.
Tags: **[verified live]** — live on these versions; **[source]** — source;
**[docs]** — official pages. Recheck facts before changing the adapter.

Two companions, split out on September 21, 2026 when this file passed 400 lines. The
division is by how a fact is obtained, because that is how it ages: what a binary
answers when you run it lives in [research-launch.md](research-launch.md) — models,
efforts, argument forms; what the protocol schema and the reference tree state lives
in [research-protocol.md](research-protocol.md). What stays here is what only a
running session shows: how a message reaches it, what it does with it, and how the
processes behave. On September 23, 2026 the file passed 400 lines again and was split
by subject: what a running Codex session shows moved to
[research-codex.md](research-codex.md), and this file keeps Claude Code, the other
harnesses and the failed-turn observations.

**[verified live; September 21, 2026]** A fact about our own checks rather than about a
harness, kept here because it is dated and ages like the rest: `go test` inherits a
session's `REWAKE_*` variables unless they are cleared, and the tests pass either way —
three packages were run with the variables set. So the reason to clear them is not a
red run. It is that a test which inherits them writes into the owner's live state
directory and reads the running session as its own.

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
- Protocol: one JSON line terminated by `\n`, then close the connection — nothing comes
  back on it; a receipt, when one is asked for, arrives on a socket of the sender's
  ([the inbound gate](#the-inbound-gate-on-rewakes-line)).
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
  - `crossSessionInbound` (`accept|hold|refuse`) and, when it is unset, the
    permission-mode parity gate can hold or refuse a message after the write has
    succeeded ([the inbound gate](#the-inbound-gate-on-rewakes-line));
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

### The inbound gate on rewake's line

**[verified live, 2.1.280; private HOME, a placeholder key and an unreachable API
endpoint, so no model was called; September 23, 2026]** What the code states about the
gate is in [research-launch.md](research-launch.md#claude-codes-cross-session-inbound-gate).
rewake's line, written and closed; the startup and settings probes added a reply address,
which the gate does not consult when it decides:

- **At startup it is held and released.** A session started in `default` mode: socket
  listening at 10:07:04.019 UTC, the line written at .115, `held inbound peer message
  (1 held, cause=mode-unknown)` at .121, `released 1 held peer message(s)
  (mode-changed)` at .228, and on screen `● Released 1 held cross-session message to Claude's queue
  (permissions are prompting again).` followed by the notice — what the owner saw on a
  restart. The window is the first two hundred milliseconds or so after the socket
  appears.
- **Once started it is accepted** in `default`, `acceptEdits`, `plan` (bypass not
  available) and `auto`, switched with shift+tab in one session: `Routed user message to
  queue` each time, no receipt, nothing written back on the connection.
- **Receipts need a reply socket.** The same line with a top-level `from:
  "uds:<dir>/reply-<pid>.sock"` and a UUID `msg_id`, the reply socket listened on by the
  writing process in the receiver's socket directory: at startup `held` came 30 ms after
  the write and `delivered` at 290 ms; under `--settings '{"crossSessionInbound":"refuse"}'`
  a `"status":"expired","status_detail":"refused"` receipt at once; under `hold` a `held`
  receipt, no prompt, and the transcript line `Held peer message — from
  uds:…/reply-<pid>.sock [verified pid <pid>]; preview: «<task-notification>» … — not
  delivered to Claude (1 held). Your "crossSessionInbound" setting is "hold"; set it to
  "accept" to deliver held messages.`; two Ctrl-C then settled both held messages as
  `expired`, delivered to the reply socket 2.3 s after the second write.
- **SessionStart is too early; the first status line is not.** Same setup, later the
  same day (11:13–11:14 UTC). Three sessions with a sync and an async SessionStart
  hook each writing a timestamp: both ran 64–85 ms after the socket appeared. Three
  more with the line, carrying a reply address, written the moment the async hook's
  file appeared (76, 83 and 81 ms after the socket): each was held 18–19 ms after the
  write, `permission-mode getter not wired (fail-closed → hold)` in the debug log,
  released about 100 ms later (`mode-changed`), and the `delivered` receipt came 305–339
  ms after the write. Three more with the line written when a status-line command
  first ran — 240–280 ms after the socket, 157–181 ms after SessionStart: `Routed user
  message to queue (priority=next)` each time, no receipt. So the wrapper opens on the
  first status line (`telemetry.Collector.Drawn`), not on SessionStart.
- Not run live: a receiver in `bypassPermissions` or in `plan` with bypass available
  (the launch was refused in this environment), the five-minute deadline, and the
  order of the repository, user and `--settings` sources.

### Hooks for one launch

- `--settings <file-or-json>` adds a settings layer for the run. Layers are
  merged (`mergeWith`), so hooks in it run next to the user's own. Only one
  `--settings` is read. **[source, older published build; verified live that the
  hook runs]**
- The Stop hook receives `last_assistant_message` on stdin. **[source; verified
  live]**
- What rewake reads from that payload (`internal/cli/turn_result.go`): `agent_id`,
  which decides whether the end counts at all — a child's inherited hook carries one
  and is ignored — then `type`, `hook_event_name`, `turn-id` or `turn_id`,
  `thread-id`, the last message under `last_assistant_message`,
  `last-assistant-message` or `last_agent_message`, and `error` with
  `error_details` for a failure. Of what the workflow suite's Claude Code fixture
  sends, only `last_assistant_message` on Stop is verified live; `hook_event_name`,
  `session_id`, `cwd`, `transcript_path`, and the StopFailure fields are
  **[assumed]**, shaped from the hook reference rather than from a running session.
  For StopFailure the fixture puts the error text in `last_assistant_message` with
  `error_details` and `error` beside it, following the reference cited below.
  September 22, 2026.
- Whether the real harness refuses an envelope carrying a field it does not serve is
  **[assumed]** too, and assumed the other way: the fixture refuses one. A fixture
  looser than the harness lets a scenario pass while the envelope is mangled on the
  way; stricter only costs a fixture change the day the envelope grows a field.
  September 22, 2026.
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

### Telemetry sources: the status line and hooks

Gathered September 23, 2026, Claude Code 2.1.280. **[verified live]** here is one
session of the installed binary under the owner's login, cheapest model, one turn
delivered through the inbox socket and one `/compact`. **[container]** is the cached
binary in the disposable container with no network and a placeholder API key: the
status line runs there, because it renders before any request. **[source]** is the
JavaScript bundled in the 2.1.280 binary.

What the `statusLine` command receives on stdin, one JSON object:
**[container; verified live]**

- `session_id`, `transcript_path`, `cwd`, `scratchpad_dir`; `version`; `workspace`
  (`current_dir`, `project_dir`, `added_dirs`); `output_style.name`.
- `model.id` and `model.display_name`.
- `effort.level` for a model that takes an effort — `medium` on Opus 5.5, then `high`
  after `/effort high`. For Haiku 4.5 the key is absent altogether.
- `context_window`: `total_input_tokens` is input plus cache creation plus cache read
  of the **last** response — the current fill, not a running total — beside
  `total_output_tokens`, `context_window_size`, `current_usage` (the four raw counts),
  and `used_percentage`/`remaining_percentage`, rounded and clamped to 0–100. Before
  the first response, and again after a compaction until the next one, the counts are
  0 and `current_usage` and both percentages are `null`. Live: 34727 of 200000, 17%,
  after one turn on Haiku 4.5; the window read 1000000 for both Opus 5.5 `[1m]` and
  Sonnet 5 in the container.
- `cost` (`total_cost_usd`, `total_duration_ms`, `total_api_duration_ms`, lines added
  and removed), `exceeds_200k_tokens`, `fast_mode`, `thinking.enabled`.
- `rate_limits` (`five_hour`, `seven_day`, each `used_percentage` and `resets_at`)
  only under a subscription login, from about three seconds after start.
- Conditionally, per the source: `session_name`, `prompt_cache`, `vim`, `agent`,
  `remote`, `pr`, `worktree`.

Nothing in it says whether the session is working, idle or waiting, and nothing
about compaction beyond the context counts falling to `null`.

When it runs **[source; the cadence confirmed in container and live]**: once when the
footer mounts; 300 ms after any change of token usage, permission mode, model, effort,
fast mode, thinking, vim mode or PR status, or after a new assistant message; on
`/clear`; at a rate-limit reset and a prompt-cache expiry; and, only when
`statusLine.refreshInterval` is set (seconds, at least 1), on that timer, idle
included. A new run aborts the one in flight. Without the interval an idle session
runs it not at all: once in 40 idle seconds, then once for each `/effort` and `/model`.
It is skipped until the workspace is trusted; exit 0 is required and stdout, trimmed,
is what is drawn. It runs as `/bin/sh -c <command>` in the project directory with
`CLAUDE_PROJECT_DIR`, `COLUMNS` and `LINES` added, the JSON plus a newline on stdin **[container]**. A managed policy may restrict the status line to its own.

How `--settings` meets a configured status line **[container; verified live]**: the
key is merged field by field. A user layer with `command`, `padding: 3` and
`refreshInterval: 2`, under a `--settings` layer naming only `type` and `command`: only
the flag's command ran, with the user's padding, eight times in about fifteen idle
seconds. Live, the owner's own `refreshInterval` drove the probe's command every
second. So a status line passed for one launch replaces the person's command for that
session and inherits whatever else they set.

Hooks **[verified live unless marked]**:

- Every hook gets `session_id`, `transcript_path`, `cwd`, `scratchpad_dir`,
  `prompt_id`, and on turn events `permission_mode`; `effort.level` when the model
  takes one **[source]** — absent on Haiku 4.5.
- `SessionStart` carries `source` and `model` (the id): `startup` at launch,
  `compact` after a compaction; `clear` and `resume` per the source.
- `UserPromptSubmit` fires for a message delivered through the inbox socket, and
  carries its text in `prompt`.
- `PreCompact` (`trigger` `manual` or `auto`, `custom_instructions`) and `PostCompact`
  (`trigger`, `compact_summary`). A manual `/compact`: `PreCompact`, about ten seconds,
  `SessionStart` with `source: compact`, `PostCompact` 8 ms later; `session_id` kept;
  no `UserPromptSubmit` or `Stop` around it. Automatic compaction runs inside a turn
  with `trigger: auto`, and a failed one runs `PreCompact` with no `PostCompact`
  **[source]**.
- `PreModelSwitch`/`PostModelSwitch`: `from_model`, `to_model`, `requested_model`,
  `source` (`command` for `/model`), `context_tokens`, `prompt_cache_warm`,
  `cache_ttl`, `estimated_cache_write_usd`, `pricing`. **[container]**
- `Notification`: `message`, `title`, `notification_type`; the binary names
  `permission_prompt`, `idle_prompt`, `agent_needs_input`, `agent_completed` and
  others **[source]**. No `idle_prompt` arrived in 80 idle seconds.
- `SessionEnd`: `reason`, `prompt_input_exit` for `/exit`. `Stop` also carries
  `background_tasks` and `session_crons` **[source]**.
- Conversation text travels in `prompt`, `last_assistant_message`,
  `compact_summary` and `custom_instructions`; a consumer that must not read
  transcripts drops them unread.

`/clear` gives the session a new `session_id`, which the next status-line run already
carries **[container]**; a compaction keeps it. Whether `Stop` fires when a person
interrupts a turn was not observed.

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
