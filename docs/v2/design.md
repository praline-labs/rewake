# The design of rewake 2.0

Stage 2 of the 2.0 rebuild, written October 5, 2026 on the `v2` branch, for review
before any code. It rests on the owner's decisions of October 5, 2026 — the nine of the
plan and the twelve answers to the revision's proposals — and on the accepted revision
([revision.md](../archive/v2/revision.md), commit `8a20bdd`). Every decision names what it rests on
and what is still unknown. The owner answered the questions of the first draft on
October 5, 2026 ([below](#the-owners-answers)); one is still open. Three later decisions
of October 13, 2026 follow them there.

| Document | What it decides |
|---|---|
| this file | the layout of 2.0, the import rule and the test that holds it, the stage plan, the owner's answers |
| [design-api.md](design-api.md) | the adapter API: the capabilities, what the core does without each, the catalogue, minimum versions |
| [design-rules.md](design-rules.md) | the core rules, restated from 1.x without its migration, and the guarantees bound to every tool transport |
| [design-claude.md](design-claude.md) | the Claude Code adapter as one mod: tools, turns, telemetry, control, wake, loading, and the stage 4 probe |
| [design-state.md](design-state.md) | the state directory of 2.0, clearing the state of 1.x, one build per room |

Where a fact is marked, the marks are those of the recon: **[doc]** official
documentation, **[live]** seen on a running harness, **[code]** read in the tree at
`8a20bdd`. A fact about the Claude Code mod API carries no mark here: the `mods` skill
holds it ([design-claude.md](design-claude.md#where-the-api-facts-live)), and a claim
about the mod that still needs a live check says so and names its stage 4 probe.

## What does not change

- **The CLI vocabulary** of 1.x stays (decision 5), less what the owner dropped on the
  revision's proposals: the hidden `observe`, `status-tap` and `bridge-hook`, `settle`,
  `--no-mail-tool` on Claude Code, and the Codex notify branch of `turn-ended`.
  `rewake guide` and `rewake edit` stay (answer 12).
- **The CLI is the interface** (answer 12). Every mail operation is a command; a session
  works through its shell and the guide. Decided by the owner, October 18, 2026: there are
  no mail tools beside the command line; the mail tool's code leaves the tree.
- **The boundaries of [project.md](../project.md#the-philosophy)**: no typing into a
  screen, and a pseudo-terminal only for a worker main starts in a space; the
  person's harness configuration is never edited; transcripts are not read; no daemon —
  what serves a session lives as long as it.
- **What the CLI promises a caller**: one command table, no arguments prints the guide,
  `--json` on every command, an unknown flag is a refusal, exit codes 0/1/2/3.

## The layout

```
cmd/rewake               main: builds the catalogue value and hands it to cli
internal/
  infra/                 state root and paths, locks, the fault seam, proc, boottime, buildtime
  core/                  knows no harness
    mail/                letters, tasks, reports, owed, awaited, questions, notes, withdraw, edit;
                         turn-end journals, marks, read clock, stop, the reconcile barrier, the seam
    receipt/             call operations and their records
    registry/            sessions, names, liveness, epochs, the live capabilities of a run
    session/             the session-state snapshot
    role/  brief/        roles, playbooks, briefings — harness words come from the adapter
    grant/               grant policy and the unforgeable authority; grants leave the code
                         (October 18, 2026), the authority stays
    control/             compact, interrupt and clear as request files
    channel/             the channel record of a run; leaves with the mail tool
  adapter/               the capability interfaces and the values they exchange; no implementation
    catalog/             the one place that names every adapter; a value, not a registry
    claude/              the Claude Code adapter; mod/ holds the module (TypeScript, answer 9) and its manifest
  host/                  the wrapper: launch lifetime, claim, signals, availability,
                         departure, notices, the delivery loop, worktree, launch aliases,
                         the endpoint the adapters reach (leaves with the mail tool)
  tool/                  the output helpers `inbox` uses; the mail tools' descriptors,
                         binding and end gate leave with the mail tool
  cli/                   words, help, exit codes — one command table; imports no adapter
tools/  test/workflow/  scripts/
```

What each owns, and where 1.x code lands, is in the revision's one-line table
([revision.md](../archive/v2/revision.md#every-part-in-one-line)); the layout above refines it in two
places. The endpoint the mod reaches is the host's (it lives as long as the wrapper),
while what a call means — the descriptor types, binding, parts, the end gate — is
`tool`'s; the descriptors themselves are built by `cli` from its command table and
passed down ([design-api.md](design-api.md#tooltransport)).

### The import rule

| Package | May import |
|---|---|
| `infra/*` | the standard library only |
| `core/*` | `infra`, other `core` packages |
| `tool` | `core`, `infra` |
| `adapter` | `core`, `infra` — the values the interfaces carry |
| `adapter/<name>` | `adapter`, `tool`, `core`, `infra`; never another adapter |
| `adapter/catalog` | `adapter` and every `adapter/<name>` — the only package that does |
| `host` | `adapter` (the interfaces), `tool`, `core`, `infra`; never an adapter |
| `cli` | `host`, `adapter`, `tool`, `core`, `infra`; never `adapter/catalog` or an adapter |
| `cmd/rewake` | `cli`, `adapter/catalog` |

Rests on: the layering faults the revision found ([revision.md](../archive/v2/revision.md#layering-faults-to-fix)) —
`cli` importing the Claude adapter in five files, the core naming a harness in three
places, the global registry filled from `init`.

### The test that holds it

An import check alone misses what the revision found most: a harness named in a string
(`channel/selection.go:57` compares the harness id with `Codex`; the role texts say "on
Codex" and "on Claude Code" in `role/playbook_main.go:16,41,45,46,49`,
`role/playbook.go:101`, `role/role.go:51`). Comparing a string needs no import. So one
test, `internal/layout_test.go`, runs with `go test` and holds three things — since the
owner's decision of October 17, 2026, the first and the third; the second is held by
review:

1. **Imports.** It loads every package of the module with its test imports
   (`go list -deps -test -json`) and
   checks each edge against the table above. A forbidden edge fails with both packages
   and the file that imports.
2. **Harness names outside the adapters.** It parses every Go file under `core`,
   `tool`, `host`, `cli` and `infra` — tests included — and fails on an identifier,
   a string literal or a comment word that names an adapter: the ids and titles the
   catalogue declares, compared without case, plus a short list of words that belong to
   one harness (`mcp__`, `CLAUDE_`, `app-server`, `.claude`, `.codex`,
   `.agents`). The list is the test's own and grows when a review finds a word; an
   exception is a line in the test with its reason, and the test fails on an exception
   nothing matches any more. Comments are included on purpose: a comment that explains
   core code by one harness's behaviour is the first step toward a branch on it.
3. **No registry.** No package under `adapter` declares `func init`, and no
   package-level variable of an adapter type or of a slice or map of them exists outside
   `adapter/catalog`'s constructor. `cmd/rewake` builds the catalogue value and passes
   it down (decision 7 needs the third adapter to be one more argument there).

Still unknown: how noisy rule 2 is on the carried code. The role texts and the channel
fold are known to fail it and are moved in stage 3; the rest is found by running it.

## The stage plan from here

Each stage is closed by its acceptance, not by code existing (`docs/roadmap/README.md`).
Every step of a stage goes through the chain of `AGENTS.md`, "Delegation and review".

| Stage | What it builds | Acceptance requires |
|---|---|---|
| 3 core | the layout and its test; `core/*`, `tool`, `host`, `cli` with the full vocabulary on the shell; the state root of 2.0 and one build per room ([design-state.md](design-state.md)); a fixture adapter for tests | closed October 13, 2026 ([its entry](../roadmap/2026-10-13-stage3-s19.md)) |
| 4 Claude Code | the probe first ([design-claude.md](design-claude.md#the-stage-4-probe)), its results written into research with the version (the first, October 13, 2026: [research-claude-mod.md](../research-claude-mod.md)); then the adapter and its mod; then the space, one step by the decision of October 18, 2026 ([stage4-space-steps.md](stage4-space-steps.md#the-build-steps)) | live: a Claude Code main and a Claude Code worker exchange a task, a question, a notify and a report, idle and busy; main opens a space, a worker started in it joins main's room, exchanges a task and its report, is reached by `rewake attach`, and ends when main's run ends, `kill -9` included; each worker's session id in the space's record for a restore (answer 7); the mod not loading is named under `--bare`, `--safe-mode` and `disableAllHooks`; a version below 2.1.287 refused |
| 4b pi | pi's adapter on its own duplex JSON-lines protocol, its extension loaded for its run, a minimum version; first Wake, TurnBoundary and Conversation, then Telemetry and Control; a read-only reviewer profile ([stage4-reading-pi.md](stage4-reading-pi.md)) | live: a Claude Code main and a pi worker exchange a task, a question, a notify and a report, idle and busy, beside Claude Code workers in the same room; a pi worker opened in a space; a version below the minimum refused |
| 5 compaction and context | the parity the room uses on 1.0.3: `rewake compact`, `interrupt` and the context in `list`, through the mod on Claude Code and natively on pi | live on both harnesses: main sees a worker's context fill, compacts it and interrupts a turn |
| 6 release 2.0.0 | live checks, briefings, notes for running 2.0 beside 1.0.3, the npm package | the full workflow suite and crosswise check; the room moved to 2.0 under its own `REWAKE_DIR`, with a way back to 1.0.3 |

Until the release, the owner works in a separate 1.x environment and the `v2` branch
releases nothing; 2.0 experiments run with their own `REWAKE_DIR`, and nothing passes
between the generations. The mixed pair — a Claude Code main with Claude Code and pi
workers in one room — is required before the release.

Decided by the owner on October 13, 2026: the next harness after Claude Code is pi, before
Codex. Decided by the owner on October 18, 2026: Codex does not return soon, and neither its
entry point nor its adapter is planned; a space is main starting its workers, without showing
their terminal inside Claude Code; grants and the mail tool leave the code; compaction and
context stay as parity. Main's decisions on the analysis the owner accepted, and the order of
work, are in [the roadmap entry](../roadmap/2026-10-18-decision.md). Stage 4 is Claude Code with
Claude Code, stage 4b Claude Code with pi.

## The owner's answers

The owner answered the design's questions on October 5, 2026. Answers 1 and 2 are superseded
by the owner's decision of October 18, 2026: the clearing of 1.x has left the code, and the mail
tools leave it.

1. **1.x state is deleted on upgrade**, not set aside, after the check that no 1.x
   wrapper is alive, with one line saying what was dropped. That check reads the
   liveness fields of a 1.x record, so 2.0 adds a legacy mark (`rewake`, dated) to its
   table ([design-state.md](design-state.md#clearing-the-state-of-1x)).
2. **The first tool set is `inbox`, `send`, `pending`, `whoami`, `retry`, `list`**
   (`cli/bridge_surface.go:21-27` plus the read-only `list`). `withdraw`, `edit`,
   `compact`, `interrupt`, `accept` and `worktree` stay shell-only until a review puts
   each on the surface ([design-api.md](design-api.md#tooltransport)).
3. **A session whose mod did not load has no live TurnBoundary**: a role that must
   report refuses to launch; a task or question to such a session exits 1, naming why;
   no new word `report` is added ([design-api.md](design-api.md#turnboundary)).
4. **A stop whose evidence cannot return is resolved by an operator decision**
   (decided October 5, 2026). The stop and its records stay until evidence returns, and
   nothing removes a record to lift it (E1, E8). Beside that, a command lets main — the
   recipient, who can check whether the report arrived — or the person record "arrived"
   or "did not arrive" for an operation whose outcome is unknown; it is recorded as an
   operator decision with delivery marked unproven, the operation's identity and the
   original evidence kept, and "did not arrive" sends the report again. Its authority,
   retry rules and tests are designed in stage 3's rules
   ([design-rules.md](design-rules.md#an-operator-decision-on-an-unknown-outcome)).
5. **No Codex code on `v2` between stages 3 and 5**: the Codex packages leave the `v2`
   tree at stage 3 (they stay on `main` and in history as reference), so the builds of
   stages 3 and 4 launch Claude Code only ([design-codex.md](../archive/v2/design-codex.md#meanwhile)).
6. **A harness version that cannot be read refuses the launch**, saying how it was
   asked; a mod against an unknown API fails silently, which is worse
   ([design-api.md](design-api.md#minimum-versions)).

Three later decisions of the owner, October 13, 2026, after stage 4's audit and its first
probe ([research-claude-mod.md](../research-claude-mod.md)):

7. **Workers close with their main** (decided October 13, 2026). No design lets a worker
   outlive the main that seated it: closing main closes every worker of its spaces, since a
   worker that outlives its main is a heavy path. What matters instead is bringing them
   back: rewake records each worker's session id — Claude Code's session id, pi's session
   id, later Codex's thread — in the space's record, beside the worker's harness, the
   worktree's path and branch, its role and name, so that main can restore the agents it
   needs later. The space's keeper may therefore live inside main's process and end with
   it; a keeper outside it, a background session, and background workers (P16) lose their
   purpose ([design-claude.md](design-claude.md#the-stage-4-probe)).
8. **A room is one main by default** (the owner's direction, October 13, 2026). In effect a
   room is now one main Claude Code session with its spaces and workers. The room is not
   removed: communication between two mains may need it, far in the future. For stage 4's
   design the default room follows the main — its name comes from the main, with no flag —
   and the room's directory-and-name scheme stays in the core
   ([design-state.md](design-state.md#the-state-root-of-20)).
9. **Go for the core, TypeScript only where a harness requires it** (decided October 13,
   2026). The core and the host side of every adapter are Go. TypeScript is written only in
   an adapter whose part inside the harness must be — the Claude Code mod, the pi
   extension. That part is small, generated or embedded by the binary for one run and
   versioned with it; the protocol between it and the host is described once, as a JSON
   schema, and the TypeScript types are generated from it.

Three more decisions of the owner, October 13, 2026, on how a space is shown. Answers 10 and
11 are superseded by the owner's decision of October 18, 2026: a space is not shown inside
Claude Code, and a person reaches a worker's terminal only with `rewake attach`.

10. **The person runs Claude Code, and the harnesses live inside it through the mod**
    (decided October 13, 2026). The approach is fixed; what stays open is only how a space's
    pseudo-terminal is shown in full ([stage4-space.md](stage4-space.md)).
11. **The full screen: a space's Text tab now, the host mode later** (decided October 13,
    2026). Now a space is drawn in its tab as styled text, and `rewake attach` in a second
    window gives its whole screen. Later, as a stage of its own and by the owner's decision on
    the principle of [project.md](../project.md#the-philosophy), a host mode, `rewake claude
    --host`: rewake holds main's terminal and switches the whole window between main and the
    workers as a transparent pseudo-terminal relay, with no multiplexer interface of its own.
    The work is only at a switch — restore the shown program's terminal modes from its
    emulator, make it redraw — and one reserved prefix key. The person's external editor
    (Ctrl+G, `EDITOR`) is never taken for this.
12. **No tmux anywhere in rewake** (decided October 13, 2026): not as the substrate a
    session runs in, not for screenshots. Screen snapshots for tests come from rewake's own
    emulator, `charmbracelet/x/vt`, over a pseudo-terminal. What agent-deck, which runs on
    tmux, teaches for the terminal, the checkouts and screen tests is gathered in
    [agent-deck-ideas.md](../archive/v2/agent-deck-ideas.md).
