# Prior-art notes

[Back to research](research.md).

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
- herdr (checked September 30, 2026 at v0.9.3 against the installed 0.8.0) is a
  terminal multiplexer with a background server that owns the agents' terminals;
  one agent drives another with `herdr agent prompt <name> "text" [--wait]`, which
  types the text and Enter into the target's PTY (`src/app/api/agents.rs:111-215`),
  and reads the result back from the screen with `agent read`. States come from
  screen manifests plus hooks it installs into the agents' own settings
  (`herdr integration install`). There is no mail: no kinds, no report returned at
  a turn's end, no pending, question, withdraw or edit, no roles or rooms, no
  telemetry of model or context, no grants, and success means the bytes were
  written, not that a turn began. A prompt carries no sender (issue #2180 closed
  unbuilt), and typing into the PTY mixes with the person's draft (#4001, #4072,
  kept as known limitations); a structured message stream (#2103) was closed
  unbuilt. The agent primitives were already in 0.8.0; 0.9.x added several machines
  over SSH (`--machine`), session restore and lossless events (`completion_seq`).
  It covers only the transport part of rewake's niche and by the opposite means;
  the two can sit together — herdr's panes, restore and machines around rewake's
  messages — once its installed hooks are checked not to clash with rewake's launch
  flags. Worth borrowing: not delivering to an agent held on an approval or question
  (`agent_blocked`), and a count of completed turns. **[source, web]**
