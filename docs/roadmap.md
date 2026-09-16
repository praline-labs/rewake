# Roadmap

Work order for `docs/design.md`. A milestone counts as closed only once its
acceptance criterion is met — not once the code is written.

## How the work is run

- One step, one commit; commit message short, subject line only, in English.
- Before every commit: `gofmt -l .` is empty, `go vet ./...` and `go test ./...`
  are green. No commit goes in with a failing check.
- Live runs happen in a separate `/tmp` directory, with their own `REWAKE_DIR`.
  The user's working harness sessions are left untouched.
- Live Codex runs spend subscription quota: cheap model, short messages, no
  more than one turn per check. Warn the owner before a run that spends quota.
- After milestones 3 and 5, a separate review agent passes over the diff:
  looking for defects and for checks that stay green on broken code. Findings get
  fixed before the next milestone.
- A milestone marked **live criterion** is accepted by running the real
  harnesses, not by tests.

## Milestone 1. Skeleton and interface — done, September 16, 2026

The CLI scaffold without delivery: command table, parsing, overview, help,
failures.

- `go.mod`, the package layout from design, `cmd/rewake`.
- `internal/cli`: the command table (name, arguments, flags, summary, examples,
  next, notes), argument parsing, the no-argument overview, per-command
  `--help`, failure formatting (reason, syntax, examples, `full help:`), "did
  you mean" by edit distance, exit codes 0/1/2/3.
- `internal/cli`: model printing — columns and `--json` from one function.
- Commands print stubs for now.

Tests: argument parsing; an unknown flag or an extra positional argument yields
exit code 2; the command table matches the handlers in both directions; the
table's examples parse without errors.

Acceptance: `rewake` with no arguments prints the overview; `rewake send` with
no arguments prints a failure with syntax and examples.

## Milestone 2. State and registry — done, September 16, 2026

- `internal/state`: the `$REWAKE_DIR` or `/tmp/rewake-<uid>` directory, owner
  and permission checks, creation with 0700, paths, atomic writes (tmp +
  rename).
- `internal/proc`: process start time from `/proc/<pid>/stat`, the process tree,
  `/proc/<pid>/fd` links. The `/proc` root is parameterized for tests.
- `internal/registry`: the session record, name publishing via `link()`,
  picking a free name, liveness by the pid-plus-start-time pair, evicting dead
  records, listing.
- `rewake list` and `rewake whoami` are for real now.

Tests: two concurrent publishes of the same name — one wins; a dead record gets
evicted, a live one doesn't; a reused pid with a different start time counts as
dead; an unsafe directory fails with exit code 2.

Acceptance: a record created by hand shows up in `list`; after killing the
process it disappears from `list`.

## Milestone 3. Claude Code: launch and delivery — done, September 16, 2026

- `internal/wrap`: launching the harness with an inherited terminal, signals
  (ignore `SIGINT`/`SIGQUIT`, forward `SIGTERM`/`SIGHUP`), stripping the parent
  session's markers, publishing and removing the record, the harness's exit
  code.
- `internal/inbox`: the message and status format, sender-side writes, the
  servicing loop with `pending` retries, TTL, moving to `done/`.
- `internal/harness/claude`: launch arguments (`--messaging-socket-path`),
  delivery as a line to the socket, the header `[rewake] from <name> · <id>`.
- `rewake send` end to end: waiting for the status, the result line, exit codes
  0/1/3.

Tests: delivery order by `id`; `pending` retries and becomes `failed` on TTL
expiry; the status is written atomically; delivery to a nonexistent socket gives
`pending`, and after the session dies, `failed`.

**Live criterion:** `rewake claude --model haiku` in one terminal, `rewake send
<name> "reply pong"` from another — a reply on screen within seconds; `list`
shows the session; harness exit removes the record and socket, and pending
messages get `failed`.

Met on September 16, 2026: delivery took 0.25 s and the agent answered within
three seconds, its reply landing in the sender's mailbox as a message of its own;
killing the terminal removed both the record and the socket. The run found two
defects that no test had:

- the agent was told to answer with `rewake send` and could not: the binary was
  not on its PATH. The wrapper now puts its own directory there;
- the first message header, `[rewake] from <name> · <id>`, was copied whole into
  the reply, which then addressed a session called `shell · 33f2`. The header now
  keeps the name on a line of its own and spells out the command to answer with.

## Review round after milestone 3 — done, September 16, 2026

An external reviewer read the three commits and reported twenty-two defects and
eighteen tests that would stay green if what they claim to check were broken.
All of it is fixed; the round is worth recording because of what it found that
neither the tests nor the live run did:

- a session name reached the file system unchecked, so `send ../../victim`
  deleted a JSON file outside the state directory while reporting no such
  session;
- `.gitignore` excluded `cmd/rewake`, so none of the three commits contained the
  program's entry point — the local build worked only because the file was there
  untracked;
- taking over the name of a session that had ended was three unsynchronised
  steps, and two wrappers could both win it and serve one mailbox;
- a mailbox belonged to a name rather than to a run of it, so mail for a session
  that had ended was handed to the next session taking that name;
- a failed launch removed the socket of a live session, and any connection error
  at all was taken as proof its owner was gone;
- signals were caught after the child was started, so a SIGTERM in that window
  killed the wrapper and left the harness without a mailbox.

Fixing these added the per-name lock, the session epoch carried by every
message, socket ownership, and tests for `internal/wrap` and the Claude Code
adapter, which had none. One fix — the epoch — was itself broken: a scripted
edit had silently not applied, and the new test caught it.

## Milestone 4. Codex: launch and delivery — done, September 16, 2026

