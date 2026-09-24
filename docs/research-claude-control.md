# Research: steering a running Claude Code session from outside

Split from [research.md](research.md) by subject on September 23, 2026: what reaches a
running Claude Code session besides a message, and what it lets rewake hear back — an
interruption, the session's own commands, text put into its input box. The Codex side of
the same questions is in [research-protocol.md](research-protocol.md#compaction-and-the-terminals-commands-over-the-protocol).

Tags, with the version in each section: **[live]** — observed in a private HOME against
a fake API on a local port, keys typed through tmux; **[source]** — read in the installed
binary's bundled source; **[docs]** — the official pages. An older source tree of the
harness served only as a map of where to look in the binary; no fact below rests on it.

## Interrupting a turn and changing the conversation

Probed on September 23, 2026, Claude Code 2.1.280, in a private HOME against a fake API
on a local port, with Esc and Ctrl+C typed through tmux **[live]** and the abort path
read in the binary **[source]**.

Through hooks and the wrapper alone, an interrupted turn is marked by nothing rewake
can hear (HF-06); a function-hooks plugin does hear it, and rewake's own plugin reports
it since September 23, 2026 ([below](#what-a-function-hooks-plugin-hears)). The probe
found:

- Esc while the request hangs, while text streams, and during a Bash tool: no `Stop`,
  `StopFailure`, `Notification` or `PostToolUseFailure`. No hook, no status-line field
  and no key of a later `UserPromptSubmit` says the turn was interrupted.
- `idle_prompt` did not come within 80 seconds after an Esc; after an ordinary `Stop`
  it came 61.6 seconds later.
- The only marker, `[Request interrupted by user]`, is in the next request's body — the
  transcript, which rewake does not read.
- Ctrl+C once or twice during a turn behaves like Esc. Twice while idle ends the
  session: `SessionEnd` with reason `prompt_input_exit`, and the wrapper exits 0.
- In the binary the abort branches return `aborted_streaming` or `aborted_tools` and
  never call the Stop hook, whose callers are `blockable_turn_end`,
  `turn_end_reactions` and `loop_tick`. A `terminal_reason` of `aborted_*` exists only
  in the SDK's stream-json output. **[source]**
- The hooks reference says the same of `Stop`: "Does not run if the stoppage occurred
  due to a user interrupt." A hook for an interruption is an open feature request,
  anthropics/claude-code#9516. **[docs]**

Changing the conversation (HF-19):

- `/clear`: `SessionEnd` with reason `clear` and the old `session_id`, then
  `SessionStart` with source `clear` and a new one, without a `model` key; a status
  line with the new id about 300 ms later.
- `/resume <id>`: `SessionEnd` with reason `resume`, then `SessionStart` with source
  `resume` and the resumed conversation's original id, with extra keys —
  `context_tokens`, `seconds_since_last_response`, `prompt_cache_likely_expired`,
  `estimated_cache_write_usd`, `model`, `prompt_id`. The picker behaves the same.
- The wrapper's socket survives both, and a delivery and its `Stop` work after each.
- Not checked: `--resume` or `--continue` at launch through rewake, and resuming a
  conversation from another project or one started outside rewake.

### A second probe: signals and hooks around an interruption

Probed on September 23, 2026, Claude Code 2.1.280, same setup **[live]**. The question
was whether anything short of the transcript reaches the wrapper or a hook process at
the moment of an interruption.

- **The wrapper gets no SIGINT.** Ctrl+C in the running terminal UI, once or twice,
  during a turn or idle, never reached the wrapper as a signal. `stty` showed `-isig`
  in both states: the UI holds the terminal raw, so Ctrl+C arrives as a key, not as a
  signal to the foreground process group. Proven with a logging build of the wrapper
  whose SIGINT-dropping goroutine wrote a line per signal; `kill -INT` from outside
  produced the line, Ctrl+C never did. Esc likewise reaches only the UI.
- **An async `UserPromptSubmit` hook outlives everything.** Its process survived `Stop`,
  Esc, Ctrl+C and the end of the session itself: seven instances, all exiting normally
  after their full 150 seconds. Nothing is sent to it, so its fate says nothing about
  the turn.
- **A sync `UserPromptSubmit` hook sees an interruption, but only of its own phase.**
  It blocks the turn — the UI shows `running UserPromptSubmit hooks…` and the request
  is not sent until the hook ends — and an interruption during it sends it SIGTERM,
  72 ms after Esc and 14 ms after Ctrl+C. Once it has returned, the request and the
  tools that follow are outside its reach, and blocking every turn behind a hook is a
  cost of its own.

So no immediate interruption signal is reachable by rewake within its boundaries through
a hook or the wrapper. Catching Esc in the wrapper would mean reading the keyboard between
the person and the harness — a pseudo-terminal proxy, which the project's boundaries
exclude ([AGENTS.md](../AGENTS.md#boundaries)).

### What a function-hooks plugin hears

Probed on September 23, 2026, Claude Code 2.1.280, same setup **[live]**, with the
event list read in the binary **[source]**. rewake builds its interruption signal on
this ([claude-plugin.md](claude-plugin.md)).

- **Enabling it** takes `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1` and `--plugin-dir <dir>`
  (or `CLAUDE_CODE_PLUGIN_DIRS`) for one launch. It loads only in a trusted workspace,
  not under `--bare` or `disableAllHooks`, and the API is marked early access
  **[source]**.
- **Its events** include `turn.start`, `turn.step`, `turn.complete` with a reason of
  `answer`, `aborted`, `refusal` or `error`, `session.measure` with the context's
  `tokens`, `window` and `percent`, `session.end`, `session.receive` and the `classic.*`
  family **[source]**.
- **An ordinary turn** gave `turn.start`, then `turn.complete` with reason `answer`, and
  the Stop hook ran **[live]**.
- **The order at a turn's end**, recorded by review-claude the same day on 2.1.280
  **[live]**: in five ordinary turns `classic.Stop` came before `turn.complete` with
  reason `answer`, by 15 to 30 ms. On an API error and on a refusal `classic.StopFailure`
  came first, then `turn.complete` with reason `error` or `refusal` about 1 ms later; a
  refusal gave one `error` report through rewake.
- **Esc during a tool** gave `turn.complete` with reason `aborted` and no Stop; Ctrl+C
  likewise, with the rewake wrapper alive **[live]**. An Esc landing on the Stop hook
  gave `classic.Stop` and then `turn.complete` with reason `aborted` **[live]**: the
  same turn ends both ways.
- **What a handler can do**: run a process and write files (`$.fs.write`) **[source]**;
  a module that fails unloads the plugin and the session goes on without it **[live]**.
  The process call is `$.process.run(argv, init)`: argv a non-empty list of strings,
  the first naming the command, and init an optional `{ cwd?, env?, stdin?, timeoutMs? }`
  — cwd a non-empty path, env an object of strings, stdin a string, timeoutMs a whole
  number of ms from 1 to 600000, 30000 when left out **[source]**. Any other form is
  refused before anything runs, with a rejected promise:
  `process.run: takes argv, a non-empty list of strings naming the command first (host
  check)`. A single options object `{ argv, init }` is such a form; a module calling it
  so ran nothing on a live session **[live]**, review-claude, September 23, 2026.
- **The load check on `$`** **[live]**, review-claude, September 24, 2026, 2.1.280: a
  module that uses `$` inside a logical expression — `$.env.get(x) || …` — does not load,
  and the debug log says `$ itself is used in a LogicalExpression (bound, passed, spread,
  returned or read); $ is always spelled $.noun.event(...) at the call site`. The module
  then hears nothing. A module also cannot see `process`: the environment is read
  through `$.env.get(name)` and the settings through `$.settings.read()`.
- **Side effects of switching function hooks on** **[live]**, review-claude, September
  23, 2026, 2.1.280: the built-in plugin-authoring skill appears in the model's skill
  list (seen in the request body), and function-hook modules of plugins the person
  installed are enabled too. What each built-in needs, and how settings layers combine,
  is [below](#built-in-plugins-and-what-the-switch-adds).
- **After a compaction** the context counts are zero until the next response, as the
  status line's are ([research.md](research.md#telemetry-sources-the-status-line-and-hooks)).
  Not checked for `session.measure` itself; rewake reads zero tokens as unknown either
  way.

### Built-in plugins and what the switch adds

Probed on September 23, 2026, Claude Code 2.1.280, by write-claude, with `-p` in a
private HOME, a stand-in API and all other outbound traffic refused **[live]**; the
registration and the gates read in the binary **[source]**.

- **The switch.** `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS` overrides the harness's remote flag
  `tengu_plugin_hooks_modules`, off by default and absent from the cached flags. It is
  three-valued: `0` gave the same run as leaving it out **[live]**. It opens the hooks
  modules of installed plugins, and the built-ins whose availability asks the same gate;
  the built-ins' own modules load regardless of it — "built-in plugins load regardless"
  in the debug log **[live]**.
- **Nine built-ins are registered** — sec-default, agents-md, telemetry,
  plugin-authoring, tips, mermaid, responsive-mode, diff, claude-test **[source]**:
  - **telemetry** and **agents-md** load without the switch: telemetry whenever
    analytics is not off, agents-md when the remote flag `tengu_agents_md_mod` is on,
    as it is in the owner's cached flags and was seeded for the probe **[live]**.
  - **plugin-authoring** loads only with the switch: its availability is the gate
    itself. It has no module and one skill, and adds one line to the skill listing of
    every request, 390 characters, about 90 to 100 tokens; the skill's body is read only
    when it is invoked. The request body was 70681 bytes without the switch and 71072
    with it, identical otherwise: this line is all the switch added to the request
    **[live]**.
  - **mermaid** needs the switch and the remote flag `tengu_mermaid_mod`; **diff**
    needs `tengu_quiet_dolphin` and the interactive mode; **tips** needs
    `tengu_tips_mod`; **responsive-mode** needs `tengu_quiet_ember`. None of those flags
    was on, and none of the four loaded, with the switch or without **[source, live]**.
    **claude-test** appeared in no run. mermaid was probed on its own the next day
    ([below](#the-mermaid-built-in)).
  - **sec-default** was not seated: without the switch because "installed plugins'
    hooks modules are off", with it because there are "no managed settings and not a
    Team or Enterprise organization" **[live]**.
- **Switching a built-in off.** `enabledPlugins` with `"<name>@builtin": false`; a
  built-in left out counts as enabled **[source]**. Seen working for telemetry,
  agents-md and plugin-authoring **[live]**. sec-default is the exception: it is read
  from managed settings only **[source]**. agents-md also takes
  `pluginConfigs["agents-md@builtin"].options.instructionFiles: "claude-md"`, which left
  only its `session.start` hook and no AGENTS.md in the request; project settings are not
  read for it **[live]**. For the skill alone, `skillOverrides: {"plugin-authoring":
  "off"}` hides the line but keeps the plugin enabled and recorded as used **[live]**.
- **`pluginUsage`.** The harness records the plugins a session used in `pluginUsage` in
  `~/.claude.json`. `rewake@inline` is written there by `--plugin-dir` itself, with the
  switch or without it; the switch adds `plugin-authoring@builtin` **[live]**.
- **Installed plugins' modules.** Each plugin's module runs in a worker process of its
  own ("hooks worker spawned (one for every plugin)"); without the switch an installed
  plugin's module is not loaded at all ("rewake@inline not loaded") **[live]**. A module
  sees nearly every event and may rewrite prompt sections and context, classic hooks'
  input and output, what a settings read returns and the tool registrations, with
  process, HTTP, file, environment and store calls at hand **[source]**. Nothing turns
  off other plugins' modules alone: `allowManagedHooksOnly` and `disableAllHooks` turn
  off rewake's as well **[source]**. No plugin installed on the owner's machine had a
  module on September 23, 2026.
- **To repeat on every new Claude Code version**: diff the request body of a `-p` run
  with the switch and without it. A new built-in gated on the switch shows there first.

### The mermaid built-in

Probed on September 24, 2026, Claude Code 2.1.280, by review-claude, in a private HOME
with a stand-in API on a local port and a proxy that logged and refused every outbound
CONNECT. The stand-in put mermaid fences into its replies, and the remote flag was
seeded into the private `.claude.json` beside a read-only copy of the owner's other
cached flags **[live]**. The module was read in the binary **[source]**.

- **What it does.** A hooks module with no skill, command, agent or setting; its
  manifest scans `hooks:["ui.render"]` and `calls:[]` **[source]**, and the debug log
  reads `hooks module mermaid@builtin loaded (native, environment 3, tier builtin);
  events: ui.render` **[live]**. Its one hook is `ui.render` on the `AssistantMessage`
  component of the terminal surface. It rewrites the reply's text before drawing: a
  `` ```mermaid `` fence becomes a block headed `mermaid · flowchart` or `mermaid ·
  sequence diagram`, drawn in box-drawing characters by its own renderer in plain JS —
  flowchart and graph with directions, subgraphs and solid, dotted and thick arrows, and
  sequenceDiagram; other diagram kinds are left alone **[source]**. A flowchart LR and a
  sequence diagram were drawn; without the flag the same reply showed the raw fence
  **[live]**.
- **Its bounds.** A fence up to 20000 characters, a drawing up to 300 lines, a reply
  grown by at most 60000, a flowchart up to 200 nodes and 400 edges; the width is
  `viewport.columns`, 80 by default. A diagram that does not fit or does not parse stays
  as it was. Results are cached in memory, at most 32 entries and 250000 characters
  **[source]**.
- **The screen only.** Under `-p` stdout carries the raw fence **[live]**. The prompt,
  the request body and the history are unchanged: the next request carried the
  assistant turn with the raw mermaid and no box characters, and the transcript on disk
  keeps the raw text **[live]**. So a report's text and the transcript are the same with
  it or without; only what the person sees changes.
- **The gate.** Available when the function-hooks switch resolves on, screen-reader
  mode is off and the remote flag `tengu_mermaid_mod` is on; not on by default
  **[source]**. The flag's value is fixed at its first read in a process, so a change
  mid-session applies only after a restart; it comes from a local override, the
  payload, the disk cache or the default **[source]**. `-p` runs **[live]**:

  | run | debug log | mermaid |
  |---|---|---|
  | switch `1`, no flag | Found 3 plugins | not loaded |
  | switch `1`, flag on | Found 4 plugins | loaded |
  | flag on, switch unset | Found 2 plugins | not loaded |
  | flag on, switch `0` | Found 2 plugins | not loaded |

  Unlike rewake's own plugin, `disableAllHooks: true` and `--bare` do not switch it off:
  it loaded under both, since its gate is the switch rather than the check that governs
  installed plugins' modules **[source, live]**.
- **Not in rewake sessions today.** On September 24, 2026 the owner's cached flags,
  read only, had no `tengu_mermaid_mod` key, so the default off applies;
  `tengu_plugin_hooks_modules` was absent too, and `pluginUsage` named no
  `mermaid@builtin`. rewake sessions do not get it, and would once the server turns the
  flag on and it reaches the disk cache.
- **No network of its own.** `calls` is empty, and the module has no fetch; it imports
  only gates, telemetry and registration **[source]**. The refused CONNECTs were the
  same with the flag and without — under `-p` five to the API host and one to a
  telemetry host, in the terminal UI the same hosts **[live]**. Its only trace is
  telemetry through the harness's shared analytics queue: `tengu_feature_ok` with
  `feature_name:"mermaid_render"` once per session per outcome (`drawn`),
  `tengu_feature_sad` on `fell_back` or `threw`, and the ordinary plugin-load event with
  `plugin_name:"mermaid"` **[source, live]** — seen in the private HOME's telemetry
  directory, where the refused sends left them. The binary also carries
  `mermaid.min.js` 11.16.1; that is the runtime for published artifacts, and the plugin
  does not use it **[source]**.
- **No files of its own** **[source: `calls` empty; live]**. The one write is the line
  `mermaid@builtin` in `pluginUsage`, made by the harness **[live]**, beside the
  telemetry events in their usual place; the drawn text reaches no file **[live]**.
- **Switching it off.** `enabledPlugins: {"mermaid@builtin": false}` through
  `--settings` gave `Found 4 plugins (3 enabled, 1 disabled)` and the module did not
  load **[live]**; a key in the flag layer wins over the same key in the user file
  ([below](#how-enabledplugins-merges-across-settings-layers)). The switch unset or `0`
  and screen-reader mode switch it off as well **[source; the switch live]**.
- **Cost per request: none.** `-p` request bodies with the flag and without were 80777
  bytes each and differed only in `metadata.user_id`, with `enabledPlugins` off the
  same **[live]**. Drawing a reply in the terminal UI took `ui.render settled in 5.5ms`
  and `2.4ms` **[live]**.
- **To re-check on a new version**, from the source alone: find the module with
  `PLUGIN_NAME:"mermaid"`, and read its `isAvailable:()=>` and its
  `scan:{hooks:[...],calls:[...]}`. A new entry in `calls` would mean network or disk of
  its own.

### How `enabledPlugins` merges across settings layers

Probed on September 23, 2026, Claude Code 2.1.280, by write-claude, same setup, the
switch on in every run **[live]**. The owner decided the same day that rewake switches
off no built-in, so rewake's `--settings` carries no `enabledPlugins`; this is recorded
so that the answer is known before anyone passes one.

The private HOME's `settings.json` enabled two dummy plugins, `alpha@probe-mkt` and
`beta@probe-mkt`, installed from a local directory marketplace. Each run passed a
`--settings` flag layer and read the debug log's plugin count and the skill listing in
the request body:

| flag layer | debug log | skills listed |
|---|---|---|
| none | Found 5 plugins (5 enabled, 0 disabled) | alpha, beta, plugin-authoring |
| `{"enabledPlugins":{"plugin-authoring@builtin":false,"mermaid@builtin":false}}` | Found 5 plugins (4 enabled, 1 disabled) | alpha, beta |
| the same plus `"alpha@probe-mkt":false`, as rewake's merge of a caller's `--settings` would pass it | Found 5 plugins (3 enabled, 2 disabled) | beta |
| the second row, with `"plugin-authoring@builtin":true` added to the user file | Found 5 plugins (4 enabled, 1 disabled) | alpha, beta |

The conclusion rests on one more fact, checked the next day, September 24, 2026, on the
same version and setup rebuilt: an installed plugin with no `enabledPlugins` entry is
off. The rebuilt HOME did not have agents-md's remote flag seeded, so each total is one
lower than above:

| user file | flag layer | debug log | skills listed |
|---|---|---|---|
| alpha and beta `true` | none | Found 4 plugins (4 enabled, 0 disabled) | alpha, beta, plugin-authoring |
| alpha `true`, beta installed with no entry | none | Found 3 plugins (3 enabled, 0 disabled) | alpha, plugin-authoring |
| the same | the built-ins' row above | Found 3 plugins (2 enabled, 1 disabled) | alpha |

So the second row of the first table could only list alpha and beta because the user
file's entries still counted. `enabledPlugins` merges key by key: a flag layer naming
only built-ins leaves the person's own entries in force, and a key the flag layer names
wins over the same key in the user file. mermaid did not load in any run, so it counts
in none of the totals.

## Commands from outside

Probed on September 23, 2026, Claude Code 2.1.280, same setup **[live]**, with the
queueing path read in the binary **[source]**.

- **The inbound socket never runs a slash command.** Every line of type `user` is
  queued with `skipSlashCommands`, `isMeta` and `skipAttachments` set, whatever its
  priority and whatever the permission mode **[source]**. `/compact`, `/clear`,
  `/model`, `/effort`, `/status`, `/cost`, `/context`, `/config`, a custom command and
  a skill each reached the model as plain text, both between turns and mid-turn
  **[live]**.
- **The socket's control actions** are `rename`, `peer_message_status`,
  `notify_when_idle`, `peer_idle_notice` and three actions for replies to artifacts;
  none of them compacts, clears, switches a model or runs a command **[source]**.
- **A typed `/compact`**, for comparison, gives `PreCompact` with trigger `manual`,
  `SessionStart` with source `compact`, then `PostCompact` **[live]** — the same
  sequence as in [research.md](research.md#telemetry-sources-the-status-line-and-hooks).

## Putting text into the input box

Probed on September 23, 2026, Claude Code 2.1.280 **[live]** and in the binary
**[source]**. The question was whether rewake can prepare a command — `/compact` with
instructions, say — for the person to confirm with Enter, since it cannot run one.

- **At launch**: the flag `--prefill`, see
  [research-launch.md](research-launch.md#a-prompt-draft-at-launch). First prompt only.
- **A function-hooks plugin.** Enabled by `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1`, loaded
  from `--plugin-dir` or `CLAUDE_CODE_PLUGIN_DIRS`, only in a trusted workspace and not
  under `--bare` or `disableAllHooks`; the API is marked early access **[source]**. A
  handler on `session.receive` can call:
  - `$.prompt.fill` — puts text into the input box without submitting. Verified: `/compact
    keep only the plan` appeared in the input, no turn ran, and Enter gave a real manual
    compaction with those instructions **[live]**. Mode `replace` silently overwrote text
    the person had typed, so a handler reads the box first or appends.
  - `$.prompt.suggest` — a grey suggestion, shown only when the input is empty and the
    session idle; Tab takes it, Enter sends it **[live]**.
  - `$.session.compact` and `$.command.run` exist in the declarations and would act
    without an Enter. Not tested live: the probe was refused by the auto-mode
    classifier **[source]**.
- **Not usable for a draft**: the socket's envelope and control frames; a command hook's
  output — `SessionStart`'s `initialUserMessage` submits at once, and only in print or
  SDK mode; the built-in prompt suggestions, which nothing outside feeds; Remote
  Control; deep links, which open a new session **[source]**.
