# The workflow suite's cases

What the end-to-end cases in `test/workflow` claim, case by case: the sessions each one
starts, what the fixture plays for it, what it must see, and the controls that prove it
can fail. How to write a scenario, a control, a fixture or a column, and what every case
shares, is in [testing.md](testing.md#extending-the-suite); each scenario's invariant as
first argued is in [check-runner-scenarios.md](check-runner-scenarios.md).

## Claude Code telemetry budgets

`claude-telemetry` runs on the Claude Code column only, with a main and a worker: the
fixture plays the worker's hooks and status line through `/bin/sh -c` with the payloads
seen live, including their conversation fields, and the case reads the result from the
main's side — its `rewake list`, its header on a message from the worker, and the
compaction notice its wrapper sends. The worker tells the main only once its wrapper has
published it idle — the header is drawn from that publication, a quarter second apart,
and a main whose first notice waited for its status line once read the worker's message
inside that gap. A listing the main's fixture could not finish in five seconds is
recorded and fails the observations that needed it. The worker runs rewake's plugin
under node with a 150K `CLAUDE_CODE_AUTO_COMPACT_WINDOW` in its settings, under the
status line's 200K, so both must show 33% of 150K; without node it is unjudged. Accepted
September 24, 2026: no case plays a plugin-less session end to end; its model window and
`interruptions unheard` header rest on unit tests and claude-interrupted's `unobserved`.
The case also times the commands, cold each time, because a telemetry hook sits in front
of every prompt. Its four controls are mutants, and each names every observation it must
break and requires all the others to hold, which stands in for a crosswise run: a mutant
that broke everything would not pass as the control of one thing.

Measured on the development machine, September 23, 2026, 40 runs each: a telemetry hook
(shell plus a cold `rewake observe` sending one datagram) median 5.4 ms, p95 6.7 ms, and
on UserPromptSubmit, which also records the turn's start on disk, median 5.1 ms, p95 5.9
ms against the same budget; the
tap in front of a one-line `sed` status line added a median of 4.8 ms, p95 6.8 ms, to
that line's own 2.6 ms. The hooks run in the background (`"async": true`), so the agent
does not wait even for that. The case's budgets are a median of 20 ms and a p95 of 50 ms
for a hook, and a median of 20 ms added by the tap: several times the measurement, so
they catch a wait, a lock or a heavy start rather than a busy machine.

## Launch, reports and what a main waits on

`wrapped-launch` runs in both columns, twice each, in two rooms: a plain launch and one
with `--command ./<harness>-worker`, a stand-in wrapper that exports a marker and execs the
fixture. Every fixture process records its arguments and whether it saw the marker, and
the case requires the wrapped processes — one for Claude Code; for Codex the version
check, the app-server and the terminal — to get argument for argument what the plain
launch gets, all to see the marker, and none of the plain ones to. The Codex launches
carry `-C` into a directory holding another script of the same name, which must never
run. It has no control in the suite; with the relative path left relative, it went red on
three of its four observations (September 23, 2026).

`pending-report` runs in both columns with three sessions. A worker reads a task and,
in that turn, runs `rewake pending` before ending it; the sender must read a `pending`
message about the task first — the mark's line, then the turn's own text — and the
worker's awaiting record must still be there.
A third session then sends the worker a task of its own, and that turn end — with no
mark — must be the `finished` report settling the first task. Its three mutants, each in
both columns, ignore the mark, let the interim turn end settle the task, and drop the
turn's text from the interim message; like the
telemetry controls, each names what it must break and requires the rest to hold.

`owed-reread` runs in both columns with a main and a worker. The worker reads a
multi-line task and, in the same turn, runs `rewake inbox --owed` in both forms, as a
session re-reading its task after a compaction would. Both must print that task in full;
the machine form carries the id the worker read it under, and the text form opens with
`Rewake: owed a report for 1 message:` and the sender's header and leaves the id out.
The turn end must then report it once and
settle it: asking changed nothing. Its mutant, an `--owed` that finds nothing, must break
the first observation and hold the second.

`awaited-view` runs in both columns with a main and two workers. main runs its own
commands when the scenario asks (the fixture's request directory): it sends one task to
each worker. One reports at once; the other runs `rewake pending` in its turn, so its
task stays owed. main's `rewake inbox --awaited`, in both forms, must then list exactly
that second task, by id, as `pending` with the interim's text, under `to <worker>`, and
leave the first out. A note from main wakes the second worker, whose next turn end
reports; the view must then be `Rewake: nobody owes you a report.` Its two mutants run on
the Claude Code column only — the view reads files the same way whichever harness wrote
them: one blind to interim reports breaks the first observation alone, one that never
lets a task go breaks both.

## Actions on a sent message

`withdraw-after-notice`, `edit-after-notice` and `addendum-owed` run in both columns with a
main and a worker whose read waits at a gate (`RW_SHIM_READ_GATE`): its turn has started
on the notice and it has not read yet. main, through its request directory, then
withdraws the task, replaces it, or adds to it, each by the short id its send printed.
The worker must find a withdrawn note and never the task's text, a recall from main
marked as the recall of the task, shown in a notice on a line of its own that begins
`Do not act on task <short id>`, with nothing reported and nothing said undelivered; the
old id replaced by the new, the new text only, a notice that shows `Replaces <short id>
(withdrawn): ` before the new text, no recall, and one report on the replacement; and
`--owed` with the addendum under its task, then one report settling both. What a
notice showed is read from the worker's own record of its deliveries, on both columns.
`withdraw-mid-turn` runs the withdrawal on the Codex column alone with the worker's
first turn held open (`RW_SHIM_HOLD_TURN`; the read gate alone leaves no turn open to
steer into): the recall must be steered into the turn the task started, as a member
whose `recalls` names the task. Their mutants run on the Claude Code column, except
recall-unnamed, which drops that member field and runs on Codex; recall-sender-first
brings back the first wording, which led with the sender, and replacement-unmarked a
replacement previewed like any letter. The one that leaves the task readable also skips
the withdrawn status: the reader shows a message with that status as its tombstone, so a
tombstone written in the wrong place alone no longer reaches the worker as the task.

## Conversations and the inbound gate on Claude Code

`thread-changed` runs on the Claude Code column only, with two workers and a sender
for each. The fixture names one conversation, the session's own, in every hook and in its
status line, and on a switch plays `/clear` once its first task has arrived: a
SessionStart with source `clear` and a new `session_id`, which every later event then
carries. The report from that worker must carry `threadChanged` and still arrive once
and settle its task; the report from the worker that stayed where its task landed must
carry none. Its three mutants, like the telemetry controls, name what they break and
require the rest to hold: a wrapper that never pins the delivery and a turn end that
ignores the Stop hook's `session_id` must each lose the mark, and a comparison that
takes every known conversation for a change must mark the worker that stayed. Before
this case the fixture's status line named a conversation of its own while its hooks
named the session, which a tracker would have read as a `/clear` in every case.

`claude-inbound` runs on the Claude Code column only: Codex has no inbound gate, and its
column is unchanged. The fixture plays the gate as the binary does — it holds whatever
arrives in the first 600 ms after its socket listens, longer than the real two hundred so
a notice that does not wait is caught every time, and it answers held, released, expired
or refused lines with receipts on the reply socket, after checking that socket is in its
own directory and listened on by the process that wrote the line. It refuses a line that
does not ask for receipts, which the real one would merely not answer, and in one mode
reports a hold 900 ms late, past rewake's 300 ms wait for a first word, since nothing
bounds how soon the real one speaks under load. Five workers each get one task from a
sender of their own: one sent as soon as the socket appears, which must go out once the
session is up and not be held; one held and released, reported delivered after the hold
and then worked; one held until it expires, reported `held` with exit 3, its sender sent a
note that it never arrived, and the task failed, unread and not owed; one refused,
reported failed with exit 1 and never worked; one held late, whose task must end failed
and unworked with its sender told, whatever `send` said first. Its six mutants deliver
without waiting, wait for SessionStart instead of the status line, read a held receipt as
a delivery, settle an expiry without telling the sender, read a refusal as a delivery,
and drop a word that comes after the window; each names what it must break and requires
the rest to hold.

## The plugin's cases

The cases that run rewake's function-hooks plugin under node — `claude-interrupted`,
`stopped-routing` and `claude-steered` — and what the fixture's plugin host plays for
them are in [testing-plugin.md](testing-plugin.md), with `codex-steered`, the same
commands on the Codex column.

## Every control

Each scenario's controls by name: the product mutants, built from a `mutation` value in
`test/workflow`, and the switches that change the fixture's world instead
([testing.md](testing.md#extending-the-suite) says why a mutant is preferred).
`docs/controls_test.go` keeps the mutants column equal to the `mutation` values in the
suite and requires every listed switch to be a name the suite spells, so a mutant added
or renamed, or a listed switch renamed, without this table turns the five checks red. It
checks neither that a new switch is listed nor that a control sits in its scenario's row:
switches have no single form in the code to find them by, and the row is kept by review.

| Scenario | Mutants | Fixture switches |
|---|---|---|
| batch-arrival | `preview`, `window`, `unwindowed`, `replay`, `peek-consumes` | — |
| task-report | `no-stop-hook`, `turn-ended-ignores-stop`, `settles-nothing` | `wrong-report`, `read-fails`, `failure-before-report`, `early-exit` |
| readiness | — | `no-direct-input`, `wrong-thread`, `no-correlated-reply` |
| mid-turn | `wait-for-idle` | `late`, `failed-operation` |
| claude-telemetry | `tap-without-owner`, `uncounted-compaction`, `silent-compaction`, `model-window` | — |
| pending-report | `pending-ignored`, `pending-settles`, `pending-text-dropped` | — |
| claude-inbound | `ungated`, `gate-on-session-start`, `held-as-delivered`, `expiry-unannounced`, `refusal-as-delivered`, `late-word-dropped` | — |
| owed-reread | `owed-empty` | — |
| thread-changed | `delivery-unpinned`, `stop-thread-ignored`, `thread-always-changed` | — |
| awaited-view | `awaited-interim-ignored`, `awaited-never-settled` | — |
| claude-interrupted | `interrupt-unpublished`, `every-end-stopped`, `plugin-not-passed`, `stop-not-heard` | — |
| claude-steered | `compact-not-run`, `in-turn-unmapped`, `interrupter-unnamed`, `line-repeated`, `idle-interrupt-done`, `silent-not-answering`, `any-role-steers`, `own-compaction-announced`, `letter-uncounted`, `waits-for-the-end`, `ended-uncounted`, `asker-untold`, `orphan-unlettered`, `stop-by-a-person` | — |
| codex-steered | `busy-unchecked`, `codex-asker-untold`, `codex-waits-for-the-end`, `codex-outcome-unkept`, `codex-interrupter-unnamed`, `codex-idle-interrupt-done`, `focus-taken` | — |
| stopped-routing | `stopped-to-main` | — |
| withdraw-after-notice | `withdraw-leaves-task`, `withdraw-silent`, `recall-sender-first` | — |
| withdraw-mid-turn | `recall-unnamed` | — |
| edit-after-notice | `edit-keeps-old-text`, `edit-unlinked`, `replacement-unmarked` | — |
| addendum-owed | `owed-flat` | — |
