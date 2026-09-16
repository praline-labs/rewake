# Session launch and signals

[Back to the design](design.md).

## Launching a harness

The common part of the wrapper:
1. Check the directory, pick and publish a name (the harness pid is still empty).
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

### Claude Code

- Everything after the harness name is passed through untouched, with one
  exception: a `--help` written first asks rewake for the command's help page
  instead of starting the harness. `rewake claude --model x --help` still reaches
  the harness.
- Add `--messaging-socket-path <dir>/sock/<name>.sock` unless the user passed
  their own; before launch, remove a stale socket file at the same path.
- Add `--append-system-prompt <intro>` (turned off by `--no-intro`).
- Add `--settings` with a Stop hook that runs `rewake turn-ended` (see "The end
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
  For an ordinary repository, the added directory is `<cwd>/.git`. A worktree
  or submodule has a `.git` file: rewake reads its `gitdir: <path>` relative to
  the checkout, then reads `commondir` relative to that metadata directory when
  present. Both per-worktree and shared metadata are added, without granting
  their parent directories or other checkouts. Discovery does not invoke Git.
- A missing or malformed pointer, a non-directory target or a symlink in the
  metadata path leaves the grant out with a one-line reason and a suggestion
  to pass the actual directories through `--add-dir`. Resume, fork, remote
  execution and `--worktree` can select a repository after launch, so their
  metadata is also left for the caller. Existing worktrees are supported.
- When `/tmp` may be excluded, rewake still says which directory to add to the
  writable paths if messages cannot be sent. Granting Git access does not grant
  the message directory or override a user's temporary-directory exclusions.
- End of a turn: `-c notify=["<rewake>","turn-ended"]`. Codex runs that program
  after every turn, outside the sandbox and without the trust a Stop hook needs.
  The key replaces the user's program, so it is passed only when neither the
  command line nor `config.toml` mentions `notify` at all; otherwise rewake says
  on stderr that turns will not be reported.
- Record `CODEX_HOME` in the session (the environment value, or `~/.codex`).
- **Verify live**, with one cheap turn, that the intro from `-c
  developer_instructions` actually reaches the model.

### The intro

Three lines, in English. It says what rewake is and where the instructions are;
the instructions themselves live in `rewake guide`, which always matches the
binary and costs context only when read:

```
You are running inside rewake as the session "<name>".
rewake lets agent sessions on this machine message each other; a message waiting
for you is announced by a line starting with "rewake:".
Run `rewake guide` before you send or read messages: it explains how.
```

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

