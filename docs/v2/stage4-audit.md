# Stage 4 audit: what Claude Code and a mod already do

Stage 4 starts with this audit, before the probe and before any adapter code (owner's
decision, October 12, 2026, restated October 13;
[design-claude.md](design-claude.md#before-the-probe-what-claude-code-already-does)).
Every feature rewake has on the Claude Code side is weighed against what the installed
Claude Code and a mod give natively, and only what survives goes into the adapter or the
probe. Written October 13, 2026, on Claude Code 2.1.296 with the `mods` skill taken from
the same version.

The owner's aim, October 13: look with Codex and other harnesses in sight too — for
example a worktree Claude Code makes, any harness seated in it, and Claude Code managing
its whole life. And the owner's ruling of the same day: working features over
super-guarantees — prefer the design that drops rewake's code for a native mechanism,
unless a property [project.md](../project.md) states is lost, and then name it. Also that
day the owner decided that the next harness after Claude Code is pi, and that Codex gets
only an entry point before 2.0, its adapter after the release: the rows weigh pi first and
keep Codex's reading for later.

Everything here is a proposal. The verdicts, the new probe rows and the changes to the
stage plan are main's to decide, and the owner's where `project.md` is touched. Each row
is argued in [stage4-audit-rows.md](stage4-audit-rows.md) (rows 1–12) and
[stage4-audit-rows-space.md](stage4-audit-rows-space.md) (rows 13–22); the pi side's and the Codex
side's own readings of the same rows are folded into each row's paragraph on the other
harnesses.

**Where each fact comes from**, marked in the rows: *help* — `claude --help` and
`claude <command> --help` of 2.1.296, read October 13, 2026; *skill* — the `mods` skill,
snapshot 2.1.296 (its facts are cited, not copied); *docs* — Claude Code's documentation
pages on cross-session messaging, worktrees, background sessions and hooks, read October
13, 2026, which are not versioned and name versions up to 2.1.290; *rewake* — this
repository's own documents and code. The audit's writer ran nothing live: that is the
probe. The one live fact it cites is main's dated observation of worktree subagents in
row 13, recorded with its version.

## The target: a space

The owner set the target on October 13, 2026, and rows 1, 13 and 16 serve it. Main, a Claude
Code session, creates a **space**: a pseudo-terminal rewake owns, shown as a tab in main's
Claude Code. In the space a worktree may be made, and then another session is seated there,
a Claude Code session first and a session of another harness later. Main creating spaces and
seating workers in them is the owner's direction; that answers row 16's "a new authority",
and its limits — which main, how many spaces, which directories, what the person sees and
may stop — are designed in stage 4 and stage 6.

**What a space is made of.**

- *The terminal*: the **space's terminal process**, a rewake process, owns the
  pseudo-terminal and runs the worker in it. Whether it is the worker's own wrapper, the
  detached process of the mod's join (row 1) or a process of its own per space stays open
  until P11 and P15 decide; this audit calls it by this one name meanwhile. It serves its
  space alone and lives as long as what runs inside (row 16).
- *The tab*: preferably Claude Code's own (owner's idea, October 13, 2026). A subagent gets
  an entry in Claude Code's tasks list, and opening the entry opens that agent's transcript.
  With a subagent of main standing for the space — the worktree's keeper below — the tasks
  list is the tab bar, and opening the space's entry replaces the subagent's view with
  rewake's own: a pseudo-terminal in which the real harness runs with its own screen. Claude
  Code is not asked to show another harness's transcript by its own means; rewake draws only
  the terminal, in one of two places (*skill*, the render sites and elements):
  - *the keeper's own transcript*, first: it holds little but the keeper's pending call of
    the mod's blocking tool, and a mod may redraw transcript rows, so that tool's row is
    redrawn as the space's terminal — a cell grid repainted by `$.ui.blit`, with a client
    element taking keys;
  - *the pane or the band*, second: switching the view to the agent runs the mod's pane and
    band hooks again with that agent in view, so the mod draws the terminal in its pane
    beside the transcript or in the band above the prompt.

  A tab of the mod's own drawing is the fallback if P15 shows neither carries it. A reload of
  the mod or main's exit loses the view, not the space (row 16).
- *The worktree*, optional, is made and removed by Claude Code even when another harness
  sits in it (owner's direction, October 13, 2026; row 13). A Claude Code worker makes its
  own with `-w`. A worker of another harness — pi, and later Codex — is seated by cwd in a
  checkout a Claude Code **keeper** holds: a background session started with
  `claude --bg -w`, or a subagent of main whose isolation is "worktree". P12b weighs the two;
  rewake keeps no worktree lifecycle of its own once one of them holds. A Claude Code worker
  started with its own `-w` has no subagent behind it, so its space has no entry in the
  tasks list unless one stands for it too; seating it in a keeper's checkout like any other
  harness is the plain answer, weighed with P12b and P15.
- *The worker*: the program the space's terminal runs — `rewake claude` (a wrapper, or the
  thin launcher of the mod's join, row 1), and in stage 4b `rewake pi`.

**rewake on Claude Code is the mod and the binary.** The person sees rewake as a mod: the
tab's content, the buttons, the tools and the events are the mod's. But a mod cannot hold a
pseudo-terminal — `$.process.spawn` gives none and a one-time stdin, there is no Node, and a
module's children die when it reloads (*skill*; the [pi](stage4-reading-pi.md) and
[Codex](stage4-reading-codex.md) readings reach the same limit from their side) — so the
space's terminal process holds it, and the mod draws its screen and forwards keys. rewake is
then the mod, its face inside Claude Code, and the rewake binary, which does what a mod
cannot: the pseudo-terminal, the endpoint, the state under the room's lease, processes that
outlive a reload, and every harness without a mod — pi through its extension, Codex through
its wrapper.

**What the probes must show for it.**

- **P11**: a Claude Code session started inside a terminal rewake owns joins the room, by its
  wrapper or by the mod's join; the terminal's owner is not the run's anchor, and a reload of
  the worker's mod keeps it in the room.
- **P12**: a `-w` from a Claude Code worker seated in a space: where the checkout lands, how
  rewake learns its path, and whether its removal waits while the worker sits there, on every
  route Claude Code removes by.
- **P12b**: a keeper holds a checkout for a worker of another harness — started without
  spending a model turn and idling cheaply, its path known to rewake, its removal refused while
  the worker is inside, the worker's Git writes allowed.
- **P15**: the space's terminal drawn where the keeper is opened from the tasks list — first
  in the keeper's own tool row, whether a transcript row may carry the cell grid and the
  client element, take keys, get a usable height in the agent's view with fullscreen on and
  off, be repainted by `$.ui.blit`, scroll and hold focus; then in the pane or the band,
  whether it follows the view switch, may be opened on it, and the size it gets — and in
  either the keys a session needs, Escape among them; a reload of main's mod and main's exit
  leave the space and its worker running.

**If a probe fails.** If the tasks list cannot carry the tab, the mod draws a tab of its own.
If P15 fails as a whole, the minimum path keeps everything but the tab: main still creates the
space, the worktree and the worker, rewake still owns the terminal, and the person attaches to
it from a terminal of their own — a command stage 4 designs. If owning the terminal fails too,
the path is today's: the person starts the worker in their own terminal with
`rewake claude`, and main seats nothing. If P12b fails, rewake keeps one worktree lifecycle of
its own that follows Claude Code's conventions (row 13).

### Two levels of UI

The owner noted on October 13, 2026 that the interface still needs thought: in Claude Code
the subagents' entries stay visible even inside one subagent's view, so the person can
switch, and with a full harness running inside a space the person meets two interfaces, one
inside the other. A proposal for stage 4's design, not a choice:

- **The outer level is main's Claude Code.** Its tasks list is the list of spaces, beside
  main's own subagents. rewake controls only what its mod draws there: the space's entry
  where the mod may shape it, and the terminal in place of the keeper's view.
- **The inner level is the seated harness's own screen**, unchanged and not rewake's. Its
  own subagents, tabs, panels and keys stay its own; rewake neither draws over them nor
  folds them into the outer list.

The questions it raises are P15's: which keys the outer level keeps — switching between
agents, Escape back to main's prompt — and how the inner harness still gets them, by a
remap or a button; how many rows and columns the inner screen gets under the outer chrome,
with fullscreen on and off; whether two nested tab bars confuse the person; and what the
space's entry shows — its name, harness and state — so that the outer list stays readable.
The choice is stage 4's design, after the probe.

## The central question: Claude Code to Claude Code through the mod alone

The owner asked (October 13) whether work between Claude Code sessions could run entirely
through the mod, with no mandatory `rewake claude` wrapper: the person loads the plugin,
the mod joins the session to a room, and the mod — official code inside Claude Code —
adds what the harness never had.

### (a) What the wrapper does today, and whether the mod can take it over

| The wrapper's job today | Through the mod alone | What is lost |
|---|---|---|
| **The run's anchor.** A run's identity is the wrapper's pid, start time and boot; every shell command proves which session it is by its chain of parents reaching that wrapper, and a decision's final transaction runs in the verified wrapper ([stage3-decision.md](stage3-decision.md), `core/registry/run.go`) | split in two: the harness process becomes the **ancestry anchor** — a model's commands run below it as they run below the wrapper today — while the endpoint, the lease and the decision's final transaction move to the detached rewake process (next row), which no model command descends from | the proof the decision rests on. Today one process is both: the caller's chain of parents reaches the run's process, and the connection's peer and the final writer are that same process ([stage3-decision.md](stage3-decision.md#authority), points 1–4; `authority.Decide`). Split, the detached writer fails the peer check against a harness anchor, and a model's shell fails the ancestry check against a detached anchor. Moving the run's anchor field does not carry the proof over; only a replacement authority contract would, and none exists (the obligations below) |
| **A process outside the harness's tree.** The wrapper is started before the harness and is not its descendant, so no model's command can have preceded it or stand in for it | the detached rewake process, started by the mod's first `session.start`, which the skill says is awaited before the first prompt — but only when the harness process starts: a later enable, reload or respawn runs its hooks alone (skill), so a join after model commands ran has no such ordering. It must outlive reloads, since a `$.process.spawn` child dies when its module unloads or reloads (skill) | if that process is a plain child of the mod, every reload opens a gap in which a model's command could bind its socket first and answer the mod as the host. Detaching at the first start removes the gap only if the mod can also tell its own process from a substitute, which detachment alone does not prove; whether a child of `$.process.run` may detach, and whose child it is, is P11 |
| **The build lease** of the room, held shared from the build check to the last effect ([state.md](../rules/state.md)) | held by that detached rewake process | nothing while it lives; that it stops writing when the harness ends is one of the obligations below |
| **Registration and the name**, unique in the room | the mod's first act asks the rewake process to register; the name comes from an environment variable or a slash command the mod registers (`$.command.register`, skill) | nothing |
| **The role briefing** by `--append-system-prompt` for one launch | `prompt.section` or `prompt.context` events (skill) are candidates; both are cached — a section's answer until the mod invalidates it, the context once per conversation until invalidated or re-read (skill) | none known; whether either carries a briefing changed between launches into a resumed conversation, with the system prompt's snapshot on (help, `--system-prompt-snapshot`), and what invalidation that needs, is P14 |
| **Launch flags**: the inbox socket path (`--messaging-socket-path`, hidden), `--allowedTools Bash(rewake:*)`, the model and effort defaults, `--plugin-dir` | the socket path is exported to hooks and children as `CLAUDE_CODE_MESSAGING_SOCKET` (docs), which the mod reads with `$.env.get`; model and effort are the person's own flags; the shell allowance is not needed for the mod's tools, which skip the permission check when the mod answers them | the shell allowance: a session started without it asks the person before each `rewake` command, and the plugin itself must be loaded by the person (below) |
| **Grants at launch** (`--add-dir` for a resumed conversation) | stage 6's mod applies grants at run time | nothing new: grants on Claude Code wait for stage 6 in any design |
| **The room's bootstrap** under the lease | done by the detached rewake process | nothing |
| **Delivery when the mod did not load** — the wrapper still wakes the session through the inbox socket and tells main the session runs without its mod | none: with no mod there is no rewake process and no session in the room | the degraded mode goes; the session is either joined whole or absent. Answer 3 of [design.md](design.md#the-owners-answers) already refuses a reporting role without the mod, so only a main without its mod loses |
| **The terminal**: the wrapper is the harness's parent and passes the terminal through | the person runs `claude` themselves | nothing; rewake stands even less between the person and the terminal |

**What the join leaves unproven.** The table moves the wrapper's jobs; it does not move the
wrapper's authority. Three obligations stay open, P11's to settle and a replacement authority
contract's to answer, before the join can stand in for the wrapper:

1. **The caller-to-final-writer proof.** [stage3-decision.md](stage3-decision.md#authority)
   holds one process as both the ancestry anchor a caller must run below and the connection's
   peer that makes the final transaction. With the harness as anchor and the detached process
   as writer, nothing yet proves to a caller that the writer answering it belongs to the run
   its ancestry reached. P11 as written checks the other direction, the detached process
   authenticating the mod's requests.
2. **A late opt-in.** A join after model commands ran — `/rewake join` typed mid-session, or
   the plugin enabled late — starts the detached process after commands that could have bound
   its endpoint first; the ordering of the first `session.start` holds only at the harness
   process's start (skill).
3. **The anchor's death during an admitted decision.** Today's wrapper refuses new requests
   from the moment its harness ends and finishes a taken transaction before its last write
   ([stage3-decision.md](stage3-decision.md#authority), point 4). A detached writer waiting
   for the mailbox lock could finish a transaction after the harness died; saying it ends
   with the harness does not give the same-lifetime property.

This audit proposes no mechanism for them. **The wrapper stays the fallback for Claude Code
unless a replacement authority contract is accepted**, by main and with its rules and tests
written before code.

### (b) How a session would join without the wrapper

The plugin reaches the session one of two ways: the person installs it (user or project
scope, an ordinary plugin), or a launch passes `--plugin-dir` for that run. An installed
plugin loads in every session of its scope, so the mod must stay idle until asked: a
variable set for the launch (`REWAKE_ROOM`, a name) or a slash command the person types
(`/rewake join <name>`) — the second a late opt-in, under obligation 2 above. Joining, the mod starts the detached rewake process with
`$.process.run`, which registers the session, takes the lease, binds the endpoint in the
run directory and lives until the harness process ends. The mod then talks to it as
[design-claude.md](design-claude.md#how-a-tool-call-runs) designs, over the run's socket
with `$.http.fetch`, each request checked by its peer; whether that peer is the harness
process is P4, as before. rewake still runs no service of its own: the process serves one
session and lives exactly as long as it.

### (c) The project's properties

- **The person's configuration is never edited.** Kept if rewake never installs the plugin
  itself: an installation is the person's own act, as installing rewake is. A launch with
  `--plugin-dir` keeps it by construction (L1).
- **Never intervenes in a session the person started.** A session that loads the plugin and
  joins has been asked in by the person — by installing it and joining — so the mod acts
  inside a session it was invited into. The property holds if joining is always the
  person's act, never the mod's default: an installed plugin that joins every session
  would break it.
- **No service beside the sessions.** Kept only by the detached process living as long as
  its session and serving it alone; a process shared by sessions would break it.
- **The core knows no harness.** Kept: the anchor and the writer are fields of the run, not
  a harness name, whatever contract binds them.
- **One build per room.** A mod is JavaScript generated per build today; an installed
  plugin is whatever version the person installed, so the mod's hello must carry the build
  it expects and the rewake binary it starts must be that build, or the join is refused.

### (d) Claude Code's own messaging as the carrier

Claude Code's `ListAgents` and `SendMessage` reach other local Claude Code sessions by
name over each session's inbox socket (docs). As the carrier of rewake's mail they lose
properties `project.md` states: a session not running is not delivered to (skill,
`SessionSendAddress`), so a message is not kept for later; at most 100 held and 50
queued messages, identical repeats dropped (docs); no kinds, no owed report, no pending,
no withdraw or edit, no proof the text was read; the sender's plugin name is a claim
"never key a guard on it" (skill, `SessionReceiveOrigin`); and no harness but Claude
Code. They serve as a **wake** carrier, which they already are: the 1.x notice line goes
over the same inbox socket. The mail stays rewake's (row 4).

### pi and Codex in such a room

pi loads in-process TypeScript extensions per run with `-e <path>` (*help*, pi 0.85.1), so a
pi session can join the same two ways: through a `rewake pi` wrapper, or through an
extension the person loads, under the same anchor question as the mod. Codex has no
in-process module, so a Codex session joins through `rewake codex`, its wrapper and its
app-server connection, as stage 5 designs; the Codex side's reading finds a join without
the wrapper possible to investigate but not proven, since neither a hook nor an MCP server
alone holds membership for a session's life. A mixed room then holds runs anchored two
ways — a wrapper, or a harness process joined by its mod or extension — and the core sees
only runs with anchors. Nothing else in the room changes.

### Proposal

**Both paths, the mod's join as the target for Claude Code.** The wrapper stays for Codex
and for every harness without an in-process module, and for pi until its own probe weighs
the extension's join. For Claude Code, `rewake claude` becomes
a thin launcher — it starts `claude` with `--plugin-dir` and the join variables and gets
out of the way — the harness process anchors the run's ancestry, and the detached
per-session process the mod starts serves the run in the wrapper's place: the endpoint, the
lease and the final writer. A person's own `claude` session with the plugin installed joins
by command. The cost is a core change — the run's anchor split from its writer, under a
replacement authority contract that answers the three obligations above — and the risk is
P11: if a child of the mod cannot detach and outlive reloads, or its parent is not the
harness process, or no contract answering the obligations is accepted, the wrapper stays
for Claude Code too, exactly as designed today. P11 runs first in the probe, since it decides the shape of the adapter
(decided by main, October 13, 2026); the choice between the paths waits for its answer.

## The verdicts

KEEP is rewake's own, harness-neutral core; NATIVE hands the feature to Claude Code's;
MOD builds it on a mod event or call; SPLIT divides it; OPEN needs the probe row named.
Each row is argued under its number in [stage4-audit-rows.md](stage4-audit-rows.md), rows 1–12,
and [stage4-audit-rows-space.md](stage4-audit-rows-space.md), rows 13–22.

| # | Feature | Verdict | In one line |
|---|---|---|---|
| 1 | Joining: wrapper or mod | OPEN, P11 | both paths; the mod's join for Claude Code if a detached child outlives reloads and a replacement authority contract is accepted, the wrapper otherwise |
| 2 | Launch: name, room, briefing, plugin, defaults | SPLIT, P13, P14 | the registry stays; the name also goes to `--name`; the briefing may move to a prompt event |
| 3 | The shell allowance `Bash(rewake:*)` | KEEP | the command line is first-class; the mod's tools need no allowance |
| 4 | The mail: kinds, inbox, owed, one report, withdraw and edit | KEEP | native messaging keeps nothing for a session that is not running and knows no reports |
| 5 | Native `SendMessage` inside a rewake session | MOD, P13 | a `session.send` guard turns a report sent past rewake back to `rewake` |
| 6 | Reading mail by tool | MOD, P2 P4 P8 | `$.tool.register`, as designed |
| 7 | Turn boundary and the confirmation | MOD, P3 | `classic.Stop` with `block`, `turn.complete`, as designed |
| 8 | Telemetry | MOD, P6 | `session.measure`, `$.session.usage()`, model and effort from the mod |
| 9 | Wake and delivery | NATIVE or MOD, P1 | Claude Code's inbox socket is already the carrier; the hidden socket flag can go |
| 10 | Compaction and interrupt | MOD, P7 | `$.session.compact`, `$.turn.abort`; native messages run no command |
| 11 | /clear and resume binding | MOD, P9 | resume is Claude Code's; the binding follows `session.end` and `classic.SessionStart` |
| 12 | Grants | SPLIT, stage 6 | the authority stays rewake's; the application moves into the mod |
| 13 | Worktree | SPLIT, P12, P12b | Claude Code makes and removes every space's checkout, any harness inside; a keeper — a background session or a subagent of main — holds one for a foreign worker; rewake's own lifecycle only as the fallback |
| 14 | The room view | KEEP | `rewake list`; activity and context come from the mod, not `claude agents --json` |
| 15 | Permissions and the person's UI | MOD, stage 6 | as designed |
| 16 | Worker tabs and buttons | SPLIT, P15 | the pseudo-terminal is rewake's, any harness inside; Claude Code's tasks list is the tab bar, the mod draws the terminal and forwards keys |
| 17 | The room panel | MOD, stage 6 | panes and buttons over `rewake list` and `inbox --owed` |
| 18 | Headless and background workers | OPEN, P16 | Claude Code's background supervisor does not fit the wrapper's lifetime; not scheduled |
| 19 | Timers, monitors, idle notices | KEEP | session-internal tools; rewake's notices stay |
| 20 | `$.store` as room state | KEEP | per machine, not per room or build |
| 21 | Variables a session exports to children | KEEP | still scrubbed before a launch |
| 22 | The version gate | KEEP | the adapter's minimum; the mod's hello adds `$.session.version()` |

## Consequences for the stage plan

Written as proposals for [the stage plan](design.md#the-stage-plan-from-here); where main
or the owner has decided one, the item says so with its date.

- **Stage 4 is ordered to serve the space** (owner's target, October 13, 2026). Its probe
  opens with P11 (row 1; decided by main, October 13, 2026), with P12 and P15 beside it
  (P15 by main's decision of the same day): the three the space depends on. P11's answer
  picks the adapter's shape — wrapper plus mod, or thin launcher plus mod plus a detached
  per-session process — and P1–P10 hold under either, P4 most of all. If the mod's join is
  taken, stage 4 also carries the core change — the run's anchor split from its writer, and
  the authority contract that replaces the same-process proof — with its rules and tests
  before code, and `rewake claude` shrinks to the launcher. P12b, the worktree's keeper, runs
  with P12.
- **Stage 4's first end to end is the space with a Claude Code worker**, if the probes allow:
  main creates a space, the worker is started in it with `-w`, and the pair exchanges a task,
  a question, a notify and a report — stage 4's live acceptance met inside a space. The tab
  in its least form (the button, the screen, the keys) moves from stage 6 into stage 4 for
  this — preferably Claude Code's tasks list as the tab bar, the mod drawing the terminal; the
  rest of the person's UI stays in stage 6. If P15 fails, stage 4 delivers the space without a
  tab and an attach from the person's terminal, and the tab waits for stage 6.
- **Stage 4 also builds what P12–P14 settle**: the worktree's whole life handed to Claude
  Code, with the keeper P12b picks (row 13), the `SendMessage` guard (row 5), `--name` and the
  briefing's carrier (row 2). Most of `internal/adapter/claude/worktree.go` — the long flag
  taken from Claude Code's arguments and its refusals — goes, since Claude Code's own `-w` then
  works and resumes into its checkout natively; `internal/host/worktree` goes too once a
  keeper holds, and stays reshaped to Claude Code's conventions only if P12b fails.
- **Stage 4b, the pi adapter**, comes next, before Codex, by the owner's decision of
  October 13, 2026 ([the stage plan](design.md#the-stage-plan-from-here)). pi carries the
  models the room's Claude Code sessions do not already serve. Its rows weigh the same
  verdicts: an extension loaded with `-e` in place of the mod, a wrapper or an extension's
  join, a worker seated by cwd in a checkout a Claude Code keeper holds. The pi reading
  finds every capability native in the extension but two: grants on shell commands need an
  OS sandbox the wrapper starts, and provider limits are not reported. Its probe items
  PI1–PI14 ([stage4-reading-pi.md](stage4-reading-pi.md)) are stage 4b's probe, under a
  pinned version. The first mixed acceptance is a Claude Code main with a pi worker seated
  in a space, by cwd, in a checkout a Claude Code keeper holds, or rewake's if P12b fails.
- **Stage 5, the Codex entry point**, by the owner's decision of October 13, 2026: Codex
  is not built now; before 2.0 it gets an entry point, a stub without a real
  implementation, and its full adapter comes after the release. The Codex column of the
  rows and [its reading](stage4-reading-codex.md) wait for that adapter. The stub's shape,
  decided by main the same day so that it meets `AGENTS.md`'s rule that a harness cannot be
  half-registered: a catalogue entry fully described by what it offers — Launch alone, its
  version read and gated, its protected directories (today held by the catalogue's
  `foreign` list) declared by the entry itself. Every capability it lacks refuses by name,
  as stage 3 does on Claude Code: with no turn boundary, a launch of a role that reports — the
  default role and write — is refused at launch, naming why
  ([design-api.md](design-api.md), `internal/cli/turn_reporting.go`), so the one launch the
  stub permits is main's, the role that reports to no one. No silent worker role is added
  for it. A command that only refuses was weighed and set aside: `rewake
  codex` would then exist with no harness in the catalogue behind it, which the rule
  forbids. The acceptance line for the full adapter, kept for after 2.0: a Codex worker
  seated in a worktree Claude Code made and holds, under the Git grant, its removal refused
  while the worker sits there.
- **Stage 6** keeps grants, permissions and the rest of the person's UI — the room panel,
  the space's tab beyond its least form, the limits on main creating spaces. P15 has moved
  beside stage 4's probe: whether a rewake process owns a pseudo-terminal changes every
  adapter's launch, and the space needs it in stage 4.
- **Headless and background workers** (row 18) stay unscheduled, and P16 with them
  (decided by main, October 13, 2026); it runs only if the owner takes them up.
- **Stage 7** is unchanged but for its mixed pair, which becomes Claude Code with pi, since
  Codex is only an entry point before 2.0.
