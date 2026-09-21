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

### Launch defaults from the environment

Sessions multiply, and one that quietly picks an expensive model costs real money
for work that did not need it. So rewake can supply a default model and reasoning
effort — as flags for a single launch, never by editing anyone's configuration.

The values are named one pair per harness, because the harnesses neither name their
models the same way nor take the reasoning effort the same way:

| Variable | What it sets |
| --- | --- |
| `REWAKE_CODEX_MODEL` | the model for a Codex launch |
| `REWAKE_CODEX_EFFORT` | its reasoning effort |
| `REWAKE_CLAUDE_MODEL` | the model for a Claude Code launch |
| `REWAKE_CLAUDE_EFFORT` | its reasoning effort |

No model names appear in this repository. Which model is cheap, and what it is
called, belongs to whoever runs the sessions.

They can be set in three places, and rewake reads the files itself — a setting that
has to be loaded by hand before every launch is not a setting but a ritual, and the
one time it is forgotten a session comes up on something else without saying so:

| Where | For |
| --- | --- |
| the environment | this launch, or this shell |
| `.rewake.env` in the working directory | one repository |
| `~/.config/rewake/settings` | everything this person launches |

Both files hold `KEY=VALUE` lines: blank lines and `#` comments are skipped, spaces
around the name and the value are trimmed, and one matching pair of quotes is removed.
Nothing more — no `$VAR` expansion, no commands, no line continuation. A file that can
run things is a different and much larger promise, and this is a settings file.

**A file may set those four names and nothing else.** Not a prefix — a list. That is
a boundary rather than tidiness: a file sitting in whatever directory somebody happens
to be in must not become a way to set arbitrary variables for the harness launching
there, and names like the state directory or the room are spelled with the same prefix.
Everything else in the file is ignored without comment, so the file can serve other
purposes too. `export KEY=VALUE` is accepted, since that is how such a file is usually
written, and a `#` after whitespace begins a comment that runs to the end of the line
— inside quotes it is part of the value.

The strongest source wins, and strength is nearness to the launch: a flag, then a
variable already in the environment, then the project file, then the user file. A
variable that is already set is never replaced by a file. `.rewake.env` in the working
directory is the only project file consulted — no walking up the tree, where a parent
directory could decide how a session launches.

Trouble is said, not swallowed: a file that exists but cannot be read, one that is not
an ordinary file at all, a line that is not `KEY=VALUE`, or a user file with
permissions wider than the `0600` its convention expects — each produces a launch note,
and none of them stops the launch. The permission warning is for the user file only: a
project file created under the usual umask is `0644`, and warning about that on every
launch would teach people to skip these notes.

### Naming a whole launch

A launch that states a role, a name, a model and a configuration override is long
enough that people stop typing it and start approximating it — and that is how a
session comes up as something other than what was meant. An alias is the opposite of
approximating: one name, one recorded set of arguments.

```toml
# ~/.config/rewake/aliases.toml, or .rewake.toml in the working directory
[alias.wcodex]
harness = "codex"
rewake  = ["--write", "--name", "writer"]
args    = ["--model", "some-model", "-c", "model_reasoning_effort=high"]
```

`rewake wcodex` is then the whole launch. TOML, because this is a structure rather
than a list of names and values; a separate file from `settings`, because that one is
`KEY=VALUE` and a file cannot be both.

**Three fields, not one list.** What rewake reads, which harness to start, and what
that harness gets are not interchangeable, and a single flat list would have to be cut
at the harness name for anyone — a person editing the file included — to know which
flag belongs to whom. **Lists, not strings**, for the same reason everywhere: a string
would have to be split into words, and splitting words means quoting rules.

An alias states a default, and a flag typed on the line *replaces* the alias's copy of
it — dropped, not merely outranked. So `rewake --name reviewer wcodex --model another`
launches as `reviewer` on `another`, and the command that reaches the harness names the
model once.

Dropping it is the only thing that works, and only for some flags. Codex refuses to
parse a repeated `--model` rather than taking the last one, so leaving both would end
the launch with a complaint about a flag the person wrote once. But a repeated
`--add-dir` is how a second directory is added, and a repeated `-c` key means one thing
for a plain value — the last wins — and another for a structured setting, which merges.
Replacing either would remove something nobody asked to remove, and sorting the two
kinds of setting apart means understanding the value, which the harness already does.

So the flags that get replaced are a closed list, published by each harness and applied
only to launches of that harness. Each entry names one parameter by all its spellings —
a typed `-m` replaces an alias's `--model` — and says whether it carries a value, which
is what keeps a dropped `--search` from taking the prompt standing behind it. Everything
else is appended and left to the harness, which is the safe answer for anything
unlisted; `-c` is never collapsed.

