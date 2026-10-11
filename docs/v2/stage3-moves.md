# Stage 3: the moves S12–S16

The steps of [stage3.md](stage3.md#the-build-order) that change where code lives and
nothing else: what each move rests on, the preparatory commits that make it a rename,
and what its review reads. The removals and behaviour changes are done by then
([stage3-steps.md](stage3-steps.md)), so a move carries only the code 2.0 keeps.

## What every move lists

A move is a rename only once nothing it moves still reaches back, and nothing left
behind reaches in by a name the rename breaks. So each move starts from an inventory of
the tree it starts from, taken at the step with `go/types` (the `Uses` of every
identifier, tests included), not with grep. Of the inventories this pass rests on, only
`inbox`'s (S13, S15) was type-checked; the turn-end inventory (S4, S14) was taken by
reading and grep, the launch-helper and event-input one (S7, S10) by grep with its
in-package callers approximate, and the test classes by grep and reading; each is marked
so where it is used, and each is retaken with `go/types` at its step, the review comparing
the two. An inventory lists:

- **declarations by file** and, for a package that splits, which side each file is on;
- **references across the cut**, both directions, non-test and test, with the methods
  whose receiver is declared on the other side;
- **consumers outside the package** — non-test, test, `cmd/rewake`, `tools/`,
  `test/workflow`;
- **the closure of every test that moves**: every identifier its file reaches, then every
  identifier those reach, through the package's other test files — helpers, fixtures,
  package variables, `run(...)`, `Context` — and the packages its imports bring. A test
  moves only when its whole closure moves with it or lives in a lower layer's test helper
  package (`registrytest`, `grantauthtest`, from S15 `core/mail/mailtest`); otherwise it
  stays and reaches the moved code through its package's own entry points. At `b6ed4ed`
  the test imports of the packages moving whole, from `go list -test`, stay inside `core`
  and `infra` but for `channel`'s import of `harness`, which S8 removes;
- **consumers by string**, which no compiler sees:
  - the suite's mutations name a source file by path (`mutation.file`,
    `test/workflow/mutant_test.go:27`); 96 of them at `b6ed4ed`, across `cli`, `inbox`,
    `wrap`, `worktree`, `control`, `grant`, `grantauth` and the adapters. A mutation
    whose text is not found exactly once fails its control (`mutant_test.go:69`), so a
    missed path turns the suite red rather than passing silently;
  - the suite's `-X` build values name a package path (`test/workflow/timings_test.go:59-66,83`):
    `inbox.builtQuiet`, `inbox.builtCap`, `cli.builtPickup`, `wrap.builtLetterWait`,
    `wrap.builtNoticeScan`, and two of Codex's gateway that leave in S8. The linker
    ignores `-X` on a symbol that does not exist, so a stale path would build at the real
    lengths and slow the suite without failing it. **S1 adds a test**, run with the five
    checks, that parses each named package and finds the named package-level string
    variable;
  - `tools/release/gate.go:178` names `internal/cli/registry.go`; `docs/code.md`'s tree
    (held by `docs/layout_test.go`); the test paths of `docs/rules/` (held by
    `docs/rules_test.go`); the transition, file-group and exception tables of the layout
    test.

A row that needs more than an import path changed is resolved by a preparatory commit
in place, before the move, behaviour unchanged. The move then changes import paths,
package clauses and the string consumers above, and nothing else; its review reads it as
a rename (`git diff -M` with the similarity of every file shown) and checks the string
consumers by their tests.

## S12. `infra` moves

`state` (with the fault seam behind `rewakefault`), `proc`, `boottime`, `buildtime` to
`internal/infra/*`. **Inventory**: they import only the standard library and each other
(`state` → `boottime`, correction 1); every other package imports them; no mutation and
no build value names them. **Consumers to rewrite**: import paths everywhere,
`docs/code.md`, the transition table (four entries go). Nothing else.

## S13. `core` moves

`receipt`; `registry` with `registrytest`; `sessionstate` as `core/session`; `control`;
`grant`; `grantauth` as `core/grant/authority` and `grantauthtest` beside it as
`core/grant/authority/authoritytest` (a subpackage, so the move stays a rename — merging
the two would not be); `role`;
`brief`; `channel`; and **the whole of `inbox` as `core/mail`**, the delivery server
inside it for now. **Inventory** (`go/types`, and `go list -test` for the tests): `inbox` imports only the
standard library, `boottime`, `buildtime`, `receipt`, `registry`, `role`, `sessionstate`
and `state` — all `core` or `infra` — and its tests add `registrytest` and `proc`, so it
may move whole with them; none of these packages imports `harness`, `bridge`,
`wrap` or `cli`, the import rule's exceptions having gone in S8–S10. **String
consumers**: the mutations in `inbox` (13) and `grantauth` (3) — `control` and `grant`
had one each at `b6ed4ed` and have none since the columns of S8 and S9 left with theirs
(recounted October 11, 2026); the build values `inbox.builtQuiet` and `inbox.builtCap`.

