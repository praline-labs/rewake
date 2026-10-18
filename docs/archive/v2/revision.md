# The revision of 1.x

Stage 1 of the 2.0 rebuild, written October 5, 2026 on the `v2` branch (cut from
`main` at `aee71ea`). For every part of 1.x — each package under `internal/`, `cmd/`,
`tools/`, `test/workflow`, the Claude Code plugin module and each group of documents —
it states what 2.0 does with it, why, what moves with it, and where it goes. It decides
no design: the adapter interface, the core rules and the minimum versions are stage 2.
Every fact here was read in the code or the documents of that commit; line counts are
`wc -l`, and a boundary inside a file is given by its lines.

## The decisions it rests on

The owner decided on October 5, 2026:

1. Claude Code is supported from 2.1.287; everything on the Claude Code side goes through
   one official mod.
2. 2.0 is built almost from scratch on the `v2` branch of this repository: the Claude
   Code legacy goes, the architecture is laid out properly, adapters are ready for
   other harnesses; proven parts are carried over with their tests.
3. There is no migration of 1.x state: it is cleared on upgrade and sessions are
   restarted. Legacy conversion, the protocol cutover, `rewake settle` and the legacy
   marks for older harness versions go.
4. On Codex, rewake reads the installed plugin manifests and `.mcp.json` from disk and
   looks for a server named `rewake`: found refuses the launch, unreadable gives no
   tool; `plugin/installed` and `plugin/read` cross-check; the limits are written down.
5. The CLI vocabulary of 1.x stays; only what this revision finds superfluous changes.
6. Delivery on Claude Code (`$.prompt.submit`, today's socket, or the harness's native
   messaging socket) is decided by a live probe, not up front.
7. The third harness is not chosen; the capability interface must let it be added
   without touching the core.
8. The person's UI (pane, status) comes after the core, as its own stage.
9. The mail tools are several separate tools — inbox, send, pending and the rest — each
   with its own argument schema and a short plain description, not one tool taking CLI
   words. The model sees `mcp__rewake__<name>` (a mod cannot register a name without
   that prefix); Codex's MCP server offers the same set with the same schemas.

## The verdicts

| Verdict | Meaning |
|---|---|
| carry over | moves to 2.0 essentially as it is, possibly under a new package path |
| change | kept, but its contract or content changes; what changes is named |
| Codex-only | kept, and serves only the Codex adapter |
| rewrite as mod | the function survives; its Claude Code side becomes the mod (JavaScript) and the Go code that receives the mod's events |
| decided by probe | the Claude Code delivery path: kept, rewritten or dropped by the stage 4 probe (decision 6) |
| drop | removed; the reason is one of: 1.x migration, an old harness version, the Claude Code MCP injection, superfluous |

## Every part in one line

Details and citations are in the file named in the last column.

