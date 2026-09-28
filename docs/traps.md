# Traps in rewake and its harnesses

What behaves other than expected, in rewake itself and in the harnesses it lives
with. The heading is the symptom: that is what will be searched for.

Moved here on September 22, 2026 from a handoff set deleted together with the
archives. Traps of the craft — the ones about how we work rather than about rewake —
stayed in the owner's private notes, along the line the owner drew on September 21.

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

### An interrupted Claude Code task is reported finished with an unrelated answer

A sender got `finished` for its task with a text that had nothing to do with it —
observed at 20:03:12 and at 20:11:30 on September 23, 2026, once in the same
conversation and once across a `/clear`. The worker's turn on the task had been
interrupted with Esc, and the report was the end of the next, unrelated turn.

Claude Code runs no Stop hook for an interrupted turn
([research-claude-control.md](research-claude-control.md#interrupting-a-turn-and-changing-the-conversation)), so the
task stays owed, and obligations are kept per wrapper run, not per conversation: the
next turn end heard, whatever it was about, settles it with its own last reply. A
shell-mode `!` command was seen to start a model turn that ends in Stop without a
UserPromptSubmit, while nothing was owed; with a task owed it would settle it the same
way — inferred, not observed.

Across a `/clear` the report now carries `threadChanged`, so the sender is warned it may
not answer the task (HF-10, [delivery-adapters.md](delivery-adapters.md#claude-code-adapter)).

Mitigated, not removed, since September 23, 2026: rewake's function-hooks plugin hears
the interruption, and the sender reads `stopped` for the task at once and waits for the
person, as on Codex ([claude-plugin.md](claude-plugin.md)). The task stays owed, and the
next turn end that finishes still settles it — the owner's decision, the same on both
harnesses. The remaining risk is a person who, after Esc, turns the session to unrelated
work: that work's `finished` then settles the interrupted task. Where the plugin does
not load — `--bare`, an untrusted workspace, another harness version — the session's
telemetry reads `interruptions: unobserved` and the defect is as before.

*September 23, 2026.*

## Tests next to a live session

### A test that inherited the session's variables declared its own session foreign

`go test` started from under rewake inherits `REWAKE_*` and reads the live state
directory as its own. Always
`env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM`, and in the
reviewer's brief too. `REWAKE_ROOM` was added on the evening of September 16.

*September 16, 2026.*

### A probe with its own HOME would still write into the person's Claude Code configuration

Worker sessions carry `CLAUDE_CONFIG_DIR`, and a probe started from one inherits it.
Claude Code reads its configuration directory from that variable before HOME, so a probe
given a private HOME would still write into the person's directory. Unset
`CLAUDE_CONFIG_DIR` as well as setting HOME. Noticed before a probe ran, not after.

*September 23, 2026.*

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

### A Codex session started inside Codex's sandbox runs no command

A live check run by a Codex session launched its subject Codex sessions from its own
shell, inside its sandbox. The subject could not run a single command: `rewake guide`, a
shell wait and `rewake inbox` all failed before starting because the nested sandbox
could not open its mount registry lock, and the subject reported the work blocked. Not a
delivery failure, and no rewake defect. The rerun passed `--sandbox danger-full-access`
to the subject sessions only, for that launch and on main's decision, with the outer
sandbox left in force. A live check that starts Codex from Codex decides this before
the first turn, not after.

*September 26, 2026, Codex CLI 0.155.1.*

### A probe in a fresh HOME cloned the plugin marketplace over SSH

On its first start in an empty HOME, Claude Code 2.1.280 installs the official plugin
marketplace by itself. Its HTTPS download could not pass the probe's dead proxy — the
order the source gives, not seen in the probe — and the fallback started a real `git clone` of the marketplace repository over SSH,
which the proxy variables do not cover; it was killed within seconds. Read in the
bundled source: the install is skipped when `CLAUDE_CODE_DISABLE_OFFICIAL_MARKETPLACE_AUTOINSTALL`
is set; otherwise, after a failed CDN download, the git fallback runs `ssh -T
git@github.com` and clones over SSH when that authenticates, over HTTPS when it does not
or when `CLAUDE_CODE_PLUGIN_PREFER_HTTPS` is set. The variable was not tried live. A
probe sets it, or closes outbound SSH, before the first launch;
`CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` was set in that probe and did not stop the
clone.

*September 26, 2026, Claude Code 2.1.280.*

### `REWAKE_ROOM` in the environment put a probe's launch in `default`

A probe exported `REWAKE_ROOM=probe` and launched two sessions. The one launched without
`--room` went to room `default` of the probe's private state directory, the two sessions
ended in different rooms, and neither could see the other until both were launched with
`rewake --room probe …`. This is the contract, not a defect:
launch takes its room from `--room` alone, and commands such as `list` and `send` read
the variable ([design.md](design.md#rooms)). Put `--room` before the harness name, like
`--main`.

*September 26, 2026.*

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

### `rewake codex --add-dir` exits 1 on Codex 0.156 and later

From 0.156 the Codex terminal refuses `--add-dir` and a
`sandbox_workspace_write.writable_roots` override beside `--remote` and exits 1 before its
first request (live on 0.157.1; 0.155.1 still took both,
[research-codex.md](research-codex.md#runtime-workspace-roots)). Every `rewake codex`
launch runs the terminal under `--remote`, so a launch that asked for a second writable
directory simply ended. Rewake now refuses both itself with exit 2 and points at
`rewake send --grant-dir`, which adds the directory for one task
([grants.md](grants.md)).

*September 27, 2026.*

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

### `rewake send --notify` takes three seconds, and a report arrives late

Not a slow harness. Since September 25, 2026 a notify or a report waits in the
recipient's wrapper for other mail that asks for nothing — three seconds after the
latest, four at most from the earliest — so a burst wakes the recipient once. The send
waits with it and answers delivered; a task or a question does not wait at all. A test
that serves a mailbox by hand has no window unless it sets `Window`, which is why the
unit tests of the old behaviour still deliver notes at once
([delivery.md](delivery.md#the-notice)).

*September 25, 2026.*

### A burst of notes split in two, a waiting question's answer announced as well

The wall clock can be stepped by seconds at any moment — a time sync does it, on some
virtual machines every half a minute — and a duration taken between two processes on
the wall clock grows or shrinks by the step. The coalescing cap counted from the
writer's `createdAt`, so a step forward inside the window closed it early and a burst
went out as two notices; `TestNotesSecondsApartAreAnnouncedOnce` failed now and then
for that alone. A waiting `send --question` was judged gone by its mark's
modification time, and a telemetry snapshot read stale by its `publishedAt`. All three
compare the boot clock now (`internal/boottime`), which every process shares and nothing
steps. A new comparison between processes over seconds belongs on it too; a term of
minutes or hours stays on the wall clock, which counts a sleeping host
([delivery.md](delivery.md#the-notice)).

*September 26, 2026.*

### A task sent after `rewake compact` on Codex failed, and so did the compaction

A compaction of a long Codex conversation outlasted the 80 seconds the mark then held
deliveries for. The task sent after it went into the compaction, the server refused it
with `ActiveTurnNotSteerable { turn_kind: Compact }`, and the task was `failed` with
"not retried automatically"; main's letter said the compaction had failed, while it
finished half a minute later. Since September 26, 2026 a compaction seen running holds
deliveries up to 10 minutes, that refusal leaves the message `pending` to go after the
end, and a compaction outliving the wait is reported by its end
([remote-control-codex.md](remote-control-codex.md)). A running wrapper built before
that day still does the old thing: send the task again once `rewake list` shows the
compaction counted.

*September 26, 2026.*

### A Codex 0.157.1 terminal is never selected

The session registers and the terminal works, but `rewake send` refuses with
`delivery thread is unavailable: ... selected conversation is not ready`, after the
first launch, `/new`, `/resume` and `codex resume <id>` alike. Codex 0.157.1 sends
`runtimeWorkspaceRoots: null` in remote mode, and the gateway recognized the terminal's
start and resume by those roots. Since September 26, 2026 its configuration's
`web_search` marks them instead ([gateway.md](gateway.md#compatibility-and-limits)); a
wrapper built before that day stays unavailable on 0.157.1 — run it on 0.155.1.

*September 26, 2026.*

### After a new commit the running wrapper stays old

An atomic installation replaces the file, not the live process. `rewake --version`
names the revision and the build time of the installed file, but it is a new process:
it says nothing of the wrapper already running, so restart by epoch. A file built
without the build time passed in (a plain `go build`, see
[install.md](install.md)) shows only the commit time, and two such builds of one
revision with a modified tree print the same line; compare the hashes for those. The
owner restarts the windows; the agent does not revive vanished workers.

*September 19, 2026; the revision in `--version` since September 25, 2026.*
