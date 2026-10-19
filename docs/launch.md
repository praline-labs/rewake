# Session launch and signals

[Back to the design](design.md).

## Launching a harness

The common part of the wrapper:
1. Check the shared state root and selected room directory. Under the room's
   launch lock, choose the role and the name, only reading: a dead session record is
   removed only by the publication over it, under the name's lock. Then publish the
   session record (the harness pid is still empty). The lock is released before
   preparing the child.
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
   read late, and following it then stopped the wrapper a second time. The
   wrapper stops itself with a signal aimed at its own thread, not at the
   process, and continues the harness only once it runs again: a process-wide
   stop is taken by another thread while the caller runs on, and continued the
   harness before the wrapper had stopped. A main in the shell has a stop
   continued at once instead: its terminal is the shell's pane, and no shell
   behind it has an `fg`.
5. Service the inbox (below) until the harness exits.
6. Remove the record, close the inbox (pending messages get the status `failed:
   session ended`), exit with the harness's code.

### Launch defaults and the worker catalogue

A default model and reasoning effort from the environment or a settings file, and the
aliases main opens workers from, are in [launch-defaults.md](launch-defaults.md).

### Room and role flags

Launch flags precede the harness name. `--room <name>` chooses the room, `default`
without it; `--main`, `--general` or `--write` chooses the role, at most one of them,
general without any; `--name <prefix>` replaces the role part of the address. What a
room isolates, what each role may do and see, and how the address is formed are
specified once, in design.md and roles.md: [Rooms](design.md#rooms), [Roles](roles.md#roles) and
[Names](roles.md#names). The variables the wrapper passes to the harness are in
[Environment the harness receives](design.md#environment-the-harness-receives).

### No session inside a session

A launch — `rewake claude` — from a
shell inside a rewake session is refused with exit 2, before anything else is checked
(main's decision of September 27, 2026). A Claude Code worker runs a plain `rewake`
command unasked, and a launch among them would start an agent with none of the
worker's limits: `rewake claude --dangerously-skip-permissions -p x` from its shell.
The sign is the environment alone: `REWAKE_SESSION` or `REWAKE_EPOCH` set, even to
nothing — emptying one is as deliberate as removing it, and removing both
(`env -u REWAKE_SESSION -u REWAKE_EPOCH`) is the way a person's live run from a
session's shell goes. A live record behind it is not required — a worker can remove or rewrite its
own — so a shell left over from a session that has ended is refused too, and a new
shell is the way out. `--help` on a launch word still prints the help page. Main starts a
worker with its own command, `rewake space open`, not this path: a detached space host,
outside main's run, runs the worker's launch with the session's variables stripped
([A space for a worker](#a-space-for-a-worker)). The
rule is on every launch page's notes and in main's playbook, so a main learns it before
it tries.

Tests and the workflow suite start sessions with these variables cleared, as the
[checks](../AGENTS.md#checks) already require for `go test`.

### Starting a wrapper instead of the harness

`--command <program>` starts that program where rewake would start the harness: a
person's own wrapper script, which sets up an environment — a configuration directory,
credentials — and then runs the harness. The harness is still named by its ordinary word:

```bash
rewake --main --name lead --command my-claude claude
```

- **Where it goes:** before the harness word, like `--name` — everything after that word
  belongs to the harness. It is single-use: named twice there, the launch is refused
  rather than the last one winning. An alias's copy is replaced by one typed on the line.
- **What it replaces:** the program, and nothing else. Every argument, variable, socket
  the adapter adds is the same. Every process of the launch runs through it — the version
  read before the claim and the harness itself — because a version read beside the wrapper
  would answer for a different program than the one the launch starts.
- **A wrapper should end with `exec`.** Without it the wrapper's shell stays the process
  rewake started: termination signals reach the
  shell, not the harness.
- **What is checked:** the value, before anything is launched — a name must be found on
  `PATH`, a path with a slash is resolved against the launch directory and passed on as
  that absolute path, and either must be an executable file; otherwise the launch is
  refused naming the value and the next action. rewake does not run the program to check it, and does not guess the harness from its name:
  wrappers are named anything. The version read before the claim does run it once, with
  the arguments the adapter's version gate names (`<wrapper> --version`,
  [design-api.md](v2/design-api.md#minimum-versions)); a wrapper that does not answer it
  with a version refuses the launch, as an unreadable version always does.
- **In a worker alias** it is the field `command = "<program>"`, which becomes the
  worker launch's `--command` — in the user alias file only
  ([launch-defaults.md](launch-defaults.md#the-worker-catalogue)).
- Owner decision, September 23, 2026: the harness is named, the program is given; no
  guessing from names and no probing.

### Claude Code

- Everything after the harness name is passed through untouched, with one
  exception: a `--help` written first asks rewake for the command's help page
  instead of starting the harness. `rewake claude --model x --help` still reaches
  the harness.
- Add `--messaging-socket-path <base>/v2/rooms/<room>/sock/<name>.<epoch>.sock` unless the user passed
  their own; before launch, remove a stale socket file at the same path.
- Add `--append-system-prompt <intro>` (turned off by `--no-intro`).
- Allow the tool's own commands without confirmation:
  `--allowedTools "Bash(rewake:*)"`, only when the caller passed no allow list of their
  own under either spelling, `--allowedTools` or `--allowed-tools`. This adds a rule for
  the run without touching the user's settings. **Verify live** that the flag adds to
  the user's permissions rather than replacing them; if it replaces them, drop the flag
  and have the overview say which rule to add to settings once.
- A caller's `--settings` reaches main's harness as given: rewake neither reads nor merges
  it. A worker's is refused, since the worker profile owns that flag.
- **The worker profile.** Every launch but main's, unless `--command` names a wrapper, which
  then decides the configuration (a launch note says so), adds `--setting-sources
  project,local` and one `--settings` JSON string: `autoMemoryEnabled: false`,
  `claudeMdExcludes` naming the person's `CLAUDE.md` in `CLAUDE_CONFIG_DIR` and in
  `~/.claude`, `env` with `DISABLE_AUTOUPDATER=1` and `DISABLE_FEEDBACK_COMMAND=1`, and the
  person's `~/.config/rewake/claude/worker/settings.json` merged over them, key by key, its
  `claudeMdExcludes` added to rewake's. That directory's `plugins/` becomes `--plugin-dir`,
  `mcp.json` `--mcp-config`, and `CLAUDE.md` follows the briefing in the one
  `--append-system-prompt`. rewake reads the directory and never writes it; an entry of the
  wrong kind or a settings file that is not a JSON object refuses the launch naming the path.
  The design and the probes are in [worker-profile.md](v2/worker-profile.md).
- Every role launches here since S4-6: the run's mod reports each turn's end once its
  hello is admitted, after the harness starts, and a task to the run fails until then
  ([stage4-turns.md](v2/stage4-turns.md#readiness-and-admission)). A cause the adapter knows
  will keep the mod from loading refuses a role that owes reports before anything is
  started, exit 1, naming the cause; a silent role starts with the cause printed as a note.

### Claude Code telemetry

A Claude Code session reports no telemetry in this build, and its row of `rewake list`
carries none. The context fill and compaction come back through the mod, as parity with 1.x
(stage 5 of [the stage plan](v2/design.md#the-stage-plan-from-here)).

### pi

pi is part of rewake (owner decision, October 19, 2026): a pinned pi, 1.3.0 today, is a
dependency of rewake's npm package, installed and updated with it ([install.md](install.md)).
Without `--command`, `rewake pi` starts that copy and no `pi` on `PATH`: it finds the
package in a `node_modules` above its own executable, as node resolves a dependency, or
where `REWAKE_PI` names a package directory or entry script (a build npm did not install),
and runs the entry with `node`. Before the claim it reads `node --version`, refusing below
22.19.0 (pi's own `engines`, which pi does not check) or with no `node` on `PATH`, then
the copy's version, refusing any but the pinned one: this build was checked with that one
([design-api.md](v2/design-api.md#minimum-versions)). A pi named with `--command` is the
escape hatch: it is held to the minimum, 1.2.0, and starts with a note when it is not the
pinned version. That wrapper must `exec` pi: the extension socket admits only the exact
harness pid and process start time; node runs the shipped entry in its own process.

Every pi rewake starts keeps its state in rewake's own directory: the launch sets
`PI_CODING_AGENT_DIR` to `~/.config/rewake/pi/agent`, made private (0700) by the launch,
and `PI_SKIP_VERSION_CHECK=1`, since rewake updates the copy. The version reads before the
claim get both too, through the gate's `Env`: pi 1.3.0 reads its settings before it answers
`--version`, and a setting of the person's own pi stopped the launch (review of October 19,
2026). They name the directory without making it; the launch makes it after the claim. Login (`/login`, kept in its
`auth.json`), settings, extensions, skills, prompts, themes, models, `pi install` packages,
trust decisions and sessions live there, apart from the person's own pi, whose `~/.pi` is
neither read nor changed. Beside the alias file, not in the state directory: it is the
person's, not a room's, and outlives a cleared temporary directory. A variable of the same
name in the person's environment does not reach the run.

The adapter embeds `extension.ts` and its management module, writing both in the run
directory. It adds `-e <extension.ts>` and, unless `--no-intro`, its briefing through the
repeatable `--append-system-prompt`. `-e` adds rewake's extension to those pi finds: the
person's, packages and built-ins load as without rewake, and so do a project's resources:
in a project with `.pi` resources or `.agents/skills` pi would otherwise ask whether to trust
it before any extension loads, rewake's included, and the run would wait for a person. By the
owner's decision of October 19, 2026 the adapter adds `--approve`, pi's trust for that one
process, which saves nothing to `trust.json`; a launch whose own arguments carry `-a`,
`--approve`, `-na` or `--no-approve` keeps its choice. The socket path is passed in `REWAKE_PI_SOCKET`; nothing is injected as a first
user message.

A new conversation named by rewake is passed as `--session-id <uuid>`. A conflicting
session, resume, continue, fork or `--no-session` option is refused. `--worktree` is
rewake's flag and starts a new conversation in its own checkout.

The extension connects from `session_start`. Its accepted hello makes Wake,
TurnBoundary and Telemetry live, plus Control when the run has its control directory;
a disconnected connection withdraws them. The boundary reports only after the harness
starts, so tasks wait for that readiness. Native context, model and thinking level feed
main's telemetry. Compact accepts a focus and refuses a running turn; interrupt waits
for the turn to settle aborted and leaves its actor in native history. No limits are
reported. The exchange and its verified native behavior are in [research-pi.md](research-pi.md).

### The shell

`rewake --shell --main <harness>` runs main in rewake's shell ([shell.md](v2/shell.md)). The
launch refuses it, exit 2, for a role that opens no spaces and when standard input or output is
not a terminal, and execs the shell — `REWAKE_SHELL`,
or `rewake-shell` beside rewake — as `<shell> --room <room> -- <rewake> <the same arguments>`
with `REWAKE_SHELL_MAIN=1`. The shell runs that command line on a pty of its own; that launch
sees the variable, takes it out of its environment and runs main as an ordinary launch whose
adapter is told the run is drawn by rewake (`LaunchRequest.Shell`): Claude Code runs fullscreen,
`CLAUDE_CODE_NO_FLICKER=1` unless the person set the variable, so it scrolls its own history with
the wheel. The shell sets `REWAKE_HOST_COLORS` in main's environment from the terminal's
answer, so workers' hosts are seeded too. It ends when main ends, with main's code, and when its
terminal goes: then it hangs main up and main's run's end ends every worker. A launch that failed
inside the shell has its last lines printed once the shell leaves the screen.

### A worktree for a launch

`rewake claude --worktree` and `rewake --main claude --worktree` give a session a checkout of its
own, on a branch of its own, under rewake's worktree directory. `rewake worktree land`
fast-forwards a branch of the checkout it came from to the worktree's commits, `finish`
lands once more and removes the worktree and its branch, and `ls` and `rm` list and
remove. What the flag does for each harness, where the checkout goes, what
`.worktreeinclude` copies in and what land, finish and rm ask first is in
[worktree.md](worktree.md).

A session cannot start another ([no session inside a session](#no-session-inside-a-session)),
so the owner starts each writer, in a checkout of its own with `--worktree=<topic>`, from
a terminal outside any session. The main session then takes the accepted work with
`rewake worktree land <topic>` and closes the checkout with
`rewake worktree finish <topic>`, rather than making one by hand with `git worktree add`.
A writer's uncommitted edits then never mix with what is being checked or committed in
the main checkout at the same time.

### A space for a worker

`rewake space open <name> --harness <id> [--role write|general] [--worktree[=<name>]] -- <harness
args>` is main's way to start a worker, built October 18, 2026 (S4-7,
[the steps](v2/stage4-space-steps.md#the-build-steps)); `--role` is `write` by default. Alternatively,
`rewake workers` lists the person's worker aliases, and `space open <name> --alias <alias>`
uses the alias's harness, role, command wrapper, args and applied settings. `--harness` and
`--alias` exclude each other, and the space owns the name and room; `--role` may override the
alias's worker role. The fields and precedence are in
[launch-defaults.md](launch-defaults.md#the-worker-catalogue).
Opening is a change, refused unless main's run has `Spaces` live, and refused for a name whose session is
already alive. It writes the space's record and re-executes rewake as a launcher with
`Setsid`; the launcher starts the space host and exits. The space host takes the room's build
lease as any writer, records its own run, and refuses the open — the record closed, the worker
never started — unless its whole chain of parents reads up to the root without meeting main's
wrapper ([SP9](rules/space.md)). Then it opens a pseudo-terminal of 120 by 40 with
`host/terminal`'s pair and starts its own executable there as
`rewake --name <name> --room <main's room> --<role> [--command <wrapper>] <harness> [--worktree…] <harness args>`, in
main's working directory, with `REWAKE_SESSION`, `REWAKE_EPOCH`, `REWAKE_ROOM`, every
`REWAKE_BRIDGE_` variable and every catalogue harness's `EnvNotInherited` stripped,
`REWAKE_DIR` set to main's base and `TERM` set. Where the harness takes a conversation id
(`ConversationOffer`), a fresh one is inserted before any prompt terminator and recorded.
For pi, `--session-id` names or creates that conversation; it is not by itself a continuation
and no longer refuses a new worktree. Resume, continue, session and fork flags still do. The worker's session is
`<name>-<harness>`. The command waits until the record is open and that session is registered
with the worker's run, at most 30 s, and fails with the record's reason otherwise.

What a harness needs to start unattended — Claude Code's folder trust, a permission mode — main
passes after `--`; rewake types nothing into the worker on its own, only the person's keys from
main's shell. The space host serves `sock/space.<id>.sock` to the shell as a raw stream: a
preamble from the emulator that keeps the worker's screen, then the worker's bytes, and the keys
and sizes the shell sends back; the last size set is the worker's ([shell.md](v2/shell.md)).

The space host reads main's run once a second (`ObserveRun` through `space.MainRuns`). Once
it reads ended it sends SIGHUP to the worker's process group, SIGTERM after 1 s, SIGKILL after
5 s more, waits up to 2 s for the group to be gone, records the end it observed and exits; a
run read as unknown ends nothing. The group is gone once only zombies are left in it: an
orphan of the worker waits there for whoever adopted it to reap it, and counting it left the
record closing with nothing of the worker running (seen October 19, 2026 under the workflow
suite's subreaper, about one run in ten). A process whose leader thread is a zombie is not
one while another of its threads runs, as after `pthread_exit` of its first thread: taking it
for one closed a space after the hangup with such a worker running (seen in review the same
day). A group not seen gone, or a checkout that could not
be looked for, leaves the record closing rather than record an end nobody saw. The view socket is
listened on before the worker starts; an open that cannot serve it is refused. Its standard streams are `/dev/null`: what goes wrong shows in
the record's end or as the open's timeout.

### The intro

The system layer uses an independent six-line briefing for general, write or
main from `internal/core/brief/roles.go`. Each names its identity and selection reason,
explains the commands that role needs, and points to guide for the full contract.
Write may commit; main delegates and reads results. All ask for the point of a message in its first line. Per-role
snapshots make wording changes reviewable.

### First input

Launch supplies only the system briefing; the first task prompts the agent to
read rewake guide. Caller prompts, continuations and their `--` delimiter stay
intact, with injected transport flags placed before the delimiter.

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
