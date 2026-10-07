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
  - `internal/bridge/endpoint/tickets_test.go` `TestOneCallGetsOneTicket`
  - `internal/cli/bridge_tool_test.go` `TestTheToolSurface`
  - `internal/cli/bridge_tool_test.go` `TestWhoamiNamesTheMailChannel`
  - `internal/cli/bridge_rules_test.go` `TestAToolRetryKeepsToTheToolSurface`
  - `internal/bridge/server/flow_test.go` `TestAToolReadIsReadOnceItsAnswerArrived` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/flow_test.go` `TestACallWithoutItsIdsRunsNothing` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/flow_test.go` `TestWordsOffTheSurfaceAreRefused` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/stdout_bound_test.go` `TestALostAnswerOfAnEffectIsFoundAgain` — rebuilt in S7 on the neutral rig
  - Gap: the transport keeps nothing a later call needs — closed in S7 (a transport restarted between two calls of one turn changes no answer).

- **T2. A call runs only under a trusted binding.** The binding names the conversation,
  turn and call as the harness itself reported them, the time, the deadline and the
  digest of the normalized arguments. Nothing the model supplies — an argument, a field
  it could shape — is authority on its own. One native call gets one binding. A ticket
  issued by the wrapper after it saw the call natively is how 1.x held this on Codex;
  [design-claude.md](../v2/design-claude.md#how-a-tool-call-runs) says how the mod holds it.
  Tests:
  - `internal/bridge/endpoint/tickets_test.go` `TestATicketNeedsTheHarnessesOwnObservation`
  - `internal/bridge/endpoint/tickets_test.go` `TestTheObservationMayComeEitherSideOfTheRequest`
  - `internal/bridge/endpoint/tickets_test.go` `TestOneCallGetsOneTicket`
  - `internal/bridge/endpoint/tickets_test.go` `TestConflictingCorrelationIsRefused`
  - `internal/bridge/endpoint/tickets_test.go` `TestAConfirmationIsOneTime`
  - `internal/bridge/endpoint/tickets_test.go` `TestTheDeadlineFollowsTheTransportsTimeout`
  - `internal/bridge/endpoint/tickets_test.go` `TestTheObservationWaitIsTwoSeconds`
  - `internal/bridge/endpoint/hello_test.go` `TestAHelloIsChecked`
  - `internal/bridge/endpoint/hello_test.go` `TestARoleAsksOnlyItsOwnQuestion`
  - `internal/bridge/endpoint/order_table_test.go` `TestEveryOrderOfTheTableIssuesOneTicketPerCall`
  - `internal/bridge/bridge_test.go` `TestATicketMissingItsScopeIsRefused`
  - `internal/bridge/bridge_test.go` `TestTheTicketIsReadFromItsDescriptor`
  - `internal/bridge/bridge_test.go` `TestAnOpenTicketDescriptorDoesNotHang`
  - `internal/bridge/bridge_test.go` `TestNoEndpointRefusesEveryTicket`
  - `internal/bridge/bridge_test.go` `TestTheDigestSeparatesWords`
  - `internal/cli/bridge_tool_test.go` `TestAToolCallWithoutAValidTicketRunsNothing`
  - Gap: a binding from the harness's own ids, checked per request on the exact harness process — closed in S7.

- **T3. Every effect happens in the core operation, under its receipt**; a transport
  never repeats, finishes or undoes one, and starts no second operation for a call
  whose first still runs.
  Tests:
  - `internal/receipt/receipt_test.go` `TestRacingCallsWithOneKeyJoinOneRecord`
  - `internal/receipt/receipt_test.go` `TestAnUnscopedKeyNeverJoins`
  - `internal/receipt/receipt_test.go` `TestTheKeyScopesByRunConversationAndTurn`
  - `internal/receipt/receipt_test.go` `TestATokenBelongsToItsRun`
  - `internal/receipt/receipt_test.go` `TestTheLockHoldsOneCallAtATime`
  - `internal/receipt/receipt_test.go` `TestUnresolvedFindsOpenOperations`
  - `internal/cli/journal_test.go` `TestAHeadsUpRepeatedInOneTurnIsPublishedOnce`
  - `internal/cli/journal_test.go` `TestAHeadsUpPastItsDeadlineIsFinishedByRetry`
  - `internal/cli/journal_test.go` `TestTheShellRefersAnOpenOperationToItsReceipt`
  - `internal/cli/journal_test.go` `TestAPendingMarkRepeatedInOneTurnIsOneMark`
  - `internal/cli/bridge_rules_test.go` `TestAnUncertainPublicationIsNotDiscarded`
  - `internal/cli/bridge_rules_test.go` `TestAnOpenOperationIsNotBypassedByAge`
  - `internal/cli/bridge_rules_test.go` `TestALateAcknowledgmentOwesNothingTwice`
  - `internal/cli/bridge_read_test.go` `TestARepeatedToolReadInOneTurnIsTheSameRead`
  - `internal/cli/bridge_read_test.go` `TestAnEmptyToolReadIsTheSameNothingInItsTurn`
  - `internal/bridge/server/order_test.go` `TestOneCallRunsOnceAcrossServers` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/fault_test.go` `TestEveryStepOfACallSurvivesAFault` — rebuilt in S7 on the neutral rig

