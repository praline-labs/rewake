# Rules: what a launch adds, L1–L3

What rewake adds to a harness's launch, in the full wording of
[design-rules.md](../v2/design-rules.md#what-a-launch-adds-l1l3), from
`mail-bridge-launch.md`.

## The rules

- **L1. The mail is added and nothing else changes.** The person's configuration is
  never edited; what a run needs is passed for that one launch. (`mail-bridge-launch.md`
  rule 1; until S9, on Claude Code, one `--plugin-dir`.)
  Tests:
  - `internal/adapter/claude/claude_test.go` `TestAddedFlagsStayBeforeTheTerminator`
  - `internal/adapter/defaults_test.go` `TestAnExplicitFlagIsNotReplaced`
  - `internal/adapter/fixture/launch_test.go` `TestALaunchLeavesThePersonsFilesAsTheyWere`
  - `internal/adapter/fixture/launch_test.go` `TestACallerCannotPassTheFixturesOwnFlags`

- **L2. Diagnostics say where, never what**: no configuration content or argument value
  is printed. (Rule 7.)
  Tests:
  - `internal/adapter/fixture/launch_test.go` `TestALaunchsDiagnosticsNeverCarryWhatTheyPointAt`
  - `internal/host/launch/version_test.go` `TestAVersionRefusalNeverCarriesWhatTheProgramPrinted`

- **L3. Nothing persists past the run**: what a launch writes lives in the run's
  directory and goes with it; after a kill, a later launch removes it once the run is
  proven dead ([the qualified guarantee](../archive/v2/stage3-rundir.md#the-qualified-guarantee)).
  (Rule 8.) Known limit, recorded on October 16, 2026, accepted with S4-6's design: on
  Claude Code the mod keeps the ends its host has not acknowledged in the harness's own store,
  outside the run's directory, and removes those of a killed run only once a later load's
  hello has the host prove that run ended, so they outlive it until a later run on the same
  machine ([stage4-turns.md](../v2/stage4-turns.md#the-mods-side)). The removal by proof is
  held by the tests below and by the mod's own in the harness's test kit ("only the keys of
  runs mod/ended answers ended are removed").
  Tests:
  - `internal/adapter/claude/modhost/turns_test.go` `TestModEndedAnswersOnlyRunsProvenEnded`
  - `internal/adapter/claude/turns_test.go` `TestARunIsProvenEndedOnlyOnProof`
  - `internal/adapter/claude/mod_test.go` `TestTheModsTestsPass`
  - `internal/host/control_test.go` `TestTheControlDirectoryLivesAsLongAsTheSession`
  - `internal/host/control_test.go` `TestAHarnessThatServesNoRequestsGetsNoDirectory`
  - `internal/host/wrap_test.go` `TestSessionIsPublishedWhileRunningAndRemovedAfter`
  - `internal/host/ownership_test.go` `TestCleanupLeavesTheNextOwnerAlone`
  - `internal/host/ownership_test.go` `TestEachRunCleansOnlyItsOwnSocket`
  - `internal/adapter/claude/claude_test.go` `TestLiveSocketIsNeverRemoved`
  - `internal/adapter/claude/claude_test.go` `TestStaleSocketIsReplaced`
  - `internal/host/rundir_test.go` `TestRunDirectoryDiesWithRun`
  - `internal/host/rundir_test.go` `TestFailedLaunchRemovesOnlyItsRunDirectory`
  - `internal/host/rundir_reclaim_test.go` `TestOldRunCleanupPreservesNewOwner`
  - `internal/host/rundir_reclaim_test.go` `TestCrashedRunIsReclaimedOnlyAfterProvenDeath`
  - `internal/host/rundir_test.go` `TestRunSocketsFitAtMaximumNames`
  - `internal/host/harness_watcher_test.go` `TestTheWrapperCountsAServicesHandlersAmongTheRunsWorkers`
  - `internal/adapter/claude/modhost/lifetime_test.go` `TestAWriteStalledAtCloseKeepsTheLeaseUntilItEnds`
  - `internal/adapter/claude/modhost/lifetime_test.go` `TestAHandlerPastTheCloseWritesNothing`

## What returns with the Codex adapter

`mail-bridge-launch.md` rules 2, 4, 5 and 6 are about injecting an MCP server and checking
its name; they return, redesigned, with the Codex adapter (stage 5).