| Part | Verdict | Why | 2.0 home | Detail |
|---|---|---|---|---|
| `cmd/rewake` | carry over | builds the catalogue explicitly instead of importing it for its `init` | `cmd/rewake` | [core](revision-core.md) |
| `internal/cli` | change | the vocabulary stays; four hidden Claude hook commands and `settle` go; it stops importing the Claude adapter | `internal/cli` | [cli](revision-cli.md) |
| `internal/inbox`, mail model | carry over | letters, tasks, reports, owed, awaited, questions, notes, withdraw, edit — the proven core | `core/mail` | [core](revision-core.md#inbox) |
| `internal/inbox`, M1 journals and seam | carry over | turn-end journals, marks, read clock, stop, reconcile, the plan/apply seam; minus the 1.x branches | `core/mail` | [core](revision-core.md#inbox) |
| `internal/inbox`, delivery server | change | `Server`, batches, window, watch move to the host; `held.go` follows the Claude delivery probe | `host` | [core](revision-core.md#the-delivery-server-and-heldgo) |
| `internal/inbox`, conversion and held-for-successor | drop (1.x migration) | `conversion*.go`, `held_take.go` and ~280 lines inside other files exist only for an earlier build's records | — | [core](revision-core.md#the-1x-migration-inside-inbox) |
| `internal/receipt` | carry over | idempotent call operations keyed by run, conversation, turn and digest | `core/receipt` | [core](revision-core.md#receipt) |
| `internal/registry` | change | sessions, names, liveness, epochs stay; `runs.go`, `BuildStamp`, `EarlierBuild*`, `CodexHome` go | `core/registry` | [core](revision-core.md#registry) |
| `internal/state` | carry over | paths, rooms, locks, the fault seam; `ToolConfigPath` and `KeeperAddress` go | `infra/state` | [core](revision-core.md#infra) |
| `internal/sessionstate` | carry over | the snapshot language of telemetry; one legacy branch goes | `core/session` | [core](revision-core.md#sessionstate-role-brief-alias) |
| `internal/role`, `internal/brief` | change | the old `worker` id goes; texts about grants and the mail tool are derived from capabilities and the new tool set | `core/role`, `core/brief` | [core](revision-core.md#sessionstate-role-brief-alias) |
| `internal/alias` | carry over | launch aliases; `harness.Flag` moves to the adapter API | `host/launch` | [core](revision-core.md#sessionstate-role-brief-alias) |
| `internal/proc`, `boottime`, `buildtime` | carry over | process, boot id and build stamp utilities | `infra` | [core](revision-core.md#infra) |
| `internal/cutover` | drop (1.x migration) | proves only that an earlier build stopped writing a mailbox | — | [core](revision-core.md#cutover) |
| `internal/grant`, `grantauth` | carry over | grant policy and the unforgeable authority are harness-neutral; the Claude keeper waits for stage 6 | `core/grant` | [core](revision-core.md#grant-grantauth-and-worktree) |
| `internal/worktree` | carry over | no harness in it beyond a record field | `host/worktree` | [core](revision-core.md#grant-grantauth-and-worktree) |
| `internal/control` | carry over | the file protocol for compact, interrupt and clear, served by both adapters | `core/control` | [core](revision-core.md#control-and-channel) |
| `internal/channel` | change | the channel record is any tool's, not Codex's; its Claude MCP branches go, its Codex branches leave the core | `core/channel` | [core](revision-core.md#control-and-channel) |
| `internal/wrap` | change | the host: launch lifetime, claim, signals, notices; MCP lifecycle moves to the Codex adapter, cutover goes | `host` | [core](revision-core.md#wrap) |
| `internal/harness` contract | change | optional interfaces become the named capabilities; the global registry goes | `adapter` | [adapters](revision-adapters.md#the-harness-contract) |
| `internal/harness` checks and gates | Codex-only | the name checks and gates exist for an injected MCP server | `adapter/codex` | [adapters](revision-adapters.md#checks-and-gates) |
| `internal/harness/catalog` | change | the one place that knows every adapter, without `init` | `adapter/catalog` | [adapters](revision-adapters.md#the-harness-contract) |
| `internal/harness/claude`, launch | change | flags, resume, worktree, protected dirs stay; socket, settings hooks, status tap and MCP injection go | `adapter/claude` | [adapters](revision-adapters.md#claude-code) |
| `internal/harness/claude`, delivery | decided by probe | socket, lane and delivery receipts | `adapter/claude` | [adapters](revision-adapters.md#delivery-decided-by-probe) |
| `internal/harness/claude`, MCP injection | drop (MCP injection) | `mailtool*.go`, `managed.go` | — | [adapters](revision-adapters.md#claude-code) |
| `internal/harness/claude/plugin.js` | rewrite as mod | becomes the one mod: tools, turns, usage, compact, abort, wake | `adapter/claude/mod` | [adapters](revision-adapters.md#claude-code-mechanism-by-mechanism) |
| `internal/harness/claude/telemetry` | rewrite as mod | becomes the receiver of the mod's events; decode, tap, status line and window go | `adapter/claude` | [adapters](revision-adapters.md#telemetry) |
| `internal/harness/codex` | Codex-only | the private app-server and its launch; G2 lands in its mail-tool check | `adapter/codex` | [adapters](revision-adapters.md#codex) |
| `internal/harness/codex/gateway` | Codex-only | the terminal gateway; the `codex <0.157.1` branches go | `adapter/codex/gateway` | [adapters](revision-adapters.md#codex) |
| `internal/bridge`, `bridge/endpoint` | change | ticket, parts, end gate and the wrapper endpoint are tool core; the Claude hook half goes | `tool` | [adapters](revision-adapters.md#the-mail-tool-bridge-endpoint-and-server) |
| `internal/bridge/server` | Codex-only | the MCP stdio server; one `rewake` tool becomes the tool set of decision 9 | `tool/mcp` | [adapters](revision-adapters.md#the-mail-tool-bridge-endpoint-and-server) |
| `tools/release`, `checksummary`, `harnesscache`, `scripts/` | carry over | release, summary and harness cache; minimum versions change | `tools/` | [tests](revision-tests.md#tools-and-scripts) |
| `tools/standin` | rewrite as mod | answers with a mail-tool call; the call becomes a mod tool | `tools/standin` | [tests](revision-tests.md#tools-and-scripts) |
| `test/workflow` | change | neutral machinery and Codex fixture stay; the Claude fixture is rebuilt as a mod host | `test/workflow` | [tests](revision-tests.md#the-workflow-suite) |
| `docs/` | change | rebuilt as rules, adapters, research and an archive of 1.x | `docs/` | [docs](revision-docs.md) |

## The 2.0 layout

The direction of the plan (core, adapter API, adapters, cli, infra), refined by what the
code showed: the delivery loop and the wrapper's life are neither core nor adapter, so
they get a `host` layer; the mail tool's endpoint is used by an adapter but knows no
harness, so it gets a `tool` layer.

```
cmd/rewake              main: builds the catalogue, hands it to cli
internal/
  infra/                state, proc, boottime, buildtime — no project imports
  core/                 knows no harness
    mail/               inbox model + M1 journals, marks, read clock, stop, reconcile, seam
    receipt/            call operations and their records
    registry/           sessions, names, liveness, epochs
    session/            sessionstate snapshot
    role/  brief/       roles, playbooks, briefings
    grant/              grant policy and authority (grant + grantauth)
    control/            compact / interrupt / clear requests as files
    channel/            the channel record of a launch
  host/                 the wrapper: launch lifetime, claim, signals, availability,
                        departure, notices, grants keeper, the delivery loop
                        (inbox Server, batch, window, watch), worktree, launch aliases
  adapter/              the capability interfaces and the catalogue
    claude/             launch flags, resume, worktree, protected dirs, permission logic,
                        mod events receiver; mod/ holds the module and its manifest
    codex/              app-server, launch, mail-tool injection and checks (G2), gates
      gateway/          the terminal gateway
  tool/                 ticket, parts, end gate, wrapper endpoint, client
    mcp/                the MCP stdio server and its child (Codex)
  cli/                  words, help, exit codes — one command table; imports no adapter
tools/  test/workflow/  scripts/
```

The rule the layout carries: `core` imports only `infra`; `tool` imports `core` and
`infra`; `adapter/*` imports `adapter`, `tool`, `core`, `infra`; `host` imports
`adapter` (the interfaces only), `tool`, `core`, `infra`; `cli` imports `host`,
`adapter` and `adapter/catalog`, never an adapter package. Stage 2 may rename packages;
a test over `go list` should hold the import rule.

## Open boundaries, settled

- **`inbox/held.go` is not 1.x migration.** It is the delivery server's half of a notice
  the harness has parked: the Claude lane is its only producer of `inbox.Held`
  (`harness/claude/lane.go:233`, `harness/claude/receipt.go:79`), received in
  `inbox/serve.go:183`. Verdict: change, decided with the Claude delivery probe; it
  moves to `host` with the delivery loop.
- **`inbox/held_take.go` is.** `TakeHeld` hands reports held for an earlier build's run
  to its successor; its one caller is `wrap/cutover.go:49`. Drop.
- **`inbox/journal_held.go` splits.** The this-build branch of `deliverReport`
  (24–35), `publishReport` (145–178) and `publishMarked` (180–212) are turn-end
  recovery and stay; the earlier-build branch (36–88), `heldSuccessor`, `hold`,
  `mootHeld` and `tellNotices` go. No report is ever held because its receipt is
  undetermined: on the journal protocol an unreadable record gives `UnknownRecordError`
  and a stop (`inbox/stop.go`), not a hold.
- **`inbox/reconcile.go` splits.** `Reconcile`, `plan`, `reconcileRecords`,
  `MailboxStopped`, `inspectMailbox` and `readEveryWait` stay; the conversion block,
  the `conversion`/`receipts`/`leftovers` fields, the check for a wait without a place
  on the read clock (262–271) and `Settle` (281–330) go.
- **`rewake settle` goes.** It answers a report that an earlier build may have written
  with neither a letter nor a mark (`cli/settle.go:28`); the only producer of that
  condition is `inbox/conversion_decide.go` (`StoppedError` at :209). Journal-protocol
  reports always carry evidence. The stop on an unreadable record stays and needs no
  command.
- **Grants are core; their application is an adapter capability.** `grant` and the
  authority of `grantauth` know no harness; `grantauth/keeper.go` serves the Claude
  permission hook and is decided in stage 6 with the mod's `classic.PermissionRequest`.
- **Worktree is host.** `internal/worktree` imports no adapter; the harness joins only
  through `harness.WorktreeHarness` (`harness/worktree.go`).
- **The channel record is core, and harness-free.** A mod's tool can fail to load
  (`--bare`, `disableAllHooks`, managed policy) just as an MCP server can. What branches
  on a harness id today — the Codex conversation selection (`channel/selection.go:57`)
  and the Codex paths of the fold (`channel/history.go:202,287`) — leaves the core:
  stage 2 moves it to the Codex adapter behind a capability or generalizes it into
  neutral events and data. No harness-specific fold stays in `core/channel`.

## Layering faults to fix

- **The plan's claim that `internal/harness` imports both adapters does not hold.**
  `go list` shows `internal/harness` importing neither, in code or tests; only
  `internal/harness/catalog` imports `claude` and `codex`, by design. The real fault
  is the global registry: `harness.Register` fills a package slice from `catalog`'s
  `init` (`harness/harness.go:13-23`), and `cmd/rewake` imports the catalogue for that
  side effect. Fix: an explicit catalogue value passed from `cmd/rewake`.
- **`internal/cli` imports the Claude adapter** in five files: `granthook.go`
  (`claude.GrantCall`), `telemetry.go`, `turnended.go`, `pending.go`, `pending_turn.go`
  (all through `claude/telemetry`); `pending.go:28-30` hard-codes the Claude harness id.
  Fix: the turn-start record moves to core and is written by the TurnBoundary
  capability; hidden commands are registered by the adapter that needs them.
- **Core functions live in `cli` and leak out.** `cli.ReportCompletion`,
  `cli.AcknowledgeRead` and `cli.ToolWords` are handed to `wrap` (`cli/launch.go:78,89`)
  and called from adapter tests. They move to `core/mail` and `tool`.
- **The core names a harness.** `channel/record.go:56` defines `Claude Harness =
  "claude"`; `registry.Session.CodexHome` keeps a Codex field in the core record;
  `grant/rules.go:317` lists `.claude`, `.codex`, `.agents` as shielded. Each comes from
  the adapter instead (`ProtectedDirs` widened to metadata directories).
- **`internal/harness` mixes the contract with core types and Codex checks.** It
  imports `bridge`, `channel`, `grant`, `grantauth`, `inbox`, `registry`,
  `sessionstate`; `Deliver` hands an adapter the whole `registry.Session`. The adapter
  API takes narrow values; checks and gates move to `adapter/codex`.
- **Telemetry is transport, state fold and publisher at once.** `claude/telemetry`
  imports `control`, `harness` and `inbox` to publish a stopped turn. The fold stays in
  the adapter; publishing goes through the TurnBoundary capability.
- **The host builds the MCP endpoint.** `wrap/mailtool.go` and `wrap/channel.go` import
  `bridge` and `bridge/endpoint` directly. The host keeps a tool lease; the Codex
  adapter owns the name checks and the injection.

## Sizes

Lines of Go (non-test) and of tests, by verdict, summed over the per-part tables of the
detail files; split files are cut at the lines named there, so each figure is within
about 1%. The totals match the tree: 52 167 lines of code plus 337 of `plugin.js`, and
86 154 lines of tests.

| Verdict | Code | Tests |
|---|---:|---:|
| carry over | 26 500 | 37 800 |
| change | 7 500 | 7 500 |
| Codex-only | 11 850 | 27 850 |
| rewrite as mod | 2 450 | 8 150 |
| decided by probe | 650 | 450 |
| drop | 3 700 | 4 400 |
| total | 52 650 | 86 150 |

Documents: of 20 666 lines in `docs/*.md`, about 11 600 stay as rules (carry over or
change), 4 200 are Codex-only, 600 are rewritten for the mod, 330 are dropped and 3 900
go to the archive with the 6 383 lines of `docs/roadmap/` ([docs](revision-docs.md)).

## Proposals for the owner

Each is a proposal, not decided. Evidence is in the file named.

1. Drop the hidden commands `observe`, `status-tap` and `bridge-hook`; fold `grant-hook`
   into the mod in stage 6 ([cli](revision-cli.md#hidden-commands)).
2. Drop the Codex notify branch of `turn-ended`: rewake installs no notify
   (`docs/launch.md:192`) ([cli](revision-cli.md#hidden-commands)).
3. Drop `--no-mail-tool` on Claude Code, where the tool is part of the mod; keep it on
   Codex only if G2 leaves a reason to launch without the tool
   ([cli](revision-cli.md#proposals)).
4. Stop replacing the person's status line (the tap): the mod's `session.usage` and
   `session.measure` give context, window, limits and cost
   ([adapters](revision-adapters.md#proposals)).
5. Drop the porting of the auto-compact window parser (`telemetry/window.go`) if the
   probe shows `session.usage` plus the environment suffice
   ([adapters](revision-adapters.md#proposals)).
6. Replace the 250 ms poll of the control directory with a stream from a child the mod
   spawns ([adapters](revision-adapters.md#proposals)).
7. Keep one WebSocket client for Codex instead of two (`codex/websocket.go` and
   `gateway/websocket*.go`) ([adapters](revision-adapters.md#proposals)).
8. Replace the exact-version gates (`harness/gates.go`, tool on Codex only at 0.159.0)
   with explicit minimum versions per adapter ([adapters](revision-adapters.md#checks-and-gates)).
9. Rename the review-round test files (`review_*_test.go`) by subject
   ([tests](revision-tests.md#proposals)).
10. Fold the overlapping documents: `delivery*.md`, the twelve `mail-bridge-*.md`, the
    `remote-control*.md`, and archive `check-runner*.md`, which was never built
    ([docs](revision-docs.md#overlaps)).
11. Keep the legacy mark as a mechanism — the rules of `docs/legacy.md` and
    `docs/legacy_test.go` with an empty table — rather than drop it with the marks: the
    first raise of a minimum version in 2.x needs it again
    ([rules](revision-rules.md#other-numbered-lists)).
12. Keep `rewake guide` and `rewake edit` as they are: both duplicate something
    (no-arguments guide; withdraw plus send) but are harmless words of the vocabulary
    ([cli](revision-cli.md#proposals)).

## Open for stage 2

- Delivery on Claude Code and the fate of `held.go`, the lane and `turn_hold.go`: the
  stage 4 probe ([adapters](revision-adapters.md#delivery-decided-by-probe)).
- The transport of the mod's tools: the wrapper endpoint with tickets and the end gate,
  or another. Not open: rules 2, 6, 7 and 8 of `mail-bridge-server.md` bind the mod
  whatever is chosen — binding to the call, a read only on proof of the whole result,
  no commit past the turn's end; a reading tool without that proof refuses before its
  effect ([rules](revision-rules.md#mail-bridge-servermd-rules-111)).
- The confirmation of an unmarked turn (`cli/turn_hold.go`) carries over as semantics;
  its candidate mechanism is `classic.Stop` answering `block`, which the 2.1.289 types
  allow before `turn.complete`. The stage 4 probe tests the block, the continuation, a
  repeated stop with `stop_hook_active`, and its agreement with `turn.complete` and an
  aborted turn without a double report ([rules](revision-rules.md#mailbox-recordsmd-turn-outcomesmd-mail-bridge-turnsmd)).
- Minimum versions: Claude Code 2.1.287 is decided; Codex has no floor constant today
  (`codex/server_version.go:31` only pins a note to 0.155.1); 0.157.1 follows from the
  dropped legacy marks.
- How 1.x state is cleared on upgrade (decision 3).
