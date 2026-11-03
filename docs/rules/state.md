---
description: The state root and its levels, the build's identity, the room's lease and its lock order, the bootstrap, the room's own copy of its build, switching a running room to another build, views and changes through the write seam, and the state of 1.x left alone.
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
mailbox, control, space, brief, the switch's `.switch.lock` — and before a worktree is created; no path takes it while
holding any of them, and there is no upgrade under a mailbox lock. No pruning and no name
lock inside a mailbox section. The brief lock is taken with no other lock of the room held,
and nothing is taken inside it ([briefs.md](../briefs.md#the-records)).

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
authorized, and the note `lease release` before it goes. A switch adds its own (below):
`switch claimed`, `switch dropping`, `switch exclusive`, then inside the commit `switch marked`,
`switch build` and `switch linked`.

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
   exclusive lock. The one other writer of `build` is the commit of a switch,
   `state.CommitSwitch`, through the same private primitive and under the same lock.

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

## The room's own copy

An upgrade replaces the file at the install path under running sessions, and Go's
`os.Executable()` drops the ` (deleted)` that `/proc/self/exe` then shows: every path rewake
handed on from it would lead an old room's sessions to the new build, which the room refuses.
So the room keeps its build (the owner's request, October 27, 2026: a running room keeps
working on its own build until main is restarted, and the upgrade reaches it at its next
launch).

- **What is kept.** `bin/rewake`, the room's build, and `bin/origin`, one line naming the
  directory that image was installed in, where what rewake ships beside itself — the bundled
  pi — is found.
- **Who writes it.** `build.Launch` and `build.Follow` (an image that adopts a run after a
  switch), and the commit of a switch, which links the staged copy in its place. `Launch`, right
  after the lease is granted, shared and authorized,
  before any other lock of the room, so the lock order above is unchanged. When `bin/rewake`
  is missing or its id is not the launch's — a switched room, one made before this rule, a
  damaged copy — it is placed again, `origin` first; else nothing is written. The primitive
  `state.PlaceExecutable` hard-links the install's file to a temporary name and keeps it only
  when its id is the launch's, which a file an upgrade already replaced fails; otherwise it
  copies `/proc/self/exe` with mode 0700. A link keeps the install's own mode, which is never
  changed. Then a rename and a directory sync. Two launches of one build write the same bytes.
  A switch happens only with no holder, and the switching launch places its copy before it
  starts its harness, so no session runs a stale one. A failed placement does not stop the
  launch: one note, and the launching rewake's own path.
- **What a session gets.** `REWAKE_EXECUTABLE` names the copy, its directory goes first on
  PATH, and Claude Code's status line runs it ([design.md](../design.md#environment-the-harness-receives)).
  A worker's `rewake …`, the tools of the mod and the pi extension, `space open` and the space
  host and worker it starts all run the room's build. The mod and the extension are embedded
  in the image. pi is not: the copy finds it above `origin`, and when an upgrade changed the
  bundled pi a new pi worker in the old room is refused with a hint to restart main
  (accepted by main, October 27, 2026).
- **The hand-off.** A command of a room — not the guide, help or the version — run by another
  build, in a room whose copy is the room's build, prints one line on stderr and execs the copy
  with the same arguments, environment and pid; the copy takes its own lease. That covers a
  person's terminal after an upgrade and main's shell, whose path is the install's. Otherwise
  the refusal stands. A launch never hands off.
- **What is left.** The copy lives and dies with the room's directory: a wiped base is a new
  bootstrap and a new copy, a copy removed under a running room leaves PATH falling through to
  the install, and the next launch of the room's build places it again. At most one binary per
  room, none extra while the link shares the install's file, but for a switch's staged copy,
  `bin/rewake.<id>`, which the next commit removes when no live mark names it. Nothing removes a room.
- **Not covered.** The upgrade to the first build with this rule still broke the rooms then
  running.

Tests:
- `internal/infra/state/executable_test.go` `TestPlaceExecutableLinksOrCopies`
- `internal/infra/build/pin_test.go` `TestLaunchKeepsTheRoomsOwnCopy`
- `internal/infra/build/pin_test.go` `TestHandoffOnlyToACopyOfTheRoomsBuild`
- `internal/host/launch/environment_test.go` `TestARunRunsItsRoomsCopy`
- `internal/cli/handoff_test.go` `TestACommandOfAnOlderRoomIsHandedToItsCopy`
- `test/workflow/pinned_build_test.go` `TestPinnedBuild`

## Switching a running room

`rewake restart rewake` switches a running room to the installed build without ending its
sessions, when every long-lived process of the room can hand its run over: the first stage of
updating a room without its restart. The lease keeps its rule: `build`
changes only under the exclusive lock with no other holder. A switch adds what makes that
happen while the sessions run, and what a crash at any step leaves sound. The code is
`internal/infra/build/switch.go`, `internal/infra/state/switch.go`, `internal/host/switching`
and `internal/cli/restart_rewake.go`.

- **The command** is a change form, so it runs the room's build. Its target is the `rewake` in
  the directory `bin/origin` names; the room's own build there is "nothing to switch", and one
  whose contracts the room does not admit is refused at once. Otherwise it hands the switch to
  **the leader**, a process of its own (`internal/cli/switch_lead.go`), and either returns —
  asked from a session, whose own open turn would keep it busy for as long as it waited:
  "the switch to build X runs once every session is idle, within 10m0s; its outcome comes to
  main as a note" — or, asked by the person, waits for the outcome on a pipe and prints it
  (owner's decision November 2, 2026; the deferral reviewed the same day).
- **The deferred leader** holds `<room>/switch-request` by its flock for as long as it lives, so
  one waits at a time ("a switch already waits for the room's sessions to be idle"); a request
  no lock holds is a dead leader's, replaced by the next. It is bound to main's run when it was
  asked, by name and epoch. Each attempt takes the room's lease with `Acquire`, claims, stages,
  admits and leads, and lets the lease go at its end, so a leader waiting holds nothing a launch
  meets. An attempt that ended only because a session was busy — at the ask or at the drop —
  is tried again 5 s later, for 10 minutes from the asking; one whose main's run ended, while
  waiting or asking, stops, and says nothing to a later main of the name. Its last image is the
  room's copy, exec'd with the outcome, since after a commit only the new build writes in the
  room: it takes a writer's lease with `Acquire`, puts the outcome to main as a note — under
  main's mailbox lock, only while main's run is still the one asked for —, answers the pipe and
  removes the request.
- **The mark**, `<room>/switch`, is one attempt: the leader's pid and start, the attempt's own
  random id — a leader that tries again claims a new attempt, which every participant joins
  again (found in review, November 2, 2026) —, the target id, the staged path, the leader's
  socket (`sock/switch.sock`), a phase — `asking`, `dropping`, `committed`, `aborting` — and a
  deadline. Its changes are ordered by `.switch.lock`, taken
  after the lease. The claim is the leader's first effect: a live mark of another attempt
  refuses it before anything is staged. A mark `asking` is live while its leader runs; from
  `dropping` on it is live until its deadline, whatever became of its leader, since something
  may be parked. A stale mark holds nothing, and the next lease granted removes it.
- **Staged and admitted.** The target is placed as `bin/rewake.<id>`, kept only when its id is
  the target's, and asked its contracts (`--version --json`): every contract it speaks and
  requires must equal the room's build's, or the attempt ends, naming the contract, and the room
  needs its restart. Among them `handover` (the wrapper's handed run), `handover.<adapter>` (an
  adapter's handed state), `handover.space` (a space host's handed terminal), `handover.mainhost` (main's host's handed terminal, shell and loop), `switch` (the mark, the socket's messages, a parked image's
  handover), `mod` and `extension`; each handed shape is pinned to its number by a test.
- **Asking.** Every wrapper, space host and main's host looks at the mark every 250 ms and,
  while it is asking, joins on the socket — again at the next look when nothing listened yet —
  answering `ready`, `busy <why>` or `cannot <why>`. One `cannot` ends the attempt at once,
  before anything stopped: "restart the room: … cannot". Busy ones are waited for 60 s.
- **Dropping.** The mark turns `dropping` and each participant stops its delivery server and
  execs its own image, parked: no lease, no effect, waiting for its order. Meanwhile a writer
  that finds a dropping mark whose leader runs lets go of its shared lock and looks again every
  50 ms, within the writers' bound, then is refused as busy; a `send --question` waiting ends its wait as pending.
  Each participant must say `parked` within 5 s; then the leader turns its lock exclusive within
  5 s, as commands finish, or names the holders left. One that turned busy since it was ready
  ends the drop, and the refusal names the busy ones alone, the others having stopped for them;
  only when none said busy does it name who did not say `parked` (November 3, 2026).
- **The commit**, `state.CommitSwitch`, only under the exclusive lock and only for the room's
  own dropping mark: the mark `committed` first, then `build` — the commit point — then the
  staged copy linked to `bin/rewake`, then the other staged copies removed. The leader keeps
  its lock shared, unauthorized, while the parked images exec the staged copy and adopt; it
  reports who was adopted and who was lost.
- **Abort**, before the commit only: the mark `aborting`, under the lease taken shared again,
  and each parked image resumes on the old build; the mark is removed once each holds the lease
  again or its deadline passes. The command exits 1, the room unchanged.
- **Launches never switch under a live mark.** One of the room's build goes on as a writer;
  another is refused, "room R is switching to build X", and `switched()` reads the mark again
  under the exclusive lock before it writes `build`.
- **Adoption only follows.** A parked or handed-over image takes the lease with `build.Follow`:
  `Acquire`, and the room's copy put in line with `build` whenever they differ. A parked image
  whose leader is gone decides at once by the record alone — its own build resumes it, the target
  sends it on, anything else ends it — and takes its lease while the mark still keeps launches
  out: a dropping mark whose leader is gone gates no writer, since nothing will commit. Waiting
  out the mark instead let a launch of a third build take the room first (found in review,
  November 2, 2026).

| The leader dies… | What is left |
|---|---|
| while asking | nothing stopped; the mark goes stale with its leader |
| while dropping, aborting, or in the commit before `build` | the mark stays live to its deadline, so no launch switches; the parked images find the old `build` and resume at once, within it |
| after `build`, before the copy is linked | `build` is the target and `bin/rewake` the old build: the first `Follow` or launch of the target places the copy |
| after the commit | the parked images find the target and exec it; the committed mark keeps launches out until its deadline |

A committed mark holds the room about 20 s after the commit, so a second switch in that time is
refused. A switch run just after a delivery may find the session busy for a moment, and is run
again. A worker's wrapper of Claude Code or pi hands over when idle (November 2, 2026): what is
handed over and what keeps it busy is in [claude.md](../claude.md#handing-the-run-over) and
[research-pi.md](../research-pi.md#handing-the-run-over-november-2-2026). A run's mirror goes over
in a memory file, and its face reconnects to the next image
([face.md](../face.md#the-socket)). An adopting image says nothing on stderr, which is the
harness's terminal: the role line and the launch's notes are said once, at the launch, and said
again they landed inside a worker's screen (found in review, November 2, 2026). A space host
hands over its worker's terminal: the pty
master, the socket's listener and its emulator's checkpoint as a seed, in a memory file, with the
shell's input frozen and its pump stopped at the snapshot; the worker, its child, runs on
([space.md](../space.md#a-switch-of-the-rooms-build), November 2, 2026). Main's host hands over
main's terminal, the shell's channel and its loop, the shell and main's wrapper running on as its
children ([shell.md](../shell.md#a-switch-of-the-rooms-build)). Main's wrapper hands over as a
worker's does, when its harness is idle, and carries two things more: that it runs in rewake's
shell, which its launch read from an environment it then cleared, so the next image answers
main's suspends again; and its notice state, taken from the scan between two scans and given back
to it after an exec that failed, so the next image tells main nothing twice — a receipt outlives
the notice in the mailbox — and loses nothing it held (`internal/host/notices_handover.go`,
November 2, 2026). Main's turns keep its wrapper busy often, and `rewake restart rewake` defers the
switch until they end rather than being refused (the deferred leader, above). Every wrapper's
delivery server hands its reminder's memory over — which letters were announced and not yet
looked at by a finished turn's end, and the latest end looked at, boot-clock times the exec keeps
(`delivery.Memory`), after it has looked at every finished end still queued for it, which the
next image would not take — so an announced letter no longer keeps the run busy: a turn that ended in an
error or was stopped looks at nothing, and found live with a pi main, the run stayed busy for the
whole of the switch's bound. What still keeps it busy is a notice the harness holds, one a late
word may still take back — within `mail.LateWordWindow` of its delivery, looked at when the
switch asks: once only a new notice pruned the memory, and found live on November 2, 2026, a main
sent one notice stayed busy through the whole of the switch's bound — and a letter waiting for its
delivery (the owner's decision, November 2, 2026). Every memory a busy answer reads is looked at
by its age when asked, not only on the next event: the mod endpoint's control requests and asks
([claude.md](../claude.md#handing-the-run-over)) and main's host's reason, which taking the
shell's digest publishes again. A Claude Code run also stays busy while it holds the outcome of
a compaction main has not read yet, which the handover does not carry, within main's own bound. A
Claude Code run without its mod answers `cannot`. A run whose
launch made a checkout hands over as any other, and the handover does not name it: the adopting
image finds it by its record — the session's name, run and room, and as its maker this process,
whose pid and start the exec keeps (`worktree.Record.MadeHere`) — and settles it at the run's end
as the launch would have, by the same rules and with the same lines on stderr
([worktree.md](../worktree.md#a-checkout-across-a-switch)). Until November 2, 2026 such a run
answered `cannot`, which the owner's first live switch met for every writer of the room. The
fixture's wrappers hand over as the switch's test.

**Signals across the exec.** Go overrides a SIGTERM the image inherited as ignored, so an
ignore set before the exec does not cover the new image's start. The adopting image catches
SIGTERM and SIGHUP, and drops the keyboard's SIGINT and SIGQUIT, as its first act
(`host.CatchSignals` in `rewake adopt`); the run takes that catch and forwards to the harness what
came meanwhile, as it would have. A parked image that execs the target carries what it caught in
the handover's `signals` (`Handover.Continue`), and the next image's run takes those first: the
handler that held them does not outlive the exec, and a SIGTERM caught while parked was lost at
the commit (found in review, November 2, 2026).

**An adoption is not a launch.** The adopting image goes through the wrapper's run again, so
each step a launch takes says whether it holds for a harness already running (November 3, 2026).
The adapter is asked for an adoption's plan (`LaunchRequest.Adopting`): the service, the wake and
the socket the run is served with, with no check of the harness's start and no write it made.
The host reads no version, since the harness on `PATH` may have changed since the start. What
Claude Code's adoption repeats is [claude.md](../claude.md#what-an-adoption-repeats); pi's
plans its extension's endpoint again and no longer makes its agent directory or reads its intro.
Before this, every Claude Code run met its launch's refusal of a live messaging socket, and the
image exited.

**A failed adoption holds its harness.** The harness is the wrapper's child on the wrapper's
terminal, and an image that exits hangs that terminal up: on November 3, 2026 one adoption's
failure ended main's Claude Code and with it the shell, the room and its writer
([traps.md](../traps.md#after-rewake-restart-rewake-mains-claude-code-ended-and-the-room-with-it)).
An adoption that fails before it has its harness now keeps it: it closes what was handed over
and what it started, so the mod's or the extension's next request finds nobody, and passes
signals on and answers stops as a run does. It says on stderr what failed and that the run
should be ended and started again. A run that failed under the lease, its plan or service
refused, also marks the exec `not adopted: …` for its sender and leaves a worker's main a note
(`host/unadopted.go`). A harness this build does not have, and a parked image whose wait or exec
fails, hold no lease to write by and say it on stderr only (`cli/adopt.go`). The
record, the lease and the run directory stay until the harness ends; then they go, and the image
exits with the harness's code. Held, the run is served no more: no letter wakes it and none of
its turns is reported. It is not repaired, since starting it again ends the conversation the
switch was meant to keep. The leader counts every participant that holds the lease as adopted,
so its letter names a held run among them, and the note corrects it. A later switch is refused,
the lease named as still held, until the person ends the run. A handover that cannot be read
still exits: it names no harness to hold.

Tests:
- `internal/infra/build/switch_test.go` `TestASwitchCommitsOnlyWithNoOtherHolder`
- `internal/infra/build/switch_test.go` `TestAWriterWaitsOutADroppingSwitchWithinItsBound`
- `internal/infra/build/switch_test.go` `TestAStaleSwitchMarkIsIgnoredAndRemoved`
- `internal/infra/build/switch_test.go` `TestADroppingMarkWithoutItsLeaderGatesNoWriter`
- `internal/infra/build/switch_test.go` `TestTheCommitWritesTheBuildThenTheCopy`
- `internal/infra/build/switch_launch_test.go` `TestALaunchNeverSwitchesUnderALiveMark`
- `internal/infra/build/switch_launch_test.go` `TestAdoptionOnlyFollows`
- `internal/infra/build/switch_launch_test.go` `TestFollowPutsTheCopyInLineWithTheBuild`
- `internal/infra/build/switch_launch_test.go` `TestASecondAttemptIsRefusedBeforeItStages`
- `internal/host/switching/join_test.go` `TestAParkedImageFollowsTheRecordWithoutItsLeader`
- `internal/host/switching/join_test.go` `TestAParticipantJoinsALeaderThatServesLate`
- `internal/host/handover_shape_test.go` `TestTheHandoverContractChangesWithItsShape`
- `internal/host/handover_mirror_test.go` `TestAFaceReconnectsToTheModelHandedOver`
- `internal/host/handover_mirror_test.go` `TestMainsWrapperHandsOverLikeAWorkers`
- `internal/host/notices_handover_test.go` `TestMainsNoticesGoOverWithItsRun`
- `internal/host/notices_handover_test.go` `TestADropTakesMainsNoticesAndAFailedExecResumesThem`
- `internal/host/handover_mirror_test.go` `TestASignalBeforeTheRunsCatchWaitsForTheRun`
- `internal/host/handover_signals_test.go` `TestASignalCaughtWhileParkedReachesTheNextImagesRun`
- `internal/core/mirror/handover_test.go` `TestTheNextImageAdoptsTheModelAsItWas`
- `internal/core/mirror/handover_test.go` `TestTheHandedModelChangesWithItsShape`
- `internal/adapter/claude/handover_test.go` `TestAnIdleClaudeRunIsHandedOverAndTheModsNextPollAnswered`
- `internal/adapter/claude/handover_test.go` `TestTheClaudeHandoverChangesWithItsShape`
- `internal/adapter/pi/handover_test.go` `TestAnIdlePiRunHandsItsConnectionOver`
- `internal/adapter/pi/handover_test.go` `TestThePiHandoverChangesWithItsShape`
- `internal/adapter/fixture/handover_shape_test.go` `TestTheFixtureHandoverChangesWithItsShape`
- `internal/adapter/claude/modhost/protocol_pin_test.go` `TestTheModContractChangesWithItsProtocol`
- `internal/cli/restart_rewake_test.go` `TestRestartRewakeRefusesAnotherContract`
- `internal/cli/restart_rewake_test.go` `TestRestartRewakeNamesWhoCannotHandOver`
- `internal/cli/restart_rewake_test.go` `TestAQuestionEndsItsWaitAtADrop`
- `test/leaserig/leader_test.go` `TestARoomWithNothingToAskSwitches`
- `test/leaserig/leader_test.go` `TestALeaderKilledAtEachStepLeavesOneBuild`
- `test/leaserig/leader_test.go` `TestAForeignLaunchDuringParkingCannotOverturnTheSwitch`
- `internal/host/spacehost/handover_shape_test.go` `TestTheSpaceHandoverContractChangesWithItsShape`
- `internal/host/stream/handover_test.go` `TestAHandoverKeepsTheScreenAndWhatTheProgramWritesMeanwhile`
- `internal/host/stream/handover_test.go` `TestAClusterSplitAtTheHandoverIsJoinedAfterIt`
- `internal/host/stream/handover_test.go` `TestAPasteInFlightArrivesWholeAndInputAfterTheFreezeIsRefused`
- `internal/host/stream/handover_test.go` `TestAHandoverIsRefusedWhileAWriteIsStuck`
- `internal/host/stream/handover_test.go` `TestAResumedRelayServesTheSameScreen`
- `internal/host/terminal/checkpoint_test.go` `TestThePreambleCarriesTheStateLaterOutputUses`
- `internal/host/terminal/checkpoint_test.go` `TestTheContinuationsTheCheckpointChanges`
- `internal/host/delivery/held_late_test.go` `TestADeliveredNoticeKeepsTheRunBusyOnlyWithinTheWindow`
- `internal/adapter/claude/modhost/management_test.go` `TestAnUnansweredRequestOfAnEarlierLoadKeepsNothingBusy`
- `internal/core/ask/ask_test.go` `TestAnUnsettledRequestIsCountedOnlyUntilItGoes`
- `internal/adapter/claude/modhost/handover_outcome_test.go` `TestAnOutcomeMainHasNotReadKeepsTheRunBusy`
- `internal/host/mainhost/drop_test.go` `TestTheDigestTakenWakesTheLoopForASwitch`
- `internal/cli/adopt_worktree_test.go` `TestTheAdoptedRunFindsTheCheckoutItsLaunchMade`
- `internal/adapter/claude/adoption_test.go` `TestAnAdoptionPlansOnTheLiveSocket`
- `internal/adapter/pi/adoption_test.go` `TestAnAdoptionPlansWithoutTheLaunchsSteps`
- `internal/host/unadopted_test.go` `TestAFailedAdoptionHoldsTheHarness`
- `test/workflow/claude_adopt_test.go` `TestClaudeAdopt`
- `test/workflow/fixture_switch_test.go` `TestFixtureSwitch`
- `test/workflow/fixture_switch_worktree_test.go` `TestFixtureSwitchWorktree`

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
| change | `update`, `update <version>`, each with `--json` | they write the record of npm's answer under the root, and in a room hold its lease as every change does |
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
A folder of the project, where a brief may live, is admitted for the length of one call:
`WithOutside(room, folder, fn)` refuses unless the room is leased and the folder is absolute, clean,
reaches no link and lies neither in, around nor under any state directory, and inside `fn` the
guard takes writes under the folder as it takes a room's: the view check and the lease check of
each write, against the admitting room, so a write after the lease went is refused, and a link
anywhere between the folder and the path refuses it too, checked at the write. Only the briefs'
package refers to it (`TestOnlyTheBriefsAdmitAFolderOutsideTheRoom`, over the syntax tree). A reply socket outside the room is claimed for that one clean path and widens nothing of
any state: a path in a state directory, as written or where its links lead, is guarded by
the lease of the room it lies in, and a spelling that only cleans to the claimed path is
refused, since the kernel follows a link before a `..`. The write primitives are
`WriteAtomic`, `WriteAtomicHeld`, `PublishExclusive`, `PlaceExecutable`, `Remove`, `Rename`, `EnsureSubdir`,
the locks (`WithRoomLock`, `WithNameLock`, `TryWithNameLock`, `WithSpaceLock`, `WithBriefLock`,
`WithMailboxLock`, `WithMailboxLockAt`, `TryHold`), and the os calls they name behind the same guard
(`OpenFile` with a write flag, `WriteFile`, `Link`, `Move`, `Touch`, `Discard`,
`RemoveAll`, `Flock`), and `WriteUpdateRecord`, the one write of a path in no room: `<root>/update.json`,
the last answer of npm for a newer release, a cache the rooms share rather than any room's state
([install.md](../install.md#updating-from-inside-rewake)). It takes no lease, refuses in a view,
writes only that path, by rename, and only when the root exists and passes its checks; it creates
nothing, and the last writer wins. The read clock goes through `state.Counter`, the sockets through
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
- `internal/infra/state/outside_test.go` `TestOnlyTheBriefsAdmitAFolderOutsideTheRoom`
- `internal/infra/state/outside_test.go` `TestAFolderOutsideTheRoomIsWritableOnlyInsideItsAdmission`
- `internal/core/taskbrief/outside_test.go` `TestOnlyAFolderOfTheProjectIsAdmitted`
- `internal/core/taskbrief/outside_test.go` `TestALinkInsideAnAdmittedFolderTakesNoWrite`
- `internal/core/taskbrief/outside_test.go` `TestAnAdmissionEndsWithItsRoomsLease`
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
