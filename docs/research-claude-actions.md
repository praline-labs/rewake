# Research: acting on a Claude Code session from its plugin

Split from [research-claude-control.md](research-claude-control.md) by subject on
September 24, 2026: that document is about what reaches a running session from outside
and what rewake hears back; this one is about what a function-hooks plugin can do to the
session it runs in — compact it, abort its turn, poll a file, swallow a socket line,
fill the harness's task list — with the forms and refusals of each. `rewake compact` and `rewake interrupt` rest on it
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
- **Every compaction runs through the plugins' `session.compact` handlers**, with the
  compaction itself beneath `next(e)`: `PreCompact` runs inside it, and a handler's
  `await next(e)` returns once it is done, with `tokensBefore` and `tokensAfter`. The
  trigger is `plugin` for a module's call and `manual`, `auto` or `precompute` for the
  others, and a handler may change neither the trigger nor the `agentId` **[source,
  2.1.280, September 24, 2026, write-claude]**. A module's call while a typed `/compact`
  ran was refused with `a turn is in flight; the conversation compacts between turns`,
  and "Not enough messages" came after `PreCompact`, with no `PostCompact` after it
  **[live, 2.1.280, September 24, 2026, review-claude]**.
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
  same number, `sleep(ms[, { signal }])` returning a promise, and a callback that throws
  is logged as a warning **[source]**. `$.clock.now()` exists; what it answers was not
  read **[source]**. A module
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

## The task list

Probed by review-claude on September 24, 2026, Claude Code 2.1.280, in a private HOME
against a stand-in API **[live]** and in the binary **[source]**, for a todo list sent
with a task ([work-queue.md](work-queue.md#then-a-todo-list-sent-with-a-task)).

- **The tools** are `TaskCreate`, `TaskUpdate`, `TaskList` and `TaskGet`; `TodoWrite` is
  the older one, which takes their place only with `CLAUDE_CODE_ENABLE_TASKS=0`
  **[source]**.
- **They are off by model.** One gate lets them in for a background session, for a
  launch that names a task tool in `--tools` or `--allowedTools`, for a model on an
  older list or one whose name cannot be resolved, or with
  `CLAUDE_CODE_ENABLE_TODO_TOOLS` set to a true value **[source]**. For the current
  models none of that holds by default: a request's tool list carried no task tool for
  them, with or without rewake's flags, and carried all four for an older model under
  every one of rewake's flags **[live]**. rewake does not switch them off. A remote flag,
  a settings key, the permission mode and interactivity do not reach the gate
  **[source]**.
- **Switching them on for one launch**: `CLAUDE_CODE_ENABLE_TODO_TOOLS=1`, alone or with
  all of rewake's flags, gave the four tools on a current model; `--allowedTools
  TaskCreate` did too, but adds an allow rule besides **[live]**.
- **The plugin can create items**: `$.tool.call({ tool: "TaskCreate", subject,
  description, activeForm })` from `session.start` created tasks `1` and `2`, answered
  with the task's id and "Task #1 created successfully: …", and they showed in the
  terminal **[live]**. Without the tools switched on it is refused with `no tool named
  "TaskCreate" in this session` **[live]**. There is no `$.tasks` or `$.todos` of its
  own **[source]**. It was tried in `session.start` only, not at a message's delivery.
- **The model does not see items made that way**: none were in the request body. It
  learns of them from `TaskList` or from the task's text **[live]**.
- **Forms** **[source]**: `TaskCreate` takes `{ subject, description, activeForm?,
  metadata? }`, one task per call, ids `"1"`, `"2"` in order, status `pending`;
  `TaskUpdate` takes `{ taskId, status?, subject?, description?, activeForm?, … }` with
  status `pending`, `in_progress`, `completed` or `deleted`; `TaskList` takes `{}`,
  `TaskGet` `{ taskId }`.
- **Storage**: a file per task under `<config>/tasks/<listId>/`, with a lock; the list id
  comes from `CLAUDE_CODE_TASK_LIST_ID`, else the team, else the session id, which it was
  live **[source, live]**. Whether the list survives `--resume` — rewake launches with it
  — was not checked.
- **In the terminal**: the list is drawn under the transcript as "N tasks (x done, y in
  progress, z open)"; after the model's own task call it shows by itself, while items
  the plugin made need `ctrl+t`, and the status line hints at it **[live]**.
