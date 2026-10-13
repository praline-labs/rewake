# Rules: every tool transport, T1–T11

The guarantees of every tool transport, in the full wording of
[design-rules.md](../v2/design-rules.md#tools-t1t11), from `mail-bridge-server.md` rules
1–11.

These bind **every tool transport** — the Claude Code mod, a Codex MCP server, any
next one. The transport is each adapter's choice; the guarantees are not. (Restated
from `mail-bridge-server.md` 1–11 after the second revision review, R1.)

## The rules

- **T1. A transport decides no mail.** It carries a call to the core and the answer
  back, and keeps nothing a later call needs; what a call did lives in the receipts.
  Tests:
  - `internal/host/endpoint/tickets_test.go` `TestOneCallGetsOneTicket`
  - `internal/cli/bridge_tool_test.go` `TestTheToolSurface`
  - `internal/cli/bridge_tool_test.go` `TestWhoamiNamesTheMailChannel`
  - `internal/cli/bridge_rules_test.go` `TestAToolRetryKeepsToTheToolSurface`
  - `test/toolrig/flow_test.go` `TestAToolReadIsReadOnceItsAnswerArrived`
  - `test/toolrig/flow_test.go` `TestACallWithoutItsIdsRunsNothing`
  - `test/toolrig/flow_test.go` `TestWordsOffTheSurfaceAreRefused`
  - `test/toolrig/answer_bound_test.go` `TestALostAnswerOfAnEffectIsFoundAgain`
  - `test/toolrig/gaps_test.go` `TestARestartedTransportChangesNoAnswer`
  - `test/toolrig/transport_test.go` `TestTheTransportIsOfferedTheCLIsTools`

