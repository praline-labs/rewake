# Research: what a running Codex session shows

Split out of [research.md](research.md) on September 23, 2026, when that file passed the
project's 400-line limit again. The division is by subject: the facts here are obtained
the same way as there — by watching a session behave — and are about Codex CLI, while
research.md keeps Claude Code. What the generated protocol schema and the reference tree
state is in [research-protocol.md](research-protocol.md); the Codex sandbox is in
[research-permissions.md](research-permissions.md).

Tags: **[verified live]** — live on the named versions; **[source]** — read in the
source; **[docs]** — official pages. Facts here age with harness versions: recheck
before touching an adapter.

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

**[owner live probes, 0.154.0]** Same-turn steering and fresh `/new` delivery both
reached the TUI and the model. (The probe transcripts lived in a temporary directory
that is long gone; the observation is what remains.)
A closed old thread can absorb input without a visible answer, and resuming an
empty thread did not reliably reply. Track closure; never deliver to a cached
id after losing evidence that it remains current.

**[source and no-model probes]** The research report behind this — kept in a
temporary directory that is long gone — recorded RPC initialization, failed and
interrupted outcomes, owned locks and the remote configuration boundary. TUI notify is not forwarded; developer instructions are
conditional on a feature there, so launch configuration reaches the server too.
TUI --add-dir is carried as runtimeWorkspaceRoots; -C and positional input are
forwarded. REWAKE_* survives default shell environment filtering.

The owned server uses CODEX_INTERNAL_APP_SERVER_REMOTE_CONTROL_DISABLED=1
(transport/remote_control/mod.rs:87) to keep this local launch off the persisted
remote-control path. This internal marker and the remote configuration rules
are version-specific. The later [installed native-mailbox acceptance](native-mailbox-acceptance.md)
closes the supported launch/delivery path; raw protocol probes alone did not do so.
