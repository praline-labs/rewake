# Revision: the CLI

`internal/cli`: 8 874 / 12 079, 65 code files and 78 test files. The vocabulary stays
(decision 5); what goes is the code that exists for Claude Code's command hooks, the
MCP injection and the 1.x migration. The overview is in [revision.md](revision.md).

## The command table

From `internal/cli/registry.go:60-282` and `registry_internal.go:9-73`.

| Command | File | Verdict | Why |
|---|---|---|---|
| `claude`, `codex` | `launch.go` (247) | change | built from the adapter's Launch capability; `GatesAssumedEnv` (`launch.go:57-62`) is rethought with the gates |
| `worktree ls`, `land`, `finish`, `rm` | `worktree*.go`, `launch_worktree.go` | carry over | no harness in it; `--worktree` reaches the adapter through `WorktreeHarness` |
| `list`, `whoami` | `sessions.go`, `session_state.go`, `session_table.go` | carry over | shows the session snapshot |
| `send` with `--notify`, `--question`, `--to`, `--wait`, `--grant-dir`, `--grant-dir-broad`, `--grant-git` | `send*.go`, `registry_send.go` | carry over | the mail core; the grant flags ask the Permissions capability |
| `withdraw`, `edit` | `withdraw*.go`, `edit.go` | carry over | |
| `inbox` with `--peek`, `--message`, `--owed`, `--awaited`, `--next` | `inbox*.go`, `output_parts.go`, `read_ack.go` | carry over | `--next` and parts serve the tool's size bound |
| `pending` | `pending.go`, `pending_turn.go` | change | the Claude check (`pending.go:65`, `claudeHarnessID` at 28–30) and the help text about the held turn end become a TurnBoundary question |
| `retry` | `retry.go` (100) | carry over | the way back to a receipt's operation; its help, which speaks of one tool (20–24), is rewritten for the tool set |
| `compact`, `interrupt` | `steer.go` (238) | change | the Control capability; the 5 s pickup bound is the plugin's 250 ms poll (`steer.go:51-56`) and follows the mod's mechanism |
| `accept` | `accept.go` (126) | Codex-only | taking a Codex resume into another conversation (`harness.Accepting`) |
| `settle` | `settle.go` (138) | drop (1.x migration) | "a report an earlier build may have written" (`settle.go:28`) |
| `guide` | `output.go` | carry over | proposal 11 |
| hidden: `turn-ended` | `turnended.go` (210) | rewrite as mod | Claude Code's Stop hook; the core of it (`endTurnContext`) carries over |
| hidden: `observe` | `telemetry.go:15-37` | drop | Claude hooks and plugin reporting; replaced by the mod's events |
| hidden: `status-tap` | `telemetry.go:39-56` | drop | the status-line tap |
| hidden: `grant-hook` | `granthook.go` (52) | rewrite as mod, stage 6 | Claude `PreToolUse`/`PermissionRequest` |
| hidden: `bridge-serve` | `bridge_serve.go` | Codex-only | the MCP stdio server |
| hidden: `bridge-hook` | `bridge_serve.go:70-79` | drop (MCP injection) | Claude hooks around the MCP tool |

Global flags `--json`, `--help`, `--version` carry over; `Version = "1.0.3"`
(`registry.go:15`) becomes 2.0.0 through the release.

## Hidden commands

Six hidden commands today (`registry_internal.go:9-73`); four exist only for Claude Code's
command hooks or the MCP injection. In 2.0 the mod reports events itself, so `observe`,
`status-tap` and `bridge-hook` have no caller; `grant-hook` waits for stage 6;
`turn-ended` keeps its core path for whatever reports a turn end from outside.

The Codex branch of `turn-ended` (`turnended.go:52-56`, `turn_result.go` around 35–47,
`agent-turn-complete`/`task_complete`) is dead today: rewake installs no notify
(`docs/launch.md:192`), and a payload without a boundary is refused on every attempt
(`docs/turn-end-recovery.md:30`). Codex reports through the gateway and
`cli.ReportCompletion`.

In 2.0 a hidden command an adapter needs is registered by that adapter in the table;
the names `harness.Observe`, `GrantHook`, `StatusTap` leave the shared package.

## File groups

