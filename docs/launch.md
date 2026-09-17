# Session launch and signals

[Back to the design](design.md).

## Launching a harness

The common part of the wrapper:
1. Check the shared state root and selected room directory. Under the room's
   launch lock, choose the role and publish the name (the harness pid is still
   empty). The lock is released before preparing the child.
2. Start an optional session-owned backend and wait for its connection. Then
   launch the harness: `exec.Cmd` with inherited stdin/stdout/stderr, the same
   terminal and process group. Arguments after the harness name are passed
   through as-is.
3. Add the harness's pid and start time to the record.
4. Catch and drop `SIGINT`, `SIGQUIT` (the harness handles them, being in the
   same foreground group). Caught, not ignored: an ignored signal stays ignored
   across exec, and the harness would lose Ctrl+C. Forward `SIGTERM`, `SIGHUP`
   to the harness. Follow a harness that stops on its own into the stop — but
   only while it is stopped: after Ctrl+Z and `fg` the report of the stop is
   read late, and following it then stopped the wrapper a second time.
5. Service the inbox (below) until the harness exits.
6. Remove the record, close the inbox (pending messages get the status `failed:
   session ended`), exit with the harness's code.

### Room and role flags

Launch flags precede the harness name. `--room <name>` selects the room and
otherwise defaults to `default`; it does not inherit the launching process's
room. After role selection under the room lock, the address is
`<role ID>-<harness ID>`. `--name <prefix>` replaces only the role prefix;
it always receives the harness suffix, without suffix deduplication.
Automatic conflicts add -2, -3 after the harness ID; explicit conflicts refuse
with the final address. Prefix and complete address use the lower-case name
syntax, and the complete address must fit 32 characters. Choose a shorter
prefix if appending the harness or a collision number would exceed it.
Existing session records and direct messaging addresses are not rewritten.

`--main`, `--general` and `--write` explicitly select one role and cannot be
combined. Without a role flag, every launch is general, regardless of the room's
occupants. Only explicit --main creates main; it refuses when the room already
has one, naming its occupant and suggesting a launch without `--main`. General
or write sessions can start first. No session is promoted when main exits, and
subsequent unflagged launches still use general. A --name prefix never selects
main or changes reporting and permissions.

The wrapper passes the shared root as `REWAKE_DIR` and its room as `REWAKE_ROOM`,
replacing the parent's room marker alongside session and epoch. The room lock
covers role choice and name publication together, preventing concurrent mains.
List and identity commands use the inherited room and accept no `--room` flag.

### Claude Code

- Everything after the harness name is passed through untouched, with one
  exception: a `--help` written first asks rewake for the command's help page
  instead of starting the harness. `rewake claude --model x --help` still reaches
  the harness.
- Add `--messaging-socket-path <root>/rooms/<room>/sock/<name>.<epoch>.sock` unless the user passed
  their own; before launch, remove a stale socket file at the same path.
