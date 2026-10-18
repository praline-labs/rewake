# Rules: the effects of a mail operation, E1–E8, and the operator decision, D1–D8

The rules every mail operation keeps — what proves an effect done or not done, and what
an unknown stops — in the full wording of [design-rules.md](../v2/design-rules.md#effects-e1e8),
from `mail-bridge-cli.md` rules 1–8 without the 1.x migration. Each rule ends with the
tests that hold it ([README.md](README.md)). The operator decision on an unknown
outcome, which the last clause of E8 names, follows them as D1–D8, built in S18
([stage3-decision-recovery.md](../archive/v2/stage3-decision-recovery.md#its-rules)): when
evidence cannot return, the subject's recipient run or the person decides, and the
decision settles the stop without being evidence.

## The rules

- **E1. An operation whose effect is unknown is never discarded and never bypassed.**
  A receipt goes only when its effect is proven absent or proven done; a step not
  recorded as done is no proof it was not done; age decides nothing — who continues an
  operation is decided by its scope or its receipt. (M1 rule 1.)
  Tests:
  - `internal/core/mail/settle_copy_test.go` `TestAFailedArchiveKeepsTheLastCopy`
  - `internal/core/mail/settle_copy_test.go` `TestSettlingADeliveredLetterKeepsItsOnlyCopy`
  - `internal/core/mail/settle_copy_test.go` `TestAnIntentWhoseLetterIsRefusedKeepsItsProof`
  - `internal/core/mail/settle_copy_test.go` `TestADirectoryAtTheArchiveIsNoCopy`
  - `internal/core/mail/settle_copy_test.go` `TestASymbolicLinkIsNoLandingProof`
  - `internal/core/mail/journal_unknown_test.go` `TestAReportFoundWithoutItsMarkIsMarkedPublished`
  - `internal/core/mail/journal_unknown_test.go` `TestAnUnknownPublicationMarkIsNeverPermission`
  - `internal/core/mail/claims_test.go` `TestAnIntentWithoutItsLetterIsWrittenAgain`
  - `internal/core/mail/claims_test.go` `TestTheSweepKeepsALetterWhoseIntentItCouldNotSettle`
  - `internal/core/mail/claims_test.go` `TestAMailboxThatCannotBeSearchedProvesNothing`
  - `internal/core/mail/journal_test.go` `TestAJournalDoesNotRepublishASweptReport`
  - `internal/core/mail/journal_test.go` `TestAReportWhoseEntryFailedIsNotRepublishedAfterTheSweep`
  - `internal/cli/journal_test.go` `TestTheShellRefersAnOpenOperationToItsReceipt`
  - `internal/core/receipt/receipt_test.go` `TestUnresolvedFindsOpenOperations`
  - `internal/core/receipt/receipt_test.go` `TestSweepKeepsWhatARecoveryReads`

- **E2. A lock is removed only by whoever holds it**, and a call that took a lock checks
  the file at the path is still its own. (M1 rule 2.)
  Tests:
  - `internal/core/receipt/receipt_test.go` `TestTheLockHoldsOneCallAtATime`
  - `internal/core/receipt/receipt_test.go` `TestTheSweepRespectsAHeldLock`
  - `internal/infra/state/held_test.go` `TestTryHoldWaitsForTheWriter`
  - `internal/infra/state/held_test.go` `TestTryHoldRefusesAFileRemovedBeforeItsLock`
  - `internal/infra/state/held_test.go` `TestTryHoldRefusesAFileReplacedBeforeItsLock`
  - `internal/infra/state/lock_test.go` `TestNameLockHoldsBetweenProcesses`
  - `internal/infra/state/state_test.go` `TestNameLockSerialisesClaims`
  - `internal/host/delivery/sweep_gate_test.go` `TestASweepWithoutTheLockRetiresNothing`

- **E3. Every check that decides an effect runs inside the critical section, right
  before the effect, and again on a retry**, which is a new call with its own deadline
  and transport and the same limits. (M1 rule 3.)
  Tests:
  - `test/decisionrig/deadline_test.go` `TestTheWrapperWritesNoDecisionPastTheCallsDeadline`
  - `internal/cli/addendum_test.go` `TestAnAddendumIsAskedAgainUnderTheLock`
  - `internal/cli/addendum_test.go` `TestAnAddendumsEditIsAskedAgainUnderTheLock`
  - `internal/core/mail/claims_test.go` `TestPublicationOfReadsTheMailbox`
  - `internal/core/mail/claims_test.go` `TestPublishOnceWritesNothingWhenItsCheckFails`
  - `internal/host/delivery/batch_edges_test.go` `TestGroupedPreparationRechecksAnswerLeasesExpiryAndEpoch`
  - `internal/host/delivery/withdraw_test.go` `TestTheLastCheckBeforeANoticeSeesAWithdrawal`
  - `internal/host/delivery/window_test.go` `TestARetryDoesNotRestartTheWindow`
  - `internal/adapter/fixture/turns_test.go` `TestATurnEndIsRetriedWithItsOwnDeadline`
  - `internal/adapter/fixture/turns_test.go` `TestATurnEndThatNeverTakesIsRefusedAfterItsAttempts`
  - `internal/adapter/fixture/order_test.go` `TestAWithdrawalStopsAnEndsAttempts`
  - `internal/adapter/fixture/order_test.go` `TestAClosedConnectionTakesNoTurn`
  - `internal/core/mail/publication_race_test.go` `TestTheSweepWaitsForAnAttemptsEvidence`
  - `internal/core/mail/publication_race_test.go` `TestAdmissionIsInsideTheSection`
  - `internal/cli/publication_race_test.go` `TestTheSweepWaitsForAHeadsUpsEvidence`
  - `internal/cli/publication_race_test.go` `TestAHeadsUpsAdmissionIsInsideTheSection`
  - `internal/core/mail/publication_bounds_test.go` `TestAFirstLandingAcrossRunEndIsWrittenOnce`
  - `internal/core/mail/publication_bounds_test.go` `TestABusyRecipientFailsTheTurnWithoutAStop`
  - `internal/core/mail/publication_bounds_test.go` `TestTwoMailboxesPublishingToEachOtherDoNotDeadlock`
  - `internal/core/mail/publication_bounds_test.go` `TestAReportToItselfIsPublishedInsideItsOwnLock`
  - `internal/core/mail/publication_bounds_test.go` `TestAPublicationReadsTheRunWithoutCleaningUp`
  - `internal/host/delivery/settle_listing_test.go` `TestSettlingWaitsForALockTheServerHolds`
  - `internal/host/delivery/settle_lock_test.go` `TestADeferredSettlementGetsAnotherPass`
  - `internal/host/delivery/settle_recorded_test.go` `TestARestartSettlesALateRefusalLeftUnread`
  - `internal/host/delivery/settle_recorded_test.go` `TestASettlingCutShortIsSettledOnceItCan`
  - `internal/host/delivery/settle_recorded_test.go` `TestALateRefusalItsStatusMissedIsRecordedThenSettled`
  - `internal/host/delivery/settle_recorded_test.go` `TestALetterReadableOnPurposeStaysUnread`
  - `internal/host/delivery/settle_recheck_test.go` `TestAForeignLetterSettlesByItsStatusUnderTheLock`
  - `internal/host/delivery/settle_recheck_test.go` `TestARecordedRefusalIsRecheckedUnderTheLock`
  - `internal/host/delivery/settle_recheck_test.go` `TestARefusalOnRecordAStopKeepsWaitsTheRetryInterval`

- **E4. A read's completion is one durable fact every channel shares.** A letter leaves
  `unread/` only by being read once a part of it was shown, and never comes back; a late
  acknowledgment finds it read and marks nothing. (M1 rule 4.)
  Tests:
  - `internal/core/mail/claims_test.go` `TestALetterBeingReadInPartsCannotBeTakenBack`
  - `internal/core/mail/claims_test.go` `TestTheSweepKeepsALetterBeingRead`
  - `internal/core/mail/claims_test.go` `TestALetterIsPublishedOnceAcrossReadAndSweep`
  - `internal/host/delivery/claims_test.go` `TestTheServerKeepsALetterBeingRead`
  - `internal/core/mail/unread_test.go` `TestReadingHandsEachMessageOnce`
  - `internal/host/delivery/unread_test.go` `TestAReadDuringAPendingNoticeStands`
  - `internal/host/delivery/unread_test.go` `TestAReadIsNotUndoneByTheNoticeResult`
  - `internal/cli/read_test.go` `TestSendCountsAReadMessageAsDelivered`
  - `internal/cli/read_test.go` `TestTwoReadersAtOnceShowATaskOnce`

- **E5. The size bound holds on the final encoded bytes of every answer, in one place**;
  every text part names its letter, part, count and byte range. (M1 rule 5.) Since
  October 18, 2026 no answer is cut: the bound and its parts were the mail tool's, and the
  shell prints a read whole.

- **E6. A check has three outcomes: found, proven absent, unknown.** "No such file"
  proves absence. When recording a stop's original observation, Lstat finding a
  component that is not a directory (ENOTDIR) also proves that exact path absent at that
  time. Every other lookup error leaves its presence unknown. A later ENOTDIR is not
  evidence that resolves an occurrence; nor does a later absence resolve a path whose
  original presence was found or unknown. An unknown stops the effect and leaves the
  operation where a retry takes it up. Readers that only inform fold an unknown into
  their most cautious answer. (M1 rule 6.)
  Tests:
  - `internal/core/mail/lookups_test.go` `TestALetterThatCannotBeReadIsNotLeftOut`
  - `internal/core/mail/lookups_test.go` `TestAStatusIsFoundAbsentOrUnknown`
  - `internal/core/mail/lookups_test.go` `TestAnsweredNeedsAMailboxItCanSearch`
  - `internal/core/mail/lookups_test.go` `TestAWaiterThatCannotBeReadIsNotWrittenOver`
  - `internal/core/mail/lookups_test.go` `TestAReadStopsOnAStatusItCannotRead`
  - `internal/core/mail/lookups_test.go` `TestAWithdrawalThatCannotLookStopsBeforeWriting`
  - `internal/core/mail/lookups_test.go` `TestAClaimedLetterThatCannotBeReadIsNotOwedByNobody`
  - `internal/core/mail/lookups_test.go` `TestATurnRecordThatCannotBeReadIsNotNone`
  - `internal/core/mail/lookups_test.go` `TestTheSweepKeepsWhatAnUnreadableWaiterMayOwe`
  - `internal/core/mail/lookups_test.go` `TestNothingButTestsCallsTheLossyWaiters`
  - `internal/core/mail/lookups_test.go` `TestADamagedWaitRecordIsNotReadAsZeros`
  - `internal/core/mail/unknown_test.go` `TestAClaimThatCannotBeReadHoldsTheLetter`
  - `internal/core/mail/unknown_test.go` `TestAStatusThatCannotBeReadStopsTheLookup`
  - `internal/core/mail/unknown_test.go` `TestAMarkOrAWaiterThatCannotBeReadIsNotNone`
  - `internal/core/mail/records_test.go` `TestAFileOfNoKnownKindStopsTheMailbox`
  - `internal/core/mail/records_test.go` `TestANonRegularEntryAtEveryRecordPathStops`
  - `internal/core/mail/records_test.go` `TestUnlistedRecordsNamesAStrayDirectory`
  - `internal/core/mail/journal_unknown_test.go` `TestAnUnreadableInterimStopsTheJournal`
  - `internal/core/mail/confirm_test.go` `TestAnUnreadableKeptAnswerIsNotWrittenOver`
  - `internal/core/mail/settle_copy_test.go` `TestAPublishedMarkDoesNotOutweighAStageThatCannotBeSearched`
  - `internal/cli/awaited_unknown_test.go` `TestAwaitedTakesAnUnreadableRecipientForLive`
  - `internal/cli/list_unknown_test.go` `TestListShowsAnUnreadableSessionAsUnknown`
  - `internal/core/mail/stop_unknown_test.go` `TestANotADirectoryOnTheWayRecordsThePathAbsent`

- **E7. Every effect has an immutable identity and a proven scope, and recovery advances
  it only from durable evidence tied to that identity.** (M1 rule 7, clauses identity,
  clock, journal and scope.)
  - A turn end is one operation — publish its reports, take the kept answer they carry,
    clear the waits they answer, record whether the end was interim — named by its run
    and its event, never by an attempt; everything it writes is named from that.
  - Its scope is fixed once, before its first effect, from evidence the event carries.
    The read boundary is a position on the run's read clock, which numbers every read
    and every hold; a hold reserves its position, keeps the answer, then commits the
    clock, and a position is never issued twice. A held end confirmed again after a
    crash before the commit raises the clock to the position its kept answer records,
    allocating none, so its continuation's boundary takes the answer (S5).
  - The turn's window on the boot clock picks the pending marks that make it interim,
    opening after the turn's start or the latest earlier end a journal records,
    whichever is later; no end removes a pending mark, which lives as long as its run.
  - Every 2.0 TurnBoundary names its event (a turn id), so the 1.x case of an end heard
    once with no event goes; an end that names an event and carries no boundary still
    takes no effect on any attempt, and its waits stay owed.
  - What the operation will do is written down first (the journal), and nothing but the
    journal publishes. A stop after an interruption goes on taking its held answer (the
    owner, September 30, 2026).
  - An adapter whose ends come over a protocol and are retried — Claude Code's mod, from
    S4-6 — hands the core every copy of one end as its first capture: the event, the read
    boundary and both times fixed on its first arrival, kept when the core's attempt fails;
    a copy whose content differs from the first is refused and hands the core nothing. A copy
    is its end id and content, whichever admitted load of the run sends it, so an end stored
    by a load that was reloaded or never admitted is sent and taken under a later one. An end
    whose harness gives no start time has a window opening at the latest earlier end a
    journal records, so a pending mark of a turn whose end never reached the core falls into
    the next end's window ([stage4-turns.md](../v2/stage4-turns.md#the-start-time)). The core
    answers a receipt only for an end it took: a run whose name is proven to have passed on
    gets no receipt and its end is not sent again, and one whose record could not be read is
    sent again until it can ([stage4-turns.md](../v2/stage4-turns.md#the-hosts-side)).
  Tests:
  - `internal/adapter/claude/modhost/turns_test.go` `TestARetriedEndHandsTheCoreItsFirstCapture`
  - `internal/adapter/claude/modhost/turns_test.go` `TestTwoCopiesOfAnEndAtOnceAreCapturedOnce`
  - `internal/adapter/claude/modhost/turns_test.go` `TestAKnownEndWithOtherContentIsRefused`
  - `internal/adapter/claude/modhost/turns_test.go` `TestAnEndSentByALaterLoadIsTheSameEnd`
  - `internal/adapter/claude/mod_test.go` `TestTheModsTestsPass`
  - `internal/adapter/claude/modhost/turns_test.go` `TestAnEndOfARunGoneIsAnswered410WithNoReceipt`
  - `internal/adapter/claude/turns_test.go` `TestARunsEndReachesTheCoreThroughItsMod`
  - `internal/core/mail/pending_test.go` `TestTheWindowOpensAfterTheRunsLatestEarlierEnd`
  - `internal/core/registry/ownership_test.go` `TestOwnershipIsReadAsOwnedProvenNotOwnedOrUnknown`
  - `internal/cli/completion_owner_test.go` `TestAnEndOfARunWhoseNamePassedOnIsGoneAndTakesNothing`
  - `internal/cli/completion_owner_test.go` `TestAnEndWhoseRecordCannotBeReadIsRetriedAndPublishedOnce`
  - `internal/cli/completion_owner_test.go` `TestAnEndTakenWhoseReceiptWasLostIsAnsweredFromTheJournal`
  - `internal/cli/turn_journal_retry_test.go` `TestALateRetryAfterTheSweepPublishesNothing`
  - `internal/cli/turn_journal_retry_test.go` `TestARetryAnswersOnlyWhatItsFirstAttemptSaw`
  - `internal/cli/turn_test.go` `TestARetriedReadReportsOnce`
  - `internal/cli/stopped_error_retry_test.go` `TestStoppedAndErrorRetriesSettleOriginalScopeOnce`
  - `internal/cli/turn_retry_test.go` `TestRetriedTurnDoesNotConsumeLaterWork`
  - `internal/cli/turn_retry_test.go` `TestPartialTurnRetryKeepsOriginalOutcome`
  - `internal/cli/turn_scope_test.go` `TestARetryWhoseFirstAttemptRecordedNothingKeepsItsScope`
  - `internal/cli/turn_scope_test.go` `TestARetryTakesOnlyTheAnswerItsFirstAttemptSaw`
  - `internal/cli/turn_scope_test.go` `TestAnEndWithNothingOwedKeepsItsScopeOnRetry`
  - `internal/cli/turn_journal_test.go` `TestARecoveredEndTakesTheAnswerItPublished`
  - `internal/cli/turn_journal_test.go` `TestALateRetryKeepsALaterAnswer`
  - `internal/core/mail/journal_test.go` `TestAJournalTakesOnlyTheAnswerItCarried`
  - `internal/core/mail/read_boundary_test.go` `TestReadBoundaryExcludesLaterReadsAndIncludesSameTurnSteering`
  - `internal/core/mail/read_boundary_test.go` `TestReadBoundaryCaptureNeverWaitsForMailbox`
  - `internal/core/mail/read_boundary_test.go` `TestReadBoundaryHighWatermarkPreventsReuseAfterCounterLoss`
  - `internal/core/mail/read_boundary_test.go` `TestReadClockProcessWriter`
  - `internal/core/mail/read_boundary_test.go` `TestReadBoundaryIsSharedWithAnotherReaderProcess`
  - `internal/core/mail/confirm_test.go` `TestAKeptAnswerBelongsToItsRun`
  - `internal/core/mail/confirm_test.go` `TestAKeptAnswerIsTakenOnlyAtOrBelowTheBoundary`
  - `internal/core/mail/pending_test.go` `TestAMarkIsNeverTakenAway`
  - `internal/core/mail/pending_test.go` `TestTheWindowOpensAfterItsStartAndClosesAtItsEnd`
  - `internal/core/mail/pending_test.go` `TestAMarkOfAnotherRunIsIgnoredAndSwept`
  - `internal/core/mail/pending_test.go` `TestTheLatestMarkInATurnWins`
  - `internal/core/mail/pending_test.go` `TestAMarkIsWrittenOncePerCall`
  - `internal/core/mail/pending_test.go` `TestAnUnreadableMarkInTheWindowIsAnError`
  - `internal/core/mail/pending_test.go` `TestTheWindowOpensAfterTheRunsLatestEarlierEnd`
  - `internal/cli/pending_test.go` `TestAPendingTurnEndKeepsTheTaskOwed`
  - `internal/cli/pending_test.go` `TestAMarkDoesNotSurviveAnInterruptedTurn`
  - `internal/cli/pending_test.go` `TestAPendingTurnEndWithoutTextIsItsMark`
  - `internal/cli/pending_test.go` `TestAnUnmarkedTurnEndStillReports`
  - `internal/cli/pending_test.go` `TestAFailedOrStoppedTurnIgnoresTheMark`
  - `internal/cli/turn_journal_test.go` `TestAPendingEndWhoseJournalFailedStaysPending`
  - `internal/cli/turn_journal_retry_test.go` `TestARecoveredPendingEndKeepsItsInterimOnRecord`
  - `internal/cli/turn_journal_test.go` `TestATurnEndThatCannotRecordItsSequencePublishesNothing`
  - `internal/cli/turn_journal_test.go` `TestAnUnpublishedReportIsCompletedByTheNextTurnEnd`
  - `internal/cli/turn_journal_test.go` `TestAnAdoptedWaitWhoseReportIsOutIsNotAnsweredAgain`
  - `internal/cli/turn_journal_test.go` `TestAnEndWhoseJournalFailedIsPreparedAgainOnRetry`
  - `internal/cli/turn_journal_test.go` `TestAnEndRecordedBeforeItsEffectsIsNotPreparedAgain`
  - `internal/core/mail/journal_test.go` `TestAnAbandonedJournalWriteIsNotAJournal`
  - `internal/core/mail/held_end_test.go` `TestAHeldEndRaisesTheClockToItsPosition`
  - `internal/cli/turn_confirm_repeat_test.go` `TestAHoldCutByACrashIsHeldOnceWhenConfirmedAgain`
  - `internal/cli/turn_test.go` `TestTwoTurnEndsAtOnceReportOnce` — a boundary end handed to the core since S9
  - `internal/cli/neutral_end_test.go` `TestNeutralEndsWithDistinctIDsReportTwice`
  - `internal/cli/neutral_end_test.go` `TestNeutralStoppedThenFinishedEndsOfOneTurnReportTwice`
  - `internal/cli/neutral_end_test.go` `TestNeutralTwoEndsAtOnceReportOnce`
  - `internal/cli/neutral_end_test.go` `TestNeutralEndsWithoutTheirScopeReportNothing`
  - `internal/adapter/fixture/turns_test.go` `TestAnEndSentAgainIsTheSameCompletion`
  - `internal/core/mail/kept_clock_test.go` `TestAHoldReservesItsPositionBeforeItCommits`
  - `internal/core/mail/kept_clock_test.go` `TestAFailedHoldLeavesAGapAndNoPositionIsIssuedTwice`
  - `internal/cli/turn_end_api_test.go` `TestATurnEndWithoutItsEventOrAKnownReasonIsRefused`
  - `internal/host/turn_boundary_test.go` `TestTheWrapperPassesEachEndOnWithItsEventAndReason`

- **E8. Each effect is proven done, proven not done, or unknown; an unknown stops every
  change to its mailbox until evidence settles it.** (M1 rule 8, clauses evidence,
  stop, proof, retention and reconcile.)
  - Done is proven only by evidence written after the effect under the operation's
    name: the journal's entry, the recipient's `published` mark, the letter in the
    recipient's mailbox, a completed journal. Not done is proven only where the protocol
    makes absence visible: a report with no mark, an `intent` mark and no letter, a kept
    answer or interim record still in place. Everything else is unknown.
  - An unknown stops the mailbox: records kept, nothing published, cleared, adopted or
    read; the stop is a record (`stopped`) naming its exact cause and path. Letters from
    others still arrive, recovery writes its own records, main is told once.
  - What a stop is decided by is read before any effect: the barrier and every changing
    call walk the mailbox against one list of record kinds, then run their effects as a
    plan that writes nothing, through the one file seam both passes use; only a plan that
    meets no unknown lets the same code run for real.
  - A stop the plan found goes once the plan finds its cause gone; one only an effect
    met goes once a barrier has run every effect through.
  - The barrier runs under the mailbox lock before a turn end reads a wait: plan; then
    complete every unfinished journal in turn — each report published, or recorded moot
    when its recipient's run has ended or was replaced, and saved before the next; then
    the steps the journal named when it was written: the kept answer it takes, the waits
    it clears, its interim record. Journal operations never overlap — an end reads only
    the waits the barrier left, after the barrier has completed every journal — so no
    report answers what another journal's report closed, and no report is decided
    superseded. (The 1.x steps 2, 3 and 5 — conversion, successor, held — go, and with
    the conversion its collection of closed obligations.)
  - Proof lives as long as anything could replay it; a journal is kept while its run may
    retry it, an unfinished one for good, and nothing that names no run is swept by age.
  - **A stop is lifted only by returning evidence, never by removing its cause.**
    Removing a record whose effect is unknown does not make the effect absent: the next
    plan would read "no such file", stop seeing the cause, and publish without the
    evidence E1 asks for. So the stop and every record it names stay until evidence
    returns — in the ordinary case by fixing what made the record unreadable (its mode,
    its directory, the disk). No command goes around the barrier; the operator decision
    below neither removes a record nor counts as evidence.
  Tests:
  - `internal/core/mail/journal_unknown_test.go` `TestAReportFoundWithoutItsMarkIsMarkedPublished`
  - `internal/core/mail/journal_unknown_test.go` `TestAnUnknownPublicationMarkIsNeverPermission`
  - `internal/core/mail/claims_test.go` `TestALetterIsPublishedOnceAcrossReadAndSweep`
  - `internal/core/mail/claims_test.go` `TestAnIntentWithoutItsLetterIsWrittenAgain`
  - `internal/core/mail/journal_test.go` `TestAJournalDoesNotRepublishASweptReport`
  - `internal/core/mail/journal_test.go` `TestAReportWhoseEntryFailedIsNotRepublishedAfterTheSweep`
  - `internal/core/mail/stop_writes_test.go` `TestAGateWhoseStopCannotBeWrittenStillStops`
  - `internal/core/mail/stop_writes_test.go` `TestAFailedResolutionReachesTheCaller`
  - `internal/core/mail/answer_stop_test.go` `TestAStoppedMailboxDoesNotTakeAnAnswer`
  - `internal/cli/stop_gate_test.go` `TestAnUnreadableJournalStopsEveryCall`
  - `internal/cli/stop_gate_test.go` `TestUnknownEvidenceStopsTheCallsAfterTheEnd`
  - `internal/core/mail/plan_faults_test.go` `TestPlanFaultSmoke`
  - `internal/core/mail/journal_test.go` `TestADoneJournalIsKeptWhileItsRunLives`
  - `internal/cli/turn_journal_retry_test.go` `TestALateRetryAfterTheSweepPublishesNothing`
  - `internal/core/receipt/receipt_test.go` `TestSweepKeepsWhatARecoveryReads`
  - `internal/core/mail/sweep_test.go` `TestAnOwedMessageOutlivesTheSweep`
  - `internal/core/mail/sweep_test.go` `TestAnOwedMessageLeftUnreadOutlivesTheSweep`
  - `internal/core/mail/evidence_test.go` `TestTheEvidenceOfEveryEffectIsReadBeforeTheFirst` — rebuilt in S2 on journal records only
  - `internal/core/mail/effect_stop_test.go` `TestAnUnknownOnlyAnEffectMetStopsUntilTheBarrier` — rebuilt in S2 on journal records only
  - `internal/core/mail/effect_stop_test.go` `TestAStopThatCouldNotBeRecordedNamesEveryFailedWrite` — rebuilt in S2 on journal records only
  - `internal/core/mail/late_unknown_test.go` `TestEveryDurableStopOutlivesAFailedRetry` — rebuilt in S2 on journal records only
  - `internal/core/mail/late_unknown_test.go` `TestEveryEvidencePathIsReadBeforeTheFirstEffect` — rebuilt in S2 on journal records only
  - `internal/core/mail/journal_moot_test.go` `TestAReportForAnEndedRunIsMoot` — rebuilt in S2 on journal records only
  - `internal/core/mail/journal_moot_test.go` `TestAMootReportClearsWhatItAnswered` — rebuilt in S2 on journal records only
  - `internal/core/mail/reconcile_stop_test.go` `TestAnUnreadableJournalStopsEveryEffect`
  - `internal/core/mail/reconcile_stop_test.go` `TestAnUnreadableRecordStopsTheMailbox`
  - `internal/core/mail/stop_occurrence_test.go` `TestTwoCausesOneResolvedTheStopHolds`
  - `internal/core/mail/stop_occurrence_test.go` `TestARecipientsEndLeavesAHeldReportUnsettled`
  - `internal/core/mail/stop_occurrence_test.go` `TestMainIsToldOnceAcrossBarriersAndCalls`
  - `internal/core/mail/stop_occurrence_test.go` `TestLettersArriveIntoAStoppedMailbox`
  - `internal/core/mail/stop_sweep_test.go` `TestASweepBesideAStopRemovesNoNamedPath`
  - `internal/core/mail/sweep_norun_test.go` `TestNothingThatNamesNoRunIsSweptByAge`
  - `internal/core/mail/publication_race_test.go` `TestASavedPublishedStays`
  - `internal/core/mail/publication_race_test.go` `TestACrashBeforeTheSaveLeavesTheReportMoot`
  - `internal/host/delivery/sweep_gate_test.go` `TestAStaleSweeperRetiresNothing`
  - `internal/core/mail/publication_crash_test.go` `TestAKillAtAnyWriteLandsTheReportAtMostOnce`
  - `internal/core/mail/stop_occurrence_test.go` `TestARecurringCauseOfARecordIsANewOccurrence`
  - `internal/core/mail/stop_occurrence_test.go` `TestARecurringCauseAboutAnOperationIsANewOccurrence`
  - `internal/core/mail/stop_unknown_test.go` `TestAResolutionIsReadBeforeItLiftsTheStop`
  - `internal/core/mail/stop_unknown_test.go` `TestAnOccurrenceRecordsAPathsPresenceUnknownApartFromAbsent`
  - `internal/core/mail/stop_unknown_test.go` `TestAProofWhosePresenceWasUnknownResolvesNothingOnceGone`
  - `internal/core/mail/stop_unknown_test.go` `TestReleasingAnAnswerKeepsAMarkTheStopNames`
  - `internal/core/mail/stop_unknown_test.go` `TestSettlingKeepsALetterTheStopNames`
  - `internal/core/mail/stop_unknown_test.go` `TestAnEffectsOccurrenceWhoseResolutionDoesNotReadHoldsTheEffects`
  - `internal/core/mail/stop_ended_evidence_test.go` `TestAHeldReportOfAnEndedRunIsSettledByItsEvidence`
  - `internal/core/mail/stop_ended_evidence_test.go` `TestEndedEvidenceReadsOnlyProof`
  - `internal/core/mail/stop_settled_test.go` `TestAResolvedOccurrenceKeepsItsReportsDisposition`
  - `internal/core/mail/stop_settled_test.go` `TestAReportNeverHeldIsMootOnceItsRunEnded`
  - `internal/core/mail/stop_settled_test.go` `TestAResolvedRecordThatDoesNotReadStopsTheBarrier`
  - `internal/core/mail/decision_view_test.go` `TestDecisionDisposesOnlyTheOccurrencesItNames`
  - `internal/core/mail/decision_listing_test.go` `TestDecisionListingNamesEveryOccurrenceTheDecisionDoesNotDispose`
  - `internal/cli/decide_undisposed_test.go` `TestEveryStopViewNamesAnOccurrenceTheDecisionDoesNotDispose`
  - `internal/core/mail/decision_recovery_test.go` `TestDecisionPrecedesAllRecoveryEvidence`
  - `internal/core/mail/decision_world_test.go` `TestDecisionCopiesFollowWorldTruth`

## How E8 lifts a stop

E8 says a stop the plan found goes once the plan finds its cause gone, and that a stop is
lifted only by returning evidence, never by removing its cause. Read literally, the first
lets the removal of an unreadable record lift a stop, which the second forbids. Stage 3
reads them together as correction 2 of [stage3.md](../archive/v2/stage3.md#corrections-to-the-accepted-design),
agreed in review: a stop is one write-once record per occurrence of a cause; an
occurrence is resolved only by evidence about its effect — the effect proven done or proven
not done — never by its path becoming absent or merely readable; an effect-only cause is
resolved the same way, once a barrier has run its effect through to evidence, not by the
barrier having run; one resolution never resolves another, and a cause that recurs after
its resolution is a new occurrence. S3 built it
([stage3-steps.md](../archive/v2/stage3-steps.md#s3-a-stop-is-resolved-per-cause-by-evidence-codex)),
and inverted the tests above that lifted a stop by a removed cause; each keeps a half on
valid bytes closed to reading, where opening them again resolves the cause. The wording
of E8 above is the accepted one: the single record it names, `stopped`, is from S3 one
record per occurrence, `stops/<key>/<occurrence>`, with its resolution beside it
([mailbox-records.md](../mailbox-records.md#the-stop-on-record)).

## The operator decision, D1–D8

When evidence cannot return, the sender's mailbox stays stopped until the subject's
recipient run or the person decides whether the report arrived. The rules are worded in
[stage3-decision-recovery.md](../archive/v2/stage3-decision-recovery.md#its-rules), from the model
of [stage3-decision-model.md](../archive/v2/stage3-decision-model.md); the command that makes a
decision is `rewake decide` ([stage3-decision.md](../v2/stage3-decision.md#the-command)).
A decision is no evidence and removes no record: E8 holds beside it, and these rules say
what a decision may settle instead.

- **D1. A decision is a choice under an unknown outcome, never evidence: nothing derives
  delivery from it, and every effect taken on its authority carries its id.** Ordinary
  activity in the recipient's mailbox follows its own owners and needs no knowledge of it.
  Tests:
  - `internal/core/mail/decision_world_test.go` `TestDecisionCopiesFollowWorldTruth`
  - `internal/core/mail/decision_identity_test.go` `TestDecisionStepsStandOnTheirDecision`
  - `internal/cli/decide_views_test.go` `TestDecidedArrivedClosesTheWait`
  - `internal/cli/decide_views_test.go` `TestAWaitingQuestionEndsWhenADecisionClosesIt`
  - `internal/core/mail/decision_final_lock_test.go` `TestDecisionFinalEvidenceIsLockedThroughCommit`
  - `internal/host/delivery/decision_race_test.go` `TestDecisionRacesWithWrapperAndDelivery`

- **D2. Only the subject's recipient run, vouched for by its wrapper — the same run after
  a compaction — or the person, once every identity check has read and found no session,
  decides; the record names who and whether verified; the recipient run and main are told
  of a decision they did not make.**
  Tests:
  - `test/decisionrig/authority_test.go` `TestDecisionAuthorityWithProcesses`
  - `test/decisionrig/boundary_test.go` `TestDecisionAuthorityEndsAtCommitBoundary`
  - `internal/core/person/person_test.go` `TestPersonDecisionFailsClosed`
  - `test/decisionrig/person_test.go` `TestPersonDecisionOnRealProcesses`
  - `internal/cli/decide_test.go` `TestDecideBelowTheRecipientAsksItsWrapper`
  - `internal/cli/decide_test.go` `TestDecideRefusesWhoMayNot`
  - `internal/core/authority/decide_test.go` `TestAWrapperTakesADecisionOnlyFromBelow`
  - `internal/core/authority/decide_test.go` `TestTheAskerBelievesOnlyTheRecipientsWrapper`
  - `internal/cli/decide_form_test.go` `TestDecisionFormAccess`
  - `internal/core/mail/decision_decide_notes_test.go` `TestDecideFreezesANoteToTheRoomsMain`
  - `internal/core/mail/decision_decide_notes_test.go` `TestDecideRefusesWhileTheRoomsMainCannotBeJudged`
  - `internal/core/mail/decision_liveness_test.go` `TestDecisionNoteToARunNotKnownToHaveEndedStaysOwed`

- **D3. A decision needs a subject read from a readable journal and decides that subject
  only.** Its write-once record, kept while the mailbox lives, holds the subject's
  identity and the observations its final plan used — bytes and their hashes for every
  read that returned, the error and metadata for every read that failed — and nothing of
  what could not be read.
  Tests:
  - `internal/core/mail/decision_subjects_test.go` `TestDecisionFindsSubjectsBehindOtherUnknowns`
  - `internal/core/mail/decision_view_test.go` `TestDecisionDisposesOnlyTheOccurrencesItNames`
  - `internal/core/mail/decision_capture_test.go` `TestDecisionObservationsAreTheFinalReads`
  - `internal/core/mail/decision_strict_test.go` `TestDecisionObservationsShareNothingWithTheCapture`
  - `internal/core/mail/decision_strict_test.go` `TestDecisionRecordsRefuseBytesAfterTheirValue`
  - `internal/core/mail/decision_decide_run_test.go` `TestDecideHoldsTheReportToTheJournalsRun`
  - `internal/core/mail/decision_special_test.go` `TestDecisionSpecialFilesAuthorizeNothing`
  - `internal/core/mail/decision_matrix_links_test.go` `TestDecisionWorldsBehindLinks`

- **D4. A decision is written once, under the sender's mailbox lock, after the plan finds
  the subject still unknown there, before any effect, with the whole resend in it; the
  same words answer it, the opposite words are refused.**
  Tests:
  - `internal/core/mail/decision_decide_test.go` `TestDecideRecordsOneDecisionAndFinishesIt`
  - `internal/core/mail/decision_decide_test.go` `TestDecideAgainKeepsTheFirstDecision`
  - `internal/core/mail/decision_decide_test.go` `TestDecideRefusesWithNothingWritten`
  - `internal/core/mail/decision_decide_test.go` `TestDecideOnABusyMailboxWritesNothing`
  - `internal/core/mail/decision_decide_ended_test.go` `TestDecideOnAHeldReportOfAnEndedRunReadsItsEvidence`
  - `internal/core/mail/decision_final_agree_test.go` `TestDecisionFinalReadAgreesWithTheBarrier`
  - `internal/core/mail/decision_final_lock_test.go` `TestDecisionFinalEvidenceIsLockedThroughCommit`
  - `internal/cli/decide_lease_test.go` `TestDecisionLeasePrecedesAllLocks`
  - `internal/core/mail/decision_durable_test.go` `TestDecisionEffectsWaitForTheDecisionToBeDurable`
  - `internal/core/mail/decision_matrix_test.go` `TestDecisionDurabilityAtEveryCut`
  - `internal/infra/state/points_test.go` `TestAWriteIsToldEachPointBeforeItIsReached`
  - `internal/host/delivery/decision_race_test.go` `TestDecisionRacesWithWrapperAndDelivery`

- **D5. "Arrived" closes the report's step; "did not arrive" closes it and installs the
  resend.** The sender's recovery reads the decision before the report's live evidence,
  never publishes the report after either, and never changes the decision or cancels the
  resend on later proof. The journal's tail runs, as any tail does, in the first barrier
  whose plan meets no unknown in the mailbox.
  Tests:
  - `internal/core/mail/decision_recovery_test.go` `TestDecisionPrecedesAllRecoveryEvidence`
  - `internal/core/mail/decision_recovery_test.go` `TestDecisionInstallationBeforeAdmission`
  - `internal/core/mail/decision_recovery_test.go` `TestDecisionRecoveryRecordsStopWhenUnknown`
  - `internal/core/mail/decision_closure_test.go` `TestDecisionClosureKeepsProvenance`
  - `internal/core/mail/decision_identity_test.go` `TestDecisionClosureNamesItsOperationAndItsTail`

- **D6. A resend is an ordinary report operation with a fresh, bounded id and its ancestry
  in fields: its dispositions are E8's, and when unknown it is a subject decided with the
  first decision as parent.** Moot says only that an attempt found its run ended before
  another disposition was recorded, never that it did not land; its done form keeps a
  bounded summary of its disposition and decisions, and nothing renders more than a form
  keeps.
  Tests:
  - `internal/core/mail/decision_resend_contract_test.go` `TestDecisionResendKeepsPublicationContract`
  - `internal/core/mail/decision_world_test.go` `TestDecisionCopiesFollowWorldTruth`
  - `internal/core/mail/decision_resend_test.go` `TestDecisionResendOfAnotherKindStaysUnknown`
  - `internal/core/mail/decision_resend_test.go` `TestDecisionResendSummaryThatDoesNotReadKeepsItsCompletion`
  - `internal/core/mail/decision_view_test.go` `TestDecisionViewsDuringEveryPhase`
  - `internal/cli/decide_views_test.go` `TestTheResendSaysItIsOne`

- **D7. Each effect of a decision is installed at most once while the decision lives,
  recognised by an identity its progress and retention do not change:** the resend by its
  name in either form, kept while the mailbox lives; a note by the told run's once record,
  moot once that run has ended. Installation waits on no admission; a progressed,
  completed or retired effect is never installed again, never a new obligation and never a
  stop.
  Tests:
  - `internal/core/mail/decision_view_test.go` `TestDecisionEffectsSurviveAllRetention`
  - `internal/core/mail/decision_recovery_test.go` `TestDecisionEffectsAreNotRepeatedAfterRetention`
  - `internal/core/mail/decision_recovery_test.go` `TestDecisionInstallationBeforeAdmission`
  - `internal/core/mail/decision_install_test.go` `TestDecisionInstallationSurvivesEveryBrokenWrite`
  - `internal/core/mail/decision_identity_test.go` `TestDecisionToldRecordsOfAnotherNoteTellNothing`
  - `internal/core/mail/decision_test.go` `TestDecisionCorruptionNeverAuthorizesEffects`
  - `internal/core/mail/decision_matrix_test.go` `TestDecisionDurabilityAtEveryCut`

- **D8. Evidence that returns after a decision changes nothing taken or recorded on the
  sender's side.**
  Tests:
  - `internal/core/mail/decision_recovery_test.go` `TestDecisionPrecedesAllRecoveryEvidence`
  - `internal/core/mail/decision_final_lock_test.go` `TestDecisionFinalEvidenceIsLockedThroughCommit`
  - `internal/core/mail/decision_world_test.go` `TestDecisionCopiesFollowWorldTruth`
