# The workflow suite's cases

What the end-to-end cases in `test/workflow` claim, case by case: the sessions each one
starts, what the fixture plays for it, what it must see, and the controls that prove it
can fail. How to write a scenario, a control, a fixture or a column, and what every case
shares, is in [testing.md](testing.md#extending-the-suite); each scenario's invariant as
first argued is in [check-runner-scenarios.md](archive/check-runner-scenarios.md).

The Codex column and its cases — `codex-steered`, `codex-compact-hold`,
`codex-tui-later-shape`, `codex-worktree`, `codex-grant-dir`, `codex-grant-forgery`,
`codex-grant-resume` and `readiness` with their controls — left with the Codex adapter in
S8, October 8, 2026; they are kept with its code in
[archive/1.x/codex](../archive/1.x/codex/README.md), as they last ran. The fixes of S8's
review brought the neutral part of three of them back on the gate column:
`fixture-worktree`, `fixture-grant-forgery` and `fixture-grant-dir`, below.

## The retired Claude Code column

The Claude Code column and its cases — `claude-telemetry`, `claude-interrupted`,
`claude-steered`, `claude-inbound`, `claude-grant-dir`, `claude-grant-resume` and
`claude-worktree`, with their controls, and the column's share of every case run in all
columns — retired in S9, October 11, 2026, when Claude Code's hooks, plugin and
telemetry left; the column returns with stage 4, when Claude Code gets a turn boundary
again. Their code is in the history at `98acc3b` (`test/workflow/claude*_test.go`), and
how they ran is in [testing-plugin.md](archive/1.x/testing-plugin.md) and the S9 entry
of the roadmap. What they held of the code that stays is held now by the fixture's cases
— `fixture-steered`, `thread-changed` and `stopped-routing` below took the neutral part
of `claude-steered`, `thread-changed` and `claude-interrupted` — and, for the code only
Claude Code runs, by unit tests, each killing the column's mutant of it:

| Retired mutant | Kept code | The unit test that kills it |
|---|---|---|
| `ungated` | `internal/host/delivery/serve.go` | `internal/host/delivery` `TestTheFirstNoticeWaitsForTheOpening` |
| `expiry-unannounced` | `internal/host/delivery/held.go` | `internal/host/delivery` `TestAnExpiredTaskIsFailedAndItsSenderTold` |
| `held-as-delivered` | `internal/adapter/claude/receipt.go` | `internal/adapter/claude` `TestLaneHeldThenReleased`, `TestParseReceipt` |
| `refusal-as-delivered` | `internal/adapter/claude/receipt.go` | `internal/adapter/claude` `TestLaneRefusalFails`, `TestParseReceipt` |
| `late-word-dropped` | `internal/adapter/claude/lane.go` | `internal/adapter/claude` `TestLaneLateWordReachesTheServer` |
| `claude-grant-unshielded` | `internal/core/grant/rules.go` | `internal/core/grant` `TestAShieldedDirectoryIsGrantedOnlyByName` |
| `claude-resume-not-given` | `internal/host/wrap.go` | `internal/host` `TestAResumedLaunchStartsTheHarnessWithTheGrantsConfirmedAgain` |
| `claude-resume-closed-held` | `internal/host/grants.go` | `internal/host` `TestMainsAuthorityAsksTheMailboxWhetherATaskIsOpen` |
| `claude-worktree-parent` | `internal/adapter/claude/worktree.go` | `internal/adapter/claude` `TestALaunchWorksInTheDirectoryItWasStartedIn` |
| `delivery-unpinned`, the column's target | `internal/host/wrap.go` (`thread = source.Thread`) | `internal/host` `TestTheWrapperPinsDeliveryToTheObservedThread` |

`delivery-unpinned` of `thread-changed` mutated, on the Claude Code column, the
wrapper's branch that pins a delivery to the conversation an observer reports — code
only tests reach since S9, as no adapter sets an observer; the fixture's mutant of that
name edits `internal/host/delivery/prepare.go` instead, and both are kept. The other mutants of the column broke code that left with it: the plugin's module, the
telemetry collector and tap, the permission hook and the grant keeper. The gap until
stage 4: no workflow case runs Claude Code's inbound lane — its gate, its receipts, its
late word — end to end against a session; the unit tests above hold each link, and the
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
launch gets, all to see the marker, and none of the plain ones to. It has no control in the suite; with the relative path left relative, it went red on
three of its four observations (September 23, 2026).

`pending-report` runs in every column with three sessions. A worker reads a task and,
in that turn, runs `rewake pending` before ending it; the sender must read a `pending`
message about the task first — the mark's line, then the turn's own text — and the
worker's awaiting record must still be there.
A third session then sends the worker a task of its own, and that turn end — with no
mark — must be the `finished` report settling the first task. Its three mutants, each in
every column, ignore the mark, let the interim turn end settle the task, and drop the
turn's text from the interim message; each names
what it must break and requires the rest to hold.

`pending-confirm` runs in the fixture column with four sessions; the Claude Code column
ran it through the Stop hook until S9. A worker ends its first turn about a task pending;
woken by a second session's mail, its next turn ends with no mark, and rewake must hold
it exactly once, quoting the pending line. The worker's program asks the core through
the adapter's `Confirm` before an end closes and goes on only with the reason it is
given; its model, asked the first time, runs `rewake pending` with a second line: the
sender must read an interim message with that line, then the held answer, then the
continuation, and the task must stay owed. Woken by a third session, the worker ends
unmarked again, is held once more about the second line, and only ends the turn: the
report must be the held answer and then the continuation, and it must settle the task.
Its two mutants never hold, and publish the continuation without the held answer
(September 26, 2026; on the fixture since October 7, 2026).

`turn-reasons` runs in the fixture column, with five sessions: the path it holds — a
turn's outcome and text in the program's end frame, the adapter's neutral completion and
the wrapper's confirmation — is the fixture's own. A worker whose turns fail holds a task from main and one
from a peer; each sender must read an `error` about its own task carrying the turn's
reason, and a session that waits on nothing must receive nothing from it. A second
worker's first turn is interrupted while it holds a task from main, which must read it
`stopped` with that turn's reason. The reasons are set by `RW_SHIM_END_REASON` and differ
from anything the mail carried. Its three mutants drop the text on the way to the
completion, take a failure for a finished turn, and take an interruption for one; each
names what it must break and requires the rest to hold (October 7, 2026).

The cases that make rewake's calls through the fixture program's tool transport —
`tool-inbox`, `tool-send`, `tool-pending`, `tool-whoami`, `tool-retry` and `tool-list` —
and `fixture-turn-start`, the program's timed turn start, are in
[testing-tools.md](testing-tools.md), with the read's schedule and the crosswise check.

`owed-reread` runs in every column with a main and a worker. The worker reads a
multi-line task and, in the same turn, runs `rewake inbox --owed` in both forms, as a
session re-reading its task after a compaction would. Both must print that task in full;
the machine form carries the id the worker read it under, and the text form opens with
`Rewake: owed a report for 1 message:` and the sender's header and leaves the id out.
The turn end must then report it once and
settle it: asking changed nothing. Its mutant, an `--owed` that finds nothing, must break
the first observation and hold the second.

`awaited-view` runs in every column with a main and two workers. main runs its own
commands when the scenario asks (the fixture's request directory): it sends one task to
each worker. One reports at once; the other runs `rewake pending` in its turn, so its
task stays owed. main's `rewake inbox --awaited`, in both forms, must then list exactly
that second task, by id, as `pending` with the interim's text, under `to <worker>`, and
leave the first out. A note from main wakes the second worker, whose next turn end
reports; the view must then be `Rewake: nobody owes you a report.` Its two mutants run on
the gate column only — the view reads files the same way whichever harness wrote them:
one blind to interim reports breaks the first observation alone, one that never lets a
task go breaks both.

## A worktree for a Codex launch

`codex-worktree` left with the Codex column in S8, October 8, 2026, and is kept as it
last ran in [archive/1.x/codex](../archive/1.x/codex/README.md), where the records that
cite it read it. Every observation and all eleven mutants held the core's worktree code,
and they run again on the gate column as `fixture-worktree`.

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
commit and remove the checkout, its record, git's own entry and the branch. Its eleven
mutants each name what they break and require the rest to hold: a launch that never
enters its checkout; a removal that takes no session for a running one, which also leaves
nothing to visit and lets finish go ahead; one that sees only the session the checkout
was made for, which removes it from under the visitor; one that deletes the directory
behind git's back, whose entries stay; one blind to ignored files; one that asks whether
a commit is held only after the HEAD moved; one that removes a moved checkout without
`--force`; a checkout made detached, which nothing can land and finish will not take; a
land that makes a merge commit; a finish that asks nothing first; and one that leaves the
branch behind. That the removal of a missing checkout takes out its own entry and no
other, where `git worktree prune` would take every missing checkout's, is held by
`internal/host/worktree` (`TestAMissingCheckoutIsRemovedAlone`).

## A worktree for a Claude Code launch

`claude-worktree` retired with the Claude Code column in S9, October 11, 2026. It held
the adapter's part, where its launch works, and its mutant `claude-worktree-parent` is
killed now by `TestALaunchWorksInTheDirectoryItWasStartedIn` in `internal/adapter/claude`;
the refusals of `-w`, `--tmux` and the continuations are unit tests there and in
`internal/cli`. `fixture-worktree` above holds the core's part.

## A directory granted with a task

`fixture-grant-dir` holds the core's part of a grant on the gate column, with a fixture
main and two fixture write workers. The fixture takes `--grant-dir` and a fixture main
can grant; its adapter passes a letter's granted directories with the delivery, and its
program records each letter that came with them. It applies nothing: waiting for an idle
session, adding the directory and taking it out are the Permissions capability's,
designed in stage 6; Claude Code's hooks held them until S9.
Main sends the first worker, which holds its first turn open, a task with `--grant-dir`
on a directory in the user's cache: the program must be offered that directory with that
letter alone, and a plain task steered into the same turn must offer none. Asked from the
test process, main's wrapper must confirm the grant while the task is open and refuse it
once the task is reported on. The second worker's program refuses every reservation
until the case lets it (`RW_SHIM_HOLD_RESERVE`); main sends it a task granting another
directory, which must wait `pending`; the directory is removed and the program let go,
and the letter must fail `its grant does not pass: … is gone` without the program being
offered it, and main must read the note that it was not delivered. Its mutants deliver
without checking the directories again (`grant-not-rechecked`, which breaks the recheck
alone), fail a grant without telling its sender (`grant-refusal-untold`, which breaks the
recheck alone), write the letter without registering its grant (`grant-unregistered`,
which breaks all three: no wrapper confirms it), and keep a reported task's grant on
main's wrapper (`grant-held-after-report`, which breaks the lifetime alone). The three
mutants of the archived `codex-grant-dir` changed the Codex adapter's sandbox roots and
stay with it.

`fixture-grant-forgery` is a fixture write worker trying to grant itself a directory in
the name of a running fixture main, five ways. Its own process runs `rewake send` with
main's `REWAKE_SESSION` and `REWAKE_EPOCH`, and so does a process it detached through
`setsid`: both must exit 1 and leave no letter. The test process, outside main's tree,
writes a registration straight to main's address, as a forger's own client would,
skipping the checks `rewake send` makes of its listener, and puts the letter in the
worker's mailbox: main's wrapper must refuse the registration and the letter fail. A
letter carrying a grant is put in by hand in main's name, and another in main's name
whose run is the worker's own while a listener in the test process answers at that run's
address and confirms the grant: both must fail without the program being offered the
directory — the first because main never registered it, the second because the answer
came from a process other than the run the letter names. What it does not prove is the
namespace check: every process in the suite shares the wrappers' namespaces, as a worker
outside a sandbox does, so those checks have their own unit tests in
`internal/core/grant/authority`. Its mutants take registrations from any process
(`grant-from-anywhere`, which breaks the direct registration alone — `rewake send` still
registers only with a listener above it), deliver a grant without asking main's wrapper
(`grant-unconfirmed`, which breaks the three letters), and take an answer from any
listener (`grant-any-listener`, which breaks the foreign answer alone).

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
whose `recalls` names the task. Their mutants run on the gate column; recall-unnamed
drops that member field from the gate's adapter; recall-sender-first
brings back the first wording, which led with the sender, and replacement-unmarked a
replacement previewed like any letter. The one that leaves the task readable also skips
the withdrawn status: the reader shows a message with that status as its tombstone, so a
tombstone written in the wrong place alone no longer reaches the worker as the task.

## Conversations

`thread-changed` runs on the fixture column with two workers and a sender for each; it
ran on the Claude Code column until S9, through the hooks' `session_id`. One worker's
program starts a new conversation once its task has arrived, as a person's `/clear`
does, tells its adapter, and works the task there; the other works its task where it
landed. The report from the first must carry `threadChanged` and still arrive once and
settle its task; the report from the worker that stayed must carry none. Its three
mutants name what they break and require the rest to hold: a wrapper that never pins
the delivery and a turn end that drops the conversation it ended in
(`end-thread-ignored`, in place of the column's `stop-thread-ignored`) must each lose
the mark, and a comparison that takes every known conversation for a change must mark
the worker that stayed.

`claude-inbound`, the inbound gate on Claude Code, retired with its column in S9; its
mutants on the code that stays are killed by the unit tests in the table at the top.

## A stop, and steering a session

`stopped-routing` runs on every column. The fixture harness's program takes the switch
`RW_SHIM_INTERRUPT_FIRST_TURN` and ends its first turn interrupted. A main, running its
own commands, sends one worker a task and another a `--notify`, which owes nothing; both
workers and main have their first turn interrupted. main must read `stopped` about the
task and list it as `stopped` in `--awaited`; the notified worker's stop, once its
receipt shows it was acted on, must leave nothing in main's inbox, read or not, and the
worker must read `idle`; main's own stop must leave nothing in its own inbox, unread
mail included, where a report main gives itself lies unannounced. Its mutant,
stopped-to-main, restores the rule the owner removed on September 23, 2026 — a stop
nobody waits on goes to main — and must break the last two observations and leave the
first.

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
wrong call. Its six mutants each break one link of the core's path and name what they
break: a request sent whatever the live set says (`steers-unlive`; until S11
`silent-not-answering`, a request nobody took reported as failed, which no run reaches
since a run without Control is refused first),
any role steering (`any-role-steers`), main noticed of its own compaction
(`own-compaction-announced`), a letter without the count (`letter-uncounted`), a letter
lost with its worker (`orphan-unlettered`), and every stop put down to a person
(`stop-by-a-person`). The eight mutants of `claude-steered` that broke the plugin's
module left with it. What it does not prove: that a real harness carries these calls out
as the program does.

## The live set and the channel

`fixture-live` (S11, October 11, 2026) runs six fixture sessions under one main, each
withholding what its program's switch says, and judges what main's `rewake list --json`
and the commands show. full proves everything: its record's live set names every
capability, its channel shows the tool connected, its briefing carries the whole
transport sentence, and once stopped its last snapshot keeps the channel record frozen at
the exit. wakeless (`RW_SHIM_NO_WAKE`) has no Wake live: a note and a task to it are
each accepted at once, exit 3, by send's own answer that no notice can reach it, and once
its server has tried them each waits readable in its inbox with its stored status
pending. toolless (`RW_SHIM_NO_TOOL`) says so on its standard error at launch, carries
the shell's clause alone in its briefing, keeps the tool offered and unproven in its
record — starting, or failing with no hello once the timer passes — never no tool, and
its shell read lands in its record. dropper
(`RW_SHIM_DROP_AFTER=1`) closes its connection after its first turn: its whole live set
is withdrawn at once, its channel shows the tool failing with the server gone, and a
second task to it fails, exit 1, with nothing written. denied makes one call its harness
refuses for permission (a call marked `denied` in `RW_SHIM_TOOL_CALLS`, the program's
`tool-denied` frame): its channel shows the tool denied by policy. stuck
(`RW_SHIM_NO_BOUNDARY`) is a worker whose run proves no turn boundary: its launch is
refused, exit 1, and leaves no session. The steering refusal without a live Control is
`fixture-steered`'s bare. Its eleven mutants each break one decision by the live set or
one transition of the channel keeper: a letter waiting for a delivery nothing makes
(`send-unwoken`), a reservation or a conversation asked of a Wake that is not live, which
fails the letter (`reserve-unguarded`, `thread-unguarded`), a task sent whatever the boundary's mark says (`task-unbounded`), a
worker's launch that does not wait (`launch-unawaited`), a lost connection kept in the
record (`lost-kept`), an unproven tool taken for no tool (`unproven-taken-for-none`),
a denial reaching nothing (`denial-dropped`), the shell's observations never folded
(`shell-unread`), the record not frozen at the exit (`exit-unfrozen`), and a briefing
built as if no tool were live (`brief-toolless`). What it does not prove: that a real
harness proves or loses a capability as the program does.

## A decision from a sandboxed shell

`fixture-decide-sandboxed` (S18, October 12, 2026) is a recipient whose shell cannot
`connect()` deciding a report nobody can prove reached it
([stage3-decision.md](v2/stage3-decision.md#the-command)). Worker api owes main web a
report; api's end is held (`RW_SHIM_HOLD_END`) while the case makes the report's
publication mark in web's mailbox unreadable, so the released end meets an unknown and
api's mailbox stops on it. web's shell then runs `rewake decide api R --arrived` under a
seccomp filter that answers `connect` with EPERM (a request marked `sandboxed`, run by
the program re-executed under `RW_SHIM_SANDBOXED`): it exits 1 naming the tool, and no
decision exists. web's tool call with the same words, made once the case writes R into
the file the call waits on (`after` in `RW_SHIM_TOOL_CALLS`), is recorded verified as
web's run, and web's `rewake inbox --awaited` then shows its task closed by that
decision, with nothing left for api's mailbox to decide. On an architecture the filter is
not written for, the case is unsupported for `seccomp-connect-filter`. Its three mutants:
a wrapper out of reach taken for the person (`decide-unreachable-as-person`), a tool call
that decides as the person without asking the wrapper (`decide-tool-as-person`), and a
wrapper that writes the decision unverified (`decide-wrapper-unverified`). What it does
not prove: that a real harness's sandbox refuses as the filter does.

## Every control

Each scenario's controls by name: the product mutants, built from a `mutation` value in
`test/workflow`, and the switches that change the fixture's world instead
([testing.md](testing.md#extending-the-suite) says why a mutant is preferred).
The table is kept by hand: the test that held its mutants column equal to the suite was
removed by the owner's decision of October 17, 2026.

| Scenario | Mutants | Fixture switches |
|---|---|---|
| batch-arrival | `preview`, `window`, `unwindowed`, `replay`, `peek-consumes` | — |
| task-report | `settles-nothing` | `wrong-report`, `read-fails`, `failure-before-report`, `early-exit` |
| mid-turn | `wait-for-idle` | `late`, `failed-operation` |
| pending-report | `pending-ignored`, `pending-settles`, `pending-text-dropped` | — |
| pending-confirm | `pending-unconfirmed`, `confirm-answer-dropped` | — |
| owed-reread | `owed-empty` | — |
| thread-changed | `delivery-unpinned`, `end-thread-ignored`, `thread-always-changed` | — |
| awaited-view | `awaited-interim-ignored`, `awaited-never-settled` | — |
| fixture-steered | `steers-unlive`, `any-role-steers`, `own-compaction-announced`, `letter-uncounted`, `orphan-unlettered`, `stop-by-a-person` | — |
| stopped-routing | `stopped-to-main` | — |
| turn-reasons | `reason-dropped`, `failure-completed`, `stop-completed` | — |
| tool-inbox | `tool-inbox-unshown`, `tool-read-unacknowledged` | — |
| tool-send | `tool-send-refused` | — |
| tool-pending | `tool-pending-refused` | — |
| tool-whoami | `tool-whoami-refused` | — |
| tool-retry | `tool-retry-refused` | — |
| tool-list | `tool-list-refused` | — |
| fixture-turn-start | `turn-start-untimed` | — |
| withdraw-after-notice | `withdraw-leaves-task`, `withdraw-silent`, `recall-sender-first` | — |
| withdraw-mid-turn | `recall-unnamed` | — |
| edit-after-notice | `edit-keeps-old-text`, `edit-unlinked`, `replacement-unmarked` | — |
| addendum-owed | `owed-flat` | — |
| fixture-worktree | `worktree-not-entered`, `worktree-running-ignored`, `worktree-visitor-ignored`, `worktree-git-kept`, `worktree-ignored-unseen`, `worktree-branch-trusted`, `worktree-moved-unrefused`, `worktree-detached`, `worktree-land-merges`, `worktree-finish-unchecked`, `worktree-finish-branch-kept` | — |
| fixture-grant-dir | `grant-not-rechecked`, `grant-refusal-untold`, `grant-unregistered`, `grant-held-after-report` | `RW_SHIM_HOLD_RESERVE` |
| fixture-grant-forgery | `grant-from-anywhere`, `grant-unconfirmed`, `grant-any-listener` | — |
| fixture-live | `send-unwoken`, `reserve-unguarded`, `thread-unguarded`, `task-unbounded`, `launch-unawaited`, `lost-kept`, `unproven-taken-for-none`, `denial-dropped`, `shell-unread`, `exit-unfrozen`, `brief-toolless` | `RW_SHIM_NO_WAKE`, `RW_SHIM_NO_TOOL`, `RW_SHIM_DROP_AFTER`, `RW_SHIM_NO_BOUNDARY` |
| fixture-decide-sandboxed | `decide-unreachable-as-person`, `decide-tool-as-person`, `decide-wrapper-unverified` | `RW_SHIM_HOLD_END`, `RW_SHIM_SANDBOXED` |
