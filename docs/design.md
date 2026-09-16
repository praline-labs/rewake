# rewake: first-version design

The facts behind this design live in `docs/research.md`. This document covers what we're
building and how.

## Goal

`rewake` lets interactive coding-agent harnesses on one machine talk to each
other. A human launches a harness through the tool — `rewake claude`, `rewake
codex` — and gets an ordinary program in their terminal. The session registers
itself, and any other session, or a human from a shell, can write to it with
`rewake send <name> "text"`. The text arrives at the recipient as an incoming
message and wakes it if it's idle.

The primary user of these commands is an agent calling them from its own shell.
That's where the requirements on output and failure messages come from (see the
"Interface" section).

## Scope of the first version

In scope: Claude Code and Codex, the commands `claude`, `codex`, `list`, `send`,
`whoami`, the no-argument overview, `--json`. Linux.

Out of scope: pi, opencode, grok (their delivery paths are covered in research);
macOS (needs replacements for `/proc`); delivery receipts from the recipient;
message history; a UI.

Never doing: a pseudo-terminal proxy or typing text into someone else's screen;
editing the user's harness configs; reading transcript contents.

## Design

### Processes

**Wrapper** — the `rewake claude|codex` process. It stays the harness's parent
for its whole life: launches it with an inherited terminal, registers the
session, services its inbox, and on exit deregisters the session and returns the
harness's exit code. There's no daemon: each wrapper is the delivery point for
its own session only.

**Sender** — any process that calls `rewake send`. It doesn't deliver anything
itself: it drops a file into the recipient's inbox and waits for a status. This
way sending works the same from a plain shell, from Claude Code's Bash, and from
a Codex sandbox, which can write to `/tmp` but is denied sockets and `~/.codex`.

There's no separate daemon: nothing to start, stop, or recover, and there's no
state where "the sessions exist but the daemon is dead". Exactly as many wrapper
processes run as there are open sessions; a session without a wrapper doesn't
take part in messaging.

### State directory

`$REWAKE_DIR`, defaulting to `/tmp/rewake-<uid>`. It has to be `/tmp`: the Codex
sandbox writes there by default, and `$XDG_RUNTIME_DIR` isn't reachable from it.
The wrapper passes `REWAKE_DIR` to children explicitly, so a sandbox with a
different `TMPDIR` still finds the same directory.

Checked on every open: it's a directory, not a symlink, owned by the current
uid, with no group or other access; otherwise it fails with an explanation.
Created with 0700.

```
/tmp/rewake-<uid>/
  sessions/<name>.json       session record
  inbox/<name>/<id>.json     message awaiting delivery
  inbox/<name>/<id>.status   status written by the servicing process
  inbox/<name>/done/         delivered and failed, for diagnostics (cleaned up by age)
  sock/<name>.sock           Claude Code inbound socket, path set by the wrapper
```

The socket path is deliberately short: the limit is 103 bytes.

### Session record

```json
{
  "name": "claude-2",
  "harness": "claude",
  "managed": true,
  "servicePid": 12345,
  "serviceStart": 1671399,
  "harnessPid": 12346,
  "harnessStart": 1671402,
  "cwd": "/home/u/code/x",
  "startedAt": "2026-09-16T00:08:03Z",
  "claude": { "socket": "/tmp/rewake-1000/sock/claude-2.sock" },
  "codex": { "home": "/home/u/.codex" }
}
```

- `serviceStart`, `harnessStart` — field 22 of `/proc/<pid>/stat` (start time in
  ticks). Liveness = the process exists and the start time matches: pids get
  reused.
- A session is alive as long as both the servicing process and the harness are
  alive. Any reader of the registry deletes a dead record.
- Publishing a record is atomic and exclusive: write a temp file, then `link()`
  it to the final name — `link` fails if the name is taken. If the existing
  record belongs to a dead session, it's removed and the attempt retried; for a
  live one, the name stays taken.
- Updating one's own record (for example, the harness pid after launch) uses a
  temp file and `rename()`.

### Names

`[a-z0-9][a-z0-9._-]{0,31}`. The default is the harness name; if that's taken,
`claude-2`, `claude-3`. Explicit: `rewake --name api claude`. A session's name is
its address, so a live name is never reused.

### Environment the harness receives

- `REWAKE_SESSION=<name>` — who I am; `send` uses it to sign the sender.
- `REWAKE_DIR=<directory>`.
- Inherited Claude Code markers are stripped (list in research): otherwise
  `rewake claude` launched from inside another session would inherit that
  session's socket and have transcript saving disabled.

None of the variable names contain `KEY`, `SECRET`, or `TOKEN`: Codex strips
those from the agent's command environment.

## Launching a harness

The common part of the wrapper:
1. Check the directory, pick and publish a name (the harness pid is still empty).
2. Launch the harness: `exec.Cmd` with inherited stdin/stdout/stderr, the same
   terminal and process group. Arguments after the harness name are passed
   through as-is.
