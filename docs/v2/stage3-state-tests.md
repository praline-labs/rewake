# Stage 3: the tests of state and builds

The tests that hold each decision of [stage3-state.md](stage3-state.md), by area and by the
commit of S17 that brings them, and the earlier oracles the change keeps.

Each decision is held by a test that fails on a mutant of it, one per decision through
`-overlay`. Process races use real separate binaries and controlled pauses, never
goroutines sharing one process's lease.

| Area | Tests |
|---|---|
| identity (commit 2) | `TestBuildIDIsExecutableContent` (a copy equal, a changed byte different, the installed path replaced or removed still hashes the running image); `TestUnreadableImageHasNoEffects`; `TestVersionJSONCarriesBuildID`, the human line unchanged |
| lease (commit 2) | `TestSharedWritersExcludeBuildSwitch`, `TestLaunchReplacesMissingOrInvalidBuildOnlyExclusively`, `TestDowngradeRechecksBuild`, `TestReplacedLeaseLockIsReopened`, `TestRemovedLeaseLockIsNotRecreatedByAWriter`, `TestIncompleteRoomWriterCreatesNothing`, `TestHolderDiagnosticsUseDeviceAndInode`, `TestLeaseDescriptorsAreNotInherited`, `TestAKeptLeaseOutlastsItsOwnersRelease`; every contention bounded, a writer beside a stalled switch refused after its bound (`TestAStalledSwitchBoundsItsWriters`) |
| process races (commit 2, `test/leaserig`) | `TestDifferentImagesWithSameVersionCannotShareRoom` (one tree built twice, the second with a build time passed in by `-ldflags`: the same version, different ids, the second refused while a session of the first runs), `TestArtifactCopyCanJoinRoomAndUseTools` (the tool's peer takes a copy too: `TestPeerCheckIsTheBuildsContent`), `TestOldCommandBlocksNewBuildUntilExitOrKill`, `TestOrphanChildRefusesAfterBuildSwitch`, `TestConcurrentLaunchesOfOneBuildAllTakeTheRoom` (D4a: launches of one build held together after reading a missing record, or another build's with nothing holding it, then let go at once, all take the room, repeated); `TestHostLeaseOutlivesCleanup` in `host`, the lease taken before the claim and released after the record and the socket are gone, and `TestRunKeepsTheLeaseUntilItsWorkersStop`, after a worker paused in a write past every bounded stop |
| both routes (commit 2) | `TestBothRoutesRefuseAnotherBuildsRoom` (the shell and the tool refused in a room of another build, the tool before its ticket is confirmed), `TestChangeFormsOnAnEmptyBaseCreateNothing` |
| bootstrap (commit 2) | `TestBootstrapRecoversAfterEveryStep`, `TestBootstrapRefusesUnsafeExistingComponents`, `TestBootstrapSyncFailureStopsBeforeClaim`, `TestBootstrapRetryReestablishesAFailedSync`, `TestBootstrapFlushesALevelAnotherLaunchMade`, `TestOnlyAcquireCallsBootstrapPrimitives` with `TestBootstrapCallerCheckSeesEveryReference`: a kill or a failure at each create, lock and record step, and no session record after one |
| roots (commit 3) | `TestTwoRoomsUseOnlyV2`, `TestChildBaseRoundTripAndGrantAuthority` (a shell of the session and a tool call's child read the exported base; their `Dir()` is the wrapper's room byte for byte, one `v2` in it; the grant authority's address, which hashes the room, is the wrapper's), `TestRootInverseRejectsLegacyOrMalformedRoom` and, for a worktree owner of another base or of 1.x, `TestWorktreeOwnerRootIsChecked`, `TestResolversNeverCreate`, `TestOrdinaryCommandsNeverTouchLegacyTree` |
| views and forms (commits 1 and 3) | `TestEveryParsedFormHasAccessClass`, `TestEveryViewLeavesMissingAndExistingStateUnchanged`, `TestForeignBuildViewsWarnOnce`, `TestViewWriteAttemptFailsEvenIfIgnored` (and in `infra/state`, `TestEveryWritePrimitiveRefusesInViewMode`, `TestTryHoldRefusesInViewMode`), `TestToolOutputContinuationHoldsLease`: guide, help and version, `--json`, every command's help page, unknown commands and flags and stray words, dead and unknown records, a missing base, long output, a stalled exclusive holder |
| seam (commits 5 and 6) | `TestEveryMutationRequiresItsRoomLease`, `TestViewWritePrecedesNoLease`, `TestCrossRoomAndReleasedLeaseCannotWrite`, `TestStateWritesHaveNoBypass` (the static test fails on an alias, a function value and a stale exception), `TestEveryChangeFormHoldsLeaseThroughItsLastEffect`: raw descriptors, mapped writes, both ends of a rename, the close-time unlink, the fixture's and the fault seam's callbacks |
| L3 (commit 4) | `TestRunDirectoryDiesWithRun`, `TestFailedLaunchRemovesOnlyItsRunDirectory`, `TestOldRunCleanupPreservesNewOwner`, `TestCrashedRunIsReclaimedOnlyAfterProvenDeath`, `TestRunSocketsFitAtMaximumNames` |

These keep their oracles: `TestAdmissionIsInsideTheSection`,
`TestTheSweepWaitsForAnAttemptsEvidence`, `TestAKillAtAnyWriteLandsTheReportAtMostOnce`,
`TestTwoMailboxesPublishingToEachOtherDoNotDeadlock`,
`TestAReportToItselfIsPublishedInsideItsOwnLock`,
`TestAPublicationReadsTheRunWithoutCleaningUp`, `TestTheLockedEntryPointsKeepTheirCallers`,
`TestAStaleSweeperRetiresNothing`, `TestASweepWithoutTheLockRetiresNothing`,
`TestADeferredSettlementGetsAnotherPass`, `TestModifyChangesTheRecordAsItStands`,
`TestTheLiveSetIsRecordedAndADropLeavesTheRun`, `TestTheLiveSetDoesNotWriteAnotherRunsRecord`,
`TestTheHeartbeatDoesNotBringAMarkBack`, `TestALongAnswerComesInParts`,
`TestEveryTextPartNamesItself`, `TestCleanupLeavesTheNextOwnerAlone`,
`TestEachRunCleansOnlyItsOwnSocket`; the read clock's order against a held end, a
mailbox's stop, T5–T8 and the shell channel's evidence too. The bootstrap and the lease
never manufacture a shell observation for a refused call, a failure no more than a
success (`TestAFailedLeaseLeavesNoShellObservation`). The workflow suite and the tool rig
pass on the new roots with their controls still failing their mutants; the crosswise
check runs when a control changes.
