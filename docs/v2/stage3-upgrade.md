# Stage 3: clearing the state of 1.x

How S17 builds the clearing that [design-state.md](design-state.md#clearing-the-state-of-1x)
decided: at the first 2.0 launch that finds a 1.x tree, refuse while a 1.x session runs,
and otherwise delete the tree. The owner decided that 1.x state is deleted after a safe
shutdown, not set aside, and that nothing is stopped automatically; this file keeps both.
What it adds is how "safe" is established, kept apart from deleting, and what happens at
every crash and every read that fails. Citations of 1.x code are to `b6ed4ed`, the last
revision with 1.x's layout. The Codex side's readiness review of October 11, 2026,
adopted by main on October 12 (D1, D4, D6–D9), corrected this plan; it is the seventh
commit of S17 ([stage3-state.md](stage3-state.md#the-sequence)).

## What 1.x does that the clearing has to meet

- A 1.x launch creates its room before it claims (`cli/launch.go:48`, `RoomDir` creates
  `rooms/<room>` and its directories), then takes `<room>/.launch.lock` with a blocking
  exclusive `flock` (`wrap/claim.go:41`, `state/state.go:298-310`), with no check that the
  locked file is still the one at the path. It holds the lock only while it claims and
  publishes its session record; a running session holds no lock at all.
- Every 1.x command resolves its room through `Dir()`, which recreates `rooms/<room>` and
  its directories (`state/rooms.go:27-58`), and writes by path string.
- So a 1.x process cannot be fenced by a lock it does not take, and one that runs across
  a rename writes into a recreated `rooms/`, not into the renamed tree.

## The supported upgrade

**An externally quiesced 1.x generation**: the person ends every 1.x session and runs no
1.x command, then launches 2.0 — what the release notes and `docs/install.md` say
([design-state.md](design-state.md#clearing-the-state-of-1x)). rewake cannot prove that
state: a 1.x command between two locks is visible to nothing. So the protocol proves what
it can, **refuses a launch whenever a live 1.x session is seen or a check cannot be
made**, and never deletes a tree that did not pass validation after it was set aside.
What it cannot see is outside the supported upgrade, and its reach is bounded: such a
command writes into a recreated `rooms/`, or fails; it never touches `v2/`, and the next
launch finds the recreated tree and runs the protocol on it.

**A precondition: one pid namespace.** The check of a wrapper compares two numbers with
`/proc`, which means something only when the launch sees the pid namespace the 1.x
wrappers ran in. A 1.x record names its namespace (`pidNamespace`, written since
September 16, 2026, by every released 1.x from 1.0.0 on); a record naming another one or
none, or a launch that cannot read its own, cannot establish the precondition, and the
launch refuses. The protocol does not claim to see
wrappers hidden in a namespace the launch cannot read.

## The trees and their states

Every name below is directly under the base, and nothing else outside `v2/` is touched.

| Name | State | Reachable by 1.x paths |
|---|---|---|
| `rooms/` | live 1.x state | yes |
| `.rooms-1x-candidate-<id>` | set aside, **not validated** | no, but a 1.x process may hold a lock or a file in it |
| `.rooms-1x-validated-<id>` | validated after it was set aside: safe to delete | no |

`<id>` is the clearer's pid, its process start and a random part, so it cannot recur
across boots; a rename onto a name that exists fails rather than replacing it. A tree's
state is its name: a rename is the only transition, followed by a sync of the base
directory, and a transition counts only once that sync succeeded. A validated tree is
never renamed back; a candidate becomes validated only by a validation run after it was
set aside, by whoever runs it. Every name is checked with `lstat` for a directory before
it is used, and nothing follows a symlink out of the base.

## The cleanup authority

Clearing renames trees under the base and locks `v2/.clear.lock`, which a room's lease
cannot authorize: the seam refuses those paths to a room-scoped authorization. The launch
takes a separate capability scoped to the base, from `infra/build` while it holds its
room's lease, for exactly these names, and releases it before its claim (D4, D6).

## The protocol

At every launch, after the lease's bootstrap has made the root
([stage3-state.md](stage3-state.md#the-bootstrap)) and before the launch's own claim,
the launch reads the base directory once. If none of the names above is there, it goes
on. Otherwise it runs both parts below; other commands never look. The first read only
chooses the fast path: the directory is read again once the clear lock is held.

**One at a time.** Take `<base>/v2/.clear.lock` exclusive, waiting at most the clear
bound (10 s; T9). Not taken within it: the launch **refuses**, exit 1, "another launch is
checking the state of rewake 1.x; launch again". A held lock is never read as an answer
about 1.x.

**Admission — whether this launch may go on.** Under the clear lock, over `rooms/` and
every candidate tree:

1. For each room, open `.launch.lock` (created if missing, so a 1.x launch that opens it
   later finds this inode) and take it exclusive, waiting at most 2 s per room. There is
   no separate total: the total is the number of rooms times 2 s, and with many rooms the
   launch says on stderr that it is checking them. Then try each name lock
   (`sessions/.<name>.lock`) and mailbox lock (`inbox/<name>/.lock`) that exists, without
   waiting, and release it at once: these are probes, not held. A lock not taken:
   **refuse**, exit 1, naming the room and the lock — a 1.x command is running there.
2. For each `sessions/<name>.json`, read only `servicePid`, `serviceStart` and
   `pidNamespace` (`docs/session-record.md`) and look at `/proc/<pid>/stat`: no
   `/proc/<pid>` is proof the process is gone; another start time is another process; a
   zombie is not running; a permission, parse or read failure is unknown. `proc.Alive`,
   which takes a failed read for death, is not used, and a zero pid or start, or no
   namespace, is an unreadable record. This read is a new legacy mark, beside the seven of
   [legacy.md](../legacy.md): `// legacy(rewake <YYYY-MM-DD>): a 1.x session record is
   read for its wrapper's pid, start time and pid namespace only, to see whether a 1.x
   session still runs; remove when no session started by an earlier build is
   registered`, dated the day S17 lands. A live wrapper: **refuse**, exit 1, naming each
   live session and its room, saying that 2.0 does not read 1.x mail and those sessions
   have to be ended.
3. A directory that cannot be listed, a record or a `stat` that cannot be read, a record
   of another pid namespace: **refuse**, exit 1, naming the path and saying the person may
   remove it once no 1.x session runs. An unknown is not taken for absence.

Admitted: no live 1.x session, no 1.x lock held, every check read. The `.launch.lock`
descriptors of `rooms/` stay open and locked for the clearing; the rest are released.

**Clearing — what may be deleted.** Still under the clear lock, after admission:

4. **Finish validated trees.** Each `.rooms-1x-validated-*` is deleted. A deletion that
   fails part way is finished by the next launch; only a validated tree is ever deleted
   in part.
5. **Set `rooms/` aside**, if present: rename it to `.rooms-1x-candidate-<id>` while its
   room locks are held, and sync the base. A failed rename or sync stops the clearing: a
   rename whose durability is unknown is no ground to delete, and the launch refuses with
   what failed.
6. **Validate every candidate** — this one and any left by a clearer that died — on its
   contents now, never on what a clearer remembers. For this launch's own candidate the
   room locks taken in step 1 are reused: each descriptor is checked to be the file now at
   the renamed path, since reopening and locking it again would contend with itself; a
   candidate left by another clearer gets fresh descriptors, each taken without waiting
   and checked against its path. Then every name and mailbox lock is tried without
   waiting, every session record read as in step 2, and the same for a `rooms/` recreated
   meanwhile, whose existence alone fails nothing. All free, all read, no wrapper alive:
   rename the candidate to `.rooms-1x-validated-<id>` — keeping its own id — while its
   locks are still held, sync the base, then delete it. Otherwise it stays a candidate,
   its locks released: if `rooms/` is absent, it is renamed back with a rename that never
   replaces (`renameat2` with `RENAME_NOREPLACE`), so a 1.x process holding a file in it
   writes where it expects, and a `rooms/` that appeared meanwhile keeps the candidate
   where it is. What happens to the launch depends on what validation found:
   - **Evidence that contradicts the admission revokes it**: a live wrapper, a 1.x lock
     held, a record, `stat` or directory that cannot be read — in a candidate or in a
     recreated `rooms/`. These are exactly the findings that refuse in steps 1–3, seen
     later; the launch **refuses**, exit 1, with step 1's, 2's or 3's message, and the
     tree is kept as above. An admission answers what was true when it was read; it is
     no permission to ignore live state seen after it. A recreated bare `rooms/` is no
     such evidence.
   - **A deletion that fails after a successful validation** defers only the deletion:
     the tree is already validated, step 4 of the next launch finishes it, and this
     launch goes on, its line saying what is left to delete.
7. **Count and say.** One line on stderr for what was deleted: `rewake: removed the state
   of rewake 1.x (3 rooms, 5 unread letters, 1 undelivered report).` The counts come from
   a minimal reader of 1.x mailboxes, marked like step 2's and separate from admission,
   which never decides anything: it counts each letter once, however many links its
   waiting and unread copies have, and a journal whose report is uncertain counts as
   unknown, not as absent. Before deleting, the clearer writes the counts under `v2/`,
   keyed by the tree's id, so a launch finishing a partial deletion reports the original
   counts rather than what is left. A count that cannot be established is printed as
   "unknown"; only completed removals are reported as removed, and a deferred deletion is
   said separately. One summary per invocation: a crash between the deletion and the line
   may lose the line, and nothing promises it exactly once.

Admission is this launch's own answer, read under the clear lock and kept through the
clearing unless step 6 finds evidence against it; a failed deletion never changes it. A
1.x launch waiting on a room lock across step 5 takes the old inode afterwards and writes
by path into a recreated `rooms/` — a 1.x session this launch's validation or the next
launch's admission finds alive. These checks do not fence a 1.x process between two
locks: the external quiescence remains the requirement. Nothing shuts a session down,
takes a name over, imports old mail or claims a fence.

## Every crash and every failed read

The clear lock is an `flock`, released at the holder's death, so whoever holds it next
finds only the trees and their names. Where a clearer can stop, and what follows:

| Stopped | Left behind | What the next launch does |
|---|---|---|
| during admission | nothing changed | admission again |
| after the rename of step 5, before its sync | `rooms/` or a candidate, whichever the disk kept | admission over what is there |
| after the rename of step 5 | a candidate | admits over it (steps 1–3 include candidates), then validates it in step 6 |
| during validation | a candidate | validates it again from its contents |
| after the rename to validated | a validated tree | deletes it in step 4 |
| during deletion | part of a validated tree, and its counts under `v2/` | finishes it in step 4, reporting the recorded counts |
| a read fails, a lock is held or a wrapper is alive in step 6 | a candidate, or `rooms/` again | this launch has refused; the next launch's admission refuses while the finding holds, and validation runs again once it does not |
| a rename or a sync fails | the tree as the disk kept it | this launch has refused; nothing was deleted on it |
| a deletion fails after validation | part of a validated tree | this launch went on; the next one finishes it in step 4 |

A kill proves recovery from a crash of the process; a failed sync is injected through
the fault seam, and the order of durable steps is explicit — no kill is taken to simulate
a loss of power.

## The tests

A 1.x binary cannot be built inside the 2.0 tree, so the tests drive a **stand-in of 1.x**,
a small program in the test package that does what the cited 1.x code does, its bugs
included: create a room by `MkdirAll`, take `.launch.lock` with a blocking `flock` by path
with no recheck of the inode, publish a session record with its own pid, start and
namespace, write letters by path string, take and release mailbox locks. The clearer's
pause and crash points are steps of the fault seam (`state.Step`), so a schedule stops or
kills it between two steps.

The expected result comes from the table of states and the two parts above, not from
the code: a launch goes on exactly when, at its admission and again at its validation, no
live 1.x session, no held 1.x lock and no unreadable check is found in `rooms/`, in any
candidate or in a recreated `rooms/` — a failed deletion after a passed validation does
not count against it; a tree is deleted only after a validation that ran after its
rename; nothing outside these names and `v2/` changes (the base hashed before and after).
The dimensions, crossed:

| Dimension | Values |
|---|---|
| the old state | quiet; a live wrapper registered; an unregistered launch holding `.launch.lock`; a command holding a name or a mailbox lock; a record or `/proc` data that cannot be read; a record of another namespace; after admission: a room created bare, a room created with a live wrapper, a record made unreadable, a lock taken before validation, a lock file replaced; `rooms/` recreated by path after step 5, bare and with a live wrapper; a rename or a sync that fails; a deletion that fails; a validated tree partly deleted |
| where the clearer stops | not; killed after each step and each rename of the log; paused between each two steps while the second launch runs |
| the trees at entry | `rooms/` only; a candidate only; a validated tree only; `rooms/` with a candidate; all three |
| launches | one; two at once |

Two at once: each gets its own admission — both refuse when a live session or an
unreadable record is there, both go on when it is quiet — and the second, waiting past
the clear bound behind a paused first, refuses rather than going on. The worlds covering
every pair of values run in the five checks; every world runs before S17 lands and again
at the stage's acceptance; the report shows the coverage, and a few happy paths are not
called the matrix.

The named tests: `TestLegacyAdmissionRejectsLiveBusyAndUnknown`,
`TestLegacyMissingProcessIsDeadButReadFailureUnknown`,
`TestCandidateRequiresPostRenameValidation`, `TestValidationReusesOwnedLocks`,
`TestRecreatedRoomsRevokesAdmissionOnlyOnEvidence`,
`TestCandidateRestoreNeverReplacesRooms`, `TestValidatedDeletionFailureDefersOnlyDeletion`,
`TestClearLockTimeoutRefuses`, `TestLegacyClearCrashMatrix`,
`TestLegacyClearConcurrentMatrix`, `TestLegacySummarySurvivesPartialDeletion`,
`TestLegacyClearNeverFollowsForeignPaths`.

**The live gate, before S17's last commit**, run by main, never on the owner's base and
never stopping their sessions: a real 1.x installation, two sessions in a fresh private
base under `/tmp`, one ended and one running, then the candidate 2.0 launch against that
base, with the inherited session and epoch cleared. Asserted: exit 1, the diagnostic
naming the live room and session, the old mail kept while the wrapper runs. Then only the
test session is ended, its generation quiesced, and the launch retried: the tree removed,
a truthful summary, a normal start under `v2/`, unrelated files and the worktree storage
untouched. Recorded: both artifacts' identities and versions, the commands, exit codes,
stderr, and the state before and after. A stand-in test is not this gate, and stage 7's
broader upgrade walk does not waive it.

## Still unknown

- Whether a 1.x `inbox` holding only an open file between two locks can be caught by any
  check cheaper than the person's quiescence; this pass says it cannot, and bounds its
  reach instead.