3. Add the harness's pid and start time to the record.
4. Ignore `SIGINT`, `SIGQUIT` (the harness handles them, being in the same
   foreground group); forward `SIGTERM`, `SIGHUP` to the harness.
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
- Allow the tool's own commands without confirmation:
  `--allowedTools "Bash(rewake:*)"`. This adds a rule for the run without
  touching the user's settings. **Verify live** that the flag adds to the user's
  permissions rather than replacing them; if it replaces them, drop the flag and
  have the overview say which rule to add to settings once.

### Codex

- Intro: `-c developer_instructions=<text>`. This key **replaces** the user's
  value instead of adding to it, so the wrapper reads
  `$CODEX_HOME/config.toml`, takes the existing value if any, and passes the
  concatenation of "the user's value, a blank line, the intro". If there's no
  existing key, it passes just the intro. The `additional_developer_instructions`
  key doesn't work for this: it belongs to the managed-requirements layer, not
  the user config.
- Permissions: the state directory lives in `/tmp`, where the sandbox writes by
  default. If the user's config excludes `/tmp` from
  `sandbox_workspace_write` (`exclude_slash_tmp = true`), the wrapper adds the
  directory to `writable_roots` via `-c`, keeping the paths already listed there.
  No confirmation is needed to run `rewake`: the command runs inside the
  sandbox.
- Record `CODEX_HOME` in the session (the environment value, or `~/.codex`).
- **Verify live**, with one cheap turn, that the intro from `-c
  developer_instructions` actually reaches the model.

### The intro

Short text, in English:

