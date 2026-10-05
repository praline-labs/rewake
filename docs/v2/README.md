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

Stage 3, the rules the core is built by — part A accepted in review round 4 on October 5,
2026, with the publication contract accepted in round 7; part B, the operator decision,
accepted in review round 7:

- [stage3.md](stage3.md) — the overview: what holds in every step, the build order one
  line per step, its two acceptances (part A, everything but the operator decision; part
  B, the decision), what each step needs and from where, the documents, the corrections to
  the accepted design, what is still unknown and the owner's answers of October 5, 2026.
  Open it first.
- [stage3-steps.md](stage3-steps.md) — steps S1–S6 in detail: the tests first, the
  migration's removal, the per-cause stop, the turn-end inputs, the neutral confirmation
  and the fixture, its gate; what each writes, deletes and tests, and what review checks.
- [stage3-publication.md](stage3-publication.md) — S3's first commit, found in review
  round 5: a once-publication reads its recipient's run and evidence and writes inside one
  critical section of the recipient's lock, and the proof of a landing is retired only
  there by the live run; the race it closes, why the contract is enough, its cost and
  tests, and what the rules documents need.
- [stage3-steps-adapters.md](stage3-steps-adapters.md) — steps S7–S11: the tool path on
  the fixture, Codex and Claude Code's hook machinery leaving, the adapter API, the host
  on the live set.
- [stage3-moves.md](stage3-moves.md) — the moves S12–S16: the inventory each rests on
  (declarations, string consumers, mutations, build values), the preparatory commits,
  the move of the turn-end code and of the delivery server.
- [stage3-packages.md](stage3-packages.md) — every package of `internal/` and
  `cmd/rewake` by file group, and every split file by declaration, with its 2.0 place or
  its removal and the step.
- [stage3-tests.md](stage3-tests.md) — how a rule names its tests, `docs/rules_test.go`
  with the build variants, and the effect rules E1–E8 and the turn outcomes with their
  tests and gaps.
- [stage3-tests-tcl.md](stage3-tests-tcl.md) — the tool rules T1–T11, the channel record
  C1–C8, the launch rules L1–L3 and the host, with their tests, the neutral rig and the
  gaps.
- [stage3-fixture.md](stage3-fixture.md) — the fixture adapter on the 1.x contract, its
  readiness exchange, what each step can prove on it, its program, its column, and the
  gate across the steps.
- [stage3-state.md](stage3-state.md) — the state root `v2/` and its inverse, what is a
  writer, the build id and the room's lease with its bootstrap writes, the concurrent
  tests, the cost of the hash.
- [stage3-upgrade.md](stage3-upgrade.md) — clearing the state of 1.x: what 1.x does,
  the supported upgrade, the exclusion protocol, its tests against a stand-in of 1.x.
- [stage3-decision-model.md](stage3-decision-model.md) — part B, the operator decision as
  an abstract model: historical landing, evidence, the sender's disposition, choice, run
  lifetimes and the mailbox's admission kept apart, the excluded combinations, the transitions, the expected outcome of each world, what a
  decision is answerable for, and correction 9 in its normative form.
- [stage3-decision.md](stage3-decision.md) — part B, the operator decision on an unknown
  outcome: its subject, its authority, the command, the write-once records and what every
  reader consults.
- [stage3-decision-recovery.md](stage3-decision-recovery.md) — part B, the protocol
  derived from the model: the decision's record and observations, installing its effects
  with proofs that outlive them, composition, the rules D1–D8 and their tests with the
  model as the oracle.
