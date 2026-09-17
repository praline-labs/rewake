# Research: how to deliver a message to a live harness session

Gathered September 15-16, 2026. Versions: Claude Code 2.1.270, Codex CLI 0.154.0.
Facts are tagged by source: **[verified live]** — checked by running these versions,
**[source]** — read in the code, **[docs]** — official pages.
Facts age with harness versions: recheck before changing the adapter.

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

Checked with `codex sandbox -P :workspace -C <dir> -- <cmd>`, without calling the model:

| action from inside the sandbox | result |
|---|---|
| write to `/tmp/...` | allowed |
| `connect()` to a unix socket | `EPERM` |
| write to `~/.codex` | read-only file system |
| variable | `CODEX_SANDBOX_NETWORK_DISABLED=1` |

**[verified live]** The network seccomp filter cuts off any socket domain,
including AF_UNIX, when the network is disabled. In legacy workspace-write,
`sandbox_workspace_write.network_access = true` allows the network while
`~/.codex` stays read-only. An explicit `-P :workspace` ignores that legacy
setting and uses its profile's network policy. With the default restricted
network the agent cannot connect to the delivery socket or call the queue,
but it can write files to `/tmp`.

### Git metadata writes by role

**[source: snapshot `44b9011`, September 13, 2026; live checks: CLI 0.154.0,
September 16, 2026]** The source workspace has version `0.0.0` in its Cargo
manifest; that placeholder does not identify the installed release's commit.

- `.git`, `.agents` and `.codex` are protected by default:
  `codex-rs/protocol/src/permissions.rs:799–854`. The Linux runtime binds writable
  roots, then reapplies protected subpaths with `--ro-bind`:
  `codex-rs/linux-sandbox/src/bwrap.rs:594–627,1039–1088`.
- The legacy override
  `-c 'sandbox_workspace_write.writable_roots=["<cwd>/.git"]'` permits commits
  but **replaces the array**. A sandbox check with an existing configured root
  confirmed that root was lost. The tmp and network fields are unaffected.
  Rewake does not use this override for Git access.
- **`--add-dir <gitdir>` adds to the configured roots.** A live `codex exec`
  run committed successfully with this flag and `-s workspace-write`; its
  sandbox header included both the added `.git` and all previously configured
  roots. This is the flag rewake passes for main and write.
- The flag is shared with TUI and repeatable: `add_dir` is a `Vec<PathBuf>` in
  `codex-rs/utils/cli/src/shared_options.rs:74–76`. TUI forwards it as
  `additional_writable_roots` in `codex-rs/tui/src/startup_orchestration.rs:128–143`.
  `codex-rs/core/src/config/mod.rs:3454–3468` combines cwd, additional and
  configured roots, then deduplicates them. Both interactive and exec argument
  parsers accepted repeated identical flags with `--help`, without a model call.
- Explicit permission profiles ignore legacy roots, network and tmp settings:
  `codex-rs/core/src/config/mod.rs:3438–3468,3510–3547`. Rewake leaves the selected
  profile intact and adds runtime roots. A restrictive selected policy can still
  refuse writes; the adapter does not change it to workspace-write.
- `codex sandbox` does **not** forward root-level `--add-dir`: its dispatch and
  config overrides omit `additional_writable_roots`
  (`codex-rs/cli/src/main.rs:1698–1740`, `debug_sandbox.rs:629–633`). An EROFS from
  that command with `--add-dir` does not describe TUI or exec behavior.

The baseline and legacy comparison need no model call:

```bash
mkdir -p /tmp/git-permission-check/{repo,home,tmp}
git -C /tmp/git-permission-check/repo init -q
export CODEX_HOME=/tmp/git-permission-check/home
export TMPDIR=/tmp/git-permission-check/tmp
codex sandbox -P :workspace -C /tmp/git-permission-check/repo -- \
  git -c user.name=Test -c user.email=test@example.invalid \
  commit --allow-empty -m 'Check default metadata protection'
# exit 128: .git/index.lock: Read-only file system
cd /tmp/git-permission-check/repo
codex sandbox -c 'sandbox_mode="workspace-write"' \
  -c 'sandbox_workspace_write.writable_roots=["/tmp/git-permission-check/repo/.git"]' -- \
  git -c user.name=Test -c user.email=test@example.invalid \
  commit --allow-empty -m 'Check scoped metadata access'
# exit 0; this demonstrates the legacy override, not the rewake launch path
```

