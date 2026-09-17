# Earlier review rounds

[Back to the roadmap](roadmap.md).

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

## Review round three — done, September 16, 2026

The reviewer ran the previous round's fixes against real PTYs, separate
processes and the built CLI. Eight of the fifteen held outright; the rest had
edges left, and three of the new findings were caused by the previous fixes.

**The terminal handover was a mistake and is gone.** Giving the harness its own
process group and handing it the terminal fixed the doubled Ctrl+C and broke
everything around it: Ctrl+Z left a stopped harness holding the terminal with no
way back to the shell, `rewake claude &` took the terminal away from the shell
that started it, and with stdin from a pipe the harness could not read `/dev/tty`
at all. The harness is in the wrapper's group again, which is what makes the
terminal treat it as the program it is. The doubled signal is instead answered by
asking the harness: a termination request is repeated only if the harness is
still running a moment later, which it is not when the signal came to the group.

Also closed in this round:

- **A wrapper on its way out refused the next session's mail.** Foreign mail is
  now left where it is; a session refuses only what is addressed to it, and
  sweeps mail for the session that came before once, at startup.
- **The wrapper stopped cleaning up its own socket** — ownership was checked
  after the record had been removed, so the answer was always no.
- **`Update` went around the lock and the ownership check**, so a slow update
  could overwrite the record of whoever took the name next.
- **A reader that cannot tell which pid namespace it is in** now judges nothing
  rather than judging wrongly.
- **Valid TOML that the Codex reader still misread:** a quoted key, a key further
  down the file, a multi-line literal taken for a finished short one, and the
  escapes inside a multi-line string.
- **Flag forms that slipped past the same protection:** `--config=…`, `-ckey=…`
  and `-pwork`.
- **A relative `CODEX_HOME`** never matched the absolute paths `/proc` reports.
- **The thread search picked a nested `codex exec`** when the command was a
  launcher holding no lock of its own; it now goes outwards one generation at a
  time and stops at the first that holds a thread.
- **A thread change during delivery queued the message twice.** It is reported as
  failed with the reason instead: a repeated instruction is worse than a missing
  one, and the sender decides.
- **The queue call left a child running past its deadline**; it now runs in its
  own process group and the deadline reaches all of it.
- **Message ids were ordered by millisecond**, so two messages written in the
  same one could be delivered in the wrong order.

## Review round six — done, September 16, 2026

Codex read `a2241e1` asking what it broke. Its sandbox could not write even to
`/tmp`, so the round was read-only; every finding was reproduced here by a test
that fails on `a2241e1` before being fixed, and every fix was checked by a
mutation its test catches.

- **A read message made `send` fail.** Reading sets the status to `read`, and
  `send` treated anything but `delivered` as a failure. Read now counts as
  delivered.
- **The notice could arrive before the message was readable.** It went out
  first and the message moved to `unread/` after. The message is now linked
  into `unread/` before the notice, and the waiting copy removed after.
- **A report went to whoever held the name when the turn ended**, and a new run
  of a name inherited its predecessor's waiters. Messages carry `fromEpoch`,
  waiters are kept per run of the reader with the run of the writer, and a new
  run sweeps the old ones.
- **A process left over from an ended run could read the next run's mail.**
  The wrapper passes `REWAKE_EPOCH`; `inbox`, `send` and `turn-ended` act only
  for the run that still holds the name.
- **Reading marked messages before printing them**, so a failed write lost them.
  Printing comes first now.
- **Waiters were forgotten before the report or the answer was written.** They
  are forgotten after.
- **`turn-ended` waited forever on an open pipe** with no payload. Three seconds.
- **An escaped TOML key could spell `notify`** past the substring check. Any
  escape now keeps rewake's `notify` out.

Five tests were passing for the wrong reason — blocking `done/`, which a
delivered message no longer goes to, or measuring after the server stopped —
and now fail when the step they name is broken.

## Review round seven — done, September 16, 2026

The first round Codex ran as a rewake session, and so the first with write
access to `/tmp`: every finding came with a run. Six: one High, five Medium.
Five of them were one cause — the server, `rewake inbox`, `turn-ended` and
`send` each changing the same message or waiter with nothing ordering them —
and each round's fix had moved the race rather than removed it.

- **A read undone by the notice result.** A message linked before its notice
  could be read while `codex queue` ran; a pending result then relinked it and
  handed the task out again, a failed one reported a read message as refused.
- **Two readers showed one task**, since printing moved ahead of marking.
- **Two ends of a turn reported twice**, since waiters were no longer taken
  before reporting.
- **Clearing an ended run's wait cleared the new run's** of the same name.
- **A failed read record lost the owed report**: the message was in `done/`
  before its waiter was written.
- **A process with no `REWAKE_EPOCH` was given the current run.**

Fixed at the cause: one `flock` per mailbox for every change of state, with the
harness call left outside it; `read` as a final status; the move to `done/` as
the last step of reading; clearing a waiter only if it still names the run
reported to; and no run, no access.

