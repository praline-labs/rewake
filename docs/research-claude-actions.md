# Research: acting on a Claude Code session from its plugin

Split from [research-claude-control.md](research-claude-control.md) by subject on
September 24, 2026: that document is about what reaches a running session from outside
and what rewake hears back; this one is about what a function-hooks plugin can do to the
session it runs in — compact it, abort its turn, poll a file, swallow a socket line —
with the forms and refusals of each. `rewake compact` and `rewake interrupt` rest on it
([remote-control.md](remote-control.md)). The Codex side of the same requests is in
[research-protocol.md](research-protocol.md#compaction-and-interrupt-on-request).

Tags: **[live]** — observed in a private HOME against a stand-in API; **[source]** — read
in the installed binary's bundled source. Facts age with harness versions: recheck
before touching the module.

## Compaction, abort, polling

Probed by review-claude on September 24, 2026, Claude Code 2.1.280, with a stand-in API
**[live]** and in the binary **[source]**; compaction was first seen live on September 23.

- **`$.session.compact()` or `$.session.compact({ instructions })`** **[source]**; any
  other form is refused with `takes { instructions } (a string) or nothing`. Idle, it
  compacted in 52 ms against the stand-in: the plugin event `session.compact` with
  trigger `plugin` and the instructions, then the hooks `PreCompact` (trigger `manual`),
  `SessionStart` (source `compact`) and `PostCompact`, and "Conversation compacted" on
  the screen **[live]**. The instructions reached the summary request as
  `Additional Instructions:` followed by the text, and rewake's telemetry counted the
  compaction (`completedCompactions` 1) **[live]**. The result carries `messages` — the
  summary's text — with `tokensBefore`, `tokensAfter` and `usage` **[source]**.
- **Mid-turn it is refused at once and atomically**: `rewake: $.session.compact: a turn
  is running (<turnId>); the conversation compacts between turns, so call it from
  turn.complete or later` — the host prefixes every refusal a plugin gets with the
  plugin's name — and the turn went on to end with `answer` **[live]**; the same from a
  clock callback and from `session.receive`. A short conversation is refused with
  `rewake: $.session.compact: Not enough messages to compact.` **[live, 2.1.280,
  September 24, 2026, review-claude]**. Other refusals **[source]**: `compaction is switched off in this
  session (DISABLE_COMPACT), for /compact and plugins alike`, and one for a call from a
  hook that holds the turn.
- **`$.turn.abort({ turnId })`** ends the running turn: `turn.complete` with reason
  `aborted` came 2 ms later **[live]**. Idle it is refused with `<plugin>: $.turn.abort:
  no turn is running (asked for …)` — `ctl: $.turn.abort: no turn is running (asked for
  none)` from a probe plugin named `ctl` on September 23 **[live]** — and a wrong id with
  `… is not the running turn (Y)` **[source]**. Unlike an Esc before the first output, the prompt stays in the
  conversation; the next request carried the aborted prompt and the next one in a single
  user message, with no marker of the abort, and the screen showed no "Interrupted"
  **[live]**.
- **Whether a turn runs** is not what `$.session.turns()` answers: it counts user
  messages **[source, live]**; a module tracks its turn from its own `turn.*` events.
- **`$.clock.every(ms, fn)`** takes a non-negative number of milliseconds (at least 1
  for `every`) and a function, and returns `{ cancel }`; `after` and `sleep` take the
  same number, and a callback that throws is logged as a warning **[source]**. A module
  polling a file every 250 ms from `session.start` took each request within one period,
  and both compaction and abort worked from the callback **[live]**.
- **`$.fs` takes positional arguments** — `read(path[, { as }])`, `write(path, text)`,
  `exists(path)`, `stat`, `list`, `ancestors` — and refuses `{ path }` with `takes a
  path` **[live]**. `read` returns the text; `write` makes the parent directories and
  writes in place, not by rename, up to 4 MiB; `exists` answers from a stat; there is no
  remove **[source]**. A `read` of a missing file is logged as an `[ERROR]` in the
  debug log each time — 220 lines in one probe of polling by `read` **[live]** — so a
  poll asks `exists` first. Two `write`s not awaited raced each other **[live]**.
- **Editing the module reloads it**: `session.start` runs again, `<plugin>: reloaded`
  on the screen **[live]**.
- **`session.receive` sees socket lines** of type `user` with `origin: { kind: "peer" }`,
  mid-turn too, as they arrive; returning `{ consumed: … }` swallows the line — no
  turn, no request, nothing on the screen, and the debug log says `consumed by a hook`
  **[source, live]**. No receipt is sent for a consumed line. Control frames never reach
  it: an unknown action gives only `Unhandled control action` **[source, live]**.
  rewake does not use this path: without the module the command would reach the model.

