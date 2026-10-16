# Rules: the adapter API

What an adapter offers the core, how the core does without what one lacks, the catalogue
every adapter is named in, and the version a launch requires. The design is
[design-api.md](../v2/design-api.md), and how to add a harness is `AGENTS.md`, "Adding a
harness". What one run has proven of its adapter's offer — the live set every other
session's decision reads — is the host's rule "The live set" in [host.md](host.md). The
rules have no numbers: each section below is one rule, ends with the tests that hold it,
and is known to the rules test by its heading.

## The capabilities

An adapter offers named capabilities, each a small interface in `internal/adapter`
carrying narrow values, never a session record whole: Launch, which is required — an
adapter without it is no adapter — and Wake, ToolTransport, TurnBoundary, Telemetry,
Control, Permissions, PersonUI and Spaces, each optional. What an adapter offers is static,
read from the catalogue before a launch (`Offers`); what one run serves is the plan its
`Launch` returns, and a capability is live for the run only where the offer and the plan
both carry it, so an offer with nothing in the plan behind it is never taken for served.
The core does without what is missing and says so rather than pretending: a role that
owes reports does not launch on a harness that offers no turn boundary, and says from
when it will; a compaction or an interrupt of a session whose harness does not offer the
verb, or its focus, is a wrong call, exit 2, with nothing asked; a task granting a
directory to a session whose harness takes none fails, exit 1, nothing sent; opening a space
from a role that opens none, or on a harness that offers none, is a wrong call, exit 2, and
on a run that has not proven its spaces, or has lost them, fails, exit 1; spaces are never
planned, and a main run on Claude Code has them once its mod's hello is admitted; a clause
about the harnesses that lack a capability agrees with how many lack it. Every turn end
a boundary reports names its event, with its reason; an end with neither an event nor a
known reason is refused, each reason reports its own outcome, and a turn's start is
recorded once, opening the turn where the run has a tool.

