# Claude Code telemetry

How a Claude Code session's model, effort, context, compactions and activity reach its
wrapper, and from there `rewake list`, the main-only header and the compaction notices.
The readers and what each field means are in
[session-state.md](session-state.md#claude-code-source); the facts about the harness this
rests on are in [research.md](research.md#telemetry-sources-the-status-line-and-hooks) and
[research-launch.md](research-launch.md#hook-options-and-settings-order-that-rewakes-launch-layer-relies-on).

Claude Code offers no channel to listen on, but it runs commands we name and hands each
a JSON object on stdin: hooks at points of a session's life, and the status-line command
whenever model, effort, context or the conversation changes
([research.md](research.md#telemetry-sources-the-status-line-and-hooks)). The launch
layer uses both; the wrapper listens.

- **The collector.** Before the harness starts, the wrapper binds a datagram socket
  `sock/<name>.<epoch>.obs` beside the inbox socket (digest fallback past 103 bytes) and
  folds what arrives into the snapshot it publishes with its heartbeat
  ([session-state.md](session-state.md#claude-code-source)). A collector that cannot
  start costs telemetry, never the launch.
- **The hooks.** `rewake observe <socket>` on SessionStart, UserPromptSubmit,
  PreCompact, PostCompact, Notification, Stop, StopFailure and SessionEnd, every role,
  each with `"async": true`: the harness runs it in the background and does not wait
  (supported on 2.1.280, read in the binary). The command decodes only the named fields
  it needs, sends one datagram without waiting — a missing or busy socket fails on the
  spot — and exits 0 whatever happened. Each event carries its process's start on the
  boot clock (`CLOCK_BOOTTIME`, read while the package initializes, before stdin), which
  the collector compares to put late background hooks in order. On UserPromptSubmit the
  hook also records that start in `<socket>.turn/`, without a lock, for binding a
  `rewake pending` mark to its turn
  ([turn-outcomes.md](turn-outcomes.md#interim-turn-ends-rewake-pending)). The
  end-of-turn hooks stay in the foreground: a
  turn's end has to be recorded before the session goes idle.
- **Seen live.** September 23, 2026, Claude Code 2.1.280, rewake built from `d975dd7`: a
  main's `rewake list` showed model, effort, context, compactions and activity for
  itself and its workers, a worker's context stayed unknown until its first reply, and a
  worker's compaction reached the main as `Primary compaction completed (observed count
  1).` The evidence is in [harness-features.md](harness-features.md) under HF-11 and
  HF-22.
- **The conversation.** Every event's `session_id` is the conversation the session is
  in; the collector keeps the latest by the boot clock, and the wrapper pins each owed
  delivery to it, so a report from another conversation carries `threadChanged`
  ([delivery-adapters.md](delivery-adapters.md#claude-code-adapter)).
- **The plugin.** No hook marks an interruption — Esc and Ctrl+C run no Stop,
  StopFailure or Notification ([research-claude-control.md](research-claude-control.md#interrupting-a-turn-and-changing-the-conversation)).
  Since September 23, 2026 rewake's own function-hooks plugin reports turn starts, turn
  ends with their reason and the context fill through the same `rewake observe`, so an
  interrupted session reads `idle` and its task gets `stopped`
  ([claude-plugin.md](claude-plugin.md)). Where the plugin does not load, a session a
  person interrupts stays `working` until the next event that ends a turn or starts one
  afresh, as before, and its telemetry says interruptions are `unobserved`.
- **The window.** A person can cap the context below the model's window —
  `CLAUDE_CODE_AUTO_COMPACT_WINDOW`, the `--autocompact` flag or the `autoCompactWindow`
  settings key — and the harness then compacts against the cap, while the status line
  and `session.measure` still report the model's window and the percent of it
  ([research.md](research.md#telemetry-sources-the-status-line-and-hooks)). So the plugin
  reads the variable through `$.env.get` and the key through `$.settings.read()` at the
  session's start and at every measure of the context, and sends the two raw values with
  `plugin.ready` and `session.measure`; the wrapper takes the flag from the launch's own
  arguments, the last one given, since a plugin cannot see it. The collector reads each
  value by the harness's rules and in its order — the variable, then the flag, whose
  `auto` sets the key aside, then the key — and where the result is below the model's
  window the listing shows it as the window, with the percent computed against it. The
  tokens stay the status line's. The newest report decides, so a value edited
  mid-session follows the harness from the next measure. The compaction point — 89% of
  a 300K window — is not shown anywhere: it depends on the model's output limit, which
  rewake does not know, and computing it would copy the harness's arithmetic. Without
  any of the three the numbers are the status line's, as before; where the plugin did
  not load, the listing shows the model's window whatever the person set, the flag
  included, because the variable and the key are read only through the plugin.
  Owner decision, September 24, 2026: the listing shows the person's limit as the
  window, read from the harness's own sources rather than assumed; the owner does not
  change the value mid-conversation, and the reading at every measure follows an edit
  anyway. Leaving the
  compaction point out is a design choice made for the reason above, not part of that
  decision.
- **The tap.** The status line becomes `rewake status-tap <socket> <sources> [caller]`.
  It reads the payload, sends what the status line says, finds the person's command
  (below), and then replaces itself with
  `/bin/sh -c <command>` with the same payload on stdin — exactly how the harness runs a
  status line (seen in a container on 2.1.280: `/bin/sh -c`, project directory, the
  harness's environment, JSON and a newline on stdin). So the person's command runs in
  the same process, directory and environment, under the harness's own timeout, with its
  output and exit code untouched. A payload too large for a pipe falls back to running
  the command as a child, with termination signals passed on. No configured status line:
  the tap prints nothing. A person without a status line of their own therefore now has
  one — rewake's tap, printing nothing; whether an empty status line looks the same on
  screen as none was not verified (September 23, 2026). Parsing that fails, a changed format and a wrapper that is gone
  all end the same way — the command runs as if rewake were not there.
- **Which command the tap runs** is found each time it runs, from its own environment
  and directory — the harness's — the way Claude Code merges its layers, field by field,
  lowest first: user, project, local, then a caller's `--settings`; the managed policy
  sits above all of them. At call time rather than at launch because a person may set
  `CLAUDE_CONFIG_DIR` in a script of their own right before starting Claude Code, where
  the rewake wrapper never sees it; the tap, started by the harness, does. Only what the
  tap cannot learn is fixed at launch and passed in its arguments: which of the three
  files this launch reads, and the status line of a caller's `--settings`. Reading three
  small files costs nothing the measurement shows, so nothing is cached. That order is Claude Code's
  own list in 2.1.280 (`userSettings, projectSettings, localSettings, flagSettings,
  policySettings`, read in the binary). The user layer is `$CLAUDE_CONFIG_DIR` or
  `~/.claude`, `settings.json` (or `cowork_settings.json` under
  `CLAUDE_CODE_USE_COWORK_PLUGINS`); project and local are `.claude/settings.json` and
  `.claude/settings.local.json` under `CLAUDE_PROJECT_DIR`, or the tap's directory. `--restricted` drops all three
  files and `--setting-sources` names the ones read. A policy status line wins over the
  tap anyway, so the person's display stays the policy's; rewake then notes on stderr
  that model, effort and context stay unknown. A policy delivered from a server cannot be
  read at launch and behaves the same way. Only the `statusLine` key of these files is
  read; nothing is written.
- **Owner decisions, September 23, 2026.** The person's status line must keep working
  whatever it is, including when they run Claude Code through a wrapper of their own —
  hence resolution from every layer rather than the settings files alone, the merge into
  a caller's `--settings`, and resolution in the tap's own environment at each call (the
  owner runs workers with `CLAUDE_CONFIG_DIR` pointing at a separate directory). Conversation fields in a payload (the prompt, the last
  reply, a compaction summary, custom instructions and anything like them) are never
  stored, logged, forwarded or used: the decoders name the fields they want and skip the
  rest, never unmarshalling into a generic map. Hooks must not slow the agent: fire and
  forget, no waiting, no retry, no lock. Measured figures are in
  [testing-cases.md](testing-cases.md#claude-code-telemetry-budgets).