`control` moves with `Serve`, which S11 connects to the fixture's live Control; until S11
only its tests reach it (S8's review, October 8, 2026).

The delivery server's files stay together in `core/mail` until S15, which the layout
test's table of file groups records — `core/mail`: `serve`, `batch`, `window`,
`watch_linux`, `held`, `retention`, `announcement`, `availability`, `thread`, `outcome`
and `settle_recorded` (newer than the inventory at `b6ed4ed`), bound for `host/delivery`
in S15. The entry records the group and nothing more: its files are judged by `core`'s
import rule like the rest of `core/mail`, as strictly as before the move, and it fails
once a file it names is gone or it names none, so S15 removes it (decided October 11,
2026). Splitting the package in the same commit as the move would make the move a
rewrite; doing it here, before the turn-end code joins it, would split it twice.

## S14. The turn-end and read code to core/mail

**What S4 already did.** The logic takes core values: `inbox.TurnEnd`,
`inbox.AttemptScope`, `inbox.ReadEvidence`, `inbox.EndGate`, `inbox.Error`, the run as
`(dir, session, epoch)`. After S8 and S9 the external test callers are gone with their
packages (`bridge/server/order_marks_test.go:58`, `rig_test.go:111`,
`harness/codex/report_boundary_test.go:84`), and so are the hook payload decoder and
the hook command of `cli/turnended.go`; its turn-end functions remain. S9 also renamed
`hookLockWait`, which every end waits under, to `turnEndLockWait`, and `printHold` left
with the hook command.

**The inventory**, taken with `go/types` over the four build variants at `2846a50`
(October 11, 2026; the first, at `b6ed4ed`, was by reading and grep). What moves reaches
nothing in `cli` outside itself and no package `core/mail` does not already import:

| Declarations | Goes to | Why |
|---|---|---|
| `endTurnContext`, `completeTurnContext`, `turnMark`, `turnEndLockWait` (`cli/turnended.go`) | `core/mail` (`turn_end.go`) | the end under the mailbox lock; the wait serves every end, not only a hook's |
| `prepareTurnReports`, `publishTurnContext`, `turnOp`, `reportOfOp` | `core/mail` (`turn_reports.go`) | the journal, the reports and their identities |
| `holdTurn`, `holdReason`, `liveSenders`, `joinTurnText` | `core/mail` (`turn_hold.go`) | the hold, taken as an argument since S4 and set by the neutral confirmation since S5, with the held end's identity; the last two sit in `turn_hold.go` and move with it |
| `completeTurn` (`turnended.go:18`) | stays in `cli`, in a test file | a wrapper only tests call (65 uses); production code with no production caller does not stay in a production file |
| `markVerdict` and its constants, `judgeMark` | `core/mail` (`pending_mark.go`) | the mark's verdict |
| `inOwnTurn`, `attemptScope` | stay with their callers in `cli` (`journal.go:183`, `pending.go:118`) | they read the ticket and the binding's declared property; the core gets the booleans, as `inbox.AttemptScope` since S4 |
| `acknowledgeRead`, `acknowledge`, `answeredBy`, `markWhole`, `ackBudget`, `ErrNotWhole`, `ErrTurnEnded` | `core/mail` (`read_ack.go`) | the read's acknowledgment |
| `ReportCompletion`, `ConfirmCompletion`, `AcknowledgeRead` | stay in `cli` as shims | they convert the adapter's completion and the tool's exposure; `cli/launch.go:68-85` keeps passing them |
| `settleLetters`, `readSite` | stay in `cli` | the shell read's path (`inbox_parts_emit.go:123`) |

**The preparatory commits**, in place, behaviour unchanged (decided October 11, 2026):

1. The test seam `beforeReports`, a package variable read inside the end, becomes the
   field `BeforeReports` of `TurnEndOptions`, which the core function takes; `cli` keeps
   its variable and fills the option at its four calls (`ReportCompletion`,
   `ConfirmCompletion`, `completeTurn`, the test helper `endTurn`), so no test reaches
   into another package's variable and none changes. Its setters:
   `cli/turn_test.go:90-106`, `neutral_end_test.go:89-106`,
   `bridge_lookups_test.go:203-207`, `turn_journal_retry_test.go:85-89` and eight places
   in `turn_journal_test.go`; each assigns it before the end that should see it.
2. The three files that hold both sides are split so each moving file moves whole:
   `turnended.go` becomes `turn_end.go` without `completeTurn` (to `turn_end_test.go`)
   and `beforeReports` (to `completion.go`); the verdict and `judgeMark` leave
   `pending_turn.go` for `pending_mark.go`; `AcknowledgeRead` and `settleLetters` leave
   `read_ack.go` for `read_settle.go`.
3. The names `cli`'s staying code calls are exported under their `core/mail` names:
   `EndTurn`, `CompleteTurn`, `JudgeMark`, `MarkVerdict` with `VerdictWrite`,
   `VerdictFound`, `VerdictTurnEnded`, `VerdictUnproven` (the type's word, so none reads
   as an action beside `mail.MarkRead`), `Acknowledge` (`AcknowledgeRead` is the shim's),
   `AcknowledgeParts` and `MarkWholeRead`.

**The move**: the five whole files `git mv`ed to `core/mail`, `mail.` dropped inside
them and added at `cli`'s calls; `ErrNotWhole` and `ErrTurnEnded` move without an
alias, their consumers (`cli/bridge_read_test.go:85`,
`test/toolrig/order_end_test.go:52,109`) taking the new qualifier. **String
consumers**: the mutations of `turn_reports.go` (5), `turn_hold.go` (1) and
`turnended.go` (1, on `turnMark`) get their new paths; their texts name only locals,
fields and `joinTurnText`. One more, `end-thread-ignored`
(`test/workflow/fixture_thread_test.go`), edits `cli/completion.go`, which stays, but
quotes its two calls into the end, so its texts follow the option, the export and the
qualifier. None names `turn_result.go` or `turn_payload.go` since S8 and S9 (recounted
October 11, 2026).

**Tests: none moves.** The closure rule above keeps them in `cli`: 328 of its 336 tests
reach the moving code, and each also reaches `cli`'s own helpers — `turn_scope_test.go`
reaches `readKind`, which runs `rewake inbox` (`cli/turn_journal_test.go:25-32`), and
`completeTurn`, `toolSession`, `boundaryNow`, `owes`, `answersTo` of other `cli` test
files; `turn_journal_retry_test.go` the same helpers; `review_receipt_identity_test.go`
is gone since S8. Two name the moving code directly: the helper `endTurn` calls the end,
and `TestAnAttemptNotProvenInItsTurnIsMarkUnproven` (`cli/turn_declared_test.go`) calls
`judgeMark` with the verdict's constants; it stays in `cli` against `mail.JudgeMark`,
its other helpers being `cli`'s (decided October 11, 2026). A test written later on the
core's own types belongs in `core/mail`.

## S15. The delivery server to `host/delivery` [codex]

**Inventory at `b6ed4ed`** (`inbox` cut into mail, delivery and the migration S2
removes): 453 declarations in 47 files, 61 white-box test files. Twelve methods sit on
the far side of their receiver; six remain after S2, all `(*Server)` methods in mail
files — `followEarlierRun` (`adopt.go:121`), `checkGrants` (`grants.go:43`),
`failGranted` (`:80`), `prepareDelivery` (`reservation.go:23`), `sweepFinished`
(`sweep.go:16`), `sweepFinishedLocked` (`:25`) — and S13's inventory adds a seventh,
`liveRun` in `sweep.go`, newer than `b6ed4ed`. Mail identifiers used by delivery: 71;
delivery identifiers used by mail: 31, all but nine of them the `Server`'s fields and
methods read inside those six methods.

**The preparation**, in place in `core/mail`: several commits by subject, each one
behaviour-unchanged and each followed by the four-variant inventory, rather than the one
commit first planned (decided October 11, 2026, on the retaken inventory):

1. The seven `Server` methods move to delivery files, taking every use of `Server`'s
   fields and lock with them. Where a method's body is mail's, the body stays in mail as
   an exported function named for its precondition, and the method keeps what is the
   server's (ownership below).
2. A delivery-declared identifier that mail still uses after that moves to a mail file:
   the types `Availability` and `UndeliveredNotice` (fields of `Message`,
   `inbox.go:38-39`), `removeWaiting` (`withdraw.go:187`), `answerLifetime`
   (`records.go:65`), `keepFinished` (`adopt.go:34`), `deliveryThread`
   (`adopt.go:53,97`), the errors `ErrThreadUnavailable` and `ErrNotYet`,
   `ReportThreadChanged` (`thread.go:53`, which calls `deliveryThread`), called by the
   turn-end reports in `core/mail/turn_reports.go` since S14, and whichever
   of `retentionPath`, `retainedReceipts`, `threadPath`, `keepThreadRecord` the retaken
   inventory still finds on the mail side.
3. A mail identifier delivery uses is exported, with a doc comment that says why
   delivery needs it — or, when delivery is its only user, moves to delivery: among
   them `awaitedHere`, `claimed`, `grantAlone`, `owedByAnyRun`, `joins`, `withdrawn`,
   `Status.final`, `outcome` and `result`, `isFinal`, `list`, `writeStatus`, `isIn`,
   `move`, `listIn`, `linkUnread`, `dropUnread`, `lookUp`, and `sweepTurnRecords` (the
   sweep calls it, `sweep.go:90`).
4. The tests are split: a test that uses a delivery identifier goes to the delivery
   files; one that uses only mail stays. A delivery test that builds mail state does it
   through mail's exported API or through `core/mail/mailtest`, exported helpers for
   tests; a production identifier is never exported only for a test. 39 of the test
   files touch both sides at `b6ed4ed`. A test whose subject is a mail body the server
   only arranged — a sweep of finished mail, a delivery-thread record — stays in mail
   and arranges it itself, under the real mailbox lock. `mailtest` holds fixtures of
   paths and bytes, each checked by a mail test against mail's own reader or writer, and
   never a second settlement. A write that must fail fails through `state.Fault`, which
   fails one exact path before `WriteAtomic` writes.

**Ownership** (decided October 11, 2026). Mail keeps the body of the sweep of finished
mail with everything it runs — the retirement of letters, claims, marks, turn records,
retained receipts and thread records — settlement with its look at copies and at the
mailbox's stop, the stop's retention, the sweep of an earlier run's waits, and the
synthetic `withdrawn` outcome, which is no status on disk. They are exported as
`SweepFinishedLocked`, `SettleLocked` and `SweepAwaitingLocked`: the caller holds the
mailbox lock, and none takes it, since the lock is not reentrant. Delivery keeps
`Server.sweepFinished` — its context, the acquisition of the mailbox lock and the
live-run read `liveRun` — and calls mail's function inside its existing locked callback;
settlement is called from `settleOutcome` and `settleUnreadOnly`, inside their sections,
and the awaiting sweep from `followEarlierRun`, in its present order and without a
live-run read. No lock is released and taken again at the package boundary, and
settlement gains no live-run check.

**The caller restriction.** A structural test in `internal`
(`internal/layout_callers_test.go`) restricts the production
references to those three functions — calls and function values, in every build
variant: `SweepFinishedLocked` only from `(*Server).sweepFinished`, `SettleLocked` only
from `settleOutcome` and `settleUnreadOnly`, `SweepAwaitingLocked` only from
`followEarlierRun`. It fails on a new caller and on an expected caller that no longer
calls. This is a discipline of the repository, not a capability of the lock: nothing at
run time proves the caller holds it.

**The fault exception (c)** (decided October 11, 2026). Two schedules cross the package boundary and cannot be
built on exported operations, files or `state.Fault`: a waiting copy whose removal fails
while the server settles (the failing removal is mail's `removeWaiting`, not a state
operation), and the witness that settlement lists the mailbox's stops only while
holding the lock. For them, and only them, package mail carries test operations in a
file built under `rewakefault` — an exact-path failure of a waiting copy's removal that
counts its attempts, and an observer scoped to one mailbox's listing of its stops — and
the delivery tests that use them are built under the same tag, so they run in the
`rewakefault,rewakefixture` race run of the five checks. The operations are exported
functions only: no private type, global or alias of the stop machinery crosses, and no
production hook exists in an untagged build.

After it, `go/types` finds no reference from a mail file into a delivery file, which
the review checks by rerunning the inventory.

**The move**: the delivery files to `host/delivery` (`package delivery`), importing
`core/mail`; the layout test's file-group entry goes. **String consumers**: the
mutations of `serve`, `outcome` and `thread`, one each, and none of `held` (S13's count,
October 11, 2026; `b6ed4ed` had `held`, `outcome`, `serve` (2), `thread`); `wrap`'s
construction of the server. **[codex]**: the delivery server is process behaviour, and the split decides
which lock and which sweep belong to whom.

## S16. `tool` and `host` move

`bridge` (the ticket, the parts, `ResultCap`) and the end gate (`bridge/endpoint/gate.go`)
to `tool`; the rest of `bridge/endpoint` to `host/endpoint`; `wrap` to `host`; `alias` to
`host/launch`; `worktree` to `host/worktree`. **Inventory**: the end gate's one method is
already declared again in core (S4), so `tool` imports no host; `bridge/endpoint`'s
neutral input (S7) leaves no adapter parser behind. **String consumers**: the mutations
of `wrap` (11) and `worktree` (5); the build values `wrap.builtLetterWait` and
`wrap.builtNoticeScan`. The review files renamed by subject (answer 9;
[design-docs-tests.md](design-docs-tests.md#the-review-files)).
