# Stage 3: the fixture adapter and its column

The fixture adapter is the harness the core is proven on in stage 3: no real harness,
every capability controllable, so the neutral scenarios run end to end and each
capability's absence can be produced on purpose. Written on the 1.x contract in S5,
made the suite's gate in S6, given its tool transport in S7, re-expressed on the
capabilities in S10 ([stage3-steps.md](stage3-steps.md)). The suite today is described in
`docs/testing.md` and `docs/testing-cases.md`; citations are to `test/workflow/` and
`internal/harness/backend.go` at `b6ed4ed`.

## The adapter

`internal/harness/fixture` (from S10 `internal/adapter/fixture`), in files under the
build tag `rewakefixture`, registered by a tagged file of the catalogue — an `init`
under rule 3's exception until S10, a line of the tagged constructor after — so a build
without the tag has no fixture. The suite builds with the tag; a release never does.

**On the 1.x contract (S5).** It implements what the Codex adapter implements, so the
wrapper drives it by the path it drives Codex by, and the column works the day it lands:

- `Backend` (`backend.go:97-103`): `Start` launches the program and returns once the
  readiness exchange below has run; `Deliver` hands a letter to the program and maps its
  answer to `inbox.Result`; `Thread` answers the conversation the program reported;
  `Done` and `Close` follow the program's process.
- `ProcessBackend` (`:107`): the program's pid, which the wrapper's peer checks use.
- `ReservingBackend` (`:112`): a reservation the program answers before the letter, as
  Codex's turn start does, so the batch path runs on it.
