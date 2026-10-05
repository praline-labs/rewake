# Stage 3: state and builds

How S17 builds what [design-state.md](design-state.md) decided: the state root `v2/` and
its inverse, what is a writer, one build per room, and the measurement of the hash.
Clearing the state of 1.x is in [stage3-upgrade.md](stage3-upgrade.md). Citations are
to `b6ed4ed`. S17 lands as five commits, each green, in an order where each needs only
what an earlier one made:

1. **The build id and the lease**, on today's layout, with the bootstrap that creates the
   room's directories ([below](#the-bootstrap-writes)); the resolvers still create too,
   so nothing depends yet on the bootstrap alone. Writers take the lease; nothing refuses
   without one.
2. **The roots**: `v2/`, the checked inverse, and resolvers that create nothing — from
   here the bootstrap is the only creation, and every writer reaches it through the lease
   it already takes.
3. **Views read-only and the forms classified**; view mode refuses with `ErrViewWrite`.
4. **The writes routed through the seam**; `ErrNoLease` is enabled here, with the static
   test.
5. **The clearing of 1.x** ([stage3-upgrade.md](stage3-upgrade.md)).

At every one of them, the tests run a launch on an empty base and a session's change
forms after it; from the second, every view form on an empty base and on a room that does
not exist, the base's bytes unchanged; from the third, a view that would write fails;
from the fourth, a write without a lease fails.

## The roots

Three levels, each with its own function; at `b6ed4ed` only two exist, and `Dir()`
(`state/rooms.go:27`) returns a **room**, not a root.

| Level | Function in 2.0 | Value | At `b6ed4ed` |
|---|---|---|---|
| base | `state.Base()` | `$REWAKE_DIR` (absolute) or `os.TempDir()/rewake-<uid>` | `state.Root()` (`state/state.go:50`) |
| root | `state.Root()` | `<base>/v2` | — |
| room | `state.RoomDir(root, room)`; `state.Dir()` for the current one | `<root>/rooms/<room>` | the same names, over the base |

The inverse is checked, never counted: `state.RootForRoom(room)` answers the root only
when the room's parent is named `rooms` and its grandparent `v2`, and an error otherwise;
`state.BaseForRoom(room)` is that root's parent. At `b6ed4ed` `RootForRoom` is two parent
hops with no check (`rooms.go:67`), which under `v2/` would hand a child `<base>/v2` as
its `REWAKE_DIR` and make it resolve `<base>/v2/v2`.