The owner found a seventh by using it: a task sent to Codex in answer to its
"ready" went out marked as a reply, so Codex's report never came. The reply
mark is gone, and so is clearing a wait on a direct message — both lost reports
that were owed. One more wake per exchange is the price.

The pre-link check for a read that landed between the unlocked status check and
the lock has no test of its own: that window cannot be reached without a hook
in the server.

## Review round five — closed, September 16, 2026

The nine findings, fixed on the branch `fix/round-five`:

- Ctrl+C: the keyboard signals are caught rather than ignored, so the harness
  starts with them at their defaults. Checked in a real PTY.
- Ctrl+Z and `fg`: the wrapper follows a harness into a stop only while the
  harness is stopped. Checked in a real PTY, for Ctrl+Z and for a job stopped
  on terminal input.
- Shutdown lifts the retry limit, so an outcome known only in memory is
  written on the way out.
- The Codex configuration is no longer parsed. A key mentioned in any form is
  left alone with a note; user-defined sandbox roots are left alone.
- The socket path carries the run, so a wrapper removes its own socket and
  cannot reach the next run's.
- A closed watch channel is set aside instead of spinning, and a watch whose
  mailbox disappeared keeps asking until it comes back.
- `send` re-reads the status before calling a result lost.
- The shim finds the platform package the way Node does — nearest
  `node_modules` first, from inside the entry package upwards — instead of from
  three fixed places. It lives in `scripts/shim.sh` now, with tests over the
  nested, hoisted and linked layouts; a real `npm install` with a hoisted
  platform package runs `rewake --version` from the nested entry.

A test that staged a takeover without the name lock failed one run in many; it
takes the lock now.

## Review round five — findings, September 16, 2026

Run against `1d276de`, asked "what did these fixes break" rather than "is it
fixed". Nine defects, all reproduced by running: six regressions of round
four's fixes and three causes left in place.

- High: `signal.Ignore` is inherited through exec, so Ctrl+C no longer reaches
  a harness without handlers of its own; Ctrl+Z then `fg` stops the wrapper
  again while the harness runs; shutdown within the retry window loses a
  delivered status; the multi-line skip swallows real instructions after
  `other = """a " # b"""`; socket removal still happens outside the name lock;
  an escaped `\"""` or a string inside an array still becomes sandbox roots.
- Medium: a mailbox replaced with a gap closes the watch channel and spins a
  core; `send` reports a fresh delivery as a lost result; the shim misses a
  hoisted platform package.
- Eleven tests stay green on the broken code, one of them a false oracle
  (`TestWatchSurvivesAReplacedMailbox` takes a closed channel for an event).

## Review round four — done, September 16, 2026

Fourteen findings, four of them High, and the first was a regression from the
round before it: narrowing the signal handler to SIGTERM and SIGHUP left SIGINT
at its default, so Ctrl+C killed the wrapper while the harness carried on. An
interrupted turn ended the session's mailbox with it. The keyboard signals are
ignored explicitly now, and a live run confirms it: Ctrl+C into the session, then
`list` still shows it and the next message is delivered in 0.04 s.

Also closed:

- **Ownership was asked once and acted on later.** The record and its socket are
  now decided by a single answer: the socket goes only if the removal actually
  happened, so a name that changed hands in between no longer costs a live
  session its socket.
- **A serving goroutine that started late** — after its harness was gone and the
  name had changed hands — refused the new session's mail on its way past. It now
  asks whether it still owns the name before touching the mailbox at all.
- **A section header written inside somebody's instructions was read as
  configuration.** An example in `developer_instructions` could set the sandbox
  permissions of the run. Multi-line values are now stepped over rather than
  parsed.
- **A high file descriptor crashed the wrapper**: `select` cannot wait past 1024
  and the index was not checked. That is the case for falling back to the poll,
  not for a panic.
- **The watch answered its own writes.** With `done/` unwritable, each status
  write produced an event, which produced a pass, which wrote the status again —
  186 times in 600 ms. Only a message file counts as a change now, and rewriting
  an outcome waits its retry interval.
- **A harness that stopped itself** left the shell waiting on a wrapper that was
  still running. The wrapper follows it into the stop and back out of it.
- **A swept answer was reported as pending**, promising a delivery that had
  already happened; the sender now says the result is no longer kept.
- **The archived message kept the age it was written at**, so an old message
  refused at startup was swept in the same breath.
- Smaller: `-c=key=value`, escapes in a triple-quoted string that closes on its
  own line, a watch that never came back after the mailbox was replaced, the
  shim preferring a stale sibling package over its own dependency, and a package
  version that did not reach `rewake --version`.

## Review round nine — done, September 16, 2026

Six findings closed through one answer reservation protocol and a role check:

- **Failed stdout consumed the answer and returned success (1).** Output now
  precedes acknowledgement; write errors fail the command and preserve the reply.
