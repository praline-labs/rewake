# A telemetry collector for Claude Code — September 23, 2026

Claude Code offers no channel a wrapper can listen on, so until this day every Claude
Code session read as `unknown` in `rewake list` and in the main-only header: model,
effort, context, compactions and activity all came from the Codex server and had no
counterpart here (HF-11, HF-22, entry 5 of the
[parity queue](../harness-features.md#open)). What the harness does offer was
established the same morning ([research.md](../research.md#telemetry-sources-the-status-line-and-hooks)):
it runs commands we name and hands each a JSON object — hooks at points of a session's
life, and the status-line command whenever what it shows changes. The status line
carries model, effort and context fill; the hooks carry activity and compactions.

**What was built.** Both, feeding one collector. Mechanism:
[claude-telemetry.md](../claude-telemetry.md).

- The wrapper binds a datagram socket beside the inbox socket and folds what arrives
  into the snapshot it publishes with its heartbeat, as the Codex side does
  ([session-state.md](../session-state.md#claude-code-source)). A collector that cannot
  start costs telemetry, never the launch.
- `rewake observe` runs as a background hook (`"async": true`) on SessionStart,
  UserPromptSubmit, PreCompact, PostCompact, Notification, Stop, StopFailure and
  SessionEnd: it decodes only the fields it names, sends one datagram without waiting
  and exits 0 whatever happened. Each event carries its process's start on the boot
  clock, so late background hooks are put in order.
- The status line becomes `rewake status-tap`, which sends what the payload says and
  then replaces itself with the person's own status-line command, found at each call
  from every settings layer the way Claude Code merges them. The person's display is
  what it was.
- A compaction counted from PostCompact hands the main the same notice the Codex side
  sends.

**Decisions.** Conversation text in a payload is never stored, logged or forwarded;
the decoders skip it unread. Hooks must not slow the agent: no waiting, no retry, no
lock. The person's status line must keep working whatever it is. The recorded owner
decisions of the day are in [claude-telemetry.md](../claude-telemetry.md).

**Tests.** Unit tests over the decoder, the fold, the collector and the tap; the
workflow case `claude-telemetry` runs a main and a worker on the Claude Code column and
reads the worker's telemetry from the main's `rewake list`, its header and the
compaction notice, with budgets on the hook and the tap and three mutant controls
([testing.md](../testing.md#claude-code-telemetry-budgets)).

**Observed live.** September 23, 2026, Claude Code 2.1.280, rewake built from
`d975dd7`: a main's `rewake list` showed model, effort, context, compactions and
activity for itself and its workers; a worker's context stayed unknown until its first
reply, as designed; the header on a worker's message read `write-claude: idle | context
22% used / 1000K | compactions 0`; and a worker's compaction reached the main as
`Primary compaction completed (observed count 1).` The evidence is in the rows HF-11 and
HF-22 of [harness-features.md](../harness-features.md).

**Open.** Activity stays `working` after a person interrupts a turn with Esc, until the
next prompt or Stop: no hook was seen to mark the interruption. The waiting state during
a turn is unknown. Both are recorded as limits in
[session-state.md](../session-state.md#claude-code-source).
