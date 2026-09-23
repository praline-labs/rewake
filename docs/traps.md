# Traps in rewake and its harnesses

What behaves other than expected, in rewake itself and in the harnesses it lives
with. The heading is the symptom: that is what will be searched for.

Moved here on September 22, 2026 from a handoff set deleted together with the
archives. Traps of the craft — the ones about how we work rather than about rewake —
stayed in `.shift/knowledge/traps.md`, along the line the owner drew on September 21.

Some entries have already sunk into the code and the rules; they stay here because a
rule without its reason is the first thing forgotten.

## Roles and reports

### A role's zero value switched reports off

The boolean field was chosen so that its zero value meant "stay silent". A role that
forgot to set it stopped reporting — and that looked like a working system that
merely had no reports.

For a boolean, the exception must be `true` and the zero value the ordinary
behaviour: `Silent`, not `ReportsTurns`.

*September 16, 2026.*

### Main's own cut-off arrived as a letter from itself

`You've reached your Fable limit` reached main as `claude · error` in its own
mailbox. That is not a fault: it is how rewake keeps main's error without waking it.

*September 16, 2026.*

## Tests next to a live session

### A test that inherited the session's variables declared its own session foreign

`go test` started from under rewake inherits `REWAKE_*` and reads the live state
directory as its own. Always
`env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM`, and in the
reviewer's brief too. `REWAKE_ROOM` was added on the evening of September 16.

*September 16, 2026.*

### `pgrep -x codex` finds somebody else's Codex

Take the harness process from the session record — `harnessPid` in
`<state>/rooms/<room>/sessions/<name>.json` — not from a search by name: the owner's
own Codex runs on the same machine.

*September 16, 2026.*

### A reader inside the sandbox declared live sessions dead

The Codex sandbox has its own pid namespace, and live processes are not visible from
it. Check any conclusion about another process against `/proc/self/ns/pid`.

*September 16, 2026.*

### `/proc` does not see the real pids, and that means neither an old build nor death

A managed shell has a different pid namespace: reading `/proc/<pid>/exe` gave
`ENOENT` for everything, the process's own entry included, and a quick check first
concluded that an old revision was installed. The right answer is "unknown", plus a
separate comparison of the installed file's hash and of new epochs in the registry.
Do not kill or restart on such a conclusion.

*September 19, 2026.*

## Arguments and continuing a conversation

### Under `--remote`, continuing a thread refuses because of our own flag

`rewake --name write --write codex resume` answers at once with "Permission overrides
are not supported when resuming a remote task". The Codex interface under `--remote`
forbids any change of permissions on continuation, `--add-dir` included, which
rewake gave to the write role (`tui/src/app/config_persistence.rs:49–80`). Fixed in
`7aca589`.

On `resume` and `fork`, look at the interface's argv: it must carry no `--add-dir`,
`--sandbox`, `-c sandbox_*` or `-c permissions*`.

*September 17, 2026.*

### Native remote resume rejects permission flags even from a test recipe

The same trap again, in the UI stand of September 20: `-c sandbox_mode` and
`approval_policy` passed on the line blocked `/resume`. The cure is not editing the
owner's configuration but an isolated private `HOME` carrying the same defaults — and
a check of the actual argv. A protocol client does not prove that the continuation
went through in the real TUI.

*September 20, 2026.*

### `codex app-server proxy` is not JSONL

A JSON line on its stdin gets no answer: the proxy forwards bytes to a socket that
expects WebSocket. The client has to do an HTTP Upgrade and send masked frames —
which is what our shim in `test/workflow` does.

*September 17, 2026.*

### A positional prompt eaten by a variadic

The greeting for Claude Code ended up inside the `--allowedTools` list: `<tools...>`
swallows every word that follows. In Codex the variadic is `--image`. Put `--` before
a harness's positional prompt. The greeting has since been removed, but the rule holds
for any prompt.

*September 17, 2026.*

## Delivery and its proofs

### The end of a model turn was confused with the boundary between tools

Mail piled up and main slept until the owner's message. The false trail: since the
ACK had already announced future mail, one could wait for a peek or for the end of
the turn. The real cause was a wrong statement of the task: a newly ready batch must
be sent at once, by native start-or-steer, without waiting for completion.

*September 17–20, 2026.*

### Delivery of a report to the mailbox does not prove its native announcement to the sender

The old test passed on a single HTTP request. What has to be checked is the report's
id and epoch on the sender's side, and then the reading of the report. An
availability notification or a second arbitrary request does not stand in for the
event that is needed.

*September 19, 2026.*

### The Ran row runs no command and confirms no reading

The UI event goes downstream only, after the native ACK. It is not allowed to
synthesize a turn's terminal, to pass cosmetics through the reading server, or to
repeat a letter when the UI queue is full. Under streaming the row is deferred, and
the exact order of events cannot be reconstructed from the final screen.

*September 20, 2026.*

### An idle fresh workspace turned out read-only without notice

Main woke, but `.lock` was not written: an ordinary repository is trusted, a new
temporary directory is read-only by default. That is no proof of a delivery defect.
The owner's rules are not widened automatically; an isolated stand gets an explicit
`workspace-write`, and if it needs remote resume, the defaults go into a private
configuration, not into flags.

*September 19–20, 2026.*

### A message reported delivered was held by Claude Code — the status is now honest

`rewake send` answered `delivered via socket`, and the receiving Claude Code session
showed nothing until a moment later it printed `Released 1 held cross-session message to
Claude's queue (permissions are prompting again).` The write had succeeded; a successful
write is all the adapter knows, and the receiver's inbound gate decides afterwards.

Three ways it happens. A line that arrives in the first two hundred milliseconds or so
after the socket appears is held until the interface is up, then released — with that
misleading "prompting again", since the release counts as a mode change. A receiver in
`bypassPermissions`, or in `plan` with bypass available, holds every rewake line: the line
asserts no permission-mode class, a prompt waits for the person, and unless they approve
it the message is dropped at a deadline, five minutes by default. And `crossSessionInbound` set to `hold` or `refuse` anywhere the
receiver reads it holds or refuses regardless of mode. The receiver reports each of these
only to a reply socket named in the line, and rewake named none, so nothing told the
sender.

Now every line names the wrapper's reply socket, and a held line is reported `held`
(exit 3), then delivered or failed as the receipts come; a held task that expires
sends its sender a note. A hold reported after rewake's 300 ms wait for a first word
takes the delivery back, so `delivered` from `send` is final only for work the agent has
read; a late failure sends the same note. The first notice to a new session waits for its status line,
so the startup hold no longer applies. What stays: a hold is still a hold — the
message waits for the person at the receiving session, and rewake does not release it.
The mechanism is in [delivery-adapters.md](delivery-adapters.md#claude-code-adapter). Details: [research-launch.md](research-launch.md#claude-codes-cross-session-inbound-gate),
[research.md](research.md#the-inbound-gate-on-rewakes-line).

*September 23, 2026.*

### After a new commit the running wrapper stays old

An atomic installation replaces the file, not the live process. Compare the hash of
the candidate and of the installed file, and restart by epoch, not by `--version`
alone — the version may stay `0.0.1`. The owner restarts the windows; the agent does
not revive vanished workers.

*September 19, 2026.*