The positive legacy check uses shell cwd: sandbox's `-C` requires `-P`, which
selects a profile and ignores the legacy grant. Its legacy default is read-only,
so that check chooses workspace-write explicitly. A separate live exec turn
verified the actual additive launch path from the same temporary repository:

```bash
codex exec --add-dir /tmp/git-permission-check/repo/.git -s workspace-write \
  'Run git -c user.name=Test -c user.email=test@example.invalid commit --allow-empty -m "Check additive metadata access" and report its exit code.'
```

Unlike the sandbox probes, this invokes a model; the September 16 verification
was performed by the orchestrating session. Rewake's automated checks use
launch plans and temporary Git fixtures instead. The metadata resolver reads
`.git` and `commondir` without invoking Git; tests compare it with
`git rev-parse --path-format=absolute --git-dir --git-common-dir` for ordinary
repositories, worktrees and submodules. Both worktree metadata directories are
needed: Git writes per-worktree state and shared repository state.

### Managed worktrees and continuation permissions

**[source: snapshot `44b9011`; sandbox verification with CLI 0.154.0,
September 17, 2026; no model call]** Local `resume` and `fork` keep the extra
metadata root discovered from launch cwd. They may choose another conversation
cwd; the extra source root is still useful and does not replace the workspace.

Managed worktree allocation uses `$CODEX_HOME/worktrees/<four-character id>/<repo>`
by default, or `desktop.git-worktree-root` when configured
(`worktree/src/settings.rs:43–56`, `worktree/src/paths.rs:13–30`, relative to
`codex-rs`). `worktree/src/lib.rs:85–99` runs a detached `git worktree add`;
`:147–159` preserves the source cwd's relative subdirectory within the checkout.
The TUI retains additional writable roots, sets overrides.cwd to the new
checkout cwd, and rebuilds config (`tui/src/worktree_startup.rs:217–221,277–286`).
That cwd becomes a workspace root (`core/src/config/mod.rs:3454–3468`).

The checkout is writable as cwd, but the parent metadata grant alone is not
reliable. A sibling checkout succeeded with only the source `.git`; a checkout
in the deeper managed layout (`<home>/worktrees/abcd/<repo>`) failed to create
`<source>/.git/worktrees/<name>/index.lock`. Adding that exact private gitdir
alongside the common `.git` made the commit succeed. Granting only the private
gitdir failed to write objects. Both tmp write exclusions were enabled, so the
probe did not borrow broad access to `/tmp`.

The source explains the observed layout sensitivity: worktree pointer targets
are protected subpaths (`protocol/src/permissions.rs:2212–2231`), while Linux
sorts writable roots by depth and reapplies read-only subpaths after each bind
(`linux-sandbox/src/bwrap.rs:559–560,583–627`). A parent root alone does not
reliably override the later private-metadata carveout.

The sandbox subcommand ignores root-level `--add-dir`, as noted above. The
model-free probes therefore compared effective writable-root lists from an
already-created worktree cwd. Parent-only failed in the managed layout; this
form, with both roots, succeeded:

```bash
codex sandbox -c 'sandbox_mode="workspace-write"' \
  -c 'sandbox_workspace_write.exclude_slash_tmp=true' \
  -c 'sandbox_workspace_write.exclude_tmpdir_env_var=true' \
  -c 'sandbox_workspace_write.writable_roots=["<source-repo>/.git","<source-repo>/.git/worktrees/<name>"]' -- \
  sh -c 'printf "fixture\n" > probe.txt && git add probe.txt && git -c user.name=Test -c user.email=test@example.invalid commit -m "Check worktree access"'
```

Rewake grants metadata on local resume/fork but retains the new `--worktree`
skip: it cannot know that private gitdir before the harness allocates it. A
supported alternative is to create the worktree first, then launch from its
checkout; metadata discovery can then add both directories. Future automatic
support needs the allocated path before sandbox permissions are finalized, or
a reliable upstream carveout override. Rewake's session record retains the
wrapper's original cwd, not the dynamically chosen checkout path, and it does
not read saved transcripts to predict where a continuation will run.

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
are version-specific. The full rewake launch still needs owner acceptance after
its fake-server checks; raw protocol probes alone do not close the milestone.

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
`--` separates the bootstrap prompt from either option's values.
