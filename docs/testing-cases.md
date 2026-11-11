---
description: What each of the 38 workflow scenarios in test/workflow claims — the sessions it starts, what the fixture plays and what it must see — and what none of them proves.
---

# The workflow suite's cases

What the end-to-end cases in `test/workflow` claim, case by case: the sessions each one
starts, what the fixture plays for it and what it must see. How to write a scenario, a fixture
or a column, and what every case shares, is in [testing.md](testing.md#extending-the-suite).
Every case runs on the fixture column. None proves that a model read its mail and decided to act
on it, or that a real harness carries a call out as the fixture's program does: a green run is
never evidence of understanding. Claude Code's own code — its inbound lane, gate, receipts and
late word, its launch in the directory it was started in, the refusals of `-w`, `--tmux` and
the continuations — is held by unit tests in `internal/adapter/claude`, `internal/host` and
`internal/host/delivery`, and checked live.

## Delivery

`task-report` runs two sessions: a task reaches an idle session, which consumes it, and the
report that comes back corresponds to exactly that message from exactly that session. The
sender sends with its own rewake and reads its own mailbox, so the half of the claim that lives
on the sender's side is checked too.

`question`: one running session sends `rewake send <name> "..." --question` and waits; the
session asked reads it and ends its turn, and the waiting command prints that end as the
answer from the session it asked. The question is delivered to the recipient once and read
once, and the answer reaches the asker only through the command: nothing but the recipient's
heads-up is delivered to it or read, and its inbox holds nothing unread. The asker runs the
command itself, through its request directory.

`batch-arrival`: two letters that arrive inside one collection window are announced as one
group of fixed membership whose notice previews the latest member, and a third letter after the
window belongs to another group. Reads stay per message — an overview consumes nothing, one read
takes one letter — and what was announced is never announced again as new mail, and reminded of
once at most ([delivery.md](delivery.md#the-notice)): the recipient leaves the
last member of a group unread until the next delivery, and the third letter comes late enough for
a replay to have shown in between. Three heads-ups 600 ms apart must not wake the recipient one
by one.

`mid-turn`: a letter delivered to a session already working reaches it without waiting for the
turn to end, and the work already running still reports its own outcome, with no second outcome
for the arrival. The session holds its turn open and writes down when it waits; the sender sends
the second letter once the room's telemetry says the recipient is working, and the scenario reads
the recipient's own record of whether the letter was steered into the open turn.

`unread-reminder`: a task steered into a turn that ends without reading it brings one reminder,
naming that task alone; the turn it wakes reads nothing either, and brings no second one; once
the worker reads again — a note wakes it — the task's report reaches main. The worker holds its
turns open and, while a file the scenario writes exists, reads no mail (`RW_SHIM_LEAVE_UNREAD`);
the reminder is told from other notices by its words.

`pinned-build`: a room keeps its build across an upgrade. main and a worker start from an install
of the case's own, which is then replaced by another build — the same program with bytes
appended, so another id; the worker's `rewake send`, found on its PATH, still works and the
room's build is the first install's; a person's `rewake list` of the new build is handed to the
room's copy, its stderr saying so; once the sessions end, a new launch takes the new build and
the room's copy becomes it.

`fixture-switch`: a running room switches to another build. A fixture main, launched on a
terminal as a person does and so behind its host with the shell as the host's child, and a worker
start from an install of the case's own, which is then replaced by the same tree linked with
another build time; the person's `rewake restart rewake` switches the room: main's host keeps its
pid and runs the new image, the shell keeps its channel, is not replaced, and attaches to main's
stream again, both wrappers keep their pid, epoch and harness and run the new image, the room's
build and copy are the new build, and a task sent across the switch gets exactly one report. With
a space open, a second switch back goes through: main's host, the space host and its worker keep
their pid and run the first build, the space stays open with the same screen, no line of an
adopting wrapper added, and a task gets one report. The shell is the test binary speaking the
host's channel, which attaches again a second after main's stream ends, as the real one does. The
committed switch's socket, which its leader cannot remove once its lease is revoked, is the
scenario's to clear after checking nothing listens.

`fixture-main-switch`: a switch main asks for, and a window closed during one. Main, on a terminal
behind its host, holds each turn open four seconds; from the turn the worker's arrival began it
runs `rewake restart rewake`, which answers at once that the switch is deferred while the room
keeps its build and main's turn is open; once main is idle the switch goes through, main's host and
wrapper keep their pid and epoch and run the new build, and main reads the outcome as a note.
`rewake restart shell`, asked of the adopted host, then answers generation 2. Last, with main idle,
the person switches back and closes the window the moment main's host is seen parked — its
handover, in `/proc/<pid>/environ`, names the park — so the shell's `lost` waits in its channel for
the image that takes the run on: that image ends main's host, main and the shell, the switch has
committed and holds the room no more, and the worker beside main runs on.

`fixture-switch-errored`: a switch right after a turn ended in an error, as one does on an API
error, a limit or the person's Esc. A worker whose turns all fail takes a task in a turn that reads
nothing: the task stays announced and unread, and no finished end looks at it. The person's
`rewake restart rewake` goes through within 45 s, and the worker's wrapper keeps its pid and epoch
and runs the new build. Found live with a pi main without a model, refused as busy for the whole
of the switch's bound; the reminder carried across is the delivery server's own test.

`fixture-wrapper-killed`: a fixture wrapper killed outright takes its session half with it within
three seconds. The fixture's program watches its parent's pid, since the backend asks for no
parent-death signal, which the exec of a wrapper handing its run over would fire. The killed
run's socket and record, which nothing removes, are the scenario's to clear.

## Launch, reports and what a main waits on

`wrapped-launch` runs twice, in two rooms: a plain launch and one with `--command
./<harness>-worker`, a stand-in wrapper that exports a marker and execs the fixture. Every
fixture process records its arguments and whether it saw the marker, and the case requires the
wrapped processes to get argument for argument what the plain launch gets, all to see the
marker, and none of the plain ones to.

`pending-report` runs three sessions. A worker reads a task and, in that turn, runs `rewake
pending` before ending it; the sender must read a `pending` message about the task first — the
mark's line, then the turn's own text — and the worker's awaiting record must still be there. A
third session then sends the worker a task of its own, and that turn end — with no mark — must
be the `finished` report settling the first task.

`pending-confirm` runs four sessions. A worker ends its first turn about a task pending; woken by
a second session's mail, its next turn ends with no mark, and rewake must hold it exactly once,
quoting the pending line. The worker's program asks the core through the adapter's `Confirm`
before an end closes and goes on only with the reason it is given; its model, asked the first
time, runs `rewake pending` with a second line: the sender must read an interim message with
that line, then the held answer, then the continuation, and the task must stay owed. Woken by a
third session, the worker ends unmarked again, is held once more about the second line, and only
ends the turn: the report must be the held answer and then the continuation, and it must settle
the task.

`stalled-pending` runs two sessions on a binary of its own, built with the bound of a stalled
pending cut to twelve seconds. A worker reads main's task and ends that turn pending, then stays
idle with nothing to wake it. Before the bound, main's `rewake list` must show the worker
`pending, idle`, and `rewake inbox --awaited` the task as `pending, idle <1m` with the pending
line. Past the bound the worker must read exactly one notify of its own quoting that line and
naming main, and the turn it starts, ending unmarked, must be the `finished` report that settles
the task, with no second reminder. No unit test holds this crossing: the fixture's activity
reaching the snapshot, the wrapper's server judging it, the letter announced and the woken turn's
report.

`turn-reasons` runs five sessions: the path it holds — a turn's outcome and text in the
program's end frame, the adapter's neutral completion and the wrapper's confirmation — is the
fixture's own. A worker whose turns fail holds a task from main and one from a peer; each sender
must read an `error` about its own task carrying the turn's reason, and a session that waits on
nothing must receive nothing from it. A second worker's first turn is interrupted while it holds
a task from main, which must read it `stopped` with that turn's reason. The reasons are set by
`RW_SHIM_END_REASON` and differ from anything the mail carried.

`fixture-turn-start` runs two tasks through one worker and requires the program's own time of
each turn's start in the mailbox, later for the later turn: the program times its start before
the turn does anything and sends it with the start, as a harness that knows when its turn began
does, and the host records it where a pending mark of an earlier turn whose end was lost finds
its turn over.

`owed-reread` runs a main and a worker. The worker reads a multi-line task and, in the same
turn, runs `rewake inbox --owed` in both forms, as a session re-reading its task after a
compaction would. Both must print that task in full; the machine form carries the id the worker
read it under, and the text form opens with `Rewake: owed a report for 1 message:` and the
sender's header and leaves the id out. The turn end must then report it once and settle it:
asking changed nothing.

`awaited-view` runs a main and two workers. main runs its own commands when the scenario asks
(the fixture's request directory): it sends one task to each worker. One reports at once; the
other runs `rewake pending` in its turn, so its task stays owed. main's `rewake inbox
--awaited`, in both forms, must then list exactly that second task, by id, as `pending` with the
interim's text — `pending, idle <1m` in the text form, the worker idle under its mark — under `to
<worker>`, and leave the first out. A note from main wakes the second
worker, whose next turn end reports; the view must then be `Rewake: nobody owes you a report.`

`brief` runs a main, a worker and a reviewer, each running its commands when the scenario asks. main makes
a brief of two items and sends the worker a task with `--brief`; the worker's inbox must print
the brief's object with the task, its title and its items. The worker's own `brief show` must
name edition 1, its tick of item 1 by that edition must answer the item's text, and its comment
must be accepted. A third session, a reviewer, is given the brief with `--brief --review`: its
tick must be refused, exit 1, saying ticking is the worker's, and its comment accepted. main's
`brief show --json` must then have item 1 done, item 2 not, and the two comments, the worker's with
capacity `work` and the reviewer's with `review` ([briefs.md](briefs.md)).

`brief-skill` runs a main and a worker. Two skills are written into the fixture's skill folder in
the case's home, the first linking `../<second>/SKILL.md`; main makes a brief whose `Skills:` line
names the first and sends it to the worker with `--brief`. The worker's letter must give the path
of the copied `SKILL.md`, under the room's state, and the file there must hold the skill; next it
must give the second's copy, beside the first, marked `broughtBy` the first; main's `brief close`
must then leave no file at the first's path ([briefs.md](briefs.md#skills)).

## A worktree

`fixture-worktree` takes rewake's `--worktree`. A worker is launched from `src/nested` of a
fresh repository with `--worktree=probe`, the worktree directory left at its default under the
case's home, and each half of the fixture's program writes the directory it started in
(`RW_SHIM_CWD_FILE`). The checkout must be one of the source's HEAD on a new branch `probe`, the
source still on `main`, under that directory, and the session's record, the terminal and the
session half must all work in its `src/nested`; a task a main sends must be delivered there;
`rewake worktree rm` must refuse the checkout while the worker runs and say so. While the worker
runs, a commit made in the checkout is landed — `main` moves to it, its file appears in the
source, the checkout stays on `probe` and the worker keeps running — and after a second commit
`finish` must refuse, naming the worker, and move nothing. After the worker has ended, rm must
still refuse while a visitor session started in the checkout runs. Three spare checkouts show
what else rm keeps: one holding a `.env` the repository ignores (and `--force` removes it), one
whose commit no ref holds once its checkout is detached and every ref holding it is deleted
(and removed once they are back), and one whose directory was moved away, where the refusal
points at `git worktree repair` and `--force` takes out its record and git's entry. Once nothing
holds the first checkout, `finish` must land the second commit and remove the checkout, its
record, git's own entry and the branch. That the removal of a missing checkout takes out its own
entry and no other is held by `internal/host/worktree` (`TestAMissingCheckoutIsRemovedAlone`).

## Actions on a sent message

`withdraw-after-notice`, `edit-after-notice` and `addendum-owed` run a main and a worker whose
read waits at a gate (`RW_SHIM_READ_GATE`): its turn has started on the notice and it has not
read yet. main, through its request directory, then withdraws the task, replaces it, or adds to
it, each by the short id its send printed. The worker must find a withdrawn note and never the
task's text, a recall from main marked as the recall of the task, shown in a notice on a line of
its own that begins `Do not act on task <short id>`, with nothing reported and nothing said
undelivered; the old id replaced by the new, the new text only, a notice that shows `Replaces
<short id> (withdrawn): ` before the new text, no recall, and one report on the replacement; and
`--owed` with the addendum under its task, then one report settling both. What a notice showed
is read from the worker's own record of its deliveries. `withdraw-mid-turn` runs the withdrawal
with the worker's first turn held open (`RW_SHIM_HOLD_TURN`; the read gate alone leaves no turn
open to steer into): the recall must be steered into the turn the task started, as a member whose
`recalls` names the task.

## Conversations

`thread-changed` runs two workers and a sender for each. One worker's program starts a new
conversation once its task has arrived, as a person's `/clear` does, tells its adapter, and works
the task there; the other works its task where it landed. The report from the first must carry
`threadChanged` and still arrive once and settle its task; the report from the worker that stayed
must carry none.

## A stop, and steering a session

`stopped-routing`: the fixture's program takes the switch `RW_SHIM_INTERRUPT_FIRST_TURN` and ends
its first turn interrupted. A main, running its own commands, sends one worker a task and another
a `--notify`, which owes nothing; both workers and main have their first turn interrupted. main
must read `stopped` about the task and list it as `stopped` in `--awaited`; the notified worker's
stop, once its receipt shows it was acted on, must leave nothing in main's inbox, read or not, and
the worker must read `idle`; main's own stop must leave nothing in its own inbox, unread mail
included, where a report main gives itself lies unannounced.

`fixture-steered` is `rewake compact` and `rewake interrupt`. A main, running its own commands,
steers three workers, and each request travels the command, the control directory, the program
serving it, the session carrying it out, and back. calm works its task to the end and is idle: a
compaction with a focus must answer `started` while it still runs, before the telemetry counts
it; its end must reach main as a notify with the token counts and the compaction's number, and
main, which asked, must get no notice of it besides; an interrupt must be refused as `no turn
running`; calm killed in the middle of a second compaction must still leave main a letter, from
its own wrapper's record. busy is held in its first turn: a compaction must be refused as `in a
turn` with the turn going on; an interrupt must end it, and main must read `stopped` naming
itself, as its `--awaited` does. bare serves no control, so its run never makes Control live:
every request must be refused, exit 1, before anything is asked. A worker asking is a wrong call.

## The live set

`fixture-live` runs sessions under one main, each withholding what its program's switch says,
and judges what main's `rewake list --json` and the commands show. full proves everything: its
record's live set names every capability. wakeless (`RW_SHIM_NO_WAKE`) has no Wake live: a note
and a task to it are each accepted at once, exit 3, by send's own answer that no notice can reach
it, and once its server has tried them each waits readable in its inbox with its stored status
pending. dropper (`RW_SHIM_DROP_AFTER=1`) closes its connection after its first turn: its whole
live set is withdrawn at once, and a second task to it fails, exit 1, with nothing written. stuck
(`RW_SHIM_NO_BOUNDARY`) is a worker whose run proves no turn boundary: its launch is refused,
exit 1, and leaves no session. The steering refusal without a live Control is `fixture-steered`'s
bare.

## Spaces

`fixture-space` is a space on its main path: main opens one with `rewake space open`, the
worker it starts joins main's room as an ordinary session, takes a task and reports it, and once
main's run ends the space host stops the worker and closes the record from what it observed. The
space host is a detached rewake process, the worker an ordinary launch of the fixture in its
terminal; nothing of it is the fixture's but the offer of spaces a main run proves.

`fixture-space-close` is main closing a worker it raised: main opens a space, hands its worker a
task the worker leaves pending, and runs `rewake space close`; the space host ends the worker,
the record closes with main's reason, and main's `rewake inbox --awaited` reads the task as lost,
its worker ended — while main itself runs on.

`fixture-space-mode` is a worker raised in a runtime work mode: a project's
`.rewake/aliases.toml` names a template whose `start_mode` is the project's own
`.rewake/modes/survey.md`; main, working in that project, opens the space without `--work-mode`,
and the worker's briefing carries that one mode section. The mode crosses from main's
`space open` through the space host's environment into the worker's launch, which only a run of
all three shows.

`fixture-space-worktree` is a worker opened from a template with `worktree = true`: main opens
it in a repository, the worker works in a checkout of its own and asks `rewake worktree status`,
which names that checkout and forecasts its removal; main's `space close` then removes the clean
checkout and its branch, as forecast.

`fixture-space-worktree-enter` is a worker opened from a template with `enter_worktree = true`:
it starts in main's repository with no checkout, enters one mid-session with `rewake worktree
enter`, commits there, and main's `space close` keeps that checkout, saying why — the checkout
the worker claimed after its launch is the space's own.
