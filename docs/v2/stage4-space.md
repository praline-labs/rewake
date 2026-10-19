# Stage 4: the first working space

**Superseded October 18, 2026** by the owner's decision: a space is main starting its workers,
not shown inside Claude Code, so the keeper subagent, the screen drawn in main's session, its
keys and its prefix were never built; their sections were removed on October 19, 2026. The
one step that replaced them is in [stage4-space-steps.md](stage4-space-steps.md#the-build-steps),
the space as built is [the rules](../rules/space.md), and this design stays as the record of
the hosted launch, the record and the reasoning behind them.

The design of stage 4's first end to end, written October 13, 2026 on branch
`s4-space-design`, for review before any code. It rests on the owner's target
([stage4-audit.md](../archive/v2/stage4-audit.md#the-target-a-space)), the owner's answers 7–9
([design.md](design.md#the-owners-answers)), the owner's decisions of the same day
recorded below, and the probe's records on Claude Code 2.1.296
([research-claude-mod.md](../research-claude-mod.md)). Its rules, tests, build steps and open
questions are in [stage4-space-steps.md](stage4-space-steps.md). A call, event or field of
the mod API named here is named only to say what rewake does with it; its shape and limits
are the `mods` skill's ([design-claude.md](design-claude.md#where-the-api-facts-live)).

## The target

Main, a Claude Code session launched by rewake, opens a space: a worker started through
rewake in a pseudo-terminal rewake owns, inside a worktree. Main and the worker
exchange a task and its report through rewake's mail. Closing main closes the space and its
worker. The space's record keeps the worker's session id, harness, worktree path and branch,
role and name, and a checkout kept with changes is reminded until it is merged or removed.

Decided by the owner, October 13, 2026: no terminal parser of rewake's own; a maintained Go
library is chosen by the dependency rule ([below](#the-terminal-library)). How the worker's
screen reaches the person is the shell's since October 19, 2026 ([shell.md](shell.md)).

## 1. The pieces and where they live

| Piece | Where | Why there |
|---|---|---|
| the mod | `internal/adapter/claude/mod/`: TypeScript sources, `plugin.json`, `hooks.json`, the generated protocol types; embedded with `embed` | answer 9: TypeScript only in the part inside the harness, versioned with the binary. Embedded, not generated: the sources are static, so they are read, validated and tested as files; what differs per run comes from the run's environment |
| writing it for a run | `internal/adapter/claude`, in `Launch`: the embedded tree written into the run directory, `--plugin-dir <run dir>/mod` added | L1 and L3: one launch's option, gone with the run ([design-claude.md](design-claude.md#loading-the-mod-for-one-run)) |
| the mod's endpoint | `internal/adapter/claude/modhost`: an HTTP server on the run's mod socket, each request checked by its peer, translated into the adapter API's calls | the host side of the adapter is Go (answer 9); it speaks the mod's protocol and nothing else, and reaches the core only through what the host hands it, as a `TurnBoundary` reaches a `TurnHandler` today |
| the space host | `internal/host/spacehost`: opening and closing spaces, the hosted launch of a worker, its pseudo-terminal and size | a host job: it starts processes, lives with main's wrapper and writes records; harness-neutral, so a pi or Codex worker is the same code |
| the terminal | `internal/host/terminal`: the pseudo-terminal by the standard library (Linux `/dev/ptmx` and its ioctls, `Setsid` and `Setctty`), the emulator from a library behind one small interface, the screen as styled rows | rewake runs on Linux only (it reads `/proc`), so a pseudo-terminal needs no dependency; the emulator is the one place a library enters, so swapping it touches one package |
| the space record | `internal/core/space`: the record, its states, the kept-checkout reminder; written through `internal/infra/state` under the room's lease | a record of rewake's own that every command may read; the core knows no harness: the record names the worker's harness by its id |
| the adapter API | `internal/adapter`: `SpacesCapability` in the live set | the import rule: an adapter never imports the host, so the host composes itself in, as it does with `TurnHandler` |
| the commands | `internal/cli`: `rewake space open`, `rewake space list`, `rewake space close` in the command table | the command line is first-class; a tool for main's model is projected from the same table later ([section 9](#9-commands)) |

The protocol schema and its generator are in [section 2](#2-the-protocol). The layout test
holds every edge above: `host/spacehost` imports `adapter`, `core`, `infra` and `host/terminal`;
`host/terminal` the standard library and the emulator; `adapter/claude/modhost` `adapter`,
`core` and `infra`; no `core` or `infra` package imports a library.

### The terminal library

**Superseded, October 18, 2026:** the emulator left the code with this library once a space was
shown only through `rewake attach` ([the terminal](stage4-space-terminal.md)), and came back the
same day, under this exception, for the workers' panel ([workers-panel.md](../archive/v2/workers-panel.md), replaced by the host mode).

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

None meets both halves of the rule. **Decided by the owner, October 13, 2026:
`charmbracelet/x/vt`, pinned, as an exception to the dependency rule.** The reason: the
decision to draw full attributes and wide glyphs needs an emulator that tracks widths, and it
is the only maintained one that does; the st ports would shift every column after a wide
glyph. The exception covers this library in `internal/host/terminal` only; its build step
(S4-4) measures what it really brings into the binary with `go list -deps` and records it. The
library sits behind that package's interface — write bytes, resize, read the screen as styled
rows with the cursor — and no other package sees it. What S4-4 measured it to bring in, and
what it does not do, is [stage4-space-terminal.md](stage4-space-terminal.md).

## 2. The protocol

**Transport.** HTTP/1.1 over a Unix socket, the run's mod socket, in the short form `sock/`
keeps for sockets ([design-state.md](design-state.md#the-state-root-of-20)); the launch sets
its path in the harness's environment, `REWAKE_MOD_SOCKET`, and the mod reads it with
`$.env.get`. The path is not a secret: no request carries authority but its peer.

**Who answers.** Main's wrapper, through `modhost`, while the wrapper stays — the mod's join is
not in this step
([stage4-audit.md](../archive/v2/stage4-audit.md#a-what-the-wrapper-does-today-and-whether-the-mod-can-take-it-over)).
A worker's wrapper answers its own mod the same way. Each request is checked on its own: the
peer of the connection, by `SO_PEERCRED`, must be the harness process the wrapper started,
alive with its start time before and after the answer — the check the endpoint already makes
for a transport (`internal/host/endpoint/transport.go`). That `$.http.fetch` connects from the
harness process is P4, answered positive by the first build step on October 13, 2026, 2.1.296
([the record](../research-claude-mod.md#p4-the-peer-of-the-mods-requests)); a hooks worker
respawned after a crash and a shell child started before a reload stay open.

**Bounds.** `$.http.fetch` gave up after 30 s in every probe run, so no request is held
longer than 20 s; a wait is a loop of polls. A request body is at most 64 KiB — a turn's end
1 MiB, its answer cut by the mod past that and saying so.

**Endpoints.** Every one a `POST` with a JSON body, versioned under `/v1/`:

| Path | Caller | Asks | Answers |
|---|---|---|---|
| `hello` | every mod load | the session id, the harness version, the load's id and number | the run's role, whether it hosts spaces, the heartbeat's generation; 410 to a load a higher-numbered one superseded, naming that one's number |
| `turn/start`, `turn/end` | every load's mod, for its own conversation's turns | the turn's id; at its end the end's id, the reason and the answer | a receipt once the core took it ([section 5](#5-reports)) |
| `mod/alive` | every admitted load, in a loop | the load | within 15 s, done; 409 once its generation lapsed or is not the current one, and the mod says hello again; 410 once a later load superseded it ([the live set](stage4-turns.md#the-live-set)) |
| `mod/ended` | every admitted load, at its start | the epochs of other runs whose ends the store keeps | those rewake proves ended ([the outbox](stage4-turns.md#the-mods-side)) |
| `session/closing` | main's mod, at `session.end` when the process exits | the reason | at once: the wrapper takes over the shutdown ([section 7](#7-closing)) |

**One description.** The protocol is one JSON schema,
`internal/adapter/claude/mod/protocol.schema.json` (answer 9). A small generator of rewake's
own, `tools/protogen`, reads the subset the schema uses — objects, strings, integers,
booleans, arrays, enums, references — and writes the TypeScript types the mod imports and the
Go types `modhost` decodes into. A test regenerates both in memory and fails when a checked-in
file differs, so neither side can drift from the schema.

## 4. The worker's launch

rewake refuses to start a harness from a shell inside a session, exit 2
(`internal/cli/launch.go`, `refuseNestedLaunch`), and that stays: a session does not start
sessions. The hosted launch is not a session's shell starting one; it is **main's wrapper**
starting one, on a request the mod made under the peer check.

**The topology: a worker is not main's descendant.** Decided by main, October 13, 2026: the
worker is started **detached**, outside main's process tree, so no signal of main's shutdown
reaches what ends the worker and no command the worker runs passes for one of main's. The
operator-decision authority that first asked for it was removed on October 18, 2026; the rule
stays as SP9 ([the rules](../rules/space.md)).

- **Who starts it.** The space host in main's wrapper, which lives exactly as long as main's
  run and so as long as the space (answer 7, no service beside the sessions). It opens the
  pseudo-terminal, keeps the master, and starts the wrapper's own image, `/proc/self/exe`,
  under a name of its own as a short-lived launcher — the pattern of the probe holder
  (`internal/host/launch/probe_holder.go`). The launcher gets the terminal's slave side as its
  standard streams and the read end of a liveness pipe as an inherited descriptor. It starts
  the worker's wrapper in a new session with the slave side as its controlling terminal
  (`Setsid`, `Setctty`), reports that process's pid and start time on a second pipe, and exits.
  The worker's wrapper is reparented away from main's tree: to the nearest subreaper above
  main's wrapper, or to init. The image is the room's build (T10).
- **The check that it left main's tree.** Two parts, both before any code the worker controls
  runs. Main's
  wrapper reads its own `PR_GET_CHILD_SUBREAPER` at the open and refuses the open when set:
  then an orphan would come back to it. rewake sets the flag only in the probe holder, a
  process of its own; Claude Code is main's wrapper's child, never on the launcher's path, so
  nothing it sets can take the orphan. And once the launcher has exited, the host requires
  the whole chain of parents read up to init to miss main's wrapper (`proc.Outside`); an
  ancestry that cannot be read is unknown, not a success, and a worker that still descends or
  cannot be shown not to is stopped and the open fails, naming why.
- **The launch gate.** The launcher and the worker's wrapper are rewake's own code, so they may
  descend from main for the moment the reparenting takes; nothing the worker controls may. The
  worker's wrapper therefore waits, before it starts its harness or anything the harness
  configures to run, for one byte the host writes on the liveness pipe once the check passed;
  the end of the pipe before that byte aborts the launch.
  If detaching cannot be guaranteed on a machine, the open is refused there and main decides.
- **The command.** `<image> <harness> <the harness's new-conversation arguments>` with
  `REWAKE_SPACE=<room>/<space id>`, from which the launch takes the name, role and room (since
  October 19, 2026; [launch.md](../launch.md#a-space-for-a-worker)), in its checkout and with the terminal at the
  size asked for. It is an ordinary launch from there on, but
  for its process groups: it claims its name, takes the room's lease, writes its own run
  directory and its own mod.
- **Its process groups** (decided by main, October 13, 2026). An ordinary launch keeps the
  harness in the wrapper's group on purpose, so that a person's shell sees one job
  (`internal/host/wrap.go`); that stays. A hosted launch has no shell: its wrapper leads the
  session of a terminal only rewake holds. There the wrapper starts its harness in a process
  group of its own and makes that group the terminal's foreground group (`tcsetpgrp`); the
  wrapper stays the session leader in its own group, outside every target of the shutdown in
  [section 7](#7-closing), so it lives to reap the harness and record the end. The terminal
  then behaves as for any foreground job: Ctrl+C and Ctrl+Z written into the space reach the
  harness's group through the terminal's line discipline, and the wrapper gets neither. A
  harness that stops has no shell to bring it back, so the wrapper continues its group at once
  and gives it the terminal again. After the first handoff the wrapper's own group is an
  orphaned background group of its terminal, and Linux lets such a group change the foreground
  only while SIGTTOU is blocked or ignored — a handler is not enough (`tty_check_change` in
  `drivers/tty/tty_jobctrl.c`, read at v6.12). So the wrapper blocks SIGTTOU around each of its
  own `tcsetpgrp` calls and nowhere else, and the harness starts with the default signal mask
  and dispositions, restored before its exec, so it inherits no blocked or ignored job-control
  signal.
- **The environment.** The wrapper's own, less what names a session or a transport —
  `REWAKE_SESSION`, `REWAKE_EPOCH`, `REWAKE_ROOM`, `REWAKE_DIR`, `REWAKE_MOD_SOCKET`, every
  `REWAKE_BRIDGE_` variable — and less the worker harness's `EnvNotInherited`, as the probe
  environment drops them today (`internal/host/launch/environment.go`, `ProbeEnv`). Then the
  host adds values of its own: `REWAKE_DIR` set to the base main's checked room belongs to
  (`state.BaseForRoom`), so the space's room is the same room under the same base and
  never the default one; the liveness descriptor's number; and `TERM=xterm-256color`. The
  person's preferences (`REWAKE_CLAUDE_MODEL`, `REWAKE_WORKTREES` and the like) stay. The
  refusal of a nested launch reads `REWAKE_SESSION` and `REWAKE_EPOCH` only, and neither
  reaches the worker.
- **The liveness pipe.** Its write end is held by main's wrapper alone: created close-on-exec
  and passed to no child, so neither main's harness nor the launcher holds it. The worker's
  wrapper reads the other end; the end of the pipe means main's wrapper is gone or let the
  worker go, and the worker's wrapper then shuts its run down by itself within a bound
  ([section 7](#7-closing)).
- **Its session id.** rewake picks a random UUID before the launch and writes it into the
  space record; the worker's adapter turns it into its harness's arguments. Every adapter's
  offer answers whether it can, a flag or the reason there is none (`ConversationOffer`,
  [design-api.md](design-api.md#conversation-names), built in S4-5): Claude Code answers
  `--session-id <id>` (`claude --help`, 2.1.296), pi `--session-id <id>` later
  ([the pi reading](stage4-reading-pi.md)). A harness without it gets no id, and its record
  says so.
- **Its name and role.** Given with the open, or by default `<space name>`; the role is
  `write` by default, any role the worker's harness can serve ([section 5](#5-reports)).
- **How it registers.** As any launch does: its session record names its run; the space
  record names the worker's name and, once its wrapper runs, its epoch, which the host reads
  from the record that pid writes, checked against the pid and start time the launcher reported.

## 5. Reports

Stage 3 refuses a reporting role on Claude Code, since its adapter has no turn boundary
(`internal/cli/turn_reporting.go`). A role that does not report gives no report, which the
target needs. **Decided: the worker's mod serves TurnBoundary, reporting without holding**,
`TurnBoundary{Reports: true, HoldsEnd: false}`, live once the run's mod said hello: `turn.start`
and `turn.complete` of the session's own conversation go to the host as `turn/start` and
`turn/end`, the answer as the report, an end retried under its id until its receipt comes; a
heartbeat whose lapse withdraws what the mod proved. The confirmation through `classic.Stop`
with `block` (O1) is a later step of stage 4, with P3. The design, what not holding costs and
the build are [stage4-turns.md](stage4-turns.md), written for S4-6 on October 16, 2026.

Wake stays Claude Code's inbox socket of stage 3; a worker's notices reach it as they reach
any Claude Code session.

## 7. Closing

The shutdown belongs to the wrappers, never to a hook: `session.end` has one wall-clock bound
for its whole chain, 1.5 s by default, its `$` waits included, and at the bound an in-flight
call is aborted (the skill, `SessionEndInput`, 2.1.296). What a hook starts in that time is a
signal, not the work.

**How one worker is stopped.** Main's wrapper closes that worker's liveness pipe. The worker's
own wrapper sees the end of the pipe — or a hang-up of its terminal, which it takes the same way
— and runs a bounded shutdown itself: SIGHUP to its harness's process group, SIGTERM after 5 s,
SIGKILL after 2 s more. That group is the harness's own and never holds the wrapper
([section 4](#4-the-workers-launch)), so the wrapper outlives every signal it sends: it reaps the
harness, sees no process left in the group, records its run's end from that observation, and
exits. The worker's wrapper does this,
not main's, so it happens also when main's wrapper is gone. Main's wrapper observes the end — the
worker wrapper's pid with its start time gone — and only an observed end counts; one still there
after the bound and a margin has its session killed by main's wrapper.

**main ends.** At `session.end` with an exit reason, main's mod sends `session/closing`, best
effort, under the hook's bound; an abort loses nothing, since the wrapper also sees its harness
exit. On either signal the host writes each open space's record as closing and stops each
worker. Once each worker's end is observed the host inspects its checkout — gone, kept with
changes or commits — and marks each record closed
with what it saw. A record is closed from that observation, never from a release sent. **The
reminder of kept checkouts covers what stays** (the owner, October 13, 2026): such a checkout is
listed to main at its start and by `rewake space list` until it is merged or removed, with the
Git commands that clear it. rewake removes no checkout itself in this step: what is inside is
work, and only main or the person knows whether it is wanted.

**A conversation ends, the process goes on.** `clear` and `resume` end main's conversation and
not its process; for `clear` no new `session.start` fires (the skill). Spaces belong to main's
run, not to its conversation, so these reasons send nothing and close nothing: workers go on,
and their mail with them.

**main's wrapper is killed.** The kernel closes its pipes and the terminal's master: each worker's
wrapper sees the end of its liveness pipe and the hang-up, and runs the same bounded shutdown on
its own. The next launch of a main in that room reads the records still open whose main's run is
proven dead,
marks them closed "main ended without closing" with what it sees of each worker and checkout, and
the reminder covers the checkouts.

**One space closed.** `rewake space close <name>` does the same for one space: the worker stopped,
its end observed, the checkout inspected, the record closed.

## 8. Restore

Not in this step: the first working space ends at the record. A restore — resuming a worker
with `--resume <session id>` in its checkout — is the next step's design. This step writes
everything a restore reads, and `rewake space list` shows closed spaces with
their session ids.

## 9. Commands

The smallest set is three commands in the command table:

- `rewake space open [<name>] [--harness <id>] [--role <role>] [--base <commit>]` — a change;
  main only; refused unless main's run has `SpacesCapability` live. It answers once the
  worker is started, or with the step that failed.
- `rewake space list` — a view: open spaces and closed ones, their workers' names, harnesses,
  session ids, checkouts, branches, and kept checkouts flagged.
- `rewake space close <name>` — a change; main only.

Main's model calls them from its shell, and the person with `! rewake space open` at main's
prompt. A slash command or a button for the person comes after the first space works, and
the person's UI is stage 6; tools for the model left with the mail tool on October 18, 2026
([design-api.md](design-api.md#tooltransport)).

The owner floated pi's interface as the renderer and pseudo-terminal host, with another
harness spawned inside it; it is recorded as an alternative under the open questions, not
designed here.
