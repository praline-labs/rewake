# Stage 3: the rules of the core build

The rules stage 3 is built by, written October 5, 2026 on the `v2` branch at `b6ed4ed`,
for review before any code; revised the same day after the first, second and third review
rounds, and after the owner answered its questions. Stage 3
builds the core of 2.0: the layout and its test, `core`, `tool`, `host` and `cli` with the
full vocabulary on the shell, the carried tests with their rules, the state root, one
build per room, and a fixture adapter. Its acceptance is the stage 3 row of
[design.md](design.md#the-stage-plan-from-here); these files say how the build gets
there one reviewable commit at a time.

| Document | What it fixes |
|---|---|
| this file | what holds in every step, the build order, what each step needs and from where, the documents, the corrections to the accepted design, what is still unknown, the owner's answers |
| [stage3-steps.md](stage3-steps.md) | steps S1–S6: the tests first, the migration, the stop, the turn-end inputs, the confirmation and the fixture, its gate |
| [stage3-publication.md](stage3-publication.md) | S3's first commit: a publication's liveness, evidence and write in one critical section of the recipient's lock, and the proof retired only under it |
| [stage3-steps-adapters.md](stage3-steps-adapters.md) | steps S7–S11: the tool path, the removals of Codex and of the hooks, the adapter API, the host |
| [stage3-moves.md](stage3-moves.md) | steps S12–S16: the inventories each move rests on, the preparatory extractions, the moves |
| [stage3-packages.md](stage3-packages.md) | every 1.x package, file group and split declaration with its 2.0 place or its removal, and the step |
| [stage3-tests.md](stage3-tests.md) | how a rule names its tests, `docs/rules_test.go` with the build variants, E1–E8 and the turn outcomes |
| [stage3-tests-tcl.md](stage3-tests-tcl.md) | T1–T11, C1–C8, L1–L3 and the host, with their tests and gaps |
| [stage3-fixture.md](stage3-fixture.md) | the fixture adapter, its readiness exchange, its column and the gate across the steps |
| [stage3-state.md](stage3-state.md) | the state root `v2/` and its inverse, the build id, the room's lease and what is a writer |
| [stage3-upgrade.md](stage3-upgrade.md) | clearing the state of 1.x: the supported upgrade and its exclusion protocol |
| [stage3-decision-model.md](stage3-decision-model.md) | part B: the operator decision as an abstract model — the world, its outcomes, what a decision is answerable for, correction 9 |
| [stage3-decision.md](stage3-decision.md) | part B: subject, authority with correction 8, command, records, what every reader consults |
| [stage3-decision-recovery.md](stage3-decision-recovery.md) | part B: the decision's record, installing its effects, composition, the rules D1–D8 and their tests |

Citations of 1.x code are to the tree at `b6ed4ed`. **[unverified]** marks what was
inferred from names, comments or a grep rather than read through. Of the inventories
behind these rules, `inbox`'s was type-checked with `go/types` and the test imports
taken with `go list -test`; the turn-end inventory was taken by reading and grep, the
launch-helper and event-input inventory by grep, the test classes by grep and reading.
Every inventory is retaken with `go/types` at its step
([stage3-moves.md](stage3-moves.md#what-every-move-lists)).

## What holds in every step

- **Each step is one commit with the five checks green** (`AGENTS.md`, "Checks"), unless
  it lists numbered commits, each of which keeps them green. `internal/layout_test.go`
  and `docs/rules_test.go` are green from S1 on. A step that cannot keep them green is
  split, never landed red.
- **A move is a rename only when its inventory says so.** Before a package or file group
  moves, its declarations and their consumers, tests included, are listed; every
  receiver, private helper, value owned by a lower layer and input the core must define
  for itself is resolved by a preparatory commit in place, behaviour unchanged. Only
  then is the move a rename with imports changed, and its review reads a rename
  ([stage3-moves.md](stage3-moves.md)). A behaviour change never rides in a move.
- **A replacement runs before its predecessor goes, and exists before its first
  user.** A rig, an oracle, a driver or a gate is replaced by a step that runs the new
  one beside the old one, green, before a later step deletes the old one; and no step
  uses what a later step makes. Each replacement, where it starts, who first needs it and
  when its predecessor goes, is in [the table below](#what-each-step-needs-and-from-where).
- **A behaviour change names the assertions it changes.** The step that changes
  behaviour lists every existing assertion it inverts or rewrites, found by a search the
  review repeats, and lands them in its own commit; a step that changes no behaviour
  changes no assertion (S3's table, S5's rebuilt oracles, S11's search).
- **The workflow suite runs at every step that changes what it drives**, named in the
  step table: the five checks only compile it. The step's review gets the summary lines
  of `tools/checksummary` for each run; a step that changes the gate gets a run before
  and after.
- **Removal takes its tests and its documents with it.** A test that holds a carried
  rule and rode only on removed machinery is rebuilt in that commit, or earlier, or named
  as a gap with the step that closes it ([stage3-tests.md](stage3-tests.md#gaps-and-their-steps)).
- **The core names no harness.** From S1 the layout test fails on a new mention; the old
  ones are exceptions with a reason and the step that removes them.
- **The chain of `AGENTS.md`**: the writer writes the step; a session on the
  orchestrator's harness reviews it; the writer fixes. Steps on transport, protocol,
  process behaviour and the core's effects also pass Codex-side acceptance before the
  commit — marked **[codex]**. The stage as a whole is accepted Codex-side once more.

## The build order

One line per step; S1–S6 are detailed in [stage3-steps.md](stage3-steps.md), S7–S11 in
[stage3-steps-adapters.md](stage3-steps-adapters.md), S12–S16 in
[stage3-moves.md](stage3-moves.md). "Suite" says whether the workflow suite runs.

| Step | What it does | Suite | Review |
|---|---|---|---|
| S1 | the tests first: `internal/layout_test.go` over every build variant, `docs/rules/`, `docs/rules_test.go`, the archive-aware link resolution and the archive's hash table | — | review |
| S2 | the 1.x migration leaves; the live parts of `turn_records.go` and `registry/run.go` stay | yes | review [codex] |
| S3 | one landing per copy: a publication inside its recipient's lock (commit 1); a stop is a write-once record per cause, resolved per cause only by returning evidence | yes | review [codex] |
| S4 | core-owned turn-end and read inputs, in place, five commits, behaviour unchanged | yes | review |
| S5 | the neutral confirmation in the 1.x contract; the fixture adapter on it, its readiness exchange, its column beside Codex and Claude Code; the oracles of the Codex and hook paths rebuilt on it; the harness-free L1 and L2 tests | yes | review [codex] |
| S6 | the fixture column becomes the gate | before and after | review |
| S7 | the tool path on the fixture: the neutral endpoint input, descriptors, binding, the per-request peer check, the neutral rig beside the old one | yes | review [codex] |
| S8 | Codex, the MCP injection and `bridge/server` leave; the channel space on the neutral alphabet | yes | review [codex] |
| S9 | Claude Code's hook machinery leaves: it offers Launch and Wake; a reporting role refuses on it | yes | review |
| S10 | the adapter API and the catalogue as a value; `harness` becomes `adapter`; the launch helpers placed by the import rule | yes | review |
| S11 | the host on the live capability set | yes | review [codex] |
| S12 | `infra` moves | — | review |
| S13 | `core` moves, the whole of `inbox` to `core/mail` with the delivery server inside it for now | yes | review |
| S14 | the turn-end and read code moves from `cli` to `core/mail` | yes | review |
| S15 | the delivery server leaves `core/mail` for `host/delivery`: one preparatory commit, one move | yes | review [codex] |
| S16 | `tool` and `host` move | yes | review |
| S17 | the build id and the room's lease with its bootstrap, then the state root `v2/`, read-only views, the write seam, clearing 1.x | yes | review [codex] |
| S18 | the operator decision (part B) | yes | review [codex] |
| S19 | closing: every gap closed, the tables empty, `AGENTS.md` and the documents, the stage acceptance | yes, with the crosswise check | review, then the stage [codex] |

**Why this order.** The removals and the behaviour changes come before the moves, so the
moves carry less code and run under the fixture gate. Codex stays until the gate and the
tool rig it drives have replacements that ran (answer 5 fixes that Codex leaves the tree
in stage 3, not in which step). The fixture first speaks the 1.x contract
(`harness.Backend`, `harness/backend.go:97-117`), whose methods are already harness-free,
so the wrapper drives it through the path it drives Codex by and the column works on the
day it lands; S10 re-expresses it, with Claude Code, on the capabilities. **The layout
test is proven first, in S1**: a transition table maps each 1.x package to its 2.0
layer, its three rules each fail a synthetic case before they are trusted, and every
known failure is an exception with its reason and step that fails once nothing matches
it ([stage3-steps.md](stage3-steps.md#s1-the-tests-first)).

## Two acceptances: part A and part B

Recovery after an operator decision drew findings in every review round, while the rest of
the stage converged; so the stage is accepted in two parts, each with its own verdict,
reviewed in the same round.

**Status, October 5, 2026.** Part A was accepted in review round 4, with two local
corrections to S5's held-end identity, applied: the held operation kept in every form of
the journal that took the kept answer, and a recovered hold completing its own clock
position. S1 may start. Review round 5 found a race in the carried publication — an
ordinary retry landing one report twice — and main placed its fix in part A as S3's first
commit ([stage3-publication.md](stage3-publication.md)); that contract was accepted in
review round 7, with two test wordings applied as the review gave them. Part B was
accepted in review round 7.

- **Part A — S1–S17 and S19**: everything but the operator decision, in every document
  except the three below.
- **Part B — S18**: the operator decision and its recovery,
  [stage3-decision-model.md](stage3-decision-model.md),
  [stage3-decision.md](stage3-decision.md) and
  [stage3-decision-recovery.md](stage3-decision-recovery.md). It is accepted before S18
  is built.

**What part A relies on from part B: nothing**, with one qualification: S19's rules are
accepted with part A, but S19 cannot run or close the stage before S18 is built. S1–S17
are built, tested and committed with no decision. Between S2, which removes 1.x's `settle`, and S18, an unknown outcome is
lifted only by evidence about its effect and a stop stays until then — affordable, since
the branch releases nothing before stage 7. Where a part A document names the decision, it
says what S18 will add, never what part A needs: the disposition beside S3's resolution,
the D rules in `docs/rules/`, `decide`'s forms in the command table, its row in the
package map.

**What part B relies on from part A**: S3's occurrences, each naming the operation it is
about; S3's publication contract, on which I4 rests
([stage3-publication.md](stage3-publication.md)); S4's core-owned turn-end inputs; the fixture of S5 with its Control, for the
compaction case; S17's lease and write seam. A change to any of these after part A's
acceptance is read by part B's review. **S19** closes the stage, so it is built after
S18; nothing else in part A waits.

## What each step needs and from where

Every replacement, the step that makes it and runs it beside its predecessor, the first
step that relies on it, and the step that removes what it replaces. A row whose user
came before its maker would be a step that cannot be built; the review of each step
checks its rows.

| Replacement | Made, run beside the old | First relied on | Predecessor goes |
|---|---|---|---|
| a once-publication inside its recipient's lock, its proof retired only there by the live run | S3, commit 1 | S3 (the moot order), S5, S7, S18 (I4) | the liveness check before the lock, S3 commit 1 |
| core-owned turn-end and read inputs (`inbox.TurnEnd`, `AttemptScope`, `ReadEvidence`, `EndGate`) | S4 | S5 (the fixture's ends), S10, S14 | — (the CLI's own values, S4) |
| the neutral confirmation of an end (`CompletionHandler.Confirm`) | S5, commit 1 | S5, commit 2 (the fixture; `TestPendingConfirm` on its column) | the Stop hook's hold, S9 |
| the fixture adapter and its column | S5 | S6 (the gate) | the Codex column S8, the Claude Code column S9 |
| the oracles of the Codex notify and hook ends, the end identity, the reserved letter's readability, on neutral completions | S5 | — | the gateway and notify S8, the hook S9 |
| L1 and L2 without a harness | S5 | — | the Claude Code hook and plugin tests, S9 |
| the gate as a column's field, on the fixture | S6 | S8 | `isGate` on the Codex name, S6 |
| the neutral endpoint input and `test/toolrig` | S7 | S8 | `bridge/server`, the Codex parser S8; the hook path S9 |
| the transport's declaration that its turn ids are never reused | S7 | S7 (`inOwnTurn`) | `CodexTransport`, S8 |
| the turn's latest start as a core record | S7 | S7 (`judgeMark`) | the telemetry file, S9 |
| the adapter API and the catalogue value | S10 | S11 | the 1.x contract and `init` registry, S10 (re-expressed in one commit, the suite before and after) |
| the bootstrap of the room's directories (`build.Acquire`) | S17, commit 1 | S17, commit 2 (resolvers create nothing) | the resolvers' creation, S17 commit 2 |
| the lease | S17, commit 1 | S17, commit 4 (`ErrNoLease`) | — |
| the 1.x admission, apart from deletion | S17, commit 5 | the same commit | — |

The tests follow the same rule: a test whose closure — the helpers and drivers it
reaches, not only its own file — depends on something a step removes is rebuilt in that
step or earlier ([stage3-moves.md](stage3-moves.md#what-every-move-lists)); the test
files importing a removed package at `b6ed4ed` are each assigned in the exception table
of [S1](stage3-steps.md#s1-the-tests-first).

## Documents

- **`docs/rules/` is created in S1** with its `README.md` and one file per group:
  `effects.md` (E1–E8, the decision from S18), `turns.md` (O1–O2), `tools.md`
  (T1–T11), `channel.md` (C1–C8), `launch.md` (L1–L3), `host.md`. Each states its rules
  in the full wording of [design-rules.md](design-rules.md), independent of the code, and
  ends with the tests that hold them ([stage3-tests.md](stage3-tests.md#how-a-rule-names-its-tests)).
  S19 adds `layout.md`, `adapter-api.md` and `state.md`.
- **`docs/archive-1.x/` is created in S2** for the documents of code each step removes:
  the cutover's in S2 (`protocol-cutover.md`, archived rather than dropped as
  [revision-docs.md](revision-docs.md#records--the-archive-of-1x) says, because
  records link to it), the Codex and MCP ones with `mail-bridge-server.md` in S8, the
  Claude Code hook, plugin and telemetry ones in S9 (`testing-plugin.md` and
  `grants-claude.md` included: their 1.x text is archived, the mod's is written in
  stages 4 and 6). A moved document keeps its bytes.
- **How the links stay true.** Moving a document unchanged breaks its links to
  documents that stay, and the links of unchanged records into it
  (`docs/map_test.go:134` resolves from the linking document's directory). So S1 adds
  one archive-aware rule to the map test, reviewed with it: the archive's `README.md`
  holds a table of every moved document — its path before the move, the step, the
  SHA-256 of its bytes. A link inside `docs/archive-1.x/` resolves from the directory the
  document was moved from, then through the table; a link in a **record** (everything
  under `roadmap/` and `archive-1.x/`, `reviews*.md`, the records list of
  [revision-docs.md](revision-docs.md#records--the-archive-of-1x)) to a moved path
  resolves through the table. Every other document gets no such help: its links into
  what moved are rewritten in the same commit, and the map test fails on each one left.
  A test hashes every archived file against the table, so "unchanged" is checked, not
  promised. The cost, said plainly: on a forge's file view, a link inside an archived
  document opens a path that is not there; the archive's `README.md` says so and lists
  where each document went. Each archive step runs `go test ./docs/` as part of the
  five checks, with the map, links, anchors and hashes.
- **What waits for stage 7**: the root map's split, `research/` by harness,
  `adapters/claude/` (stage 4), the fold of `docs/v2/` into `rules/`, the records'
  move. Research stays in place: a fact about Codex is still a fact, dated.
- **`AGENTS.md`** changes in S1 (the five checks gain the build variants, the layout and
  rules tests; the reading order adds `docs/rules/`), in S8 (Codex's commands and the
  version cache go), in S10 ("Adding a harness" as capabilities plus one line in the
  catalogue's constructor) and in S19.
- **The documentation tests on every step**: `docs/map_test.go` (map, links, anchors,
  the archive rule), `docs/layout_test.go` (the tree of `docs/code.md` equal to the
  packages: every step that moves or removes a package rewrites it),
  `docs/controls_test.go` (the controls table equal to the suite's mutants: S2, S8 and S9
  update it), `docs/legacy_test.go` (S2 removes the migration's marks, S8 Codex's, S17
  adds the one `rewake` mark), `docs/rules_test.go` ([stage3-tests.md](stage3-tests.md#docsrules_testgo)).

## Corrections to the accepted design

Found while writing these rules; the accepted documents are not changed by this pass.
Each item is a proposal with the reading these rules use until it is decided; the
review's answer to each is noted.

1. **`infra` cannot be "the standard library only"** ([design.md](design.md#the-import-rule)):
   `state` imports `boottime` (`go list`). Proposed: `infra/*` imports the standard
   library and other `infra` packages, the graph acyclic. Review: agreed.
2. **E8's two sentences on lifting a stop** ([design-rules.md](design-rules.md#effects-e1e8)):
   "a stop the plan found goes once the plan finds its cause gone" read literally lets
   the removal of an unreadable record lift it, which "never by removing its cause"
   forbids; the code and two tests do the first (`inbox/stop.go:13-22`,
   `inbox/reconcile_stop_test.go:74,105`). Proposed, with the review's refinement: a
   stop is one write-once record per occurrence of a cause; an occurrence is resolved
   only by evidence about its effect — the effect proven done or proven not done — never
   by its path becoming absent or merely readable; an effect-only cause is resolved once
   a barrier has run its effects through; one resolution never resolves another, and a
   cause that recurs after its resolution is a new occurrence. Built in S3
   ([stage3-steps.md](stage3-steps.md#s3-a-stop-is-resolved-per-cause-by-evidence-codex)). Review: agreed with this refinement.
3. **The suite's gate is the Codex column** (`test/workflow/column_test.go:64`), which
   answer 5 removes. Proposed: the fixture column runs beside it (S5) and becomes the gate
   (S6) before the Codex column goes (S8), with the exact policy of
   [stage3-fixture.md](stage3-fixture.md#the-gate-across-the-steps). Review: a
   replacement gate is needed; the earlier hole was refused, this order is its fix.
4. **Answers 3 and 5 together**: stage 3's Claude Code has no TurnBoundary, so a role
   that must report refuses on it. These rules read it as Launch plus Wake (the 1.x
   messaging line until P1, [design-claude.md](design-claude.md#wake-and-delivery)).
   Review: supported. Decided by the owner on October 5, 2026 (answer 1 below).
5. **`accept`** is named among the shell-only commands of answer 2 but exists only for
   a Codex resume (`cli/accept.go:1-3`); it leaves in S8 and returns in stage 5.
   Review: agreed.
6. **`tool/mcp`.** The revision puts `bridge/server` there; the design lays out no
   `tool/mcp` before stage 5. Now: nothing of `bridge/server` outlives Codex — the
   neutral rig of S7 replaces its tests, and it leaves with Codex in S8. Its neutral
   framing (`frames.go`, `encoder.go`, `child.go`) is history stage 5 may read.
7. **The map test is not carried unchanged** ([design-docs-tests.md](design-docs-tests.md#documents)
   says it is): the archive-aware rule above is added in S1, because a record that
   moves unchanged cannot otherwise keep its links.
8. **The decision's authority is the recipient run**: part B,
   [stage3-decision.md](stage3-decision.md#authority). Decided by the owner on October 5,
   2026 (answers 4 and 5 below).
9. **The originals are kept as the observations the decision was made on, not untouched
   for good**: part B, in the normative form the third review recommended,
   [stage3-decision-model.md](stage3-decision-model.md#correction-9-in-its-normative-form).
   For review with part B.

## Still unknown

- **How noisy rule 2 is**: 89 non-test and 177 test lines in the core packages name a
  harness [unverified: a grep for the ids]. S1 writes the exceptions it finds.
- **The cost of hashing the executable cold.** One warm measurement under load, not
  evidence; S17 measures ([stage3-state.md](stage3-state.md#the-cost-of-the-hash)).
- **Whether the inbound messaging line works without the 1.x hooks** that S9 removes
  (`harness/claude/lane.go:1-4` [unverified beyond the file heads]); checked live
  before S9's commit.

## The owner's answers

Decided by the owner on October 5, 2026, each as recommended; the first review had
supported each recommendation. Each decision is written once, in the place the rules
that follow from it live, and the other documents link there.

1. **Stage 3's Claude Code offers Launch and Wake only**, with no interim turn
   boundary; a reporting role on it refuses at the launch, available from stage 4.
   Canonical: the refusal paragraph of
   [S9](stage3-steps-adapters.md#s9-claude-codes-hook-machinery-leaves).
2. **The command is `rewake decide <name> <report> --arrived | --not-arrived`.**
   Canonical: [the command](stage3-decision.md#the-command).
3. **A stop with no subject is lifted by no command**; rewake names the records and why
   nothing can decide them, the person repairs the file, and the diagnostic does not
   suggest that a new name or room settles the old obligations. Canonical:
   [what can be decided](stage3-decision.md#what-can-be-decided).
4. **The person's path stays**, marked unverified, main and the recipient told once;
   every failed or unreadable identity check refuses. Canonical:
   [authority](stage3-decision.md#authority).
5. **Verified authority is the recipient run only**: a later run of the recipient's
   name, or the room's main for another session's report, does not decide — the person
   does. The owner's condition: a compaction does not take the authority away from the
   run. Canonical: [authority](stage3-decision.md#authority).
