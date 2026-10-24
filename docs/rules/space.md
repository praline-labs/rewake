# Rules: the first working space, SP1–SP10

What a space guarantees: a hosted worker, its record and its closing, in the
wording of [stage4-space-steps.md](../v2/stage4-space-steps.md#rules-before-code), the
second half of the design in [stage4-space.md](../v2/stage4-space.md). Written in stage 4's
step S4-2, October 13, 2026.

A rule enters this document in the step that builds the test holding it, as the steps table
names it ([the build steps](../v2/stage4-space-steps.md#the-build-steps)); until then it
stands in the design alone. Decided by main, October 13, 2026: SP3, SP4 and SP9 are here
from S4-2, each held in the part an existing test holds; SP2 and SP7 entered with S4-3, which
built the record ([stage4-space-record.md](../v2/stage4-space-record.md)). Since the owner's
decision of October 18, 2026 SP5 and SP6, the keeper's seat and its keys, are not built, and SP1
and SP8 enter with the one space step that replaces S4-7 to S4-10.

**What the space step changed, October 18, 2026** (S4-7, [the entry](../roadmap/2026-10-18-space.md)).
A space is main starting a worker: `rewake space open` runs a detached space host, which runs an
ordinary launch of the worker in a pseudo-terminal it owns, watches main's run and ends the
worker when that run ends. There is no keeper, no screen, no keys and no liveness pipe, and the
space host takes no request of the mod: its only socket served the person's own terminal
relayed to the worker's, open to any process of the user as every room file is. So SP1
is held by the space host's watch, SP3 by the environment it builds, SP9 by its own position
outside main's run before the worker starts; SP4 stays the rule of main's mod socket. SP8 — the
next start closing a record left open — is not built: a record is closed by its space host,
and one whose space host died with the machine stays open in the listing. Later that day the
owner decided on main's panel: the same socket also served a view of the worker's screen, which
main's wrapper relayed to its mod ([the archived design](../archive/v2/workers-panel.md)). The
same evening the host mode replaced both the panel and the attach, and on October 19, 2026 the
shell replaced the host mode: the socket serves the raw stream only, to the shell's panes
([shell.md](../v2/shell.md)), and the space host still takes no request of the mod.

## The rules

- **SP1. A space lives no longer than its main.** When main's run ends — by its harness
  ending or by its wrapper being killed, SIGKILL included — every worker of its spaces is ended
  within a bound, by a process that no signal of that shutdown reaches, which records the end
  it saw. A conversation that ends while the process goes on (`clear`, `resume`) closes
  nothing.

  Held since S4-7, October 18, 2026: the space host, a process outside main's run, reads main's
  run once a second; once it reads ended, it sends the worker's process group SIGHUP, after a
  second SIGTERM, after five more SIGKILL, waits up to two seconds for the group to be gone, and
  closes the record with the worker seen ended and the signal that ended it; a group still
  there after SIGKILL leaves the record closing, since no end was seen. Since October 21, 2026
  the sequence starts earlier than main's run read ended: main's mod tells its wrapper
  `session/closing` at `session.end` of an exit, about half a second before the process is gone,
  and a main in the shell then moves each of its open spaces to closing with "main's session
  ended" (`space.BeginClosingFor`), which the hosts read at their next tick (SP10's path); main's
  own end is final, so no work that would have gone on is stopped. Live, `/exit` with two haiku
  workers: the last worker's record closed 2.6–3.4 s after the key before (hosts reading main's run
  gone, up to a second after its exit), 1.5–1.9 s after. A worker that ends
  by itself closes the space the same way, with its exit and its last line. Main's run read as
  unknown ends nothing.
  Tests:
  - `internal/host/spacehost/stop_test.go` `TestTheCloseSequenceEndsAWorkerThatIgnoresHangupAndTerm`
  - `test/workflow/fixture_space_test.go` `TestFixtureSpace`

- **SP2. The record comes first.** A space's record — the worker's harness, name, role, the worker type's name when it has one,
  session id where the harness takes one, the checkout's path and branch — is written before
  the worker starts, and every later state is written before the act it names is answered. A
  record is closed only from what was observed — the worker's end, the checkout's state — never
  from a release or a signal sent.

  Held today in the record itself: it is written only through the state package under the
  room's lease; it begins opening with the worker fixed, takes its checkout before the worker's
  run, and moves through its states in one order, a step out of it refused; a close takes only
  an observation — the worker seen to end or never started, the checkout found there, gone or
  not to be looked at — and refuses one that says nothing of either or contradicts the record,
  and a record on disk whose end omits the checkout's observation does not read; and a crash
  between two steps
  leaves the earlier one whole, which a later close finishes. Since S4-7, October 18, 2026, the
  steps are the space host's: it records its own run before anything is checked, the worker's
  run once the worker's wrapper is started, and the checkout once the worker's session claims
  one; `rewake space open` answers only once the record is open and the worker's session is
  registered with that run. A close records the worker seen ended or never started and what
  was then seen of the checkout; with `--worktree`, or a template that lets the worker enter
  one mid-session (`enter_worktree = true`), it looks for the claimed checkout once more,
  closing included, and a look that fails leaves the record closing, never closed as a space
  that made none. The format on disk is held by its own test.
  Tests:
  - `internal/core/space/format_test.go` `TestTheRecordsFormatOnDisk`
  - `internal/host/spacehost/close_test.go` `TestACheckoutClaimedBeforeAnyPollIsKeptWhenMainEndsFirst`
  - `internal/core/space/store_test.go` `TestTheRecordIsWrittenOnlyUnderTheRoomsLease`
  - `internal/core/space/store_test.go` `TestTheStatesMoveInTheirOrder`
  - `internal/core/space/store_test.go` `TestARecordClosesOnlyFromAnObservation`
  - `internal/core/space/store_test.go` `TestARecordLeftByACrashBetweenStepsCanBeClosed`
  - `internal/core/space/observation_test.go` `TestACloseNeedsTheCheckoutObserved`
  - `internal/core/space/observation_test.go` `TestARecordWithoutTheCheckoutObservedDoesNotRead`

- **SP3. A hosted launch inherits nothing of a session.** Its environment carries no variable
  that names main's session or a transport of main's run, and none of the worker harness's
  `EnvNotInherited`; what it carries of rewake's identity and transport is set fresh by the
  host — the base of main's checked room and the room — while the person's preferences pass as
  they are. It runs the wrapper's own image and joins main's room under main's base.

  Held since S4-7, October 18, 2026: the space host strips `REWAKE_SESSION`, `REWAKE_EPOCH`,
  `REWAKE_ROOM`, `REWAKE_DIR`, its own spec, every `REWAKE_BRIDGE_` variable and every catalogue
  harness's `EnvNotInherited` (which carries `REWAKE_MOD_SOCKET`), sets `REWAKE_DIR` to the base
  of main's room — refusing a base that is not absolute, so the default is never taken — and
  `TERM`, and runs its own executable as `rewake <harness>` with `REWAKE_SPACE=<room>/<space id>`
  and `REWAKE_SPACE_TEMPLATE=<alias>`, the alias every space is opened with, carried in the
  host's spec, not the record. The launch believes them only from its parent, that space's host, and takes the name, role and room
  from the space's record (October 19, 2026). From there the worker is an ordinary launch,
  whose identity reaches its harness from its own wrapper.
  Tests:
  - `internal/host/spacehost/env_test.go` `TestTheWorkerGetsMainsBaseAndNothingOfItsSession`
  - `internal/host/wrap_test.go` `TestSessionEnvironmentReachesTheHarness`
  - `internal/host/wrap_test.go` `TestWhatTheAdapterSaysNotToInheritStaysOut`

- **SP4. Only main's harness process reaches main's space host.** Every request of the mod's
  protocol is checked by its peer on its own; a descendant is refused.

  Held for the mod's socket since S4-5, October 13, 2026: every request is a connection of its
  own, its peer read by `SO_PEERCRED`; only the harness process the host started, alive with
  its start time, is served, a process below it or beside it of the same user is refused
  whatever it asks, a request before the host names the harness waits within a bound, and the
  peer is checked again before any effect of a hello and again before the answer.
  The turn's endpoints and the heartbeat follow the same check: the peer is checked again before
  each effect of a start, an end and a poll, and again before the answer
  ([claude-turns.md](../claude-turns.md#the-hosts-side)). Held since S4-6, October 16,
  2026: the start and the end since its part A, the poll since its part B.
  Tests:
  - `internal/adapter/claude/modhost/server_test.go` `TestOnlyTheHarnessesOwnProcessIsServed`
  - `internal/adapter/claude/modhost/server_test.go` `TestAReplacedHarnessIsNotServed`
  - `internal/adapter/claude/modhost/server_test.go` `TestARequestBeforeTheHarnessIsNamedWaits`
  - `internal/adapter/claude/modhost/server_test.go` `TestEveryRequestIsAConnectionOfItsOwn`
  - `internal/adapter/claude/modhost/admission_test.go` `TestAHelloFromAReplacedHarnessHasNoEffect`
  - `internal/adapter/claude/modhost/turns_test.go` `TestATurnFromAReplacedHarnessHasNoEffect`
  - `internal/adapter/claude/modhost/turns_test.go` `TestARequestDelayedAcrossCloseTakesNoEffect`
  - `internal/adapter/claude/modhost/alive_test.go` `TestAPollFromAReplacedHarnessRenewsNothing`
  - `internal/adapter/claude/modhost/alive_test.go` `TestAPollPastTheCloseRenewsNothing`

  Since S4-7, October 18, 2026, the space host is no endpoint of the mod: it takes no request
  of main's harness, and its socket serves the worker's raw stream to the shell and decides
  nothing.

- **SP7. A worker's checkout goes at its close when nothing would be lost, and a kept one is
  never forgotten.** Once the worker's end is observed, the close removes the worker's checkout
  and its branch when removing them loses nothing; otherwise the space closes kept, says why,
  and is listed to main at its start and by `rewake space list` until the checkout is gone.

  Decided by the owner, October 20, 2026: until then rewake removed no checkout, and a room
  gathered eighteen checkouts of merged work. Held since that day: the space host, after the
  worker's process group is seen gone, looks and removes under one hold of the repository's
  lock, as `rewake worktree rm` does (`worktree.Release`). It keeps the checkout for anything rm
  would keep it for but files git ignores — a rewake session in it, changes, a tracked file
  marked assume-unchanged or
  skip-worktree, whose edits status does not show, a HEAD no ref holds, a `git worktree lock`, a submodule,
  a directory or repository gone — and for a commit, at its HEAD or its branch's tip, that no
  local branch but its own holds. Any local branch, not only the one it was made from: main may
  land with `--into`, and the checkout it came from may have switched since; a tag or a
  remote-tracking ref does not count, being moved or pruned with no thought of this work. Its
  branch is then dropped as rm drops one, and so is the branch the checkout was on at removal
  when the worker switched to one of its own (`git switch -c`), by the same rule: dropped when
  another ref holds its tip, kept when it holds work nothing else has — its commits are safe on
  it, which is why the checkout may go, and the end's reason then says so, before any
  "its checkout stays", as "; its branch <name> stays: its commits are on no other branch" (or
  "checked out at <path>"), so `rewake space close` and `rewake space list` show it. Files git ignores — `node_modules`, logs, a check's
  output — go with the checkout: almost every writer leaves some, and of seventeen checkouts
  kept, fourteen were kept only for them, so the owner decided the same day that they do not
  keep one at its close; rm and finish, the person's own commands, still keep them. The worker
  sees this decision before its end: `rewake worktree status` forecasts it by the same policy
  (`worktree.CloseForecast`), with only the worker's own run not counted
  ([worktree-tools.md](../worktree-tools.md)). Why a checkout stays is added to the end's reason
  after "; its checkout stays: ", so the record's format is unchanged; `rewake space close`
  answers it beside where the checkout is, and `rewake space list` with the reminder. A space
  that main's end closed has nobody to answer: the record and the listing carry it to the next
  main. A space closes kept when its checkout was found there or could not be looked at; a kept
  checkout is reminded while it is there, and while it cannot be looked at, with what was found
  in it and the Git commands that clear it; one seen gone is not; and the package that keeps the
  records runs no command and removes no file. Known limits: the reminder to main at its start
  (October 13, 2026); a close that waits on the repository's lock, held at most a minute by a
  launch copying `.worktreeinclude`'s files, can outlast the 20 s `rewake space close` waits.
  Tests:
  - `internal/core/space/list_test.go` `TestAKeptCheckoutIsRemindedUntilItIsGone`
  - `internal/core/space/list_test.go` `TestAnUnknownCheckoutIsReminded`
  - `internal/core/space/observation_test.go` `TestAnUnknownCheckoutKeepsTheSpace`
  - `internal/core/space/list_test.go` `TestThePackageRemovesNoCheckout`
  - `internal/cli/space_test.go` `TestSpaceListShowsOpenClosedAndKept`
  - `internal/host/worktree/release_test.go` `TestReleaseRemovesOnlyWhatLosesNothing`

- **SP9. A hosted worker is never main's descendant.** Its space host runs outside main's run,
  checked before the worker starts; an open that cannot show it is refused, and nothing the
  worker controls runs before that check. So no signal of main's shutdown reaches the process
  that ends the worker (SP1).

  Held since S4-7, October 18, 2026: `rewake space open` re-executes rewake as a launcher in a
  session of its own, which starts the space host and exits; the space host waits until the
  launcher has left as its parent, then reads its whole chain of parents up to the root: one
  that meets main's wrapper, a parent started after its child, a loop or an unreadable link
  refuse, so a wrapper that is a subreaper and adopted it refuses too. A refusal closes the
  record with the worker never started and says why; the worker's wrapper is started only
  after it.
  Tests:
  - `internal/infra/proc/lineage_test.go` `TestOutsideIsShownOnlyFromAWholeChain`
  - `internal/host/spacehost/host_test.go` `TestASpaceHostBelowMainStartsNoWorker`

- **SP10. Main closes a space through its host.** Main's `rewake space close` ends only a space
  its own run opened, and only by the record: it moves an open space to closing with main's
  reason, and the space host, never main, ends the worker by SP1's sequence and closes the record
  from what it observed, the reason kept. Main signals no process of the worker.

  Held since October 19, 2026: the space host re-reads the record once a second, and one found
  closing ends its watch with the record's reason; the command answers once the record is
  closed or kept, within 20 s ([launch.md](../launch.md#a-space-for-a-worker)).

  A space whose host is gone — killed or crashed, proven by the host's run read ended
  (`space.HostRuns`, the test every run's liveness uses) — has nobody to read the record, and its
  closing used to stay for good (seen October 21, 2026). Then `rewake space close` finishes it
  itself, by the same sequence and under the same rules (`spacehost.FinishGone`): it stops what
  runs of the worker's process group, releases the checkout by SP7, and closes the record from
  what it saw, the reason adding "; its space host was gone". A worker whose wrapper cannot be
  told alive or ended leaves the space closing, and so does a group still there after SIGKILL: the
  group is stopped whatever the wrapper reads, since a harness it started may outlive it, and the
  checkout is released only once the whole group is seen gone (a wrapper's pid taken by another
  live process leaves nothing to stop). Known limit: the host's socket file stays in the room.
  Tests:
  - `internal/host/spacehost/close_main_test.go` `TestMainsCloseEndsTheWorkerThroughTheSpaceHost`
  - `internal/cli/space_close_test.go` `TestSpaceCloseIsRefusedButForMainsOwnOpenSpace`
  - `test/workflow/fixture_space_close_test.go` `TestFixtureSpaceClose`
  - `internal/host/spacehost/gone_test.go` `TestASpaceWhoseHostIsGoneIsFinishedByTheClose`
  - `internal/host/spacehost/gone_test.go` `TestAGroupLeftByADeadWrapperIsStoppedBeforeTheEndIsRecorded`
  - `internal/core/space/mainend_test.go` `TestMainsEndingClosesOnlyItsOpenSpaces`
