# Stage 3: rules and their tests — tools, channel, launch, host

The tool rules T1–T11, the channel record C1–C8, the launch rules L1–L3 and the host's
tests, in the form of [stage3-tests.md](stage3-tests.md) — which says how a rule names its
tests, what `docs/rules_test.go` checks, and what carry, rebuild and drop mean. Paths
are under `internal/` with the line of the `func`; **[unverified]** marks a class taken
from a name or a grep.

## The neutral rig

Most of what holds T2–T9 today runs in `bridge/server`'s rig: `newRig`
(`bridge/server/rig_test.go`) builds `cmd/rewake` with the tag `rewakefault`, starts a
host endpoint with `cli.ToolWords` and `cli.AcknowledgeRead` (`:109,:111`), and drives the
real MCP stdio server through `mcp_client_test.go`, which runs the `rewake` child. S7
builds `test/toolrig` beside it: the server replaced by the fixture's transport
([stage3-fixture.md](stage3-fixture.md#the-tool-transport)), everything else kept:

- the fixture transport plays the harness's side of a call over the host endpoint's
  socket, as the Claude Code mod will: the native call observed, the request, the
  confirmation, the child's exit, the result recorded — each step controllable (held,
  dropped, repeated, reordered, its text equal, altered, failed or past the limit);
- the end gate (`endpoint.NewGate`), the read clock and `AcknowledgeRead` (in
  `core/mail` from S14) unchanged;
- the fault seam of `cmd/rewake` under `rewakefault`, its roles renamed: "server"
  becomes the fixture transport, "child" stays the `rewake` child; the wrapper's fault
  plan (`wrapperPlan`) unchanged;
- `fault_test.go`'s method unchanged: one clean run with a log of every durable step,
  then one case per step with the transport or the child killed there.

The oracles are the rule texts, as today; only the driver changes. Each rebuilt oracle
is shown to fail under the mutation its old one failed under; the old rig runs beside
the new one until S8 deletes it.

## T1–T11: every tool transport

The endpoint and CLI tests build tickets with the constant `bridge.CodexTransport`
(`endpoint/helpers_test.go:25,69`, `cli/bridge_tool_test.go:73`, `cli/journal_test.go:74`)
and `cli/pending_turn.go`'s `inOwnTurn` keys on it. The contract that replaces it is one,
stated in [S7](stage3-steps-adapters.md#s7-the-tool-path-on-the-fixture-codex): the
transport may declare that its turn ids are never reused; declared, a later attempt in the
same turn marks; absent, only the first attempt marks. Those tests run on a neutral test
transport with the declaration made and withheld.

- **T1 — a transport decides no mail.** Carry: `bridge/endpoint/tickets_test.go:48`,
  `cli/bridge_tool_test.go:133,225`, `cli/bridge_rules_test.go:201`. Rebuild in S7:
  `bridge/server/flow_test.go:40,136,159`, `stdout_bound_test.go:169`. Gap: no test
  says the transport keeps nothing a later call needs — closed in S7 (a transport
  restarted between two calls of one turn changes no answer).
- **T2 — a trusted binding.** Carry: `endpoint/tickets_test.go:17,29,48,74,114,153,190`,
  `endpoint/hello_test.go:40,80`, `endpoint/order_table_test.go:56` (pure table),
  `bridge/bridge_test.go:26,50,72,96,157`, `cli/bridge_tool_test.go:194`. Codex later:
  `endpoint/bind_test.go:16,43,69,100,115`, `events_test.go:73,164`. Drop with the
  Claude hooks: `bind_test.go:87`, `events_test.go:102,218`,
  `tickets_end_test.go:40` (its oracle, no ticket after a capture, is rebuilt in S7),
  `server/flow_test.go:172`. Gap: a binding from the harness's own ids, checked per
  request on the exact harness process — closed in S7.
- **T3 — effects in the core operation, one operation per call.** Carry:
  `receipt/receipt_test.go:26,65,79,96,114,138`, `cli/journal_test.go:39,82,111,157`,
  `cli/bridge_rules_test.go:26,93,253`, `cli/bridge_read_test.go:118,146`. Rebuild in
  S7: `bridge/server/order_test.go:82` (two transports for one native call),
  `fault_test.go:91`.
- **T4 — bound to its operation before its first effect.** Carry:
  `cli/bridge_rules_test.go:93,201,218`, `cli/bridge_read_test.go:165`,
  `cli/journal_test.go:111`. Rebuild in S7: `bridge/server/child_bound_test.go:145`,
  `stdout_bound_test.go:169`, `fault_test.go:91`. Gap: the shell running the same
  words on the proof of absence — closed in S7.
- **T5 — bounded, in one place each.** Carry: `bridge/bridge_test.go:104,127`,
  `cli/bridge_tool_test.go:250`, `cli/bridge_rules_test.go:177,305,323,332`,
  `endpoint/hello_test.go:101`. Rebuild in S7 on the endpoint's encoder:
  `bridge/server/encoder_test.go:19,52,66`, `bound_test.go:32,101`,
  `stdout_bound_test.go:33,131`. Gap: a request bounded before it is parsed at the
  endpoint (only `server/mcp_test.go:106` checks it, for MCP) — closed in S7.
- **T6 — a read only on proof of the whole result.** Carry:
  `bridge/bridge_test.go:104`, `cli/bridge_read_test.go:52,204`,
  `cli/bridge_lookups_test.go:37,53,83,100,140`, `cli/bridge_unknown_test.go:21,37,73`,
  `cli/bridge_rules_test.go:117,285`. Split in S7: `endpoint/events_test.go:135` holds
  a neutral oracle — a ticket used once, under its binding — behind raw Codex events, so
  its driver becomes the neutral input; `:191` exercises the decoding of Codex result
  shapes (`exposureOf`), which leaves with Codex in S8, and a neutral oracle — the whole
  result and its size — rebuilt on the neutral input. Rebuild in S7:
  `bridge/server/flow_test.go:40,63,95,191`, `evidence_test.go:80,218`. Drop:
  `endpoint/limits_test.go:63,120` (the Claude hook's limits),
  `wrap/mailtool_launch_test.go:176`. Gap: a reading tool whose transport cannot give
  the proof refuses before its first effect — closed in S7 (the fixture offers a
  transport without result evidence).
- **T7 — commits stop at the end; a mark speaks for its own turn.** Carry:
  `endpoint/gate_test.go:18`, `endpoint/tickets_end_test.go:22,56,74,140` (renamed off
  Codex), `cli/pending_test.go:60,106,132,198` (each with the declaration present and
  absent, S7), `cli/bridge_rules_test.go:130`,
  `cli/journal_test.go:178`. Drop: `cli/pending_test.go:256` (the end heard once with
  no event). Rebuild in S7: `bridge/server/order_call_test.go:46`,
  `order_gen_test.go:87`, `order_marks_test.go:133`, `flow_test.go:119`. Gaps: no test
  names `markUnproven` (`cli/pending_turn.go:10-31,65`); none has a lost end with a
  `retry` from another turn, which T7 requires — both closed in S7: a pending call in
  turn A expires, A's end is lost, `rewake retry` runs in turn B, and the answer is
  "not marked", no mark is written, the receipt is finished as not made, the letter
  shows again.
- **T8 — a boundary is a cut between commits.** Carry: `endpoint/gate_test.go:42,82,
  97,115`, `cli/turn_scope_test.go:16,36,93`, `cli/turn_journal_test.go:71,102,139,214,
  238,293,311,327`, `cli/turn_journal_retry_test.go:77,114,142`,
  `cli/turn_retry_test.go:33,106`. Rebuild in S7: `bridge/server/order_test.go:17`,
  `order_cut_test.go:23,115`, `order_gen_test.go:87`, `order_marks_test.go:133`.
- **T9 — every wait bounded, nothing waits holding what it waits for.** Carry:
  `endpoint/tickets_test.go:153,190`, `endpoint/gate_test.go:115`,
  `endpoint/tickets_end_test.go:140`, `receipt/receipt_test.go:114,224`,
  `cli/bridge_rules_test.go:117,130`, `bridge/bridge_test.go:72`. Rebuild in S7:
  `bridge/server/child_bound_test.go:23`, `stdout_bound_test.go:33,131`,
  `mcp_test.go:186`. Codex later: `mcp_test.go:138` (the MCP server's cap on calls).
- **T10 — one build per room.** Carry: `endpoint/hello_test.go:40` (another build's
  hello refused, through the `SameBuild` seam). Gap: the build id and the room's lease —
  closed in S17 ([stage3-state.md](stage3-state.md#the-tests)).
- **T11 — a failing tool never weakens the mail.** Carry:
  `cli/bridge_read_test.go:165`, `cli/bridge_rules_test.go:218`,
  `cli/shell_channel_test.go:24,96`, `receipt/shell_test.go:19,92,109,134`,
  `cli/journal_test.go:111`, `bridge/bridge_test.go:96`. Rebuild in S7: the wrapper
  faults of `fault_test.go:91`. Gap: one set of words once through the tool and once
  through the shell under one receipt — closed in S7.

## C1–C8: the channel record

The 1.x record carries the harness as a field (`channel/record.go:55-56`, `New(harness,
injected, ...)`), its fold branches on it (`events.go:112,131`, `history.go:202,287-300`),
and its spaces walk a harness axis. In 2.0 the record is any transport's: S8 removes the
harness field and the Codex and Claude MCP letters. What stays of the alphabet is what a
neutral transport and the shell produce: `hello-*`, `close-*`, `refused-*` (the peer
check), `bound` (the 1.x `validated*`: a call met its binding), `timer*` (no hello
within its bound after the run's start), `denied*`, `shell-*`, `exit`, `land`. Gone:
`session-started`, `call-seen`, `not-observed`, `cannot-start` (Claude hook and MCP
start), `thread`, `startup-failed*` (Codex). The starts become "tool offered" and "no
tool", two instead of four.

- **C1** — rebuild in S8: `channel/space_test.go:204`
  `TestEveryEventSequenceKeepsTheRules` (depth 4, every start, on the new alphabet),
  `table_test.go:217` (its 9 Claude rows recast on the new kinds; its 16 Codex rows go
  with `table_codex_test.go`, Codex later); carry: `space_oracle_test.go:117`,
  `wrap/channel_test.go:148`.
- **C2** — rebuild in S8: `connections_test.go:28,54,92`, `wrap/channel_test.go:68`;
  Codex later: `connections_test.go:73`.
- **C3** — rebuild with C1 (the "not observed" rows and the label check). Gap: no test
  of its own that a wait names what was not observed and never claims a failure —
  closed in S8 with the rebuilt table.
- **C4** — rebuild with `space_test.go:204` (`refused-stray`, `denied`); carry
  `wrap/channel_test.go:148`.
- **C5** — rebuild in S8: `order_test.go:15`, `late_test.go:14,36`,
  `wrap/channel_order_test.go:22,45,63`; Codex later: `late_test.go:80`.
- **C6** — carry: `preview_test.go:15`, `notices_test.go:18,47,73,92` (with a neutral
  constructor), `windows_test.go:13`, `harness/notice_test.go:11,24,38,59` and
  `notice_recall_test.go:25,38,51,70,85,98,118` (to `inbox` in S10, `core/mail` in S13),
  `wrap/channel_test.go:87,98`, `wrap/channel_order_test.go:87`.
- **C7** — rebuild with C1 (`denied`, `denied-held`); carry `wrap/channel_test.go:116`,
  `wrap/channel_order_test.go:186`. The adapter must emit the denial event; on the
  fixture it is a switch.
- **C8** — carry: `wrap/channel_test.go:135,164`, `wrap/channel_order_test.go:113,134,
  171`; rebuild with C1 (the frozen record). Gap: "nothing granted or approved by the
  channel" has no test of its own — closed in S8 (no event of any letter writes a grant
  record or a permission decision).
- Drop: `table_codex_test.go:282` (reconnect through `/mcp`, `notices.go:33-35,115`).
  Codex later: `selection_space_test.go:71,85,97` with `selection_oracle_test.go`, the
  best seed for stage 5, kept in history.

## L1–L3: what a launch adds

The tests of these rules live in the Claude Code adapter and hold its hook and plugin
launch: `harness/claude/plugin_test.go:20,66,231`, `settings_test.go:103,185,222,238,
275,285`, `hook_test.go:15`, `plugin_window_test.go:21`, `permission_test.go:195`,
`telemetry/collector_test.go:61,184`, `lane_test.go:229,325`. They go with that
machinery in S9 and return on the mod in stage 4. What stays in stage 3:

- **L1 — the mail is added and nothing else changes.** Carry:
  `harness/claude/claude_test.go:44` (added flags before the terminator),
  `harness/defaults_test.go:35`. Gap: no harness-free test that a launch leaves the
  person's configuration as it was — closed in S5 on the fixture: the digest of the
  person's home and working directory before and after a launch, the pattern of
  `harness/claude/mailtool_inject_test.go:90` (`digestTree`), which goes in S8.
- **L2 — diagnostics say where, never what.** No harness-free test exists. Gap —
  closed in S5: a fixture launch whose configuration and arguments hold a marker, every
  refusal and note of the launch checked not to contain it.
- **L3 — nothing persists past the run.** Carry: `wrap/control_test.go:33,49`,
  `wrap/wrap_test.go:120`, `wrap/ownership_test.go:24,303`,
  `harness/claude/claude_test.go:101,118` (the messaging socket, kept with Wake). Gap:
  the run directory `run/<name>.<epoch>/` removed with the run — closed in S17.

## The host

The host's `Tests:` block in `docs/rules/host.md` names what the wrapper's tests hold,
all carried from `wrap` to `host` in S16: `ownership_test.go`, `stop_follow_test.go`,
`signals_test.go`, `wrap_test.go`, `control_test.go`, `availability*_test.go`,
`compaction_letters*_test.go`, `departure_evidence_test.go`, `session_notices_test.go`,
`hold_notices_test.go`, `naming_test.go`, `role_selection_test.go`,
`room_context_test.go`, `window_test.go`. Drop in S2: `launch_evidence_test.go`,
`launch_order_test.go:22` (the cutover's proof); `launch_order_test.go:68` (main told of
earlier runs at its start) is read again in S2 and kept if it holds anything beyond the
cutover. Drop in S8: `mailtool_launch_test.go`, `mailtool_version_test.go` (Codex
later). `thread_test.go` tests the wrapper's use of `ThreadTracker` and `ThreadSource`
through its own fakes (`threadedHarness`, `threadObserver`): it carries, recast on the
API's interfaces in S10, whether or not a real adapter implements them in stage 3.

The grant scheme's three steps carry with `grantauth` and `wrap/grants*`
(`grantauth_test.go:54,76,84,102,119,131,151,163,181,215`, `checks_test.go:35,57,76,156`,
`helper_test.go:85,108`, `resume_test.go:77,188,220,238`, `wrap/grants_test.go:51,153`,
`wrap/grant_checks_test.go:25,43`, `wrap/grant_resume_test.go:119`). The keeper's tests
(`keeper_test.go`, `checks_test.go:47`, `helper_test.go:130`, `wrap/grants_test.go:102`,
`wrap/grant_checks_test.go:58`) go with it in S9. Gap: step 3's "the task fails, naming
the main that sent it" — closed in S9.

## Gaps and their steps

| Rule | Gap | Closed in |
|---|---|---|
| T1 | the transport keeps nothing a later call needs | S7 |
| T2 | a binding from the harness's ids, checked per request on the exact process | S7 |
| T4 | the shell runs the same words on the proof of absence | S7 |
| T5 | the endpoint bounds a request before parsing it | S7 |
| T6 | a reading tool without result proof refuses before its first effect | S7 |
| T7 | `markUnproven`; a lost end with a `retry` from another turn | S7 |
| T10 | the build id and the room's lease | S17 |
| T11 | one set of words through tool and shell under one receipt | S7 |
| C3 | a wait names what was not observed, never a failure | S8 |
| C8 | nothing granted or approved by the channel | S8 |
| L1 | a launch leaves the person's configuration unchanged, harness-free | S5 |
| L2 | no configuration content or argument value in a diagnostic, harness-free | S5 |
| L3 | the run directory goes with the run | S17 |
| grants | step 3: the task fails naming the main that sent it | S9 |
