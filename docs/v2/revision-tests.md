# Revision: tests and tools

What the tests prove is the part of 1.x most worth carrying: several were built as
generated spaces with an oracle taken from the rules rather than from the code. This
file names them, then the workflow suite, the tools and the scripts. The overview is in
[revision.md](revision.md).

## Generated spaces, fault tests and tables

| Test | What generates the cases | Oracle | Verdict |
|---|---|---|---|
| `inbox/plan_faults_test.go` (`TestEveryReadOfThePlanStopsBeforeTheFirstEffect`) with `seam_probe_test.go` | scenes × every read of the plan, taken from the seam (`fileAccess`), not a list × fault kind (path unreachable, closed, a directory, unparseable) × pass | the gate answers stopped, only the stop record changes; the barrier after a fault ends where the scene without one does | carry over, the framework whole |
| `inbox/plan_pairs_test.go`, `plan_writes_test.go` | pairs of failing reads; every write of the barrier, from the seam, in three runs | a stop of an effect outlives its cause; after one failed write the next attempt arrives, nothing lost or done twice | carry over |
| `inbox/plan_scenes_test.go` | 11 scenes | as above | 4 carry over; 7 are about earlier receipts or held reports and go or are rebuilt on journal records |
| `inbox/late_unknown_test.go` | stops, cancelled and failed retries; evidence paths | a stop stands until the barrier passes | change: the `conversionLab` fixture is rebuilt |
| `inbox/inbox_test.go` (`stateDir`), `records_test.go` | every inbox and CLI test builds a state directory | a file of a kind not on the record list fails the test | carry over |
| `channel/space_test.go` with `space_oracle_test.go` | every event sequence up to length N over an alphabet per event kind, on both harnesses | each step checked against `mail-bridge-channel.md`, not against the fold | change: the Claude letters of the alphabet go |
| `channel/selection_space_test.go` with `selection_oracle_test.go` | Codex selections, bindings and connections | an oracle built differently on purpose: reads ahead how each selection ended, folds once by event time | carry over; follows the selection to the adapter or to neutral events, still checking event order |
| `channel/order_test.go`, `late_test.go` | every arrival order | rule 5: any order folds as by event time | change |
| `bridge/server/fault_test.go`, `fault_variants_test.go`, `fault_scenarios_test.go`, `wrapper_fault_test.go` | the fault test derived from the log: a clean call logs the operations of server, child and wrapper; each durable step becomes a case (child cut, write failed, server cut) through the fault seam (`state/fault_build.go`, build tag `rewakefault`) | no letter lost or doubled, no false "read" | carry over, `tool/mcp` |
| `bridge/server/order_gen_test.go`, `order_call_test.go` | every order of the events of two turns, in a fresh rig | one ticket per call; an end's boundary includes the confirmation | carry over |
| `bridge/endpoint/order_table_test.go` | any order of heard, asked, confirmed, aged, retried | at most one ticket per call | carry over, `tool` |
| `cli/bridge_rules_test.go` and the turn-journal tests (`turn_journal`, `turn_journal_retry`, `turn_retry`, `turn_scope`) | written scenarios per rule 1–6 against a defect once found; repeats of one event, lost journal writes | the rules of `mail-bridge-cli.md` | carry over |
| `wrap/ownership_test.go`, `stop_follow_test.go`, `signals_test.go` | process group, signals, stop following | the host cleans only its own socket and stops with the harness | carry over, `host` |
| `wrap/channel_order_test.go` | channel events in any order | event time, not fold time | Codex-only |
| `harness/claude/plugin_control_host_test.go` | plays steps to the module in `node` under an imitation of the host | the module loads and answers | rewrite as mod: the seed of the mod's host test, with `claude plugin validate` |
| tables: `harness/settings_test`, `defaults_test`, `notice_test`, `claude/permission_test`, `telemetry/state_test` | literal cases | settings precedence; journal to decision; compaction state is monotonic and a late event does not overwrite a newer one | carry over (`permission_test` with stage 6) |

