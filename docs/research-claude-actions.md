# Research: acting on a Claude Code session from its plugin and its hooks

Split from [research-claude-control.md](research-claude-control.md) by subject on
September 24, 2026: that document is about what reaches a running session from outside
and what rewake hears back; this one is about what a function-hooks plugin can do to the
session it runs in — compact it, abort its turn, poll a file, swallow a socket line,
fill the harness's task list — with the forms and refusals of each; and what a Stop hook
in rewake's settings layer can do to a turn's end, and a permission hook to its working
directories. `rewake compact` and `rewake interrupt` rest on it
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
  hook that holds the turn. Two more stand beside the in-flight one in the binary's
  string table: `an external turn is driving the conversation; it compacts between
  turns` and `a thin client's conversation lives on the remote session; compact it
  there`; next to them `the compaction produced no summary` and `a newer turn began
  before the compaction could be applied`, which by their words come after the summary
  was asked for **[strings of the 2.1.280 binary, September 24, 2026, write-claude;
  the code around them is compiled and was not read]**.
- **A compaction runs through the plugins' `session.compact` handlers**, with the
  compaction itself beneath `next(e)`: `PreCompact` runs inside it, and a handler's
  `await next(e)` returns once it is done, with `tokensBefore` and `tokensAfter`. The
  trigger is `plugin` for a module's call and `manual`, `auto` or `precompute` for the
  others, and a handler may change neither the trigger nor the `agentId` **[source,
  2.1.280, September 24, 2026, write-claude]**. **The calling plugin's own handlers are
  skipped** for the compaction its `$.session.compact` raised: the debug log says
  `hooks module rewake@inline session.compact skipped: re-entry (the plugin's own code
  raised it; origin rewake)` **[live + source, 2.1.280, September 24, 2026,
  review-claude]**. A module sees only the compactions it did not ask for there. A module's call while a typed `/compact`
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
with a task, a feature the owner later dropped
([work-queue.md](work-queue.md#dropped-a-todo-list-sent-with-a-task)).

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

## Holding a turn's end from the Stop hook

Probed on September 26, 2026, Claude Code 2.1.280, in a private HOME against a stand-in
API **[live]** and in the binary **[source]**, for a Stop-hook confirmation after an
interim turn end ([the proposal](roadmap/2026-09-26-pending-text.md#what-was-not-done)).
The hook sat in a layer given with `--settings`, as rewake's own hooks do.

- **A block holds the turn.** A Stop hook that printed `{"decision":"block","reason":…}`
  kept the turn going: the model was called again at once, with the reason in the new
  user message as `Stop hook blocking error from command: "<command>": <reason>`, and
  the screen showed `Stop hook error: <reason>` **[live]**.
- **The second call knows it is one.** The first Stop payload carried `stop_hook_active:
  false` and the whole reply in `last_assistant_message`; the Stop after the
  continuation carried `stop_hook_active: true`, and `last_assistant_message` held only
  the continuation's text, not the reply before the block **[live]**.
- **Eight blocks in a row, then the harness ends the turn.** The ninth consecutive block
  was overridden with `A hook blocked the turn from ending 9 consecutive times —
  overriding and ending turn`, and the turn ended **[live]**.
  `CLAUDE_CODE_STOP_HOOK_BLOCK_CAP` sets the limit, default 8; a value of 0 or less
  turns it off **[source]**.
- **An Esc during the continuation fires neither Stop nor StopFailure** **[live]**: the
  stand-in answered slowly, the turn showed "Interrupted", and the hook log of that run
  stayed empty; that empty log was not kept apart from the cap run's.
  StopFailure runs only for a turn that ended on an API error **[source]**; an Esc is
  heard by the function-hooks plugin alone
  ([research-claude-control.md](research-claude-control.md#what-a-function-hooks-plugin-hears)).

## A directory given to a running session

Probed on September 27, 2026, Claude Code 2.1.280, in a private HOME against a stand-in
API **[live]**, for a directory granted with a task ([grants.md](grants.md#claude-code)).
The session ran as `claude --settings <layer> --permission-mode acceptEdits`, with a
PreToolUse and a PermissionRequest hook in that layer. Evidence:
`~/.cache/rewake/evidence/2026-09-27/claude-grant-dir-probe/`.

- **A write outside the working directories** raises PreToolUse, then PermissionRequest
  with `permission_suggestions` `[{"type":"addDirectories","directories":[<parent>],"destination":"session"}]`.
  A hook answering `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow","updatedPermissions":[{"type":"addDirectories","directories":[<dir>],"destination":"session"}]}}}`
  lets the call run; the terminal shows "Allowed by PermissionRequest hook", and the next
  model request carries "Additional working directories added: - <dir>".
- **After that the directory is a working one.** A Write and a Bash `touch` inside it ran
  with no prompt and no PermissionRequest; the same `touch` in another outside directory
  prompted. Nothing was written to disk: no project `.claude/`, user settings unchanged.
- **`removeDirectories`** in the same answer, on any request, takes a directory out; the
  model is told "Additional working directories removed", and the next write there
  prompts. It removed a directory given at launch with `--add-dir` as well.
- **PreToolUse cannot change permissions**, but `permissionDecision: "ask"` on a call that
  would run anyway — a Write inside the working directory — makes the harness raise a
  PermissionRequest, whose answer then carries the removal.
- **A cold resume** (`/exit`, then `--resume <id>`) does not restore a directory added for
  the session: the next Write prompted. `--resume <id> --add-dir <dir>` did.

Not tried: auto mode, `bypassPermissions`, a sandboxed Bash.
