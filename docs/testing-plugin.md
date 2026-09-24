# Testing rewake's plugin

The workflow cases that run rewake's function-hooks plugin for Claude Code: how the
fixture loads and hosts the module, and what each case claims, with its controls. The
tiers, the commands and how to read a result are in [testing.md](testing.md); the plugin
itself is in [claude-plugin.md](claude-plugin.md) and
[remote-control.md](remote-control.md).

## Claude Code interruptions

`claude-interrupted` runs on the Claude Code column only, with a main and four workers,
main running its own commands. The fixture loads the plugin rewake passed the way the
harness does — it refuses a directory that is not laid out as a plugin, or one passed
without function hooks switched on — and runs its module under node, loaded once for the
session so that its state lives from one event to the next, calling each handler with
the events the harness sends and running what the module asks `$.process.run` to run.
That call is checked as the 2.1.280 binary checks it — `(argv, init)`, argv a non-empty
list of strings — and any other form is refused and recorded in the case's evidence: a
host that also took `({ argv, init })` once passed a module whose every call the real
harness refused. The events throw when the module reads the prompt, the answer or the
last message. The fixture plays the order seen live: `classic.Stop` or
`classic.StopFailure`, then the hooks, then `turn.complete`. What the fixture cannot
prove is that the harness calls the module as it does; only a live session shows that.
The case's own PATH holds no node, so it passes the node it found; without one the case
and its controls are unsupported for the capability `node`, and every other case runs
with the plugin passed and not loaded. One worker's first turn reads its task and ends
as an Esc ends it: `turn.complete` with reason `aborted` and no Stop hook. main must read
`stopped` about the task before any later turn and still list it as `stopped` in
`--awaited`; the listing must show that worker `idle` with interruptions `observed`; after
a note, its next turn end must bring exactly one `finished`, which settles the task, and
the worker must have published one stop in all. A second worker's ordinary turn must
give exactly one `finished` and publish no stop, though the plugin heard its end too. A
third is interrupted just as its turn ends, the way an Esc landing on the Stop hook was
seen live: the Stop hook reports and `turn.complete` still says `aborted`; main must read
one `finished`, and the worker must publish no stop. A stop is counted by the turn receipt
the worker keeps for it, not in main's inbox: a `stopped` after the `finished` that
settled every wait goes to nobody
([turn-outcomes.md](turn-outcomes.md#keyboard-stops)), so main would never see it; on
this column only a stop leaves a receipt, since the Stop hook's report names no turn. A fourth is interrupted in a session whose harness does not load the plugin:
interruptions read `unobserved`, nothing arrives, and the next `finished` settles the task,
as before the plugin. Its four mutants — a collector that publishes nothing for
`aborted`, one that takes every turn end for an interruption, a launch that never
carries the plugin, and a module that does not heed the Stop hook — each name what they
break and require the rest to hold.

## Where a stop goes

`stopped-routing` runs on both columns, the Claude Code one through the plugin as above
and unsupported for `node` without it, and the Codex one through the fixture's switch
`RW_SHIM_INTERRUPT_FIRST_TURN`, which ends a session's first turn with `turn/completed`
of status `interrupted`, the shape the schema case checks. A main, running its own
commands, sends one worker a task and another a `--notify`, which owes nothing; both
workers and main have their first turn interrupted. main must read `stopped` about the
task and list it as `stopped` in `--awaited`; the notified worker's stop, once its
receipt shows it was acted on, must leave nothing in main's inbox, read or not, and the
worker must read `idle`; main's own stop must leave nothing in its own inbox. Its mutant,
stopped-to-main, restores the rule the owner removed on September 23, 2026 — a stop
nobody waits on goes to main — and must break the last two observations and leave the
first.

## Steering a session

`claude-steered` runs on the Claude Code column, and `codex-steered`, below, on the Codex
one. In `claude-steered` a main, running its own commands,
steers three workers with `rewake compact` and `rewake interrupt`, and each request
travels the product's whole path — the command, the control directory, the module
polling it, the session carrying it out, and back. Like `claude-interrupted` it is
unsupported for `node` without one, with its controls.

For it the fixture's plugin host serves what a module asks of the session itself. The
host gives the module `$.clock.every` and `$.clock.sleep` on real timers, `$.fs` with positional arguments
on real files — a `read` of a missing file logged as an `[ERROR]`, a `write` over 4 MiB
refused — `$.session.compact` and `$.turn.abort`, each as strict as 2.1.280 about its
forms ([research-claude-actions.md](research-claude-actions.md#compaction-abort-polling)).
The last two go out to the session as a line naming the call, and the session answers
on the host's stdin: a compaction is refused while a turn runs and with compaction
switched off by `DISABLE_COMPACT`, in the harness's words; one that goes ahead raises
`session.compact`, which the plugin's own handlers do not see, as the harness skips
them for re-entry, and runs the hooks `PreCompact`,
`SessionStart` with source `compact` and `PostCompact`, so rewake's telemetry counts it
by its own path, and answers with the summary and 120000 and 9000 tokens. Before any
turn has ended it stops after `PreCompact` and refuses the conversation as too short,
as the harness does. An abort is
refused with no turn running or for another turn's id, again in the harness's words;
otherwise it ends the running turn with `turn.complete` reason `aborted` and no Stop hook,
before it answers. Every refusal carries the plugin's name in front, as the harness puts
it there. A turn can be aborted only while the fixture holds it open: the switch
`RW_SHIM_HOLD_FIRST_TURN` keeps a session's first turn running after it reads its mail,
for twelve seconds at most, after which it ends as an ordinary turn.

The workers: calm works its task to the end, busy is held in its first turn, and bare
runs without the module. The observations:

- a compaction of bare is refused as `not answering`, exit 1, after the pickup limit;
- a compaction of busy is refused as `in a turn`, exit 1, and busy still reads `working`
  with nothing reported about its task;
- an interrupt of busy is `done`, exit 0, and main reads `stopped` about the task with
  the text "lead-claude interrupted this turn with rewake interrupt", which its
  `rewake inbox --awaited` shows after `stopped:` as well;
- busy's next notice ends with "lead-claude interrupted your previous turn with rewake
  interrupt.", and the notice after it does not;
- an interrupt of calm, idle, is refused as `no turn running`, exit 1;
- a compaction of calm with a focus is `done`, exit 0, with the host's token counts, and
  main's `rewake list --json` then counts one compaction for calm;
- that answer carries `compaction` 1, and main is sent no "Rewake: context compacted
  (compaction 1)." within five seconds. main's wrapper looks once a second, and the
  telemetry case sees the notice within five. Nothing later can stand in for the wait,
  since the absence is what is observed;
- calm, a worker, asking for a compaction is a wrong call, exit 2.

Its eleven mutants each break one link and name what they must break:

- a module that never asks the host to compact;
- one that does not know the host's mid-turn refusal;
- one that aborts without saying who asked;
- a lane that never uses the interrupt mark up;
- a module that answers an idle interrupt as done;
- a control package that reports a request nobody took as `failed`;
- a command that lets any role steer;
- a main wrapper that announces the compaction it asked for;
- a command that answers without the count;
- a module that compacts without telling rewake who asked;
- an awaited list that puts every stop down to a person.

A module that tells rewake who asked but does not wait for the report to be sent is not
among them, nor one that does not wait out the second after another compaction. The
fixture's host runs the report quickly enough that it would still arrive first, and the
fixture has no typed `/compact`, so the module test under node
(`plugin_compact_test.go`) checks both instead: its host resolves `$.process.run` a
timer later, and plays a compaction the module did not ask for. Both hosts are held to
the re-entry rule by a test of their own: `TestTheHostSkipsTheModulesHandlerForItsOwnCompaction`
and `TestClaudeShimSkipsAPluginsHandlersForItsOwnCall`.

What it cannot show: that the real harness carries these calls out as the fixture does
— its forms, refusals and effects were seen live by review-claude on September 24, 2026,
and are recorded in [research-claude-actions.md](research-claude-actions.md); where the
harness draws the line for "Not enough messages to compact", which the fixture puts at
one finished turn; the focus reaching the summary request, which only a live session
shows; and a stalled or killed session, which the unit tests of `internal/control`
cover.

### On Codex

`codex-steered` runs the same commands on the Codex column, where there is no plugin:
the worker's wrapper serves the control directory and carries a request out over its
app-server connection. The fixture's shim answers the two requests as the server of
0.155.1 does ([research-protocol.md](research-protocol.md#compaction-and-interrupt-on-request)):
`thread/compact/start` with `{}`, then the compaction as a turn of its own — the status
active, `turn/started`, the `contextCompaction` item started, a token usage of 9000, the
item completed, the status idle, `turn/completed` — and `turn/interrupt` with `{}`,
ending the held turn as interrupted, or refused in the server's words with no turn
running or another turn's id. A compaction that arrives while a turn is held aborts that
turn first, as Codex's `compact()` does, so a wrapper that does not refuse it breaks the
turn rather than passing. A work turn reports a usage of 120000 before it completes. The
switch `RW_SHIM_HOLD_TURN` holds busy's first turn for ten seconds at most. The shape
case checks the shim's reply and events for both requests against the schema, and that
the shim accepts neither request in a form the schema refuses.

calm works its task to the end and busy is held in its first turn; main is a Codex
session too. The observations:

- a compaction of calm with a focus is a wrong call, exit 2, naming the focus, and calm's
  compaction count afterwards is the one of the plain compaction below;
- a compaction of busy is refused as `in a turn`, exit 1, and busy still reads `working`
  with nothing reported about its task;
- an interrupt of busy is `done`, exit 0, and main reads `stopped` with "lead-codex
  interrupted this turn with rewake interrupt", as its `rewake inbox --awaited` does;
- busy's next notice carries no line about the interrupt;
- an interrupt of calm, idle, is refused as `no turn running`, exit 1;
- a compaction of calm is `done`, exit 0, with 120000 tokens before and 9000 after, and
  the telemetry counts one compaction;
- that answer carries `compaction` 1, and main is sent no compaction notice within five
  seconds;
- calm asking for a compaction is a wrong call, exit 2.

Its five mutants: a wrapper that sends a compaction whatever runs, which breaks the
refusal and, the held turn being aborted by it, the interrupt; a telemetry that counts
the compaction without its request and asker; a wrapper that interrupts without keeping
who asked; one that answers an idle interrupt as done; and a Codex harness that lets a
focus through to the wrapper.

What it cannot show: what the terminal does meanwhile — the live run of September 24,
2026 saw it hold a message typed during the compaction and send it after — and how long
a real compaction takes.