- **T2. A call runs only under a trusted binding.** The binding names the conversation,
  turn and call as the harness itself reported them, the time, the deadline and the
  digest of the normalized arguments. Nothing the model supplies — an argument, a field
  it could shape — is authority on its own. One native call gets one binding. A ticket
  issued by the wrapper after it saw the call natively is how 1.x held this on Codex;
  [design-claude.md](../v2/design-claude.md#how-a-tool-call-runs) says how the mod holds it.
  Tests:
  - `internal/host/endpoint/tickets_test.go` `TestATicketNeedsTheHarnessesOwnObservation`
  - `internal/host/endpoint/tickets_test.go` `TestTheObservationMayComeEitherSideOfTheRequest`
  - `internal/host/endpoint/tickets_test.go` `TestOneCallGetsOneTicket`
  - `internal/host/endpoint/tickets_test.go` `TestConflictingCorrelationIsRefused`
  - `internal/host/endpoint/tickets_test.go` `TestAConfirmationIsOneTime`
  - `internal/host/endpoint/tickets_test.go` `TestTheDeadlineFollowsTheTransportsTimeout`
  - `internal/host/endpoint/tickets_test.go` `TestTheObservationWaitIsTwoSeconds`
  - `internal/host/endpoint/hello_test.go` `TestAHelloIsChecked`
  - `internal/host/endpoint/hello_test.go` `TestARoleAsksOnlyItsOwnQuestion`
  - `internal/host/endpoint/order_table_test.go` `TestEveryOrderOfTheTableIssuesOneTicketPerCall`
  - `internal/tool/ticket_test.go` `TestATicketMissingItsScopeIsRefused`
  - `internal/tool/ticket_test.go` `TestTheTicketIsReadFromItsDescriptor`
  - `internal/tool/ticket_test.go` `TestAnOpenTicketDescriptorDoesNotHang`
  - `internal/tool/ticket_test.go` `TestNoEndpointRefusesEveryTicket`
  - `internal/tool/ticket_test.go` `TestTheDigestSeparatesWords`
  - `internal/cli/bridge_tool_test.go` `TestAToolCallWithoutAValidTicketRunsNothing`
  - `internal/host/endpoint/transport_test.go` `TestATransportsCallRunsInAChildOfTheWrapper`
  - `internal/host/endpoint/transport_test.go` `TestOnlyTheTransportsOwnProcessIsServed`
  - `test/toolrig/transport_test.go` `TestOnlyTheHarnesssOwnProcessIsServed`
  - `internal/host/endpoint/close_test.go` `TestARequestAfterTheTransportIsWithdrawnRunsNothing`
  - `internal/host/endpoint/transport_test.go` `TestAnAnswerGoesOnlyToTheTransportThatAsked`
  - `internal/host/endpoint/transport_test.go` `TestACallTheToolsDoNotOfferRunsNothing`
  - `internal/adapter/fixture/tool_test.go` `TestALiveToolTransportNamesTheProgramToTheEndpoint`
  - `internal/adapter/fixture/tool_test.go` `TestTheProgramsReportsBecomeTheNeutralInput`
  - `internal/host/tools_test.go` `TestAHarnessThatCarriesItsCallsIsOfferedTheTools`

- **T3. Every effect happens in the core operation, under its receipt**; a transport
  never repeats, finishes or undoes one, and starts no second operation for a call
  whose first still runs.
  Tests:
  - `internal/core/receipt/receipt_test.go` `TestRacingCallsWithOneKeyJoinOneRecord`
  - `internal/core/receipt/receipt_test.go` `TestAnUnscopedKeyNeverJoins`
  - `internal/core/receipt/receipt_test.go` `TestTheKeyScopesByRunConversationAndTurn`
  - `internal/core/receipt/receipt_test.go` `TestATokenBelongsToItsRun`
  - `internal/core/receipt/receipt_test.go` `TestTheLockHoldsOneCallAtATime`
  - `internal/core/receipt/receipt_test.go` `TestUnresolvedFindsOpenOperations`
  - `internal/cli/journal_test.go` `TestAHeadsUpRepeatedInOneTurnIsPublishedOnce`
  - `internal/cli/journal_test.go` `TestAHeadsUpPastItsDeadlineIsFinishedByRetry`
  - `internal/cli/journal_test.go` `TestTheShellRefersAnOpenOperationToItsReceipt`
  - `internal/cli/journal_test.go` `TestAPendingMarkRepeatedInOneTurnIsOneMark`
  - `internal/cli/bridge_rules_test.go` `TestAnUncertainPublicationIsNotDiscarded`
  - `internal/cli/bridge_rules_test.go` `TestAnOpenOperationIsNotBypassedByAge`
  - `internal/cli/bridge_rules_test.go` `TestALateAcknowledgmentOwesNothingTwice`
  - `internal/cli/bridge_read_test.go` `TestARepeatedToolReadInOneTurnIsTheSameRead`
  - `internal/cli/bridge_read_test.go` `TestAnEmptyToolReadIsTheSameNothingInItsTurn`
  - `test/toolrig/order_test.go` `TestOneCallRunsOnceAcrossRequests`
  - `test/toolrig/fault_test.go` `TestEveryStepOfACallSurvivesAFault`

- **T4. A call is bound to its operation before its first effect**: a record keyed by
  the native call, whose absence ("no such file") proves the call made no effect. Only
  on that proof may the answer send the same words to the shell.
  Tests:
  - `internal/cli/bridge_rules_test.go` `TestAnOpenOperationIsNotBypassedByAge`
  - `internal/cli/bridge_rules_test.go` `TestAToolRetryKeepsToTheToolSurface`
  - `internal/cli/bridge_rules_test.go` `TestAShellRetryDoesNotReadStdinAgain`
  - `internal/cli/bridge_read_test.go` `TestTheShellFinishesALostToolRead`
  - `internal/cli/journal_test.go` `TestTheShellRefersAnOpenOperationToItsReceipt`
  - `test/toolrig/child_bound_test.go` `TestAnOutlivedCallWithABindingNamesItsRetry`
  - `test/toolrig/answer_bound_test.go` `TestALostAnswerOfAnEffectIsFoundAgain`
  - `test/toolrig/fault_test.go` `TestEveryStepOfACallSurvivesAFault`
  - `test/toolrig/gaps_test.go` `TestTheShellRunsTheSameWordsOnTheProofOfAbsence`

- **T5. Everything read and written is bounded, in one place each.** A request is
  bounded before it is parsed; every answer passes one encoder and one bound on its
  final bytes; an answer that does not fit is replaced whole, never clipped.
  Tests:
  - `internal/tool/ticket_test.go` `TestOnlyAWholeResultIsEvidence`
  - `internal/tool/ticket_test.go` `TestCutKeepsCharactersAndBounds`
  - `internal/cli/bridge_tool_test.go` `TestALongAnswerComesInParts`
  - `internal/cli/bridge_rules_test.go` `TestAContinuationRefreshesALetterBeforeAnyPart`
  - `internal/cli/bridge_rules_test.go` `TestAnEscapedDiagnosticFits`
  - `internal/cli/bridge_rules_test.go` `TestAnAnswerThatCannotBeKeptStillFits`
  - `internal/cli/bridge_rules_test.go` `TestEveryTextPartNamesItself`
  - `internal/host/endpoint/hello_test.go` `TestTheConnectionsAreBounded`
  - `internal/host/endpoint/answer_test.go` `TestTheEncoderReplacesAnAnswerThatDoesNotFit`
  - `internal/host/endpoint/answer_test.go` `TestAChildsOutputIsCapped`
  - `internal/host/endpoint/answer_test.go` `TestOnlyTheEncoderAnswers`
  - `internal/host/endpoint/answer_test.go` `FuzzEveryAnswerFits`
  - `internal/host/endpoint/answer_test.go` `FuzzRequestsAreBounded`
  - `test/toolrig/answer_bound_test.go` `TestATransportThatStopsReading`
  - `internal/host/endpoint/transport_test.go` `TestATransportsRequestIsBoundedBeforeItIsParsed`
  - `test/toolrig/transport_test.go` `TestTheEndpointBoundsWhatComesIn`
  - `internal/host/endpoint/transport_test.go` `TestAnAnswerPastItsBoundIsReplacedWhole`
  - `internal/tool/descriptor_test.go` `TestACallsArgumentsBecomeTheCommandsWords`

- **T6. A letter is read only on proof that the call's own whole result reached the
  model.** The proof is the harness's record of the result for that native call, equal
  to the answer the core recorded before returning it, direct, successful, and inside
  the harness's **effective** result limit — the limit under which the harness passes a
  result whole rather than a preview. Text that merely contains a letter proves nothing.
  A reading tool whose transport cannot give that proof **refuses before its first
  effect**, and the shell (`rewake inbox`) stays the way to read. A child process that
  marks read on printing is no way round this: under a tool call its output is not yet
  shown to the model (`cli/inbox.go:114-119`; persisted output seen on 2.1.289,
  `.scratch/v2-recon/claude-mods.md:52`).
  Tests:
  - `internal/tool/ticket_test.go` `TestOnlyAWholeResultIsEvidence`
  - `internal/cli/bridge_read_test.go` `TestAToolReadIsReadOnlyWhenEveryPartArrived`
  - `internal/cli/bridge_read_test.go` `TestAWithdrawalMeetsAToolRead`
  - `internal/cli/bridge_lookups_test.go` `TestAReadThatCouldNotSeeTheLetterFreezesNothing`
  - `internal/cli/bridge_lookups_test.go` `TestAClaimedLetterThatCannotBeReadLeavesTheMarkOpen`
  - `internal/cli/bridge_lookups_test.go` `TestAnAcknowledgmentDoesNotWriteOverAWaiterItCannotRead`
  - `internal/cli/bridge_lookups_test.go` `TestARetriedAcknowledgmentStopsOnAStatusItCannotRead`
  - `internal/cli/bridge_lookups_test.go` `TestAKeptAnswerThatCannotBeReadIsNotDropped`
  - `internal/cli/bridge_unknown_test.go` `TestARefusedReadIsReplayedInItsTurn`
  - `internal/cli/bridge_unknown_test.go` `TestALetterThatCannotBeLookedUpIsNotShown`
  - `internal/cli/bridge_unknown_test.go` `TestAWaiterThatCannotBeReadLeavesTheMarkOpen`
  - `internal/cli/bridge_rules_test.go` `TestAnExpiredReadShowsAndClaimsNothing`
  - `internal/cli/bridge_rules_test.go` `TestALetterReadElsewhereShowsAsGone`
  - `internal/host/endpoint/input_test.go` `TestOnlyAUsedTicketWithItsBindingIsAcknowledged`
  - `internal/host/endpoint/input_test.go` `TestTheWholeResultAndItsSize`
  - `test/toolrig/flow_test.go` `TestAToolReadIsReadOnceItsAnswerArrived`
  - `test/toolrig/flow_test.go` `TestALongReadGoesOnThroughItsNextWords`
  - `test/toolrig/flow_test.go` `TestAFailedResultReadsNothing`
  - `test/toolrig/flow_test.go` `TestAToolThatReadsOffLeavesTheLetterUnread`
  - `test/toolrig/evidence_test.go` `TestOnlyTheRecordedAnswerItselfIsEvidence`
  - `test/toolrig/evidence_test.go` `TestACompletionWhoseBindingCannotBeReadIsSpent`

- **T7. A call's commits stop at its turn's end, and a pending mark speaks only for
  its own turn.** Two conditions, both needed:
  - **Not ended.** A pending mark or a read's acknowledgment checks, under the mailbox
    lock, for an end of the run on record at or after its time, or a turn start
    recorded after it — the later of the start the host records in the mailbox from
    the neutral input and, until S9, the telemetry file — and in the host for ends it
    has heard and not yet recorded;
    found, nothing is committed and the letter shows again.
  - **Its own turn, proven.** A pending mark is written only by an attempt proven to
    run in the turn its operation was made in: the attempt that created the operation,
    or a later one whose binding names the same conversation and turn on a transport
    whose turn ids are proven never reused. No absence proves the turn still open —
    of a later turn's start, of an end. An attempt that cannot prove it does not mark:
    the outcome is **unproven**, the mark is finished as not made, and the answer says
    why (`cli/pending_turn.go:7-22`, `inOwnTurn`; `core/mail/pending_mark.go:46`,
    `VerdictUnproven`). This binds the shell, `retry` and every tool transport alike:
    a `retry` from a later turn never writes an earlier turn's mark. Until an adapter
    proves its turn ids are never reused — on Claude Code across an interrupt, a
    reload, `/clear` and resume (P3, P9) — only the first attempt counts there, as 1.x
    counts it for `prompt_id`.
  The carried test includes a lost end with a `retry` from another turn.
  Tests:
  - `internal/tool/gate_test.go` `TestAnAcknowledgmentAfterANotedEndWritesNothing`
  - `internal/host/endpoint/tickets_end_test.go` `TestACapturedTurnIssuesNoMoreTickets`
  - `internal/host/endpoint/tickets_end_test.go` `TestARunPastTheTicketsItRemembersRefuses`
  - `internal/host/endpoint/tickets_end_test.go` `TestTheLastTicketARunRemembersIsIssuedOnce`
  - `internal/host/endpoint/tickets_end_test.go` `TestNoTicketWhileACaptureWaitsForAWriter`
  - `internal/cli/pending_test.go` `TestAPendingTurnEndKeepsTheTaskOwed`
  - `internal/cli/pending_test.go` `TestAMarkDoesNotSurviveAnInterruptedTurn`
  - `internal/cli/pending_test.go` `TestALatePublishedTurnIsAReportWithItsOwnText`
  - `internal/cli/pending_test.go` `TestAFailedOrStoppedTurnIgnoresTheMark`
  - `internal/cli/bridge_rules_test.go` `TestAPendingMarkIsNotMadeAfterItsDeadline`
  - `internal/cli/journal_test.go` `TestARefusedMarkIsReplayedInItsTurn`
  - `test/toolrig/order_call_test.go` `TestEveryOrderOfACallAgainstItsTurnsEnd`
  - `test/toolrig/order_gen_test.go` `TestEveryOrderOfTwoTurnsKeepsTheEndsBoundary`
  - `test/toolrig/order_marks_test.go` `TestAMarkBetweenCaptureAndJournal`
  - `test/toolrig/flow_test.go` `TestAnAnswerAfterTheEndReadsNothing`
  - `test/toolrig/flow_test.go` `TestACallOutsideAnOpenTurnRunsNothing`
  - `test/toolrig/order_end_test.go` `TestAnEndOnRecordTheGateNeverHeardRefusesTheAcknowledgment`
  - `test/toolrig/order_end_test.go` `TestAnEndCapturedBeforeAnAcknowledgmentsCheckLeavesTheLetterUnread`
  - `internal/cli/turn_declared_test.go` `TestAnAttemptNotProvenInItsTurnIsMarkUnproven`
  - `internal/cli/turn_declared_test.go` `TestALostEndWithARetryFromAnotherTurnMarksNothing`
  - `internal/cli/turn_declared_test.go` `TestADeclarationOnOneEndOnlyProvesNoTurn`
  - `internal/cli/bridge_unknown_test.go` `TestAWaiterThatCannotBeReadLeavesTheMarkOpen`
  - `internal/cli/journal_test.go` `TestAPendingMarkRepeatedInOneTurnIsOneMark`
  - `internal/host/endpoint/input_test.go` `TestATicketCarriesItsTransportsDeclaration`
  - `internal/host/turn_start_test.go` `TestABoundaryWithoutAToolRecordsItsStart`
  - `internal/host/turn_start_test.go` `TestABoundaryWithAToolRecordsItsStartOnceAndOpensItsTurn`
  - `internal/cli/turn_declared_test.go` `TestALostEndWithALaterStartOnRecordMarksNothing`
  - `test/workflow/fixture_turn_start_test.go` `TestFixtureTurnStart`
  - `internal/core/mail/turn_start_test.go` `TestTheLatestTurnStartIsTheLargestReading`
  - `internal/core/mail/turn_start_test.go` `TestAnUnreadableTurnStartIsAnError`

- **T8. An end's boundary is a cut between commits**, taken at the end's own event —
  the clock when no acknowledgment is between its check and its close, or the snapshot
  the acknowledgment takes after its last write under the lock. Each acknowledgment is
  applied once.
  Tests:
  - `internal/tool/gate_test.go` `TestACaptureTakesTheClosingSnapshotOfTheWritingAcknowledgment`
  - `internal/tool/gate_test.go` `TestACaptureThatWaitedDoesNotSampleAgain`
  - `internal/tool/gate_test.go` `TestACaptureWithNothingWritingSamplesUnderTheMutex`
  - `internal/tool/gate_test.go` `TestOnlyOneAcknowledgmentWrites`
  - `internal/cli/turn_scope_test.go` `TestARetryWhoseFirstAttemptRecordedNothingKeepsItsScope`
  - `internal/cli/turn_scope_test.go` `TestARetryTakesOnlyTheAnswerItsFirstAttemptSaw`
  - `internal/cli/turn_scope_test.go` `TestAnEndWithNothingOwedKeepsItsScopeOnRetry`
  - `internal/cli/turn_journal_test.go` `TestATurnEndThatCannotRecordItsSequencePublishesNothing`
  - `internal/cli/turn_journal_test.go` `TestAnUnpublishedReportIsCompletedByTheNextTurnEnd`
  - `internal/cli/turn_journal_test.go` `TestAnAdoptedWaitWhoseReportIsOutIsNotAnsweredAgain`
  - `internal/cli/turn_journal_test.go` `TestAnEndWhoseJournalFailedIsPreparedAgainOnRetry`
  - `internal/cli/turn_journal_test.go` `TestAnEndRecordedBeforeItsEffectsIsNotPreparedAgain`
  - `internal/cli/turn_journal_test.go` `TestARecoveredEndTakesTheAnswerItPublished`
  - `internal/cli/turn_journal_test.go` `TestALateRetryKeepsALaterAnswer`
  - `internal/cli/turn_journal_test.go` `TestAPendingEndWhoseJournalFailedStaysPending`
  - `internal/cli/turn_journal_retry_test.go` `TestARecoveredPendingEndKeepsItsInterimOnRecord`
  - `internal/cli/turn_journal_retry_test.go` `TestALateRetryAfterTheSweepPublishesNothing`
  - `internal/cli/turn_journal_retry_test.go` `TestARetryAnswersOnlyWhatItsFirstAttemptSaw`
  - `internal/cli/turn_retry_test.go` `TestRetriedTurnDoesNotConsumeLaterWork`
  - `internal/cli/turn_retry_test.go` `TestPartialTurnRetryKeepsOriginalOutcome`
  - `internal/adapter/fixture/order_test.go` `TestAnEndCapturesItsBoundaryWhenItsFrameArrives`
  - `internal/adapter/fixture/order_test.go` `TestATurnStartCapturesItsBoundaryWhenItsFrameArrives`
  - `test/toolrig/order_test.go` `TestAnEndCapturedDuringAnAcknowledgmentIncludesIt`
  - `test/toolrig/order_cut_test.go` `TestAnAcknowledgmentCutMidway`
  - `test/toolrig/order_cut_test.go` `TestAnEscNobodyHeard`
  - `test/toolrig/order_gen_test.go` `TestEveryOrderOfTwoTurnsKeepsTheEndsBoundary`
  - `test/toolrig/order_marks_test.go` `TestAMarkBetweenCaptureAndJournal`

- **T9. Every wait has a bound, and nothing waits holding what it waits for**; a wait
  that runs out leaves a defined outcome. A write begun under a held lock is not timed.
  Tests:
  - `internal/host/endpoint/tickets_test.go` `TestTheDeadlineFollowsTheTransportsTimeout`
  - `internal/host/endpoint/tickets_test.go` `TestTheObservationWaitIsTwoSeconds`
  - `internal/tool/gate_test.go` `TestOnlyOneAcknowledgmentWrites`
  - `internal/host/endpoint/tickets_end_test.go` `TestNoTicketWhileACaptureWaitsForAWriter`
  - `internal/core/receipt/receipt_test.go` `TestTheLockHoldsOneCallAtATime`
  - `internal/core/receipt/receipt_test.go` `TestTheSweepRespectsAHeldLock`
  - `internal/cli/bridge_rules_test.go` `TestAnExpiredReadShowsAndClaimsNothing`
  - `internal/cli/bridge_rules_test.go` `TestAPendingMarkIsNotMadeAfterItsDeadline`
  - `internal/tool/ticket_test.go` `TestAnOpenTicketDescriptorDoesNotHang`
  - `internal/adapter/fixture/order_test.go` `TestASilentProbeCostsOnlyItsCapability`
  - `internal/adapter/fixture/order_test.go` `TestCloseWaitsForACallIntoTheHandler`
  - `internal/adapter/fixture/order_test.go` `TestNothingReachesTheHandlerOnceClosed`
  - `internal/adapter/fixture/order_test.go` `TestCloseWaitsForAFrameBeingHandled`
  - `internal/adapter/fixture/order_test.go` `TestNothingReachesTheHandlerWhileClosing`
  - `test/toolrig/child_bound_test.go` `TestAChildPastItsDeadline`
  - `test/toolrig/answer_bound_test.go` `TestATransportThatStopsReading`
  - `test/toolrig/transport_test.go` `TestTheEndpointFinishesItsCallsAsItCloses`
  - `internal/host/endpoint/close_test.go` `TestCloseLetsTheCallsItTookConfirmTheirChildren`
  - `test/toolrig/transport_test.go` `TestAFifthCallIsBusy`
  - `test/toolrig/transport_test.go` `TestACallNobodyWaitsForIsNotAnswered`

- **T10. One build per room.** The wrapper, whatever runs the core for a call, and the
  CLI of every session of a room are one build — one artifact, by the hash of its
  content, not by its version line; every writer holds the room's build lease while it
  acts, and anything of another build is refused before it acts
  ([design-state.md](../v2/design-state.md#one-build-per-room)). How the build is
  identified and how the lease holds a room are rules of their own, with their tests, in
  [state.md](state.md#the-builds-identity); the tests below hold the tool's side of it:
  the hello and the peer check by the build's content, a copy admitted, a call refused in
  a room of another build before its ticket is confirmed, an orphaned child refused after
  a switch, and the lease held through a bounded answer's continuation.
  Tests:
  - `internal/host/endpoint/hello_test.go` `TestAHelloIsChecked`
  - `internal/host/endpoint/peer_test.go` `TestPeerCheckIsTheBuildsContent`
  - `test/leaserig/lease_test.go` `TestArtifactCopyCanJoinRoomAndUseTools`
  - `internal/cli/lease_test.go` `TestBothRoutesRefuseAnotherBuildsRoom`
  - `test/leaserig/lease_test.go` `TestOrphanChildRefusesAfterBuildSwitch`
  - `internal/cli/view_test.go` `TestToolOutputContinuationHoldsLease`

- **T11. A failing tool never weakens the mail.** The shell takes the same words under
  the same receipts; the surface, the read boundary and the refusals are the CLI's.
  Tests:
  - `internal/cli/bridge_read_test.go` `TestTheShellFinishesALostToolRead`
  - `internal/cli/bridge_rules_test.go` `TestAShellRetryDoesNotReadStdinAgain`
  - `internal/cli/shell_channel_test.go` `TestEveryShellCommandLeavesItsEvidence`
  - `internal/cli/shell_channel_test.go` `TestTheShellEvidenceKeepsTheTimeOfItsWrite`
  - `internal/core/receipt/shell_test.go` `TestTakeShellOverEveryEntry`
  - `internal/core/receipt/shell_test.go` `TestTheBoundKeepsTheNewest`
  - `internal/core/receipt/shell_test.go` `TestEveryErrorHasItsClass`
  - `internal/core/receipt/shell_test.go` `TestAnUnwritableStateLosesTheObservation`
  - `internal/cli/journal_test.go` `TestTheShellRefersAnOpenOperationToItsReceipt`
  - `internal/tool/ticket_test.go` `TestNoEndpointRefusesEveryTicket`
  - `test/toolrig/fault_test.go` `TestEveryStepOfACallSurvivesAFault`
  - `test/toolrig/gaps_test.go` `TestOneSetOfWordsThroughTheToolAndTheShellIsOneOperation`

## The carried order and fault tests

The carried fault and order tests (`bridge/server/fault_test.go`, `order_cut_test.go`,
`order_gen_test.go`, `host/endpoint/order_table_test.go`) hold T2–T8; their oracles
are transport-neutral and run against every transport
([design-docs-tests.md](../v2/design-docs-tests.md#tests)). S7 rebuilt them on the neutral
rig ([stage3-tests-tcl.md](../v2/stage3-tests-tcl.md#the-neutral-rig)) in `test/toolrig`:
the fixture's program is the harness's process, its tool calls reach the host endpoint's
transport role, and the faults reach the program, the child and the endpoint's own steps.
Each rebuilt oracle fails under the mutation its old one failed under. The old ones left
with `bridge/server`'s rig in S8 and are kept in
[archive/1.x/codex](../../archive/1.x/codex/README.md).
