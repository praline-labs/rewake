# Design: state, the state of 1.x, one build per room

There is no migration of 1.x state: it is cleared on upgrade and sessions are restarted
(decision 3). This file says where 2.0 keeps its state, how the 1.x state goes, and the
rule that replaces the protocol cutover. The overview is in [design.md](design.md).

## The state root of 2.0

The base stays `$REWAKE_DIR`, by default `/tmp/rewake-<uid>`, with the same checks
(a directory, not a symlink, owned by the user, mode 0700) and for the same reason: a
Codex sandbox writes `/tmp` by default (`docs/design.md`, "State directory"). Inside it
2.0 keeps everything under its root **`v2/`**: `v2/rooms/<room>/...`
([stage3-state.md](stage3-state.md#the-roots) names the three levels). 1.x never reads there, and
2.0 never reads the 1.x tree, so neither build can take the other's
records for its own — the property the cutover had to prove with run records, successor
bindings and a scan of processes (`docs/protocol-cutover.md`), now held by a path.

The tree below `v2/rooms/<room>/` is the 1.x tree ([design.md](../design.md#state-directory)) less what the
migration and the Claude Code MCP injection needed:

- gone: `runs/` (run records and successors), `inbox/<name>/turns/` (earlier-build
  receipts), the conversion journal, `sock/*.mcp.json`, `sock/*.obs` and `obs.turn/`
  (the mod reports through the endpoint), `sock/*.obs.plugin/` (the mod lives in the run
  directory);
- new: `build` and `build.lock` (below), and a run directory per run,
  `run/<name>.<epoch>/`, holding what an adapter writes for one run — the mod; its
  socket keeps the short form in `sock/` — removed with the run, or, after a run killed
  outright, by a later launch once that run is proven dead (L3,
  [stage3-rundir.md](../archive/v2/stage3-rundir.md));
- unchanged: sessions, mailboxes with their journals, receipts, marks, claims and once
  records, control requests, letters, threads where an adapter uses them.

The worktrees and their records stay where they are, in the user's data directory
(`~/.local/share/rewake/worktrees`, `docs/worktree.md`), outside the base. They
hold work, so the upgrade never touches them, and 2.0 reads their records as 1.x wrote
them: `worktree` carries over unchanged (revision).

A room is, by the owner's direction of October 13, 2026, one main and its spaces and
workers by default ([answer 8](design.md#the-owners-answers)). The tree above does not
change for it: `v2/rooms/<room>/` and the room's name stay in the core, kept for
communication between two mains later, and stage 4's design only makes the default room
follow the main, its name taken from the main without a flag.

## The state of 1.x

From stage 3 to October 18, 2026 the first launch cleared the 1.x tree under the base, by the
owner's answer 1 of October 5; the owner's decision of October 18 took that out of the code. 2.0
leaves the 1.x tree as it is, and the person removes it by hand once no 1.x session runs
(`docs/install.md`). The clearing's protocol is kept in
[stage3-upgrade.md](../archive/v2/stage3-upgrade.md).

## One build per room

Replaces the protocol cutover and M2 rule 10 (one build per run). **All sessions of a
room, and every rewake process acting for them, are one build.** Two conditions hold it:
an exact identity of the build, and a boundary no writer of another build crosses.

**The identity is the artifact, not its version line.** The **build id** is the SHA-256
of the running executable's content, read once per process through `/proc/self/exe`,
which opens the image the process runs even after the file at the install path is
replaced or removed. `rewake --version` stays the human line and is not an identity: it
keeps only a `Modified` flag, its build time is optional, and it prints a short revision
and the time to the minute (`cli/version.go:21-30, 81`), so two builds of one dirty tree
print the same line. A copy of the same artifact has the same id and is admitted — by
the lease and by the endpoint's check of a peer alike; any other bytes are another build.
An image that cannot be read gives no id, and the process refuses before its first
effect, exit 1, naming why. The cost of hashing the
image once per command is measured in stage 3.

**The boundary is a lease every writer holds.** The room keeps `build`, the id of its
writers, and `build.lock`, a file locked with `flock`:

- **Every writer holds the lock shared, then checks.** A wrapper holds it for its whole
  life, through the last write of every goroutine it started, even one that ends after
  the wrapper's own stop gave up waiting for it; a CLI command that may change the room's
  state, from before its build check to its exit; a child a wrapper starts that writes — the CLI child of a tool call — takes
  its own shared lock before its first effect, so it keeps the boundary even after its
  wrapper is gone (`mail-bridge-server.md`, "Running the child"). With
  the lock held, the writer reads `build`: its own id goes on; another id, or a file it
  cannot read, refuses before the first effect, exit 1, naming the room's build.
- **Changing the build takes the lock exclusive**, without waiting. A launch whose id
  differs from `build`, or that finds no file or an unreadable one, asks for it; any
  holder — a live session, a running command, an orphaned child — makes the request
  fail. A failed request reads `build` again under the shared lock and is granted when it
  now holds the launch's own id, which another launch of the same build wrote; otherwise it
  asks again after a short random pause, for two seconds of retries against shared
  holders, each shared reacquisition bounded on its own by the writers' wait (D4a in
  [stage3-state.md](stage3-state.md#one-build-per-room)). Then the launch refuses, exit 1,
  naming the holders `/proc/locks` shows, by pid
  and a safe label, never a command line: end them, or launch into another room
  (`--room`). Held, the launch writes its id to `build` by rename, then turns its lock to shared and reads `build` again like any
  writer: `flock` does not promise the conversion is atomic.
- **No guess about liveness.** The kernel drops a `flock` with its last descriptor, so a
  crashed writer holds nothing and a live one cannot be missed. A reader of the old
  build that takes the shared lock after the change finds the new id and refuses, which
  closes the order "A checked, A's wrapper ended, B rewrote `build`, A acted" — A's
  command still held the lock, so B could not take it. An unreadable `build` is
  rewritten only under the exclusive lock, which proves no writer depends on it.

The room's launch lock (`.launch.lock`, `state/rooms.go:62`) stays for claiming names.
Read-only commands (`list`, `whoami`, `guide`, `--help`) take no lock, answer, and say
when the builds differ. This is what an npm upgrade under running sessions meets: the
new binary at the install path refuses to change the room until its sessions end,
instead of writing records the running wrappers' build does not expect. **What a
wrapper runs for itself** starts from `/proc/self/exe` or `/proc/<wrapper pid>/exe`,
never a path looked up again (`mail-bridge-server.md` rule 10 kept this for the MCP
server), so it has the wrapper's id.

Stage 3 tests: two builds of one revision from different dirty trees are refused in one
room; a copy of the same artifact is admitted; a command of the old build holding the
lock refuses a new launch, and the launch goes on once the command exits or is killed;
an orphaned child of the old build after a new launch refuses before its first effect.
Whether `flock` works inside a Codex sandbox on the state directory is stage 5's to
verify.

Still unknown: whether a person running two installs (a development build beside the
npm one) wants a build mismatch to refuse or only warn for read-only commands; the design
refuses changes and warns on views.
