---
description: What a launch does on each harness — the wrapper, the arguments and environment it adds, the workers' profile and service folder, pi's bundled copy, starting the shell, and signals.
---

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
   behind it has an `fg`. Its own suspend is answered too: Claude Code and pi
   suspend by waiting for `SIGCONT` and sending `SIGTSTP` to their group, which
   on the shell's pane is orphaned — the wrapper leads the pty's session, its
   parent the shell outside it — so the kernel drops the stop; the wrapper
   catches that `SIGTSTP` and sends the harness the `SIGCONT` it waits for.
5. Service the inbox (below) until the harness exits.
6. Remove the record, close the inbox (pending messages get the status `failed:
   session ended`), exit with the harness's code.

### Launch defaults and the worker catalogue

A default model and reasoning effort from the environment or a settings file, and the
aliases main opens workers from, are in [launch-defaults.md](launch-defaults.md).

### Main, in the room of its directory

A person's launch is `rewake [--command
<program>] [--no-intro] <harness> [the harness's flags]` and nothing else: it always starts
**main**, `main-<harness>`, and the person never starts a worker from a terminal. There is no
role, name, room or shell flag. Workers are raised by main alone, with `rewake space open`
([space.md](space.md)).

- **The room is the launch directory**, resolved with symlinks followed. Its name on disk is
  `v2/rooms/<slug>-<hash>`: the directory's last element in lower case, every run of
  characters outside `[a-z0-9]` one dash, cut to 19 characters (`dir` when nothing is left),
  then the first 12 hex digits of the SHA-256 of the resolved path — 32 characters at most,
  readable, and apart for two checkouts named alike (`state.RoomOf`). The launch writes the
  resolved path into the room's file `directory` once, under its lease; `rewake list` shows
  the room as `<name> (<directory>)`, and `--json` as `roomDirectory`.
- **One main per directory.** A second launch in a directory whose main runs is refused,
  exit 1, before anything starts: `<directory> already has main <name>: one main per
  directory. Use that one, or start rewake in another directory, which is another room.`
  Another directory is another room.
- **A command outside any session** takes the room of its working directory; a session's
  commands take `REWAKE_ROOM`, as before. A launch reads no `REWAKE_ROOM`.
- **Workers are in main's room whatever their checkout**: the space names the room, not the
  worker's directory.
- **The room `default`** of older builds is left as it is: no launch names it, and nothing
  migrates it. A room holding only finished mail can be deleted by hand.
- **The `rewaketest` build**, which the workflow suite runs, keeps `--name`, `--room`,
  `--main`, `--general` and `--write` on the launch: the suite starts its
  workers directly, without a main to open their spaces. No flag there means main too.

What a room isolates, what each role may do and see, and how the address is formed are
specified once, in design.md and roles.md: [Rooms](design.md#rooms), [Roles](roles.md#roles) and
[Names](roles.md#names). The variables the wrapper passes to the harness are in
[Environment the harness receives](design.md#environment-the-harness-receives).

### No session inside a session

A launch — `rewake claude` — from a
shell inside a rewake session is refused with exit 2, before anything else is checked. A Claude Code worker runs a plain `rewake`
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
([space.md](space.md)). The
rule is on every launch page's notes and in main's playbook, so a main learns it before
it tries.

Tests and the workflow suite start sessions with these variables cleared, as the
[checks](../AGENTS.md#checks) already require for `go test`.

### Starting a wrapper instead of the harness

`--command <program>` starts that program where rewake would start the harness: a
person's own wrapper script, which sets up an environment — a configuration directory,
credentials — and then runs the harness. The harness is still named by its ordinary word:

```bash
rewake --command my-claude claude
```

- **Where it goes:** before the harness word — everything after that word
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
  [adapter-api.md](rules/adapter-api.md#minimum-versions)); a wrapper that does not answer it
  with a version refuses the launch, as an unreadable version always does.
- **In a worker alias** it is the field `command = "<program>"`, which becomes the
  worker launch's `--command` — in the user alias file only
  ([launch-defaults.md](launch-defaults.md#the-worker-catalogue)).

### Claude Code

- Everything after the harness name is passed through untouched, with one
  exception: a `--help` written first asks rewake for the command's help page
  instead of starting the harness. `rewake claude --model x --help` still reaches
  the harness.
- Add `--messaging-socket-path <base>/v2/rooms/<room>/sock/<name>.<epoch>.sock` unless the user passed
  their own; before launch, remove a stale socket file at the same path.
- Add one `--append-system-prompt`: rewake's briefing (turned off by `--no-intro`), which ends,
  for a worker raised for a type, with the type's text ([worker-types.md](worker-types.md)), then
  the caller's own `--append-system-prompt`,
  joined by blank lines. Claude Code keeps only the last `--append-system-prompt` (2.1.302:
  of two, the first is lost; `--append-system-prompt-file` applies beside it), so the
  caller's is taken out of the arguments and kept in rewake's rather than added beside it; of
  several, the last is kept, as Claude Code would. The whole is one argument, which Linux holds
  to 131071 bytes (one more fails exec with "argument list too long"):
  a longer prompt refuses the launch, naming each part's size and the type's, rather than be cut
  — each part can be within its own bound and the sum not, a type over a large prompt of the
  caller's.
- Allow the tool's own commands without confirmation:
  `--allowedTools "Bash(rewake:*)"`, only when the caller passed no allow list of their
  own under either spelling, `--allowedTools` or `--allowed-tools`. This adds a rule for
  the run without touching the user's settings.
- Main's status line: a launch of main with a run directory, not under `--command`, adds one
  `--settings` holding only `statusLine`, rewake's command, which runs the person's own and
  tells the host the model's name, thinking, effort and the cache's life
  ([claude.md](claude.md#the-cache-ttl)). A caller's own `--settings` reaches main's harness as
  given in its place: rewake neither reads nor merges it, and the launch notes that main's row
  then lacks those. A caller's `--setting-sources` likewise leaves main without rewake's status
  line, with the same note. A worker's of either is refused, since the worker profile owns both.
- **The workers' profile.** Every launch but main's, unless `--command` names a wrapper, sets
  `CLAUDE_CONFIG_DIR` to the workers' service folder and adds one `--settings` with what the run
  alone gets: [worker-profile.md](worker-profile.md).
- Every role launches here: the run's mod reports each turn's end once its
  hello is admitted, after the harness starts, and a task to the run fails until then
  ([claude-turns.md](claude-turns.md#readiness-and-admission)). A cause the adapter knows
  will keep the mod from loading refuses a role that owes reports before anything is
  started, exit 1, naming the cause; a silent role starts with the cause printed as a note.

### Claude Code telemetry

A Claude Code session reports its telemetry through the run's mod once its hello is admitted:
activity, the configured model, the reasoning effort the main loop asks for (the launch's
`--effort` until the mod sees one), the context used and its window, and compactions. Its row of
`rewake list` carries them; a run whose mod did not load carries none.

### pi

pi is part of rewake: a pinned pi, 1.3.0 today, is a
dependency of rewake's npm package, installed and updated with it ([install.md](install.md)).
Without `--command`, `rewake pi` starts that copy and no `pi` on `PATH`: it finds the
package in a `node_modules` above its own executable, as node resolves a dependency, or
where `REWAKE_PI` names a package directory or entry script (a build npm did not install),
and runs the entry with `node`. Before the claim it reads `node --version`, refusing below
22.19.0 (pi's own `engines`, which pi does not check) or with no `node` on `PATH`, then
the copy's version, refusing any but the pinned one: this build was checked with that one
([adapter-api.md](rules/adapter-api.md#minimum-versions)). A pi named with `--command` is the
escape hatch: it is held to the minimum, 1.2.0, and starts with a note when it is not the
pinned version. That wrapper must `exec` pi: the extension socket admits only the exact
harness pid and process start time; node runs the shipped entry in its own process.

Every pi rewake starts keeps its state in rewake's own directory: the launch sets
`PI_CODING_AGENT_DIR` to `~/.config/rewake/pi/agent`, made private (0700) by the launch,
and `PI_SKIP_VERSION_CHECK=1`, since rewake updates the copy. The version reads before the
claim get both too, through the gate's `Env`: pi 1.3.0 reads its settings before it answers
`--version`, and a setting of the person's own pi would stop the launch. They name the directory without making it; the launch makes it after the claim. Login (`/login`, kept in its
`auth.json`), settings, extensions, skills, prompts, themes, models, `pi install` packages,
trust decisions and sessions live there, apart from the person's own pi, whose `~/.pi` is
neither read nor changed. Beside the alias file, not in the state directory: it is the
person's, not a room's, and outlives a cleared temporary directory. A variable of the same
name in the person's environment does not reach the run.

A worker's sessions are kept apart from main's: every
launch but main's sets `PI_CODING_AGENT_SESSION_DIR` to `worker-sessions/` of the agent
directory, made 0700 with the folders below, over any of the person's environment; pi 1.3.0
stores and looks up sessions there, below `--session-dir` and above the `sessionDir` setting
(its `environment-variables.md`, `cli.md`; not run, since a session file needs a turn). So a
person's `--continue` or picker in a checkout a worker shared no longer lands in the worker's.
A worker's own `--session-dir` is refused, naming the folder. Main keeps the person's variable: it is not among the names a launch drops before the adapter, which replaces it for a worker alone.

The launch also makes `extensions/` and `skills/` in that directory (0700) when they are
missing, so it is visible where a person's additions go, and leaves what is in them alone
(a skills installer may already link into `skills/`). An extension is a file in `extensions/`,
or `PI_CODING_AGENT_DIR=~/.config/rewake/pi/agent pi install npm:<package>`; a skill is a
folder with a `SKILL.md` in `skills/`.

The adapter embeds `extension.ts`, its management module and its `worktree_status` tool
([worktree-tools.md](worktree-tools.md)), writing them in the run directory. It adds `-e <extension.ts>` and, unless `--no-intro`, its briefing through the
repeatable `--append-system-prompt`, the type's text at its end for a worker raised for one
([worker-types.md](worker-types.md)). That flag turns off pi's own discovery of
`APPEND_SYSTEM.md`, so the adapter adds, after the briefing, the file pi
would have read: the project's `.pi/APPEND_SYSTEM.md` in the working directory when the project
is trusted — the last of the launch's `--approve`, `-a`, `--no-approve` and `-na` decides, as pi
parses them, and with none rewake adds `--approve` — else the agent directory's; none when the caller's own
`--append-system-prompt` had turned discovery off already, as pi does. `-e` adds rewake's extension to those pi finds: the
person's, packages and built-ins load as without rewake, and so do a project's resources:
in a project with `.pi` resources or `.agents/skills` pi would otherwise ask whether to trust
it before any extension loads, rewake's included, and the run would wait for a person. The
adapter adds `--approve`, pi's trust for that one
process, which saves nothing to `trust.json`; a launch whose own arguments carry `-a`,
`--approve`, `-na` or `--no-approve` keeps its choice. The socket path is passed in `REWAKE_PI_SOCKET`; nothing is injected as a first
user message.

A new conversation named by rewake is passed as `--session-id <uuid>`. A conflicting
session, resume, continue, fork or `--no-session` option is refused. `--worktree` is
rewake's flag and starts a new conversation in its own checkout.

The extension connects from `session_start`. Its accepted hello makes Wake,
TurnBoundary and Telemetry live, plus Control when the run has its control directory and
Spaces on a main's run; a disconnected connection withdraws them. At `session_shutdown`
for a quit it tells the host `closing` first, which a main in the shell turns into the
shell's closing mark. The boundary reports only after the harness
starts, so tasks wait for that readiness. Native context, model and thinking level feed
main's telemetry. Compact accepts a focus and refuses a running turn; interrupt waits
for the turn to settle aborted and leaves its actor in native history. No limits are
reported. The exchange and its verified native behavior are in [research-pi.md](research-pi.md).

### The shell

Main runs in rewake's shell ([shell.md](shell.md)) whenever standard input and output are
both terminals and its adapter offers spaces, with no flag to turn it off — Claude Code's and
pi's alike. Otherwise main runs plain: under a
script or a pipe. The launch reads no harness flag, so
`rewake claude -p` typed at a terminal runs inside the shell too (a known limit). The launch
execs the shell — `REWAKE_SHELL`, or `rewake-shell` beside rewake, refused exit 1 when neither
is there — as `<shell> --room <room> -- <rewake> <the same arguments>` with
`REWAKE_SHELL_MAIN=1`. The shell runs that command line on a pty of its own; that launch
sees the variable, takes it out of its environment and runs main as an ordinary launch whose
adapter is told the run is drawn by rewake (`LaunchRequest.Shell`): Claude Code runs fullscreen,
`CLAUDE_CODE_NO_FLICKER=1` unless the person set the variable, so it scrolls its own history with
the wheel. The shell sets `REWAKE_HOST_COLORS` in main's environment from the terminal's
answer, so workers' hosts are seeded too. Main's wrapper writes on its terminal one thing of its
own, the shell's closing mark, when main's mod says Claude Code's session has ended, or pi's
extension says pi quits ([shell.md](shell.md#who-owns-which-pty)). It ends when main ends, with main's code, and when its
terminal goes: then it hangs main up and main's run's end ends every worker. A launch that failed
inside the shell has its last lines printed once the shell leaves the screen.

### A worktree for a launch

`rewake claude --worktree` gives main, and `rewake space open <name> … --worktree` a worker, a
checkout of its own, on a branch of its own, under rewake's worktree directory. `rewake worktree land`
fast-forwards a branch of the checkout it came from to the worktree's commits, `finish`
lands once more and removes the worktree and its branch, and `ls` and `rm` list and
remove. What the flag does for each harness, where the checkout goes, what
`.worktreeinclude` copies in and what land, finish and rm ask first is in
[worktree.md](worktree.md).

Main raises each writer in a checkout of its own with `rewake space open <name> --alias
<alias> --worktree=<topic>`; a session cannot start another by a launch
([no session inside a session](#no-session-inside-a-session)). Main then takes the accepted work with
`rewake worktree land <topic>` — or fast-forwards its own branch to the writer's — rather than
making a checkout by hand with `git worktree add`. The space's close
removes the checkout and its branch when nothing in them would be lost, every commit already on
another local branch among it ([SP7](rules/space.md)); one that stays is cleared with
`rewake worktree finish <topic>` or `rm`. A template may say `worktree = true` instead of the
typed flag, which then names the checkout, and `--no-worktree` leaves it out; the worker's
`worktree_status` tool tells it its checkout and what the close would do with it
([worktree-tools.md](worktree-tools.md)). A Claude Code template may say `enter_worktree = true`
instead: the worker starts where main works and enters one checkout of its own mid-session
through its `worktree_enter` tool, which the close releases the same way.
A writer's uncommitted edits then never mix with what is being checked or committed in
the main checkout at the same time.

### A space for a worker

Main starts a worker with `rewake space open <name> --alias <alias> … -- <harness args>`: a
detached space host runs the worker's launch in a pseudo-terminal it owns, with the session's
variables stripped and `REWAKE_SPACE` naming the space, and ends it when main's run ends. The
launch, the record, the closing and the listing are in [space.md](space.md).

### The intro

The system layer uses an independent briefing for general, write or main from
`internal/core/brief/roles.go`: the whole playbook of the role, with sections on the work, the
reports, the limits and the mail. Each names its identity and selection reason, explains the commands that role
needs, and points to guide for the full contract.
Write may commit; main delegates and reads results. All ask for the point of a message in its first line. Per-role
snapshots make wording changes reviewable. A worker raised for a type gets, after its role's
sections, `YOUR KIND OF WORK: <type>` and the type's text ([worker-types.md](worker-types.md)).

### First input

Launch supplies only the system briefing; the first task prompts the agent to
read rewake guide. Caller prompts, continuations and their `--` delimiter stay
intact, with injected transport flags placed before the delimiter.

Startup creates no empty dialogue and adds no automatic message to a resumed
conversation.

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

Forwarding stays: it exists to avoid leaving a
session unreachable, not to interfere. The residual case is a harness that
deliberately takes longer than the grace period to shut down — it receives a
second signal. Tools of this kind (`tini`, `dumb-init`) forward unconditionally;
this is that, with one question asked first.
