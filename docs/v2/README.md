# rewake 2.0

The documents of the 2.0 rebuild on the `v2` branch. The plan from here follows the owner's
decision of October 18, 2026 — pi after Claude Code, Codex not soon, a space as main starting
its workers without showing them inside Claude Code ([the stage plan](design.md#the-stage-plan-from-here),
[its entry](../roadmap/2026-10-18-decision.md)). The closed stages — the revision of 1.x, stage
3's plans and evidence, stage 4's audit — are in [the archive](../archive/README.md).

The design:

- [design.md](design.md) — the overview: what does not change, the layout of 2.0 with its
  import rule and the test that holds it, the stage plan with each stage's acceptance, and the
  owner's answers. Open it first.
- [design-api.md](design-api.md) — the adapter API: the capabilities, offered and live, what
  the core does without each, the catalogue as a value, minimum versions and the legacy mark.
- [design-rules.md](design-rules.md) — the core rules restated for 2.0: effects E1–E8, turn
  outcomes, what a launch adds L1–L3; the tool transport's T1–T11, the channel record's
  C1–C8 and grants left with their code on October 18, 2026.
- [design-claude.md](design-claude.md) — the Claude Code adapter as one mod: loading, what
  happens when it does not load, the turn boundary, telemetry, control, wake, and the stage 4
  probe P1–P16 with what it answered.
- [design-state.md](design-state.md) — the state root of 2.0, clearing the state of 1.x, one
  build per room.

Stage 3's document that the living ones cite:

- [stage3-state.md](stage3-state.md) — the state root `v2/` and its inverse, what is a writer,
  the build id and the room's lease with its bootstrap writes.

Stage 4, Claude Code's adapter as one mod, then the space:

- [stage4-turns.md](stage4-turns.md) — S4-6, built and accepted October 16, 2026: a Claude Code
  worker's turns reported through the mod, the end retried under its id and taken once, the
  heartbeat, the known causes that refuse a reporting role. Open it before changing how a turn
  of a Claude Code session reaches the core.
- [stage4-turns-build.md](stage4-turns-build.md) — S4-6's second half: its clauses in
  `docs/rules/`, its build in three parts and the live acceptance.
- [stage4-space.md](stage4-space.md) — the space's design of October 13, 2026; its keeper,
  screen and keys superseded on October 18, 2026. Open it for the hosted launch and the record.
- [stage4-space-steps.md](stage4-space-steps.md) — its second half: tests, the rules SP1–SP9,
  the build steps S4-1 to S4-6 and the one space step S4-7, the open questions.
- [stage4-space-record.md](stage4-space-record.md) — the space record and `rewake space list`
  as S4-3 built them. Open it before writing a space's record from the host.
- [stage4-space-terminal.md](stage4-space-terminal.md) — `internal/host/terminal` as S4-4
  built it; its emulator left the code on October 18, 2026 and came back the same day for the
  workers' panel, which the host mode replaced that evening; the shell replaced the host mode on
  October 19, 2026.
- [stage4-reading-pi.md](stage4-reading-pi.md) — pi read on October 13, 2026 at 0.85.1 and
  1.2.0, capability by capability, its risks and the probe items PI1–PI14. Open it before
  stage 4b.

Not scheduled:

- [ideas.md](ideas.md) — the ideas the owner recorded on October 8, 2026 and the recon of
  Claude Code mods that bears on them.
