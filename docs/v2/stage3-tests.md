# Stage 3: rules and their tests — effects and turn outcomes

How a rule in `docs/rules/` names the tests that hold it, what `docs/rules_test.go`
checks, and for the effect rules E1–E8 and the turn outcomes: the tests that carry, the
ones that need a rebuilt lab or oracle, and the clauses no test holds. The tool,
channel, launch and host rules are in [stage3-tests-tcl.md](stage3-tests-tcl.md). The
rules' wording is [design-rules.md](design-rules.md); the mapping rests on the revision
of the tests ([revision-tests.md](revision-tests.md)) and a recon of the test files at
`b6ed4ed`, by grep and reading; a class taken from a test's name and fixtures rather
than its body is marked **[unverified]**.

Paths are under `internal/` and give the line of the `func`. **Carry**: moves as is,
only its import paths change. **Rebuild**: the case stays, the lab or rig under it is
replaced, in the step named. **Drop**: it holds only the migration, `settle` or a
removed harness mechanism.

## How a rule names its tests

A rule in `docs/rules/*.md` is a list item opening with its number in bold — `- **E4.`
— and runs to the next rule or heading. It ends with a `Tests:` line and one item per
test, the file and the function in backticks:

```markdown
- **E4. A read's completion is one durable fact every channel shares.** ...
  Tests:
  - `internal/inbox/claims_test.go` `TestTheSweepKeepsALetterBeingRead`
  - `internal/cli/bridge_rules_test.go` `TestALateAcknowledgmentOwesNothingTwice`
  - Gap: one fact across every channel, not only tool and shell — closed in S7.
```