- Add `--append-system-prompt <intro>` (turned off by `--no-intro`).
- Add `--settings` with Stop and StopFailure hooks that run `rewake turn-ended` (see "The end
  of a turn"). Claude Code merges settings layers, so the hook runs next to the
  user's own. If the caller passed `--settings`, nothing is added — only one is
  read — and rewake says on stderr that turns will not be reported.
- Allow the tool's own commands without confirmation:
  `--allowedTools "Bash(rewake:*)"`. This adds a rule for the run without
  touching the user's settings. **Verify live** that the flag adds to the user's
  permissions rather than replacing them; if it replaces them, drop the flag and
  have the overview say which rule to add to settings once.

### Codex

The wrapper owns two children: a foreground app-server on a private Unix socket
and the ordinary TUI connected with --remote. The socket uses the existing
per-run digest fallback and 103-byte limit. The server has its own process group,
so keyboard interrupts reach the TUI without killing its transport. Both children
end with the session; no daemon lifecycle command is used. Server stderr goes
to the adjacent private .log file. Server death terminates the TUI and refuses
pending mail as session ended.

The adapter connects and initializes its WebSocket client before starting the
TUI. Explicit -c overrides and the generated developer_instructions are passed
to the server. A mention of developer_instructions in user configuration still
suppresses the generated value; user files are never edited. The server receives
the session's REWAKE_* environment, which the default shell policy inherits.
Its remote-control startup is disabled with the version-specific internal marker.
Rewake does not install notify; a caller's own configured program stays theirs.

On fresh launches, main and write add --add-dir Git metadata roots on the TUI. Its
thread/start runtimeWorkspaceRoots passes them to the server; synthesizing a
replacement writable_roots setting would lose the user's roots. Discovery still
validates ordinary repositories, worktrees, submodules and commondir. General
gets no extra roots. Sandbox, approval, network and tmp policies are not replaced.
The existing warning about excluded temporary directories remains relevant.

-C/--cd selects the effective server cwd as well as the TUI request. Resume and
fork retain their arguments but receive no generated permission overrides: the
remote TUI refuses these before attaching. Committing roles print
`resumed thread gets Git metadata access with each rewake task; turns you start yourself use the thread's stored roots`.
Startup sends no automatic input or permission RPC. On each delivered task or
question, committing roles read the current local workspace roots and append only
missing Git metadata directories to that turn/start request. General and reports
receive no grant. Unreadable roots leave the request unchanged with a delivery
status note. The selected policy still applies; a steered turn retains its current
permissions and the new roots affect subsequent turns. A manual TUI turn can
replace these roots again, so the next task reads them afresh.
See [continuation permission evidence](continuation-permissions.md).

Caller permission flags and permission-related -c overrides stay untouched.
For resume/fork, a separate stderr note explains that the remote TUI rejects
them and advises removing them or starting a new thread. Arguments after -- are
prompt text, not permission flags. The registry keeps
the wrapper's original cwd. Existing --remote, --profile, --worktree and --oss/--local-provider launches
are refused with advice: they cannot safely share this owned server topology or
forward all configuration. Create the checkout first and use explicit settings.
Unknown TUI arguments are preserved, not interpreted as server configuration.

Startup checks codex --version against 0.154.0 and warns, without refusing a
different version. A fake executable and socket server cover the process and
protocol contract. Real-model acceptance remains a separate owner-run check.

### The intro

The system layer uses an independent six-line briefing for general, write or
main from `internal/brief/roles.go`. Each names its identity and selection reason,
explains the commands that role needs, and points to guide for the full contract.
General receives no Git metadata grant; write may commit; main delegates and
reads results. All ask for the point of a message in its first line. Per-role
snapshots make wording changes reviewable.

### First input

Launch supplies only the system briefing; the first task prompts the agent to
read rewake guide. Caller prompts, continuations and their `--` delimiter stay
intact, with injected transport flags placed before the delimiter. A fresh
server thread accepts turn/start before any operator input.

**Owner decision, September 17, 2026:** startup must not create an empty dialogue
or add automatic messages to resumed conversations.

## Signals, and what the wrapper does not do

The wrapper does not take part in the agent's work: it does not type into its
screen, edit its configuration or read its transcript. Signals are the one place
where it has to act at all, and only because of how they arrive.

A signal from the keyboard — Ctrl+C, Ctrl+Z, Ctrl+\ — goes to the whole
foreground process group. The harness is in it, so it gets them directly and the
wrapper passes on nothing. A `kill` aimed at the wrapper's pid, from a script or
a supervisor, reaches nobody else: without the wrapper acting, the harness would
keep running with its mailbox unserved and its record gone — an agent still alive
and no longer addressable.

The two cases are indistinguishable from the signal itself: the kernel does not
say whether it went to the group or to one process. So the answer comes from the
harness. A termination request is repeated to it only if it is still running a
moment later, which after a group signal it usually is not.

**Decision, September 16, 2026:** forwarding stays. It exists to avoid leaving a
session unreachable, not to interfere. The residual case is a harness that
deliberately takes longer than the grace period to shut down — it receives a
second signal. Tools of this kind (`tini`, `dumb-init`) forward unconditionally;
this is that, with one question asked first.
