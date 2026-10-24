---
description: L1–L3: what a launch adds to a harness, what it never changes, and that nothing it writes outlives the run.
---

# Rules: what a launch adds, L1–L3

What rewake adds to a harness's launch.

## The rules

- **L1. The mail is added and nothing else changes.** The person's configuration is
  never edited; what a run needs is passed for that one launch.
  Tests:
  - `internal/adapter/claude/claude_test.go` `TestAddedFlagsStayBeforeTheTerminator`
  - `internal/adapter/defaults_test.go` `TestAnExplicitFlagIsNotReplaced`
  - `internal/adapter/fixture/launch_test.go` `TestALaunchLeavesThePersonsFilesAsTheyWere`
  - `internal/adapter/fixture/launch_test.go` `TestACallerCannotPassTheFixturesOwnFlags`

- **L2. Diagnostics say where, never what**: no configuration content or argument value
  is printed.
  Tests:
  - `internal/adapter/fixture/launch_test.go` `TestALaunchsDiagnosticsNeverCarryWhatTheyPointAt`
  - `internal/host/launch/version_test.go` `TestAVersionRefusalNeverCarriesWhatTheProgramPrinted`

- **L3. Nothing persists past the run**: what a launch writes lives in the run's
  directory and goes with it; after a kill, a later launch removes it once the run is
  proven dead. A run that ends — its harness exiting, a signal it handles, a failed launch —
  removes its directory before the wrapper exits, unless a worker of the run is stalled past
  the bounded wait of its stops; the run then ends as a killed one does. A wrapper killed by
  `SIGKILL` leaves its directory, and its control directory, until a later launch in the
  room proves the run dead by the epoch in the directory's name — the wrapper gone, its pid
  another process's or a zombie's, or another boot — the judgment that prunes a dead record
  (`registry.ObserveRun`). A run still running, one that cannot be told, and an entry that
  names no run are kept; the proof assumes the wrappers' pid namespace, and a room with no
  later launch keeps a killed run's directories.
  Known limit, accepted with the design of the mod's turns: on
  Claude Code the mod keeps the ends its host has not acknowledged in the harness's own store,
  outside the run's directory, and removes those of a killed run only once a later load's
  hello has the host prove that run ended, so they outlive it until a later run on the same
  machine ([claude-turns.md](../claude-turns.md#the-mods-side)). The removal by proof is
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
