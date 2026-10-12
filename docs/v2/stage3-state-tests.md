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
| seam (commits 5 and 6) | in `infra/state`, over one list of every write primitive (`primitives_scene_test.go`) the view test runs too: `TestEveryMutationRequiresItsRoomLease` (each primitive writes under the lease and, the lease let go, refuses with `ErrNoLease`, changes nothing and is counted; the bootstrap's alone need none), `TestViewWritePrecedesNoLease`, `TestCrossRoomAndReleasedLeaseCannotWrite` (another room, a room whose name begins the same, each end of a link, a move and a rename, the last of two locks closed), `TestAnAuthorizationDiesWithItsLock` (a lock closed after a repeated `Authorize`, unlocked, or refused a conversion authorizes nothing); the mapped counter's open, store and flush and the listener's bind and close-time unlink are primitives of that list. `TestStateWritesHaveNoBypass` with `TestTheBypassCheckSeesEveryWrite` and `TestTheBypassTableFailsOnAStaleEntry`: the static test below. In `cli`, `TestEveryChangeFormHoldsLeaseThroughItsLastEffect` (every documented change form but a launch, the test holding no lease, refused no write, a swallowed one included, through `state.NoLeaseRefusals`) and `TestAFailedLeaseLeavesNoShellObservation` (R2, with the test's lease held and let go). The adapters' writes end inside their `Close`: `TestNothingOfTheBackendWritesAfterClose` (fixture), `TestNothingOfTheLaneWritesAfterClose` |
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

**The static test** (`internal/infra/state/bypass_test.go`) parses every production file under
`internal/` outside `infra/state`, whatever build tags it carries, so the four build variants
are read at once, the tagged fixture and fault seam among them. It fails on a reference —
a call, a value kept and called later, an argument, through any import name or a dot import
— to a write the seam stands for: `os.WriteFile`, `os.Create`, `os.CreateTemp`, `os.OpenFile`
unless its flag is spelled from read flags alone, `os.Remove`, `os.RemoveAll`, `os.Rename`,
`os.Link`, `os.Symlink`, `os.Mkdir*`, `os.Chmod`, `os.Chown`, `os.Chtimes`, `os.Truncate`,
`os.CopyFS`, `os.OpenRoot`, the `syscall` forms of the same, the owners, times, extended
attributes and nodes among them, with `syscall.Open` read by its flag and `syscall.Mmap` by its
protection, `syscall.Flock`, any method named `Chmod` or `Chown` but a package's function — a
descriptor changes its file's mode and owner whatever it was opened for, a read-only one too —
and a listener whose address is not proven abstract: only a string literal beginning with `@`,
a concatenation whose first operand is one, or a `UnixAddr` whose `Name` is one; a variable or
a call may be a path, whatever literal it holds. Its table
(`bypass_table_test.go`) holds each exception by file, effect, count and reason, and fails on
an effect found more or fewer times than its entry counts, an entry nothing matches and an
entry without a reason. The exceptions, classified by effect and scope, never a whole
adapter: `host/worktree`, which writes beside a checkout and in the user's data directory,
outside the state (its records, serial locks, copies, temporary hook files and removal); the
grant authority's abstract socket; the test helpers that are not `_test.go` files
(`mailtest`, `registrytest`, `buildtest`). Without type information it counts every method
of those two names, whatever its receiver, the safe side; a content write or a truncate
through a descriptor needs a writable one, which only an open the check counts or the
guarded `state.OpenFile` gives. `TestTheBypassCheckSeesADescriptorsMetadataChange` and
`TestTheBypassCheckExcusesOnlyAProvenAbstractAddress` hold both. Its own mutants edit a copy of
the tree rather than pass `-overlay`, since the scanner reads sources from disk.
