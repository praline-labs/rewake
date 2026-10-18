# The workflow suite's cases

What the end-to-end cases in `test/workflow` claim, case by case: the sessions each one
starts, what the fixture plays for it and what it must see. How to write a scenario, a
fixture or a column, and what every case shares, is in [testing.md](testing.md#extending-the-suite); each scenario's invariant as
first argued is in [check-runner-scenarios.md](archive/check-runner-scenarios.md).

The Codex column and its cases — `codex-steered`, `codex-compact-hold`,
`codex-tui-later-shape`, `codex-worktree`, `codex-grant-dir`, `codex-grant-forgery`,
`codex-grant-resume` and `readiness` — left with the Codex adapter in
S8, October 8, 2026; they are kept with its code in
[archive/1.x/codex](../archive/1.x/codex/README.md), as they last ran. The fixes of S8's
review brought the neutral part of three of them back on the gate column:
`fixture-worktree` below, and `fixture-grant-forgery` and `fixture-grant-dir`, which
left with grants on October 18, 2026.

## The retired Claude Code column

The Claude Code column and its cases — `claude-telemetry`, `claude-interrupted`,
`claude-steered`, `claude-inbound`, `claude-grant-dir`, `claude-grant-resume` and
`claude-worktree`, and the column's share of every case run in all
columns — retired in S9, October 11, 2026, when Claude Code's hooks, plugin and
telemetry left; the column returns with stage 4, when Claude Code gets a turn boundary
again. Their code is in the history at `98acc3b` (`test/workflow/claude*_test.go`), and
how they ran is in [testing-plugin.md](archive/1.x/testing-plugin.md) and the S9 entry
of the roadmap. What they held of the code that stays is held now by the fixture's cases
— `fixture-steered`, `thread-changed` and `stopped-routing` below took the neutral part
of `claude-steered`, `thread-changed` and `claude-interrupted` — and, for the code only
Claude Code runs, by unit tests in `internal/adapter/claude`, `internal/host` and
`internal/host/delivery`. The gap until stage 4: no workflow case runs Claude Code's inbound lane — its gate,
its receipts, its late word — end to end against a session; the unit tests hold each link, and the
live check of S9 woke a Claude Code main through it.

## Claude Code telemetry budgets

Retired with the Claude Code column in S9, October 11, 2026, with the telemetry they
measured: the budgets, their measurement and their controls are in
[claude-telemetry.md](archive/1.x/claude-telemetry.md) and in the history at `98acc3b`.

## Launch, reports and what a main waits on

`wrapped-launch` runs in every column, twice each, in two rooms: a plain launch and one
with `--command ./<harness>-worker`, a stand-in wrapper that exports a marker and execs the
fixture. Every fixture process records its arguments and whether it saw the marker, and
the case requires the wrapped processes to get argument for argument what the plain
launch gets, all to see the marker, and none of the plain ones to. With the relative path left relative, it went red on
three of its four observations (September 23, 2026).

`pending-report` runs in every column with three sessions. A worker reads a task and,
in that turn, runs `rewake pending` before ending it; the sender must read a `pending`
message about the task first — the mark's line, then the turn's own text — and the
worker's awaiting record must still be there.
A third session then sends the worker a task of its own, and that turn end — with no
mark — must be the `finished` report settling the first task.

`pending-confirm` runs in the fixture column with four sessions; the Claude Code column
ran it through the Stop hook until S9. A worker ends its first turn about a task pending;
woken by a second session's mail, its next turn ends with no mark, and rewake must hold
it exactly once, quoting the pending line. The worker's program asks the core through
the adapter's `Confirm` before an end closes and goes on only with the reason it is
given; its model, asked the first time, runs `rewake pending` with a second line: the
sender must read an interim message with that line, then the held answer, then the
continuation, and the task must stay owed. Woken by a third session, the worker ends
unmarked again, is held once more about the second line, and only ends the turn: the
report must be the held answer and then the continuation, and it must settle the task
(September 26, 2026; on the fixture since October 7, 2026).

`turn-reasons` runs in the fixture column, with five sessions: the path it holds — a
turn's outcome and text in the program's end frame, the adapter's neutral completion and
the wrapper's confirmation — is the fixture's own. A worker whose turns fail holds a task from main and one
from a peer; each sender must read an `error` about its own task carrying the turn's
reason, and a session that waits on nothing must receive nothing from it. A second
worker's first turn is interrupted while it holds a task from main, which must read it
`stopped` with that turn's reason. The reasons are set by `RW_SHIM_END_REASON` and differ
from anything the mail carried (October 7, 2026).

`fixture-turn-start` runs two tasks through one fixture worker and requires the program's
own time of each turn's start in the mailbox, later for the later turn: the program times
its start before the turn does anything and sends it with the start, as a harness that
knows when its turn began does, and the host records it where a pending mark of an
earlier turn whose end was lost finds its turn over. The cases that called rewake through
the fixture's tool transport left with the mail tool on October 18, 2026
([testing-tools.md](archive/testing-tools.md)).

`owed-reread` runs in every column with a main and a worker. The worker reads a
multi-line task and, in the same turn, runs `rewake inbox --owed` in both forms, as a
session re-reading its task after a compaction would. Both must print that task in full;
the machine form carries the id the worker read it under, and the text form opens with
`Rewake: owed a report for 1 message:` and the sender's header and leaves the id out.
The turn end must then report it once and
settle it: asking changed nothing.

`awaited-view` runs in every column with a main and two workers. main runs its own
commands when the scenario asks (the fixture's request directory): it sends one task to
each worker. One reports at once; the other runs `rewake pending` in its turn, so its
task stays owed. main's `rewake inbox --awaited`, in both forms, must then list exactly
that second task, by id, as `pending` with the interim's text, under `to <worker>`, and
leave the first out. A note from main wakes the second worker, whose next turn end
reports; the view must then be `Rewake: nobody owes you a report.`

## A worktree for a Codex launch

`codex-worktree` left with the Codex column in S8, October 8, 2026, and is kept as it
last ran in [archive/1.x/codex](../archive/1.x/codex/README.md), where the records that
cite it read it. What it held of the core's worktree code runs again on the gate column
as `fixture-worktree`.

## A worktree on the gate column

`fixture-worktree` runs on the fixture column, which takes rewake's `--worktree` for the
purpose. A worker is launched from `src/nested` of a fresh repository with
`--worktree=probe`, the worktree directory left at its default under the case's home,
and each half of the fixture's program writes the directory it started in
(`RW_SHIM_CWD_FILE`). The checkout must be one of the source's HEAD on a new branch
`probe`, the source still on `main`, under that directory, and the session's record, the
terminal and the session half must all work in its `src/nested`; a task a main sends must
be delivered there; `rewake worktree rm` must refuse the checkout while the worker runs
and say so. While the worker runs, a commit made in the checkout is landed — `main` moves
to it, its file appears in the source, the checkout stays on `probe` and the worker keeps
running — and after a second commit `finish` must refuse, naming the worker, and move
nothing. After the worker has ended, rm must still refuse while a visitor session
started in the checkout runs. Three spare checkouts, launched and ended for the purpose,
show what else rm keeps: one holding a `.env` the repository ignores (and `--force`
removes it), one whose commit no ref holds once its checkout is detached and every ref
holding it is deleted (and removed once they are back), and one whose directory was moved
away, where the refusal points at `git worktree repair` and `--force` takes out its record
and git's entry. Once nothing holds the first checkout, `finish` must land the second
commit and remove the checkout, its record, git's own entry and the branch. That the removal of a missing checkout takes out its own entry and no
other, where `git worktree prune` would take every missing checkout's, is held by
`internal/host/worktree` (`TestAMissingCheckoutIsRemovedAlone`).

## A worktree for a Claude Code launch

`claude-worktree` retired with the Claude Code column in S9, October 11, 2026. It held
the adapter's part, where its launch works, which
`TestALaunchWorksInTheDirectoryItWasStartedIn` in `internal/adapter/claude` holds now;
the refusals of `-w`, `--tmux` and the continuations are unit tests there and in
`internal/cli`. `fixture-worktree` above holds the core's part.

## Actions on a sent message

`withdraw-after-notice`, `edit-after-notice` and `addendum-owed` run in every column with a
main and a worker whose read waits at a gate (`RW_SHIM_READ_GATE`): its turn has started
on the notice and it has not read yet. main, through its request directory, then
withdraws the task, replaces it, or adds to it, each by the short id its send printed.
The worker must find a withdrawn note and never the task's text, a recall from main
marked as the recall of the task, shown in a notice on a line of its own that begins
`Do not act on task <short id>`, with nothing reported and nothing said undelivered; the
old id replaced by the new, the new text only, a notice that shows `Replaces <short id>
(withdrawn): ` before the new text, no recall, and one report on the replacement; and
`--owed` with the addendum under its task, then one report settling both. What a
notice showed is read from the worker's own record of its deliveries, on every column.
`withdraw-mid-turn` runs the withdrawal on every column that has a turn in progress to
deliver into — the fixture's — with the worker's first turn held open (`RW_SHIM_HOLD_TURN`; the read gate alone leaves no turn open to
steer into): the recall must be steered into the turn the task started, as a member
whose `recalls` names the task.

## Conversations

`thread-changed` runs on the fixture column with two workers and a sender for each; it
ran on the Claude Code column until S9, through the hooks' `session_id`. One worker's
program starts a new conversation once its task has arrived, as a person's `/clear`
does, tells its adapter, and works the task there; the other works its task where it
landed. The report from the first must carry `threadChanged` and still arrive once and
settle its task; the report from the worker that stayed must carry none.

`claude-inbound`, the inbound gate on Claude Code, retired with its column in S9; unit
tests in `internal/adapter/claude` hold the code that stays.

## A stop, and steering a session

`stopped-routing` runs on every column. The fixture harness's program takes the switch
`RW_SHIM_INTERRUPT_FIRST_TURN` and ends its first turn interrupted. A main, running its
own commands, sends one worker a task and another a `--notify`, which owes nothing; both
workers and main have their first turn interrupted. main must read `stopped` about the
task and list it as `stopped` in `--awaited`; the notified worker's stop, once its
receipt shows it was acted on, must leave nothing in main's inbox, read or not, and the
worker must read `idle`; main's own stop must leave nothing in its own inbox, unread
mail included, where a report main gives itself lies unannounced.

`fixture-steered` is `rewake compact` and `rewake interrupt` on the gate column, where
the fixture is the one harness offering Control from S9 until stage 4. A main, running its own
commands, steers three workers, and each request travels the command, the control
directory, the program serving it, the session carrying it out, and back. calm works its
task to the end and is idle: a compaction with a focus must answer `started` while it
still runs, before the telemetry counts it; its end must reach main as a notify with the
token counts and the compaction's number, and main, which asked, must get no notice of
it besides; an interrupt must be refused as `no turn running`; calm killed in the middle
of a second compaction must still leave main a letter, from its own wrapper's record.
busy is held in its first turn: a compaction must be refused as `in a turn` with the turn
going on; an interrupt must end it, and main must read `stopped` naming itself, as its
`--awaited` does. bare serves no control, so its run never makes Control live: every
request must be refused, exit 1, before anything is asked (S11). A worker asking is a
wrong call. What it does not prove: that a real harness carries these calls out
as the program does.

## The live set

`fixture-live` (S11, October 11, 2026) runs fixture sessions under one main, each
withholding what its program's switch says, and judges what main's `rewake list --json`
and the commands show. full proves everything: its record's live set names every
capability. wakeless (`RW_SHIM_NO_WAKE`) has no Wake live: a note and a task to it are
each accepted at once, exit 3, by send's own answer that no notice can reach it, and once
its server has tried them each waits readable in its inbox with its stored status
pending. dropper (`RW_SHIM_DROP_AFTER=1`) closes its connection after its first turn: its
whole live set is withdrawn at once, and a second task to it fails, exit 1, with nothing
written. stuck (`RW_SHIM_NO_BOUNDARY`) is a worker whose run proves no turn boundary: its
launch is refused, exit 1, and leaves no session. The steering refusal without a live
Control is `fixture-steered`'s bare. What it does not prove: that a real harness proves or
loses a capability as the program does.

The tool's channel, its sessions and `fixture-decide-sandboxed`, a decision from a shell
that cannot reach its wrapper, left with the mail tool on October 18, 2026.
