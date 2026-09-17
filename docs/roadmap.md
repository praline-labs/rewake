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

## Milestone 5. Intro and permissions — done, September 16, 2026

- The intro for both harnesses, plus the `--no-intro` flag.
- `--allowedTools "Bash(rewake:*)"` for Claude Code.
- Verify that the flag adds to the user's rules rather than replacing them; if
  it replaces them, drop the flag and have the overview name the rule to add
  once.

**Live criterion:** an agent launched through the wrapper, asked "who are you
in rewake", answers with its own name, and runs `rewake send` without
confirmation. For Codex, the same question in one cheap turn.

Met on September 16, 2026, as a side effect of the milestone 3 and 4 runs: both
agents answered through `rewake send` without being told how and without a
confirmation prompt, the Codex one from inside its sandbox.

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

## Milestone 7. Notices instead of pasted text — done, September 16, 2026

The owner's call after living with milestone 6 for an hour: a message pasted
into a session looked like something the user typed, drowned the screen in a
block of text plus two lines of rewake hints plus Claude Code's own paragraph,
and gave the agent no sense that a tool was involved.

- A harness is told that mail is waiting — `rewake: api notify, 1 new message` —
  and the agent reads it with the new `rewake inbox`. Claude Code draws the
  notice as a single `● …` line because it is wrapped in `<task-notification>`;
  Codex gets the same line as a plain message.
- Messages have a kind: `notify`, `question` (`send --question`), `finished`.
- The end of a turn is reported to whoever wrote during it, with the last reply:
  a Stop hook passed in `--settings` for Claude Code, `-c notify` for Codex. A
  direct answer replaces that report, and reading an answer or a report asks
  for nothing back.
- The intro shrank to what rewake is and "run `rewake guide`"; the instructions
  moved into the guide.

**Live criterion, met:** two Claude Code sessions on Sonnet — one asked the other
a question on a shell's instruction, both read the guide on their own, the
question and the answer each arrived as one line, and the end of the asking
session's turn came back as `● rewake: web finished`. Then Claude Code and
Codex: a task sent to Codex arrived as a plain `rewake: api notify` line, Codex
read it with `rewake inbox`, answered in its final message only, and the notify
program turned that into `● rewake: cx finished` on the Claude side with `42`
in the inbox.

The run found two things no test had:

- an agent tried to answer a message from `shell` with `rewake send shell`; the
  old message text used to say that cannot work, and nothing said it any more.
  The guide says it now;
- an answer read by the asking session put the answering one on its waiting
  list, so the answering session woke up once more only to read that its answer
  had been read. Messages to a waiting session are now marked as replies.

The review round five findings (see below) are still open.

## Milestone 9. Roles — done, September 16, 2026

The owner, after restarting under the new build: the session handing out work
got a report of its own turn back at its worker, and the two would wake each
other forever. Roles now live in a catalogue, `internal/role`, and the one that
hands out work has role `main`: it gets every report and reports
nothing. Rooms now select main automatically when none is alive; an explicit
`--general` always keeps reporting behavior. The `write` role now reports like a worker
and requests Git metadata access; main requests the same access independently
of its reporting policy.

## Milestone 8. Three kinds of message — done, September 16, 2026

The owner's call after the first real rounds with Codex: a heads-up should not
wake anybody back, a question should block until it is answered, and work is
reported by ending the turn with the result rather than by another send.

- `task` (default): the reader owes a report; the report is its final message.
- `question` (`--question`): the same, and `send` blocks until the answer and
  prints it; the answer is not announced to the asking agent a second time.
- `notify` (`--notify`): owes nothing.
- The intro and the guide say how to answer: end the turn with the result.
- Each kind is its own file in `internal/cli`, listed in one table.

In Codex the notice keeps the 🟢: Codex strips control characters from a user
message, so a coloured `●` like Claude Code's is not possible there.

Earlier findings and fixes are in the [review history](reviews.md).

## Review round eight — done, September 16, 2026

Four findings, three of them regressions of round seven's lock:

- **A reader stuck on its stdout held the mailbox**, and with it delivery, the
  end of turns and the wrapper's exit. Every wait for the lock now ends: the
  server's with its session, the hook's and the reader's after a few seconds.
- **An unusable `.lock` left senders with a pending that said nothing.** The
  server now works without a lock nobody can take; readers fail and say why.
- **A read retried after a failed last step reported twice.** The `read`
  status marks the retry, and the wait is not recorded again.
- **A waiter that could not be removed was reported at every turn.** Reports
  now carry an id derived from the wait and are written once.

Codex also showed that three tests passed with delivery held under the lock:
their reader skipped the lock. The test reader takes the real one now.

## Later, as needed


- pi, opencode, grok — their delivery paths are already covered in
  `docs/research.md`.
- A busy/idle signal for the recipient, and choosing delivery priority from it.
- A status column in `list` sourced from the Claude Code registry.
- macOS: replacements for `/proc` (`lsof`, `ps -o lstart`).
- Read receipts: `rewake inbox` already writes a `read` status; `send` does not
  report it yet.

## Risks

| risk | mitigation |
|---|---|
| Claude Code's private socket protocol changes or gets disabled remotely | delivery is isolated in the adapter; on socket failure, a clear `failed`, not silence |
| Codex's ~10-second delay confuses the sender | stated plainly in the result line; an own app-server later |
| a message to a fresh Codex thread sits `pending` | exit code 3 and text explaining it delivers after the first turn |
| the wrapper died, the harness is alive | the session is treated as dead; delivery fails with a reason instead of going silent |
| identical text in a row gets dropped by the recipient | a short id in every message's header |

## Local install, without publishing

Two ways, and they answer different questions.

