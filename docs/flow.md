# The whole flow, end to end

What happens from the moment a person types `rewake claude` to the moment a
report lands back in the sender's mailbox — every process, every file and every
status on the way. The pieces are specified in [design.md](design.md),
[launch.md](launch.md) and [delivery.md](delivery.md); this page strings them
together so the process can be read in one sitting instead of being recovered
from the code.

## Cast

| who | what it is | lives |
|---|---|---|
| **wrapper** | the `rewake claude` / `rewake codex` process, the harness's parent | as long as its harness |
| **harness** | Claude Code or Codex, the ordinary program in the terminal | the session |
| **agent** | the model inside the harness, calling `rewake send`, `rewake inbox`, `rewake whoami` from its shell | a turn at a time |
| **sender** | any `rewake send` process: an agent's shell, a person's shell | until the status arrives |
| **hook** | `rewake turn-ended`, run by the harness at the end of every turn | a moment |

There is no daemon. Each wrapper services exactly one mailbox: its own
session's. Everything the processes share is files under the state directory,
`/tmp/rewake-<uid>` (`REWAKE_DIR`), 0700, checked on every open:

```
sessions/<name>.json             who is running: pids, start times, cwd, role, harness details
inbox/<name>/<id>.json           a message the wrapper still has to announce
inbox/<name>/<id>.status         pending | delivered | read | failed
inbox/<name>/unread/<id>.json    announced, not yet read by the agent
inbox/<name>/done/<id>.json      read or failed; swept after a day
inbox/<name>/awaiting/<epoch>/<peer>   who is owed a report by this run
inbox/<name>/answering/<id>      a send --question is waiting for this answer
inbox/<name>/received/<id>       that question's answer was printed
inbox/<name>/.lock               the mailbox lock, one flock for every state change
sock/<name>.<epoch>.sock         Claude Code's inbound socket for this run
```

## Act 1. A session starts

`rewake --write --name write codex` (or `rewake --main claude`, or plain `rewake claude`).

1. **The directory.** The wrapper opens `REWAKE_DIR`, creating it 0700 if it is
   missing, and refuses a symlink, a foreign owner or loose permissions.
2. **The name.** Explicit from `--name`, else the harness name, else `claude-2`,
   `claude-3`. The record is published with `link()`: a taken name stays taken
   while its session is alive; a dead record is evicted and the attempt retried.
3. **The run.** The wrapper's pid and start time make the **epoch**
   (`<pid>.<ticks>`). A name can be started many times; the epoch says which
   start this is.
4. **The role.** `--main` selects the silent role; `--write` selects a writer
   that reports like a worker. Both request Git metadata access from the sandbox
   adapter. No flag is the worker, with no extra Git grant. The record keeps the
   role id.
5. **The harness command line.** The user's arguments go through untouched.
   rewake adds, for one launch only and never into a config file:
   - Claude Code: `--messaging-socket-path sock/<name>.<epoch>.sock`,
     `--append-system-prompt <intro>`, `--allowedTools "Bash(rewake:*)"`, and
     `--settings` with a Stop hook running `rewake turn-ended` — unless the
     role is silent, or the user passed their own `--settings`;
   - Codex: `-c developer_instructions=<intro>` and
     `-c notify=["rewake","turn-ended"]` — each only when `config.toml` does not
     mention the key at all, because these keys replace rather than add. A
     silent role gets no `notify`. Main and write also append `--add-dir` for
     the repository's Git metadata. `-C`/`--cd` chooses the effective cwd; a
     `.git` pointer and `commondir` identify worktree and submodule metadata.
     Existing roots, selected profiles and the caller's `--add-dir` flags stay
     intact. If the metadata cannot be resolved, a note says which directories
     the caller must supply. Sandbox mode, tmp and network policy are unchanged.
6. **The environment.** `REWAKE_SESSION=<name>`, `REWAKE_EPOCH=<epoch>`,
   `REWAKE_DIR`; inherited Claude Code markers are stripped so a session
   started from inside another does not borrow its socket.
7. **Launch.** The harness starts with the wrapper's terminal and process
   group. Its pid and start time are added to the record. From now on the
   session is alive only while both processes are.