- **T4. A call is bound to its operation before its first effect**: a record keyed by
  the native call, whose absence ("no such file") proves the call made no effect. Only
  on that proof may the answer send the same words to the shell.
  Tests:
  - `internal/cli/bridge_rules_test.go` `TestAnOpenOperationIsNotBypassedByAge`
  - `internal/cli/bridge_rules_test.go` `TestAToolRetryKeepsToTheToolSurface`
  - `internal/cli/bridge_rules_test.go` `TestAShellRetryDoesNotReadStdinAgain`
  - `internal/cli/bridge_read_test.go` `TestTheShellFinishesALostToolRead`
  - `internal/cli/journal_test.go` `TestTheShellRefersAnOpenOperationToItsReceipt`
  - `internal/bridge/server/child_bound_test.go` `TestAnOutlivedCallWithABindingNamesItsRetry` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/stdout_bound_test.go` `TestALostAnswerOfAnEffectIsFoundAgain` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/fault_test.go` `TestEveryStepOfACallSurvivesAFault` — rebuilt in S7 on the neutral rig
  - Gap: the shell running the same words on the proof of absence — closed in S7.

- **T5. Everything read and written is bounded, in one place each.** A request is
  bounded before it is parsed; every answer passes one encoder and one bound on its
  final bytes; an answer that does not fit is replaced whole, never clipped.
  Tests:
  - `internal/bridge/bridge_test.go` `TestOnlyAWholeResultIsEvidence`
  - `internal/bridge/bridge_test.go` `TestCutKeepsCharactersAndBounds`
  - `internal/cli/bridge_tool_test.go` `TestALongAnswerComesInParts`
  - `internal/cli/bridge_rules_test.go` `TestAContinuationRefreshesALetterBeforeAnyPart`
  - `internal/cli/bridge_rules_test.go` `TestAnEscapedDiagnosticFits`
  - `internal/cli/bridge_rules_test.go` `TestAnAnswerThatCannotBeKeptStillFits`
  - `internal/cli/bridge_rules_test.go` `TestEveryTextPartNamesItself`
  - `internal/bridge/endpoint/hello_test.go` `TestTheConnectionsAreBounded`
  - `internal/bridge/server/encoder_test.go` `TestTheEncoderReplacesAnAnswerThatDoesNotFit` — rebuilt in S7 on the endpoint's encoder
  - `internal/bridge/server/encoder_test.go` `TestAChildsOutputIsCapped` — rebuilt in S7 on the endpoint's encoder
  - `internal/bridge/server/encoder_test.go` `TestOnlyTheEncoderWritesToStdout` — rebuilt in S7 on the endpoint's encoder
  - `internal/bridge/server/bound_test.go` `FuzzEveryMessageWrittenFits` — rebuilt in S7 on the endpoint's encoder
  - `internal/bridge/server/bound_test.go` `FuzzFramesAreBounded` — rebuilt in S7 on the endpoint's encoder
  - `internal/bridge/server/stdout_bound_test.go` `TestAClientThatStopsReading` — rebuilt in S7 on the endpoint's encoder
  - `internal/bridge/server/stdout_bound_test.go` `TestAReaderThatResumesInTimeGetsEveryReply` — rebuilt in S7 on the endpoint's encoder
  - Gap: a request bounded before it is parsed at the endpoint — closed in S7.

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
  - `internal/bridge/bridge_test.go` `TestOnlyAWholeResultIsEvidence`
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
  - `internal/bridge/endpoint/events_test.go` `TestOnlyAUsedTicketWithItsBindingIsAcknowledged` — split in S7: its neutral oracle on the neutral input
  - `internal/bridge/endpoint/events_test.go` `TestTheExposureOfEachResultShape` — split in S7: its neutral oracle on the neutral input
  - `internal/bridge/server/flow_test.go` `TestAToolReadIsReadOnceItsAnswerArrived` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/flow_test.go` `TestALongReadGoesOnThroughItsNextWords` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/flow_test.go` `TestAFailedResultReadsNothing` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/flow_test.go` `TestAToolThatReadsOffLeavesTheLetterUnread` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/evidence_test.go` `TestOnlyTheRecordedAnswerItselfIsEvidence` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/evidence_test.go` `TestACompletionWhoseBindingCannotBeReadIsSpent` — rebuilt in S7 on the neutral rig
  - Gap: a reading tool whose transport cannot give the proof refuses before its first effect — closed in S7 (the fixture offers a transport without result evidence).

- **T7. A call's commits stop at its turn's end, and a pending mark speaks only for
  its own turn.** Two conditions, both needed:
  - **Not ended.** A pending mark or a read's acknowledgment checks, under the mailbox
    lock, for an end of the run on record at or after its time, or a turn start
    recorded after it, and in the host for ends it has heard and not yet recorded;
    found, nothing is committed and the letter shows again.
  - **Its own turn, proven.** A pending mark is written only by an attempt proven to
    run in the turn its operation was made in: the attempt that created the operation,
    or a later one whose binding names the same conversation and turn on a transport
    whose turn ids are proven never reused. No absence proves the turn still open —
    of a later turn's start, of an end. An attempt that cannot prove it does not mark:
    the outcome is **unproven**, the mark is finished as not made, and the answer says
    why (`cli/pending_turn.go:10-31,65`: `inOwnTurn`, `markUnproven`). This binds the
    shell, `retry` and every tool transport alike: a `retry` from a later turn never
    writes an earlier turn's mark. Until an adapter proves its turn ids are never
    reused — on Claude Code across an interrupt, a reload, `/clear` and resume (P3,
    P9) — only the first attempt counts there, as 1.x counts it for `prompt_id`.
  The carried test includes a lost end with a `retry` from another turn.
  Tests:
  - `internal/bridge/endpoint/gate_test.go` `TestAnAcknowledgmentAfterANotedEndWritesNothing`
  - `internal/bridge/endpoint/tickets_end_test.go` `TestACapturedCodexTurnIssuesNoMoreTickets`
  - `internal/bridge/endpoint/tickets_end_test.go` `TestARunPastTheTicketsItRemembersRefuses`
  - `internal/bridge/endpoint/tickets_end_test.go` `TestTheLastTicketARunRemembersIsIssuedOnce`
  - `internal/bridge/endpoint/tickets_end_test.go` `TestNoTicketWhileACaptureWaitsForAWriter`
  - `internal/cli/pending_test.go` `TestAPendingTurnEndKeepsTheTaskOwed`
  - `internal/cli/pending_test.go` `TestAMarkDoesNotSurviveAnInterruptedTurn`
  - `internal/cli/pending_test.go` `TestALatePublishedTurnIsAReportWithItsOwnText`
  - `internal/cli/pending_test.go` `TestAFailedOrStoppedTurnIgnoresTheMark`
  - `internal/cli/bridge_rules_test.go` `TestAPendingMarkIsNotMadeAfterItsDeadline`
  - `internal/cli/journal_test.go` `TestARefusedMarkIsReplayedInItsTurn`
  - `internal/bridge/server/order_call_test.go` `TestEveryOrderOfACallAgainstItsTurnsEnd` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/order_gen_test.go` `TestEveryOrderOfTwoTurnsKeepsTheEndsBoundary` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/order_marks_test.go` `TestAMarkBetweenCaptureAndJournal` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/flow_test.go` `TestAnAnswerAfterTheEndReadsNothing` — rebuilt in S7 on the neutral rig
  - Gap: no test names `markUnproven` (`cli/pending_turn.go:10-31,65`) — closed in S7.
  - Gap: a lost end with a `retry` from another turn: the answer "not marked", no mark written, the receipt finished as not made, the letter shown again — closed in S7.

