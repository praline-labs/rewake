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

`wrapped-launch` runs in every column, twice each, in two rooms: a plain launch and one
with `--command ./<harness>-worker`, a stand-in wrapper that exports a marker and execs the
fixture. Every fixture process records its arguments and whether it saw the marker, and
the case requires the wrapped processes — one for Claude Code; for Codex the version
check, the app-server and the terminal — to get argument for argument what the plain
launch gets, all to see the marker, and none of the plain ones to. The Codex launches
carry `-C` into a directory holding another script of the same name, which must never
run. It has no control in the suite; with the relative path left relative, it went red on
three of its four observations (September 23, 2026).

`pending-report` runs in every column with three sessions. A worker reads a task and,
in that turn, runs `rewake pending` before ending it; the sender must read a `pending`
message about the task first — the mark's line, then the turn's own text — and the
worker's awaiting record must still be there.
A third session then sends the worker a task of its own, and that turn end — with no
mark — must be the `finished` report settling the first task. Its three mutants, each in
every column, ignore the mark, let the interim turn end settle the task, and drop the
turn's text from the interim message; like the
telemetry controls, each names what it must break and requires the rest to hold. On the
Claude Code column the unmarked turn end is now held once by the Stop hook first; the
fixture's model then only ends the turn, so the report follows as before.

`pending-confirm` runs in the Claude Code and fixture columns with four sessions. A worker ends its
first turn about a task pending; woken by a second session's mail, its next turn ends
with no mark, and the Stop hook must hold it exactly once, quoting the pending line. The
fixture answers a block as the harness does — it asks its model again and runs the Stop
hooks once more with `stop_hook_active: true` and the new answer alone — and its model,
asked the first time, runs `rewake pending` with a second line: the sender must read an
interim message with that line, then the held answer, then the continuation, and the
task must stay owed. Woken by a third session, the worker ends unmarked again, is held
once more about the second line, and only ends the turn: the report must be the held
answer and then the continuation, and it must settle the task. Its two mutants never
hold, and publish the continuation without the held answer (September 26, 2026). On the
fixture column the worker's program asks the core through the adapter's `Confirm` before
an end closes and goes on only with the reason it is given; the case and both controls
run there too (October 7, 2026).

`turn-reasons` runs in the fixture column only, with five sessions: the path it holds — a
turn's outcome and text in the program's end frame, the adapter's neutral completion and
the wrapper's confirmation — is the fixture's own, and the other columns hear their ends
through their harness's events. A worker whose turns fail holds a task from main and one
from a peer; each sender must read an `error` about its own task carrying the turn's
reason, and a session that waits on nothing must receive nothing from it. A second
worker's first turn is interrupted while it holds a task from main, which must read it
`stopped` with that turn's reason. The reasons are set by `RW_SHIM_END_REASON` and differ
from anything the mail carried. Its three mutants drop the text on the way to the
completion, take a failure for a finished turn, and take an interruption for one; each
names what it must break and requires the rest to hold (October 7, 2026).

`tool-inbox`, `tool-send`, `tool-pending`, `tool-whoami`, `tool-retry` and `tool-list` run
in the fixture column only, one per tool of the set, each with a main and a worker: the
transport is the fixture's own program, which registers the tools the probe offers and,
asked by `RW_SHIM_TOOL_CALLS`, makes its first turn's calls as a harness would — the
native call reported to the adapter, the request to the wrapper's endpoint, the result it
would hand the model reported back — and logs each answer. The main sends a task; the
worker's turn makes the call and ends. `tool-inbox` reads the task through the tool: the
answer must show it, and the turn's end must report and settle it, so the read counted.
The program's model goes on until the letter leaves the unread overview, as a real
model takes its time over a result: an end that came first would show the letter again
(T7, T8), which the fixture's ends otherwise do within milliseconds. `tool-send` must
reach the main as one heads-up; `tool-pending` must be accepted and reach the main as an
interim message with its line while the task stays owed; `tool-whoami` must name the
worker and say the call came through the tool; `tool-retry` retries a heads-up by the
receipt its answer named and must answer that receipt with the heads-up still one;
`tool-list` must name both sessions. None has a control yet (October 8, 2026).

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

