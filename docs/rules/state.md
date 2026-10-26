---
description: The state root and its levels, the build's identity, the room's lease and its lock order, the bootstrap, views and changes through the write seam, and the state of 1.x left alone.
---

# Rules: state and builds

Where rewake 2.0 keeps its state, how one build holds a room, which calls may change the
state and through what, and that the state of 1.x is left alone. The tree a room holds is in
[design.md](../design.md#state-directory), and the run directory, which goes with its run, is
L3 in [launch.md](launch.md). The rules have no numbers: each section below is one rule, with
the tests that hold it.

## The state root

The base is `$REWAKE_DIR`, absolute, or `rewake-<uid>` in the system's temporary
directory: a directory, not a symbolic link, owned by the user, mode 0700. Inside it 2.0
keeps everything under its root `v2/`, and each room at `v2/rooms/<room>`, so 1.x never
reads 2.0's records and 2.0 never reads the 1.x tree. The inverse is checked,
never counted: a room's root is its grandparent only when its parent is named `rooms` and
its grandparent `v2`, an error otherwise, and every caller handles that error rather than
substituting the ambient `REWAKE_DIR`. A child of the wrapper — a shell of the session —
resolves the same room from the base the wrapper exported, byte for byte, so the run's
authority's address, which hashes the room, is the wrapper's. **No
resolver creates anything**: the base, the root and the room are made only by a launch's
bootstrap, under the room's lease, each level flushed, recovering after a failure at any
step and refusing a component that exists but is unsafe. The worktrees and their records
stay in the user's data directory, outside the base.

### The roots

| Level | Function | Value |
|---|---|---|
| base | `state.Base()` | `$REWAKE_DIR` (absolute) or `os.TempDir()/rewake-<uid>` |
| root | `state.Root()` | `<base>/v2` |
| room | `state.RoomDir(root, room)`; `state.Dir()` for the current one | `<root>/rooms/<room>` |

`state.RootForRoom(room)` returns the root and no error only when the room's parent is named
`rooms` and its grandparent `v2`; `state.BaseForRoom(room)` is that root's parent, with the same
error. Each resolver, when the path exists, verifies it (`Verify`: a directory, no symlink, the
user's, mode 0700) and creates nothing. A child's `REWAKE_DIR` is `BaseForRoom` of its wrapper's
room, so the child resolves the root once. A test that writes a room has it made by
`buildtest.Room` (`internal/infra/build/buildtest`): the acquisition of a launch of the test
binary, held shared until the test ends, as a session holds its room's (`buildtest.Established`
releases it at once, `buildtest.Release` before a switch); the workflow suite names its rooms
through `Isolation.Room`.

Tests:
- `internal/infra/state/rooms_test.go` `TestResolversNeverCreate`
- `internal/infra/state/rooms_test.go` `TestRootInverseRejectsLegacyOrMalformedRoom`
- `internal/cli/roots_test.go` `TestTwoRoomsUseOnlyV2`
- `internal/cli/roots_test.go` `TestWorktreeOwnerRootIsChecked`
- `internal/cli/roots_test.go` `TestOrdinaryCommandsNeverTouchLegacyTree`
- `internal/host/roots_test.go` `TestChildBaseRoundTripAndAuthority`
- `internal/cli/lease_test.go` `TestChangeFormsOnAnEmptyBaseCreateNothing`
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
refuses before its first effect — before even the bootstrap creates anything — exit 1,
naming why. `--version --json` carries the id;
the human line is unchanged. **The id stays the SHA-256 of the image whatever its cost
measures**: nothing switches to another identity on a threshold, and a weaker identity
would be the owner's decision on a separate proposal that states its guarantee and the
adversarial test it fails — two images with the same embedded toolchain build ID and
different bytes, for one. Measured on a release build of 8.6 MB on an idle machine, the
hash took a median of 2.76 ms warm (p95 3.79 ms) and 4.02 ms with none of its pages
resident; `rewake pending` took a median of 20.62 ms with it and 18.66 ms without. The
endpoint's peer check, the launch probe and what a wrapper runs for itself start from
`/proc/self/exe`, never from a path looked up again.

Tests:
- `internal/infra/build/id_test.go` `TestBuildIDIsExecutableContent`
- `internal/infra/build/id_test.go` `TestUnreadableImageHasNoEffects`
- `internal/cli/lease_test.go` `TestVersionJSONCarriesBuildID`
- `test/leaserig/lease_test.go` `TestDifferentImagesWithSameVersionCannotShareRoom`
- `test/leaserig/lease_test.go` `TestArtifactCopyCanJoinRoom`

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
reads `build` again like any writer, since `flock` does not promise the conversion is
atomic. **No guess about liveness**: the kernel drops a `flock` with its last descriptor,
so a crashed writer holds nothing and a live one cannot be missed; an unreadable `build`
is rewritten only under the exclusive lock. A lock file replaced is reopened, one removed
is never recreated by a writer, the lease's descriptors are not inherited, and every
contention is bounded. The room's launch lock stays for claiming names. Only a launch
establishes a build, so a writer makes nothing of a room no launch completed: it refuses
there and leaves the base as it found it. Holders are found in `/proc/locks` by the device
and inode of `build.lock` and named by pid and a safe label — the executable's base name
and, when it is a session, its name — never a command line; missing or changing holder
details make the diagnostic poorer and never permit the exclusive lock.

**A refused exclusive request is not yet a refusal.** Two launches of one build into a fresh
room both read the record missing and both ask for the lock exclusive; flock's conversion
lets one through. The other takes the lock shared again within the writers' bound and reads
the record: its own build means the winner switched the room, and the lease is granted.
Otherwise it lets go, waits a short random pause, and asks again. The retries have two
seconds (`switchWait`), checked after each shared reacquisition once the record is not the
launch's own: past them the launch is refused naming the holders. Each shared reacquisition
has the writers' own bound (`shareWait`, ten seconds), which a switch that stalls while
holding the lock exclusive uses up: then the launch is refused as busy.

**Lock order.** The build lease comes before every other lock of the room — launch, name,
mailbox, control, space, brief — and before a worktree is created; no path takes it while
holding any of them, and there is no upgrade under a mailbox lock. No pruning and no name
lock inside a mailbox section. The brief lock is taken with no other lock of the room held,
and nothing is taken inside it ([roadmap/briefs.md](../roadmap/briefs.md)).

**Who owns what.** `infra/state` owns an opaque authorization scoped to one room;
`infra/build` owns the identity and the acquisition and calls `state`, which never imports
`build`. No ambient boolean and no environment variable grants writing. The CLI hands its
acquisition to the host rather than holding a shared lease while the host asks for an
exclusive one. Every callback and resource of a run stops before the lease closes, and the
run counts what may still write — the availability publisher, the session-state worker and
every write of the live set — keeping its own hold on the lease (`build.Lease.Keep`) until
the last of them has returned. Their stops wait only so long, so a write stalled in the
filesystem does not hold the wrapper's exit: the lease then goes when that worker ends, or
with the process. Known limit: a worker that never returns holds the lease until the
process exits, on purpose.

The fault seam names steps at the lease, so a test orders one process against another:
`bootstrap <level>` before each level is made, `lease switch` once a launch holding the
lease shared has read a record not its own, `lease exclusive` once a launch holds it
exclusive, `lease downgrade` before it turns shared, `leased` once the lease is held and
authorized, and the note `lease release` before it goes.

### The bootstrap

The lease lives in the room, so establishing it writes before any lease exists. These
writes are primitives of `infra/state`, the only ones that work without a lease — in view
mode they refuse like every other — and `build.Acquire` is their one caller; a test over the
module fails on any other reference to them, a function value or a second import name
included.

1. `state.Bootstrap(base, room)`: the base, the root, the room and its `sessions`, `inbox`,
   `sock`, each created with mode 0700 when missing and verified when present, never
   repaired by `chmod`; each level, made or found, is synced into its parent, and a failed
   sync stops the launch before its claim. A level found is synced too: one a concurrent
   launch made and has not flushed exists without its name being durable. A crash may leave
   an incomplete empty skeleton, which the next launch completes.
2. `state.CreateLeaseLock(room)`, a launch's: `build.lock`, opened with
   `O_CREAT|O_CLOEXEC`, mode 0600, and locked; after the lock is taken, the open file and the
   path must be the same file, or it is opened, and created, again. Release never unlinks it.
   A writer's `state.OpenLeaseLock(room)` opens the same file without `O_CREAT`: a room
   without its lock is a room without a build, and the writer refuses it as it found it.
3. `state.WriteBuildRecord(room, id)`: `build`, written by rename and sync, only under the
   exclusive lock.

So `infra/build` writes nothing itself.

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

### What is a writer

**Forms, not commands, are classified**, `view` or `change`, from the parsed call before
the handler and before anything that can prune; a form is the command plus the flags and
verbs that change what it does, so `inbox` and `inbox --owed` are two. A call refused
before it parses — an unknown command or flag, a stray argument, an alias that does not
expand — has no form, takes no lease and writes nothing.

| Class | Forms | Why |
|---|---|---|
| change | a launch; `send` in every kind; `withdraw`; `edit`; `inbox`, `inbox --message`; `pending`; `compact`, `interrupt`; `answer`; `space open`, `space close`; `brief new`, `edit`, `close`, `tick`, `comment`; `worktree enter`, `rm`, `land`, `finish` | they write the room |
| change | `inbox --peek` | it takes the mailbox lock, which orders it against a read in progress; taking a lock is a write |
| view | bare `rewake`, `guide`, `--help`, `--version`, each with `--json`; `list`; `whoami`; `workers`; `statusline`; `inbox --owed`, `inbox --awaited`; `worktree ls`, and a `worktree`, `space` or `brief` call with no change verb; `space list`; `brief list`, `brief show` | they answer from what is there |

Each command's entry in the command table (`internal/cli/registry.go`, `access.go`) is the
source; a test fails on a form without a class.
A view looks the caller up and lists sessions without pruning; a dead record is pruned only
in a change form, under its lease. One invocation is one
scope: the lease is held from the class through the handler and the printing of the
answer; a lease refused ends it before the handler. **A view writes nothing** — no
temporary or lock file, no pruning, on error paths too — takes no lease and waits
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
any state: a path in a state directory, as written or where its links lead, is guarded by
the lease of the room it lies in, and a spelling that only cleans to the claimed path is
refused, since the kernel follows a link before a `..`. The write primitives are
`WriteAtomic`, `WriteAtomicHeld`, `PublishExclusive`, `Remove`, `Rename`, `EnsureSubdir`,
the locks (`WithRoomLock`, `WithNameLock`, `TryWithNameLock`, `WithSpaceLock`, `WithBriefLock`,
`WithMailboxLock`, `WithMailboxLockAt`, `TryHold`), and the os calls they name behind the same guard
(`OpenFile` with a write flag, `WriteFile`, `Link`, `Move`, `Touch`, `Discard`,
`RemoveAll`, `Flock`); the read clock goes through `state.Counter`, the sockets through
`state.Listen`, whose close removes the path once. A static test over `internal/`, in both
build variants, fails on a write outside the state package, except in its own table of
file, effect, count and reason, which fails on a stale entry. The same lease rule covers
`tools/` and `test/`: a driver there that writes a room itself holds the room's lease as a
session does. An adapter's writes end inside its `Close`.

Known limits: the static check reads no types, so it takes every `Chmod` or `Chown` method
for a write; `Authorize` and `ClaimSocket` are scoped by the lease, not restricted to their
callers.

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
- `internal/cli/lease_test.go` `TestAFailedLeaseWritesNothing`
- `internal/infra/state/bypass_test.go` `TestStateWritesHaveNoBypass`
- `internal/infra/state/bypass_test.go` `TestTheBypassCheckSeesEveryWrite`
- `internal/infra/state/bypass_test.go` `TestTheBypassTableFailsOnAStaleEntry`
- `internal/infra/state/bypass_test.go` `TestTheBypassCheckSeesADescriptorsMetadataChange`
- `internal/infra/state/bypass_test.go` `TestTheBypassCheckExcusesOnlyAProvenAbstractAddress`

## The state of 1.x

2.0 never reads the tree rewake 1.x left under the base, and no command clears it: a launch
leaves it as it is, and the person may remove it by hand once no 1.x session runs.

Tests:
- `internal/cli/roots_test.go` keeps the 1.x tree byte for byte through a launch's lease, the
  change forms and the views.
- `internal/infra/state/plaindir_test.go` `TestAPlainDirListingRefusesWithoutWaitingOrFollowing`
