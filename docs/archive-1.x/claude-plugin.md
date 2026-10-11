# Claude Code plugin

How rewake hears what no Claude Code hook says — above all a turn a person interrupted
with Esc or Ctrl+C — through a function-hooks plugin of its own, and what the wrapper
does with it. Stage 1, observation only, approved by the owner on September 23, 2026:
the plugin runs no command in the session and compacts nothing. Stage 2, built on
September 24, 2026, lets a main act through it — compact the session or abort its turn
on request, and nothing else — described in [remote-control.md](remote-control.md);
this document is about what the plugin hears. Switching function hooks
on for the launch does change more than loading this plugin — see
[Limits](#limits). The facts about the harness it rests on are in
[research-claude-control.md](research-claude-control.md#what-a-function-hooks-plugin-hears)
and HF-06 in [harness-features.md](harness-features.md#capability-map); the telemetry it
extends is in [claude-telemetry.md](claude-telemetry.md).

## What the launch carries

- **The plugin** is written by the wrapper for each run, beside the telemetry socket:
  `sock/<name>.<epoch>.obs.plugin/` with `.claude-plugin/plugin.json` (name `rewake`),
  `hooks/hooks.json` naming one module, and `hooks/rewake.js`, the module embedded in
  the binary (`internal/harness/claude/plugin.js`) with the command it runs written in.
  The wrapper removes it when it exits, with the socket, and also when the collector
  could not start; only a wrapper killed outright leaves it behind.
- **Two launch additions**, for this launch only: `--plugin-dir <that directory>` and
  `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1` in the harness's environment. The person's own
  `--plugin-dir` flags stay beside it, and rewake reads and writes nothing in their
  configuration for it; the harness itself records the plugin's use there, see
  [Limits](#limits). The `--settings` merge and `--command` are unchanged.
- **Not passed**, with a launch note saying interruptions are not heard: under
  `--bare`, which loads no plugins, and when the person set the switch to an off value
  (`0`, `false`, `no`, `off`, empty) — turning it back on would change more than
  rewake's own plugin. Without a telemetry socket there is nobody to report to, and the
  plugin is not written.

## What the plugin reports

The module registers six handlers. Each reporting one picks out the fields rewake uses,
starts `rewake observe <socket>` with them as JSON on stdin — the command and the
datagram the telemetry hooks use — through `$.process.run(argv, { stdin, timeoutMs })`,
does not wait for it, and passes the event on with `next`. The harness takes exactly
that form: an options object as the only argument is refused with a rejected promise,
and an early version of the module that called it so reported nothing on a live session
while every fixture passed. A refused or failed start is caught and dropped; the harness
would otherwise unload the whole plugin.

| Harness event | Sent as | Fields |
|---|---|---|
| `session.start` | `plugin.ready` | the window limit |
| `turn.start` | `turn.start` | the turn id |
| `turn.complete` | `turn.complete` | the turn id and the reason: `answer`, `aborted`, `refusal` or `error`; for a turn a main's `rewake interrupt` aborted, `by` with the main's name |
| `session.measure` | `session.measure` | `tokens`, `window` and `percent` of the context and the window limit, only when `changed` names `context` |
| `classic.Stop`, `classic.StopFailure` | nothing | none: they mark the turn as reported by its hook |
| none: a main's `rewake compact` about to run | `compact.asked` | the request id and the main's name; the module waits until it is sent, then compacts ([remote-control.md](remote-control.md#on-claude-code)) |
| none: that compaction refused or failed | `compact.refused` | the request id and whether the host had started it, sent before the answer |
| `session.compact`, only for a compaction the module did not ask for — the host skips its handlers for its own as re-entry | nothing | none: its end starts the second the module waits before a main's compaction |

An event that carries an `agentId` is a subagent's and is not sent. `session.measure`
fires for rate limits and cost as well, and each report is a process, so only a change
of the context is sent. The prompt, the answer, the last message and every other
conversation field are never read: a test runs the module under node with events that
throw when `text`, `answer`, `prompt`, `last_assistant_message` or `error_details` is
touched (`internal/harness/claude/plugin_test.go`). The collector decodes only the named
fields (`internal/harness/claude/telemetry/plugin.go`), as it does for hooks.

**The window limit** is the raw `CLAUDE_CODE_AUTO_COMPACT_WINDOW`, read with
`$.env.get`, and the `autoCompactWindow` key of `$.settings.read()`, sent as `limit`
with `env` and `settings`. When either read throws, the event carries no `limit` at
all, and the last limit the wrapper heard stands: a read that failed is not a source
that went away, and reporting it as none would put the model's window back in the
listing. A test under node fails each read in turn and checks that. The environment
and the settings are the person's and can hold secrets, so nothing but that variable
and that key leaves the module, and a test under node gives it an environment and a
settings `env` block with secrets in them and checks that none is sent. What the wrapper does
with the values is in [claude-telemetry.md](claude-telemetry.md). The harness refuses to
load a module that uses `$` inside a logical expression
([research-claude-control.md](research-claude-control.md#what-a-function-hooks-plugin-hears)),
so each read stands in its own `try`, and a test reads the module for `$` beside `||`,
`&&` or `??`.

**An Esc on the Stop hook.** Live, a person's Esc can land after the turn's Stop hook has
run: `classic.Stop`, then `turn.complete` with reason `aborted`. The hook has already
reported the turn as `finished`, and a `stopped` after it would give the turn a second
outcome — one that reaches nobody, since the finished settled every wait, and that is
still wrong. So the module remembers, from
`turn.start` on, whether `classic.Stop` or `classic.StopFailure` came in the current turn,
and does not report an `aborted` end after one. The two handlers read nothing from the
event.

## What the wrapper does with it

- **An interruption is `stopped`, at once.** `turn.complete` with reason `aborted`
  makes the collector publish a completion of kind `stopped` through the same path a
  Codex keyboard stop takes ([turn-outcomes.md](turn-outcomes.md#keyboard-stops)): the
  same text, the same notice, advisory, and the waits kept. Its report id is built from
  `claude/<turn id>`, its read boundary is what the session had read when the event was
  heard, and its end is recorded as the earliest start of the next turn, as a Stop hook's
  is. The next turn end that finishes settles the task — the owner's decision that the
  two harnesses must not diverge here. A `stopped` goes only to the sessions waiting on
  this one; an Esc on the person's own prompt, with no rewake task owed, sends nothing
  and the session just turns idle — the owner's decision of September 23, 2026, shared
  with Codex ([turn-outcomes.md](turn-outcomes.md#keyboard-stops)). A turn a main
  aborted carries `by`, and its `stopped` says that main interrupted it rather than the
  person at the keyboard ([remote-control.md](remote-control.md#on-claude-code)).
- **Exactly one report per turn.** An ordinary turn now ends twice for rewake: in the
  plugin's `turn.complete` and in the Stop or StopFailure hook. Only `aborted` is taken
  from the plugin; `answer`, `refusal` and `error` are left to the hooks, which carry the
  last message and the error the report needs. An interrupted turn runs no hook, so
  nothing else reports it; one interrupted after its hook ran is not reported by the
  plugin at all ([above](#what-the-plugin-reports)).
- **Activity.** `turn.start` sets `working` and `turn.complete` sets `idle`, beside the
  hooks' own signals and ordered with them by the boot clock, so a late event of an
  earlier turn changes nothing. This ends "working after Esc".
- **Context.** `session.measure` fills the context only while the status line has said
  nothing about it; the status line stays the source whenever it runs. Zero tokens
  from either reads as unknown, and a compaction's end drops the fill counted before
  it, so after a compaction the listing reads `unknown` until the next response
  ([session-state.md](session-state.md#claude-code-source)).

## Whether interruptions are heard

The telemetry snapshot carries `interruptions`
([session-state.md](session-state.md#claude-code-source)): `observed` once any plugin
event arrived, `unobserved` once a turn was heard through the hooks with no plugin
event before it, and absent before either. That is decided by the plugin's own events,
not by a version check: whether it loads depends on more than the version — a trusted
workspace, `disableAllHooks`, a module that fails — and only its first event shows that
it did. Without it the session runs exactly as before the plugin: an interrupted turn
is not reported, and the next turn end that finishes settles its task.

## Limits

- **The remaining risk** is the one [traps.md](traps.md#an-interrupted-claude-code-task-is-reported-finished-with-an-unrelated-answer)
  keeps: after `stopped`, the sender waits for the person; a person who then turns the
  session to unrelated work settles the interrupted task with that work's `finished`.
- **A plugin that unloads later** — a module failing after it loaded — leaves
  `interruptions` at `observed`, and an interruption after that goes unheard.
- **The API is early access** and may change between releases. What the plugin reads
  was seen live on Claude Code 2.1.280; a release that renames an event or a field makes
  it fall silent, which is the fallback above.
- **Accepted live on one run of each key.** The fixture runs the module rewake writes
  under node, with a `$.process.run` as strict as the harness's, and plays it the events
  in the order seen live. That the real harness calls the module was seen by
  review-claude on September 23, 2026, on 2.1.280, in a private HOME with a stand-in
  API, with the interruption simulated by `$.turn.abort`. The owner accepted it at the
  keyboard on September 24, 2026, on 2.1.280 with rewake built from `d97da91`: an Esc
  and then a Ctrl+C, each during a worker's task, each gave main a `stopped`, and both
  tasks stayed awaited. The Ctrl+C's `stopped` arrived at most 5 s after its task was
  sent, the turn's start included; the Esc's has no such bound, since the time of the
  keypress is unknown. HF-06 is **live** on Claude Code; the observation and its limits —
  one run of each key, the keypress-to-notice time not measured apart, replay not
  checked deliberately — are in
  [harness-features.md](harness-features.md#capability-map).
- **The order** of the hook and `turn.complete`, seen live on 2.1.280 by review-claude
  on September 23, 2026: in five ordinary turns `classic.Stop` came 15 to 30 ms before
  `turn.complete` with reason `answer`; on an API error and on a refusal
  `classic.StopFailure` came first and `turn.complete` with reason `error` or `refusal`
  about 1 ms after it, and a refusal gave one `error` report. Only the Esc guard above
  rests on the order, and it holds whichever way an ordinary end runs, since only
  `aborted` is taken from the plugin.
- **What `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1` adds beyond rewake's plugin**, seen live
  on 2.1.280 on September 23, 2026
  ([research-claude-control.md](research-claude-control.md#built-in-plugins-and-what-the-switch-adds)):
  - **The plugin-authoring skill.** The switch adds the harness's built-in
    plugin-authoring skill to the model's skill list: one line of about 390 characters
    in every request. The owner accepted this on September 23, 2026, and rewake switches
    no built-in off; its `--settings` carries no `enabledPlugins`.
  - **Other plugins' modules.** A function-hooks module of a plugin the person installs
    later runs only in rewake sessions, where the switch is on, and sees nearly every
    event there. None of the person's plugins had one on that day.
  - **`pluginUsage`.** The harness records `rewake@inline` there in `~/.claude.json`
    because of `--plugin-dir`, with the switch or without it; the switch adds
    `plugin-authoring@builtin`. rewake itself writes nothing there.
  - **The mermaid built-in**, once the harness's remote flag `tengu_mermaid_mod` turns on;
    it was off in the owner's cached flags on September 24, 2026. It draws mermaid fences
    on the screen only — the request, the report's text and the transcript stay the same —
    and neither `disableAllHooks` nor `--bare` stops it; `enabledPlugins` does
    ([research-claude-control.md](research-claude-control.md#the-mermaid-built-in)).
  - **On every new Claude Code version**, repeat the request-body diff of a run with the
    switch and without it: a new built-in gated on the switch would show there.

## Tests

Unit: decoding and the refusals (`telemetry/plugin_test.go`), the collector publishing
`stopped` only for `aborted`, the launch additions and where they are left out, and the
module under node (`claude/plugin_test.go`) with a host that checks `$.process.run` as
the harness does and a test that the host refuses the forms the harness refuses. The
workflow case `claude-interrupted` and its four mutant controls are described in
[testing-plugin.md](testing-plugin.md#claude-code-interruptions), and `stopped-routing`, where a stop
goes on both harnesses, in [the section after it](testing-plugin.md#where-a-stop-goes).
Stage 2's tests — the control protocol, the commands, the module acting under a strict
host, and the stopped text and notice line after main's interrupt — are listed in
[remote-control-tests.md](remote-control-tests.md).
