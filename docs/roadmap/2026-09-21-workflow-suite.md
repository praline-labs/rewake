# The workflow suite — in progress since September 21, 2026

`test/workflow` runs a built rewake end to end: its own binary, a private HOME, state
directory and PATH, process groups it owns and kills, and a switch, `REWAKE_WORKFLOW=1`,
without which every scenario skips itself and the five checks stay cheap. With the
switch set and nothing run, the suite fails rather than reporting green on nothing.
The harness in front of it is a shim that plays the Codex app-server, re-executed
from the test binary, whose answers are checked against the saved 0.155.1 schema.

What exists, by scenario name: `stub`; `codex-conversation-accepted`, a Codex session
taken to an accepted conversation without delivery, with readiness controls that must
turn the case red; `task-report`, the first of the three selected scenarios — a task
delivered to an idle session and the report that answers it — with its negative
controls; `batch-arrival`, the second selected scenario, since September 22, 2026 —
two letters in one collection window announced as one group, a third outside it,
an overview that consumes nothing, reads one member at a time, no replay — with four
controls that each mutate the product through a build overlay rather than the
fixture, and a crosswise check of them under `REWAKE_WORKFLOW_CROSS=1`; `mid-turn`,
the third, since September 22, 2026 — a letter steered into a turn the recipient
holds open, with three controls of which one mutates the product; `second-terminal`,
which states that a turn ended twice by a misbehaving server yields one report, and
says in its own text that it has no reachable control;
and two self-checks, `shim-answers-match-schema` and `self-check-incomplete`.
The scenarios are described in [check-runner-scenarios.md](../check-runner-scenarios.md),
the shape in [check-runner-proposal.md](../check-runner-proposal.md), the evidence
contract in [check-runner.md](../check-runner.md). What building each of them cost,
and what it does not prove, is in its own record:
[task-report](2026-09-21-scenario-task-report.md),
[batch-arrival](2026-09-22-scenario-batch-arrival.md) and
[mid-turn](2026-09-22-scenario-mid-turn.md).

What does not exist: `ack-recovery`, the fourth scenario; a fixture for the Claude
Code column, so the scenario × harness matrix has one column running; and a runner
command — the proposal chose `go test` with a summarizer over `go test -json` and
rejected a separate binary,
and the summarizer is not written either. The paid tier with a real model has not run
under the suite.
