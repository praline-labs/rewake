# Design: Codex

Nothing of Codex is designed or ported in this stage. The owner decided on October 5,
2026 (answer 7 to the revision's proposals) that once the skeleton of 2.0 exists — the
core, the adapter API, a place for every part — the Codex adapter is designed afresh,
and how best to build it is thought through then. This file fixes only what the
skeleton must leave room for. The overview is in [design.md](design.md).

## What 2.0 must serve

The owner clarified decision 2 on October 5, 2026: in 2.0 a **Claude Code main works
both ways with Claude Code workers and with Codex workers** — tasks out, reports back —
in the same room. A Codex main is not a goal now. So the release (stage 7) requires the
pair live, and the core may not assume that main and workers run on one harness: a task
from a Claude Code main to a Codex worker, and its report back, go through the same mail
and the same rules (E1–E8, [design-rules.md](design-rules.md)) as between two Claude
Code sessions.

## The reserved place

`internal/adapter/codex`, empty in the `v2` tree until stage 5. The catalogue lists
only adapters that exist, so the builds of stages 3 and 4 launch Claude Code only
(the owner's answer 5, [design.md](design.md#the-owners-answers)). Nothing in `core`,
`tool`, `host` or `cli` may anticipate Codex by name (`internal/layout_test.go`); what
1.x kept for Codex in shared places leaves them in stage 3: `registry.Session.CodexHome`,
the Codex branches of the channel fold (`channel/selection.go:57`,
`channel/history.go:202,287`), the Codex sentences of the role texts.

## The capabilities it is expected to fill

From what 1.x does on Codex, as a starting point for stage 5's design, not a decision:

| Capability | In 1.x | For the pair |
|---|---|---|
| Launch | the private app-server and the terminal gateway (`harness/codex`, `codex/gateway`) | needed |
| Wake | a turn started or steered in the session's thread | needed: a worker must hear a task |
| TurnBoundary | the thread's turn events through the gateway | needed: a worker reports by ending its turn |
| ToolTransport | an MCP server injected for the run (`bridge/server`), never live (gate G2 open) | needed only where the sandbox closes the state directory; then with the same tools and schemas (decision 9) and G2 by reading plugin manifests and `.mcp.json` (decision 4, `.scratch/v2-recon/codex-plugins-g2.md`) |
| Telemetry | token usage from the thread | wanted |
| Control | compact and interrupt through the app-server (`server_steer.go`, `gateway/steer.go`) | wanted: main compacts its workers |
| Permissions | `--grant-git`, `--grant-dir` through the thread's sandbox roots | wanted: a write worker commits only by grant |
| PersonUI | — | later |

So the least a Codex adapter must offer for the pair is Launch, Wake and TurnBoundary,
with mail through the shell. The capability API ([design-api.md](design-api.md)) is
shaped so that this minimum fits without a change to the core.

## Meanwhile

Until stage 5 lands, the owner works in a **separate 1.x environment**: main and its
workers, Claude Code and Codex alike, run one chosen 1.x build — one, not a mix of the
1.0.3 release and an unreleased `main`. 2.0 is not part of that environment. Its
experiments run with their **own `REWAKE_DIR`**; on the shared root the first 2.0
session would refuse while 1.x wrappers live ([design-state.md](design-state.md#clearing-the-state-of-1x)), and
by design nothing passes between the generations: no bridge, no compatibility through
the shell. A 1.x worker cannot serve a 2.0 main — the two keep different state roots,
and a room holds one build (T10); reaching the state directory from a shell is not
enough, since a worker also needs registration, Wake and TurnBoundary of the same 2.0
build.

The sequence: stage 3 gives the core on the fixture, stage 4 the Claude Code pair,
stage 5 adds Codex workers to the same 2.0 room. **The mixed pair — a Claude Code main
with Claude Code and Codex workers — is required before the release**: until stage 5 is
accepted it is not met, and stage 7 cannot release.

The 1.x Codex code stays on `main` and in history as the reference stage 5 reads,
with its tests: the fixture (`codexshim_*`), the generated selection space and its
oracle (`channel/selection_space_test.go`, `selection_oracle_test.go`), the gateway's
tests. Which of them return, and in what form, is stage 5's design.

## Open for stage 5

- The minimum Codex version (none is declared today; `codex/server_version.go:31` pins
  only a note).
- One WebSocket client or two (answer 7: not decided).
- Whether the tools go through an MCP server and the host's endpoint as in 1.x, held to
  T1–T11 ([design-rules.md](design-rules.md#tools-t1t11)).
- How a Codex session's conversation and turn ids reach the binding (T2).