- `internal/harness/codex`: finding the current thread from open lock files,
  chosen by mtime; delivery via `codex queue`; parsing its errors (`no rollout
  found` → `pending`, `No active session` → `failed`); `CODEX_HOME` in the
  session record.
- The Codex wrapper: the intro via `-c developer_instructions`, concatenated
  with the user's value; the state directory added to `writable_roots` under
  `exclude_slash_tmp`.

Tests: thread lookup on a fixture of a process tree and a `/proc` directory;
picking the most recent lock; classifying `codex queue` errors by their text.

**Live criterion:** `rewake codex`, then `rewake send` before the first message
— exit code 3 and `pending`; after the session's first turn, the same message
gets delivered and Codex starts a turn on its own. Separately: `codex sandbox -P
:workspace -- rewake send …` delivers to Claude Code — verifying the inbox path,
without calling the model.

Met on September 16, 2026, in full: the message sent before the first turn came
back as pending with its reason, the thread id was read from the open lock file,
and after one turn of the session the waiting message was delivered on its own,
Codex started a turn and answered through `rewake send` from inside its sandbox.

The run found a defect no test had. A Codex agent runs its commands in a sandbox
with its own pid namespace, where every process but its own is missing — so
`rewake list`, run from there, judged every session dead and deleted the record
of the session that was running it. Records now carry the namespace their pids
belong to, and a reader in a different one neither reports a session gone nor
removes anything.

## Review round two — done, September 16, 2026

The same reviewer checked the fixes by running them rather than reading them, and
twelve of the twenty-two held. What the second round found, and what was done:

- **The name lock covered only part of the work.** Claiming a free name, removing
  a record and pruning a dead one all went around it, so two claimants could
  still cross inside a takeover. Every change to a name now happens under its
  lock, and a record is removed only while it still describes the session that is
  removing it.
- **A stopped wrapper whose harness had died deleted the record of whoever held
  the name next**, and refused that session's mail on the way out. Both are now
  decided by the epoch, not by the name.
- **A failed status write re-delivered a message that had already arrived.** The
  outcome is remembered when it happens, not when it is written down; writing and
  archiving are retried, delivery is not.
- **Shutdown turned a delivered message into a failed one.** It now refuses only
  what has no outcome yet.
- **A message with no epoch at all** — from a version before epochs — is refused
  rather than handed to whoever shares the name.
- **One Ctrl+C reached the harness twice**: it shared the wrapper's process group
  and also got the forwarded copy. The harness now runs in its own group and is
  given the terminal, the way a shell runs a job, so signals arrive once.
- **The Codex configuration reader replaced settings it could not read.** It now
  tells "absent" from "present but not understood", and an override is passed
  only when what it replaces is fully known — otherwise rewake says on stderr what
  it did not do. A selected profile or a `-c` from the caller counts as unread.
- **Quoting was Go's, not TOML's**: a control character came out as `\xNN`, which
  Codex cannot parse, and it fell back to reading the argument as a raw string.
- **A symlinked `CODEX_HOME`** never matched the resolved paths `/proc` reports,
  so every message sat pending. **A nested `codex exec`** could win the thread
  lookup over the session that was addressed. **`/new` while a message was being
  queued** put it in an abandoned conversation; that is now reported as pending
  and delivered again.
- **The queue call could outlast its timeout** by two seconds, waiting on pipes a
  grandchild still held.
- **`HasFlag` searched past `--`**, so a prompt that looked like a flag disabled
  the socket flag that belongs with it.

Tests came too: the name lock is now proven between two processes rather than two
goroutines, the traversal test puts its victim where the name actually points,
and the Codex adapter is checked against configurations that are valid TOML but
outside what the reader takes apart.

## Milestone 5. Intro and permissions

- The intro for both harnesses, plus the `--no-intro` flag.
- `--allowedTools "Bash(rewake:*)"` for Claude Code.
- Verify that the flag adds to the user's rules rather than replacing them; if
  it replaces them, drop the flag and have the overview name the rule to add
  once.

**Live criterion:** an agent launched through the wrapper, asked "who are you
in rewake", answers with its own name, and runs `rewake send` without
confirmation. For Codex, the same question in one cheap turn.

## Milestone 6. Ready for daily use

- Cleaning up `done/` by age; clear failures at the edges (a session dying
  while a wait is in progress, a taken name, an unreachable directory).
- `README.md`: what it is, installation, the three commands, the trust
  boundary.
- Switching the inbox from polling to inotify, with polling as a fallback.
- Building for linux-amd64 and linux-arm64, publishing `@iiiokojiadbi/rewake`
  with platform packages; publish only on the owner's explicit word.

Acceptance: a week of use without manual intervention; not one case of a
message silently getting lost.

## Later, as needed

- pi, opencode, grok — their delivery paths are already covered in
  `docs/research.md`.
- A busy/idle signal for the recipient, and choosing delivery priority from it.
- A status column in `list` sourced from the Claude Code registry.
- macOS: replacements for `/proc` (`lsof`, `ps -o lstart`).
- Read receipts: the recipient confirms not delivery but that it read the
  message.

## Risks

| risk | mitigation |
|---|---|
| Claude Code's private socket protocol changes or gets disabled remotely | delivery is isolated in the adapter; on socket failure, a clear `failed`, not silence |
| Codex's ~10-second delay confuses the sender | stated plainly in the result line; an own app-server later |
| a message to a fresh Codex thread sits `pending` | exit code 3 and text explaining it delivers after the first turn |
| the wrapper died, the harness is alive | the session is treated as dead; delivery fails with a reason instead of going silent |
| identical text in a row gets dropped by the recipient | a short id in every message's header |
