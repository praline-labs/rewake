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
    `docs/rules_test.go`); the transition and exception tables of the layout test.

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
`grant`; `grantauth` as `core/grant/authority` and `grantauthtest` beside it (a
subpackage, so the move stays a rename — merging the two would not be); `role`;
`brief`; `channel`; and **the whole of `inbox` as `core/mail`**, the delivery server
inside it for now. **Inventory** (`go/types`, and `go list -test` for the tests): `inbox` imports only the
standard library, `boottime`, `buildtime`, `receipt`, `registry`, `role`, `sessionstate`
and `state` — all `core` or `infra` — and its tests add `registrytest` and `proc`, so it
may move whole with them; none of these packages imports `harness`, `bridge`,
`wrap` or `cli`, the import rule's exceptions having gone in S8–S10. **String
consumers**: the mutations in `inbox` (12), `control`, `grant`, `grantauth`; the build
values `inbox.builtQuiet` and `inbox.builtCap`.

The delivery server's files stay together in `core/mail` until S15, which the transition
table records as a file-group entry — `core/mail`: `serve`, `batch`, `window`,
`watch_linux`, `held`, `retention`, `announcement`, `availability`, `thread`, `outcome`
are `host` until S15 — so the layout test knows the group is in transit and fails once
it is gone. Splitting the package in the same commit as the move would make the move a
rewrite; doing it here, before the turn-end code joins it, would split it twice.

## S14. The turn-end and read code to core/mail

**What S4 already did.** The logic takes core values: `inbox.TurnEnd`,
`inbox.AttemptScope`, `inbox.ReadEvidence`, `inbox.EndGate`, `inbox.Error`, the run as
`(dir, session, epoch)`. After S8 and S9 the external test callers are gone with their
packages (`bridge/server/order_marks_test.go:58`, `rig_test.go:111`,
`harness/codex/report_boundary_test.go:84`), and so are the hook payload decoder and
the hook command of `cli/turnended.go`; its turn-end functions remain.

**The inventory at `b6ed4ed`** (the cli files' declarations, by reading and grep —
retaken with `go/types` at the step):

| Declarations | Goes to | Why |
|---|---|---|
| `endTurnContext`, `completeTurnContext`, `turnMark`, `hookLockWait` (`cli/turnended.go:102-187`) | `core/mail` (`turn_end.go`) | the end under the mailbox lock; the wait serves every end, not only a hook's |
| `prepareTurnReports`, `publishTurnContext`, `turnOp`, `reportOfOp`, `liveSenders`, `joinTurnText` | `core/mail` (`turn_reports.go`) | the journal, the reports and their identities |
| `completeTurn` (`turnended.go:98`) | stays in `cli` | a wrapper only tests call |
| `holdTurn`, `holdReason` | `core/mail` (`turn_hold.go`) | the hold, taken as an argument since S4 and set by the neutral confirmation since S5, with the held end's identity |
| `printHold` | stays in `cli` | it writes the command's output |
| `markVerdict` and its constants, `judgeMark` | `core/mail` (`pending_mark.go`) | the mark's verdict |
| `inOwnTurn` | stays with its callers in `cli` (`journal.go:182`, `pending.go:127`) | it reads the ticket and the binding's declared property; the core gets the boolean |
| `acknowledge`, `answeredBy`, `markWhole`, `ackBudget`, `ErrNotWhole`, `ErrTurnEnded` | `core/mail` (`read_ack.go`) | the read's acknowledgment |
| `ReportCompletion`, `ConfirmCompletion`, `AcknowledgeRead` | stay in `cli` as shims | they convert the adapter's completion and the tool's exposure; `cli/launch.go:77-89` keeps passing them, `ConfirmCompletion` beside `ReportCompletion` since S5 |
| `settleLetters`, `readSite` | stay in `cli` | the shell read's path (`inbox_parts_emit.go:123`) |

**The preparatory commit**, in place: the test seam `beforeReports`, a package variable
that cli tests set (`cli/turn_test.go:116-132`, `bridge_lookups_test.go:203-207`),
becomes a field of the options the core function takes, which `cli` fills from its own
variable, so no test reaches into another package's variable; `hookLockWait`, which
every end waits under (`turnended.go:125`), is renamed for what it is.

**The move**: those declarations to `core/mail` files; `cli` imports them back.
**String consumers**: the mutations of `turn_reports.go` (5), `turn_result.go` (2),
`turn_hold.go` (1) and `turnended.go` (1, on `turnMark`, `:165`) get their new paths in
the same commit.

**Tests: none moves.** The closure rule above keeps them in `cli`: `turn_scope_test.go`
reaches `readKind`, which runs `rewake inbox` (`cli/turn_journal_test.go:25-32`), and
`completeTurn`, `toolSession`, `boundaryNow`, `owes`, `answersTo` of other `cli` test
files; `turn_journal_retry_test.go` the same helpers; `review_receipt_identity_test.go`
drove the gateway until S8 and keeps only `:109`, which calls `ReportCompletion`. None of
the `cli` tests names an unexported identifier that moves (a search for each of the
table's names at `b6ed4ed`; `turnResult` is `inbox.TurnEnd` from S4, `ErrNotWhole` is
exported, `beforeReports` is the option of the preparatory commit), so each keeps
holding its rule through `cli`'s entry points. A test written later on the core's own
types belongs in `core/mail`.

## S15. The delivery server to `host/delivery` [codex]

**Inventory at `b6ed4ed`** (`inbox` cut into mail, delivery and the migration S2
removes): 453 declarations in 47 files, 61 white-box test files. Twelve methods sit on
the far side of their receiver; six remain after S2, all `(*Server)` methods in mail
files — `followEarlierRun` (`adopt.go:121`), `checkGrants` (`grants.go:43`),
`failGranted` (`:80`), `prepareDelivery` (`reservation.go:23`), `sweepFinished`
(`sweep.go:16`), `sweepFinishedLocked` (`:25`). Mail identifiers used by delivery: 71;
delivery identifiers used by mail: 31, all but nine of them the `Server`'s fields and
methods read inside those six methods.

**The preparatory commit**, in place in `core/mail`:

1. The six `Server` methods move to delivery files, taking every use of `Server`'s
   fields and lock with them.
2. A delivery-declared identifier that mail still uses after that moves to a mail file:
   the types `Availability` and `UndeliveredNotice` (fields of `Message`,
   `inbox.go:38-39`), `removeWaiting` (`withdraw.go:187`), `answerLifetime`
   (`records.go:65`), `keepFinished` (`adopt.go:34`), `deliveryThread`
   (`adopt.go:53,97`), the errors `ErrThreadUnavailable` and `ErrNotYet`, and whichever
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
   files touch both sides at `b6ed4ed`.

After it, `go/types` finds no reference from a mail file into a delivery file, which
the review checks by rerunning the inventory.

**The move**: the delivery files to `host/delivery` (`package delivery`), importing
`core/mail`; the transition table's file-group entry goes. **String consumers**: the
mutations of `held`, `outcome`, `serve` (2), `thread`; `wrap`'s construction of the
server. **[codex]**: the delivery server is process behaviour, and the split decides
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