- **An abandoned send suppressed notification permanently (2).** Reserved
  reports stay queued, and the server checks the lease on every tick.
- **One reader consumed a shared answer (3).** Each question records successful
  receipt; the report stays unread until every consumer has received it.
- **Ordinary inbox stole a reserved reply (4).** Reads and notice counts skip
  answers reserved by a fresh lease.
- **Silent sessions accepted questions they could not answer (5).** Refused
  before publication, with an explanation and a plain-send or notify hint.
- **Fast replies were announced before send reserved them (6).** Reservation
  and its heartbeat now start before publication and last through output.

Regression tests reproduce all six failures. Each has a failing mutation check;
answer matching also rejects a report for a different question, and the report
identity test compares the exact original id. Shared replies with abandoned
consumers and renewal during delivery are covered separately.

## Review round ten — done, September 16, 2026

- **Git pointers granted ordinary directories (1).** Gitdir must have HEAD;
  shared metadata must also have objects and refs. Invalid pointers grant nothing.
- **Room names collided with old state (2).** Rooms now live under `rooms/`;
  registry readers preserve JSON that has no session process identity.
- **Nested checkout directories missed Git metadata (3).** Discovery walks up
  to the nearest .git, validates it, and never falls back past an invalid one.
- **Released old answers expired without a notice (4).** Accepted reports
  survive their reservation, restarts and queued-mail sweeping.
- **The pointer size test passed without a bound (5).** A short valid path with
  oversized newline padding now distinguishes bounded from unbounded reading.
- **Owner decision (6): mark reports after a thread change.** Delivery context
  is recorded before readability. A known mismatch at turn end adds
  threadChanged and a visible warning. No automatic reset or resend is performed.

Each regression has a failing mutation check. Structural Git checks are compared
with real repositories, worktrees and submodules; delivery is exercised with
isolated state and fake harnesses.

## Expanded checks — done, September 17, 2026

The stricter formatter and all configured linters now pass. Unused helpers were
removed, cleanup errors are explicitly discarded only where they cannot change
the result, and successful file writes retain their checked close path. Spelling,
comments and equivalent expressions follow the configured checks. Delivery,
reporting and signal behavior are unchanged; the suite runs with race detection
and shuffled test order. Earlier review rounds are in [reviews.md](reviews.md).

## Git writes for local continuations — done, September 17, 2026

`resume` and `fork` now keep the metadata grant discovered from launch cwd.
The earlier blanket skip prevented resumed writers from committing. Remote
execution still skips local paths with an explanation.

New `--worktree` support remains deferred after source inspection and sandbox
probes: a managed checkout's private gitdir can stay read-only despite a writable
source `.git`. Both private and common metadata roots are needed, but the private
path is allocated later by the harness. Creating the worktree first remains
supported. Research records the source locations, layout-dependent results and
the wrapper's unchanged launch cwd. Regression tests and mutations protect
resume/fork grants and the honest worktree refusal.

## Review round eleven — done, September 17, 2026

Retention now distinguishes a reserved answer from an ordinary report. Release
starts one finite delivery window, never renewed by retry; receipts outlive the
reports that reference them. Mixed thread comparison checks later deliveries,
and the publication test waits for the harness pid before checking removal.

Agent system text now lives in internal/brief, with per-role
snapshots. Role data no longer carries injected prose; harness helpers are
split into plans, flags, environment, hooks and notices.

The reporting role is now general (--general). Legacy worker records normalize
to general. General, write and main have independent short system briefings
with reviewed snapshots instead of a shared paragraph plus suffixes.

Failed turns now use hook-only error reports, with fallback to the room's main
and local retention for main's own failure. Explicit reasons stay unchanged;
empty received completions after work are textless errors. The legacy notify
failure-observation gap is documented, without reading transcripts.

Notices now include a bounded first-line preview authored by the sender. The
latest available letter supplies the preview and error color; full text remains
in inbox. Empty first lines are not skipped in search of a summary.

Validation: all five repository checks pass. Twenty targeted mutations were
caught. Isolated fake-process runs covered caller arguments, both notice
transports, error routing and blocking error replies. No real harness ran.

## Review round twelve — done, September 17, 2026

Nested-agent completions no longer settle parent tasks: agent_id filters both
success and failure before any mailbox state changes. A root agent_type still
reports normally. The regression covers both child events and the parent result.

Identified turns persist their complete report batch and waiter/message snapshot
before the first publication. Retries reuse recipients, content and report ids;
cleanup removes only that snapshot, retaining later work for its own result.

Preview coverage now measures CJK terminal columns independently of the production
width estimator. A mutation treating wide glyphs as narrow fails at 102 columns.

Validation: all five repository checks, callback regressions, targeted mutations,
and isolated fake-process delivery pass. Failed-turn observation still depends on the harness emitting a callback; the
previous legacy-notify limitation remains unchanged. No real harness was run.
