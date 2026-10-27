---
description: The adapter API — capabilities offered and live, what the core does without each, the interfaces in internal/adapter, the catalogue, minimum and pinned versions, and adding a harness.
---

# Rules: the adapter API

What an adapter offers the core, as named capabilities, what the core does when an adapter —
or one run of it — lacks one, the catalogue every adapter is named in, and the version a
launch requires. How to add a harness is `AGENTS.md`, "Adding a harness". The rules have no
numbers: each section below is one rule, with the tests that hold it.

## Why capabilities

The core knows no harness: what rewake does with messages is written once, and each harness
joins through an adapter that declares what it can do, so a new harness is added without
touching the core (the owner's decision, [project.md](../project.md#the-philosophy)). Every
capability is a small interface in `internal/adapter` carrying narrow values — never a session
record whole — and the core learns what a harness can do from its offer, never by asserting
its types at a call site.

## The capabilities

An adapter offers named capabilities: Launch, which is required — an adapter without it is no
adapter — and Wake, TurnBoundary, Telemetry, Control and Spaces, each optional. What
an adapter offers is static, read from the catalogue before a launch (`Offers`,
`internal/adapter/offer.go`); what one run serves is the plan its `Launch` returns, and a
capability is live for the run only where the offer and the plan both carry it, so an offer
with nothing in the plan behind it is never taken for served.

| Capability | What the adapter does | Without it, the core |
|---|---|---|
| Launch | describes and starts one run | — |
| Wake | brings a notice to the session: wakes it idle, steers it busy where it can | writes the letter; `send` exits 3, "accepted, no notice can reach this session"; the session reads when it runs `rewake inbox` |
| TurnBoundary | reports a turn's start and end, with the end's answer, and may confirm an end before it closes | publishes no report; a role that owes reports does not launch on it ([below](#turnboundary)) |
| Telemetry | reports context, limits and cost | `list` and main see the state as unknown |
| Control | compacts (with a focus where offered) and interrupts | `rewake compact` or `interrupt` to that session is a wrong call, exit 2, naming the missing verb |
| Spaces | hosts spaces from a main run | `rewake space open` refuses, naming the missing level ([below](#spaces)) |

The core does without what is missing and says so rather than pretending. Every turn end a
boundary reports names its event, with its reason; an end with neither an event nor a known
reason is refused, each reason reports its own outcome, and a turn's start is recorded once.

What each adapter offers today:

| | Wake | TurnBoundary | Telemetry | Control | Spaces | Conversation |
|---|---|---|---|---|---|---|
| Claude Code | yes | reports, after the start, holds none | yes | compact with focus, interrupt | yes | `--session-id` |
| pi | yes | reports, after the start, holds none | yes | compact with focus, interrupt | yes | `--session-id` |
| fixture | yes | reports and holds | yes | compact with focus, interrupt | yes | none |

Tests:
- `internal/host/live_test.go` `TestARunThatReportsNoneHasItsPlanLive`
- `internal/cli/live_set_test.go` `TestTheLiveBoundaryDecidesNotTheOffer`
- `internal/cli/turn_reporting_test.go` `TestAReportingRoleDoesNotLaunchOnAHarnessThatReportsNoTurns`
- `internal/cli/steer_test.go` `TestAHarnessThatCannotTakeItIsAWrongCall`
- `internal/cli/steer_test.go` `TestAVerbTheHarnessDoesNotOfferIsAWrongCall`
- `internal/cli/turn_end_api_test.go` `TestATurnEndWithoutItsEventOrAKnownReasonIsRefused`
- `internal/cli/turn_end_api_test.go` `TestEachEndReasonReportsItsOutcome`
- `internal/host/turn_boundary_test.go` `TestTheWrapperPassesEachEndOnWithItsEventAndReason`

### Launch

Launch is the `Harness` interface of `internal/adapter/plan.go`, shown without its comments:

```go
type Harness interface {
    ID() string
    Title() string
    Summary() string
    Examples() []string
    Notes() []string
    SingleUseFlags() []Flag
    Version() VersionGate
    EnvNotInherited() []string
    Offers() Offer
    Launch(request LaunchRequest) (LaunchPlan, error)
}
```

- `SingleUseFlags` names the flags a harness takes at most once, every spelling of each, so a
  worker alias's flag and one typed after `space open --` for the same parameter replace
  rather than repeat each other; a catalogue test requires the list to be non-empty.
- `Version` is the gate the host reads before the claim ([below](#minimum-versions)), and
  `EnvNotInherited` the variables the host leaves out of `LaunchRequest.Env`.
- `Launch` turns a neutral description — name, role, room, the briefing, the working
  directory, a conversation id, the endpoint's address — and the run directory the host owns,
  where the adapter may write what lives exactly as long as the run, into the plan of one run.
- A briefing that needs a sentence about its own harness — how a session on it commits — gets
  it from the adapter, as `brief.Context.Harness`, a section after the role's own; the role
  texts and the CLI's help name no harness. A help text that needs one takes it from the
  catalogue it was built from (`internal/cli/registry_words.go`).

The optional interfaces beside it:

- `LaunchSettingsHarness` translates an alias's model and effort into harness flags; context
  never reaches it. `host/launch` reads the worker aliases into templates, applying those
  settings, and builds a space's complete worker command, wrapper and fresh conversation id
  included; `test/launch` checks that command against every registered harness, with and
  without a worktree.
- `Shipped` is a harness whose pinned copy is installed with rewake (pi): its `Locate` finds
  the copy and its runtime before the claim, and the host passes the copy to Launch as
  `LaunchRequest.Start`, which `Invocation` puts before the caller's arguments unless
  `--command` names another program.
- `WorktreeHarness` gives rewake's worktree to a launch: `WorktreeFlag()`, the spelling the
  launch command takes for itself; `WorktreeRefusal(args)`, what the harness cannot do in a
  checkout of its own, asked before anything is made (a continued conversation; for Claude
  Code its own `-w` and `--tmux` beside rewake's); and `LaunchDirectory(args)`, the directory
  the launch would work in and the arguments without what chose it. The launch then makes the
  checkout in `internal/host/worktree` and starts the session inside it
  ([worktree.md](../worktree.md)). Claude Code implements it; a worker that may enter a
  checkout is told it starts in main's and enters before its first change
  ([worktree-tools.md](../worktree-tools.md#a-workers-entry-enter_worktree--true-and-worktree_enter)).
- `ThreadTracker` follows the conversation from the outside, given the run's process.
- `Resumer` gives the arguments after `rewake <harness>` that continue a conversation by its
  id, for herdr's resume command ([launch.md](../launch.md#herdr)). Claude Code answers
  `--resume <id>` and pi `--session-id <id>`; pi's `--resume` opens a picker.

The plan carries one run's capabilities as interfaces the wrapper calls and never asserts on
the adapter: `Service`, started before the harness and closed with it, a `ProcessService`
when it owns a process; `Wake`; `TurnBoundary`, handed a `TurnHandler` before the service
starts; `Telemetry`; `Thread`, the run's conversation; and a `LiveReporter` where the run
proves what it serves ([below](#two-levels-offered-and-live)).

### The harness process

A service that serves the harness process itself — an endpoint whose every request is checked
by its peer — implements `HarnessWatcher`: the host names the process it started, with its
start time, as soon as `exec` returns, before it waits for anything else. Until then a request
is held, within a bound, and refused after it; from then on a peer that is not that process,
alive with that start, is refused. A launch through `--command` names the wrapper script it
started, so a harness the script runs as a child is not the named process and is refused: such
a run's mod is never admitted, and its run takes no tasks — a documented limit, not a refusal
([claude-turns.md](../claude-turns.md#readiness-and-admission)). No test holds the
`--command` cases; they were run live with the installed harness.

### Handlers that write the room

A service whose request handlers write the room — a record in the run's directory — implements
`WorkerCounter`: before it starts, the host hands it the run's `Workers`, and each handler is
admitted among the run's workers and dismissed when it returns. Admission fails once the run
begins to end, and the request is refused. The service's close is bounded: it stops admission,
waits for the handlers in flight within its bound and then cuts the rest, and a handler that
outlasts it — stalled in the filesystem, say — stays counted, so the run's end holds the room's
lease and its directory until that handler's last write rather than the close holding the
wrapper's exit ([state.md](state.md)).

### Wake

Given a notice — built by `mail.Notice`, never the message's text — and the run, it delivers
and answers delivered, held (with the harness's reason) or failed, and later, for a held one,
the outcome; a `Reserver` and a `Receiver` where the run answers before the letter. The host's
delivery loop decides what to send and when; the adapter only carries it. Whether a busy
session is steered or queued is part of the answer, because the notice texts promise "active
work is steered, idle work is woken". A run without a `Wake` answers each delivery that no
notice can reach the session. How each harness carries a notice is
[delivery-adapters.md](../delivery-adapters.md).

### TurnBoundary

Reports, for the session's own conversation (never a subagent's): a turn started (its id, and
when on the harness's own clock where the harness says); a turn ended; and, where the adapter
can hold an end open, asks before it closes whether it may. Every end is a `TurnEnd`
(`internal/adapter/turn.go`): the event it came from, the end's reason — answered, aborted,
refused or failed — and the answer in the harness's words, with the scope captured from the
run's read clock. The core derives the report from the reason: answered reports finished,
aborted stopped, failed and refused an error ([turn-outcomes.md](../turn-outcomes.md)). An
adapter that can hold an end confirms it through `TurnHandler.Confirm` rather than publishing
it, so the core can continue the turn instead of closing it; one that cannot says so in its
offer (`HoldsEnd`), and the core then cannot keep a task open across an unmarked interim end.
A turn's start goes to `TurnHandler.Started`, and the wrapper records a timed start once
(`mail.Box.RecordTurnStart`): it is the evidence a pending mark of an earlier turn is judged by
when that turn's end was lost
([delivery-turn-end.md](../delivery-turn-end.md#pending-marks)).

Without a live TurnBoundary a session's turn ends report nothing, and a worker's contract —
"end your turn; your final reply goes to the sender" — does not hold. So a launch of a role
that owes reports refuses when the boundary cannot be live, naming the cause; once running,
the session record says the boundary is not live, and a task or question sent to it fails,
exit 1, naming why. Notifies and reads still work through the shell; the shell never stands
in for the boundary.

Claude Code's boundary reports and holds no end, proven by its run's mod after the harness
starts, so a role that owes reports launches on it. A cause the adapter knows will keep the mod
from loading — the mod socket's path past what a unix socket allows, no run directory or inbox
socket, the mod's files not written, its endpoint not listening — refuses such a launch before
the harness starts, exit 1, naming the cause by where it is, and leaves a silent role going
with a note.

Tests:
- `internal/adapter/claude/turns_test.go` `TestTheOfferReportsTurnsAfterTheStartAndHoldsNone`
- `internal/adapter/claude/turns_test.go` `TestAKnownCauseWithholdsTheBoundary`
- `internal/host/after_start_test.go` `TestAKnownCauseRefusesAReportingRoleBeforeTheStart`

### Telemetry

Context tokens, window and percent; rate limits; cost; folded into the session-state snapshot
(`core/session`), as [session-state.md](../session-state.md) describes. Reading a transcript is
not a source.

### Control

Each verb is offered and answered on its own: compact, with a focus where the harness can pass
one for a single compaction, and interrupt the running turn. Requests travel as files in the
run's control directory (`core/control`); the adapter decides how it hears them. A verb the
adapter does not offer is refused by the CLI before a file is written
([remote-control.md](../remote-control.md)).

### Spaces

A main run that hosts spaces: it opens a worker in a pseudo-terminal the space host holds,
which rewake's shell shows beside main ([space.md](space.md), [shell.md](../shell.md)).
Three levels decide it, and a refusal names the one that is missing:

- **The role.** `Role.Spaces` says a session of the role opens spaces: main alone. Opening one
  from another role is a wrong call, exit 2.
- **The offer.** `Offer.Spaces` says a run of the harness can host them. A harness that does
  not offer them is a wrong call too, exit 2, naming the harness; so is a session recorded by a
  build whose catalogue has no such harness.
- **The live set.** Spaces are never planned: a run proves them, so an adapter that offers them
  has its plan carry a `LiveReporter`, and a catalogue test holds it. A run of a role that
  opens spaces reports `spaces` once it can host them and withdraws it when it loses them. A
  main session without them live is refused as a target, exit 1: its run has not proven them
  or has lost them, which a later run may not repeat.

On Claude Code the proof is the mod's hello: an admitted hello on a main run makes Spaces live,
and the lapse of its heartbeat withdraws them with its turn boundary; a run whose mod did not
load never has them. On pi it is the extension's hello, and its disconnect withdraws them; a
`/new` or `/reload` reconnects, and the workers, whose hosts follow main's run rather than its
live set, go on. The fixture proves them through the host's neutral seam. The refusal is
`spacesRefusal` in `internal/cli`, which `rewake space open` asks before anything is written.

Tests:
- `internal/cli/space_refusal_test.go` `TestASpaceIsRefusedByTheMissingCapability`
- `internal/adapter/catalog/offers_test.go` `TestEveryHarnessThatOffersSpacesHasItsRunProveThem`

### Conversation names

A space's record keeps its worker's conversation id, so the worker can be found and restored.
rewake chooses the id, a UUID, before the launch and hands it in `LaunchRequest.Conversation`;
the adapter's Launch passes it with the harness's flag. Every adapter answers in its offer
whether it can, either way — `ConversationOffer.Flag`, or `Unnamed` with the reason there is
none — because the zero value would read as both "cannot" and "forgot to say". A catalogue test
holds the answer: exactly one of the two; a flag taken once with a value, carried by Launch
with the id; and, without one, Launch refusing an id rather than dropping it. Claude Code
answers `--session-id <uuid>` (`claude --help`, 2.1.296) and refuses rewake's id beside
arguments that already name or continue a conversation; pi answers `--session-id`; the fixture
answers that it takes no id.

Tests:
- `internal/adapter/catalog/offers_test.go` `TestEveryHarnessAnswersWhetherItNamesItsConversation`
- `internal/adapter/claude/conversation_test.go` `TestAConversationIdIsRefusedBesideOneTheArgumentsName`

## Two levels: offered and live

A capability is **offered** by the adapter, known from the catalogue before a launch, and
**live** for one run once the run proves it: Claude Code offers TurnBoundary, but a run whose
mod did not load does not have it
([claude.md](../claude.md#when-the-mod-does-not-load)). A run proves what it
serves through its plan's `LiveReporter` (`internal/adapter/run.go`): the whole set once the
exchange has run and again at every change, a lost capability at once. A run whose plan
carries no reporter has live what its plan carries, from its start.

The host records each set in the session record (`registry.Session.Live`, written under the
name's lock by `registry.Modify`), and every decision another session makes about the run
reads it, never the offer: a letter to a run without a live Wake is accepted, exit 3, and
waits in its inbox; a task or a question to one without a live TurnBoundary fails, exit 1,
nothing sent; a compaction or an interrupt of one without a live Control fails, exit 1,
nothing asked; a launch of a role that owes reports waits for its TurnBoundary up to the
readiness bound and is refused after. Liveness never changes the run: a capability withdrawn
and made live again leaves its epoch, name and role as they were. A set whose write fails is
not taken for recorded: the run's own view takes it at once, and the record is written again,
once at once and then with retries, until it lands or the run ends — never once the record has
become a later run's.

The wait before the start holds for a boundary the run proves before its harness starts. One
the run proves only once its harness runs (`TurnBoundaryOffer.AfterStart`: Claude Code's mod,
pi's extension) is not awaited before the start: the harness starts, the set is Wake alone
until the proof, and the run's availability waits for the boundary up to its own bound, past
which availability goes ahead with the boundary not live. What a mod proved is withdrawn when
its heartbeat lapses — within 30 s of its last poll — and made live again by a later admitted
hello; a hello of a load a later one superseded, or with a lower load number than the current
load's — the number each load takes from its run's counter before its first hello — is
refused and changes nothing, however late it arrives, and each report carries the number of
the transition it follows, so a report of an earlier one changes nothing. An end that arrives
after a withdrawal is still taken: a withdrawn boundary refuses new tasks, it drops no report
([claude-turns.md](../claude-turns.md#the-live-set)).

Tests:
- `internal/core/registry/live_test.go` `TestModifyChangesTheRecordAsItStands`
- `internal/host/live_test.go` `TestTheLiveSetIsRecordedAndADropLeavesTheRun`
- `internal/host/live_test.go` `TestTheLiveSetDoesNotWriteAnotherRunsRecord`
- `internal/host/live_test.go` `TestAWithdrawalThatFailsToLandIsWrittenAgain`
- `internal/host/live_test.go` `TestAReportingRoleWaitsForItsBoundary`
- `internal/host/live_test.go` `TestABoundaryNeverReportedIsRefusedAtTheBound`
- `internal/host/live_test.go` `TestADeliveryWithoutALiveWakeWaits`
- `internal/host/live_test.go` `TestWithoutALiveWakeNothingIsReservedOrPinned`
- `internal/host/after_start_test.go` `TestABoundaryProvenAfterTheStartStartsTheHarnessAndAvailabilityWaitsForIt`
- `internal/host/after_start_test.go` `TestAvailabilityGoesAheadPastTheBoundWithTheBoundaryNotLive`
- `internal/host/after_start_test.go` `TestABoundaryWithdrawnAfterAvailabilityLeavesWakeUntilProvenAgain`
- `internal/host/delivery/reservation_test.go` `TestNoReservationTakesThePlainPath`
- `internal/adapter/claude/mod_live_test.go` `TestTheSetIsWakeUntilTheHelloThenTheBoundaryAndSpacesOnMain`
- `internal/adapter/claude/mod_live_test.go` `TestALapseLeavesWakeAndANewHelloRestoresTheSet`
- `internal/adapter/claude/modhost/alive_test.go` `TestALapseWithdrawsWhatTheModProvedAndANewHelloRestoresIt`
- `internal/adapter/claude/modhost/alive_test.go` `TestAHelloRetriedAfterItsAnswerWasLostKeepsItsGeneration`
- `internal/adapter/claude/modhost/alive_test.go` `TestAnEarlierLoadsHelloAfterALaterOnesIsRefused`
- `internal/adapter/claude/modhost/alive_test.go` `TestAHelloOfAnotherLoadWithTheCurrentNumberBindsAsTheLater`
- `internal/adapter/claude/modhost/alive_test.go` `TestAPollDelayedAcrossANewHelloOfTheSameLoadIsRefused`
- `internal/adapter/claude/modhost/alive_test.go` `TestALapseTimerOfAnEarlierTransitionDoesNothing`
- `internal/adapter/claude/modhost/alive_test.go` `TestAnEndAfterALapseIsTakenAndMakesNothingLive`
- `internal/adapter/claude/modhost/alive_test.go` `TestALapseAfterTheCloseReportsNothing`
- `internal/adapter/claude/modhost/alive_test.go` `TestAHeldPollHoldsNoLock`
- `internal/cli/live_set_test.go` `TestASendToARunWithoutALiveWakeIsAcceptedUnwoken`
- `internal/cli/live_set_test.go` `TestATaskToARunWithoutALiveBoundaryFails`
- `internal/cli/live_set_test.go` `TestABoundaryWithdrawnWhileStdinIsReadRefusesTheTask`
- `internal/cli/live_set_test.go` `TestSteeringARunWithoutALiveControlFails`
- `internal/adapter/fixture/live_test.go` `TestTheRunReportsItsLiveSetAsItChanges`
- `internal/adapter/fixture/live_test.go` `TestTheFirstReportLeavesOutWhatTheExchangeDidNotProve`
- `internal/adapter/fixture/live_test.go` `TestControlIsCarriedOnlyWhileLive`
- `test/workflow/fixture_live_test.go` `TestFixtureLive`

## The catalogue

`adapter/catalog` has one constructor, `catalog.New()`, that returns an `adapter.Catalog` —
the adapters in the order the guide shows them — and is the only package that names every
adapter. `cmd/rewake` builds the value and passes it to `cli.Run`, which derives the command
table from it for that invocation, hands each launch its adapter and the host the interfaces,
and asks it through `Catalog.Offers(id)` before it writes anything. No package holds adapters
at package level and none registers one from `init`, so a test builds a catalogue of its own.
A harness is fully described or absent: the launch command, the guide's entry, its help page,
the step in FLOW and the `harnesses` field of the machine form all derive from the value. The
fixture harness is in the catalogue only in a build with its tag; it offers every capability
the core has a part in, so the workflow suite holds the core's part of each
([testing-cases.md](../testing-cases.md)).

Tests:
- `internal/adapter/catalog/catalog_test.go` `TestEveryHarnessNamesItsSingleUseFlags`
- `internal/adapter/catalog/catalog_test.go` `TestEveryWorktreeFlagIsTakenOnce`
- `internal/adapter/catalog/fixture_absent_test.go` `TestTheFixtureIsAbsentWithoutItsTag`
- `internal/adapter/catalog/fixture_present_test.go` `TestTheFixtureIsPresentWithItsTag`
- `internal/cli/cli_test.go` `TestGuideJSONCarriesTheTable`
- `internal/cli/cli_test.go` `TestHelpForEveryCommand`

## Minimum versions

Each adapter declares the oldest version of its harness it starts and how to ask the installed
program for its version. A launch reads it before the claim, running the program the launch
will start as a bounded probe in a process group of its own, killed with its group, whose
output is never shown: below the minimum it refuses, exit 1, naming the installed version and
the minimum ("Claude Code 2.1.280 is installed; rewake needs 2.1.287 or later"); when no
version can be read it refuses, exit 1, saying how the version was asked and what came of it;
at or above it the launch goes on. A version newer than any a live check ran on is not refused
and not noted. A gate is a minimum or nothing — an adapter without one reads nothing — and
never an exact version. Claude Code's minimum is 2.1.287, pi's 1.2.0.

The one exact version is a copy shipped with rewake (owner decision, October 19, 2026): pi is
installed and updated with rewake, so the room never runs a pi rewake was not checked with.
Without `--command` the launch starts the copy `Locate` finds, reads its runtime's version
against the runtime's minimum, then the copy's against the gate's `Pinned`, and refuses any
other version, exit 1, before the claim, naming the one found and the pinned one. A pi named
with `--command` is held to the minimum, with a note when it is not the pinned version
([launch.md](../launch.md#pi)).

The table of oldest supported versions in
[legacy.md](../legacy.md#oldest-supported-versions) is the floor of the legacy marks, and
nothing reads it against the adapters' minimums. The mod API promises no stability: it changes
in patch releases too, so mod work compares the `mods` skill's version with the installed one,
and each probe result is written with the version it ran on.

Tests:
- `internal/host/launch/version_shipped_test.go` `TestTheShippedCopyRunsOnlyAtItsPinnedVersion`
- `internal/host/launch/version_test.go` `TestAVersionBelowTheMinimumIsRefused`
- `internal/host/launch/version_test.go` `TestAnUnreadableVersionIsRefused`
- `internal/host/launch/version_test.go` `TestAVersionRefusalNeverCarriesWhatTheProgramPrinted`
- `internal/host/launch/version_test.go` `TestAGateWithoutAMinimumReadsNothing`
- `internal/host/launch/probe_test.go` `TestAProbePastItsBoundIsEndedWithItsGroup`
- `internal/host/launch/probe_test.go` `TestAProbeIsNotHeldByItsOutputPastTheGrace`
- `internal/adapter/claude/version_test.go` `TestTheGateNamesTheMinimumAndHowToAsk`
- `internal/adapter/fixture/launch_test.go` `TestTheGateNamesTheMinimumAndHowToAsk`
