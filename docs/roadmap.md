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

## Milestone 4. Codex: launch and delivery

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
