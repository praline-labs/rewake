# Injecting the mail tool at launch

The third of three stages of [mail-bridge.md](mail-bridge.md): how a launch through rewake
gives the harness the server of [mail-bridge-server.md](mail-bridge-server.md), approves
exactly its one tool, refuses a launch whose person already has a server named `rewake`,
and proves that nothing of the person's configuration changed. The run's mail channel —
its record, notices, display and the briefing — is in
[mail-bridge-channel.md](mail-bridge-channel.md). **Design, third pass, October 4, 2026;
built and its code accepted the same day; the live checks ran the same day**
([mail-bridge-live.md](mail-bridge-live.md)); the rules they changed are marked
*revised after the live checks*, and were built the same day. Until
a gate closes, the launch takes the action this document gives for what it leaves unknown
([gates](#gates)). The reviews' findings and what meets each are in the stage's reports.

Facts are marked by how they are known: *verified* — run on October 1, 2026 against the
installed Codex 0.159.0 or Claude Code 2.1.284, in a disposable home, no model turn;
*source* — read at the tag `rust-v0.159.0` of the Codex reference tree, or as strings in
the Claude Code 2.1.284 binary; *probe* — the probes of September 30
([mail-bridge.md](mail-bridge.md)); *gate* — unknown until the named live check, with
the action taken until then.

## The rules

1. **The tool is added, and nothing else changes.** On Codex the launch adds leaves
   under the absent `mcp_servers.rewake` table and nothing outside it. On Claude Code it
   adds one `--mcp-config` layer holding only our server, one allowance naming only our
   tool, and the observer hooks inside the one settings layer rewake already passes. No
   configuration file, configuration home, global approval, sandbox, code mode, output
   limit or timeout is written or overridden; the timeouts our own entry carries are
   part of it and bind our server only. How that is proven, without a harness and live,
   is in [mail-bridge-live.md](mail-bridge-live.md#proving-preservation).
2. **The name is proven free, source by source, before the run is published.** Each
   harness has a table below: every source of an MCP server name it reads, the check
   that covers it, and what is done where coverage is unknown. A name found in a covered
   source — disabled, rejected or pending approval included — refuses the launch. A
   check that fails refuses with the reason (the specification's rule). A source known
   in advance to be beyond any check of this launch — a working directory the harness
   will choose itself — gets no tool rather than a guess. An unknown source is never
   taken for an empty one, and a working launch has no source behind an open gate: a
   gate that closes without a check for its source makes that source's action "no tool"
   wherever it may apply. Rewake never takes another name, never renames or removes the
   person's entry, and never edits the file that holds it.
3. **Rewake approves exactly `mcp__rewake__rewake` for this launch** (main's decision of
   October 1, 2026): `default_tools_approval_mode="approve"` on our one-tool server on
   Codex, the exact name in the allowances on Claude Code. A denial or a managed
   restriction the person set stays in force and is shown, never overridden.
4. **The server is injected only where it can work.** No endpoint or capability, no
   hooks to observe the calls, a check that cannot cover this launch's directory, or the
   person's `--no-mail-tool`: the run goes on through the shell, the launch says why on
   stderr, and the channel record starts as "no tool". A launch is refused only by rule 2.
   `--no-mail-tool` is rewake's launch flag (the owner's decision of October 4, 2026): it
   skips the injection and the name check, and the channel shows "no tool" with the
   reason `--no-mail-tool`. It is the person's way past a name check they cannot satisfy
   at once, chosen by them; rewake never takes it on its own after a refusal.
5. **The limits that decide a read are the harness's effective ones, taken where the
   harness applies them.** A limit not known for a call keeps that call from
   acknowledging a read; a timeout not known keeps the call from running. No value the
   wrapper merely inherited stands in for one the harness may have replaced.
6. **Checks run the person's programs only as the harness itself would, bounded, and
   leave nothing behind.** Each check runs in a session of its own with a time bound
   and captured, bounded output, under a holder: a process of rewake's that starts
   nothing but the check and is the subreaper of its tree. Whatever the check started is
   ended and reaped on every outcome — every descendant of the holder, in whatever group
   or session it moved to, since every orphan of the tree comes to the holder; a process
   that cannot be ended, or a holder that cannot say how its tree ended, refuses the
   launch. Neither a group nor a session tells the check's processes from rewake's own: a
   descendant can leave either, and rewake's other children may live in sessions of
   their own. Only the holder's tree is the check's, and the wrapper itself is no
   subreaper.
7. **Diagnostics say where, never what** ([the refusal](#the-refusal-and-what-may-be-shown)).
   No output of a check, no argument value and no configuration content is printed,
   logged, noted or published, on any surface.
8. **Nothing persists past the run.** What the launch writes lives beside the run's
   sockets, or in the check's private directory, and goes with them.

## Where the rules live

| Rule | Where |
|---|---|
| 1, 3 | `internal/harness/codex/mailtool.go` (the leaves), `internal/harness/claude/mailtool.go` (the server file and the allowance), `claude/settings.go` (the hooks) |
| 2, 6, 7 | `codex/mailtool_check.go` and `codex/mailtool_layers.go`, `claude/mailtool_check.go`, `internal/harness/check.go` (the bounded run), `check_holder.go` (the holder) and `check_diagnostic.go` (the closed list), called by `chooseTool` in `internal/wrap/mailtool_launch.go` before the claim |
| 4 | `internal/wrap/mailtool_launch.go` (the choice and its note), `internal/wrap/mailtool.go` (the capability and endpoint before the plan), `--no-mail-tool` in `internal/cli/registry.go` |
| 5 | `claude/settings.go` and `bridge-hook` (values per call), `internal/bridge/endpoint/limits.go` (deadline, acknowledgment), `claude/mailtool_check.go` (`ToolTimeoutVariable`, the setting the launch's deadline reads), `codex/mailtool_inject.go` (reads off) |
| gates | `internal/harness/gates.go`: the table, `REWAKE_GATES_ASSUMED`, what each launch takes as open; `gates_version.go`: the version read for it ([mail-bridge-version.md](mail-bridge-version.md)) |

## The launch, in order

1. **The arguments are settled** as today: aliases, defaults, `--command`. Gates taken
   as closed by `REWAKE_GATES_ASSUMED` are said on stderr first
   ([gates taken as closed](#gates-taken-as-closed)).
2. **The name is checked** (rule 2), before the run is claimed, so a refusal publishes
   nothing: no run record, no session, no socket. Skipped where rule 4 already leaves the
   tool out.
3. **The run is claimed** as today.
4. **The capability and the endpoint come first**, before the plan: the endpoint listens
   on the run's context socket. Either failing, the plan injects nothing (rule 4).
5. **The plan adds the tool**: the executable (`os.Executable`, resolved once),
   `bridge-serve`, the environment `REWAKE_DIR`, `REWAKE_ROOM`, `REWAKE_SESSION`,
   `REWAKE_EPOCH`, `REWAKE_BRIDGE_CAPABILITY`, and each harness's additions below.
6. **On Codex, the startup probe checks the injection** — that our entry is ours alone
   and what the read conditions are — the injection check, which the gateway repeats at
   every thread, apart from the proof that the name was free.
7. **The harness starts**; the channel record follows
   ([mail-bridge-channel.md](mail-bridge-channel.md)).

## Codex

What the launch adds, the name check by a separate app-server before the claim, the
injection check at start and at every thread with its step 0 for the request itself, and
the output limit are in [mail-bridge-launch-codex.md](mail-bridge-launch-codex.md).

## Claude Code

**What is added.**

- `--mcp-config <path>`, the file `sock/<name>.<epoch>.mcp.json` written 0600 for the run
  and removed with it, holding only `{"mcpServers": {"rewake": {...}}}` with the
  command, `["bridge-serve"]`, the environment and the server's own `timeout` of
  30000 ms, which the documentation says takes precedence over `MCP_TOOL_TIMEOUT` (gate
  G8); a file keeps the capability off the command line.
- `--allowedTools mcp__rewake__rewake`, a flag of its own beside whatever the caller
  passed under either spelling (probe 2: both apply). `Bash(rewake:*)` keeps its own
  condition, since the shell stays the fallback.
- In the one settings layer ([launch.md](launch.md#claude-code)), PreToolUse, PostToolUse
  and PostToolUseFailure entries matching `mcp__rewake__rewake`, each running `rewake
  bridge-hook <context socket>` in the foreground with a timeout of 5 s, after the
  person's own.
- No `--strict-mcp-config`; a caller's stays, and the harness loads their explicit set
  plus ours (probe 2).

**No tool** (rule 4), with a launch note: the caller's `--settings` cannot be read or
merged, so no hook would observe a call; `--bare`, whose hooks are unverified; a managed
MCP file (below); the short `-w`, Claude Code's own worktree, whose directory and
checkout the harness picks after the launch (*live*,
[research-launch.md](research-launch.md#a-worktree-at-launch)), so no check can run there
first. Every spelling of `-w` the harness's parser takes counts — alone, its value as the
next word or joined, and `w` among the letters of a group of short flags, which
over-covers rather than misses; a test lists them against the parser of the installed
version. rewake's own `--worktree` is not this: rewake makes that checkout and moves
into it before the launch, so the check runs there.

**A managed `managed-mcp.json`** (*revised after the live checks; built*, `claude/managed.go`) makes
the harness refuse every `--mcp-config` and exit 1 (*live*, 2.1.284), so ours would stop
the launch. Only the file's existence is asked, by `lstat` in the harness's managed
directory — `/etc/claude-code` on Linux, `/Library/Application Support/ClaudeCode` on
macOS (*bundled source*). Present in any form — a link, a directory, unreadable or
unparsed, which the harness treats as in control too (*bundled source*:
"unusable-managed-mcp") — gives no tool, reason `a managed MCP configuration is
present`; only `ENOENT` proves it absent, and any other error gives no tool with `the
managed MCP configuration could not be checked`. A WSL policy chain under which the
harness skips `/etc/claude-code` over-covers. A caller's own `--mcp-config` beside the
file still stops the harness; rewake only adds no refusal of its own.

**The name check, before the claim.** `<program> mcp get rewake`, in the launch
directory, with the harness's environment (`CLAUDE_CONFIG_DIR` included), bounded at
10 s; and rewake's own reading of every `--mcp-config` value. *Verified:* absent, it exits
1 with `No MCP server named "rewake".`; present, it exits 0 with a `Scope:` line. **A
present, approved entry is started once by the check's health check**, as `claude mcp
list` starts every server, which is why `list` is never used. Its output prints the
entry's environment; rewake reads the `Scope:` line and discards the rest unprinted.
Only the line's first word names the scope (*verified*: `User config (available in all
your projects)` carries the word `project` among the words after it); a first word
outside the vocabulary is shown as `a scope rewake does not name`.
Whatever the check started is ended and reaped as on Codex, by the check's holder
(rule 6): a server that left the check's group or session is still in the holder's tree.

| Source of a name | Covered by | Known how | Unknown, and the action |
|---|---|---|---|
| user scope | `mcp get` (`User config`) | *verified* | — |
| local scope, keyed by the directory | `mcp get` (`Local config`) | *verified* | — |
| `.mcp.json` of the launch directory, approved, pending or rejected | `mcp get` (`Project config`) | *verified* | — |
| `.mcp.json` of a parent directory | rewake: every parent up to `/`, read only | — | **gate G5**: whether the harness reads it; until it closes one naming `rewake` refuses, and one that cannot be read or parsed refuses as a check that failed (main's decision of October 4, 2026) |
| managed `managed-mcp.json` | rewake: whether the file exists, never its content | *live* on 2.1.284 (October 4, 2026): present, it takes exclusive control, `mcp get` shows only its servers, and a launch with `--mcp-config` exits 1 | present in any form: no tool, no name check (G6; *revised, built*) |
| the caller's `--mcp-config` | rewake: every value of every occurrence | *probe*: repeated flags add | — |
| `--setting-sources` | `mcp get` reads every source; one left out still refuses | — | over-covers, never under |
| plugins' servers | — | names carry the plugin's prefix | **gate G7**: that no plugin server can be named `rewake` bare; until it closes, no tool, with a note naming the gate |
| connectors of the account | — | *probe*: named `claude.ai <name>` | — |
| the directory of `-w` | — | *live*: chosen by the harness after launch | no tool |

**Reading `--mcp-config`.** Its values are a list (`<configs...>`): every word after the
flag up to the next word starting with `-`, the `--mcp-config=<value>` form, repeated
flags, and nothing after `--`. A value is a file, read from the launch directory, or
inline JSON; its `mcpServers` must not name `rewake`. A value that cannot be read or
parsed refuses, as the harness would fail on it. `harness.FlagValues` takes one word per
flag and is extended for list flags, not reused as it is.

**The limits, per call** (rule 5). *Verified* on October 1, 2026 (2.1.284, scratch home,
no model turn): values from the user settings' `env` and the caller's `--settings` `env`
reached a SessionStart hook and the stdio server, over the wrapper's own. That shows
where the values travel at start — not which value the harness applies to a given call,
nor whether a settings change reaches a running session; both are **gate G8**.

`rewake bridge-hook` sends the raw `MCP_TOOL_TIMEOUT` and `MAX_MCP_OUTPUT_TOKENS` of its
own environment with each PreToolUse observation, and again with the call's PostToolUse
or PostToolUseFailure:

- **timeout**: unset is the harness's default. A value of decimal digits alone, read as
  whole milliseconds, gives the deadline: the lesser of 25 s and the value less 2 s, so
  7000 gives the least working deadline, 5 s. A value under 7000, a sign, a fraction,
  other text or a number past 64 bits refuses that call before its ticket, naming the
  setting — a refusal of one call, not a block and not evidence about the transport.
  Once G8 proves the server's own `timeout` binds, the deadline is 25 s as on Codex.
- **output**: unset is the default probe 2 calibrated, and the call may acknowledge a
  read; a whole number at or above the bound L5 calibrated for the running version —
  2048 for 2.1.284 — leaves it so (*revised after the live checks; built*,
  `endpoint.Config.OutputBound`), and
  any other value, or any value under a version with no bound, turns acknowledgment off
  for that call; a value that is not a whole number counts as lowered. Over the limit
  the harness replaces the whole result with an error rather than cutting it (*live*),
  so a smaller value loses the answer, not a tail of it.

A call acknowledges a read only when its two snapshots agree, and only once G8 has shown
that the PreToolUse value is the one the harness applies to that call's result. If G8
finds that the harness takes the value once at start, a SessionStart entry is added, its
snapshot holds for the run, and a call whose own differs neither acknowledges nor gets
more than the lesser deadline. Tool reads stay refused on Claude Code until then, and
until a live check shows which hook field a nested agent's call always carries
([the turn a call belongs to](mail-bridge-turns.md#the-turn-a-call-belongs-to)).

## The refusal, and what may be shown

Exit 1, before anything is published, on stderr:

```
rewake: not starting: you already have an MCP server named rewake (<where>).
rewake adds its own server under that name for this launch, and the harness would
merge the two or keep only one. Rename or remove your entry, then launch again;
rewake changes none of your files.
```

**What may appear in any diagnostic** — stderr, the run's logs, launch notes, session
state and mail notices — is a closed list:

- the program's base name and the check's fixed label (`mcp get`, `config/read`);
- an outcome class: exit code N, no answer within its bound, an answer not recognized,
  could not start, could not be ended, could not be read (a file rewake reads itself);
- the name of a scope or layer from a fixed vocabulary (`user`, `local`, `project`,
  `managed`, `session flags`, `packaged defaults` — Codex's effective table with no layer
  naming it — and `a scope rewake does not name`), and the path of its file;
- an argument's position and flag name — `-c` argument 3, `--mcp-config` value 2 — and
  for `-c` the key cut to `mcp_servers.rewake`, never further and never its value.

Never the output of a check, stdout or stderr; never an argument's value, inline JSON or
a whole command line; never a parser's error, which may quote what it read. When absence
could not be established, the refusal says which check, its outcome class, and a
template to run by hand — `claude mcp get rewake`, or `codex mcp get rewake` with the
same `-c` values — without repeating them.

## Failure points of a launch

| Point | Proven | Unknown | Rewake does |
|---|---|---|---|
| every covered source answers absent, no gate open | the name free in the covered sources, then | a file edited before the harness reads it | injects |
| a source whose gate closed without a check | — | whether it names `rewake` | no tool wherever it may apply |
| a covered source names `rewake` | the person has one | — | refuses, naming where |
| a check exits otherwise, answers unrecognized text or passes its bound | — | whether the name is free | refuses, with the check, its outcome class and the template |
| a check's process cannot be ended or reaped | — | what it still runs | refuses, naming the check |
| a caller's `--mcp-config` unreadable | — | what it holds | refuses, naming the value's position |
| `-w` in any spelling, `--bare`, unmergeable `--settings`, `--no-mail-tool`, a managed `managed-mcp.json` | the tool could not be checked, observed or added | — | no tool, with a note |
| the harness version unknown (Claude Code: no `claude` on `PATH` or a path naming no version; Codex: an unreadable `--version`) | — | which gates hold for it | every gate open: no tool, with a note |
| Codex: `initialize`'s `userAgent` names another version than the read, or none | — | which gates hold for it | the choice made again without a version: kept when every gate it needs is assumed, else withdrawn — no tool, with a note ([the version, confirmed at start](mail-bridge-launch-codex.md#the-version-confirmed-at-start)) |
| G2 open (Codex), G7 open (Claude Code), G4 open with requirements naming MCP servers | — | a source of the name no check covers yet | no tool, with a note naming the gate; never a refusal |
| the endpoint or the capability fails | — | — | no tool, with a note |
| Codex: our effective entry holds a key not ours | something merged | from where | refuses; the record removed as after a failed start |
| Codex: a thread whose directory or registrations hold another `rewake` | the person has one there | — | that request refused; the session goes on without it |
| Codex: a thread's cwd unknown, or its check fails or passes 5 s | — | whether the name is free there | that request refused, nothing forwarded |
| Codex: a thread request's `config` holds a key off the list, or `features` unlike the caller's | — | what that thread would load | that request refused, nothing forwarded |
| Codex: a thread request names a cwd with no trust decision | — | whether the app-server would trust it and load its project layer | that request refused; no trust written by rewake |
| Codex: `omit_tools_from` not in effect, an output limit set | reads not shown direct or whole | — | tool reads off |
| Claude Code: a call's timeout or output limit unknown or low | — | what the harness does with that call | the call refused, or its read not acknowledged |
| a file changes between the check and the harness reading it | — | whether the name is still free | Claude Code may replace the person's server; Codex's gateway checks again at each thread |
| the check starts the person's `rewake` (Claude Code) | it ran once, under the person's own harness | its side effects | refuses, saying so |

## Gates

Each is a live or source check of the stage's plan, with sentinels that write when
started; the stage is accepted only with each closed, and how each is run is in
[mail-bridge-live.md](mail-bridge-live.md). A gate is closed in code only by an entry in
`closedGates` (`internal/harness/gates.go`) naming the harness and the versions its check
ran against; a version not named keeps it open. The table holds G1 for codex-cli
0.159.0 and G5 and G7 for Claude Code 2.1.284, closed by the live checks of October 4,
2026 ([the run](mail-bridge-live.md#the-run-of-october-4-2026)); every other launch takes
the action of each open gate, given below in parentheses, with the run's answer after it.

Which version a launch takes for this table, and what an unknown one means, is in
[mail-bridge-version.md](mail-bridge-version.md).

- **G1** no MCP server starts in the check's app-server without a thread. (Nothing of
  its own: the check's app-server runs only once G2 is closed.) *Closed for 0.159.0.*
- **G2** a Codex call that lists the registrations a thread request will have — the
  plugins it selects, extensions, hosted apps, compatibility servers — without starting
  them. (No tool on Codex, with a note naming the gate; no check runs.) *Open: the source
  of 0.159.0 has no such call.*
- **G3** the cwd of a Codex resume or fork whose request names none. (Such a request is
  refused.) *Answered on 0.159.0: the thread's recorded cwd; open until the gateway checks
  that one rather than the server's.*
- **G4** Codex requirements on MCP servers, by their exact fields. (Requirements with any
  key naming MCP and a value set give no tool, with a note; taking G4 as closed does not
  change that, since which fields admit ours is what the gate settles.)
- **G5** a parent directory's `.mcp.json` and `mcp get`. (rewake reads every parent's
  file itself and refuses on one naming `rewake`.) *Closed for 2.1.284: the harness reads
  a parent's file at any depth, across a git root.*
- **G6** `managed-mcp.json` and `mcp get`. (Nothing of its own.) *Answered on 2.1.284:
  the file makes the harness refuse any `--mcp-config` and exit 1, so a launch with our
  server does not start.* For the name check it leaves nothing to check — no server of
  ours can be added beside it — so no tool and no `mcp get` (above); the gate closes for
  a version once a launch with the file present starts without the tool, live.
- **G7** that no Claude Code plugin server can be named `rewake` bare. (No tool on
  Claude Code, with a note naming the gate.) *Closed for 2.1.284: every plugin server is
  keyed `plugin:<plugin>:<server>`.*
- **G8** which `MCP_TOOL_TIMEOUT` and `MAX_MCP_OUTPUT_TOKENS` Claude Code applies to a
  call: the values at PreToolUse and at the result, set in user, caller, project and
  managed settings, and changed in the same process between calls and during a long one;
  whether the harness takes them once or per call; whether our server's own `timeout`
  binds. A SessionStart observation proves none of this. Its user and managed parts run
  where those layers may be written — a scratch configuration directory, a container —
  against a stand-in API that answers with a call of our tool, never in the person's
  configuration ([mail-bridge-live.md](mail-bridge-live.md#the-cases)). A part that
  cannot be run stays unproven, and a run where that layer's `env` names either variable
  — the launch reads the names only — acknowledges no read on Claude Code; a managed
  settings file present counts as naming them. (No call acknowledges a read on Claude
  Code; tool reads there stay refused regardless, as above.) *Partly answered on
  2.1.284: our server's own `timeout` binds over `MCP_TOOL_TIMEOUT`, and the caller's
  `env` limits apply; the other layers and a change during a session not run — open.*
- **G9** the parameters of a Codex thread request that reach its layers, the injection,
  the limits or the registrations — `config`, `cwd`, `sandbox`, `permissions` and the
  trust branch they open — read in the source of each version accepted: the key Codex
  forms for a cwd's trust, and that no other path changes the active layers. Until it
  closes for a version, step 0 refuses what it does not cover. (A thread request naming a
  cwd is refused.) *Not closable as built on 0.159.0: a request's `config` reaches the
  `sessionFlags` layer unfiltered, and `selectedCapabilityRoots` adds plugin
  registrations.*
- **L4** the native signal of a policy denial of our tool, on each harness. (No block is
  ever set; a denied call is only the harness's own answer to the model.) *Open: no
  person-reachable denial on Codex 0.159.0, no distinct hook event on Claude Code 2.1.284.*
- **L5** the smallest output limits that still deliver a 4 KiB result whole. (Under a version
  without a bound, any output limit set turns acknowledgment, or on Codex tool reads, off.) *Answered: 861 for
  `tool_output_token_limit` on 0.159.0, 2048 for `MAX_MCP_OUTPUT_TOKENS` on 2.1.284.
  Revised and built: the code takes them per version from `outputBounds` beside
  `closedGates` ([the harness version](mail-bridge-version.md)).*

### Gates taken as closed

The live checks run a launch with the tool where an open gate would keep it out, so a
launch can take named gates as closed: `REWAKE_GATES_ASSUMED`, a comma-separated list of
names from the table above (main's decision of October 4, 2026). It is for verification
only — a live check of the gate itself or of a case behind it — and appears in no
briefing and no help example.

- Off by default; only the table's names are taken, and any other name refuses the
  launch with exit 2, listing the gates.
- Each assumed gate is said on stderr at launch, before the check: `rewake: taking gates
  <names> as closed for this launch (REWAKE_GATES_ASSUMED, for verification only)`.
- The session record keeps the list; `whoami` and `rewake list` show `Gates taken as
  closed for verification: <names>.` for that run.
- A gate taken as closed skips only its own action: the checks that cover a source still
  run, and every other open gate keeps its action.
- A gate the live task closes is recorded in `closedGates` for the harness version it ran
  against; the variable is then needed only for gates still open.
