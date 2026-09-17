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