8. **The intro.** The agent's first context says: you are session `<name>`
   under rewake, a waiting message is announced by a line starting with
   `Rewake:`, run `rewake guide` before sending or reading. Everything else the
   agent needs is in the guide, which always matches the binary.
9. **Serving.** The wrapper watches `inbox/<name>/` with inotify, polls every
   second as the safety net, sweeps old mail every ten minutes, and waits for
   the harness to exit.

`rewake list` now shows the session: name, harness, age, cwd, and the non-default role
(`main` or `write`). It reads the records, checks that both pids are alive with
matching start times, and deletes any record whose session is dead.

## Act 2. A task is sent

`rewake send write "run the smoke and report what failed"` — from the main
session's shell, or from a person's.

1. **Who is sending.** `from` is `REWAKE_SESSION` when the run in
   `REWAKE_EPOCH` still holds that name, else `shell`. A leftover process of an
   ended run cannot sign as the new one.
2. **Who receives.** The recipient must be alive; otherwise exit 2 with the live
   names listed.
3. **The kind.** Plain `send` is a `task`; `--question` and `--notify` are the
   other two. A question to a silent recipient is refused here, before anything
   is written: that role never reports, so it could never answer.
4. **The file.** `inbox/write/<id>.json.tmp`, renamed to `.json`. The id is
   time-sortable. Nothing else is touched: the sender does not deliver.
5. **The wait.** The sender polls `<id>.status` for up to `--wait` seconds (5
   by default) and prints one line: `delivered to write via codex queue; …`,
   `pending for write: …` (exit 3), or `failed` (exit 1).

## Act 3. The wrapper announces it

The recipient's wrapper sees the rename and, under the mailbox lock:

1. **Readable first.** Hard-links the message into `unread/`. An agent told
   about mail may run `rewake inbox` at once, so the text is there before the
   notice goes out.
2. **The notice.** One line, the same for every harness:
   `Rewake: claude task, 1 new message(s)`. The count is this run's unread
   mail. The text of the message is never in it.
3. **The adapter**, outside the lock because it can take seconds:
   - Claude Code: connect to the session's socket and write one JSON line
     whose content is a `<task-notification>` block with that summary. The
     interface draws it as a single green `● Rewake: claude task, 1 new
     message(s)` line — the same line its own background tasks get — and the
     model wakes if it was idle.
   - Codex: find the current thread through `/proc` (the
     `thread-writer-locks/<uuid>.lock` the harness holds open, newest first) and
     run `codex queue --thread <uuid> --message "🟢 <notice>"`. Codex takes the
     queue on its next idle check, about ten seconds. A thread that has not
     had its first turn answers `no rollout found`: the message stays
     `pending` and is retried until it lands or its TTL (30 minutes) expires.
4. **The status.** `delivered` (the waiting copy is removed, `unread/` keeps
   the message), `pending` (retried every two seconds), or `failed` (the message
   moves to `done/`). Written atomically; the sender is reading it.

## Act 4. The agent reads

The notice wakes the agent, which runs `rewake inbox` in its shell.

1. **Whose mailbox.** `REWAKE_SESSION` names it, `REWAKE_EPOCH` proves the run:
   a stale run, or a process with a name and no epoch, is refused. Mail of an
   earlier run is not shown.
2. **Print first.** Under the lock, every unread message of this run is
   printed, oldest first, with its sender, kind and time. Reports reserved by a
   waiting `send --question` are skipped (Act 6).
3. **Then record.** Only once the output got through: for each `task` or
   `question` from a session, the sender's run is written into
   `awaiting/<own epoch>/<sender>` — this run now owes that run a report. A
   `finished` or a `notify`, or a message from a plain shell, records nothing.
   Then the `read` status, then the move to `done/`. A failure at any step
   leaves the message unread for the next `inbox`.
4. **The rule the agent follows**, from the guide: do the work, then end the
   turn with the result as the final message and stop. Do not answer with
   `rewake send`; do not answer a `notify` at all.

## Act 5. The turn ends and the report goes back

The harness itself says when a turn is over: Claude Code through the Stop hook,
Codex through `notify`. Both run `rewake turn-ended` with the last reply of the
turn in the payload.

1. **Who is owed.** Under the lock, `turn-ended` reads
   `awaiting/<own epoch>/`. A silent role has none and the hook was never
   installed; the command then does nothing.