**None of these creates anything.** Each resolves and, when the path exists, verifies
it as today (`Verify`, `state.go:88`: a directory, no symlink, the user's, mode 0700). At
`b6ed4ed` `Root`, `RoomDir` and `Dir` create the base, the room and its `sessions`,
`inbox` and `sock` (`state.go:58`, `rooms.go:36-58`), so every view creates state. From
S17's second commit only the lease's bootstrap creates them
([below](#the-bootstrap-writes)), which the first commit put in place.

**Every caller, by the level it means** (from the inventory of callers, `go/types`):

| Caller | Means | 2.0 |
|---|---|---|
| `cli/launch.go:40,48` | the base, then the room | `Base`, `Root`, `RoomDir`; created by `build.Acquire` |
| `cli/self.go:83,87` (`callerPlaybook`, for `guide` and help) | the room of `REWAKE_ROOM` | resolved without creating; a room that does not exist gives no playbook |
| `cli/sessions.go:51,89` (`Directory` of `list`, `whoami`) | what `REWAKE_DIR` should be set to | `BaseForRoom` |
| `wrap/wrap.go:158` into `harness/environment.go:43` | the child's `REWAKE_DIR` | `BaseForRoom`: the child resolves the root once |
| `cli/send_dir.go:69`, `wrap/grants.go:44`, `wrap/grant_resume.go:65` into `grant.CurrentEnv` (`grant/tiers.go:41`) | two things: the state tree no grant may reach, and the rooms whose sessions' working directories `registry.WorkDirs` lists (`registry/observe.go:21`) | `Env` takes the base for the protected tree — 1.x's leftovers included — and lists rooms under `RootOf(base)` |
| `cli/worktree_keep.go:103-114` (`runningIn`) | the rooms of the current root and of the root the worktree's owner registered in | `Root()`, and `RootForRoom(owner.Dir)`; a worktree record written by 1.x names a 1.x room, which fails the check and is skipped — its sessions are not 2.0's |
| `registry/observe.go:22` (`WorkDirs`) | a root | unchanged, given the root |
| `wrap/mailtool_launch.go:58,88`, `cutover/name.go:100` | — | removed in S8 and S2 |

**Grant addresses** hash the room directory (`AuthorityAddress`, `state/paths.go:95`),
so under `v2/` every address changes, and 1.x and 2.0 never share one. The CLI and the
wrapper take the room from the same resolution — the CLI's `Dir()` from the base the
wrapper exported — so they agree; the round trip below proves it.

**The tests that build a state directory** set the base (`REWAKE_DIR`) as today; the
paths they spell (`rooms/default/...` in about ten unit tests and in the suite,
`regression_test.go:57`, `isolation_test.go:225,237` and the others of the inventory)
gain `v2/` in the second commit, with the roots they spell, through one helper per
package rather than by hand; the first commit keeps today's layout and its paths.

**Tests.**

- *Two rooms*: launches into two rooms of one base; each room is `<base>/v2/rooms/<r>`;
  nothing else appears under the base; `list` in each shows its own.
- *The round trip*: a wrapper's child — a shell of the session and the CLI child of a
  tool call — reads the `REWAKE_DIR` the wrapper exported; its `Dir()` equals the
  wrapper's room byte for byte, with exactly one `v2` component; a grant it sends reaches
  the wrapper's authority.
- *No ordinary operation in the 1.x tree*: with a 1.x `rooms/` present and its clearing
  deferred, every form of the command table runs, and the 1.x tree's bytes are unchanged.

## What is a writer

**Forms, not commands, are classified.** Each command form in the command table carries
its class, `view` or `change`; a form is the command plus the flags that change what it
does, so `inbox` and `inbox --owed` are two forms. `cli.Run` reads the class from the parsed call before the handler, and a table
test fails on a form without one, so S18 classifies `decide`'s two forms when it adds
them ([stage3-decision.md](stage3-decision.md#the-command)). The classes at `b6ed4ed`,
with the writes the inventory found:

| Class | Forms | Why |
|---|---|---|
| change | a launch; `send` in every kind; `withdraw`; `edit`; `inbox`, `inbox --message`, `inbox --next`; `pending`; `retry`; `compact`, `interrupt`, `clear`; `worktree rm`, `land`, `finish` | they write the room |
| change | `inbox --peek` | it takes the mailbox lock (`cli/inbox.go:79`), which orders it against a read in progress; taking the lock is a write, so it holds a lease rather than losing the lock |
| view | bare `rewake`, `--help`, `<command> --help`, `--version`, `guide`; `list`; `whoami`; `inbox --owed`, `inbox --awaited`; `worktree ls` | they answer from what is there |

**Views are made read-only**, at the places the inventory found them writing:

- `Dir()` and the roots create nothing (above);
- `ownRun` (`cli/self.go:39-62`) looks the caller up with `LookupReadOnly`
  (`registry/observe.go:7`) in a view, instead of `Lookup`, which prunes a dead record
  under the name lock (`registry.go:211-231,268-288`);
- `list` reads with `ListReadOnly` (`registry.go:294`) instead of `List`, which prunes
  every dead record (`:291,315-317`);
- `observeShell` (`cli/shell_channel.go:29-69`), which writes a shell note after a call,
  runs only after a change form, as it already skips `--peek`, `--owed` and `--awaited`;
- pruning a dead record happens only in a change form, under its lease.

**A view cannot write by accident.** `cli.Run` puts the process in view mode for a view
form, and in view mode every write primitive of `infra/state` refuses with
`ErrViewWrite`, so a view that would write fails in its first test. A view of a room
that does not exist answers that, without creating it.

**Every write goes through the seam** (the fourth commit). The write primitives of `infra/state` —
`WriteAtomic`, `WriteAtomicHeld`, `PublishExclusive`, `Remove`, `Rename`, `EnsureSubdir`,
the locks (`WithRoomLock`, `WithNameLock`, `TryWithNameLock`, `WithMailboxLock`) — refuse
with `ErrNoLease` when the process holds no lease for that room. The inventory found
writes under the room that bypass them; each is routed through a primitive — a new one
where none fits, `Link` and `RemoveAll` among them — in the same fourth commit:

- `registry.go:112,175,231` (`os.Remove`);
- `control.go:126-129,156-164,241-245`, `control/serve.go:46,88-92`;
- `inbox`: `answer_reservation.go:21`, `answer_mark.go:35`, `read_boundary.go:47`,
  `unread.go:29` (`os.Link`), `once.go:214`, `marks.go:151`, `waiters.go:326`
  (`os.RemoveAll`);
- `receipt.go:265` (its lock, with its identity recheck kept);
- `wrap.go:134,380`; `bridge/endpoint/endpoint.go:72-78` (the socket's removal and mode);
- `grant/journal.go:157`;
- the lock files themselves (`state.go:251,281,299`), opened inside the primitives.

A static test over `internal/` fails on a direct call that writes — `os.WriteFile`,
`os.Create`, `os.OpenFile` with a write flag, `os.Remove`, `os.RemoveAll`, `os.Rename`,
`os.Link`, `os.Mkdir*`, `os.Chmod`, `syscall.Flock`, a listener on a filesystem path —
outside `infra/state`, except in its own table of file, call and reason: `worktree`
writes beside a checkout, outside the state (`worktree/records.go:155-157`); test files.
The same lease rule covers `tools/` and `test/`, which call the CLI, not the primitives.

## One build per room

**The build id** is the SHA-256 of the executable's content, as
[design-state.md](design-state.md#one-build-per-room) decided: `infra/build` hashes
`/proc/self/exe` once per process, on first use, with `crypto/sha256` over a streamed
read. An image that cannot be opened or read gives no id, and the process refuses before
its first effect, exit 1, naming why. `--version --json` carries it as `build`.

**The lease**: `build.lock` in the room, with `flock(2)`.

- **Writers** — a wrapper for its life, taken before it claims a name; a change form in
  `cli.Run` before its handler, to its exit; a child the wrapper starts, its own — take it
  shared, then read `build`: their own id goes on; another id or an unreadable file
  refuses, exit 1, naming the room's build and the way past it (end the room's sessions,
  or `--room`).
- **Changing the build.** A launch whose id differs from `build`, or that finds it
  missing or unreadable, takes the lock exclusive without waiting. Refused: exit 1,
  naming the holders from `/proc/locks` (the lines whose inode is `build.lock`'s, mapped
  to their command lines). Taken: it writes `build`, converts to shared, and reads `build`
  again like any writer, since `flock` does not promise the conversion is atomic.
- **Views** take no lock; when `build` differs from their own id they add one line.
- **What a wrapper runs for itself** is started from `/proc/self/exe`, never from a path
  looked up again.

### The bootstrap writes

The lease lives in the room, so establishing it writes before any lease exists. These
writes are three primitives of `infra/state`, the only ones that work without a lease —
in view mode they refuse like every other — and `build.Acquire` (in `infra/build`) is
their one caller, which a test over the module checks:

1. `state.Bootstrap(base, room)`: the base, the root, the room and its `sessions`,
   `inbox`, `sock`, created with mode 0700 and verified (today's `ensureDir`,
   `state.go:68-86`);
2. `state.OpenLeaseLock(room)`: `build.lock`, opened with `O_CREAT`, mode 0600, and
   locked; after the lock is taken, the open file and the path must be the same file (as
   `receipt.Lock` checks, `receipt.go:262-285`), or it is opened again;
3. `state.WriteBuildRecord(room, id)`: `build`, written by rename and sync, only under
   the exclusive lock.

So `infra/build` writes nothing itself, and the static test below needs no exception for
it. A view does none of these.

## The tests

`internal/infra/build` unit tests: the id of a copy equals the original's; any changed
byte changes it; an unreadable image refuses; shared and exclusive exclude each other;
the holders are named from `/proc/locks`; the downgrade reads `build` again; a replaced
`build.lock` is opened again.

The concurrent scenarios, as tests in `internal/host` that build real binaries in a
temporary module copy (as `mutant_test.go` builds with `-overlay`) and run them as
processes with their own base:

1. **Two builds of one revision from different trees**: the same commit, the second
   with one string literal changed through an overlay. Both print the same `--version`
   line — asserted, so the test shows why the version is no identity — and their ids
   differ. With a session of the first live in a room, a launch of the second into that
   room is refused, naming the session.
2. **A copy of the same artifact** launched into the room is admitted.
3. **An old writer while a new build launches**: a command of the old build held by the
   fault seam after it took its lease; the new launch is refused, naming it; the command
   exits, or is killed, and the launch goes on.
4. **An orphaned child of the old build**: held before it takes its lease; the wrapper is
   killed; the new build launches; the child resumes, takes the lease, finds the new id
   and refuses before its first effect.
5. **Views of another build**: build A holds the lease, dead session records are
   present; every view form of build B works, says the build differs once, and the room's
   bytes — hashed before and after — are unchanged.
6. **A writer without a lease**: every write primitive refuses without one
   (`ErrNoLease`), and in view mode (`ErrViewWrite`); every change form, run once, holds
   the lease at each primitive it reaches.
7. **An unreadable `build`** is rewritten only by a launch holding the lock exclusive.

## The cost of the hash

**Measured in S17**, on an idle machine (`AGENTS.md`: no timing under another heavy run),
on the binary as the release builds it: a benchmark of the id with the page cache warm
and cold (the file's pages dropped with `posix_fadvise(POSIX_FADV_DONTNEED)` before each
run, which needs no privilege); the end-to-end time of one change form (`rewake pending`
in a test room), median and 95th percentile over 100 runs, with the id and without. A
first look, not evidence: on October 5, 2026 a 14.3 MB build hashed warm in under 10 ms
with `sha256sum`, under a load average of 2.6.

**The id stays the SHA-256 of the image whatever the numbers say.** Nothing here
switches to another identity on a threshold. If the cost is judged too high, the numbers
go to the owner with a separate proposal that states its weaker guarantee exactly and
comes with the adversarial test it would fail — two images with the same embedded
toolchain build ID and different bytes, for one — and the identity changes only on the
owner's answer. The numbers are written into `docs/rules/state.md` in S19.

## Still unknown

- Whether `flock` on the state directory behaves inside a Codex sandbox: stage 5's to
  verify ([design-state.md](design-state.md#one-build-per-room)).
- Whether two installs side by side — a development build and the published one —
  should warn rather than refuse on change forms; the design refuses, and this pass
  keeps it.