| Group | Code | Tests | Verdict | 2.0 home |
|---|---:|---:|---|---|
| parser, help, guide, output (`args.go`, `run.go`, `render.go`, `model.go`, `output.go`, `errors.go`, `naming.go`, `version.go`) | 1 175 | 1 337 | carry over | `cli` |
| the table (`registry.go`) | 333 | — | change: the notes "Claude Code receives through its inbox socket" (around 285–333) are derived from capabilities | `cli` |
| hidden group (`registry_internal.go`) | 73 | — | change | `cli`, filled by adapters |
| send kinds (`send*.go`, `sent_*.go`, `registry_send.go`) | 1 034 | ~1 318 | carry over | logic to `core/mail`, words in `cli` |
| edit and withdraw | 403 | 300 | carry over | `core/mail` |
| inbox (`inbox*.go`, `output_parts.go`, `read_ack.go`) | 1 734 | 1 283 | carry over | `core/mail` + `cli` |
| pending and turn end (`pending*.go`, `turn_*.go`, `completion.go`, `turnended.go`, `turn_hold.go`) | 870 | 2 484 | change: ~600 lines of journals and report publication carry over; the Claude hook payload (`turn_result.go`), `turn_hold.go` (95) keeps its semantics on the mod's `classic.Stop`, and the Claude checks in `pending*.go` are rewritten for the mod | `core/mail` + `adapter/claude` |
| Claude hooks (`telemetry.go`, `granthook.go`) | 108 | 82 | drop / rewrite as mod | — |
| grants, the CLI side (`send_dir.go`, `send_git.go`, `send_grant.go`) | 255 | 584 | change: through the Permissions capability | `cli` |
| worktree, the CLI side | 738 | 1 073 | carry over | `cli` + `host` |
| the tool's CLI side (`bridge_run.go`, `bridge_serve.go`, `bridge_surface.go`, `journal*.go`, `retry.go`, `shell_channel.go`) | 877 | 1 789 | change: receipts and rules 1–8 carry over to `tool`/`core`; the server half is Codex-only; `bridge-hook` goes; `ToolWords` becomes the per-tool schemas of decision 9 | `tool` |
| self and sessions | 525 | 563 | change: `ownRun` stays; `errUpgraded`, `errNoRun`, `refuseUpgraded` (`self.go:17-31,64-71`) go | `cli` |
| settle and the 1.x branches | 138 + ~60 | 449 + ~80 | drop | — |
| control (`steer.go`, `accept.go`) | 364 | 1 056 | change / Codex-only; `gateway_*_test.go` (559) test the gateway through the CLI | `cli` |
| launch | 247 | 61 | change | `cli` |

The scattered 1.x branches: `errors.Is(err, errUpgraded)` in `bridge_run.go:87`,
`inbox.go:53-55`, `output_parts.go:134`, `pending.go:56`, `retry.go:47`,
`send.go:130-133`, `sent_ref.go:18`, `steer.go:95`, `turnended.go:41-45`,
`sessions.go:92`, `self.go:92`; `EarlierBuildEpoch` in `turn_reports.go:141`. Tests
that go: `settle_caller_test.go` (47), `upgraded_test.go` (172),
`turn_legacy_receipt_test.go` (230, the one legacy mark in `cli`).

## Importing the adapter

| File | Import | What it uses |
|---|---|---|
| `granthook.go:9` | `harness/claude` | `claude.GrantCall` on the hook's payload |
| `telemetry.go:7` | `claude/telemetry` | `DecodeHook`, `DecodePlugin`, `Send`, `RecordTurnStart`, `RunTap` |
| `turnended.go:12` | `claude/telemetry` | `ReadTurnStart`, `RecordTurnStart` (70, 94) |
| `pending_turn.go:5` | `claude/telemetry` | `ReadTurnStart` (66) |
| `pending.go:11` | `claude/telemetry` | `ReadTurnStart` when the harness is Claude (65) |

The route in 2.0:

1. The turn start — the `.turn` file of `telemetry/turnstart.go:28-61` — becomes a core
   record. The adapter's TurnBoundary writes it (the mod's `turn.start`; the gateway on
   Codex), the CLI reads it from core, and the harness-id check becomes a question to
   the capability.
2. `observe` and `status-tap` go; the mod reports to the wrapper itself.
3. `grant-hook` lives in the Claude adapter if it survives stage 6, registered by it.
4. `ReportCompletion`, `AcknowledgeRead` and `ToolWords` move out of `cli` into
   `core/mail` and `tool`; `cli/launch.go:78,89` stops handing them to `wrap`, and
   adapter tests (`codex/report_boundary_test.go:84`, `bridge/server/rig_test.go:58,109`)
   stop importing the CLI.

## Proposals

Proposals for the owner, not decided:

- Drop `--no-mail-tool` on Claude Code, where the tools are part of the mod; on Codex
  keep it only if G2 leaves a reason to launch without the tool (`registry.go:246-247`).
- Keep `rewake guide` beside the no-argument guide, and `rewake edit` beside withdraw
  plus send: duplicates, but harmless words of the vocabulary.
- Generate the help texts that name a transport ("its rewake plugin on Claude Code, its
  wrapper on Codex", `registry.go` around 186–200) from capabilities.
- Retire `GatesAssumedEnv` with the exact-version gates.
