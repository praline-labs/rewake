# The mail tool on a running harness

What each harness does with rewake's mail tool — the `rewake` MCP server the launch
injects ([mail-bridge-launch.md](mail-bridge-launch.md)) — as the live checks of
October 4, 2026 found it ([mail-bridge-live.md](mail-bridge-live.md#the-run-of-october-4-2026)).
Split from [research.md](research.md) and [research-codex.md](research-codex.md) by
subject: everything about the tool's transport on both harnesses is here. Versions:
codex-cli 0.159.0 and Claude Code 2.1.284, the owner's logins unless a scratch home is
named. Each fact says how it was obtained: *live*, *source* (the Codex reference tree at
`rust-v0.159.0`), or *bundled source* (text in the Claude Code 2.1.284 binary).

## Codex 0.159.0

### Trust and the launch

- **A `-c` key is split on every dot, quotes included** (*source*,
  `config/src/overrides.rs` 22: `parse_overrides` keeps the quotes as part of the key).
  So `-c 'projects."<path>".trust_level="trusted"'` never names the project. The form
  that does puts the path in the value: `-c 'projects={"<absolute path>"={trust_level="trusted"}}'`.
  *Live* (scratch `CODEX_HOME`, app-server): with the value form a `thread/start` naming
  the folder under `workspace-write` left `config.toml` byte-identical; with the dotted
  form, and with no `-c`, the same request wrote the folder's trust there. In the owner's
  login, launches with the value form showed no folder-trust prompt.
- **The trust key** the app-server writes is the main repository root, canonicalized,
  else the cwd; the lookup tries the cwd, then the repository root (*source*). Trust is
  written only on `thread/start`.
- **The app-server's `initialize` answer carries its version** in `userAgent`:
  `<clientInfo.name>/<version> (<os>; <arch>) <terminal> (<client name>; <client
  version>)` — `probe/0.159.0 (Ubuntu 24.4.0; x86_64) xterm-256color (probe; 0)` for a
  client named `probe` (*live*, scratch `CODEX_HOME`, no thread; *source*
  `login/src/auth/default_client.rs` 152: the version is the build's `CARGO_PKG_VERSION`).
- **The standalone install names its version in the path**:
  `…/packages/standalone/releases/0.159.0-<target>/bin/codex` (*live*).
- **A fresh home's first terminal start writes `[tui] screen_reader_detection_done`**
  (*live*, scratch home).
- **The hook-review screen** for a project's hooks: `-c bypass_hook_trust=true` does not
  skip it; `--dangerously-bypass-hook-trust` does on a fresh launch but not with
  `resume <id>`, where "Continue without trusting" writes nothing (*live*; *source*
  `startup_hooks_review.rs` 140).
- **Approval policies**: `-a` takes only `on-request` and `never`; `-c
  approval_policy="untrusted"` makes the app-server exit 1. Our tool ran with no prompt
  under `on-request`, `never` and `-c approval_policy="on-failure"` (*live*). No denial of
  our tool is reachable for a person except through a layer naming `rewake`, which the
  name check refuses, or managed requirements.

### What starts a server — G1, G2

- `initialize`, `config/read` and `configRequirements/read` start no MCP server;
  `mcpServerStatus/list` starts every configured one (*live*, sentinel servers in a
  scratch home; the source agrees). **G1 closed for 0.159.0.**
- No call lists a thread request's registrations without starting them (*source*).
  **G2 stays open.**

### Threads and cwd — G3, G9

- **A resume or fork naming no cwd runs in the thread's recorded cwd** (*live*): a thread
  recorded in folder A, resumed from folder B, ran `pwd` in A, while the gateway checked
  the server's cwd. G3 is answered; the gateway has to use the recorded cwd before it
  can be closed.
- **A thread request's `config` reaches the `sessionFlags` layer unfiltered**, and the
  experimental `selectedCapabilityRoots` adds plugin registrations that step 0 does not
  check (*source*). G9 is not closable as built.

### Results and the output limit — L5

- **A tool result** reaches the model as a `function_call_output` of two items: `Wall
  time: <s> seconds\nOutput:`, then the text (*live*). A 6313-byte letter with multibyte
  characters, read in four parts of 2048 bytes, arrived in each part byte for byte equal
  to the letter's slice; each result was about 2.3 KiB.
- **`tool_output_token_limit`** cuts the text in the middle, marked `…<n> tokens
  truncated…`. Ours runs on the direct path (`omit_tools_from = ["code_mode"]`); there
  861 keeps a 4096-byte ASCII result whole and 860 cuts it (*live*, `codex exec`, a
  control server with the same `omit_tools_from`). That is the source's arithmetic: the
  header's 9 tokens plus `ceil(bytes/4)` must fit `ceil(1.2 × N)`
  (`utils/output-truncation`, `utils/string/src/truncate.rs` 71); for a 2332-byte read
  result the smallest is 493 (*source* only). The model used truncates in Tokens mode;
  Bytes mode gives the same bounds with `…<n> chars truncated…` (*source*). Through code
  mode a call is a `custom_tool_call_output` ("Script completed") and 861 did not cut it.

### Calls and agents — L7, L8

- **Calls of one MCP server run serially**: `supports_parallel_tool_calls` defaults to
  false (*source*, `config/src/mcp_types.rs` 592) and rewake does not set it; asked for a
  parallel batch, the model issued the calls seconds apart (*live*). Twenty sequential
  calls in one turn each got a ticket and answered.
- **Sub-agents are on by default** (`multi_agent`, stable, `features/src/lib.rs` 1327).
  `spawn_agent` runs a thread of its own whose `rewake` server is a second instance of the
  run's (*live*: the channel's generation 2, live `[1, 2]`). Its tool call is not seen by
  the gateway and is refused after 2 s as unreported — which the channel took for "calls
  not observed" and told the parent "rewake tool failed" (an open finding). The
  sub-agent's shell resolved `rewake` by a `PATH` that was not the parent's.
- **Every MCP request names its calling thread**: `_meta.threadId` and `_meta.sessionId`
  are set on each tool call (*source*, `core/src/mcp_tool_call.rs` 1410–1428), so a
  sub-agent's call names the sub-agent's thread.
- **A created thread's events reach every initialized connection**: on a new thread,
  sub-agents' included, the app-server attaches each initialized connection as a
  listener, best effort (*source*, `app-server/src/lib.rs` 1274–1292,
  `thread_processor.rs` 3608). rewake's gateway passes on tool items of the primary
  thread only, which is why it never saw the sub-agent's call.

### Faults and turn ends — L6, L10

- A killed server stays down for the run: later calls answer `tool call failed for
  rewake/rewake … Transport closed` (*live*, as probe 1 saw).
- A tool `pending` and a normal end give the sender kind `pending`; an Esc right after it
  gives `stopped` (*live*).

## Claude Code 2.1.284

### Launch and configuration

- **`CLAUDE_CONFIG_DIR`** moves the whole configuration: with it set, settings and
  `.claude.json` are that directory's, and a harness started from such a shell inherits
  it — the configuration a check must leave unchanged is that one (*live*).
- **MCP servers wait for the startup dialogs**: in a folder never trusted none starts
  until the dialog is answered, so the hello timer, counted from the harness's start,
  passed with the dialog open and sent the session to the shell (*live*, twice; an open
  finding). The order, timed (*live*, scratch `CLAUDE_CONFIG_DIR` with no login, a
  `--mcp-config` server and a `--settings` SessionStart hook that log their start): with
  the trust dialog open 20 s nothing started; answered, a second dialog came — external
  imports of a parent `CLAUDE.md` — and still nothing; once it was answered the server
  started and SessionStart ran 30 ms later. In the folder trusted, no dialog: the server
  1.0 s after launch, SessionStart 40 ms after it. So SessionStart marks the servers'
  start, dialogs or none.
- **Trust is recorded for the enclosing repository's root**, not the folder: answering
  the dialog in a folder inside a git repository set `hasTrustDialogAccepted` on the
  repository root's project entry (*live*, scratch configuration).
- **The native installer names its version in the path**: `claude` resolves to
  `…/share/claude/versions/2.1.284` (*live*).
- **`--strict-mcp-config` on a cold resume** keeps only the `--mcp-config` servers (*live*).
- **A parent's `.mcp.json`**, at any depth and across a git root, is loaded and shown by
  `mcp get` (*live*, scratch home). **G5 closed for 2.1.284.**
- **A plugin's server is keyed `plugin:<plugin>:<server>`**: a plugin named `rewake` with
  a server `rewake` gave `mcp__plugin_rewake_rewake__*` (*live*; *bundled source*).
  **G7 closed for 2.1.284.**
- **`managed-mcp.json`** takes exclusive control: with `/etc/claude-code/managed-mcp.json`
  present, `-p … --mcp-config <file>` exits 1 with "You cannot dynamically configure MCP
  servers when an enterprise MCP config is present"; `mcp get` shows the managed servers
  as "Enterprise config (managed by your organization)" and no other (*live*, disposable
  container, no network). A launch carrying our server does not start at all. The
  directory is `/etc/claude-code` on Linux, `/Library/Application Support/ClaudeCode` on
  macOS, `C:\Program Files\ClaudeCode` on Windows, with no override (*bundled source*).
  A file present but invalid or unreadable keeps the control as well: "only your
  organization's managed MCP servers can load until it is fixed or removed" (*bundled
  source*, `unusable-managed-mcp`). Under WSL a Windows policy chain can stop the harness
  reading `/etc/claude-code` (*bundled source*); not run.

### Permissions — L4

- A `permissions.deny` naming the tool removes it from the model's tools; no hook fires.
- `permissions.ask` under `-p`: PreToolUse, then PermissionRequest, then the result
  "Claude requested permissions to use mcp__rewake__rewake, but you haven't granted it
  yet."; interactive, answered No: PreToolUse, PermissionRequest, then the turn is
  interrupted. No PermissionDenied or PostToolUseFailure in either (*live*). No hook event
  tells a denial of our tool apart.

### Limits — L5, G8

- **`MAX_MCP_OUTPUT_TOKENS`** from the caller's `--settings` `env` applies. An estimate of
  `round(length / 4)` UTF-16 units at or under half the limit passes uncounted; above it
  an API count decides (*bundled source*). Over the limit the whole result is **replaced**,
  not truncated: "Error: result (4,096 characters across 1 line) exceeds maximum allowed
  tokens. Output has been saved to <file>" with advice to read the file (*live*). 2048
  kept a dense 4096-character result whole; 2047 replaced it; 2047 kept 4096 × `x`
  whole. So 2048 is the smallest limit that keeps every 4096-character result whole.
- **A server's own `timeout`** in its entry binds over `MCP_TOOL_TIMEOUT`: with
  `MCP_TOOL_TIMEOUT=7000` in the caller's settings, an entry with `"timeout": 12000`
  timed out after 12 s and one without after 7 s (*live*). The binary's text: "Overrides
  the MCP_TOOL_TIMEOUT environment variable for this server. Hard wall-clock limit per
  call; progress notifications do not extend it. Values below 1000ms are ignored".
  rewake's entry carries 30000; a stalled call of ours ended with "MCP server "rewake"
  tool "rewake" sent no response or progress for 30s; aborting." A separate idle bound,
  `CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT` ("0 disables"), did not shorten a call whose entry
  had a timeout (*live*, 4000 against 12000). Not run: the project, user and managed
  layers, and a change during a session.

### Calls and agents — L7, L8, L9

- **Parallel calls**: four responses of five `tool_use` blocks each, twenty tickets,
  twenty answers (*live*).
- **A nested agent's call** carries `agent_id` and `agent_type` (and `effort`) in its hook
  input, with the parent's `session_id`, `prompt_id` and `transcript_path`; a top-level
  call carries no agent field (*live*, one general-purpose subagent under `-p`). Through
  rewake the subagent's call was refused as nested.
- **`/clear` and `/resume`** change the conversation and keep the server process; calls
  work after each (*live*).

### Faults and turn ends — L6, L10, L11

- **A killed server is not started again**: the next call answered "Error: No such tool
  available: mcp__rewake__rewake. Its MCP server 'rewake' has disconnected. Continue
  without this tool; it becomes callable again only if the server reconnects.", no server
  started for 75 s, and `/mcp` showed it failed until the person chose Reconnect (*live*).
  Probe 1's "restarted on the next call" does not hold for 2.1.284.
- A tool `pending` and a normal end give `pending`; an Esc right after it gives `stopped`.
- A harness timeout with the child alive: the same words again in the same turn answered
  "these words ran earlier (receipt …); nothing was done again", one effect, the channel
  unchanged (*live*).
- SIGTERM to the wrapper: the record gained only `frozen: true`; nothing written after it.
