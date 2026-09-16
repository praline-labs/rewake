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

### Thread id of a live TUI

- There is no flag to assign an id at startup. **[source]**
- From the moment it starts, the TUI process holds
  `$CODEX_HOME/thread-writer-locks/<thread-id>.lock` open; the rollout file may not
  exist yet. Visible via `/proc/<pid>/fd`. **[verified live]**
- After `/new`, the process holds **both** lock files open, the old one and the
  new one; the current one is the one whose lock file has the later mtime.
  **[verified live]**
- rollout: `$CODEX_HOME/sessions/YYYY/MM/DD/rollout-<timestamp>-<thread-id>.jsonl`,
  the id is in the filename. **[source, verified live]**
- The `SessionStart` hook receives `session_id` on stdin. Codex won't run an
  unverified hook without user approval; getting it to run live via `-c` did not
  work. **[source, docs]**
- The `CODEX_SESSION_ID`/`CODEX_THREAD_ID` variables are only set for commands
  that the agent itself executes. **[source]**

### End of a turn

- `/new` changes the active thread without restarting the wrapper or changing
  its epoch. The old and new thread locks may remain open; the existing tracker
  chooses the newest lock in the nearest process generation. A live owner run
  on September 16, 2026 showed that an old task's wait can survive `/new` and
  receive the new thread's first final reply. The hook now marks known delivery
  versus current-thread mismatches; it never clears waits or resends work.
  **[source: snapshot 44b9011; owner-observed conversation switch; fake-harness
  regression verifies the warning without a model call]**

- `notify = ["prog", ...]` runs the program after every turn with a JSON
  argument appended last: `type: "agent-turn-complete"`, `thread-id`,
  `turn-id`, `cwd`, `client`, `input-messages`, `last-assistant-message`. It can
  be set with `-c` for one launch, needs no feature flag and no trust, and runs
  outside the sandbox with the session's environment. The TUI fires it too. It
  replaces the user's own `notify`. **[source; verified live 0.154.0]**
- Lifecycle hooks (`hooks.Stop` and others, `features.hooks` on by default) from
  the user, the project or `-c` are registered only when their
  `hooks.state."<key>".trusted_hash` matches or the launch passes
  `--dangerously-bypass-hook-trust`; otherwise they are skipped silently. That
  is why a `SessionStart` hook passed with `-c` never ran. `SessionStart` also
  runs at the start of the first turn, not at launch. **[source]**
- A queued message arrives as an ordinary user message: Codex has no drawing of
  its own for a notice. **[verified live]**

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

### app-server (fallback path)

`codex app-server --listen unix://<path>` and `codex --remote unix://<path>`: the
TUI acts as a client of its own server. Methods: `thread/start`, `thread/resume`,
`thread/name/set`, `thread/queue/add`, `turn/start` (wakes an idle thread or
steps into a running turn without the 10-second delay), `turn/steer`,
`turn/interrupt`. The unix socket's authorization is file permissions only. A
standalone app-server without its own `CODEX_HOME` opens the same state files as
every other process. **[source]** The official codex plugin for Claude Code keeps
a headless app-server behind a broker on a unix socket: one owner of the
streaming turn, everyone else gets `-32001 busy`. **[source: plugin]** Not
verified live whether a TUI with `--remote` sees turns started by another
client.

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

## Prior art

- Existing tools deliver messages three ways: tmux `send-keys`
  (cli-agent-orchestrator, cyclops, agent-mux), a custom broker (Agent Intercom,
  claw-orchestrator, agent-bridge), or MCP as a mailbox (cross-agent-teams-mcp,
  mailbox-mcp). None of them use Claude Code's native socket. **[web search]**
- agent-deck keeps the harness in tmux and delivers via `send-keys`/`paste-buffer`
  with screen scraping; nearly its entire defect history traces back to that
  (swallowed Enter, merging with a draft, brittle prompt regexes). Worth keeping:
  its own session id in the environment, Claude Code with a pre-assigned
  `--session-id`, deriving the Codex thread id from the process's open files, and
  a machine-readable delivery status. **[source]**
- grok's live-session registry: one JSON file under an exclusive flock, written
  via tmp-and-rename, idempotent by id. Weak points to avoid: liveness checked by
  pid alone with no start time, and a corrupt file wipes out other entries.
  **[source]**
- deepseek-harness's delivery semantics: wake an idle recipient, and for a busy
  one, queue instead of interrupting. Its idle signal, without screen scraping, on
  Linux is `/proc/<pid>/task/<tid>/syscall`. **[source]**