**The mod gets the same fault cases.** The oracles of `bridge/server/fault_test.go:91`,
`order_cut_test.go:23` and `order_gen_test.go:87` are about the guarantee, not MCP:
they carry over to the mod's tools as cases — the result lost, replaced or truncated,
the mod failing after its child, a turn's end crossing the confirmation — and each must
end with no letter read that the model did not get whole. The MCP transport cases stay
in `tool/mcp`; the shared oracles check both transports.

The packages `cli`, `grant`, `grantauth`, `registry`, `worktree`, `proc` and the rest
of the small ones have written scenarios and tables, no generated spaces; their oracles
are literal values or real git.

## The workflow suite

`test/workflow`: 111 files, 20 920 lines. `column_test.go` defines a Codex column and a
Claude Code column and `runInColumns` runs a scenario in both.

| Group | Files | Lines | Verdict |
|---|---:|---:|---|
| neutral machinery: suite, case, column, classification, isolation, mutant, outcome, pool, process, termination, regression, shim calls, publication | 27 | 4 655 | carry over; `timings_test.go:72-79` loses the `internal/cutover` ldflag and its reference to `protocol-cutover.md` |
| neutral scenarios in both columns: awaited view, owed re-read, sent addendum, edit, recall, withdraw, pending report, stopped routing, wrapped launch, task report | 13 | 2 531 | carry over; the Claude column gets the new fixture |
| the Claude fixture `claudeshim_*`: hooks and settings, inbound gate, notice, telemetry, clear, control, records, resume, the plugin host | 12 | 2 617 | rewrite as mod: the fixture becomes a mod host (`$.tool.register`, `turn.start`/`turn.complete`, `session.usage`, abort, wake); `claudeshim_plugin_test.go` (385) is its seed |
| Claude scenarios `claude_*` and `pending_confirm` | 14 | 2 481 | rewrite as mod; `pending_confirm` keeps its negative controls (177, 181) on `classic.Stop` |
| the Codex fixture: `codexshim_*`, `codexsession`, websocket, schema, harness version, shapes | 24 | 4 994 | Codex-only; the three `legacy(codex <0.157.1)` marks (`codexshim_params_test.go:76`, `codexshim_client_test.go:139`, `codexshim_served_test.go:227`) go |
| Codex scenarios `codex_*` | 20 | 3 538 | Codex-only |
| `record/` | 1 | 104 | carry over |

The suite has no scenario for the mail tool on either harness: the tool was covered by
unit tests and live checks with `tools/standin`. 2.0 needs suite scenarios for the mod's
tools and the Codex MCP server, the same set of tools in both columns. The crosswise
check and the controls (`mutant_test.go`, `docs/controls_test.go`) carry over; the
Claude controls are rewritten with their scenarios.

The rule of `AGENTS.md` that a change to the plugin module runs the workflow suite
applies to the mod unchanged: the five checks do not start a module under its host.

## Tools and scripts

| Path | Code / tests | Verdict | Notes |
|---|---|---|---|
| `tools/release` | 1 385 / 801 | carry over | takes the version from `internal/cli/registry.go` (`gate.go:178`): follows the CLI's path |
| `tools/checksummary` | 695 / 552 | carry over | |
| `tools/harnesscache` with `cache`, `container` | 1 299 / 494 | carry over | the cached versions follow the minimums: Claude Code from 2.1.287, Codex from 0.157.1 |
| `tools/standin` | 407 / 258 | rewrite as mod | a stand-in model API that answers with a call of the mail tool through the Claude MCP path and its hooks (`main.go:1-4`); it calls a mod tool `mcp__rewake__<name>` instead |
| `scripts/pack.sh`, `scripts/shim.sh` | 150 / 125 | carry over | npm packaging and its POSIX entry |
| `dist/npm` | — | carry over | output of `pack.sh`, ignored by git; its README changes for 2.0 |
| `ideas/self-contained-task-packages.md` | 147 | carry over or archive | outside `docs/`, on no map |

## Proposals

Proposals for the owner, not decided:

- Rename the eleven test files named after a review round (632 lines) by subject:
  `gateway/review_*_test.go` (six), `cli/review_*_test.go` (three),
  `inbox/review_*_test.go` (two).
- Give the Codex MCP server and the mod one shared suite scenario per tool, so "the
  same tools on every harness" (decision 9) is checked rather than assumed.
