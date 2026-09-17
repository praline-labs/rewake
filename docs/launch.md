# Session launch and signals

[Back to the design](design.md).

## Launching a harness

The common part of the wrapper:
1. Check the shared state root and selected room directory. Under the room's
   launch lock, choose the role and publish the name (the harness pid is still
   empty). The lock is released before preparing the child.
2. Launch the harness: `exec.Cmd` with inherited stdin/stdout/stderr, the same
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
room. `--name` chooses a name unique within that room. Both use the same
lower-case name syntax, up to 32 characters.

`--main`, `--general` and `--write` explicitly select one role and cannot be
combined. Without a role flag, no live main in the room means main; an existing
main means worker. Explicit main refuses when the room already has one, naming
its occupant and suggesting a restart or a launch without `--main`. An explicit
worker or writer can be the room's first session. There is no running-session
promotion when main exits: the next automatic launch can become main.

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

- rewake does not read `config.toml`; it only asks whether a key is mentioned
  in it at all, in any form (escapes included). Every value it could pass for
  these keys replaces the user's, and two rounds of a hand-written reader each
  missed valid TOML that dropped instructions or turned prose into sandbox
  permissions.
- Intro: `-c developer_instructions=<text>`. The key **replaces** the user's
  value, so it is passed only when the configuration does not mention it and no
  profile is selected; otherwise rewake says on stderr that the briefing was
  skipped.
- Permissions: main and write request Git metadata access through repeated
  `--add-dir <metadata directory>` flags. Worker and the zero role receive no
  extra roots. The flag adds to the user's roots; no `writable_roots` array,
  permission profile, sandbox mode, approval policy, network or tmp setting is
  replaced. A caller's existing `--add-dir` flags remain, including duplicates.
  The selected sandbox policy still decides whether added roots are writable.
- The effective cwd honors `-C`/`--cd`, including joined forms, before `--`.
  The search walks from cwd towards the filesystem root, stopping at the first
  `.git`, even when it is invalid. For an ordinary repository the added directory
  is `<repository>/.git`. A worktree or submodule has a `.git` file: rewake
  reads its `gitdir: <path>` relative to the directory containing that pointer, then reads `commondir` relative to that metadata directory when
  present. The gitdir must have a regular `HEAD`; shared metadata must also have `HEAD`,
  `objects/` and `refs/`. A worktree-private gitdir need not have objects or refs.
  Both per-worktree and shared metadata are added, without granting
  their parent directories or other checkouts. Discovery does not invoke Git.
- A missing or malformed pointer, a non-directory target or a symlink in the
  metadata path leaves the grant out with a one-line reason and a suggestion
  to pass the actual directories through `--add-dir`.
- `resume` and `fork` retain the metadata grant discovered from effective launch
  cwd. The extra root does not replace the resumed conversation's workspace.
  `--remote` still skips local metadata: those paths belong to this machine.
- A new `--worktree` also needs its private gitdir, allocated after launch.
  Granting only the source `.git` is insufficient in the managed checkout
  layout: the sandbox can reapply a read-only mount to its private metadata.
  Rewake therefore keeps this skip with an explanation. Create the worktree
  first and launch from its directory; existing worktrees are supported through
  their known gitdir and commondir pointers.
- The session record's cwd remains the wrapper's launch directory. It is not
  updated when the harness changes the agent's working directory for a managed
  worktree, a continuation or `-C`.
- When `/tmp` may be excluded, rewake still says which directory to add to the
  writable paths if messages cannot be sent. Granting Git access does not grant
  the message directory or override a user's temporary-directory exclusions.
- End of a turn: `-c notify=["<rewake>","turn-ended"]`, including main so
  supplied failures remain observable. Successful main turns are filtered out.
  Codex runs that program
  after every turn, outside the sandbox and without the trust a Stop hook needs.
  The key replaces the user's program, so it is passed only when neither the
  command line nor `config.toml` mentions `notify` at all; otherwise rewake says
  on stderr that turns will not be reported.
- Record `CODEX_HOME` in the session (the environment value, or `~/.codex`).
- **Verify live**, with one cheap turn, that the intro from `-c
  developer_instructions` actually reaches the model.

### The intro

The system layer uses an independent six-line briefing for general, write or
main from `internal/brief/roles.go`. Each names its identity and selection reason,
explains the commands that role needs, and points to guide for the full contract.
General receives no Git metadata grant; write may commit; main delegates and
reads results. All ask for the point of a message in its first line. Per-role
snapshots make wording changes reviewable.

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