**To use it.** `go build -o ~/.local/bin/rewake ./cmd/rewake` puts the real
command in PATH. Nothing else is involved, so this is the honest way to live
with the tool for a while and see what it is like.

**To test how it will be installed.** `scripts/pack.sh [version]` builds the npm
packages into `dist/npm` — one per platform plus the entry package — and
publishes nothing. Then:

```bash
cd dist/npm/rewake-linux-x64 && npm pack && npm install -g ./*.tgz
cd ../rewake && npm pack && npm install -g --omit=optional ./*.tgz
rewake --version
```

That exercises everything a registry release would except the registry itself:
the package contents, the platform split, the shim resolving its binary, and the
command landing in PATH. Uninstall with
`npm uninstall -g @iiiokojiadbi/rewake @iiiokojiadbi/rewake-linux-x64`.

The entry package's `bin` is a POSIX script rather than a Node shim: it execs the
binary and disappears, so the terminal, the signals and the exit code belong to
rewake rather than to a process in between. It follows the symlink npm installs
first — resolving the link is the difference between finding the binary and
reporting it missing.

## Milestone 6 progress, September 16, 2026

- **The mailbox is watched, not polled.** A message arrives as a rename into the
  directory and the kernel says so, so delivery no longer waits for a tick. The
  poll stays at one second as the safety net: it retries pending messages and
  covers a watch the kernel would not give.
- **Finished messages are swept by age.** Delivered and refused ones, and their
  statuses, are kept a day and then let go; a message still waiting is answered
  by the TTL rather than by the sweep.

Two things the live run caught that the tests did not:

- the watch descriptor was closed by two goroutines, and once the number was
  reused that closed somebody else's file — the test framework's own directory,
  as it happened. It is now waited on rather than blindly read, and closed once;
- after the server's tick was slowed to a second, every delivery took exactly
  that long. The server was immediate; the *sender* was polling for its answer at
  the same slow interval. Measured on a live session: 0.03 s instead of 1.0 s.


## Git metadata access by role — done, September 16, 2026

Main and the new `--write` role append `--add-dir` for the working repository's
Git metadata. Write reports turns like worker; main stays silent. Worker and
an unset role receive no extra roots. The flag adds to existing writable roots
without replacing configuration or selecting a different permission profile.

Ordinary repositories use `.git`; worktrees and submodules resolve its `gitdir`
pointer, plus the worktree's `commondir`. Missing, malformed or symlinked metadata
and remote execution are skipped with actionable advice; a newly allocated
--worktree also waits for its private metadata path to become known.

**Owner decision, September 16, 2026:** use the additive flag. A live 0.154.0
`codex exec --add-dir <gitdir> -s workspace-write` committed successfully while
retaining the owner's configured roots. The earlier sandbox-only experiment
could not verify `--add-dir` because that subcommand does not forward it.

Role, argument, configuration and metadata tests cover the launch plan. Pointer
resolution is compared with Git's output for temporary repositories, worktrees
and submodules. Repeated flags are accepted by the CLI and roots are deduplicated
by its config loader. See research for versioned evidence and reproduction.

## Milestone 10. Rooms — done, September 16, 2026

**Owner decision:** rooms isolate session discovery, addressing and delivery.
`--room <name>` is launch-only and defaults to `default`; agent commands inherit
`REWAKE_ROOM`. `REWAKE_DIR` remains the shared root; `rooms/<room>/` holds each room's
sessions, mailboxes and sockets. Old root-level records are ignored.

Role choice and name publication share a room lock. Without an explicit role,
the next launch becomes main when none is alive, otherwise general. `--general`
and `--write` can start first; `--main` refuses with the live main's name and a
hint when occupied. Main and write retain their Git metadata capability. List,
whoami, the launch note and the intro identify the room and selected role.

**Acceptance, verified:** two rooms with identical session names cannot see or
message each other; task notices and final reports stay in their originating
room. The first automatic launch becomes main. An occupied explicit main is
refused. Concurrent wrappers elect exactly one main. Checks use isolated state
and a fake harness, plus regression and mutation tests.


## Review round twelve — done, September 17, 2026

Nested-agent completions no longer settle parent tasks: agent_id filters both
success and failure before any mailbox state changes. A root agent_type still
reports normally. The regression covers both child events and the parent result.

Identified turns persist their complete report batch and waiter/message snapshot
before the first publication. Retries reuse recipients, content and report ids;
cleanup removes only that snapshot, retaining later work for its own result.

Both adapters now delimit the greeting with one -- after transport flags.
Tests check its position after variadic options, not only its presence in argv.

Bootstrap ready now persists its identified completion before removing the
greeting marker. Both a replay and a retry after a receipt write failure leave
early work owed until its real result.

Preview coverage now measures CJK terminal columns independently of the production
width estimator. A mutation treating wide glyphs as narrow fails at 102 columns.

Validation: all five repository checks, callback regressions, targeted mutations,
and isolated fake-process delivery pass. The external Commander parser confirms
that both a fresh launch and an existing -- keep the greeting positional.
Failed-turn observation still depends on the harness emitting a callback; the
previous legacy-notify limitation remains unchanged. No real harness was run.

## Milestone 10. Session-owned server transport — in progress, September 17, 2026

The transport client is local to the adapter: bounded WebSocket frames over a
private Unix socket, masked writes, ping/pong, fragmentation and correlated
JSON-RPC responses. It uses only the standard library and requests experimental
capabilities during initialization. Launch integration follows separately.

Acceptance requires same-turn delivery during work, delivery to a fresh /new
thread, stopped after a keyboard interrupt, error after an API failure, and no
persistent pending delivery to a live, ready session. Model runs are performed
separately by the owner; protocol tests use a fake local server.