The reason differs by harness, and that difference is the point. Codex refuses to parse
a repeated flag, so replacement is what keeps the launch alive. Claude Code accepts one
and takes the last occurrence, so replacement there only keeps the command readable and
the two behaving alike. Both lists live in their adapters
([research-launch.md](research-launch.md) records what each was read from).

Two things this deliberately does not try to do. `--enable X` with `--disable X` is not
"the last one wins" — Codex applies every enable and then every disable — and flags that
conflict under different names, an approval policy against a sandbox mode, are not found
by comparing names at all. Resolving those is the harness's business, and an alias that
sets up such a pair is a mistake the person has to see rather than one rewake silently
half-fixes.

**Where the flags end.** `--` ends them for everything that follows it in the command
being built, not just in the half it was written in. A terminator you type ends your own
arguments; a terminator the alias carries ends its own tail *and* everything the line
adds behind it, because that is where those words land. So an alias ending in `--` is one
whose arguments are fixed: what you add after it is input for the harness, and a word of
a prompt can never take a setting out of the alias. Checking each half separately let
exactly that happen.

**Where a flag goes:** rewake's own flags before the alias name, the harness's after it.
`rewake --name reviewer wcodex --model another` sets the session name and the model;
writing `--name` after the alias would hand it to the harness, which knows nothing about
session names. That is the general rule of the command line — everything after a launch
word belongs to the harness — and an alias does not change it.

Environment defaults still fill in what the alias did not name.

An alias names arguments to rewake and nothing else. It cannot begin with a command
word, so it cannot turn `rewake <name>` into some other command — the same boundary
the settings file draws by having no substitutions.

**A project file may not choose a role.** `--main`, `--write` and `--general` belong in
the user's own file; an alias in `.rewake.toml` that carries one is refused by name. The
role decides more than the room does — a main session sees everyone's telemetry and may
ask for repository access — and a file sitting in whatever directory somebody happens to
be in should not decide that for them. The user file is also held to `0600`, and a
launch says so when it is not: it can name a role, which is more than the settings file
beside it can do.

Refusals say what happened rather than falling through: a name that is not an alias
and not a command lists both, an alias with no harness or an unknown one says so and
names the file it came from, and a file that is present but unreadable produces a note
instead of silence. A launch that quietly runs something other than what was asked for
is the failure this feature exists to prevent, so it is the one outcome that is never
allowed.

Three rules, all about not surprising anyone. An explicit flag always wins: a launch
that already names a model or an effort gets nothing added and no note. Nothing
configured means nothing substituted — behavior without configuration is exactly what
it was before — and an **empty** variable is a choice rather than an absence: it turns
the default off for that launch, and the files are not consulted behind it. And
whatever is substituted is announced in a launch note naming both the variable and
where its value came from, because a silently swapped model is the worst kind of
surprise, and the second worst is not knowing where to change it.

"Already named" means every way of naming it, because the way a person writes a
choice must not decide whether it is heard. Checked against the installed binaries,
not from memory:

- **Claude Code** takes both as flags, `--model <model>` and `--effort <level>`,
  apart or with `=`. Neither has a short form.
- **Codex** takes the model as `--model` or `-m`, apart, with `=`, or joined
  (`-mname`) — and also as the configuration key `model`. It has no flag for the
  reasoning effort at all: that is the configuration key `model_reasoning_effort`.
  Keys are recognized through `-c`/`--config` in every spelling, with whitespace
  around the key allowed, because the CLI trims it.

Not consulted, deliberately: the harnesses' own configuration files, and settings
passed with `--settings` or a Codex profile. rewake neither reads nor edits a
person's configuration, and cannot see into a file it was handed — so a model set
there and a variable set for rewake would be two answers with no way to compare them.
The variable wins, being the one set for rewake specifically. A Codex launch with a
profile is refused outright before any of this, as are `--oss` and
`--local-provider`.

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
and the ordinary TUI connected with --remote through its inline gateway. The
original socket belongs to the gateway; the native endpoint adds `.up`. The socket uses the existing
per-run digest fallback and 103-byte limit. The server has its own process group,
so keyboard interrupts reach the TUI without killing its transport. Both children
end with the session; no daemon lifecycle command is used. Server stderr goes
to the adjacent private `.up.log` file. Server death terminates the TUI and refuses
pending mail as session ended.

The adjacent `.gateway.log` records bounded, payload-free connection-close reasons;
see [startup transport diagnostics and limits](startup-transport.md).

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
the wrapper's original cwd. Existing --remote, --profile, --worktree and --oss/--local-provider launches
are refused with advice: they cannot safely share this owned server topology or
forward all configuration. Create the checkout first and use explicit settings.
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
