# Rules: the first working space, SP1–SP9

What a space guarantees: a hosted worker, its record, its keeper and its closing, in the
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

## The rules

- **SP2. The record comes first.** A space's record — the worker's harness, name, role,
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
  leaves the earlier one whole, which a later close finishes. Known limit, recorded on October
  13, 2026, owned by S4-7, which builds the open: each step written by main's wrapper before
  the act it names — the worker's launch, its gate, the open's answer; and by S4-9, which builds
  closing: the observations a close records made by the host.
  Tests:
  - `internal/core/space/store_test.go` `TestTheRecordIsWrittenOnlyUnderTheRoomsLease`
  - `internal/core/space/store_test.go` `TestTheStatesMoveInTheirOrder`
  - `internal/core/space/store_test.go` `TestARecordClosesOnlyFromAnObservation`
  - `internal/core/space/store_test.go` `TestARecordLeftByACrashBetweenStepsCanBeClosed`
  - `internal/core/space/observation_test.go` `TestACloseNeedsTheCheckoutObserved`
  - `internal/core/space/observation_test.go` `TestARecordWithoutTheCheckoutObservedDoesNotRead`

- **SP3. A hosted launch inherits nothing of a session.** Its environment carries no variable
  that names main's session or a transport of main's run, and none of the worker harness's
  `EnvNotInherited`; what it carries of rewake's identity and transport is set fresh by the
  host — the base of main's checked room, the room, the liveness descriptor — while the person's
  preferences pass as they are. It runs the wrapper's own image and joins main's room under
  main's base.

  Held today for an ordinary launch, whose environment the hosted one starts from: the
  session's identity reaches the harness from the wrapper that runs it, never from a parent
  session, and what the adapter says its runs must not inherit stays out while the rest
  passes. Known limit, recorded on October 13, 2026, owned by S4-7, which builds the hosted
  launch: the transport variables left out (`REWAKE_MOD_SOCKET`, every `REWAKE_BRIDGE_`
  variable), the base set from main's checked room, the liveness descriptor, the wrapper's
  own image and the join of main's room under main's base.
  Tests:
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
  ([stage4-turns.md](../v2/stage4-turns.md#the-hosts-side)). Held since S4-6, October 16,
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

- **SP7. A kept checkout is never forgotten and never removed by rewake.** A closed space
  whose checkout is still there is listed to main at its start and by `rewake space list`
  until the checkout is gone; rewake removes no checkout.

  Held today for the listing: a space closes kept when its checkout was found there or could
  not be looked at; a kept checkout is reminded while it is there, and while it
  cannot be looked at, with what was found in it and the Git commands that clear it; one seen
  gone is not; `rewake space list` shows it beside the open and closed spaces; and the package
  that keeps the records runs no command and removes no file. Known limit, recorded on October
  13, 2026, owned by S4-9, which builds closing: the reminder to main at its start, and a
  closing that removes no checkout.
  Tests:
  - `internal/core/space/list_test.go` `TestAKeptCheckoutIsRemindedUntilItIsGone`
  - `internal/core/space/list_test.go` `TestAnUnknownCheckoutIsReminded`
  - `internal/core/space/observation_test.go` `TestAnUnknownCheckoutKeepsTheSpace`
  - `internal/core/space/list_test.go` `TestThePackageRemovesNoCheckout`
  - `internal/cli/space_test.go` `TestSpaceListShowsOpenClosedAndKept`

- **SP9. A hosted worker is never main's descendant.** Its wrapper is reparented away from
  main's wrapper before it is seated, checked by the same ancestry test the authority makes; an
  open that cannot prove it is refused, and nothing the worker controls runs before that proof.
  So main's decision endpoint refuses the worker's commands by the existing rule, and
  no authority rule changes.

  Held today in the part that does not change: the ancestry test walks a process's parents
  to the wrapper and refuses a stranger and a reused pid, and main's decision
  endpoint refuses a process that does not run below main's wrapper. Known limit, recorded
  on October 13, 2026, owned by S4-7, which builds the hosted launch: the detached launcher,
  the refusal when main's wrapper is a subreaper, the ancestry proven by the whole chain with
  an unreadable one refusing the open, and the launch gate before anything the worker
  controls.
  Tests:
  - `internal/infra/proc/lineage_test.go` `TestDescendsWalksParentsToTheAncestor`
  - `internal/infra/proc/lineage_test.go` `TestDescendsRefusesAReusedParent`
  - `internal/core/authority/decide_test.go` `TestAWrapperTakesADecisionOnlyFromBelow`
