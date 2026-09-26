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
   read late, and following it then stopped the wrapper a second time. The
   wrapper stops itself with a signal aimed at its own thread, not at the
   process, and continues the harness only once it runs again: a process-wide
   stop is taken by another thread while the caller runs on, and continued the
   harness before the wrapper had stopped.
5. Service the inbox (below) until the harness exits.
6. Remove the record, close the inbox (pending messages get the status `failed:
   session ended`), exit with the harness's code.

### Launch defaults and aliases

A default model and reasoning effort from the environment or a settings file, and
launch aliases that name a whole set of arguments, are in
[launch-defaults.md](launch-defaults.md).

### Room and role flags

Launch flags precede the harness name. `--room <name>` chooses the room, `default`
without it; `--main`, `--general` or `--write` chooses the role, at most one of them,
general without any; `--name <prefix>` replaces the role part of the address. What a
room isolates, what each role may do and see, and how the address is formed are
specified once, in design.md and roles.md: [Rooms](design.md#rooms), [Roles](roles.md#roles) and
[Names](roles.md#names). The variables the wrapper passes to the harness are in
[Environment the harness receives](design.md#environment-the-harness-receives).

### Starting a wrapper instead of the harness

`--command <program>` starts that program where rewake would start the harness: a
person's own wrapper script, which sets up an environment — a configuration directory,
credentials — and then runs the harness. The harness is still named by its ordinary word:

```bash
rewake --general --name review --command claude-worker claude
```

- **Where it goes:** before the harness word, like `--name` — everything after that word
  belongs to the harness. It is single-use: named twice there, the launch is refused
  rather than the last one winning. An alias's copy is replaced by one typed on the line.
- **What it replaces:** the program, and nothing else. Every argument, variable, socket
  and settings layer the adapter adds is the same, and the telemetry hooks and the status
  line still call rewake. For Codex both halves run through it — the version check, the
  owned app-server and the terminal — because a server started beside the wrapper would
  run in a different environment from the terminal.
- **The Codex home rewake reads** is still taken from its own environment at launch. A
  wrapper that sets `CODEX_HOME` runs both halves there, but rewake decides from its own
  view whether to pass the briefing — so with `developer_instructions` in the wrapper's
  `config.toml` and not in rewake's, the briefing passed with `-c` would replace them —
  and reads its notes and records the home from it too. So a launch with `--command` on
  Codex says on stderr which home rewake read, and to export `CODEX_HOME` for rewake as
  well when the wrapper sets it.
- **A wrapper should end with `exec`.** Without it the wrapper's shell stays the process
  rewake started: termination signals and the server's parent-death signal reach the
  shell, not the harness.
- **What is checked:** the value, before anything is launched — a name must be found on
  `PATH`, a path with a slash is resolved against the launch directory and passed on as
  that absolute path, and either must be an executable file; otherwise the launch is
  refused naming the value and the next action. The absolute path matters for Codex,
  whose server starts in the `-C` directory: a relative one would resolve there. rewake
  does not run the program to check it, and does not guess the harness from its name:
  wrappers are named anything. For Codex, `<wrapper> --version` does run at start, as the
  version check always does, through the wrapper like the other two processes; a wrapper
  that does not answer it only produces the version note.
- **In an alias** it is the field `command = "<program>"`, which becomes `--command`
  before the harness word — in the user alias file only
  ([launch-defaults.md](launch-defaults.md#naming-a-whole-launch)). With it, a launch
  through a wrapper is one word: `[alias.review]` with `harness = "claude"`, `rewake =
  ["--general", "--name", "review"]` and `command = "claude-worker"` is `rewake review`.
- Owner decision, September 23, 2026: the harness is named, the program is given; no
  guessing from names and no probing.

### Claude Code

- Everything after the harness name is passed through untouched, with one
  exception: a `--help` written first asks rewake for the command's help page
  instead of starting the harness. `rewake claude --model x --help` still reaches
  the harness.
- Add `--messaging-socket-path <root>/rooms/<room>/sock/<name>.<epoch>.sock` unless the user passed
  their own; before launch, remove a stale socket file at the same path.
- Add `--append-system-prompt <intro>` (turned off by `--no-intro`).
- Add one `--settings` layer carrying the Stop and StopFailure hooks that run
  `rewake turn-ended` (see "The end of a turn"), the telemetry hooks and the status-line
  tap ([claude-telemetry.md](claude-telemetry.md)). Claude Code merges settings layers, so the
  hooks run next to the user's own. It reads only one `--settings` and takes the last,
  so a caller's own is merged into rather than replaced or skipped: their keys stay,
  their hooks for an event come first and ours are appended, their status line keeps
  every field but the command. A caller's layer that cannot be read or merged is left
  as given, alone, and rewake says on stderr what is not happening. Since September 23,
  2026 a caller's `--settings` file is read at launch and passed to the harness inline,
  as the merged JSON, instead of as its path (`internal/harness/claude/settings.go`). Two
  consequences: everything in that file, environment values included, is visible in the
  harness's `/proc/<pid>/cmdline`; and edits to the file during the session are not
  picked up.
- Allow the tool's own commands without confirmation:
  `--allowedTools "Bash(rewake:*)"`. This adds a rule for the run without
  touching the user's settings. **Verify live** that the flag adds to the user's
  permissions rather than replacing them; if it replaces them, drop the flag and
  have the overview say which rule to add to settings once.

- Pass rewake's function-hooks plugin: `--plugin-dir sock/<name>.<epoch>.obs.plugin`,
  a directory the wrapper writes for the run, and `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1`
  in the harness's environment. Left out under `--bare` and when the person set that
  variable off, with a launch note ([claude-plugin.md](claude-plugin.md)).

### Claude Code telemetry

The telemetry hooks, the status-line tap and the collector they report to are specified
in [claude-telemetry.md](claude-telemetry.md); the plugin that reports to the same
collector in [claude-plugin.md](claude-plugin.md).

### Codex

The wrapper owns two children: a foreground app-server on a private Unix socket
and the ordinary TUI connected with --remote through its inline gateway. The
original socket belongs to the gateway; the native endpoint adds `.up`. The socket uses the existing
per-run digest fallback and 103-byte limit. The server has its own process group,
so keyboard interrupts reach the TUI without killing its transport. Both children
end with the session; no daemon lifecycle command is used. Server stderr goes
to the adjacent private `.up.log` file. Server death terminates the TUI and refuses
pending mail as session ended.

The adjacent `.gateway.log` records bounded, payload-free connection-close reasons;
see [startup transport diagnostics and limits](startup-transport.md).

Once the app-server is up the wrapper also serves the run's control directory, polling
it every 100 ms and carrying a main's `rewake compact` or `rewake interrupt` out through
the gateway ([remote-control-codex.md](remote-control-codex.md)).

The adapter initializes and closes a startup probe before starting the TUI; it
never discovers or resumes a root. The gateway forwards the TUI connection. Explicit -c overrides and the generated developer_instructions are passed
to the server. A mention of developer_instructions in user configuration still
suppresses the generated value; user files are never edited. The server receives
the session's REWAKE_* environment, which the default shell policy inherits.
Its remote-control startup is disabled with the version-specific internal marker.
Rewake does not install notify; a caller's own configured program stays theirs.

No role automatically adds Git metadata roots on fresh launch, resume or fork.
Owner-supplied roots and sandbox, approval, network and temporary-directory policies
are preserved. -C/--cd still selects the effective server cwd and TUI request.
Continuations retain caller input without generated permission overrides; unsupported
caller permission overrides keep the existing native-compatibility diagnostic.

Only a verified main's [explicit --grant-git task/question](git-grants.md) requests
additional metadata roots. It reads current local roots without history and appends
only missing validated metadata for an eligible recipient. A steered active turn
keeps its existing permission context; new roots may affect only subsequent turns.
Manual input can replace roots, so a later explicit request checks them afresh.
See [continuation permission evidence](continuation-permissions.md).

Caller permission flags and permission-related -c overrides stay untouched.
For resume/fork, a separate stderr note explains that the remote TUI rejects
them and advises removing them or starting a new thread. Arguments after -- are
prompt text, not permission flags. The registry keeps
the wrapper's cwd — the launch directory, or the checkout of a `--worktree` launch below.
Existing --remote, --profile and --oss/--local-provider launches
are refused with advice: they cannot safely share this owned server topology or
forward all configuration. Use explicit settings.

#### A worktree for a launch

`rewake codex --worktree` gives the session a checkout of its own. Codex's terminal
cannot make one under rewake — it refuses `--worktree` together with `--remote`, before
it creates anything, and rewake always starts it with `--remote`
([research-codex.md](research-codex.md#--worktree-with-a-remote-terminal)) — so the
launch command takes the flag for itself and never passes it on. Decided by the owner on
September 26, 2026, as the variant that keeps only the substance of Codex's worktree:
a detached checkout made with the public `git worktree add`, and rewake's own record of
it. Codex's private scheme — its directory layout, a `--no-checkout` add,
`config.worktree`, the thread binding in the git metadata — is not repeated: it is no
contract, and a copy would drift from the next version silently.

- **The flag.** Codex's own spelling, read as a switch: `--worktree` alone asks for a
  generated name, six hex digits; `--worktree=<name>` names the checkout — a letter or
  digit, then up to 39 letters, digits, `-` or `_`. A spaced value is not read: the word
  after the switch stays the harness's, a prompt most often, as it would be for Codex
  itself. The flag after `--` is prompt text and stays. Given twice, or as `--worktree=`,
  it is a wrong call.
- **The checkout.** The repository is the one holding the launch directory — `-C` or
  `--cd` when given, else the current one. rewake adds a detached checkout of its HEAD
  commit: detached, so the branch checked out in the source stays free and a second
  launch from the same place does not collide with the first. Uncommitted changes in the
  source do not come along.
- **Where.** `$REWAKE_WORKTREES` when set (absolute), else
  `$XDG_DATA_HOME/rewake/worktrees`, else `~/.local/share/rewake/worktrees`; under it one
  directory per repository, named for it with a short hash of its Git directory so two
  repositories of one name stay apart, and in that one each checkout beside its record:
  `<repository>-<hash>/<name>/` and `<name>.json`. Not the state directory — that lives
  in `/tmp` and would not outlive a restart, while a checkout holds work — and not inside
  the repository, where it would show up in `git status` and in the agent's own
  searches: a `$REWAKE_WORKTREES` inside it, compared with symbolic links resolved, is
  refused. One place for every repository, so `rewake worktree ls` sees them all.
- **Where the launch starts.** At the launch directory's place within the checkout, as
  Codex does; at the checkout's top when that directory is not in the commit, an
  untracked one say. The wrapper changes into it before the session is registered, and
  `-C`/`--cd` are taken out of the arguments, so the record, the terminal and the
  app-server all work there. Relative paths among the harness arguments resolve there
  too.
- **Trust.** Codex resolves a linked worktree's trust to its main checkout, so a
  launch from a trusted repository keeps that trust in its checkout
  ([research-codex.md](research-codex.md#--worktree-with-a-remote-terminal)).
- **The record** says which repository (its shared Git directory and the checkout the
  launch came from), which commit, where, when, and — written once the session's name is
  claimed — which session: name, room, run and the room's state directory.
- **A new conversation only.** `resume` and `fork` are refused with `--worktree`: they
  continue a conversation in the directory it was started in. The terminal's
  `thread/resume` carries a cwd only from its own `-C`/`--cd`, which rewake takes out,
  and the server then restores the saved one, while the session's workspace roots are
  already the checkout's: the model would work in one place with its rights named for
  another (seen on 0.155.1 in a container with the real binaries, September 26, 2026;
  fork read in the source, not run). Moving the conversation into the checkout — handing
  the terminal the checkout as its `-C` — is not built: what that does to a continued
  conversation's permissions ([continuation-permissions.md](continuation-permissions.md))
  was not checked. The refusal names the two ways on: a new conversation with
  `--worktree`, or the continuation without it.
- **Refusals** exit 2 and nothing prompts; every git call runs with
  `GIT_TERMINAL_PROMPT=0` and in a session of its own, with no terminal to open. Most
  come before anything is made: the flag given twice or as `--worktree=`, `resume` or
  `fork`, `--remote`, a profile or `--oss`/`--local-provider`, a launch directory that
  does not resolve, a name out of shape, a launch directory outside any working tree, a
  repository with no commit yet, a worktree directory inside the repository. By then the
  worktree directory itself may have been created. A name taken by a checkout or by a
  directory in its place is found once the repository's directory under it is made,
  which is removed again when empty; the refusal gives the next step: another name,
  `rewake worktree rm`, or `cd` into the existing one. A refusal of the registration — a
  session name or a main already taken — comes after the checkout was made: it is taken
  back, and the line saying where the session works is printed only once the name is
  claimed.
- **After the session** the checkout stays, as Codex's own would, and a line on stderr
  says where and how to remove it. A launch that fails after the checkout was made, or
  whose harness exits with an error — a resume of a conversation that does not exist,
  say — takes it back when it is still at its commit and rm without `--force` would
  remove it.

`rewake worktree ls` lists the checkouts with their owner, the rewake sessions still
running in each, and whether removing one would lose anything; `--json` gives the same
model. A record whose path is not the directory of its own name beside it is not
listed: rewake did not make that path, and rm would remove it.
`rewake worktree rm <name>` — or `<repository>/<name>` when a name is in two
repositories — removes one through `git worktree remove`, so the repository forgets it
too. Without `--force` it never loses work, and when it cannot tell, it refuses. It
refuses, naming each reason:

- changes `git status` shows;
- files git ignores — a `.env`, a local build — which `git worktree remove --force`
  deletes without a word;
- a HEAD no branch, tag or remote-tracking ref holds, asked every time: the commit a
  checkout was made at is lost too once the branch that held it is deleted;
- a rewake session still running in it: the one it was made for, or any whose working
  directory is in the checkout — started there by hand after the first ended, say — in
  any room of the current state directory and of the one the owner registered in. A
  process rewake did not start is not seen;
- a directory gone while the repository still lists it: it may have been moved with its
  work, and `git worktree repair <new path>` run in the repository reconnects it. With
  `--force` its own entry is removed with `git worktree remove`, never `git worktree
  prune`, which would take every other missing checkout of the repository along. One
  the repository no longer lists either leaves only the record, which rm removes.

`--force` removes it anyway. Looking never stands in a session's way: git runs with
`GIT_OPTIONAL_LOCKS=0`, so `git status` does not take `index.lock` from under a commit
the session is making. Nothing is removed automatically.

A main's `--grant-git` reaches such a checkout as it does a worktree made by hand: the
grant reads the conversation's cwd, which is in the checkout, and resolves its private
and common Git directories
([research-permissions.md](research-permissions.md#managed-worktrees-and-continuation-permissions)).
Claude Code keeps its own `-w`/`--worktree`: rewake does not take it.

Unknown TUI arguments are preserved, not interpreted as server configuration.

Startup checks codex --version against the version the transport was last observed
working on — 0.155.1 since September 21, 2026 — and warns, without refusing a different
version. Moving the pin takes two edits that have to agree: the constant
`lastObservedServerVersion` in `internal/harness/codex/server.go`, and the literal the test
fixture prints. The duplication is deliberate: a fixture echoing the constant back
would make the matching test tautological and let a typo in the pin through. A fake
executable and socket server cover the process and protocol contract. Real-model
acceptance remains a separate owner-run check.

### Fresh workspace permissions

The native adapter preserves the caller's sandbox selection. In the September 20
owner check, a fresh disposable directory without a trust/sandbox selection chose
read-only and could not write the inbox `.lock`. The owner selected
`-c 'sandbox_mode="workspace-write"'` for that test launch; configuration and runtime
were not modified. Inbox reads need write access for locking and receipts. This
acceptance does not establish inbox support under deliberately selected read-only
permissions. [Evidence](native-mailbox-acceptance.md) and the
[updated owner recipe](native-mailbox-check.md) retain that prerequisite.

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
