# Design: state, the upgrade from 1.x, one build per room

There is no migration of 1.x state: it is cleared on upgrade and sessions are restarted
(decision 3). This file says where 2.0 keeps its state, how the 1.x state goes, and the
rule that replaces the protocol cutover. The overview is in [design.md](design.md).

## The state root of 2.0

The root stays `$REWAKE_DIR`, by default `/tmp/rewake-<uid>`, with the same checks
(a directory, not a symlink, owned by the user, mode 0700) and for the same reason: a
Codex sandbox writes `/tmp` by default (`docs/design.md`, "State directory"). Inside it
2.0 keeps everything under **`v2/`**: `v2/rooms/<room>/...`. 1.x never reads there, and
2.0 never reads the 1.x tree except to clear it, so neither build can take the other's
records for its own — the property the cutover had to prove with run records, successor
bindings and a scan of processes (`docs/protocol-cutover.md`), now held by a path.

The tree below `v2/rooms/<room>/` is the 1.x tree (`docs/design.md:77-111`) less what the
migration and the Claude Code MCP injection needed:

- gone: `runs/` (run records and successors), `inbox/<name>/turns/` (earlier-build
  receipts), the conversion journal, `sock/*.mcp.json`, `sock/*.obs` and `obs.turn/`
  (the mod reports through the endpoint), `sock/*.obs.plugin/` (the mod lives in the run
  directory);
- new: `build` (below), and a run directory per run, `run/<name>.<epoch>/`, holding what
  an adapter writes for one run — the mod and its socket — removed with the run (L3);
- unchanged: sessions, mailboxes with their journals, receipts, marks, claims and once
  records, control requests, letters, threads where an adapter uses them.

The worktrees and their records stay where they are, in the user's data directory
(`~/.local/share/rewake/worktrees`, `docs/worktree.md`), outside the state root. They
hold work, so the upgrade never touches them, and 2.0 reads their records as 1.x wrote
them: `worktree` carries over unchanged (revision).

## Clearing the state of 1.x

**When.** At the first launch of a 2.0 session (`rewake claude ...`) that finds a 1.x
tree — `rooms/` directly under the root. Other commands do not clear: a `send` or an
`inbox` must stay quick and must not surprise; they see only `v2/`.

**What is checked first.** Whether any 1.x wrapper still runs. The launch reads only
the liveness fields of the 1.x session records — `pid` and the process start time
(`docs/session-record.md`) — and compares them with `/proc`. This one read of a 1.x
format carries the legacy mark (`legacy(rewake <date>)`, dated the day the 2.0 state
root lands, `docs/legacy.md`) and goes with it — the one mark 2.0 starts with, against
the empty table of answer 11 (recorded with answer 1).

- **A 1.x session alive**: the launch refuses, exit 1, naming each live session with its
  room, and saying that rewake 2.0 does not read 1.x mail, so those sessions have to be
  ended (and their conversations resumed under 2.0, where a harness can resume). Nothing
  is stopped or restarted automatically (`AGENTS.md`, "Delegation and review").
- **A record that cannot be read**: the launch does not clear, says once which file it
  could not read, and goes on — the 1.x tree is not in its way, since 2.0 lives in `v2/`.
- **None alive**: the launch removes the 1.x tree — `rooms/` and nothing else under the
  root — and prints one line on stderr: what was removed, with the counts of unread and
  undelivered letters dropped, so the person knows whether mail was lost.

The owner decided on October 5, 2026 that 1.x state is deleted, not set aside (answer 1
of [design.md](design.md#the-owners-answers)).

**What the person is told before.** The release notes and `docs/install.md` of 2.0 say:
end every rewake session, update, start again; unread mail of 1.x is not carried over.
The npm package's install step does nothing to the state: the first launch does.

Outside the state root rewake 1.x changed nothing of the person's (it never edits a
harness's configuration), so there is nothing else to clear.

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
print the same line. A copy of the same artifact has the same id and is admitted; any
other bytes are another build. An image that cannot be read gives no id, and the
process refuses before its first effect, exit 1, naming why. The cost of hashing the
image once per command is measured in stage 3.

**The boundary is a lease every writer holds.** The room keeps `build`, the id of its
writers, and `build.lock`, a file locked with `flock`:

- **Every writer holds the lock shared, then checks.** A wrapper holds it for its whole
  life; a CLI command that may change the room's state, from before its build check to
  its exit; a child a wrapper starts — the CLI child of a tool call, the mod's stream
  child — takes its own shared lock before its first effect, so it keeps the boundary
  even after its wrapper is gone (`mail-bridge-server.md`, "Running the child"). With
  the lock held, the writer reads `build`: its own id goes on; another id, or a file it
  cannot read, refuses before the first effect, exit 1, naming the room's build.
- **Changing the build takes the lock exclusive**, without waiting. A launch whose id
  differs from `build`, or that finds no file or an unreadable one, asks for it; any
  holder — a live session, a running command, an orphaned child — makes the request
  fail, and the launch refuses, exit 1, naming the holders `/proc/locks` shows: end
  them, or launch into another room (`--room`). Held, the launch writes its id to
  `build` by rename, then turns its lock to shared and reads `build` again like any
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
