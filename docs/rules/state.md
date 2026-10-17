# Rules: state and builds

Where rewake 2.0 keeps its state, how one build holds a room, which calls may change the
state and through what, and how the state of 1.x is cleared. The design is
[design-state.md](../v2/design-state.md); how stage 3 built it, with every caller and
every write it routes, is [stage3-state.md](../v2/stage3-state.md), the clearing's
protocol [stage3-upgrade.md](../v2/stage3-upgrade.md), and the run directory, which goes
with its run, is L3 in [launch.md](launch.md). The rules have no numbers: each section
below is one rule, with the tests that hold it. That a tool call keeps the same build is T10 in [tools.md](tools.md).

## The state root

The base is `$REWAKE_DIR`, absolute, or `rewake-<uid>` in the system's temporary
directory: a directory, not a symbolic link, owned by the user, mode 0700. Inside it 2.0
keeps everything under its root `v2/`, and each room at `v2/rooms/<room>`, so 1.x never
reads 2.0's records and 2.0 reads the 1.x tree only to clear it. The inverse is checked,
never counted: a room's root is its grandparent only when its parent is named `rooms` and
its grandparent `v2`, an error otherwise, and every caller handles that error rather than
substituting the ambient `REWAKE_DIR`. A child of the wrapper — a shell of the session, a
tool call's CLI — resolves the same room from the base the wrapper exported, byte for
byte, so a grant authority's address, which hashes the room, is the wrapper's. **No
resolver creates anything**: the base, the root and the room are made only by a launch's
bootstrap, under the room's lease, each level flushed, recovering after a failure at any
step and refusing a component that exists but is unsafe. The worktrees and their records
stay in the user's data directory, outside the base.

Tests:
- `internal/infra/state/rooms_test.go` `TestResolversNeverCreate`
- `internal/infra/state/rooms_test.go` `TestRootInverseRejectsLegacyOrMalformedRoom`
- `internal/cli/roots_test.go` `TestTwoRoomsUseOnlyV2`
- `internal/cli/roots_test.go` `TestWorktreeOwnerRootIsChecked`
- `internal/cli/roots_test.go` `TestOrdinaryCommandsNeverTouchLegacyTree`
- `internal/host/roots_test.go` `TestChildBaseRoundTripAndGrantAuthority`
- `internal/cli/lease_test.go` `TestChangeFormsOnAnEmptyBaseCreateNothing`
- `test/leaserig/lease_test.go` `TestBootstrapRecoversAfterEveryStep`
- `internal/infra/state/bootstrap_test.go` `TestBootstrapRefusesUnsafeExistingComponents`
- `internal/infra/state/bootstrap_test.go` `TestBootstrapSyncFailureStopsBeforeClaim`
- `internal/infra/state/bootstrap_test.go` `TestBootstrapRetryReestablishesAFailedSync`
- `internal/infra/state/bootstrap_test.go` `TestBootstrapFlushesALevelAnotherLaunchMade`
- `internal/infra/build/primitives_test.go` `TestOnlyAcquireCallsBootstrapPrimitives`
- `internal/infra/build/primitives_test.go` `TestBootstrapCallerCheckSeesEveryReference`

## The build's identity