A move rewrites the paths in the same commit (the mechanical steps do it with the
import paths). Rules without a number — the host's — use the same `Tests:` block, and
the numbering adds two letters to the four of the design: **O** for the turn outcomes
(O1 interim wake, O2 failed and interrupted turns), so they are checked like the rest,
and **D** for the operator decision from S18, part B
([stage3-decision-recovery.md](stage3-decision-recovery.md#its-rules)).

## docs/rules_test.go

Runs with the five checks, parses Go files and Markdown, runs nothing:

1. Every number of E1–E8, T1–T11, C1–C8, L1–L3, O1–O2 (and D1–D8 from S18, part B) appears
   exactly once across `docs/rules/`, so a rule cannot be lost in a move.
2. Every rule has at least one test or one gap line.
3. Every named test exists: the file is in the module and declares `func <Name>(` as a
   `Test`, `Fuzz` or `Example` function; a name in a `rewakefault` or other tagged file
   is found by parsing the file, not by building it.
4. Every named test is run by one of the checks: a test in a file under a build tag is
   named only if a check of [AGENTS.md](../../AGENTS.md#checks) builds with that tag
   (from S1 the five checks run `go vet`, `staticcheck` and `go test` also with
   `-tags rewakefault`, from S5 with `rewakefixture` and both), so a rule cannot rest on
   a test nothing runs.
5. Every gap names the step that closes it (`closed in S<n>`), and the step is one of
   [stage3.md](stage3.md#the-build-order).
6. From S19, any gap line fails, which is how the stage acceptance "every carried rule
   names its tests and they exist" is checked rather than read.

Its own negative cases: a rule with no test, a test that does not exist, a test under a
tag no check builds, a number twice, a gap with no step — each on a synthetic document.

## E1–E8

**E1 — an unknown effect is never discarded or bypassed.** Carry:
`cli/bridge_rules_test.go:26` `TestAnUncertainPublicationIsNotDiscarded`, `:58`
`TestAHeadsUpToAnEndedRunWithNothingLeftStaysUnknown`, `:93`
`TestAnOpenOperationIsNotBypassedByAge`; `inbox/journal_unknown_test.go:23,62`;
`inbox/claims_test.go:126` `TestAnIntentWithoutItsLetterIsWrittenAgain`, `:197`, `:283`;
`inbox/journal_test.go:54,154`; `cli/journal_test.go:82,111`;
`receipt/receipt_test.go:138,173`. Drop: `inbox/conversion_test.go:148,221` (earlier
receipts); their case — an unknown report stops, nothing published, main told once — is
held again by S2's rebuilt scenes and S3's new test.

**E2 — a lock is removed only by its holder.** Carry: `receipt/receipt_test.go:114,224`
`TestTheSweepRespectsAHeldLock`; `state/held_test.go:9,30,52`; `state/lock_test.go:49`;
`state/state_test.go:213`. Gap: the inbox sweep (`inbox/sweep.go`) removing a record
only under its lock has no test of its own — closed in S3.

**E3 — every check runs in the critical section, again on a retry.** Carry:
`cli/bridge_rules_test.go:117,130,177,201,218`; `cli/addendum_test.go:31,43`;
`inbox/claims_test.go:226,248`; `inbox/batch_edges_test.go:11`;
`inbox/withdraw_test.go:294`; `inbox/window_test.go:264`. Gaps: a turn end's retry
with its own deadline (only the tool's and the shell's retries are tested) — closed in
S5, on the fixture's turns; a once-publication checks its recipient's run outside the
recipient's critical section (`journal_held.go:145-161`, `cli/journal_steps.go:41`) —
closed in S3, commit 1 ([stage3-publication.md](stage3-publication.md#tests)).

**E4 — a read's completion is one durable fact.** `cli/read_ack.go` has no test file;
`AcknowledgeRead` is held through: carry `cli/bridge_rules_test.go:253,285`;
`inbox/claims_test.go:34,67,88,171`; `cli/bridge_read_test.go:52,118,165,258`;
`cli/bridge_lookups_test.go:53,83,100`; `inbox/unread_test.go:85,205,234`;
`cli/read_test.go:70,215`; `cli/stop_gate_test.go:53`. Gap: one fact across every
channel — today tool and shell only — closed in S7 with the fixture's tool.

**E5 — the size bound on the final encoded bytes, in one place.** Held today in the
server's encoder (`bridge/server/encoder.go:12,110`) with `bridge/parts.go:12-25`. Carry:
`bridge/bridge_test.go:104,127`; `cli/bridge_rules_test.go:305,323,332`;
`cli/bridge_tool_test.go:250`. Rebuild in S7 (the encoder goes with `bridge/server` in S8):
`bridge/server/encoder_test.go:19,52,66` and `bound_test.go:32,101`
(`FuzzEveryMessageWrittenFits`, `FuzzFramesAreBounded`) become the bound of the host
endpoint's one encoder, run against the fixture's transport.

**E6 — three outcomes, only "no such file" proves absence.** Carry:
`inbox/lookups_test.go:24,47,69,86,104,123,147,165,196,220,266`;
`inbox/unknown_test.go:34,45,62`; `inbox/records_test.go:41,66,112`;
`inbox/journal_unknown_test.go:148`; `cli/bridge_unknown_test.go:21,37,73,98`;
`cli/bridge_lookups_test.go:37,140,159,199`; `inbox/confirm_test.go:74`. Gap: readers
that only inform (`awaited`, `owed`, `list`) folding an unknown into their most
cautious answer — closed in S3.

**E7 — immutable identity, proven scope, recovery from durable evidence.** Carry:
identity `cli/turn_journal_retry_test.go:114,142`, `cli/turn_test.go:143`,
`cli/review_receipt_identity_test.go:109` (renamed by subject),
`cli/turn_retry_test.go:33,106`; scope `cli/turn_scope_test.go:16,36,93`,
`cli/turn_journal_test.go:293,311`, `inbox/journal_test.go:86`; the read clock
`inbox/read_boundary_test.go:16,60,86,119,132`, `inbox/confirm_test.go:15,47`; the
pending window `inbox/pending_test.go:34,49,77,95,117,136,218`,
`cli/pending_test.go:60,106,164,183,198` (`:106` read; the others **[unverified]**),
`cli/turn_journal_test.go:327`, `cli/turn_journal_retry_test.go:77`; the journal first
`cli/turn_journal_test.go:71,102,139,214,238`, `inbox/journal_test.go:204`. Rebuild in
S5 on neutral completions, beside the paths they run on until those leave:
`cli/review_receipt_identity_test.go:50,82` (their neutral oracle — distinct ids report
twice, a stopped then a finished end of one turn report twice and the finished one
settles; their gateway legs go in S8, the gateway's advisory gaps Codex later);
`cli/turn_test.go:109` `TestTwoTurnEndsAtOnceReportOnce` (hook run, until S9);
`cli/error_report_test.go:77` (an end with no boundary, through the Codex notify, until
S8). All of these stay in `cli`: their helpers are `cli`'s
([stage3-moves.md](stage3-moves.md#s14-the-turn-end-and-read-code-to-coremail)).
Drop: `cli/pending_test.go:256` (the end heard once with no event), `inbox/reconcile_stop_test.go:167`, `cli/turn_legacy_receipt_test.go`.
Gaps: a hold reserving its read-clock position, never issued twice, never covered by a
boundary before it commits (`inbox/kept.go:70-80`; only reads are tested at `:86`) —
closed in S3; every TurnBoundary names its event — closed in S10 with the adapter API.

**E8 — done, not done, or unknown; an unknown stops the mailbox.** Carry: the evidence
`inbox/journal_unknown_test.go:23,62`, `inbox/claims_test.go:88,126`,
`inbox/journal_test.go:54,154`; the stop record `inbox/stop_writes_test.go:17,55`,
`inbox/answer_stop_test.go:16`; the plan before any effect
`inbox/plan_faults_test.go:35` with `plan_pairs`, `plan_writes` and `seam_probe` (no
functions of their own; they run inside it); retention `inbox/journal_test.go:171`,
`cli/turn_journal_retry_test.go:114`, `receipt/receipt_test.go:173`,
`inbox/sweep_test.go:120,164`; gap: a publication's proof retired while an attempt
admitted to its run can still replay it, or by a sweeper that is not the live run
(`sweep.go:74-89`, `once.go:206-216`) — closed in S3, commit 1
([stage3-publication.md](stage3-publication.md#tests)). Rebuild in S2 on a two-session lab with journal records
only: `inbox/evidence_test.go:27`, `effect_stop_test.go:22,80`,
`late_unknown_test.go:23,115` (of its six evidence paths, "recipient" and "owed note"
stay), `conversion_moot_test.go:13,38`, `conversion_test.go:293` (renamed by subject),
`stop_writes_test.go:17,55` (the same lab) and `plan_scenes_test.go` (below), their
assertions unchanged. Carry through S2: `cli/stop_gate_test.go:17,53,79` and
`inbox/answer_stop_test.go:16`, on ordinary current journals. **Inverted in S3**
(correction 2 of [stage3.md](stage3.md#corrections-to-the-accepted-design)), every test
whose removal of a cause lifts a stop — `inbox/reconcile_stop_test.go:74,105`,
`answer_stop_test.go:16`, `stop_writes_test.go:17`, `cli/stop_gate_test.go:17` — and
`stop_writes_test.go:55` rewritten for a resolution, each with its repair half on valid
bytes behind mode 0000; the search and the table are in
[S3](stage3-steps.md#s3-a-stop-is-resolved-per-cause-by-evidence-codex).
Drop: `inbox/evidence_test.go:66`, `conversion_test.go:180,221,262,344`,
`reconcile_stop_test.go:18,44,144`, `held_successor_test.go`, `cli/upgraded_test.go`,
`cli/settle_caller_test.go`.

The eleven plan scenes (`inbox/plan_scenes_test.go:23-127`): scenes 1 (two journals to
two recipients), 9 (the steps of a journal) and 10 (a shared wait, kept and interim)
rest on journal records only and carry. Scene 5 (a note owed) reads the journal's
`Moot` and `Notices`; `Moot` stays in 2.0's journal and `Notices` is one of the
migration's fields, so scene 5 is rebuilt on `Moot` alone in S2. Scene 11 (a record of
every kind) is rebuilt without its earlier receipt. Scenes 2, 3, 4 (held for a
successor) and 6, 7, 8 (an earlier receipt, a conversion) go.

## The turn outcomes

**O1 — an interim wake must not close a task early.** Carry: `inbox/confirm_test.go:120,
164,201`; `inbox/pending_test.go:174,187`; `inbox/journal_unknown_test.go:190`;
`cli/inbox_awaited_test.go:305`; `test/workflow/awaited_view_test.go:21`;
`test/workflow/pending_report_test.go:19` with its controls `:185,189,193`;
`cli/turn_hold_test.go:149` (through `completeTurn`). Rebuild in S5 on the neutral
confirmation, beside the Stop-hook cases that go in S9:
`cli/turn_hold_test.go:80,122,174,191,209` and `test/workflow/pending_confirm_test.go:25`
with its negative controls `:177,181`, run on the fixture column. Gap: every failure of
the check falls to publishing — `:209` tests ordinary reasons not to hold and a notify
case, not a failure of the check's own reads — closed in S5, with a fault on each read
and write of the check. Gap: a held end confirmed again is not taken for its continuation —
closed in S5's first commit, with the held end's identity and its repeated, concurrent and
crashed cases — the crash before the clock's commit, and the retry after the
continuation's journal is done, renamed and swept among them ([S5](stage3-steps.md#s5-the-confirmation-and-the-fixture-beside-the-others-codex)).

**O2 — failed and interrupted turns.** Carry: `cli/stopped_test.go:15,58,95`;
`cli/pending_test.go:106,198`; `cli/turn_journal_retry_test.go:77`;
`cli/turn_retry_test.go:106`; `cli/error_report_test.go:46` (its `completeTurn` leg);
`test/workflow/stopped_routing_test.go:27,211`. Rebuild on the fixture's turns before
the path each passes leaves, all in S5: of `error_report_test.go`, `:77` and `:126` pass
a Codex notify payload (the path leaves in S8), `:46`'s hook leg and `:99` a hook payload
(S9); `:137` and `:170` run ordinary commands and `:152` the neutral completion, and
carry. Stage 4: `test/workflow/claude_interrupted_test.go:40` with
its controls, rebuilt on the mod. Gap: the reason taken from the adapter's end event
over a neutral TurnBoundary — closed in S10.

## Gaps and their steps

| Rule | Gap | Closed in |
|---|---|---|
| E2 | the inbox sweep removes a record only under its lock | S3 |
| E3 | a turn end's retry with its own deadline | S5 |
| E3 | a once-publication checks its recipient's run inside the recipient's critical section | S3, commit 1 |
| E4 | one durable read across every channel, the fixture's tool included | S7 |
| E6 | informing readers fold an unknown into the cautious answer | S3 |
| E7 | a hold's read-clock position: reserved, never twice, not covered before commit | S3 |
| E7 | every TurnBoundary names its event | S10 |
| E8 | a stop lifts only on returning evidence (two tests say the opposite) | S3 |
| E8 | a stop tells main once and lets letters from others arrive, whatever found it | S3 |
| E8 | nothing that names no run is swept by age | S3 |
| E8 | a publication's proof outlives every attempt admitted to its run, and only the live run's sweep retires it | S3, commit 1 |
| E8 | a cause that recurs after its resolution is a new occurrence, lifted only by its own evidence | S3 |
| E8, D | the operator decision | S18 (part B) |
| O1 | a failing check falls to publishing | S5 |
| O1 | a held end confirmed again answers its hold, never publishes as its own continuation | S5 |
| O2 | the end's reason from a neutral TurnBoundary | S10 |

The gaps of T, C and L are in [stage3-tests-tcl.md](stage3-tests-tcl.md#gaps-and-their-steps).
