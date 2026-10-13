# Stage 4: the first working space

The design of stage 4's first end to end, written October 13, 2026 on branch
`s4-space-design`, for review before any code. It rests on the owner's target
([stage4-audit.md](stage4-audit.md#the-target-a-space)), the owner's answers 7–9
([design.md](design.md#the-owners-answers)), the owner's decisions of the same day
recorded below, and the probe's records on Claude Code 2.1.296
([research-claude-mod.md](../research-claude-mod.md)). Its rules, tests, build steps and open
questions are in [stage4-space-steps.md](stage4-space-steps.md). A call, event or field of
the mod API named here is named only to say what rewake does with it; its shape and limits
are the `mods` skill's ([design-claude.md](design-claude.md#where-the-api-facts-live)).

## The target

Main, a Claude Code session launched by rewake, opens a space. The mod spawns a keeper
subagent, and Claude Code makes the worktree for it. Main's wrapper starts a Claude Code
worker through rewake in a pseudo-terminal it owns, inside that worktree. The keeper's entry in
Claude Code's tasks list is the space's tab, and its view shows the worker's screen. Keys go
in, with the prefix for those the mod's key surface does not get. Main and the worker
exchange a task and its report through rewake's mail. Closing main closes the space and its
worker. The space's record keeps the worker's session id, harness, worktree path and branch,
role and name, and a checkout kept with changes is reminded until it is merged or removed.

Decided by the owner, October 13, 2026: the approach is fixed — the person runs Claude Code,
and the harnesses rewake hosts live inside it through the mod; what remains is how fully the
pseudo-terminal is shown. Decided by the owner the same day: the space's screen is drawn as
styled text in the key surface — full attributes and wide glyphs — not as a cell grid; a
cell grid is the fallback if text proves too slow. And: no terminal parser of rewake's own;
a maintained Go library is chosen by the dependency rule ([below](#the-terminal-library)).

## 1. The pieces and where they live

| Piece | Where | Why there |
|---|---|---|
| the mod | `internal/adapter/claude/mod/`: TypeScript sources, `plugin.json`, `hooks.json`, the key surface module, the generated protocol types; embedded with `embed` | answer 9: TypeScript only in the part inside the harness, versioned with the binary. Embedded, not generated: the sources are static, so they are read, validated and tested as files; what differs per run comes from the run's environment |
| writing it for a run | `internal/adapter/claude`, in `Launch`: the embedded tree written into the run directory, `--plugin-dir <run dir>/mod` added | L1 and L3: one launch's option, gone with the run ([design-claude.md](design-claude.md#loading-the-mod-for-one-run)) |
| the mod's endpoint | `internal/adapter/claude/modhost`: an HTTP server on the run's mod socket, each request checked by its peer, translated into the adapter API's calls | the host side of the adapter is Go (answer 9); it speaks the mod's protocol and nothing else, and reaches the core only through what the host hands it, as a `TurnBoundary` reaches a `TurnHandler` today |
| the space host | `internal/host/space`: opening and closing spaces, the hosted launch of a worker, its pseudo-terminal, the screen, keys and size, holds | a host job: it starts processes, lives with main's wrapper and writes records; harness-neutral, so a pi or Codex worker is the same code |
| the terminal | `internal/host/terminal`: the pseudo-terminal by the standard library (Linux `/dev/ptmx` and its ioctls, `Setsid` and `Setctty`), the emulator from a library behind one small interface, the screen as styled rows | rewake runs on Linux only (it reads `/proc`), so a pseudo-terminal needs no dependency; the emulator is the one place a library enters, so swapping it touches one package |
| the space record | `internal/core/space`: the record, its states, the kept-checkout reminder; written through `internal/infra/state` under the room's lease | a record of rewake's own that every command may read; the core knows no harness: the record names the worker's harness by its id |
| the adapter API | `internal/adapter`: a `SpaceHost` interface the host implements and hands to the run's service, the values it carries (a space, a screen of styled rows, keys, a size), and `SpacesCapability` in the live set | the import rule: an adapter never imports the host, so the host composes itself in, as it does with `TurnHandler` |
| the commands | `internal/cli`: `rewake space open`, `rewake space list`, `rewake space close` in the command table | the command line is first-class; a tool for main's model is projected from the same table later ([section 9](#9-commands)) |

The protocol schema and its generator are in [section 2](#2-the-protocol). The layout test
holds every edge above: `host/space` imports `adapter`, `core`, `infra` and `host/terminal`;
`host/terminal` the standard library and the emulator; `adapter/claude/modhost` `adapter`,
`core` and `infra`; no `core` or `infra` package imports a library.

### The terminal library

The owner's direction, October 13, 2026: no parser of rewake's own — a screen that a nested
harness draws needs CSI, SGR in all its colour forms, scroll regions, the alternate screen,
modes and the widths of wide glyphs, which is not small. The dependency rule asks a library
to carry no transitive dependencies and to be actively maintained (`AGENTS.md`, "Code").
Three candidates, read from the module proxy on October 13, 2026:

| Library | Covers | Dependencies | Maintained |
|---|---|---|---|
| `github.com/charmbracelet/x/vt` v0.1.0 (MIT) | CSI, SGR with 256 and direct colours, scroll regions, the alternate screen, modes including bracketed paste, damage tracking, callbacks for title and bell, glyph widths | three direct (`charmbracelet/ultraviolet`, `charmbracelet/x/ansi`, `charmbracelet/x/exp/ordered`), fourteen in its module graph | yes: tagged October 12, 2026, in an active repository |
| `github.com/hinshun/vt10x` (MIT) | a port of st: CSI, SGR with 256 and direct colours, scroll regions, the alternate screen; no glyph widths, no bracketed-paste mode | none | no: last commit March 2022 |
| `github.com/micro-editor/terminal` (MIT) | the same st lineage as vt10x, kept by the micro editor; no glyph widths | `creack/pty`, `golang.org/x/sys` | slowly: last commit March 2025 |

None meets both halves of the rule. **Proposed: `charmbracelet/x/vt`, pinned**, because the
owner's decision to draw full attributes and wide glyphs needs an emulator that tracks widths,
and it is the only maintained one that does; the st ports would shift every column after a
wide glyph. The cost is its dependency set, which the rule forbids, so the choice is the
owner's (open question Q1 in [the steps](stage4-space-steps.md#open-questions)). If the owner
declines, the fallback is vt10x, accepting unmaintained code and narrow glyphs only. Either
way the library sits behind `internal/host/terminal`'s interface — write bytes, resize, read
the screen as styled rows with the cursor — and no other package sees it.

## 2. The protocol

**Transport.** HTTP/1.1 over a Unix socket, the run's mod socket, in the short form `sock/`
keeps for sockets ([design-state.md](design-state.md#the-state-root-of-20)); the launch sets
its path in the harness's environment, `REWAKE_MOD_SOCKET`, and the mod reads it with
`$.env.get`. The path is not a secret: no request carries authority but its peer.

**Who answers.** Main's wrapper, through `modhost`, while the wrapper stays — until a
replacement authority contract is accepted the mod's join is not in this step
([stage4-audit.md](stage4-audit.md#a-what-the-wrapper-does-today-and-whether-the-mod-can-take-it-over)).
A worker's wrapper answers its own mod the same way. Each request is checked on its own: the
peer of the connection, by `SO_PEERCRED`, must be the harness process the wrapper started,
alive with its start time before and after the answer — the check the endpoint already makes
for a transport (`internal/host/endpoint/transport.go`). That `$.http.fetch` connects from the
harness process is P4, still open; the first build step runs it
([the steps](stage4-space-steps.md#the-build-steps)).

**Bounds.** `$.http.fetch` gave up after 30 s in every probe run, so no request is held
longer than 20 s; a wait is a loop of polls. A request body is at most 64 KiB, a screen
answer at most 96 000 characters, the room a key surface's props allow less a margin.

**Endpoints.** Every one a `POST` with a JSON body, versioned under `/v1/`:

| Path | Caller | Asks | Answers |
|---|---|---|---|
| `hello` | every mod load | the session id, the harness version, the load's id | the run's role and whether it hosts spaces |
| `turn/start`, `turn/end` | a worker's mod | the turn's id; at its end the reason and the answer | a receipt once the core took it ([section 5](#5-reports)) |
| `events` | main's mod, in a loop | the last event it saw | within 20 s, the next space to open, whose keeper the mod spawns; the spawn's `agentId` comes back with the next poll |
| `space/hold` | main's mod, for the keeper's call | the space, the call's `agentId` and `tool_use_id` | within 20 s: `held`, `released`, or at once `duplicate` |
| `space/screen` | main's mod, while the keeper's row is drawn | the space, the last screen sequence it has | within 20 s, the rows changed since, as styled runs; the size, the cursor, whether the worker lives and its exit |
| `space/keys` | main's mod, on the key surface's posts | the space, the bytes | done |
| `space/resize` | main's mod, when the row's size changes | the space, columns and rows | done |
| `session/end` | main's mod, at `session.end` | nothing | once every space is closed ([section 7](#7-closing)) |

A styled run is its text, its foreground and background (a palette index or a direct colour)
and its attributes — bold, dim, italic, underline, inverse, strikethrough — with a wide glyph
counted as two columns; a screen answer carries only the rows that changed, so a quiet screen
costs nothing.

**One description.** The protocol is one JSON schema,
`internal/adapter/claude/mod/protocol.schema.json` (answer 9). A small generator of rewake's
own, `tools/protogen`, reads the subset the schema uses — objects, strings, integers,
booleans, arrays, enums, references — and writes the TypeScript types the mod imports and the
Go types `modhost` decodes into. A test regenerates both in memory and fails when a checked-in
file differs, so neither side can drift from the schema.

## 3. The keeper

**Its type.** At `session.start` main's mod registers the agent type `rewake:keeper` with
worktree isolation, one tool — the mod's `space_hold` — and a prompt saying: call
`space_hold` once with this space; it blocks for minutes or hours, which is expected; never
call it again. Which model the keeper runs on is open (Q9): it takes one or two short turns.

**Its spawn.** When main opens a space the host queues an open request; main's mod takes it
from `events` and calls `$.agent.spawn` with the keeper's type, the space's name as the
description and its id in the prompt — no turn of main's model, as P12b saw
([the record](../research-claude-mod.md#p12b-a-subagent-of-main-as-the-worktrees-keeper)). The
spawn's `agentId` goes back to the host with the next `events` poll.

**Its one held call.** The keeper's `space_hold` call fires `tool.call` in main's mod, which
answers it itself: a loop of `space/hold` polls, each under 20 s, until the host answers
`released`, and then the result "space <name> closed". The first hold of a space seats it
([section 4](#4-the-workers-launch)).

**The second call.** Every probe run that held a keeper for 30 s saw a second `space_hold`
call about 30 s after the first, under an agent id `$.agent.list()` never listed
([the record](../research-claude-mod.md#the-keepers-second-call)). The skill names the likely
cause in its description of `agent.spawn`: a stalled agent's retry, which runs as a new
attempt and does not raise the spawn again. So a second hold of a held space is answered at
once, `duplicate`, and the mod returns "space <name> is already held by an earlier call that
is still waiting; do not call `space_hold` again"; the first call goes on, as `vI91jc`
showed. Whether the held call can
report progress so that no retry starts is open (Q4); the answer above is needed either way.

**Its release.** Closing the space answers the held call `released`; the keeper's turn ends;
Claude Code removes its checkout if unchanged and keeps it if changed, as P12b saw.

**The worktree's base.** Claude Code makes the keeper's checkout from `origin/<default>` on
a branch `worktree-agent-<id>`, while the work usually continues main's branch. Decided here:
before seating the worker, the space host moves that branch to the commit main's own checkout
has at its `HEAD` (`git reset --hard <commit>` inside the keeper's checkout), unless the open
names another base. The host does it, not the keeper's model: it costs no turn and it is a
Git operation like those `internal/host/worktree` runs today. Whether Claude Code then judges
the checkout changed and keeps it at the end is open (Q6); kept, it is reminded like any kept
checkout. The checkout's path is found by `git worktree list --porcelain`: the one whose lock
reason names the keeper's agent id and main's harness pid; that the id in the path is the
spawn's `agentId` is checked live (Q5).

## 4. The worker's launch

rewake refuses to start a harness from a shell inside a session, exit 2
(`internal/cli/launch.go`, `refuseNestedLaunch`), and that stays: a session does not start
sessions. The hosted launch is not a session's shell starting one; it is **main's wrapper**
starting one, on a request the mod made under the peer check.

- **Who starts it.** The space host in main's wrapper, which lives exactly as long as main's
  run and so as long as the space (answer 7, no service beside the sessions). It runs the
  wrapper's own image, `/proc/self/exe`, so the worker is the room's build (T10).
- **The command.** `<image> <harness> --name <name> --<role> --room <main's room> --
  <the harness's new-conversation arguments>`, in a pseudo-terminal of the row's size, with
  the keeper's checkout as its working directory. It is an ordinary launch from there on: it
  claims its name, takes the room's lease, writes its own run directory and its own mod.
- **The environment.** The wrapper's own, less every `REWAKE_` variable and the worker
  harness's `EnvNotInherited`, plus `TERM=xterm-256color`. The wrapper's environment is the
  person's terminal's, not main's harness's, so nothing main's harness exports reaches the
  worker; the scrub is kept anyway, since the refusal of a nested launch reads exactly those
  variables.
- **Its session id.** rewake picks a random UUID before the launch and writes it into the
  space record; the worker's adapter turns it into its harness's arguments. A new optional
  adapter interface, `ConversationNamer`, does that: Claude Code answers `--session-id <id>`
  (`claude --help`, 2.1.296), pi `--session-id <id>` later ([the pi reading](stage4-reading-pi.md)).
  A harness without it gets no id, and its record says so.
- **Its name and role.** Given with the open, or by default `<space name>`; the role is
  `write` by default, any role the worker's harness can serve ([section 5](#5-reports)).
- **How it registers.** As any launch does: its session record names its run; the space
  record names the worker's name and, once its wrapper runs, its epoch, which the host reads
  from the child it started.

## 5. Reports

Stage 3 refuses a reporting role on Claude Code, since its adapter has no turn boundary
(`internal/cli/turn_reporting.go`). A role that does not report gives no report, which the
target needs. **Decided: the worker's mod serves TurnBoundary, reporting without holding.**
`turn.start` goes to the host as the turn's start and `turn.complete` as its end — the reason
mapped to answered, aborted, refused or failed, and its `answer` as the report — each through
`turn/start` and `turn/end`; events carrying `agentId` are a subagent's and are not sent.
Claude Code's offer becomes `TurnBoundary{Reports: true, HoldsEnd: false}`, live once the
run's mod said hello; the core already handles a boundary that cannot hold an end. The
confirmation through `classic.Stop` with `block` (O1) is a later step of stage 4, with P3.

Wake stays Claude Code's inbox socket of stage 3; a worker's notices reach it as they reach
any Claude Code session.

## 6. Keys and the screen

**The screen.** The keeper's `space_hold` row is drawn by main's mod's `ui.render` hook while
its call runs; the row is a key surface (`Client`) whose module draws the screen as rows of
styled `Text` spans and takes the keys (the owner's decision above). The mod keeps the latest
screen in its state, polls `space/screen` while the row is mounted, and redraws the row on a
change; the rows reach the key surface as its props, within their 100 000 characters. Whether
props on a redraw repaint fast enough at 160 by 30, and at what cost, is P15c; if not, the
fallback is the cell grid P15a and P15b used, behind the same small seam in the mod — one
function from a screen to the row's tree. Only text is built.

**Keys.** The key surface turns a key into the bytes a terminal sends, as the probe's did:
letters and typed text as themselves, Alt as ESC and the key, the arrows, Home, End, Page Up
and Down, Insert and Delete in their xterm sequences, F1–F12, Tab and Shift+Tab, Backspace as
0x7f, Enter as CR, a Ctrl+letter that arrives as its control byte. It queues them and posts in
batches, since one post per frame is delivered and a later post replaces an undelivered one.

**The prefix** (the owner's direction, October 13, 2026): Alt+a, then `c` sends Ctrl+C, `e`
or `[` Escape, `x` Ctrl+X, Alt+a again Alt+a itself; any other key cancels it and sends
nothing. While the prefix waits, the row's last line is the hint
`PREFIX: c=^C  e/[=Esc  x=^X  Alt+a=Alt+a  other=cancel`. P15b carried all of it end to end.

**What the outer level keeps.** Escape returns the focus to main's prompt; Ctrl+C, Ctrl+X and
paste stay main's Claude Code's; from the prompt the tasks list switches between main, its
subagents and the spaces. A click on the row gives it the focus. Paste into a space stays open
(Q10): the clipboard cannot be read, and a bracketed paste goes to main's prompt.

## 7. Closing

**main ends.** At `session.end` main's mod calls `session/end`. The host closes every space:
it hangs up each worker's pseudo-terminal — the worker's wrapper is its session leader and
departs as on a hang-up — waits up to 5 s, then sends SIGTERM and, after 2 s more, SIGKILL to
what is left of that session; then answers every held call `released`, so each keeper ends
and Claude Code removes or keeps its checkout; then marks each record closed with what it saw
of the checkout — gone, or kept with changes or commits. The skill counts a hook's own time
only, not its waits on `$` calls, and a `space/hold` answer arrives within one poll. Whether
Claude Code finishes a keeper's removal before the process exits is open (Q7).

**main crashes.** With no `session.end`, the wrapper sees its harness end and hangs up the
workers the same way — and if the wrapper itself is killed, the kernel closes the
pseudo-terminal's master and the hang-up comes anyway. The keepers die with main's process,
and their checkouts stay, Git-locked under a dead pid (P12b, runs `tZXlTa` and `SNr8JH`).
The next launch of a main in that room reads the records still open whose main's run is
proven dead, marks them closed "main ended without closing", and inspects each checkout:
present or not, locked by a dead process, with changes or commits. **The reminder of kept
checkouts covers it** (the owner, October 13, 2026): such a checkout is listed to main at
its start and by `rewake space list` until it is merged or removed, with the Git commands
that clear it. rewake removes no checkout itself in this step: what is inside is work, and
only main or the person knows whether it is wanted.

**One space closed.** `rewake space close <name>` does the same for one space.

## 8. Restore

Not in this step: the first working space ends at the record. A restore — resuming a worker
with `--resume <session id>` in its checkout — needs a keeper over a checkout that already
exists, while Claude Code makes a new one for each keeper; that is the next step's design.
This step writes everything a restore reads, and `rewake space list` shows closed spaces with
their session ids.

## 9. Commands

The smallest set is three commands in the command table:

- `rewake space open [<name>] [--harness <id>] [--role <role>] [--base <commit>]` — a change;
  main only; refused unless main's run has `SpacesCapability` live. Main's wrapper queues the
  open for its mod and answers once the keeper is spawned and the worker started, or with
  the step that failed.
- `rewake space list` — a view: open spaces and closed ones, their workers' names, harnesses,
  session ids, checkouts, branches, and kept checkouts flagged.
- `rewake space close <name>` — a change; main only.

Main's model calls them from its shell, and the person with `! rewake space open` at main's
prompt; a command from a shell reaches its own run's wrapper as main's grants and decisions
do today. Projecting them as tools for the model, and a slash command or a button for the
person, come after the first space works: the tools follow from the table by the allowlist's
review ([design-api.md](design-api.md#tooltransport)), and the person's UI is stage 6.

The owner floated pi's interface as the renderer and pseudo-terminal host, with another
harness spawned inside it; it is recorded as an alternative under the open questions, not
designed here.
