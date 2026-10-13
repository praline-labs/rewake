# Rules: the first working space, SP1–SP9

What a space guarantees: a hosted worker, its record, its keeper and its closing, in the
wording of [stage4-space-steps.md](../v2/stage4-space-steps.md#rules-before-code), the
second half of the design in [stage4-space.md](../v2/stage4-space.md). Written in stage 4's
step S4-2, October 13, 2026.

A rule enters this document in the step that builds the test holding it, as the steps table
names it ([the build steps](../v2/stage4-space-steps.md#the-build-steps)); until then it
stands in the design alone. Decided by main, October 13, 2026: SP3, SP4 and SP9 are here
from S4-2, each held in the part an existing test holds; SP2 and SP7 entered with S4-3, which
built the record ([stage4-space-record.md](../v2/stage4-space-record.md)); SP5 enters with
S4-7, SP6 with S4-8, SP1 and SP8 with S4-9.

## The rules

- **SP2. The record comes first.** A space's record — the worker's harness, name, role,
  session id where the harness takes one, the checkout's path and branch — is written before
  the worker starts, and every later state is written before the act it names is answered. A
  record is closed only from what was observed — the worker's end, the checkout's state — never
  from a release or a signal sent.

  Held today in the record itself: it is written only through the state package under the
  room's lease; it begins opening with the worker fixed, takes its checkout before the worker's
  run, and moves through its states in one order, a step out of it refused; a close takes only
  an observation — the worker seen to end or never started, the checkout found gone or left —
  and refuses one that says nothing or contradicts the record; and a crash between two steps
  leaves the earlier one whole, which a later close finishes. Known limit, recorded on October
  13, 2026, owned by S4-7, which builds the open: each step written by main's wrapper before
  the act it names — the worker's launch, its gate, the open's answer; and by S4-9, which builds
  closing: the observations a close records made by the host.
  Tests:
  - `internal/core/space/store_test.go` `TestTheRecordIsWrittenOnlyUnderTheRoomsLease`
  - `internal/core/space/store_test.go` `TestTheStatesMoveInTheirOrder`
  - `internal/core/space/store_test.go` `TestARecordClosesOnlyFromAnObservation`
  - `internal/core/space/store_test.go` `TestARecordLeftByACrashBetweenStepsCanBeClosed`

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
  protocol is checked by its peer on its own, as T2's transport is; a descendant is refused.

  Held today for the endpoint's tool transport, the check the mod's endpoint takes over:
  only the transport's own process is served, a process below it is refused before anything
  runs, and the peer is checked again before the answer. Known limit, recorded on October
  13, 2026, owned by S4-5, which builds the mod's endpoint: the same check on every request of
  the mod's protocol on the mod socket.
  Tests:
  - `internal/host/endpoint/transport_test.go` `TestOnlyTheTransportsOwnProcessIsServed`
  - `internal/host/endpoint/transport_test.go` `TestAnAnswerGoesOnlyToTheTransportThatAsked`

- **SP7. A kept checkout is never forgotten and never removed by rewake.** A closed space
  whose checkout is still there is listed to main at its start and by `rewake space list`
  until the checkout is gone; rewake removes no checkout.

  Held today for the listing: a kept checkout is reminded while it is there, and while it
  cannot be looked at, with what was found in it and the Git commands that clear it; one seen
  gone is not; `rewake space list` shows it beside the open and closed spaces; and the package
  that keeps the records runs no command and removes no file. Known limit, recorded on October
  13, 2026, owned by S4-9, which builds closing: the reminder to main at its start, and a
  closing that removes no checkout.
  Tests:
  - `internal/core/space/list_test.go` `TestAKeptCheckoutIsRemindedUntilItIsGone`
  - `internal/core/space/list_test.go` `TestAnUnknownCheckoutIsReminded`
  - `internal/core/space/list_test.go` `TestThePackageRemovesNoCheckout`
  - `internal/cli/space_test.go` `TestSpaceListShowsOpenClosedAndKept`

- **SP9. A hosted worker is never main's descendant.** Its wrapper is reparented away from
  main's wrapper before it is seated, checked by the same ancestry test the authority makes; an
  open that cannot prove it is refused, and nothing the worker controls runs before that proof.
  So main's decision and grant endpoints refuse the worker's commands by the existing rule, and
  no authority rule changes.

  Held today in the part that does not change: the ancestry test walks a process's parents
  to the wrapper and refuses a stranger and a reused pid, and main's decision and grant
  endpoints refuse a process that does not run below main's wrapper. Known limit, recorded
  on October 13, 2026, owned by S4-7, which builds the hosted launch: the detached launcher,
  the refusal when main's wrapper is a subreaper, the ancestry proven by the whole chain with
  an unreadable one refusing the open, and the launch gate before anything the worker
  controls.
  Tests:
  - `internal/infra/proc/lineage_test.go` `TestDescendsWalksParentsToTheAncestor`
  - `internal/infra/proc/lineage_test.go` `TestDescendsRefusesAReusedParent`
  - `internal/core/grant/authority/decide_test.go` `TestAWrapperTakesADecisionOnlyFromBelow`
  - `internal/core/grant/authority/grantauth_test.go` `TestARegistrationFromOutsideTheWrapperIsRefused`
