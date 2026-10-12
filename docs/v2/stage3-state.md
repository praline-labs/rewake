# Stage 3: state and builds

How S17 builds what [design-state.md](design-state.md) decided: the state root `v2/` and
its inverse, what is a writer, one build per room, the run directory, and the measurement
of the hash. Clearing the state of 1.x is in [stage3-upgrade.md](stage3-upgrade.md), the
tests of each decision in [stage3-state-tests.md](stage3-state-tests.md).
Citations are to `ab8d752` (S16 landed). The Codex side's readiness review of October 11,
2026, adopted by main on October 12 with its decisions D1–D12, corrected this plan; the
corrections are written in here, not kept beside it.

## The sequence

S17 lands as seven commits, in three parts each reviewed and accepted on the Codex side
before the next starts: commits 1–3, 4–6, and 7 with the live gate. Each commit needs
only what an earlier one made; none is called behaviour-preserving, since the build
refusals, the root `v2/` and the end of pruning in views change behaviour on purpose.
The binaries between them are for private test bases: before commit 7 a 2.0 build beside
an installed 1.x writer that does not honour the lease is no safe coexistence.

1. **The contracts and the forms.** These documents corrected; every parsed form of the
   command table classified, in the shell and through the tool
   ([below](#what-is-a-writer)); the view paths stop pruning; one scope runs from the
   class through the handler, the shell observation and the bounded output of a tool
   call. The resolvers still create, so no read-only invariant is claimed yet.
2. **The build id, the lease and the bootstrap**, on today's layout
   ([below](#one-build-per-room)): acquired in both dispatch routes and at the direct
   entry of the host. Writers take the lease; nothing refuses without one.
3. **The roots**: `Base`, `Root`, `RoomDir` under `v2/`, the checked inverses, resolvers
   that create nothing; view mode on, with `ErrViewWrite`, now that every view path is
   repaired. Every path oracle of the tests is corrected here.
4. **The run directory** and the lifetime of what it holds ([stage3-rundir.md](stage3-rundir.md)),
   closing L3 with its qualification.
5. **Ordinary writes through the seam**: files, the name, mailbox, receipt and control
   locks, the tagged callbacks, with S3's and S15's lock and publication order kept.
6. **Mapped counters and sockets through guarded resources**; `ErrNoLease` and the
   static test on, once every path is covered. Commits 5 and 6 split by kind of effect.
7. **The clearing of 1.x** with its scoped cleanup authority
   ([stage3-upgrade.md](stage3-upgrade.md)), its crash and concurrency matrix, the live
   gate and the timing record.

At each commit from the second the tests run a launch on an empty base and a session's
change forms after it; from the third, every view form on an absent and on a populated
state, the base unchanged, and a view that would write fails; from the sixth, a write
without a lease fails.

## The roots

Three levels, each with its own function; at `ab8d752` only two exist, and `Dir()`
(`infra/state/rooms.go:27`) returns a **room**, not a root.

| Level | Function in 2.0 | Value | At `ab8d752` |
|---|---|---|---|
| base | `state.Base()` | `$REWAKE_DIR` (absolute) or `os.TempDir()/rewake-<uid>` | `state.Root()` (`infra/state/state.go:50`) |
| root | `state.Root()` | `<base>/v2` | — |
| room | `state.RoomDir(root, room)`; `state.Dir()` for the current one | `<root>/rooms/<room>` | the same names, over the base (`rooms.go:27,36`) |

The inverse is checked, never counted: `state.RootForRoom(room)` returns the root and no
error only when the room's parent is named `rooms` and its grandparent `v2`, and an
error otherwise; `state.BaseForRoom(room)` is that root's parent, with the same error.
Every caller handles the error; none substitutes the ambient `REWAKE_DIR` for a directory
that failed the check. At `ab8d752` `RootForRoom` is two parent hops with no check
(`rooms.go:67`), which under `v2/` would hand a child `<base>/v2` as its `REWAKE_DIR`.

**None of these creates anything.** Each resolves and, when the path exists, verifies it
as today (`Verify`, `state.go:88`: a directory, no symlink, the user's, mode 0700). At
`ab8d752` `Root`, `RoomDir` and `Dir` create the base, the room and its `sessions`,
`inbox` and `sock` (`ensureDir` at `state.go:72`, through `makeDir`), so every view
creates state. From commit 3 only the lease's bootstrap creates them
([below](#the-bootstrap)).

**Every caller, by the level it means:**

| Caller at `ab8d752` | Means | 2.0 |
|---|---|---|
| `cli/launch.go:35,43` | the base, then the room | `Base`, `Root`, `RoomDir`; created by `build.Acquire` |
| `cli/self.go:63,67` (`callerPlaybook`, for `guide` and help) | the current room | the current-room resolver, as every other command, from commit 1 (it passed the raw `REWAKE_ROOM`, so a session without one got no playbook); a room that does not exist gives no playbook |
| `cli/sessions.go:66,110` (`Directory` of `list`, `whoami`) | what `REWAKE_DIR` should be set to | `BaseForRoom` |
| `host/wrap.go:147` into `host/launch/environment.go` | the child's `REWAKE_DIR` | `BaseForRoom`: the child resolves the root once |
| `cli/send_dir.go:69`, `host/grants.go:39`, `host/grant_resume.go:66` into `grant.CurrentEnv` (`core/grant/tiers.go:45`) | two things: the state tree no grant may reach, and the rooms whose sessions' working directories `registry.WorkDirs` lists | `CurrentEnv` takes the base for the protected tree — the whole base, 1.x's `rooms/` and its candidate and validated trees included — and the root for `WorkDirs` |
| `cli/worktree_keep.go:103,108` (`runningIn`) | the rooms of the current root and of the root the worktree's owner registered in | `Root()`, and `RootForRoom(owner.Dir)`; a record written by 1.x names a 1.x room, which fails the check and is skipped — its sessions are not 2.0's |
| `core/registry/observe.go:28` (`WorkDirs`) | a root | unchanged, given the root |

**Grant addresses** hash the room directory (`AuthorityAddress`, `infra/state/paths.go:78`),
so under `v2/` every address changes, and 1.x and 2.0 never share one. The CLI and the
wrapper take the room from the same resolution — the CLI's `Dir()` from the base the
wrapper exported — so they agree; the round trip below proves it.

**The tests that build a state directory** set the base (`REWAKE_DIR`) as today; the paths
they spell gain `v2/` in commit 3 through one helper per package, not by hand. A test that
writes a room has it made by `buildtest.Room` (`internal/infra/build/buildtest`): the
acquisition of a launch of the test binary, since no resolver makes a room any more, and
the test holds that lease shared until it ends, as a session holds its room's
(`buildtest.Established` releases it at once, `buildtest.Release` before a switch); the
tool rig, whose room belongs to the binary it built, bootstraps the room, records that
binary's id and holds the lease itself, and the workflow suite names its rooms through
`Isolation.Room`. `test/toolrig/fault_test.go` normalizes and matches
`/rooms/default/...`, and its fault oracle stops matching without `v2`; the suite's
mutation paths are proved to fail again after the change.

## What is a writer

**Forms, not commands, are classified.** Each parsed form carries its class, `view` or
`change`; a form is the command plus the flags and verbs that change what it does, so
`inbox` and `inbox --owed` are two forms. The class is read from the parsed call before
the handler and before anything that can prune; a table test fails on a form without one,
so S18 classifies `decide`'s two forms when it adds them
([stage3-decision.md](stage3-decision.md#the-command)). There is no `clear` command:
conversation reset is not built, and no lease entry is added for it or for the hidden
hook and server commands removed in S8 and S9.

| Class | Forms | Why |
|---|---|---|
| change | a launch, with or without `--worktree`; `send` in every kind; `withdraw`; `edit`; `inbox`, `inbox --message`, `inbox --next`; `pending`; `retry`; `compact`, `interrupt`; `worktree rm`, `land`, `finish` | they write the room |
| change | `inbox --peek` | it takes the mailbox lock (`cli/inbox.go:76`), which orders it against a read in progress; taking the lock is a write, so it holds a lease rather than losing the lock |
| change | **every authorized tool call**, whatever its words, `--help` included | its binding and its bounded answer may keep a continuation (`freezeOutput` in `cli/output_parts.go`, held by `TestALongAnswerComesInParts`), so it is classified before authorization can prune and holds the lease through the bounded output; no read-only continuation store is designed in S17 |
| view | bare `rewake`, `guide`, `--help`, `<command> --help`, `--version`, each with `--json`; `list`; `whoami`; `inbox --owed`, `inbox --awaited`; `worktree ls`, and a `worktree` call with no change verb, which its handler refuses before any effect | they answer from what is there |

A call refused before it parses — an unknown command or flag, a stray argument, an alias
that does not expand — has no form: it is answered before the state is reached, takes no
lease and writes nothing.

**One scope per invocation.** In the shell it runs from the class through the handler,
`observeShell` and the printing of the answer or the refusal; through the tool, from the
class through the ticket's confirmation, the handler and `boundedAnswer`, which may write
after the handler returned. From commit 2 the lease is held for exactly that scope. A
lease refused ends it before the handler and before `observeShell`: the refusal is
printed and nothing is written, no shell note either — the note is a write of a room the
call could not acquire.

**Views stop pruning**, at the places the inventory found them writing:

- `Dir()` and the roots create nothing (commit 3, above);
- `ownRun` (`cli/self.go:31`) looks the caller up with `LookupReadOnly`
  (`core/registry/observe.go:13`) in a view, instead of `Lookup`
  (`core/registry/registry.go:264`), which prunes a dead record under the name lock;
- `list` reads with `ListReadOnly` instead of `List` (`cli/sessions.go:40`), which prunes
  every dead record, and keeps reporting unreadable records as unknown, the session
  snapshots and S11's `Live` field;
- `observeShell` (`cli/shell_channel.go:29`), which writes a shell note after a call, runs
  only after a change form, and still skips `--peek`, which writes nothing to observe;
- pruning a dead record happens only in a change form, under its lease.

**What a view guarantees** (D3): no mutation of the state by the view process — no
temporary or lock file, no pruning, no observation, on error paths too; no lease taken
and no wait for one. It is not a consistent snapshot across files, and it does not stop
a running wrapper from writing meanwhile. Access times are outside the guarantee. A
missing base, root or room gives the ordinary empty or not-a-session answer, creating
nothing. A view of another build says so once on stderr, under `--json` and on help
pages too; a build that cannot be read is "unknown", never taken for the same one.

**A view cannot write by accident.** From commit 3 `cli.Run` puts the process in view mode
for a view form, and every write primitive of `infra/state` refuses there with
`ErrViewWrite`. Taking a lock counts as a write: `TryHold`, whose answer is a bool, gives
a view false before it opens the file. The mode keeps the evidence of an attempted write,
so a view that swallows the error, or reads only the bool, still fails its invocation and
its test. A view refusal takes
precedence over a missing lease.

**Every write goes through the seam** (commits 5 and 6). The write primitives of
`infra/state` — `WriteAtomic`, `WriteAtomicHeld`, `PublishExclusive`, `Remove`, `Rename`,
`EnsureSubdir`, the locks (`WithRoomLock`, `WithNameLock`, `TryWithNameLock`,
`WithMailboxLock`, `WithMailboxLockAt`, `TryHold`) — refuse with `ErrNoLease` when the process holds
no lease for that room: one guard (`guard.go`) checks view mode on every path first, then
whether each lies in a room `Authorize` recorded, by cleaned prefix, so a write of another
room refuses like one outside every room. A lock is recorded once however often it is
authorized, and its authorization ends the moment it may hold nothing: its `Close`, its
`Unlock`, a conversion refused or any failed take, which may drop the lock it had. The writes under the room that bypass them are each routed
through a primitive, a new one where none fits: `OpenFile` (a write only with a write flag),
`WriteFile`, `Link`, `Move`, `Chmod`, `Touch`, `Discard`, `RemoveAll`, `Flock` (taking a
lock is a write, letting it go is not), each the os call it names behind the same guard and
none asking the fault plan, so a call's fault schedule counts the steps it counted before
(`Discard` and `Move` are cleanup; `Remove` and `Rename` stay the counted ones). One write
lies outside every room by the harness's protocol: a reply socket beside a socket the caller
named. `state.ClaimSocket` gives the run authority over that one path, no directory — clearing
a dead socket there, binding, the close-time unlink — checked at each act against view mode
and the room's lease. It widens nothing of any state: a path in a state directory, as written
or where its directory's links lead — a base of any root, a root with rooms, a room of either
layout by what it holds — is guarded by the lease of the room it lies in, which the claiming
room's lease is not. Then the guarded resources:

- commit 5: the registry's removals; `core/control`'s lock, request, mark and answer files
  and `serve.go`; in `core/mail` a direct write, a truncate without create, a hard link,
  the `RemoveAll`s, an exclusive create and its removal, the touches of retention, and
  `settle.go`'s function value with its `rewakefault` override; `receipt.go`'s lock, its
  identity recheck kept; `grant/journal.go`; `host/delivery`'s retention touch;
- commit 6: the read clock, `core/mail/read_boundary.go` — a shared writable mapping, a
  truncate, a store through an atomic word and a sync of an open descriptor — through
  `state.Counter`, a guarded resource, the order reserve the high-water mark, persist the
  waiter, publish the word kept, and `Snapshot` free of filesystem access and blocking locks;
- commit 6: the sockets — the endpoint's (creation, removal, mode), the wrapper's
  (`host/claim.go`), the fixture's and the adapters' — bound by `state.Listen`, whose
  close removes the path once, behind the guard, in place of Go's own unlink;
- the lock files themselves (`state.go`), opened inside the primitives.

Publication and heartbeat keep their semantics rather than becoming atomic renames: a
heartbeat never recreates a removed mark. The grant authority uses an abstract socket and
creates no file. `mail.PublishOnce` keeps S3's section: it
takes the recipient's mailbox, checks the live epoch through `LookupReadOnly` inside it,
then reads the proof and publishes, and a seam helper takes no name lock and does not
take the same mailbox again there.

A static test over `internal/`, in all four build variants, fails on a write outside
`infra/state` — a direct call, an alias, a function value, a descriptor's mode or owner, a listen not proven abstract — except in its own table of
file, effect and reason, which fails on a stale entry; what it reads and excuses is in
[stage3-state-tests.md](stage3-state-tests.md#stage-3-the-tests-of-state-and-builds), with the
worktree's writes beside a checkout the main class. A refusal for want of a lease is
counted (`state.NoLeaseRefusals`), so a swallowed one still shows in the tests. The same
lease rule covers `tools/` and `test/`, which call the CLI; where a driver there writes a room
itself — the tool rig, the forged letters of a workflow case — it holds the room's lease as a
session does. An adapter's writes end inside its `Close`: the fixture's backend joins its
routines, and the lane removes its reply socket there, its readers keeping to memory after;
a write that came later would meet `ErrNoLease` rather than land.

## One build per room

**The build id** is the SHA-256 of the executable's content, as
[design-state.md](design-state.md#one-build-per-room) decided: `infra/build` hashes
`/proc/self/exe` once per process, on first use, with `crypto/sha256` over a streamed
read. An image that cannot be opened or read gives no id, and the process refuses before
its first effect — before even the bootstrap creates anything — exit 1, naming why.
`--version --json` carries it as `build`; the human version line is unchanged.

**One identity, peers included.** The endpoint's peer check (`host/endpoint/peer.go`,
`sameBuild`) compares the executable's device and inode today, which refuses a
byte-identical copy the lease admits; it compares the content id from commit 2. The tool
child (`host/endpoint/run.go`) and the launch probe already start from `/proc/self/exe`;
`os.Executable` in the environment's `PATH` and in grant shielding is no self-spawn and
stays. The probe holder writes no room state.

**The lease**: `build.lock` in the room, with `flock(2)`, and `build` beside it.

- **Writers** — a wrapper for its life, from before it claims a name to after its last
  callback; a change form for its scope ([above](#what-is-a-writer)); each child that
  writes, its own — take it shared, then read `build`: their own id goes on; another id,
  or a missing, unreadable or malformed record, refuses, exit 1, naming the room's build
  and the way past it (end the room's sessions, or `--room`). Only a launch establishes
  a build, so a writer makes nothing of a room no launch completed: it refuses there and
  leaves the base as it found it.
- **Changing the build.** A launch whose id differs from `build`, or that finds it
  missing, unreadable or malformed, asks for the lock exclusive without waiting, without
  keeping a shared descriptor of its own beside the request. Refused, it reads `build`
  again under the shared lock and is granted on its own id, or asks again until two
  seconds have passed (D4a, under the lock order below); then exit 1, naming the holders.
  Taken: it writes `build`, converts to shared, and reads `build` again like any
  writer, since `flock` does not promise the conversion is atomic; it reads again after
  every reacquisition.
- **Holders** are found in `/proc/locks` by the device and inode of `build.lock` (an inode
  alone is not unique across devices) and named by pid and a safe label — the
  executable's base name and, when it is a session, its name — never a command line:
  diagnostics say where, never what (L2). Missing, hidden or changing holder details make
  the diagnostic poorer and never permit the exclusive lock.
- **Views** take no lock; when `build` differs from their own id they add one line, and
  say "unknown" when it cannot be read.
- **Named steps** of the fault seam order a process against another at the lease:
  `bootstrap <level>` before each level is made, `lease switch` once a launch holding the
  lease shared has read a record not its own, `lease exclusive` once a launch holds it
  exclusive, `lease downgrade` before it turns shared, `leased` once the lease is held
  and authorized; and a note, which a fault plan logs and never counts, `lease release`
  before it goes — after an invocation's last effect, where ending the process is no new
  case.
- **What a wrapper runs for itself** is started from `/proc/self/exe`, never from a path
  looked up again.

**Who owns what** (D5). `infra/state` owns an opaque authorization scoped to one room;
`infra/build` owns the identity and the acquisition and calls `state`, which never imports
`build`. No ambient boolean and no environment variable grants writing. The room is named
canonically, the authorization is void after release, the descriptors are close-on-exec,
and each writer child acquires its own. The CLI hands its acquisition to the direct entry
of the host rather than holding a shared lease while the host asks for an exclusive one;
`host.Run` called without one acquires it itself. Every callback and resource stops before
the lease closes: the live-set loop of S11 (`registry.Modify` keeps reading the record as
it stands), the channel and availability writers, the turn callbacks
(`core/mail/turn_end.go`, `turn_reports.go`, `turn_hold.go`, `pending_mark.go`,
`read_ack.go`), delivery (`host/delivery`, whose locked entry points keep their four
callers), the endpoint's acknowledgments and every cleanup. A stop that returns before
its work has stopped proves nothing, so the run counts what may still write: the
availability publisher, the session-state worker and every write of the live set, a
retry among them, are workers of the run, and the run keeps its own hold on the lease
(`build.Lease.Keep`, beside the caller's) until the last of them has returned. Their
stops wait only so long, so a write stalled in the filesystem does not hold the
wrapper's exit: `Run` may return first, and the lease then goes when that worker ends,
or with the process, and the run's directory with it ([stage3-rundir.md](stage3-rundir.md);
`TestRunKeepsTheLeaseAndItsDirectoryUntilItsWorkersStop`,
`TestALiveSetWritesNothingAfterItsStop`, `TestAStoppedWorkerCountsUntilItsLastWrite`). A
stopped live set starts no write; the channel keeper's heartbeat, the endpoint, delivery
and the fixture's backend are joined by their stops. A worktree change form takes
the selected room's lease; its Git locks still protect the checkout. Looking at sessions
of another root takes no lease there. Tests in one process restore the mode and the lease
between invocations; no test escapes the lease by a global switch — a helper acquires a
real lease or builds an explicit tested authority.

**Lock order and bounds** (D4). The build lease comes before every other lock of the
room — launch, name, mailbox, receipt, control — and before a worktree is created; no
path takes it while holding any of them, and there is no upgrade under a mailbox lock. A
launch's clearing takes the global clear lock while holding the lease and releases it
before the claim. A shared acquisition is bounded when an exclusive switch stalls;
the exclusive request does not block.

A refused exclusive request is not yet a refusal (D4a, decided October 11, 2026, after the
workflow suite found two launches of one build into a fresh room refusing each other). Two
launches that both read the record missing, or another build's with nothing holding the
room, both ask for the lock exclusive, and flock's conversion lets one through. The other
takes the lock shared again within the writers' bound and reads the record: its own build
means the winner switched the room, and the lease is granted. Otherwise it lets go, waits a
short random pause, and asks again. Two budgets bound this, and they are separate. The
retries have two seconds (`switchWait`), checked after each shared reacquisition and only
once the record is not the launch's own: past them the launch is refused naming the
holders, so a room held by another build's sessions answers after about two seconds rather
than at once. Each shared reacquisition has the writers' own bound (`shareWait`, ten
seconds), which a competing switch that stalls while holding the lock exclusive uses up:
then the launch is refused as busy, after up to that bound past the two seconds. Two
seconds is the budget against shared holders, not an end-to-end bound. The mailbox and
name discipline is unchanged: no pruning and no name lock inside a mailbox section, and
the bounded cross-mailbox publication of S3 stays.

### The bootstrap

The lease lives in the room, so establishing it writes before any lease exists. These
writes are primitives of `infra/state`, the only ones that work without a lease — in
view mode they refuse like every other — and `build.Acquire` is their one caller. A test
over the module fails on any other reference to them, not only a call written where they
are named: a function value, an argument, a dot import, a second import name, a variable
inside `infra/state`, a reference nested in the receiver of a selector. They take a
narrow authority, not a public exemption for arbitrary paths:

1. `state.Bootstrap(base, room)`: the base, the root, the room and its `sessions`,
   `inbox`, `sock`, each created with mode 0700 when missing and verified when present,
   never repaired by `chmod`; each level, made or found, is synced into its parent, and a
   failed sync stops the launch before its claim. A level found is synced too: one a
   launch made and failed to flush, or a concurrent launch made and has not flushed yet,
   exists without its name being durable, so finding it proves nothing. It is idempotent:
   a crash may leave an incomplete empty skeleton, which the next launch completes.
2. `state.CreateLeaseLock(room)`, a launch's: `build.lock`, opened with
   `O_CREAT|O_CLOEXEC`, mode 0600, and locked; after the lock is taken, the open file and
   the path must be the same file (as `receipt.Lock` checks), or it is opened again — and
   created again. Release never unlinks it. A writer's `state.OpenLeaseLock(room)` opens
   the same file without `O_CREAT`, the first time and every time again: a room a launch
   left before its lock, or whose lock is gone, is a room without a build, and the writer
   refuses it as it found it.
3. `state.WriteBuildRecord(room, id)`: `build`, written by rename and sync, only under the
   exclusive lock.

So `infra/build` writes nothing itself. A view does none of these. The clearing of 1.x
needs more than a room's lease — renaming trees under the base and locking
`v2/.clear.lock` — and takes a separate capability scoped to the base for it
([stage3-upgrade.md](stage3-upgrade.md#the-cleanup-authority)).

## The run directory

The directory of one run, what it holds and when it goes, with the qualified
guarantee of its removal, are in [stage3-rundir.md](stage3-rundir.md).

## The tests

Each decision is held by a test that fails on a mutant of it; the tests, by area and
commit, and the oracles kept are in [stage3-state-tests.md](stage3-state-tests.md).

## The cost of the hash

**Measured in S17, before commit 7**, on an idle machine (`AGENTS.md`: no timing under
another heavy run), on a binary built as `scripts/pack.sh` builds the release
(`CGO_ENABLED=0`, `-trimpath`, the version and time `-ldflags`, not stripped): the
streamed SHA-256 warm, and with the executable's pages advised away with
`posix_fadvise(POSIX_FADV_DONTNEED)` before each sample — recorded as a cache drop
requested, not as a cold cache, unless the residency is shown. The end-to-end time of one
change form — `rewake pending` against a prepared session, task and turn, so the measured
call succeeds — over 100 separate processes with the hash and 100 with only the hash
disabled in a test build, the lease and the I/O otherwise equal, the two builds in
separate rooms: median and 95th percentile, exit codes checked, the raw samples kept, with
the artifact's size, id and build flags and whether the advice succeeded. No production
flag turns the hash off. A first look, not evidence: on October 5, 2026 a 14.3 MB build
hashed warm in under 10 ms with `sha256sum`, under a load average of 2.6.

**The id stays the SHA-256 of the image whatever the numbers say.** Nothing here
switches to another identity on a threshold. If the cost is judged too high, the numbers
go to the owner with a separate proposal that states its weaker guarantee exactly and
comes with the adversarial test it would fail — two images with the same embedded
toolchain build ID and different bytes, for one — and the identity changes only on the
owner's answer. The numbers are linked from `docs/rules/state.md` in S19.

## Still unknown

- Whether `flock` on the state directory behaves inside a Codex sandbox: stage 5's to
  verify ([design-state.md](design-state.md#one-build-per-room)).
- Whether two installs side by side — a development build and the published one —
  should warn rather than refuse on change forms; the design refuses, and this pass
  keeps it.
