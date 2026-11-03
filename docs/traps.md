---
description: What misbehaves in rewake and the harnesses it lives with, by symptom — roles and reports, tests next to a live session, arguments, delivery, main's host and its shell, a switch of the room's build — each entry dated, with its cause and what to do.
---

# Traps in rewake and its harnesses

What behaves other than expected, in rewake itself and in the harnesses it lives with. The
heading is the symptom: that is what will be searched for. Some entries have already sunk into
the code and the rules; they stay here because a rule without its reason is the first thing
forgotten.

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

### A task sent from a main in print mode (`claude -p`) gets no report

A live run whose main was a print-mode Claude Code sent a worker a plain task; the worker
read it, answered on its screen and ended its turn, and nothing came back to main — no
report and no error. It looked like a turn boundary that never fired.

The main's run ends with its one turn, before the worker's turn does, and a report to an
ended run reaches nobody by rule: the worker's end skips a waiter whose run is no longer
the one holding its name (`internal/core/mail/turn_reports.go`,
[the end of a turn](delivery-turn-end.md#the-end-of-a-turn)).

From a main in print mode send a `--question` with `--wait` within the shell tool's bound,
which keeps the run alive until the answer, or run main interactively. Before suspecting the
boundary, compare when main's run ended with when the worker read the task.

*October 16, 2026.*

### An interrupted Claude Code task is reported finished with an unrelated answer

A sender got `finished` for its task with a text that had nothing to do with it, once in the
same conversation and once across a `/clear`. The worker's turn on the task had been interrupted
with Esc, and the report was the end of the next, unrelated turn: obligations are kept per
wrapper run, not per conversation, and the next turn end, whatever it was about, settled the
task with its own last reply.

The mod hears an interrupted turn: its end comes as `aborted`, the sender reads `stopped` for
the task at once, and the task stays owed — the next turn end that finishes still settles it,
the owner's decision. The remaining risk is a person who, after Esc, turns the session to
unrelated work: that work's `finished` then settles the interrupted task. Claude Code's adapter
names no delivery thread, so a report after a `/clear` carries no `threadChanged`
([delivery-turn-end.md](delivery-turn-end.md#reports-after-a-thread-change)).

*September 23, 2026; the mod's `aborted` since October 16, 2026.*

### A turn cut short to send a message was read as stopped

On Claude Code 2.1.280 the owner typed a message into a worker's running turn and sent it at
once with Ctrl+Enter. The harness ended the running turn to take the message, the plugin of the
time heard its end as `aborted`, and main got `stopped` — "the person at the keyboard stopped
this turn" — although the person had redirected the worker, and the worker went on working. A
sender reading `stopped` does not resend and waits for the person
([turn-outcomes.md](turn-outcomes.md#stops)), so the label sent it the wrong way.

Not rechecked on the mod: whether its end of such a turn is `aborted` too, and what the harness
tells the mod when an interruption carries a new message. Before taking a stop for the
person's, look at whether the worker is still working (`rewake list`).

*September 28, 2026, Claude Code 2.1.280; not rechecked since.*

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

*September 23, 2026.* Since October 23, 2026 every Claude Code worker carries it, naming
rewake's worker folder: a `claude` a worker runs — a probe, `claude plugin test` under
`go test ./internal/adapter/claude/` — reads and writes the workers' directory and uses their
login, not the person's, unless the variable is unset.

### A private HOME linked to the person's `~/.claude` wrote into their settings

A live check kept Claude Code's login by linking `~/.claude` into its private HOME.
Accepting a worker's bypass-permissions warning there appended
`skipDangerousModePermissionPrompt` to the person's own `settings.json`: a dialog's
"don't ask again" is written to the user settings whatever `--setting-sources` says.
Main removed the key. A check that needs the login asks main first; nothing of the
person's is linked where Claude Code can write.

*October 21, 2026.*

### A harness stuck in the kernel hung the suite past its timeout

`TestWorktreeEnterKeepsThePersonsPolicy` ran `claude -p` with a one-minute context and
`CombinedOutput`; it hung for good. The machine's network device was failing to come
up (`hv_netvsc … page allocation failure` in `dmesg`), so one thread of every Claude
Code start blocked in `rtnl_dumpit`, as `ip addr` did. A process with such a thread is
shown as `Zl` and is never reaped. The kill lands, but `Cmd.Wait` waits in
`Process.Wait` before `WaitDelay` ever applies. A test that runs a harness therefore
stops waiting by itself: output to a file, its own process group, and a deadline
outside `Wait` (`runBounded` in `internal/adapter/claude/mod_enter_test.go`). The
machine needs a restart; the test fails in about a minute and says so.

*October 21, 2026.*

### A harness probe without the machine's proxy ran past its minute

`TestWorktreeEnterKeepsThePersonsPolicy` turned red on `v2` and every branch alike: `claude ran
past 1m0s and was killed`, its output only the probe's `Prompt dropped by a hook`. The probe's
environment was built from nothing (HOME, CLAUDE_CONFIG_DIR, PATH), and this machine reaches the
network only through a proxy: with no proxy variables Claude Code did not exit after the dropped
prompt. With them it passes in 3 s; with them cleared from the test's environment it hangs its
minute again, every time. A stuck `ip addr` in `rtnl_dumpit` the same hour, which looked like the
trap above, was not the cause: the probe passed beside it. Every environment a test builds from
nothing carries the proxy (`internal/infra/proxyenv`), and a launch inherits it.

*October 31, 2026.*

### `/proc` does not see the real pids, and that means neither an old build nor death

A managed shell has a different pid namespace: reading `/proc/<pid>/exe` gave
`ENOENT` for everything, the process's own entry included, and a quick check first
concluded that an old revision was installed. The right answer is "unknown", plus a
separate comparison of the installed file's hash and of new epochs in the registry.
Do not kill or restart on such a conclusion.

*September 19, 2026.*

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

### `REWAKE_ROOM` in the environment put a probe's sessions in different rooms

A probe exported `REWAKE_ROOM=probe` and launched two sessions, which ended in different rooms
and could not see each other. This is the contract, not a defect: a person's launch takes the
room of its directory and reads no `REWAKE_ROOM` (only the `rewaketest` build keeps a
`--room`), while commands such as `list` and `send` outside a session read the variable or
take the room of their own working directory ([design.md](design.md#rooms)). Run a probe's
commands from main's directory, or export the room `rewake list` shows.

*September 26, 2026; rooms follow the directory since October 19, 2026.*

### An empty `.git` in `/tmp` turned the layout tests red

Six tests of `./internal` failed on an unchanged tree with `go list: …: error obtaining
VCS status: exit status 128`. They write a synthetic module into a temporary directory
and read it with `go list`, and an empty `/tmp/.git` had appeared above it: it comes and
goes while a sandboxed command of a harness runs, which masks `.git` in its writable
roots. The go command took it for a repository, asked git for the module's state, and
git refused. The module's version-control state is nothing the layout rules judge, so
their `go list` runs with `-buildvcs=false`, and a test puts an empty `.git` above a
synthetic module to keep it so.

*October 7, 2026.*

### A launch under a deep state directory exits 1 about its socket path

A Claude Code launch with `REWAKE_DIR` inside a case folder of a live check exited 1
before the harness started: `the socket path <path> is longer than the 103 bytes a unix
socket allows; use a shorter session name or REWAKE_DIR`. A unix socket's path is
bounded, and the session's messaging socket lies under the state directory, in
`v2/rooms/<room>/sock/`. A long name already falls back to a digest of name and epoch, so
the path that failed was mostly the state directory's own; a shorter session name does
not help then. Put the state directory on a short path — a directory under `/tmp`,
linked from the case folder if the case wants it there — and pass that as `REWAKE_DIR`.

*October 11, 2026.*

### A worker with worktree entry changed the branch of main's checkout without entering

A worker launched with worktree entry on started in the owner's main folder, never called
`worktree_enter`, and ran `git switch -c` there (October 21, 2026); main's later merge and push
went to that branch. Before a fast-forward merge, check the main folder with `git status -sb`:
it must be on `v2`. A brief says "before the first edit, call `worktree_enter`"; the briefing
was fixed in 8924fcb.

*October 21, 2026.*

### Main showed in herdr as `pi` during a worker's live check

While a writer ran a live check of pi in a private room, herdr showed the owner's main as `pi`:
`herdr pane list` gave the owner's pane the agent `rewake-pi`. The check's main, a `rewake pi`
started inside the worker, carried `HERDR_PANE_ID` of the owner's pane, which the worker had
inherited from main's environment; being a main, it reported itself to herdr as that pane's
agent. The role kept only the worker itself from reporting. The space host now drops every
`HERDR_*` from its environment, so a worker and whatever it starts carry none
([launch.md](launch.md#herdr)).

*November 1, 2026.*

## Arguments and continuing a conversation

### A positional prompt eaten by a variadic

The greeting for Claude Code ended up inside the `--allowedTools` list: `<tools...>`
swallows every word that follows. In Codex the variadic is `--image`. Put `--` before
a harness's positional prompt. The greeting has since been removed, but the rule holds
for any prompt.

*September 17, 2026.*

## Delivery and its proofs

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
read; a late failure sends the same note. The first notice to a new session waits — for
its status line until S9, October 11, 2026, and since the status line left for a fixed
three seconds from the start of the session's lane. That makes the startup hold rarer,
but proves nothing about readiness: nothing tells rewake the session's interface is up,
and a session slower to start can still hold the first line. It is then reported like
any hold — `held` to the sender, delivered or failed as the receipts come. What stays: a
hold is still a hold — the message waits for the person at the receiving session, and
rewake does not release it.
The mechanism is in [delivery-adapters.md](delivery-adapters.md#claude-code). Details: [research-launch.md](research-launch.md#claude-codes-cross-session-inbound-gate),
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
went out as two notices, and the window's tests failed now and then for that alone. A waiting `send --question` was judged gone by its mark's
modification time, and a telemetry snapshot read stale by its `publishedAt`. All three
compare the boot clock now (`internal/infra/boottime`), which every process shares and nothing
steps. A new comparison between processes over seconds belongs on it too; a term of
minutes or hours stays on the wall clock, which counts a sleeping host
([delivery.md](delivery.md#the-notice)).

*September 26, 2026.*

### A Claude Code session idle between tasks came back compacted

Main ended its turn after a handoff and waited for the person; when it was next woken, its
conversation had been compacted, with no `rewake compact` and no `/compact` typed. Claude Code
compacts an idle session on its own unless the settings turn it off: `"idleCompaction": false`.
A worker's launch sets it in the workers' settings (`internal/adapter/claude/settings.go`); a
main runs with the person's own settings, so a main idle between tasks is still exposed, and
its context in `rewake list` drops without anyone having asked.

When a main's context falls while it was idle, look at that setting before suspecting rewake. A
session started before the setting changed keeps the old behaviour until it is restarted.
Reported by the person and read from their settings on Claude Code 2.1.296.

*October 17, 2026.*

### After a new commit the running wrapper stays old

An atomic installation replaces the file, not the live process. `rewake --version`
names the revision and the build time of the installed file, but it is a new process:
it says nothing of the wrapper already running, so restart by epoch. A file built
without the build time passed in (a plain `go build`, see
[install.md](install.md)) shows only the commit time, and two such builds of one
revision with a modified tree print the same line; compare the hashes for those. The
owner restarts the windows; the agent does not revive vanished workers.

*September 19, 2026; the revision in `--version` since September 25, 2026.*

### A face froze on `this face speaks contract 5, the shell 4` after a change of `face/`

The face contract went from 4 to 5 (`69f31a1`) while the owner's shell, run from its sources,
spoke 4. It watches `face/src` and reloaded every face from the checkout; the new face refused
the shell, and the pi worker's pane showed only that line: nothing moved, the strip's context
went on growing, until a restart. A reload reloads the face alone: the shell's own code, its
contract among it, stays what the process loaded, as a running wrapper stays old.

A reload now starts the new face beside the running one and keeps the running one when the new
speaks another contract, saying under the input that `rewake restart shell` loads it
([face.md](face.md#hot-reload)). A pane on that line is a shell older than this, or a face started
anew, not reloaded; `rewake restart shell`, which `rewake restart` with no part names as well.

*November 1, 2026.*

### `rewake update <version>` installed and then said `no manifest beside …/.rewake-XXXX/…`

npm 11.17 replaces a global package by renaming its directory aside, to a hidden
`.rewake-<random>` beside it, and removes it after. The rewake that ran `npm install` is still
the old file, and its executable's path, read again after the install, names that removed
directory, so a plan read beside it found no manifest. The plan after an install reads the install
the command came from, taken before npm ran ([install.md](install.md#updating-from-inside-rewake)).

*November 1, 2026, in the live check of a local registry.*

### The installed shell did not draw main's output in the release gate

The 3.0.0 gate's dry run started the installed rewake on a terminal, as a person's launch does,
with a stand-in pi as main that printed a marker and ended a second later; the terminal showed the
shell's bar and hints and never the marker. The gate was right. Main's pane is fed by its host
since 85ffc35, and the host's preamble of a main on its normal screen visits a hidden alternate
screen for its margins and saved cursor (`?1047h` … `?1047l`). The shell's scanner read the leave
as main leaving its screen and showed "closing rewake…" over main for 5 s, which outlived the
stand-in; a Claude Code main had it for half a second at every launch, until it entered its
alternate screen. A switch of screens now counts once main draws on that screen
([shell.md](shell.md#the-closing-screen)).

*November 3, 2026, in the gate of the local registry; the shells from source and compiled alike.*

## Main's host and its shell

### `rewake restart shell` answered "rewake is closing", and the whole room closed

On a restart of the shell right after a landing, the command printed `the shell was not restarted:
rewake is closing`, and main, every worker and the terminal tab main ran in ended within seconds.
Neither the landing nor the new code was the cause: a checkout moved to new code under the
running shell, `bun install` and `bun run check` in its directory, a restart from the old code to
the new and a switch of the room's build all passed, alone and together. Main's host took
SIGPIPE among the signals of the terminal's end and closed as on a hangup. In Go a process that
subscribes to SIGPIPE gets it for a broken write on *any* descriptor. So when a client of
`host.sock` left with main's bytes unread, the host's next write to it closed the room. That
client was a shell that stopped reading while it gave its screen back and then exited or was
killed, or any stream or door client. A raw-stream client that stops reading while main draws and
then leaves closed a private room every time; so did a restart whose old shell was stalled. No log
of the owner's room survived, which leaves the stalled old shell's release, under the landing's
load and a Claude Code main drawing, the likely client. The host now takes SIGPIPE only so that
the write fails where it is made ([shell.md](shell.md#who-owns-which-pty)). `TestFixtureShellRestart`
has a stream client leave unread while main draws, and failed on the old code.

*November 3, 2026, in the owner's room; reproduced and checked in a private room with the real
shell and the fixture.*

## A switch of the room's build

### After `rewake restart rewake` main's Claude Code ended, and the room with it

The switch committed, then main's wrapper on the new build printed `rewake: the run handed over
could not be adopted: …/sock/<id>.sock is a live socket; another session is using this name` and
exited; its exit hung up main's terminal, and Claude Code, the shell, the room and the writer
ended. The adopted image ran the whole of a launch's plan again against a harness already
running: Claude Code's adapter refuses a messaging socket something listens on, as another
session's, and the one listening was the run's own Claude Code. Every Claude Code run, main or
worker, failed so; pi has no such check, and the live checks of the switch had run pi and the
fixture only. The adapter's plan also wrote the worker profile again, and the host ran the
version gate, which the next build's pinned pi could have refused.

An adoption now plans only what the run is served with (`Harness.Serve`, which a launch
composes and an adoption calls alone, [claude.md](claude.md#what-an-adoption-repeats)), and an adoption that fails anyway no longer
exits under its harness: it holds it, unserved, and a worker's main is told
([rules/state.md](rules/state.md#switching-a-running-room)). `TestClaudeAdopt` in the workflow
suite runs the real adapter against a stand-in listening on the socket, and failed on the old
code with the owner's line.

*November 3, 2026, in the owner's room; checked live in a private room with a Claude Code main,
a Claude Code worker and a pi worker.*
