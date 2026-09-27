# A grant a worker cannot forge — September 27, 2026

The review of stage 1 ([entry](2026-09-27-grant-dir.md)) found that a worker could
grant itself a directory. Main was recognized by `REWAKE_SESSION` and `REWAKE_EPOCH`
and a registry record, and the registry, the mailboxes and the grant journal all lie in
the state directory, which a Codex worker in `workspace-write` can write: it could run
`rewake send` with main's variables, put a letter carrying a grant into its own mailbox,
or erase the journal and escape revocation. `--grant-git` had the same hole. The owner
chose on September 27, 2026 a grant the recipient cannot forge (variant B), paused
stage 2 for it, and settled the details with the sketch the same day
([grants.md](../grants.md#what-the-owner-decided)).

## What was found

- **Forgery**, high: the verified main was whoever set main's variables; the grant rode
  in a file the worker could write.
- **An empty `--grant-dir`** granted main's working directory.
- **`--to`** overtook a waiting task that carried a grant.
- **A live session's directory** inside a grant left its `.claude/`, `.mcp.json` and
  `.rewake.toml` writable.
- **A link swapped after delivery** under `/tmp`, where the worker can write the parent.
- **`--grant-git` beside a linked worktree** opens the main repository's shared `.git`.
- **`--grant-dir existing --grant-git`** on a directory already in the roots did not add
  `existing/.git` (review-codex, live on 0.155.1).
- **The journal evicted live entries** past 64 (review-codex, live on 0.157.1): 72 roots
  stayed in the conversation, and the first task's could no longer be taken back.
- Lower: a thread that could not be read failed the task outright; the 30-minute time to
  live was nowhere in the help; `ProtectedDirs` had no test over the catalogue;
  `grants/` was never cleaned; `inbox --owed` said `grant: write` after a person's turn
  dropped the grant; `/mnt/c` was undocumented as hard.

## What was done

- **Registration.** `rewake send` with a grant registers it with its own main's
  wrapper over an abstract unix socket named from the room and its run
  (`internal/grantauth`). The wrapper takes it only from its own user and from a process
  below it — a walk up the parents with start times checked, so a reused pid does not
  pass — and holds it in memory for the task's time to live and five minutes more, at
  most 256 at once, refusing the next rather than dropping one.
- **Confirmation.** The recipient's wrapper, outside the sandbox, asks the address of
  the letter's run before delivering; the answer counts only from the process
  that run names, alive, in the same mount, user and PID namespaces. A sender alive but
  not reachable keeps the task pending until its time to live; one that has ended, or
  an answer that does not match, fails it with a note to the sender.
- **A Codex main cannot grant.** Its sandbox refuses `connect()` on a unix socket; both
  `--grant-dir` and `--grant-git` are refused with the reason. `--grant-git` from a
  Codex main is a regression of this change, written in
  [git-grants.md](../git-grants.md).
- **No subreaper.** A Codex worker's commands run in namespaces of their own
  ([research-codex.md](../research-codex.md#the-sandboxs-namespaces)); a session that
  mentions `use_legacy_landlock`, which runs them beside rewake, is refused a grant.
- **The journal** of a Codex session lives in its wrapper's memory; the file under
  `grants/` is a copy for `rewake list --json`, never read back. Live entries are never
  evicted: past 64 a new grant is refused. A file of a run no longer registered is
  removed after a minute.
- **The findings.** An empty value refused with exit 2; `--to` onto a waiting granted
  task refused with a pointer to `rewake edit`; a directory equal to or holding a live
  session's working directory in the broad tier; `/tmp` and `$TMPDIR` in the hard tier;
  a shared `.git` of another repository refused, one's own checkout's written down; the
  metadata of a covered checkout added; a thread that cannot be read retried; the time to
  live in the help and main's briefing; `inbox --owed` naming a grant that ended, and
  saying that a person's turn may have dropped one it cannot see.

## Tests

Unit tests in `internal/grantauth` — registration from below and from outside, one run
confirmed and no other, an answer from another pid, start time or namespaces refused, a
wrapper not there, the cap, the lifetime, an address not taken over; in `internal/proc`
the parent walk; in `internal/wrap` every outcome of the confirmation; in
`internal/harness/codex` the legacy Landlock refusal, a grant not given twice, nested,
past the cap, a covered checkout's metadata and a shared repository; in `internal/grant`
the temporary directories, a live session's directory and the sweep; in `internal/cli`
the empty value, the addendum and the owed lines; a test over the catalogue for
`ProtectedDirs`. Hand mutations of each rule were killed.

The workflow case `codex-grant-forgery` tries five forgeries against a running main —
a send with main's variables, the same from a process detached through `setsid`, a
registration written straight to main's address, a letter written by hand and a letter
another listener confirms — and none reaches the worker's roots. Its three mutants each break what they name
([testing-cases.md](../testing-cases.md#a-directory-granted-with-a-task)).
`codex-grant-dir` now has a Claude Code main and grants a directory outside `/tmp`.

## After review

review-claude found one medium point and three low ones, fixed the same day:

- **A sandbox main started itself** registered a grant: a command run through
  `unshare -Ur --pid --fork --mount --mount-proc` below main's wrapper was taken. Main's
  wrapper now refuses a registration from other mount, user or PID namespaces, as it
  refuses a confirmation from them; a unit test registers from such a helper process.
- **The address was bound after the record was written**, so a listener there first
  took main's registrations while send reported them. The address is now named from the
  room and the run alone, bound before the name is claimed, and `rewake send` registers
  only with a listener above itself. The forgery case's foreign letter names the
  worker's own run, since main's is bound; and since the forged sends now stop at that
  client check, a fifth forgery writes a registration straight to main's address from
  outside its tree, which is what `grant-from-anywhere` breaks.
- `grants.md` no longer says a worker's process never runs below main's wrapper; it
  names a sandbox main started and a worker's code main runs, says the legacy Landlock
  refusal is the recipient's, the live-session tier a guard against main's mistake, and
  `.git/hooks` writable only with `--grant-git`.
- `internal/state/state.go` reached 400 lines; its paths and addresses moved to
  `paths.go`.

## What stays open

- A Claude Code worker has no boundary: outside a sandbox it can forge a registry
  record, start a listener and leave main's tree. Written in
  [grants.md](../grants.md#what-a-grant-does-not-stop).
- The workflow suite cannot exercise the namespace checks, its processes sharing them;
  unit tests do, the registration's in a real user namespace.
- A grant from a Codex main, a journal keyed by thread for a cold resume, and whether
  Codex resolves a root on every command are queued
  ([work-queue.md](../work-queue.md#also-queued-not-scheduled)).
- Stage 2 is paused on its branch and is to ask the worker's wrapper, not the journal.