All sessions of a room, and every rewake process acting for them, are one build, and the
build is the artifact, not its version line: the **build id** is the SHA-256 of the
running executable's content, read once per process through `/proc/self/exe`, which
opens the image the process runs even after the file at the install path is replaced or
removed. A copy of the same artifact has the same id and is admitted; any other bytes
are another build, two builds of one dirty tree included, though they print the same
`rewake --version` line. An image that cannot be read gives no id, and the process
refuses before its first effect, exit 1, naming why. `--version --json` carries the id;
the human line is unchanged. **The id stays the SHA-256 of the image whatever its cost
measures**: nothing switches to another identity on a threshold, and a weaker identity
would be the owner's decision on a separate proposal that states its guarantee and the
adversarial test it fails. The cost was measured on October 12, 2026: a median of
2.76 ms warm for the hash of an 8.6 MB build, 4.02 ms with none of its pages resident,
and about 2 ms of a 20 ms change form
([stage3-state.md](../v2/stage3-state.md#the-cost-of-the-hash), raw samples in
[stage3-s17-evidence.md](../v2/stage3-s17-evidence.md#the-cost-of-the-hash)).

Tests:
- `internal/infra/build/id_test.go` `TestBuildIDIsExecutableContent`
- `internal/infra/build/id_test.go` `TestUnreadableImageHasNoEffects`
- `internal/cli/lease_test.go` `TestVersionJSONCarriesBuildID`
- `test/leaserig/lease_test.go` `TestDifferentImagesWithSameVersionCannotShareRoom`
- `test/leaserig/lease_test.go` `TestArtifactCopyCanJoinRoomAndUseTools`

## The room's lease

The room keeps `build`, the id of its writers, and `build.lock`, locked with `flock`.
**Every writer holds the lock shared, then checks**: a wrapper for its whole life, through
the last write of every goroutine it started, its run directory kept as long; a command
that may change the room from before its build check to its exit; a child that writes
takes its own shared lock before its first effect, so it keeps the boundary after its
wrapper is gone. With the lock held the writer reads `build`: its own id goes on; another
id, or a file it cannot read, refuses before the first effect, exit 1, naming the room's
build. **Changing the build takes the lock exclusive**, without waiting: a launch whose id
differs, or that finds no record or an unreadable one, asks for it, and any holder makes
the request fail; a failed request reads `build` again under the shared lock and is
granted when another launch of the same build wrote it, and otherwise asks again for a
bounded time, then refuses, exit 1, naming the holders by pid and a safe label, never a
command line. Held, the launch writes its id by rename, turns its lock to shared and
reads `build` again like any writer. **No guess about liveness**: the kernel drops a
`flock` with its last descriptor, so a crashed writer holds nothing and a live one cannot
be missed; an unreadable `build` is rewritten only under the exclusive lock. A lock file
replaced is reopened, one removed is never recreated by a writer, the lease's descriptors
are not inherited, and every contention is bounded. The room's launch lock stays for
claiming names.

Tests:
- `internal/infra/build/lease_test.go` `TestSharedWritersExcludeBuildSwitch`
- `internal/infra/build/lease_test.go` `TestLaunchReplacesMissingOrInvalidBuildOnlyExclusively`
- `internal/infra/build/lease_test.go` `TestDowngradeRechecksBuild`
- `internal/infra/build/lease_test.go` `TestReplacedLeaseLockIsReopened`
- `internal/infra/build/lease_test.go` `TestRemovedLeaseLockIsNotRecreatedByAWriter`
- `internal/infra/build/lease_test.go` `TestIncompleteRoomWriterCreatesNothing`
- `internal/infra/build/lease_test.go` `TestAKeptLeaseOutlastsItsOwnersRelease`
- `internal/infra/build/lease_test.go` `TestAStalledSwitchBoundsItsWriters`
- `internal/infra/build/holders_test.go` `TestHolderDiagnosticsUseDeviceAndInode`
- `internal/infra/build/holders_test.go` `TestLeaseDescriptorsAreNotInherited`
- `test/leaserig/lease_test.go` `TestOldCommandBlocksNewBuildUntilExitOrKill`
- `test/leaserig/switch_test.go` `TestConcurrentLaunchesOfOneBuildAllTakeTheRoom`
- `internal/host/lease_test.go` `TestHostLeaseOutlivesCleanup`
- `internal/host/lifetime_test.go` `TestRunKeepsTheLeaseAndItsDirectoryUntilItsWorkersStop`

## Views and changes

**Forms, not commands, are classified**, `view` or `change`, from the parsed call before
the handler and before anything that can prune; a form is the command plus the flags and
verbs that change what it does, so `inbox` and `inbox --owed` are two. Every authorized
tool call is a change, whatever its words, since its bounded answer may keep a
continuation; a call refused before it parses has no form, takes no lease and writes
nothing. One invocation is one scope: the lease is held from the class through the
handler, the shell's observation and the printing of the answer, or, through the tool,
through the ticket's confirmation and the bounded answer; a lease refused ends it before
the handler, with no shell observation either. **A view writes nothing** — no temporary
or lock file, no pruning, no observation, on error paths too — takes no lease and waits
for none; a missing base, root or room gives the ordinary empty answer; a view of another
build says so once on stderr. It is not a consistent snapshot across files. **A view
cannot write by accident**: in view mode every write primitive of the state package
refuses, taking a lock included, and the attempt is kept as evidence, so a view that
swallows the error still fails. **Every write goes through the seam**: each write
primitive refuses with `ErrNoLease` when the process holds no lease for the room the path
lies in, by cleaned prefix, so another room's write refuses like one outside every room;
a lock's authorization ends the moment it may hold nothing; a view refusal takes
precedence over a missing lease; every refusal is counted, so a swallowed one still shows.
A reply socket outside the room is claimed for that one clean path and widens nothing of
any state. A static test over `internal/`, in all four build variants, fails on a write
outside the state package, except in its own table of file, effect, count and reason,
which fails on a stale entry.

Tests:
- `internal/cli/access_test.go` `TestEveryParsedFormHasAccessClass`
- `internal/cli/view_test.go` `TestEveryViewLeavesMissingAndExistingStateUnchanged`
- `internal/cli/view_test.go` `TestForeignBuildViewsWarnOnce`
- `internal/cli/view_test.go` `TestViewWriteAttemptFailsEvenIfIgnored`
- `internal/infra/state/view_test.go` `TestEveryWritePrimitiveRefusesInViewMode`
- `internal/infra/state/view_test.go` `TestTryHoldRefusesInViewMode`
- `internal/infra/state/lease_guard_test.go` `TestEveryMutationRequiresItsRoomLease`
- `internal/infra/state/lease_guard_test.go` `TestViewWritePrecedesNoLease`
- `internal/infra/state/lease_guard_test.go` `TestCrossRoomAndReleasedLeaseCannotWrite`
- `internal/infra/state/lease_guard_test.go` `TestAnAuthorizationDiesWithItsLock`
- `internal/cli/change_lease_test.go` `TestEveryChangeFormHoldsLeaseThroughItsLastEffect`
- `internal/cli/lease_test.go` `TestAFailedLeaseLeavesNoShellObservation`
- `internal/infra/state/bypass_test.go` `TestStateWritesHaveNoBypass`
- `internal/infra/state/bypass_test.go` `TestTheBypassCheckSeesEveryWrite`
- `internal/infra/state/bypass_test.go` `TestTheBypassTableFailsOnAStaleEntry`
- `internal/infra/state/bypass_test.go` `TestTheBypassCheckSeesADescriptorsMetadataChange`
- `internal/infra/state/bypass_test.go` `TestTheBypassCheckExcusesOnlyAProvenAbstractAddress`

## Clearing the state of 1.x

There is no migration of 1.x state: the first launch of a 2.0 session that finds a 1.x
tree under the base clears it, and no other command does. It first checks that no 1.x
wrapper runs and no 1.x command holds a lock, reading only the liveness fields of the
1.x session records against `/proc`; a live session refuses the launch, exit 1, naming
each with its room, and nothing is stopped automatically; a check that cannot be made —
a record, a directory or process data that cannot be read, a lock held, a record of
another pid namespace — refuses, naming the path, since an unknown is never taken for
absence. With none alive and every check read, the launch sets the tree aside, validates
it again from its contents and deletes it only then, the 1.x trees and nothing else under
the base, never following a link, and prints one line on stderr with what was removed and
the counts of unread and undelivered letters dropped, "unknown" where a count cannot be
established. A deletion that fails after the validation is finished by a later launch. A
room's lease clears nothing of 1.x, the clearing's own authority dies with its lease, and
every wait of the clearing is bounded. The owner decided on October 5, 2026 that 1.x
state is deleted, not set aside.

Tests:
- `internal/infra/state/cleanup_test.go` `TestARoomLeaseCannotClearTheTreesOf1x`
- `internal/infra/state/cleanup_test.go` `TestTheCleanupCoversOnlyTheTreesOf1xOfItsBase`
- `internal/infra/state/cleanup_test.go` `TestTheCleanupDiesWithItsLease`
- `internal/infra/state/cleanup_test.go` `TestATreeTransitionNeverReplaces`
- `internal/infra/state/cleanup_test.go` `TestARoomLockIsCheckedAtItsRenamedPath`
- `internal/infra/state/cleanup_test.go` `TestTheClearLockAndProbesKeepTheirBounds`
- `internal/infra/state/cleanup_test.go` `TestDeletingATreeNeverFollowsALink`
- `internal/infra/state/plaindir_test.go` `TestAPlainDirListingRefusesWithoutWaitingOrFollowing`
- `internal/infra/legacy/admission_test.go` `TestLegacyAdmissionRejectsLiveBusyAndUnknown`
- `internal/infra/legacy/admission_test.go` `TestLegacyMissingProcessIsDeadButReadFailureUnknown`
- `internal/infra/legacy/validation_test.go` `TestCandidateRequiresPostRenameValidation`
- `internal/infra/legacy/validation_test.go` `TestValidationReusesOwnedLocks`
- `internal/infra/legacy/validation_test.go` `TestASetAsideDrawsAgainOnATakenName`
- `internal/infra/legacy/validation_test.go` `TestRecreatedRoomsRevokesAdmissionOnlyOnEvidence`
- `internal/infra/legacy/validation_test.go` `TestCandidateRestoreNeverReplacesRooms`
- `internal/infra/legacy/deletion_test.go` `TestValidatedDeletionFailureDefersOnlyDeletion`
- `internal/infra/legacy/deletion_test.go` `TestLegacySummarySurvivesPartialDeletion`
- `internal/infra/legacy/deletion_test.go` `TestLegacyClearNeverFollowsForeignPaths`
- `internal/infra/legacy/recovery_test.go` `TestAValidatedTreeIsDeletedOnlyOnceSettled`
- `internal/infra/legacy/recovery_test.go` `TestEveryOutcomeIsSaidBesideALaterRefusal`
- `internal/infra/legacy/special_test.go` `TestASpecialFileIsRefusedWithoutWaiting`
- `internal/infra/legacy/special_test.go` `TestASpecialSummaryIsUnknownWithoutWaiting`
- `internal/infra/legacy/special_test.go` `TestAMailboxSwappedAfterItsCheckIsNeitherFollowedNorWaitedOn`
- `internal/infra/legacy/count_test.go` `TestTheCounterCountsEachThingOnceAndNeverGuesses`
- `internal/infra/legacy/bounds_test.go` `TestTheClearingWaitsKeepTheirBounds`
- `test/upgraderig/clearlock_test.go` `TestClearLockTimeoutRefuses`
- `test/upgraderig/clearlock_test.go` `TestTheClearingActsOnWhatItFindsUnderTheLock`
- `test/upgraderig/matrix_test.go` `TestLegacyClearConcurrentMatrix`
- `test/upgraderig/matrix_test.go` `TestLegacyClearCrashMatrix`