`codex-tui-later-shape` runs on the Codex column with a main and two workers whose
fixture terminal speaks the form of Codex 0.157.1 (`RW_SHIM_TUI_SHAPE`): its start and
resume carry `runtimeWorkspaceRoots: null`, `permissions: null` and a configuration
holding only `web_search`, the resume by id with `history` and `path` null. One worker
starts fresh, the other resumes a conversation and then reads its goal, as the terminal
does. main sends each a task, and each must report it `finished`: the gateway took the
terminal's selection from its configuration. Its mutant, `roots-only-recognition`,
restores the rule that asked for roots or permissions, and must break both observations
([research-codex-live-checks.md](research-codex-live-checks.md#the-terminals-selection-on-01571)).

## A worktree for a Codex launch

`codex-worktree` runs on the Codex column only; the Claude Code launch has its own case
below. A worker is launched from `src/nested` of a fresh repository with
`--worktree=probe`, the worktree directory left at its default under the case's home,
and each half of the fixture writes the directory it started in (`RW_SHIM_CWD_FILE`).
The checkout must be one of the source's HEAD on a new branch `probe`, the source still
on `main`, under that directory, and the session's record, the terminal and the
app-server must all work in its `src/nested`; a task a main sends must be delivered
there; `rewake worktree rm` must refuse the checkout while the worker runs and say so.
While the worker runs, a commit made in the checkout is landed — `main` moves to it, its
file appears in the source, the checkout stays on `probe` and the worker keeps running
— and after a second commit `finish` must refuse, naming the worker, and move nothing.
After the worker has ended, rm must still refuse while a visitor session started in the
checkout runs. Three spare checkouts, launched and ended for the purpose, show what
else rm keeps: one holding a `.env` the repository ignores (and `--force` removes it),
one whose commit no ref holds once its checkout is detached and every ref holding it is
deleted (and removed once they are back), and one whose directory was moved away, where
the refusal points at `git worktree repair` and `--force` takes out its record and git's
entry. Once nothing holds the first checkout, `finish` must land the second commit and
remove the checkout, its record, git's own entry and the branch. Its eleven mutants each
name what they break and require the rest to hold: a launch that never enters its
checkout; a removal that takes no session for a running one, which also leaves nothing
to visit and lets finish go ahead; one that sees only the session the checkout was made
for, which removes it from under the visitor; one that deletes the directory behind
git's back, whose entries stay; one blind to ignored files; one that asks whether a
commit is held only after the HEAD moved; one that removes a moved checkout without
`--force`; a checkout made detached, which nothing can land and finish will not take; a
land that makes a merge commit; a finish that asks nothing first; and one that leaves
the branch behind. That the removal of a missing checkout takes out its own entry and no
other, where `git worktree prune` would take every missing checkout's, is held by
`internal/worktree` (`TestAMissingCheckoutIsRemovedAlone`).

## A worktree for a Claude Code launch

`claude-worktree` runs on the Claude column. A worker is launched from `src/nested` of a
fresh repository with `--worktree=probe`, and the fixture writes the directory it
started in. The checkout must be one of the source's HEAD on a new branch `probe`, and
the session's record and the harness must work in its `src/nested`; the fixture refuses
an option it does not know, so a `--worktree` passed on would end the launch. After a
commit in the checkout and the worker's end, `finish` must land the commit in `main`
with its hash and remove the checkout, its record and the branch. Its mutant,
`claude-worktree-parent`, names the parent of the launch directory and must break only
the first observation. A mutant that passed the flag on is not used: the launch it
breaks exits 2, and a case whose process failed fails its cleanup whatever its
observations say; the refusals of `-w`, `--tmux` and the continuations are unit tests in
`internal/cli`.

## A directory granted with a task

`codex-grant-dir` runs on the Codex column only: Claude Code takes a grant through its
permission hooks, and `claude-grant-dir` below is its case. Its main is a Claude Code
session, because a Codex main cannot grant ([grants.md](grants.md#who-can-grant)), and the granted directory lies in
the user's cache directory rather than under `/tmp`, which is never granted. The fixture
keeps the thread's workspace roots as the
server was seen to — set by the start, replaced by a `turn/start` that carries them —
answers `thread/read` with them in `environments` and with the status of the turn in
progress, and records the roots each delivery that carried them named. A write worker holds its
first turn open; main sends it a task with `--grant-dir` on a directory outside its
workspace, which must be accepted pending (exit 3), start a turn of its own after the
held one closes and never be steered into it, and name the directory beside `/work` in
its roots. After the worker's report, the next plain task must carry the roots without
the directory, and main's `rewake list --json` must show it `revoked`. Its mutants skip
the idle wait (`grant-steered`, which breaks the wait alone), journal the directory
without adding it (`grant-not-added`, which breaks the roots and the revocation, the
journal then saying `dropped`), and keep it after the report (`grant-kept`, which breaks
the revocation alone). The shape case checks the fixture's `thread/read` reply of a
working thread against `ThreadReadResponse`. The path rules, the journal and the
refusals are unit tests in `internal/grant`, `internal/cli` and `internal/harness/codex`.

`codex-grant-forgery` is a Codex write worker trying to grant itself a directory in the
name of a running Claude Code main, five ways. Its own process runs `rewake send` with
main's `REWAKE_SESSION` and `REWAKE_EPOCH`, and so does a process it detached through
`setsid`: both must exit 1 and leave no letter. The test process, outside main's tree,
writes a registration straight to main's address, as a forger's own client would,
skipping the checks `rewake send` makes of its listener, and puts the letter in the
worker's mailbox: main's wrapper must refuse the registration and the letter fail. A
letter carrying a grant is put in by hand in main's name, and another in main's name
whose run is the worker's own while a listener in the test process answers at that
run's address and confirms the grant: both must fail and never reach the worker's roots
— the first because main never registered it, the second because the answer came from
a process other than the run the letter names. What it does not prove is the namespace
check: every process in the suite shares the wrappers' namespaces, as a worker outside
a sandbox does, so those checks have their own unit tests in `internal/grantauth`, the
registration's from a helper in a real user namespace. Its mutants take registrations
from any process (`grant-from-anywhere`, which breaks the direct registration alone —
`rewake send` still registers only with a listener above it), deliver a grant without
asking main's wrapper (`grant-unconfirmed`, which breaks the three letters), and take
an answer from any listener (`grant-any-listener`, which breaks the foreign answer
alone).

`claude-grant-dir` runs on the Claude Code column only, with a Claude Code main and write
worker. The fixture calls tools when told to (`RW_SHIM_TOOLS`): each line `tool <Write|Read|Bash>
<path>` in the mail it read is one call, made before the turn ends. It keeps its working
directories — its own and those a hook added — and passes each call through the grant
hooks as the harness was seen to in `acceptEdits` on 2.1.280: PreToolUse for the tools its
matcher names, where deny ends the call and ask forces a PermissionRequest; a
PermissionRequest for a write outside every working directory, suggesting its parent;
allow applying `addDirectories` and `removeDirectories`, silence going to a person, who
never answers. An answer outside that shape — an update beyond the session, a kind the
hook does not make — is recorded as refused. Main grants a directory in the user's
cache; the worker runs a command in it, writes in it, runs another command in it, writes
its `.git/config` and a directory beside it: the first command goes to the person, the
write runs and adds the directory, the second command then runs unasked, and the last
two go to the person. After the report the next task writes in it, runs a command in it,
reads `README` in the worker's own directory and writes in it again: denied, to the
person, run with the directory removed, then to the person; main's `rewake list --json`
must show it `revoked`. Its mutants never
allow a write in a grant (`claude-grant-silent`, which breaks the giving and the taking
back, the grant ending at the report with nothing added), let a grant reach its Git
metadata (`claude-grant-unshielded`, which breaks the shielded writes alone), and never
learn that a task was reported on (`claude-grant-kept`, which breaks the taking back
alone). The decisions themselves, and the keeper's checks of who asks, are unit tests in
`internal/harness/claude` and `internal/grantauth`.

`claude-grant-resume` and `codex-grant-resume` are a grant across a cold resume
([grants-resume.md](grants-resume.md)), one per column, each with a Claude Code main. The
worker writes in the grant, marks its turn pending and is stopped; it is started again in
the same conversation, reports, and is stopped and started once more. The Claude Code
fixture takes `--resume <id>` and `--add-dir`: it keeps the conversation's id the resume
names, starts with the given directories as working ones, says `resume` as the
SessionStart source and records the launch among its turns. The Codex fixture keeps a
thread's roots in a file between runs (`RW_SHIM_THREAD_STORE`), as the server was seen
to: those a start names and those of every completed turn after the first, restored by a
resume that names none — so a grant given on the first turn is lost by the resume, and
the resumed terminal sends no roots, in 0.157.1's shape. After the first resume the
Claude Code run must be launched with `--add-dir` for the directory and run a command in
it unasked, and the Codex one must name it among the roots at the first delivery, the
listing showing it granted. The resumed run's report must settle the task, and then the
grant goes as any other: a write denied, the directory taken out, the listing `revoked`.
After the second resume the Claude Code run is launched with no directory, and the Codex
run's first delivery names none and journals nothing of it, though the old copy still
names the grant live. Their mutants give the harness no confirmed directory
(`claude-resume-not-given`, which breaks the giving alone), keep it as one the hook never
added (`claude-resume-not-held`, which breaks the taking back alone), restore nothing on
Codex (`codex-resume-not-restored`, which breaks the giving and the taking back), take over
no wait of an earlier run (`resume-waits-dropped`, which breaks the same two, the task
closed by the resume), and have main hold a grant after its report
(`claude-resume-closed-held` and `codex-resume-closed-held`, which break the resume after
the report alone). On Codex the report the run finds would take such a grant out again in
the same delivery, so the case asks that the run journal nothing, not only that the roots
lack it. Main's side of the confirmation, the hints and the adopted waits are unit tests in
`internal/grantauth`, `internal/grant`, `internal/wrap`, `internal/harness/codex` and
`internal/inbox`.

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
deliver into — Codex's and the fixture's; the Claude Code column records it unsupported —
with the worker's first turn held open (`RW_SHIM_HOLD_TURN`; the read gate alone leaves no turn open to
steer into): the recall must be steered into the turn the task started, as a member
whose `recalls` names the task. Their mutants run on the gate column; recall-unnamed
drops that member field from the gate's adapter; recall-sender-first
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
commands on the Codex column, and `codex-compact-hold`, a task sent into a long Codex
compaction.

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
| pending-confirm | `pending-unconfirmed`, `confirm-answer-dropped` | — |
| claude-inbound | `ungated`, `gate-on-session-start`, `held-as-delivered`, `expiry-unannounced`, `refusal-as-delivered`, `late-word-dropped` | — |
| owed-reread | `owed-empty` | — |
| thread-changed | `delivery-unpinned`, `stop-thread-ignored`, `thread-always-changed` | — |
| awaited-view | `awaited-interim-ignored`, `awaited-never-settled` | — |
| claude-interrupted | `interrupt-unpublished`, `every-end-stopped`, `plugin-not-passed`, `stop-not-heard` | — |
| claude-steered | `compact-not-run`, `in-turn-unmapped`, `interrupter-unnamed`, `line-repeated`, `idle-interrupt-done`, `silent-not-answering`, `any-role-steers`, `own-compaction-announced`, `letter-uncounted`, `waits-for-the-end`, `ended-uncounted`, `asker-untold`, `orphan-unlettered`, `stop-by-a-person` | — |
| codex-steered | `busy-unchecked`, `codex-asker-untold`, `codex-waits-for-the-end`, `codex-outcome-unkept`, `codex-interrupter-unnamed`, `codex-idle-interrupt-done`, `focus-taken` | — |
| codex-tui-later-shape | `roots-only-recognition` | — |
| codex-compact-hold | `hold-ends-at-start`, `compaction-refusal-final`, `late-end-unrecorded`, `running-taken-for-an-outcome` | — |
| codex-grant-dir | `grant-steered`, `grant-not-added`, `grant-kept` | — |
| codex-grant-forgery | `grant-from-anywhere`, `grant-unconfirmed`, `grant-any-listener` | — |
| claude-grant-dir | `claude-grant-silent`, `claude-grant-unshielded`, `claude-grant-kept` | `RW_SHIM_TOOLS` |
| claude-grant-resume | `claude-resume-not-given`, `claude-resume-not-held`, `claude-resume-closed-held` | `RW_SHIM_TOOLS` |
| codex-grant-resume | `codex-resume-not-restored`, `resume-waits-dropped`, `codex-resume-closed-held` | `RW_SHIM_THREAD_STORE` |
| stopped-routing | `stopped-to-main` | — |
| turn-reasons | `reason-dropped`, `failure-completed`, `stop-completed` | — |
| withdraw-after-notice | `withdraw-leaves-task`, `withdraw-silent`, `recall-sender-first` | — |
| withdraw-mid-turn | `recall-unnamed` | — |
| edit-after-notice | `edit-keeps-old-text`, `edit-unlinked`, `replacement-unmarked` | — |
| addendum-owed | `owed-flat` | — |
| codex-worktree | `worktree-not-entered`, `worktree-running-ignored`, `worktree-visitor-ignored`, `worktree-git-kept`, `worktree-ignored-unseen`, `worktree-branch-trusted`, `worktree-moved-unrefused`, `worktree-detached`, `worktree-land-merges`, `worktree-finish-unchecked`, `worktree-finish-branch-kept` | — |
| claude-worktree | `claude-worktree-parent` | — |