- **T8. An end's boundary is a cut between commits**, taken at the end's own event —
  the clock when no acknowledgment is between its check and its close, or the snapshot
  the acknowledgment takes after its last write under the lock. Each acknowledgment is
  applied once.
  Tests:
  - `internal/bridge/endpoint/gate_test.go` `TestACaptureTakesTheClosingSnapshotOfTheWritingAcknowledgment`
  - `internal/bridge/endpoint/gate_test.go` `TestACaptureThatWaitedDoesNotSampleAgain`
  - `internal/bridge/endpoint/gate_test.go` `TestACaptureWithNothingWritingSamplesUnderTheMutex`
  - `internal/bridge/endpoint/gate_test.go` `TestOnlyOneAcknowledgmentWrites`
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
  - `internal/harness/fixture/order_test.go` `TestAnEndCapturesItsBoundaryWhenItsFrameArrives`
  - `internal/harness/fixture/order_test.go` `TestATurnStartCapturesItsBoundaryWhenItsFrameArrives`
  - `internal/bridge/server/order_test.go` `TestAnEndCapturedDuringAnAcknowledgmentIncludesIt` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/order_cut_test.go` `TestAnAcknowledgmentCutMidway` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/order_cut_test.go` `TestAnEscNobodyHeard` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/order_gen_test.go` `TestEveryOrderOfTwoTurnsKeepsTheEndsBoundary` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/order_marks_test.go` `TestAMarkBetweenCaptureAndJournal` — rebuilt in S7 on the neutral rig