2. **The report.** For each waiting run, a `finished` message into that
   sender's mailbox: text is the final reply, `inReplyTo` lists the messages
   read from that run since the last report, `toEpoch` is that run — not
   whoever holds the name now. The id is derived from the wait, so a retried
   hook writes the same report once.
3. **Forget the wait** only after the report is written, and only if the
   waiter still names that run.
4. **The sender is woken** by its own wrapper, through Act 3, with
   `Rewake: write finished, 1 new message(s)`. The sender reads it with
   `rewake inbox`; a `finished` asks for nothing back, so the exchange ends
   here. The main session, being silent, never reports its own turns, which is
   what keeps two sessions from waking each other forever.

## Act 6. A question, when the sender wants to block

`rewake send review "which port?" --question` is a task whose sender waits for
the answer in the same command.

1. **Reserve before publishing.** The sender creates `answering/<id>` in its
   own mailbox and keeps touching it every second while it lives. Then it
   publishes the question exactly as in Act 2.
2. **The recipient does its part** exactly as in Acts 3–5; it cannot tell a
   question from a task.
3. **The report arrives** in the sender's mailbox. Its wrapper sees a fresh
   reservation for an id in `inReplyTo`, links the report into `unread/`, and
   leaves it there without a notice and without a final status, checking the
   reservation on every tick. Ordinary `inbox` skips it.
4. **The waiting send takes it.** Under the lock: print the answer, write
   `received/<id>`, remove the mark. The report moves to `done/` only once
   every question it answers has a receipt and no fresh reservation remains.
   Output failure exits 1 without a receipt, so the answer is kept.
5. **When the sender is gone** — timeout (exit 3), kill, or crash — the mark
   goes stale within three seconds and the wrapper announces the report the
   ordinary way. Nothing is lost, only blocked for a while.

## Act 7. A notify, when nothing is owed

`rewake send write "the migration is merged" --notify` goes through Acts 2–4
and stops there: reading it records no wait, so no report comes back. It is the
right kind for a heads-up and for a probe.

## Act 8. The session ends

The harness exits, or Ctrl+C reaches the foreground group, or `kill` reaches the
wrapper (which then forwards it to a harness still alive a moment later).

1. The wrapper closes the inbox: every message still `pending` gets
   `failed: session ended`.
2. The record and the socket are removed; a later `list` will not show the
   name, and a `send` to it gets exit 2.
3. The wrapper exits with the harness's exit code.

A wrapper that dies without its harness leaves an agent that is alive and no
longer addressable; the next reader of the registry sees the dead record and
evicts it.

## One exchange, as files

`claude` (main) gives `write` a task and gets the result back.

| step | inbox/write | inbox/claude |
|---|---|---|
| send | `<id>.json`, `<id>.status: pending` | |
| wrapper announces | `unread/<id>.json`, status `delivered` | |
| write reads | `awaiting/<epoch-w>/claude`, status `read`, `done/<id>.json` | |
| write's turn ends | `awaiting/` emptied | `<rid>.json` (finished, inReplyTo: id) |
| claude's wrapper announces | | `unread/<rid>.json`, status `delivered` |
| claude reads | | status `read`, `done/<rid>.json`; no wait recorded |

Exit codes the sender sees along the way: 0 delivered, 3 accepted and still
pending, 1 failed or unreachable, 2 wrong call — unknown session, bad flag, a
question to a silent role.

## Where the flow can stall, and what it says

| situation | what happens | what the sender sees |
|---|---|---|
| fresh Codex thread, no turn yet | `codex queue` has no rollout; retried until the first turn | `pending`, exit 3; expires after the TTL |
| Codex interrupted with Ctrl+C | the queue is not taken until a person continues the thread | `delivered`, then silence |
| recipient's wrapper gone | record evicted on the next read | exit 2, no such session |
| a reader's stdout blocks | it holds the lock; server waits, `turn-ended` five seconds, `inbox` ten | delays, then "mailbox is busy" |
| the main session is asked a question | refused before publication | exit 2 with a hint |
| a process of an ended run | its epoch no longer holds the name | `inbox` and `send` refuse |
| Claude Code's socket not yet up | `ENOENT` while the harness lives | `pending`, then delivered |