Claude Code reports its turns from S4-6: it offers a turn boundary that reports and does not
hold an end, proven by its run's mod after the harness starts, so a role that owes reports
launches on it. A cause the adapter knows will keep the mod from loading — today the mod
socket's path past what a unix socket allows, no run directory or inbox socket, the mod's
files not written, its endpoint not listening — refuses such a launch before the harness
starts, exit 1, naming the cause by where it is, and leaves a silent role going with a note. A
launch through `--command` whose script runs the harness as its child rather than by `exec` is
never admitted, so its run takes no tasks; that is a documented limit, not a refusal
([stage4-turns.md](../v2/stage4-turns.md#readiness-and-admission)). Known limit, recorded on
October 16, 2026: no test holds the `--command` cases, which S4-6's part C runs live with the
installed harness (stage 4).

Tests:
- `internal/host/live_test.go` `TestARunThatReportsNoneHasItsPlanLive`
- `internal/cli/live_set_test.go` `TestTheLiveBoundaryDecidesNotTheOffer`
- `internal/cli/turn_reporting_test.go` `TestAReportingRoleDoesNotLaunchOnAHarnessThatReportsNoTurns`
- `internal/cli/steer_test.go` `TestAHarnessThatCannotTakeItIsAWrongCall`
- `internal/cli/steer_test.go` `TestAVerbTheHarnessDoesNotOfferIsAWrongCall`
- `internal/cli/send_dir_test.go` `TestASessionThatTakesNoDirectoryIsSentNoGrant`
- `internal/cli/space_refusal_test.go` `TestASpaceIsRefusedByTheMissingCapability`
- `internal/adapter/claude/mod_live_test.go` `TestTheSetIsWakeUntilTheHelloThenTheBoundaryAndSpacesOnMain`
- `internal/adapter/claude/turns_test.go` `TestTheOfferReportsTurnsAfterTheStartAndHoldsNone`
- `internal/adapter/claude/turns_test.go` `TestAKnownCauseWithholdsTheBoundary`
- `internal/host/after_start_test.go` `TestAKnownCauseRefusesAReportingRoleBeforeTheStart`
- `internal/cli/registry_words_test.go` `TestADecliningClauseAgreesWithHowManyDecline`
- `internal/cli/turn_end_api_test.go` `TestATurnEndWithoutItsEventOrAKnownReasonIsRefused`
- `internal/cli/turn_end_api_test.go` `TestEachEndReasonReportsItsOutcome`
- `internal/host/turn_boundary_test.go` `TestTheWrapperPassesEachEndOnWithItsEventAndReason`
- `internal/host/turn_start_test.go` `TestABoundaryWithAToolRecordsItsStartOnceAndOpensItsTurn`
- `internal/host/turn_start_test.go` `TestABoundaryWithoutAToolRecordsItsStart`

## The catalog

`adapter/catalog` has one constructor that returns the catalogue value — the adapters in
the order the guide shows them — and is the only package that names every adapter;
`cmd/rewake` builds the value and hands it to the CLI, which hands each launch its
adapter and the host the interfaces. There is no registry: no adapter package declares
`init`, and no package-level variable holds adapters outside the constructor. A harness
is fully described or absent: the launch command, the guide's entry, its help page and
the `harnesses` field of the machine form all derive from the value; each adapter names
the flags its harness takes at most once, every spelling of them, and its own
configuration directories, which no task may grant, and the catalogue protects those of
the harnesses it has no adapter for as well. Each adapter answers whether a launch can name
its conversation — a flag taken once with a value, which its Launch carries with rewake's
id, or the reason there is none, and then its Launch refuses an id — and one that offers
spaces has its run report its live set, since spaces are never planned. The fixture harness is in the catalogue only
in a build with its tag.

Tests:
- `internal/adapter/catalog/catalog_test.go` `TestEveryHarnessNamesItsSingleUseFlags`
- `internal/adapter/catalog/catalog_test.go` `TestEveryHarnessNamesItsProtectedDirs`
- `internal/adapter/catalog/catalog_test.go` `TestTheCatalogueProtectsTheHarnessesItHasNoAdapterFor`
- `internal/adapter/catalog/catalog_test.go` `TestEveryWorktreeFlagIsTakenOnce`
- `internal/adapter/catalog/offers_test.go` `TestEveryHarnessAnswersWhetherItNamesItsConversation`
- `internal/adapter/catalog/offers_test.go` `TestEveryHarnessThatOffersSpacesHasItsRunProveThem`
- `internal/adapter/claude/conversation_test.go` `TestAConversationIdIsRefusedBesideOneTheArgumentsName`
- `internal/adapter/catalog/fixture_absent_test.go` `TestTheFixtureIsAbsentWithoutItsTag`
- `internal/adapter/catalog/fixture_present_test.go` `TestTheFixtureIsPresentWithItsTag`
- `internal/cli/send_dir_test.go` `TestAGrantRefusesWhatTheCatalogueProtects`
- `internal/cli/cli_test.go` `TestGuideJSONCarriesTheTable`
- `internal/cli/cli_test.go` `TestHelpForEveryCommand`
- `internal/layout_test.go` `TestNoAdapterIsRegistered`

## Minimum versions

Each adapter declares the oldest version of its harness it starts and how to ask the
installed program for its version. A launch reads it before the claim, running the
program the launch will start as a bounded probe whose output is never shown: below the
minimum it refuses, exit 1, naming the installed version and the minimum; when no version
can be read it refuses, exit 1, saying how the version was asked and what came of it; at
or above it the launch goes on. A gate is a minimum or nothing — an adapter without one
reads nothing — and never an exact version. Claude Code's minimum is 2.1.287.

Known limits, recorded on October 13, 2026 at stage 3's close, both owned by stage 4,
where the Claude Code adapter is built: the design's note of a version newer than the last
one a live check ran on is not built; and the table of oldest supported versions in
[legacy.md](../legacy.md#oldest-supported-versions) does not yet hold the adapters'
minimums, nor does a test read the two as one fact
([design-api.md](../v2/design-api.md#minimum-versions)).

Tests:
- `internal/host/launch/version_test.go` `TestAVersionBelowTheMinimumIsRefused`
- `internal/host/launch/version_test.go` `TestAnUnreadableVersionIsRefused`
- `internal/host/launch/version_test.go` `TestAVersionRefusalNeverCarriesWhatTheProgramPrinted`
- `internal/host/launch/version_test.go` `TestAGateWithoutAMinimumReadsNothing`
- `internal/adapter/claude/version_test.go` `TestTheGateNamesTheMinimumAndHowToAsk`
- `internal/adapter/fixture/launch_test.go` `TestTheGateNamesTheMinimumAndHowToAsk`