- `ObservedBackend` (`:132`): the session state from the program's reports.
- Turn ends through the `CompletionHandler` the wrapper passes to `Start`, as Codex's:
  `Capture` at a turn's start, `EndCapture` at its end, with the end's reason — and,
  unlike Codex, `Confirm` rather than `Publish`, the neutral confirmation S5's first
  commit adds ([stage3-steps.md](stage3-steps.md#s5-the-confirmation-and-the-fixture-beside-the-others-codex)):
  the end may be held, and the program then runs one more turn with the core's reason.
  Each end names its event; a switch drops the core's answer once and confirms the same
  end again, as a harness whose answer was lost would, and expects the same hold. A
  switch makes it an adapter that cannot hold an end and calls
  `Publish`, so the limit [design-api.md](design-api.md#turnboundary) states is tested
  too. From S7 the tool's inputs reach the endpoint through the neutral input
  instead of `ToolEvent`'s raw bytes (`:88`), which leave with Codex in S8.

**Launch.** Id `fixture`, title "Fixture harness (tests only)". The plan runs the program
`fixture` found on the run's `PATH` — the suite puts a script of that name there, as it
does `claude` and `codex` (`codexsession_test.go:221`, `installShim`) — with the neutral
launch description as flags it parses strictly, so a flag the host should not pass fails
the launch. Its version is read with `fixture --version` before the claim; the minimum is
`1.0.0`; below it, or unreadable, the launch refuses. `SingleUseFlags` and
`ProtectedDirs` (`.fixture/` under the home) are declared like a real adapter's.

## The readiness exchange

The offered set is the catalogue's and fixed. What is **live** for a run is what the
program has proven, and only that; a test switches liveness, never the offered set.

1. **Hello.** The adapter listens on a socket in the run directory and passes its path
   to the program as a flag. The program connects and sends one hello: its version, its
   pid, and the capabilities it serves (Wake, TurnBoundary, Telemetry, Control, and from
   S7 ToolTransport). The adapter accepts it only from the process it started — the
   peer's credentials must give the program's pid and start time — and only once.
2. **Probes.** For each served capability the adapter sends one probe with no side
   effect and waits for its answer within the readiness bound: Wake — a probe notice the
   program acknowledges without showing it; TurnBoundary — the program's turn state
   (idle or in a turn, with the turn's id); Telemetry — its last sample or none; Control —
   "ready" without acting; ToolTransport — the descriptors it registered and their
   digest. A capability whose probe is answered becomes live; one not answered in time,
   or answered wrongly, does not, and the others are not affected.
3. **Withdrawal.** When the socket closes, every live capability is withdrawn at once.
   A reconnect is a fresh hello and fresh probes, accepted only from the same process.
   A run whose program exits withdraws everything, as the run ends.

The program withholds on switches the suite sets in its environment: `RW_SHIM_NO_WAKE`,
`RW_SHIM_NO_BOUNDARY`, `RW_SHIM_NO_TELEMETRY`, `RW_SHIM_NO_CONTROL`, `RW_SHIM_NO_TOOL`
leave a capability out of the hello; `RW_SHIM_FAIL_PROBE=<capability>` serves it and
fails its probe; `RW_SHIM_DROP_AFTER=<turns>` closes the socket after that many turns;
`RW_SHIM_HELPER_REQUESTS` sends every request from a child process, which the peer check
must refuse; `RW_SHIM_NO_HOLD` reports ends that cannot be held; from S7
`RW_SHIM_REUSED_TURNS` withholds the declaration that turn ids are never reused.

**Who reads liveness, when.** In S5 the adapter keeps the live set itself: `Start`
returns once the exchange is done, `Deliver` answers a failed delivery without a live
Wake, and no `Publish` happens without a live TurnBoundary. The host reads the live set
only from S11, when the session record carries it and the host decides by it; S10
expresses the same set as the capabilities the adapter makes live.

## What each step can prove on the fixture

| Step | Proven on the fixture | Not yet provable there, and where it is |
|---|---|---|
| S5 | the neutral scenarios through the 1.x contract: wake, steering, reservation and batches, turn ends with their reasons, the confirmation of an end, L1, L2, E3; the exchange and every switch at the adapter's own level | the host's refusals by liveness (S11); the tool (S7); the capabilities (S10) |
| S6 | the same, as the gate | — |
| S7 | the tool path: neutral input, descriptors, binding, the peer check, every T rule on the neutral rig | — |
| S10 | each capability as an interface, the readiness re-expressed | the host's decisions (S11) |
| S11 | each absence refused through its switch; a capability lost mid-run withdrawn at once; a worker's launch waiting, bounded, for a live TurnBoundary | — |

The absence of an *offered* capability is tested below the suite, in the host's own
tests, with a catalogue value holding a stub adapter (from S10).

## The program

The program is the test binary run again, as the shims are: the `fixture` script execs it
with `RW_SHIM_HARNESS=fixture`, and `TestMain` (`suite_test.go:49`) dispatches to it. It
speaks the exchange above to the adapter and keeps the suite's test-to-harness protocol —
the environment at launch and files after it (`codexshim_env_test.go:11-130`) — because
the neutral scenarios are written against that protocol, not against a harness:

- the turn's answer is `read: ` and the output of `rewake inbox` (`codexshim_turn_test.go:340`);
- the read gate (`RW_SHIM_READ_GATE`), pending once (`RW_SHIM_PENDING_ONCE`), owed once
  (`RW_SHIM_OWED_FILE`), send as asked (`RW_SHIM_SEND_*`), the request directory
  (`RW_SHIM_REQUEST_DIR`), the exit file, the calls file and the wrapped marker;
- the records the scenarios read: turns, delivered message ids, groups, reads, sends,
  the listing;
- an interrupted first turn (`RW_SHIM_INTERRUPT_FIRST_TURN`) reported as aborted with no
  answer; a held turn (`RW_SHIM_HOLD_TURN`) that a steered letter lands in; an end the
  core holds through `Confirm`, after which the program runs one more turn with the
  reason as its input and the line `RW_SHIM_PENDING_ON_HOLD` names, up to a bound — what
  the Claude Code shim does on a Stop hook's block today (`claudeshim_hook_test.go:279,296`),
  but asked through the contract, so the decision to go on is the product's, not the
  program's.

The files of that protocol that carry a harness's name and serve every column — the
`RW_SHIM_*` constants, `sendAsAsked`, the session handle `codexSession` — are renamed by
subject in S8, when their Codex half leaves; their behaviour does not change.

## The column

A third value beside the two of `column_test.go:53-74`, `fixtureColumn{harness: "fixture",
caps: names-delivered-message-ids, observes-mid-turn-arrival}`, and a third element of
`runInColumns` (`:232-238`), in S5. `buildRewake` and `buildMutant` build with
`-tags rewakefixture` through `buildFlags` (`timings_test.go:75`). Also in S5,
`TestWithdrawMidTurn` (`sent_withdraw_test.go:39`, Codex-only at `:43`) becomes neutral,
so it runs on the fixture before the fixture is the gate.

## The gate across the steps

The gate today is a harness name (`isGate`, `column_test.go:64`); `Case.acceptable`
refuses Unsupported on it (`case_test.go:317-318`) and the record carries `Gate`
(`record_test.go:35`), which `tools/checksummary` reads (`summary.go:69-72`). Three
capabilities are registered — `reports-conversation-selection` (`column_test.go:44`),
`names-delivered-message-ids` (`:48`), `observes-mid-turn-arrival`
(`codex_mid_turn_test.go:170`) — and `node` is recorded ad hoc where `node` is missing
(`claude_steered_test.go:51`, `claude_interrupted_test.go:50`, `stopped_routing_test.go:39`).

**S6 changes, in one commit:**

- **The gate is a column's field**, `gate bool`, set on exactly one column; `isGate`
  reads it. `column_gate_test.go` checks that one column is the gate and that it
  declares every registered capability except those in an exception table — capability,
  column, reason, the step that retires it — and fails on an exception that no longer
  matches. `TestUnsupportedIsAFailureOnlyOnTheGate` is written over the gate field, not
  over two harness names.
- **The fixture column is the gate**; it declares named members and mid-turn. One
  exception: `reports-conversation-selection`, Codex's, until S8.
- **Selection's observations become the Codex column's own**: "the recipient reaches an
  accepted conversation" (`codex_task_report_test.go:65-70`) and `obsReady`
  (`codex_batch_arrival_test.go:153-155`) are recorded only on a column offering
  selection, so no other column records them as Unsupported. Elsewhere readiness is
  proven by the method the Claude Code column already uses (`sessionReady`,
  `column_test.go:245-250`).
- **`node` is keyed to the Claude Code column by name**, not by "does not offer
  selection" (`stopped_routing_test.go:36,67,216`), which the fixture would match too.
- **Every control follows the gate**: the mid-turn controls hard-wired to `codexColumn`
  (`codex_mid_turn_controls_test.go:163`) take the gate column; the task-report controls'
  `bothColumns` (`codex_task_report_controls_test.go:94`) becomes every column, its two
  Claude Code rows kept to that column.
- The prose of `checksummary` (`summary.go:200-203`, `main.go:35,165`) and
  `docs/testing.md` (`:41-44,190,202,296`) says which column is the gate.

**The required runs.** The suite runs in S5 with three columns, the Codex column still
the gate, and every neutral scenario must be green on the fixture column. S6 runs it
before the change and after, all three columns, with the crosswise check both times; the
review gets both summaries and checks, case by case, that every case green on the Codex
column before is green on the fixture column after, with the same observations less the
two of selection. S8 runs it after the Codex column goes: same cases on the fixture,
green, the exception gone. S9 runs it after the Claude Code column goes. Each run
without another heavy run going (`AGENTS.md`).

**The neutral scenarios on the fixture column**, green from S5:

| Scenario | File | Needs from the fixture |
|---|---|---|
| `TestAwaitedView` | `awaited_view_test.go:21` | wake, answer, pending once, request dir |
| `TestOwedReread` | `owed_reread_test.go:18` | owed once, send as asked |
| `TestAddendumOwed` | `sent_addendum_test.go:18` | read gate, owed once |
| `TestEditAfterNotice` | `sent_edit_test.go:16` | read gate |
| `TestWithdrawAfterNotice` | `sent_withdraw_test.go:21` | read gate, the notice's recall line in the groups record |
| `TestWithdrawMidTurn` | `sent_withdraw_test.go:39` | a held turn, steering |
| `TestPendingReport` | `pending_report_test.go:19` | pending once, a second sender |
| `TestStoppedRouting` | `stopped_routing_test.go:27` | an interrupted first turn |
| `TestWrappedLaunch` | `wrapped_launch_test.go:29` | the calls file; one start; its regexp (`:163`) and start counts (`:70,93`) take the column's values |
| `TestTaskReport` | `codex_task_report_test.go:28` | named delivered ids; renamed `task_report_test.go` in S8 |
| `TestBatchArrival` | `codex_batch_arrival_test.go:30` | read each, groups, named ids; renamed in S8 |
| `TestMidTurn` | `codex_mid_turn_test.go:30` | a held turn, `SEND_WHEN_WORKING`; renamed in S8 |
| `TestPendingConfirm` | `pending_confirm_test.go:25` | `Confirm`, the pending line on hold; on the fixture from S5 beside the Stop hook's case, with its controls (`:177,181`); the hook's case goes in S9 |

New on the fixture column: an interrupted and a failed turn reported with the reason
from the end (O2); the refusals by liveness (S11); a version below the minimum and an
unreadable one refused; L1 and L2 (S5); from S7 one scenario per tool of the set
(`inbox`, `send`, `pending`, `whoami`, `retry`, `list`).

**Controls** are mutants of the product (`mutant_test.go:27`), so a neutral scenario
takes them on the fixture unchanged. Controls of removed scenarios leave with them, and
the "Every control" table of `docs/testing-cases.md` with them (`docs/controls_test.go`).

## The tool transport

From S7 the program also plays the harness's side of a tool call, for the suite and for
the neutral rig ([stage3-tests-tcl.md](stage3-tests-tcl.md#the-neutral-rig)): it receives
the descriptors, registers the six tools, and for a call reports the native call
observed, sends the request with its binding (conversation, turn and call id chosen as a
harness would, and the transport's declaration that its turn ids are never reused —
S7's one contract, [stage3-steps-adapters.md](stage3-steps-adapters.md#s7-the-tool-path-on-the-fixture-codex):
declared by default, withheld by a switch), confirms, runs
nothing itself — the CLI child of the wrapper's image runs the words — and records the
result it would hand the model, with a switch for each way the result can go wrong:
replaced, truncated, past its limit, failed, lost. A switch also offers a transport that
cannot prove a result, so T6's refusal before the first effect is tested.

## Still unknown

- How long the suite runs with three columns (S5–S7) and with the fixture alone (from
  S9); measured at those steps without another heavy run.
- Whether the program's endpoint client should be the same code as the Claude Code mod's
  stream (stage 4); written for the fixture first, compared when the mod is built.
