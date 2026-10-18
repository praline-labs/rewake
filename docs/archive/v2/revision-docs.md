# Revision: the documentation

`docs/` holds 97 documents at the top (20 666 lines), 80 in `docs/roadmap/` (6 383) and
four documentation tests (793). The plan rebuilds it in four parts: **rules** (how 2.0
works, harness-neutral), **adapters** (Claude Code mod, Codex), **research** (facts
about each harness, dated) and **archive-1.x** (records of 1.x, never edited). The
overview is in [revision.md](revision.md).

Verdicts as in the overview, plus **archive-1.x**: a record of 1.x moved unchanged to
the archive. **drop** means the document leaves the tree; git keeps it.

## By verdict

| Verdict | Files | Lines |
|---|---:|---:|
| carry over | 13 | 1 995 |
| change | 36 | 9 572 |
| Codex-only | 22 | 4 212 |
| rewrite as mod | 4 | 628 |
| drop | 2 | 326 |
| archive-1.x | 20 | 3 933 |
| roadmap (all archive-1.x; the index is rewritten) | 80 | 6 383 |

## How it works — the rules

| Document | Lines | Verdict | What changes |
|---|---:|---|---|
| `flow.md`, `flow-endings.md` | 390, 125 | change | the Claude hooks, socket and MCP acts give way to the mod; journals stay |
| `design.md` | 357 | change | rewritten for core, host, adapters and capabilities; owner decisions keep their dates, October 5 added |
| `session-record.md` | 61 | change | the build stamp used only by the cutover goes |
| `roles.md` | 139 | carry over | |
| `code.md` | 110 | change | the 2.0 tree; `layout_test.go` reads it |
| `launch.md` | 298 | change | the Claude hook settings become the mod's loading; Codex parts move to the adapter document |
| `launch-defaults.md`, `worktree.md` | 187, 405 | carry over | `worktree.md` is over 400 lines and is split by subject when moved |
| `delivery.md`, `delivery-turn-end.md` | 381, 171 | change | the Claude socket and hook channel go; the conversion path goes |
| `delivery-owed.md`, `delivery-sent.md`, `inbox-groups.md` | 135, 266, 147 | carry over | |
| `delivery-adapters.md` | 147 | change | split into the two adapter documents |
| `mailbox-records.md`, `turn-end-recovery.md` | 181, 326 | change | the 1.x formats and the cutover go ([rules](revision-rules.md#turn-end-recoverymd)) |
| `turn-outcomes.md` | 234 | change | the Claude hook source becomes the mod's turn events |
| `report-publication.md` | 92 | change | the completion boundary through TurnBoundary |
| `session-state.md` | 264 | change | the Claude source becomes `session.usage` and `session.measure` |
| `session-activity.md` | 135 | carry over | |
| `grants.md`, `grants-resume.md` | 318, 167 | change | application per adapter through Permissions |
| `grants-authority.md`, `git-grants.md` | 102, 71 | carry over | |
| `permission-requests.md` | 385 | change | a design not built; redone for stage 6 on the mod and Codex |
| `conversation-reset.md` | 327 | change | a design not built; `rewake clear` through Control |
| `remote-control.md` | 348 | change | the command side stays; the served side moves to the mod |
| `remote-control-letter.md` | 119 | carry over | |
| `install.md` | 163 | change | release, minimum versions, clearing 1.x state |
| `traps.md` | 354 | change | the 1.x, legacy and MCP-injection traps go |
| `legacy.md` | 135 | change or drop | its table goes; the mechanism is a proposal ([rules](revision-rules.md#other-numbered-lists)) |
| `README.md` | 507 | change | the new map; over 400 lines today, so split with the subdirectory indexes |

## Adapters

| Document | Lines | Verdict | Home |
|---|---:|---|---|
| `claude-plugin.md`, `claude-telemetry.md`, `grants-claude.md` | 192, 118, 74 | rewrite as mod | adapters/claude |
| `gateway.md`, `codex-publication.md`, `delivery-conversation.md`, `startup-transport.md`, `native-mailbox.md`, `native-mailbox-ui.md`, `remote-control-codex.md`, `remote-control-codex-limits.md` | 204, 73, 139, 139, 87, 93, 349, 84 | Codex-only | adapters/codex |

## The mail tool

Twelve documents and 3 186 lines describe one tool; the Claude Code injection is spread
through them rather than kept in one.

| Document | Lines | Verdict | Notes |
|---|---:|---|---|
| `mail-bridge.md` | 376 | change | the design; the Claude section (112–133), per-launch settings and the preservation check go; decision 9's tool set comes in |
| `mail-bridge-cli.md` | 399 | change | rules 1–8 carry over without the migration clauses |
| `mail-bridge-server.md` | 371 | Codex-only, rules to `tool` | [rules](revision-rules.md#mail-bridge-servermd-rules-111) |
| `mail-bridge-turns.md` | 272 | change | Claude observer part rewritten for the mod's turn ids |
| `mail-bridge-checks.md` | 292 | change | the fault test from logs carries over to testing; the stage reviews go to the archive |
| `mail-bridge-launch.md`, `mail-bridge-launch-codex.md`, `mail-bridge-version.md` | 359, 184, 81 | Codex-only | the Claude injection (managed MCP, `.mcp.json`, install-path version) goes |
| `mail-bridge-channel.md` | 369 | change, rules to core | [rules](revision-rules.md#mail-bridge-channelmd-rules-18) |
| `mail-bridge-channel-codex.md`, `mail-bridge-channel-failures.md` | 155, 67 | Codex-only / change | |
| `mail-bridge-live.md` | 261 | archive-1.x | the plan and run of October 4, 2026 |
| `research-mail-tool.md` | 199 | change | Codex facts stay; the Claude MCP and managed facts go to the archive |

## Research

| Document | Lines | Verdict |
|---|---:|---|
| `research.md`, `research-launch.md` | 398, 397 | change: re-verified against Claude Code 2.1.289 and the supported Codex, split by harness where they mix |
| `research-protocol.md`, `research-codex.md`, `research-codex-conversation.md`, `research-codex-live-checks.md` | 332, 366, 96, 127 | Codex-only |
| `research-claude-control.md`, `research-claude-actions.md` | 384, 187 | change: the mod's API supersedes part; the basis for the mod's research |
| `research-permissions.md`, `permission-requests-sources.md` | 171, 59 | change: re-verify |
| `research-worktree.md`, `prior-art.md` | 150, 42 | carry over |
| `harness-features.md` | 165 | change: becomes the capability matrix |
| `gateway-native-evidence.md`, `continuation-permissions.md` | 141, 124 | Codex-only, research (both observed on 0.154.0) |

## Testing

| Document | Lines | Verdict |
|---|---:|---|
| `testing.md`, `testing-cases.md` | 310, 356 | change: tiers and commands stay; the Claude cases are rewritten; `controls_test.go` reads the controls table |
| `testing-plugin.md` | 244 | rewrite as mod |
| `testing-pool.md` | 97 | carry over |
| `remote-control-tests.md` | 173 | change |

## Records — the archive of 1.x

`reviews.md` 351, `reviews-later.md` 385, `reviews-native-ui-2026-09-20.md` 115,
`transport-milestones-2026-09-17.md` 76, `claude-parity-2026-09-21.md` 179,
`intermittent-bugs.md` 185, `thread-lock-probes.md` 204,
`thread-ownership-investigation.md` 293, `server-observation.md` 70,
`inbox-acceptance.md` 59, `native-mailbox-acceptance.md` 107, `native-mailbox-check.md`
86, `native-mailbox-ui-check.md` 77, `native-terminal-progress.md` 58,
`turn-end-recovery-findings.md` 88, `check-runner.md` 270, `check-runner-proposal.md`
383, `check-runner-scenarios.md` 214, `mail-bridge-live.md` 261, `work-queue.md` 472,
and all of `roadmap/`. A record is not edited when it moves (`AGENTS.md`, "Keeping the
documentation true"); links into it are rewritten by the map's checks.

Dropped: `protocol-cutover.md` (191, the 1.x migration) and the table of `legacy.md`.

## Overlaps

Proposals to fold, not decided:

- `delivery.md` has stub sections that point to `delivery-owed.md` and
  `delivery-sent.md`, an "Adapters" section repeating `delivery-adapters.md`, and an
  "end of a turn" expanded by `delivery-turn-end.md`.
- The twelve `mail-bridge-*.md` become one tool specification (shared by both
  adapters), the effect rules in the core, and a Codex injection document.
- `remote-control*.md` (five files, 1 073 lines): the command side in the rules, the
  served side per adapter.
- `check-runner*.md` (867 lines) describe a runner never built while `test/workflow`
  was built and described by `testing*.md`: archive.
- `turn-end-recovery.md`, `turn-end-recovery-findings.md`, `delivery-turn-end.md`,
  `mailbox-records.md` and `protocol-cutover.md` state rules 7–8 from five angles.
- `thread-lock-probes.md` and `thread-ownership-investigation.md` carry the same status
  header (R14 closed on September 23).

## The documentation tests

| Test | Lines | Verdict |
|---|---:|---|
| `map_test.go` | 244 | carry over: every document on a map, every link and heading resolves |
| `layout_test.go` | 114 | carry over: the tree in `code.md` equals the module's packages |
| `controls_test.go` | 194 | carry over: the controls table equals the suite's mutants |
| `legacy_test.go` | 241 | with `legacy.md`: drop, or keep with an empty table (proposal) |

Outside `docs/`: `AGENTS.md` names `docs/legacy.md` (under "Code"), the review files and
the harness catalogue under "Adding a harness"; the top-level `README.md` links the map.
Both are updated when the documents they name move.
