# rewake 2.0

The documents of the 2.0 rebuild, made on the `v2` branch. The plan they follow and the
owner's decisions of October 5, 2026 are summed up at the head of
[revision.md](revision.md); stage 2, the design, builds on what the revision decided
and starts at [design.md](design.md).

- [revision.md](revision.md) — stage 1, the revision of 1.x: what each part of 1.x
  becomes in 2.0 (carry over, change, Codex-only, rewrite as mod, drop), the 2.0 layout
  it moves to, the open boundaries the revision settled, the layering faults to fix,
  the sizes, and the behaviours proposed to the owner for dropping. Open it first; the
  files below hold its detail.
- [revision-core.md](revision-core.md) — the revision of the core, infra and host
  packages: inbox, receipt, registry, state, sessionstate, role, brief, alias, proc,
  boottime, buildtime, grant, grantauth, worktree, control, channel, wrap and cutover,
  with the boundaries of `inbox/held*` and `inbox/reconcile.go` line by line.
- [revision-cli.md](revision-cli.md) — the revision of `internal/cli`: every command of
  the table with its verdict, the file groups, the hidden commands that go, and how the
  CLI stops importing the Claude Code adapter.
- [revision-adapters.md](revision-adapters.md) — the revision of the harness contract,
  the Claude Code adapter with its telemetry and plugin module (mechanism by mechanism
  against the mod), the Codex adapter with its gateway, and the mail tool's endpoint
  and MCP server.
- [revision-rules.md](revision-rules.md) — which numbered rules of the 1.x documents
  survive into 2.0 and which go with the 1.x migration or the Claude Code MCP
  injection: `mail-bridge-cli.md` 1–8 with their clauses, `mail-bridge-server.md` 1–11,
  `mail-bridge-launch.md` 1–8, `mail-bridge-channel.md` 1–8, and the rest.
- [revision-docs.md](revision-docs.md) — the revision of `docs/`: every document by
  group with its verdict and its 2.0 home (rules, adapters, research, archive of 1.x).
- [revision-tests.md](revision-tests.md) — the revision of the tests and tools: the
  generated spaces, fault tests and table tests worth carrying with their oracles, the
  workflow suite by fixture, `tools/`, `scripts/` and the documentation tests.

Stage 2, the design of 2.0, for review before any code:

- [design.md](design.md) — the overview: what does not change, the layout of 2.0 with
  its import rule and the test that holds it (imports, harness names outside the
  adapters, no registry), the stage plan with each stage's acceptance, and the questions
  for the owner. Open it first.
- [design-api.md](design-api.md) — the adapter API: the eight capabilities, offered and
  live, what the core does without each, the catalogue as a value, minimum versions and
  the legacy mark.
- [design-rules.md](design-rules.md) — the core rules restated for 2.0: effects E1–E8,
  the guarantees of every tool transport T1–T11, turn outcomes, the channel record
  C1–C8, what a launch adds L1–L3, grants.
- [design-claude.md](design-claude.md) — the Claude Code adapter as one mod: loading,
  what happens when it does not load, the mail tools and how a call runs, the turn
  boundary, telemetry, control, the wake candidates, and the stage 4 probe P1–P10.
- [design-codex.md](design-codex.md) — Codex: the pair a Claude Code main must serve,
  the reserved place, the capabilities expected, and what keeps Codex workers running
  until stage 5.
- [design-state.md](design-state.md) — the state root of 2.0, clearing the state of
  1.x on upgrade, one build per room.
- [design-docs-tests.md](design-docs-tests.md) — the documents of 2.0 by part, each rule
  naming its tests, the workflow suite's columns, the review files renamed.
