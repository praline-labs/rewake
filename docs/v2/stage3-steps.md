# Stage 3: the steps S1–S6

The steps of [stage3.md](stage3.md#the-build-order) up to the fixture's gate: what is
written or deleted, the tests that come with it, and what its review checks. S7–S11 —
the tool path, the removals of Codex and of the hooks, the adapter API, the host — are in
[stage3-steps-adapters.md](stage3-steps-adapters.md); the moves S12–S16 in
[stage3-moves.md](stage3-moves.md); where each declaration lands in
[stage3-packages.md](stage3-packages.md). Citations are to `b6ed4ed`. Nothing below
reads a record an earlier `v2` build wrote: the branch releases nothing before stage 7,
and its tests start from empty state directories.

## S1. The tests first

**Written.** `internal/layout_test.go`, `docs/rules/`, `docs/rules_test.go`, the archive
rule of `docs/map_test.go`; `AGENTS.md`'s checks. No package moves.

**The layout test** holds the three rules of [design.md](design.md#the-test-that-holds-it)
on the 1.x tree through two tables of its own. It judges the packages under `internal/`
and `cmd/`; `test/` and `tools/` are drivers outside the layers.

- **The transition table** maps each package not yet at its 2.0 path to its layer and
  the step that moves or removes it: `state`, `proc`, `boottime`, `buildtime` are `infra`
  until S12; `receipt`, `registry`, `sessionstate`, `control`, `grant`, `grantauth`,
  `role`, `brief`, `channel`, `inbox` are `core` until S13; `harness` is the adapter API,
  `harness/claude` an adapter and `harness/catalog` the catalogue until S10; `bridge` is
  `tool` and `bridge/endpoint`, `wrap`, `alias`, `worktree` are `host` until S16;
  `cutover` goes in S2, `harness/codex` and `bridge/server` in S8,
  `harness/claude/telemetry` in S9. An entry whose package is at its 2.0 path or gone
  fails, so the table empties with the moves.
- **The exception table** lists each known failure — the rule, the file (for rule 2 the
  token), the reason, the step — and fails on one nothing matches. The import exceptions
  from `go list -deps -test` at `b6ed4ed`:

| Edge (layers) | Files | Reason | Goes in |
|---|---|---|---|
| `cli` → `harness/claude`, `harness/claude/telemetry` (adapter) | five `cli` files (revision, layering faults) | the hook commands and the telemetry receiver | S9 |
| `cli` → `bridge/server` (tool) | `cli/bridge_serve.go` | the hidden MCP server command | S8 |
| `cli` tests → `cutover`, `harness/codex/gateway`, `harness/claude/telemetry` | `grant_temp_test.go`; the six gateway files; `pending_test.go`, `turn_hold_test.go`, `inbox_awaited_test.go` | the package's `TestMain`, Codex fixtures, the turn-start file and the stop texts | S2, S8, S9 (the starts from S7) |
| `harness` (adapter API) → `bridge` (tool) | `harness/*.go` | the 1.x contract carries tool values | S10 |
| `channel` tests (core) → `harness` (adapter) | `channel/*_test.go` | the selection space keys on harness ids | S8 |
| `bridge/server` tests (tool) → `cli`, `harness` | `rig_test.go:18`, `order_marks_test.go:58` | the rig runs the CLI's read and completion | S8 |
| `wrap`, `alias` tests (host) → `harness/catalog`, `harness/claude` | their test files | the tests take a real adapter | S10 |
| `state` → `boottime` | `state/*.go` | infra on infra (correction 1) | if refused: S12 moves boot time under `state` |

Rule 3's known failure is the registry filled from `init` (`harness/harness.go:13-23`),
excepted until S10. Rule 2's are written from the test's first run, file by file.

**Build variants.** Production code exists under two tags: the fault seam
(`rewakefault`, `state/fault_build.go`) and, from S5, the fixture (`rewakefixture`). The
layout test runs its import rule over the graph of each variant —
`go list -deps -test -json` with no tag, with each tag, with both — and rule 2 parses
every file whatever its constraint. `AGENTS.md`'s checks gain `go vet` and `staticcheck`
with `-tags rewakefault` (from S5 also `rewakefixture`, and both), so tagged code is
compiled and analysed by the five checks, not only by the rig that builds it.

**Proving the test.** Each rule fails a synthetic case with the message it promises: a
fake graph with one forbidden edge (both packages and the importing file), an edge
present only under a tag, a Go file naming a catalogue id in a comment, a string and an
identifier (file, line, token), a package with an `init` that registers an adapter. Then
the real tree, green with the tables.

**`docs/rules/` and `docs/rules_test.go`**, as [stage3-tests.md](stage3-tests.md#docsrules_testgo)
says, including the check that a named test in a tagged file is run by a check built
with its tag.

**The suite's build values checked.** A test in `test/workflow` that runs without the
switch parses each package an `-X` build value names (`timings_test.go:59-66`) and
finds its package-level string variable: the linker ignores a stale path, and a move
would otherwise build the suite at the real lengths without a failure
([stage3-moves.md](stage3-moves.md#what-every-move-lists)).

**The archive rule of the map test** ([stage3.md](stage3.md#documents)): links inside
`docs/archive-1.x/` resolve from their original directory, a record's links through the
table, and every archived file is hashed against it. Each has a negative case on a
synthetic tree: an active document's link into a moved file fails; an archived file
changed by one byte fails; a record's link to a moved path absent from the table fails.

**Review checks** the negative cases fail for the right reason; every exception has a
reason and a step; every rule of [design-rules.md](design-rules.md) is in `docs/rules/`
with its full wording; every named test exists.

## S2. The 1.x migration leaves [codex]

**Deleted**, by the cross-references of `internal/inbox` and `registry` taken with
`go/types` (`.scratch` inventory of the review round, summarized here):

- **The conversion.** `inbox/conversion.go`, `conversion_decide.go`, `held_take.go`;
  in `reconcile.go` the conversion's world — `convertReceipts`, `finishConversion` and
  their call (`:126-135`), the conversion branch of `MailboxStopped` (`:168-173`), the
  `conversion` and `receipts` fields of `mailboxRecords` (`:178-182`) and their reading
  (`:195-227`), `Settle` and its refusal (`:281-330`); the `conversionFile` exclusions
  (`reconcile.go:227`, `journal_ends.go:48`, `records.go:67`). `isStopped`
  (`reconcile.go:116`) recognises the stop's own error (`RecordedStopError`,
  `stop.go`) instead of the conversion's `StoppedError`.
- **The earlier build.** `registry.BuildStamp` (`run.go:18`), the session's `Build`
  field (`registry.go:67`, written at `wrap/claim.go:77,110`, read nowhere),
  `Session.EarlierBuild` (`run.go:43`) and `EarlierBuildEpoch` (`run.go:84`). Every
  caller loses its earlier-build branch and keeps the rest: `inbox/journal_held.go:25`
  publishes or makes moot, never holds for a successor; `inbox/notices.go:39`,
  `wrap/channel.go:264` take any main; `cli/turn_reports.go:141` checks every waiter's
  run; `cli/send.go:133` and the `errUpgraded` branches of `cli/self.go:58` go; the
  hand-made form of the same check in `readEveryWait` (`reconcile.go:261`, `ParseRun`
  with an empty boot) goes. **Kept**: `Epoch`, `RunEpoch`, `ParseEpoch`, `ParseRun`,
  `ObserveRun`, `EpochAlive`, `CurrentBoot`, `isCurrentBoot` — every run's identity and
  liveness. `registry/runs.go` goes whole: its run records are written at
  `wrap/claim.go:113` and read only by its own `Successor` (`runs.go:178`), which the
  held branch of `journal_held.go:47-93` alone consults.
- **The turn receipts directory.** From `turn_records.go` only `TurnsPath` (`:18`) and
  its uses, which the conversion and `reconcile.go:217` are. **Kept**, renamed by subject
  to `inbox/journal_retention.go` with their tests: `sweepTurnRecords` (`:29`, called by
  `sweep.go:90`) and `recordEpoch` (`:62`) — the retention of completed journals of
  ended runs, which E8 and the journal's recovery rest on.
- **The cutover.** `internal/cutover`, `wrap/cutover.go` (`claimRun` with its scan and
  `TakeHeld`, `:34,49,65`): `wrap.Run` calls `claimName` directly, and `prepareRun`
  (`wrap/claim.go:106-121`) loses the successor binding. `cli/settle.go`; the `rewake`
  mark and `cli/turn_legacy_receipt_test.go`; the cutover ldflag of the suite's build
  (`test/workflow/timings_test.go:75`); `protocol-cutover.md` to the archive. The cli
  package's `TestMain` (`cli/grant_temp_test.go:19`) keeps its override of
  `grant.TempRoots` and loses the line that points `cutover.Tree` at an empty process
  tree; it is renamed `cli/main_test.go` by subject.

**Tests.** Dropped with the code: `cli/upgraded_test.go`, `cli/settle_caller_test.go`,
`inbox/held_successor_test.go`, the conversion cases of `inbox/conversion_test.go`.
Rebuilt in this commit on a two-session lab with journal records only — valid bytes, no
earlier run — as [stage3-tests.md](stage3-tests.md#e1e8) lists: `plan_scenes_test.go`
scenes 1, 5, 9, 10, 11, `evidence_test.go:27`, `effect_stop_test.go:22,80`,
`late_unknown_test.go:23,115`, `conversion_moot_test.go:13,38`, `conversion_test.go:293`,
and `stop_writes_test.go:17,55`, which build on the same `newConversionLab`. Their
assertions do not change here; those S3 changes are carried through this commit as they
are. The suite runs, before and after: no scenario changes its result.

**Review checks** no branch reads an earlier build's record; each rebuilt scene fails
under the mutation that breaks its rule; the barrier is E8's steps and no other; the
journal retention still holds `sweep_test.go:120,164`.

## S3. A stop is resolved per cause, by evidence [codex]

**Commit 1: one landing per copy.** Before the stop changes, every once-publication — a
turn-end report, a heads-up, a channel note — reads its recipient's run, its evidence and
writes inside one critical section of the recipient's mailbox lock, and the proof of a
landing is retired only under that lock by the name's live run. The race it closes, the
contract, why it is enough and its tests are in
[stage3-publication.md](stage3-publication.md). The moot order below builds on it: the
open occurrences, under the sender's lock, then that section.

**Changed.** `inbox/stop.go` and its readers, as correction 2 of
[stage3.md](stage3.md#corrections-to-the-accepted-design) proposes:

- **A cause has a key; each time it opens is an occurrence.** The **key** is derived
  from the cause's kind (an unreadable record, a record of no known kind, an effect met),
  every path it rests on and the operation it is about when one is known — for a record
  that names no operation, kind and paths alone. An **occurrence** is one record,
  `inbox/<name>/stops/<key>/<occurrence>`, the occurrence a fresh `inbox.NewID()`,
  published with `PublishExclusive` (`state/state.go:183`): the key's parts and the time.
  A plan that finds the key while an occurrence of it is open finds that occurrence —
  repeated observation adds nothing. A plan that finds it after its last occurrence was
  resolved records a new occurrence, which needs its own resolution: a resolution answers
  the evidence it read, not every later state of the path. The key is a fixed-length hash
  and the occurrence an id, so a path's history adds entries, never a longer name.
  `stops/` joins the one list of record kinds (`records.go`). The single `stopped` file of
  1.x (`stop.go:13-22`) goes.
- **An occurrence is resolved only by evidence about its effect**: the plan, under the
  mailbox lock, reads each named path and decides the effect it is about as done or not
  done (E8's first clause). "No such file" at a named path resolves nothing — the answer
  adds "removed while stopped". An effect-only cause is resolved the same way: once a
  barrier has run its effect through to evidence about the operation it is about — done
  or not done — not by the barrier having run. **A moot disposition is no such evidence**:
  the sender's recovery finds the recipient's run ended before it reads any evidence and
  records the report moot (`journal_held.go:24-35,145-155`), which says nothing of
  whether the report landed; so it consults the open occurrences about a report before
  that check, and a report with one stays where it is — the occurrence open, no moot
  recorded — until evidence returns or, from S18, an operator decision disposes of it.
  The resolution is its own write-once record,
  `stops/<key>/<occurrence>.resolved`, naming the evidence; no stop record is edited or
  removed, and a resolved occurrence never opens again.
- **One reader.** `stopState(dir, name)` answers the open occurrences, and every reader
  takes it: the barrier and `MailboxStopped` (`reconcile.go:40-95,152-165`), and through
  it `cli/inbox.go:85,139`, `inbox_parts.go:144`, `inbox_parts_emit.go:54`,
  `read_ack.go:85,173`, `pending.go:104`. A stop holds while any occurrence is open;
  resolving one never resolves another, of the same key or of any other.
- **Main is told once per occurrence**, through a once record keyed by the occurrence id
  (`inbox/once.go`), whatever found it; a new occurrence of a key tells main again.
  Letters from others still arrive.
- **Retention**: stop records and their resolutions stay while the mailbox lives, and
  the mailbox's own sweeps skip every path an open occurrence names. A named path in
  another mailbox — the recipient's mark of a report — is not protected: its removal
  resolves nothing, as above.

Part B adds a second way an occurrence closes — disposed by an operator decision — to
the same reader ([stage3-decision.md](stage3-decision.md#the-records)); nothing in S3
waits for it.

**Tests: the assertions this step inverts**, found by reading every test under
`internal/` and `test/workflow` that removes or changes the mode of a record
(`os.Remove`, `os.RemoveAll`, `os.Chmod`) and then asserts, the search the review
repeats at the step:

| Test | Its assertion today | From S3 |
|---|---|---|
| `inbox/reconcile_stop_test.go:74,105` | the malformed record removed, the stop lifts | the stop holds, "removed while stopped" |
| `inbox/answer_stop_test.go:16` | the broken journal removed, the answer is taken | the answer stays unread, the stop holds |
| `inbox/stop_writes_test.go:17` | the unreadable interim removed, the mailbox opens | it stays stopped |
| `inbox/stop_writes_test.go:55` | a failed removal of the one stop file reaches the caller | rewritten: no stop record is removed any more; a resolution whose write fails reaches the caller, the stop holds, the next barrier writes it |
| `cli/stop_gate_test.go:17` | the broken journal removed, `pending` succeeds | `pending` still exits 1, naming the removed path |

Each inverted test keeps a second half on its own fixture — a record whose original
bytes are valid, made unreadable by mode 0000 — where restoring the mode lets the plan
decide the effect and resolves the cause; the invalid bytes of the inverted cases
(`reconcile_stop_test.go:81,118`, the `{` of the others) cannot. Carried unchanged,
because they restore valid bytes or never remove the cause: `stop_gate_test.go:53,79`,
`journal_unknown_test.go:23,148`, `journal_test.go:54`, `effect_stop_test.go:22`,
`records_test.go:41` (its removal is cleanup). New: two causes, one resolved,
the stop holds; an effect-only cause resolved by the barrier once the effect reads
evidence; the recipient's run ending while an occurrence about its report is open — the
occurrence stays open, no moot is recorded, the stop holds across barriers;
main told once across barriers and calls; letters from others arrive into a stopped
mailbox; a sweep beside a
stop removes no named path; a fault on each write of a cause and a resolution, through
the stop's fault space (`plan_faults_test.go:35`). **A recurring cause**, for a record
that names no operation — a malformed `pending/interim.json`, which its owner rewrites
each turn (`inbox/interim.go:81-113`) — and for an effect about an operation: unknown,
repaired and resolved, unknown again, then the path removed — the second occurrence stays
open and the stop holds; a key found by two barriers while open — one occurrence, main
told once; the path replaced by its owner with valid bytes after a resolution, then
unreadable again — a new occurrence, main told again. The gaps of E2, E6 and E7 close here
([stage3-tests.md](stage3-tests.md#gaps-and-their-steps)).

**Review checks** the oracles are E8's wording; nothing in `inbox` removes a stop record
or a named path.

## S4. Core-owned turn-end and read inputs

Five commits, in place, behaviour unchanged, each green; the inventory is in
[stage3-moves.md](stage3-moves.md#s14-the-turn-end-and-read-code-to-coremail). After them the turn-end
and read logic still lives in `cli` files but takes only core types, so S10's API can
carry them and S14 can move the files.

1. `errorKind.kind` gives way to `inbox.Error` (`cli/turn_reports.go:131`).
2. `turn_result.go` is split: the neutral value and `kind()` stay; the hook and notify
   payload decoder `completedTurn` goes to `cli/turn_payload.go`, which leaves in S8
   and S9; `Holdable` leaves the value and becomes an argument of the end, which only the
   hook path sets — S5 gives it its neutral caller; `Pending` stays internal, decided
   from the mark (`turn_reports.go:75`). The suite's mutation strings that name the old
   text are updated in the same commit (`test/workflow/pending_confirm_test.go:165,173`,
   `pending_report_test.go:165,173,181`, `stopped_routing_test.go:207`).
3. `inbox.AttemptScope{First, InOwnTurn}`: `judgeMark` takes it and the turn's latest
   start instead of the CLI's `Context` and `operation` (`cli/pending_turn.go:21,53`) and
   the adapter's telemetry (`:66`); `cli/pending.go:127` and `journal.go:182` compute
   both, the start still from the telemetry file until S7 gives it a core record.
4. `inbox.ReadEvidence{CallID, AnswerDigest, Whole}` and `inbox.EndGate`, the one method
   of `bridge.EndGate` (`Enter(calledBoot int64) (leave func(), ok bool)`).
   `AcknowledgeRead` becomes a CLI shim converting `bridge.Exposure` and keeping its
   order — `Whole()` first (`cli/read_ack.go:50`), then the record, then the answer;
   `markWhole` takes `(dir, session, epoch)` instead of the CLI's `readSite`
   (`:107,166,189`).
5. The neutral value becomes `inbox.TurnEnd`; `ReportCompletion` converts
   `harness.Completion` — `Error` to failed, `Stopped` to stopped, anything else to
   finished, as `cli/completion.go:13` does — and the kind strings, which feed the
   operation's hash, stay "stopped", "failed", "finished".

**Review checks** each commit changes no test outcome and no record byte; the suite runs
after the fifth.

## S5. The confirmation and the fixture, beside the others [codex]

Two commits, each green.

**1. The neutral confirmation.** The 1.x contract can publish an end but not hold one:
`CompletionHandler.Publish` returns only an error, `Completion` carries no request to
hold (`harness/backend.go:62-83`), and `ReportCompletion` never lets the end be held
(`cli/completion.go:13-17`, `turnended.go:102-104`); only the Stop hook asks. The
contract gains `CompletionHandler.Confirm func(context.Context, Completion) (string,
error)`, which [design-api.md](design-api.md#turnboundary)'s TurnBoundary describes: an
adapter that can hold an end open calls it instead of `Publish`; the core runs the end
with the hold allowed — the argument S4 made — and answers "" when it published, or the
reason to hand the session when it held and kept the answer; the adapter continues the
turn with that reason and reports its next end through `Confirm` again. The wrapper
passes it beside `Publish`, through a CLI shim `ConfirmCompletion` beside
`ReportCompletion` (`cli/launch.go:77-79`). No adapter calls it in this commit, and Codex
never will; the hook path is unchanged.

**A held end has an identity.** The hook's shortcut — any end after a hold is its
continuation (`turn_hold.go:33-38`) — was made for an end heard once. A neutral end has an
event id and may be retried: if the answer to a hold is lost and the same `Completion` is
confirmed again, `endTurnContext` finds no journal, since a hold writes none
(`turnended.go:135-155`), `holdTurn` sees the kept answer and declines, and the held end
publishes without its continuation — and without the kept answer, which lies past its
boundary (`kept.go:105-109`). So in this commit:

- `Confirm` takes only an end that names its event (`Completion.ID` with its boundary);
  one without is refused, as `ReportCompletion` refuses one without a boundary.
- A hold records, in the kept answer's one atomic write, the held end's operation
  (`turnOp`) and the reason handed back (`keptRecord`, `kept.go:29-34`, gains `Held` and
  `Reason`); the journal of the end that later takes the kept answer records that
  operation beside the version it already names.
- The journal that took the kept answer keeps the held operation in every form it takes:
  open, done in place and `.done`. The carried completion rewrites it done in place with
  only `Epoch`, `Op`, `Ended` and `Done` (`journal.go:164`); that rewrite keeps the held
  operation too, so the old held end stays on record after its continuation completes.
- Under the mailbox lock, before the hold check, an end whose operation is on record is
  answered from the record, searching each journal in its open and `.done` forms: a
  journal of it — published before, as today (`turnended.go:138-144`); the kept answer's
  `Held` — the same reason again, nothing published, nothing kept again; a journal that
  took the kept answer naming it — published before, with its continuation. Journals of a
  live run stay while it lives (`sweepTurnRecords`), and retries come only from that run.
- **A recovered hold completes its own clock position.** A hold reserves the read clock's
  position n, writes the kept answer with `Seq` n, then commits the clock's word
  (`read_boundary.go:165-172`); a crash between the last two leaves `Held`, `Reason` and
  `Seq` n durable and the word below n, and `OpenReadClock` recovers from waiters only
  (`read_boundary.go:86-101`), so a continuation with no read between would capture the
  old boundary and leave the kept answer out (`kept.go:105-109`). Before answering a
  recovered hold, under the mailbox lock, the word is raised to the kept record's `Seq`
  when it is below it: no position is allocated and the answer is not rewritten; a fully
  committed hold leaves the clock as it is, and no boundary already captured for another
  event is widened. This is E7's clock contract, finished for the hold.
- Only an end of another identity takes the kept answer, by the rules of today: a
  continuation that finishes publishes it with its own text; a failed or interrupted
  continuation publishes it too, since those are never held (`turn_hold.go:26`).

**Tests**: the oracles of the hook's cases, `cli/turn_hold_test.go:80,122,174,191,209`,
written again against `ConfirmCompletion` beside them — held once after an interim end
with live senders and no mark, published otherwise, the held answer joined with the next
end — and O1's fault case: each read and write of the check (`LastInterim`,
`KeptAnswer`, the senders' lookups, `KeepAnswer`) failing through the fault seam falls to
publishing. **The repeated end**: the answer lost and the same `Confirm` again — the same
reason, nothing published, one kept answer, the read clock unmoved; two equal `Confirm`
calls at once — one hold, both answered with its reason; a crash at each write around the
hold (the read clock's reservation, the kept answer, the clock's commit), then the same
`Confirm` — held once, never published without its continuation; the crash between the
kept answer and the clock's commit, the same `Confirm`, then the continuation with no read
between — the kept text included exactly once; the continuation after a repeated
`Confirm` takes the kept answer once; the held end confirmed again after the continuation
published, with its journal open, after the done rewrite, after the rename to `.done` and
after a retention sweep while the run lives — each answered as published, nothing
published again, no new hold; a failed and an interrupted continuation each take the kept
answer; an end without an event id refused.

**2. The fixture.** `internal/harness/fixture` behind `rewakefixture`, implementing the
1.x contract the Codex adapter implements — `Backend`, `ProcessBackend`,
`ReservingBackend`, `ObservedBackend` (`harness/backend.go:97-136`) — and calling
`Confirm` for its ends, with its program, its readiness exchange, and its column beside
the Codex and Claude Code columns, not the gate ([stage3-fixture.md](stage3-fixture.md)).
It registers through a tagged file of the 1.x catalogue (an `init`, under rule 3's
exception until S10).

**Tests.** The fixture's own: the readiness exchange, each switch withholding each
capability, withdrawal on disconnect, the version refusals. On its launch, harness-free:
L1's digest of the person's home and working directory before and after a launch, and
L2's marker in configuration and arguments absent from every refusal and note — held
until now only by the Claude Code tests that leave in S9
([stage3-tests-tcl.md](stage3-tests-tcl.md#l1l3-what-a-launch-adds)). E3's turn-end
retry with its own deadline, on the fixture's turns. **`TestPendingConfirm` and its two
controls run on the fixture column** as well as Claude Code's: its hard-wired column
(`pending_confirm_test.go:30,57,188`) becomes a parameter, and the fixture's program asks
before an end closes and continues only on the core's reason. Rebuilt on neutral
completions, beside the Codex and hook paths they ran on until S8 and S9: the end-identity
oracle of `review_receipt_identity_test.go:50,82` — two ends of one conversation with
distinct ids report twice; a stopped and then a finished end of one turn report twice,
and the finished one settles; `turn_test.go:109`, two ends at once report once;
`error_report_test.go:77,126` (the Codex notify), `:46`'s hook leg and `:99` (the hook);
and the half of `gateway_integration_test.go:50` that no notice precedes a reserved
letter's readability, on the fixture's `ReservingBackend`. The suite runs with three
columns; every neutral scenario is green on the fixture column.

**Review checks** the fixture is absent from a build without the tag; it reaches the
wrapper only through the contract's interfaces; each rebuilt oracle fails under the
mutation that breaks its rule, `TestPendingConfirm`'s two controls among them.

## S6. The fixture column becomes the gate

As [stage3-fixture.md](stage3-fixture.md#the-gate-across-the-steps) fixes: the gate is a
column's field, not the Codex name; the fixture declares mid-turn and named members; the
observations of conversation selection become the Codex column's own; one exception,
`selection`, until S8. **The suite runs before the change (Codex gate) and after
(fixture gate)**, all three columns; the review compares the two summaries scenario by
scenario, and no scenario green on the old gate is missing from the new one.