- **T9. Every wait has a bound, and nothing waits holding what it waits for**; a wait
  that runs out leaves a defined outcome. A write begun under a held lock is not timed.
  Tests:
  - `internal/bridge/endpoint/tickets_test.go` `TestTheDeadlineFollowsTheTransportsTimeout`
  - `internal/bridge/endpoint/tickets_test.go` `TestTheObservationWaitIsTwoSeconds`
  - `internal/bridge/endpoint/gate_test.go` `TestOnlyOneAcknowledgmentWrites`
  - `internal/bridge/endpoint/tickets_end_test.go` `TestNoTicketWhileACaptureWaitsForAWriter`
  - `internal/receipt/receipt_test.go` `TestTheLockHoldsOneCallAtATime`
  - `internal/receipt/receipt_test.go` `TestTheSweepRespectsAHeldLock`
  - `internal/cli/bridge_rules_test.go` `TestAnExpiredReadShowsAndClaimsNothing`
  - `internal/cli/bridge_rules_test.go` `TestAPendingMarkIsNotMadeAfterItsDeadline`
  - `internal/bridge/bridge_test.go` `TestAnOpenTicketDescriptorDoesNotHang`
  - `internal/harness/fixture/order_test.go` `TestASilentProbeCostsOnlyItsCapability`
  - `internal/harness/fixture/order_test.go` `TestCloseWaitsForACallIntoTheHandler`
  - `internal/harness/fixture/order_test.go` `TestNothingReachesTheHandlerOnceClosed`
  - `internal/bridge/server/child_bound_test.go` `TestAChildPastItsDeadline` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/stdout_bound_test.go` `TestAClientThatStopsReading` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/stdout_bound_test.go` `TestAReaderThatResumesInTimeGetsEveryReply` — rebuilt in S7 on the neutral rig
  - `internal/bridge/server/mcp_test.go` `TestTheServerFinishesItsCallsAtTheEnd` — rebuilt in S7 on the neutral rig

- **T10. One build per room.** The wrapper, whatever runs the core for a call, and the
  CLI of every session of a room are one build — one artifact, by the hash of its
  content, not by its version line; every writer holds the room's build lease while it
  acts, and anything of another build is refused before it acts
  ([design-state.md](../v2/design-state.md#one-build-per-room)).
  Tests:
  - `internal/bridge/endpoint/hello_test.go` `TestAHelloIsChecked`
  - Gap: the build id and the room's lease — closed in S17.

- **T11. A failing tool never weakens the mail.** The shell takes the same words under
  the same receipts; the surface, the read boundary and the refusals are the CLI's.
  Tests:
  - `internal/cli/bridge_read_test.go` `TestTheShellFinishesALostToolRead`
  - `internal/cli/bridge_rules_test.go` `TestAShellRetryDoesNotReadStdinAgain`
  - `internal/cli/shell_channel_test.go` `TestEveryShellCommandLeavesItsEvidence`
  - `internal/cli/shell_channel_test.go` `TestTheShellEvidenceKeepsTheTimeOfItsWrite`
  - `internal/receipt/shell_test.go` `TestTakeShellOverEveryEntry`
  - `internal/receipt/shell_test.go` `TestTheBoundKeepsTheNewest`
  - `internal/receipt/shell_test.go` `TestEveryErrorHasItsClass`
  - `internal/receipt/shell_test.go` `TestAnUnwritableStateLosesTheObservation`
  - `internal/cli/journal_test.go` `TestTheShellRefersAnOpenOperationToItsReceipt`
  - `internal/bridge/bridge_test.go` `TestNoEndpointRefusesEveryTicket`
  - `internal/bridge/server/fault_test.go` `TestEveryStepOfACallSurvivesAFault` — rebuilt in S7: the wrapper faults on the neutral rig
  - Gap: one set of words once through the tool and once through the shell under one receipt — closed in S7.

## The carried order and fault tests

The carried fault and order tests (`bridge/server/fault_test.go`, `order_cut_test.go`,
`order_gen_test.go`, `bridge/endpoint/order_table_test.go`) hold T2–T8; their oracles
become transport-neutral and run against every transport
([design-docs-tests.md](../v2/design-docs-tests.md#tests)). Until S7 builds the neutral rig
([stage3-tests-tcl.md](../v2/stage3-tests-tcl.md#the-neutral-rig)), they run in
`bridge/server`'s rig, which leaves in S8.