```
You are running inside rewake as session "<name>". Other agent sessions on this
machine can message you, and you can message them.
- rewake list — who is running
- rewake send <name> "text" — deliver text to a session
Incoming messages start with "[rewake] message from: <name>" and end with the
exact command to answer them.
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

## Delivery

### Sender (`rewake send <name> <text>`)

1. Parse arguments: exactly one name and one text (`-` reads the text from
   stdin). Extra positional arguments are the error "Quote the text as one
   argument".
2. Look up a live session; if there's none, fail and list the live names.
3. The message: `{"id","from","to","text","createdAt"}`. `id` is time-sortable
   (nanosecond timestamp plus a random tail). `from` is `REWAKE_SESSION` or
   `shell`.
4. Write `inbox/<name>/<id>.json.tmp`, rename it to `.json`.
5. Wait for `.status` up to `--wait` (5 seconds by default) and print the
   result.

### Servicing process (wrapper or watchdog)

Polls the inbox every 250 ms. Messages are processed in `id` order. For each
one: call the delivery adapter, write the status (`.status.tmp`, then rename).
`delivered` and `failed` are moved to `done/`; `pending` stays and is retried
every 2 seconds. A message older than `--ttl` (30 minutes by default) gets
`failed: expired`.

Status: `{"state":"delivered|pending|failed","via":"socket|codex-queue","detail":"...","at":"..."}`.

### Text the recipient sees

```
[rewake] from <sender> · <short id>
<text>
```

The short id makes the text unique: Claude Code drops identical text from the
same sender within 30 seconds. If the sender is `shell`, the header line reads
`[rewake] from shell`.

### Claude Code adapter

Connect to `claude.socket` with a 2-second timeout, write the line
`{"type":"user","message":{"role":"user","content":<text>},"priority":"next"}`
followed by `\n`, then close. A successful write means `delivered`. `ENOENT` and
`ECONNREFUSED` mean `pending` as long as the harness is alive (the socket hasn't
been created yet, or is being recreated); otherwise `failed`.

### Codex adapter

1. Find the current thread: walk the process tree from `harnessPid`, collect the
   `/proc/<pid>/fd/*` links pointing at
   `<CODEX_HOME>/thread-writer-locks/<uuid>.lock`, and pick the lock with the
   latest mtime. None found: `pending: codex has not opened a thread yet`.
2. `codex queue --thread <uuid> --message <text>` with the session's
   `CODEX_HOME`, 15-second timeout.
3. Exit code 0 means `delivered`, noted with "Codex will start a turn within
   ~10s". `no rollout found` means `pending: the codex session has no
   conversation yet; delivers after its first turn`. Anything else is `failed`
   with Codex's error text.

The thread is chosen fresh on every attempt: after `/new` or `/resume`, the
message goes to the current thread, not the one from when the session started.

## A session without a wrapper

Not supported: an owner decision. Only what's launched through `rewake` takes
part. A plain `claude` in another terminal doesn't show up in `list` and doesn't
receive messages — it has to be restarted through the wrapper.

## Interface

The conventions are carried over from i-plane, where agents have already
proven them out.

### Commands

```
rewake                                  overview (command map, workflow, behavior notes)
rewake [--name N] claude [args...]      launch a Claude Code session under rewake
rewake [--name N] codex [args...]       launch a Codex session under rewake
rewake list [--json]                    live sessions
rewake send <name> <text|-> [--wait S] [--json]
rewake whoami [--json]                  this session's name and directory
rewake <command> --help
```

The tool's own flags only work before the harness name; everything after it
belongs to the harness.

### Output

- With no arguments — the overview, exit code 0: command groups, the workflow as
  real invocations, and behavior notes (exit codes, the refusal to ever prompt
  interactively, the Codex delay).
- One line per object, aligned columns, empty values are omitted.
  ```
  claude-2  claude  idle  /home/u/code/api  started 12m ago
  codex     codex         /home/u/code/web  started 3m ago
  ```
  Busy/idle state is shown only where the harness reports it itself: for Claude
  Code, the `status` field from `~/.claude/sessions/<pid>.json` (private format;
  a missing field means an empty column). Codex has no such signal without
  screen scraping.
- `send` prints the result as one line:
  ```
  delivered to claude-2 via socket
  delivered to codex via codex queue; codex starts a turn within ~10s
  pending for codex: the codex session has no conversation yet; delivers after its first turn
  ```
- `--json` on every command prints the full model; the text form is deliberately
  trimmed down. No colors, no TTY-dependent behavior.
- A single command table (name, arguments, flags, summary, examples, next) is
  the source for parsing, the overview, `--help`, and the hint shown on error. A
  test checks the table against the handlers and runs the examples.

### Failures and exit codes

| code | meaning |
|---|---|
| 0 | done; for `send` — delivered |
| 1 | the target failed or is unreachable: delivery `failed`, or the session died while waiting |
| 2 | invalid invocation: unknown command or flag, extra argument, no such session, name taken, unsafe directory |
| 3 | `send`: accepted but not yet delivered (`pending` at the end of `--wait`); the message will still deliver on its own |

An invocation failure shows: the reason, the syntax, an example, the flags, and
`full help: rewake <cmd> --help`. An unknown command gets the closest name ("did
you mean") if the edit distance is small. Never prompt interactively.

## Code

Go 1.25, no external dependencies (standard library and `syscall` only).

```
cmd/rewake/main.go            entry point, top-level parsing
internal/cli/                 command table, parsing, overview, help, failures, printing
internal/state/                directory: checks, paths, atomic writes
internal/registry/             session record, name publishing, liveness, listing
internal/proc/                  /proc: start time, process tree, fd links
internal/inbox/                 message, status, sender-side write, servicing loop
internal/harness/claude/        launch arguments, environment, socket delivery
internal/harness/codex/         launch arguments, thread lookup, delivery via codex queue
internal/wrap/                  wrapper: launch, signals, lifecycle
```

The harness adapter is an interface:

```go
type Harness interface {
    Name() string
    PrepareLaunch(s *registry.Session, args []string) (argv []string, env []string, err error)
    Deliver(ctx context.Context, s registry.Session, text string) inbox.Result
}
```

## Testing

Unit tests (`go test`):
- name publishing: a race between two publishes of the same name — one wins; a
  dead record gets evicted; a live one doesn't;
- liveness: a reused pid with a different start time counts as dead;
- inbox: delivery order, `pending` retries, TTL, the status is written
  atomically;
- Codex thread lookup on a fixture of a `/proc`-like tree (the `/proc` root is
  parameterized);
- argument parsing and the command table: the table's examples parse cleanly.

Live tests, scripted under tmux, in a separate `/tmp` directory:
1. `rewake claude --model haiku` and `rewake codex`, `rewake list` sees both.
2. From a shell: `send claude "reply pong"` — a reply shows up on Claude's screen
   within seconds.
3. From the Codex sandbox (`codex sandbox -P :workspace -- rewake send ...`) —
   delivery to Claude through the inbox, without calling the model.
4. `send codex` before the first message — `pending`, exit code 3; after the
   first turn — the turn starts on its own.
5. The Claude agent runs `rewake send` without confirmation (checks
   `--allowedTools`).
6. Harness exit — the record and socket are gone, pending messages got
   `failed`.

Live Codex tests spend subscription quota: run them on a cheap model with short
messages.

## Distribution

- Build: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64|arm64 go build`, a binary a few
  MB in size.
- npm: `@iiiokojiadbi/rewake` with a shim, plus platform packages
  `@iiiokojiadbi/rewake-linux-x64`, `-linux-arm64` in `optionalDependencies` (the
  esbuild pattern). The scoped package is published with `--access public`.
- A non-npm alternative: `go install`.

## Owner decisions, September 16, 2026

1. The intro is injected at launch for both harnesses, on by default; turned off
   with `--no-intro`.
2. Permissions for the tool's own commands are granted for both harnesses, not
   just Claude Code.
3. The state directory in `/tmp` with 0700 permissions — this trust boundary is
   accepted.
4. A session without a wrapper is not supported; `register` is dropped from the
   first version.
