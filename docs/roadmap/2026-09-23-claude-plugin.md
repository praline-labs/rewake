# A Claude Code plugin that hears an interruption — September 23, 2026

The owner approved, on September 23, 2026, stage 1 of a function-hooks plugin of
rewake's own for Claude Code: observation only. Claude Code runs no hook when a person
interrupts a turn with Esc or Ctrl+C, and the wrapper gets no signal, so an interrupted
task stayed owed and the next unrelated turn end settled it as `finished`
([traps.md](../traps.md#an-interrupted-claude-code-task-is-reported-finished-with-an-unrelated-answer)).
review-claude's research of the same day found that a function-hooks plugin does hear
it: `turn.complete` with reason `aborted`
([research-claude-control.md](../research-claude-control.md#what-a-function-hooks-plugin-hears)).

**What was built** ([claude-plugin.md](../claude-plugin.md)):

- The wrapper writes the plugin beside its telemetry socket for each run and passes
  `--plugin-dir` and `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1` for that launch only; the
  person's configuration, the `--settings` merge and `--command` are untouched. Not
  passed under `--bare` or when the person switched function hooks off, with a launch
  note.
- The module reports `plugin.ready`, `turn.start`, `turn.complete` with its reason and
  `session.measure` through `rewake observe`, without waiting, and never reads the
  prompt or the answer.
- An `aborted` turn end is published by the collector as `stopped` at once, with the
  Codex semantics: the same text and notice, advisory, the waits kept, and the next
  finished turn end settles the task — the owner's decision that the harnesses do not
  diverge. Other turn ends are left to the Stop and StopFailure hooks, so an ordinary
  turn gives one report.
- `turn.start` and `turn.complete` set activity, which ends "working after Esc";
  `session.measure` fills the context while the status line has said nothing.
- The snapshot's `interruptions` reads `observed` after the plugin's first event and
  `unobserved` once a turn was heard without one: a readiness event decides, not a
  version check, because loading depends on more than the version.

**Tests.** Unit tests for the decoding, the collector's `stopped`, the launch additions,
and the module run under node with events that throw on a read of conversation text.
The Claude Code fixture now loads the plugin as the harness does, stricter than it, and
runs the module; the workflow case `claude-interrupted` and its three mutant controls
are described in [testing.md](../testing.md#claude-code-interruptions).

A Claude Code run now keeps a read clock for its whole life, as a Codex run always has,
so that an interruption's report is bounded by what was read when it was heard. Its
file, `awaiting/<epoch>/.read-clock`, is skipped by everything that reads obligations;
`claude-inbound` globbed that directory with `*`, which in Go matches a leading dot,
and read the clock as a task owed. Its glob now leaves dot names out.

**The review** by review-claude the same day ran the built tree against the real
harness, in a private HOME with a stand-in API, and found the plugin silent there:

- The module called `$.process.run({ argv, init })`; 2.1.280 takes only
  `$.process.run(argv, init)` and refused every call, which the module caught, so the
  snapshot read `interruptions: unobserved` after a real turn. The fixture and the unit
  host accepted both forms. The call is fixed, and both hosts now check it as the binary
  does and refuse anything else; a unit test holds each host to the refusals.
- An Esc landing on the Stop hook ended the turn twice — `classic.Stop`, then
  `turn.complete` `aborted` — and main got a `stopped` after the sender's `finished`.
  The module now hears `classic.Stop` and `classic.StopFailure` and does not report an
  `aborted` end after one in the same turn. The fixture keeps the module loaded for the
  whole session, as the harness does, and plays the ends in the order seen live; the
  workflow case has a fourth worker for it and a fourth mutant.
- `session.measure` is sent only when `changed` names `context`; the plugin directory is
  removed also when the collector did not start; `rewake list` adds `interruptions
  unheard` to the status for a session that does not hear them.
- After a compaction the listing read `0%`: the status line reports zeros until the next
  response. A zero count now reads unknown, a compaction drops the fill counted before
  it, and a status line started before it is not taken.
- The live observations are in
  [research-claude-control.md](../research-claude-control.md#what-a-function-hooks-plugin-hears).

**The owner's decisions before the commit**, September 23, 2026:

- A `stopped` goes only to the sessions waiting for a report from the interrupted one;
  the rule that sent it to main when nobody waited is gone, on both harnesses, and so is
  the `stopped` main put into its own inbox on its own Esc
  ([turn-outcomes.md](../turn-outcomes.md#keyboard-stops)). The workflow case
  `stopped-routing` checks it on both columns, with a mutant control restoring the old
  rule; `claude-interrupted` now counts the stops a worker published, since a second
  `stopped` would reach nobody's inbox.
- The switch stays, and rewake switches no built-in plugin off: the plugin-authoring
  skill line it adds, about 390 characters per request, is accepted. write-claude's
  research of the built-ins and of how `enabledPlugins` merges across settings layers —
  key by key, the person's own entries kept — is in
  [research-claude-control.md](../research-claude-control.md#built-in-plugins-and-what-the-switch-adds);
  `pluginUsage` for `rewake@inline` turned out to come from `--plugin-dir`, not from the
  switch ([claude-plugin.md](../claude-plugin.md#limits)).

**What stays open.**

- HF-06 on Claude Code is **impl?** until the owner accepts an Esc at the keyboard on a
  live session ([harness-features.md](../harness-features.md#capability-map)). review-claude
  saw the real harness call the module the same day and rewake receive `stopped`,
  `finished` and `error`, with the interruption simulated by `$.turn.abort`.
- The remaining risk the traps entry names: a person who turns the session to unrelated
  work after Esc settles the interrupted task with that work's `finished`.
- A plugin that unloads after its first event leaves `interruptions` at `observed`.
- The API is early access; a release that changes it silences the plugin, which falls
  back to the behaviour before it.
- The cost of the module's process start per event was not measured under the harness.
- The plugin's `turn.start` is not yet recorded as a turn start for `rewake pending`;
  UserPromptSubmit still is.
- Stage 2, acting on the session, is not scheduled
  ([work-queue.md](../work-queue.md)).
