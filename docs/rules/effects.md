# Rules: the effects of a mail operation, E1–E8

The rules every mail operation keeps — what proves an effect done or not done, and what
an unknown stops — in the full wording of [design-rules.md](../v2/design-rules.md#effects-e1e8),
from `mail-bridge-cli.md` rules 1–8 without the 1.x migration. Each rule ends with the
tests that hold it ([README.md](README.md)). The operator decision on an unknown
outcome, which the last clause of E8 names, joins this file with its rules in S18
(part B, [stage3-decision-recovery.md](../v2/stage3-decision-recovery.md)); until then
nothing lifts a stop without returning evidence.

## The rules

- **E1. An operation whose effect is unknown is never discarded and never bypassed.**
  A receipt goes only when its effect is proven absent or proven done; a step not
  recorded as done is no proof it was not done; age decides nothing — who continues an
  operation is decided by its scope or its receipt. (M1 rule 1.)
  Tests:
  - `internal/cli/bridge_rules_test.go` `TestAnUncertainPublicationIsNotDiscarded`
  - `internal/cli/bridge_rules_test.go` `TestAHeadsUpToAnEndedRunWithNothingLeftStaysUnknown`
  - `internal/cli/bridge_rules_test.go` `TestAnOpenOperationIsNotBypassedByAge`
  - `internal/inbox/journal_unknown_test.go` `TestAReportFoundWithoutItsMarkIsMarkedPublished`
  - `internal/inbox/journal_unknown_test.go` `TestAnUnknownPublicationMarkIsNeverPermission`
  - `internal/inbox/claims_test.go` `TestAnIntentWithoutItsLetterIsWrittenAgain`
  - `internal/inbox/claims_test.go` `TestTheSweepKeepsALetterWhoseIntentItCouldNotSettle`
  - `internal/inbox/claims_test.go` `TestAMailboxThatCannotBeSearchedProvesNothing`
  - `internal/inbox/journal_test.go` `TestAJournalDoesNotRepublishASweptReport`
  - `internal/inbox/journal_test.go` `TestAReportWhoseEntryFailedIsNotRepublishedAfterTheSweep`
  - `internal/cli/journal_test.go` `TestAHeadsUpPastItsDeadlineIsFinishedByRetry`
  - `internal/cli/journal_test.go` `TestTheShellRefersAnOpenOperationToItsReceipt`
  - `internal/receipt/receipt_test.go` `TestUnresolvedFindsOpenOperations`
  - `internal/receipt/receipt_test.go` `TestSweepKeepsWhatARecoveryReads`

- **E2. A lock is removed only by whoever holds it**, and a call that took a lock checks
  the file at the path is still its own. (M1 rule 2.)
  Tests:
  - `internal/receipt/receipt_test.go` `TestTheLockHoldsOneCallAtATime`
  - `internal/receipt/receipt_test.go` `TestTheSweepRespectsAHeldLock`
  - `internal/state/held_test.go` `TestTryHoldWaitsForTheWriter`
  - `internal/state/held_test.go` `TestTryHoldRefusesAFileRemovedBeforeItsLock`
  - `internal/state/held_test.go` `TestTryHoldRefusesAFileReplacedBeforeItsLock`
  - `internal/state/lock_test.go` `TestNameLockHoldsBetweenProcesses`
  - `internal/state/state_test.go` `TestNameLockSerialisesClaims`
  - Gap: the inbox sweep removing a record only under its lock has no test of its own — closed in S3.

- **E3. Every check that decides an effect runs inside the critical section, right
  before the effect, and again on a retry**, which is a new call with its own deadline
  and transport and the same limits. (M1 rule 3.)
  Tests:
  - `internal/cli/bridge_rules_test.go` `TestAnExpiredReadShowsAndClaimsNothing`
  - `internal/cli/bridge_rules_test.go` `TestAPendingMarkIsNotMadeAfterItsDeadline`
  - `internal/cli/bridge_rules_test.go` `TestAContinuationRefreshesALetterBeforeAnyPart`
  - `internal/cli/bridge_rules_test.go` `TestAToolRetryKeepsToTheToolSurface`
  - `internal/cli/bridge_rules_test.go` `TestAShellRetryDoesNotReadStdinAgain`
  - `internal/cli/addendum_test.go` `TestAnAddendumIsAskedAgainUnderTheLock`
  - `internal/cli/addendum_test.go` `TestAnAddendumsEditIsAskedAgainUnderTheLock`
  - `internal/inbox/claims_test.go` `TestPublicationOfReadsTheMailbox`
  - `internal/inbox/claims_test.go` `TestPublishOnceWritesNothingWhenItsCheckFails`
  - `internal/inbox/batch_edges_test.go` `TestGroupedPreparationRechecksAnswerLeasesExpiryAndEpoch`
  - `internal/inbox/withdraw_test.go` `TestTheLastCheckBeforeANoticeSeesAWithdrawal`
  - `internal/inbox/window_test.go` `TestARetryDoesNotRestartTheWindow`
  - Gap: a turn end's retry with its own deadline (only the tool's and the shell's retries are tested) — closed in S5, on the fixture's turns.
  - Gap: a once-publication checks its recipient's run outside the recipient's critical section (journal_held.go:145-161, cli/journal_steps.go:41, wrap/channel.go:230) — closed in S3.

- **E4. A read's completion is one durable fact every channel shares.** A letter leaves
  `unread/` only by being read once a part of it was shown, and never comes back; a late
  acknowledgment finds it read and marks nothing. (M1 rule 4.)
  Tests:
  - `internal/cli/bridge_rules_test.go` `TestALateAcknowledgmentOwesNothingTwice`
  - `internal/cli/bridge_rules_test.go` `TestALetterReadElsewhereShowsAsGone`
  - `internal/inbox/claims_test.go` `TestALetterBeingReadInPartsCannotBeTakenBack`
  - `internal/inbox/claims_test.go` `TestTheSweepKeepsALetterBeingRead`
  - `internal/inbox/claims_test.go` `TestALetterIsPublishedOnceAcrossReadAndSweep`
  - `internal/inbox/claims_test.go` `TestTheServerKeepsALetterBeingRead`
  - `internal/cli/bridge_read_test.go` `TestAToolReadIsReadOnlyWhenEveryPartArrived`
  - `internal/cli/bridge_read_test.go` `TestARepeatedToolReadInOneTurnIsTheSameRead`
  - `internal/cli/bridge_read_test.go` `TestTheShellFinishesALostToolRead`
  - `internal/cli/bridge_read_test.go` `TestPendingCountsALetterBeingRead`
  - `internal/cli/bridge_lookups_test.go` `TestAClaimedLetterThatCannotBeReadLeavesTheMarkOpen`
  - `internal/cli/bridge_lookups_test.go` `TestAnAcknowledgmentDoesNotWriteOverAWaiterItCannotRead`
  - `internal/cli/bridge_lookups_test.go` `TestARetriedAcknowledgmentStopsOnAStatusItCannotRead`
  - `internal/inbox/unread_test.go` `TestReadingHandsEachMessageOnce`
  - `internal/inbox/unread_test.go` `TestAReadDuringAPendingNoticeStands`
  - `internal/inbox/unread_test.go` `TestAReadIsNotUndoneByTheNoticeResult`
  - `internal/cli/read_test.go` `TestSendCountsAReadMessageAsDelivered`
  - `internal/cli/read_test.go` `TestTwoReadersAtOnceShowATaskOnce`
  - `internal/cli/stop_gate_test.go` `TestAReadInPartsStopsWithTheMailbox`
  - Gap: one fact across every channel — today tool and shell only — closed in S7 with the fixture's tool.

- **E5. The size bound holds on the final encoded bytes of every answer, in one place**;
  every text part names its letter, part, count and byte range. (M1 rule 5.)
  Tests:
  - `internal/bridge/bridge_test.go` `TestOnlyAWholeResultIsEvidence`
  - `internal/bridge/bridge_test.go` `TestCutKeepsCharactersAndBounds`
  - `internal/cli/bridge_rules_test.go` `TestAnEscapedDiagnosticFits`
  - `internal/cli/bridge_rules_test.go` `TestAnAnswerThatCannotBeKeptStillFits`
  - `internal/cli/bridge_rules_test.go` `TestEveryTextPartNamesItself`
  - `internal/cli/bridge_tool_test.go` `TestALongAnswerComesInParts`
  - `internal/bridge/server/encoder_test.go` `TestTheEncoderReplacesAnAnswerThatDoesNotFit` — rebuilt in S7 on the host endpoint's one encoder
  - `internal/bridge/server/encoder_test.go` `TestAChildsOutputIsCapped` — rebuilt in S7 on the host endpoint's one encoder
  - `internal/bridge/server/encoder_test.go` `TestOnlyTheEncoderWritesToStdout` — rebuilt in S7 on the host endpoint's one encoder
  - `internal/bridge/server/bound_test.go` `FuzzEveryMessageWrittenFits` — rebuilt in S7 on the host endpoint's one encoder
  - `internal/bridge/server/bound_test.go` `FuzzFramesAreBounded` — rebuilt in S7 on the host endpoint's one encoder

- **E6. A check has three outcomes: found, proven absent, unknown.** Only "no such file"
  proves absence; an unknown stops the effect and leaves the operation where a retry
  takes it up. Readers that only inform fold an unknown into their most cautious answer.
  (M1 rule 6.)
  Tests:
  - `internal/inbox/lookups_test.go` `TestALetterThatCannotBeReadIsNotLeftOut`
  - `internal/inbox/lookups_test.go` `TestAStatusIsFoundAbsentOrUnknown`
  - `internal/inbox/lookups_test.go` `TestAnsweredNeedsAMailboxItCanSearch`
  - `internal/inbox/lookups_test.go` `TestAWaiterThatCannotBeReadIsNotWrittenOver`
  - `internal/inbox/lookups_test.go` `TestAReadStopsOnAStatusItCannotRead`
  - `internal/inbox/lookups_test.go` `TestAWithdrawalThatCannotLookStopsBeforeWriting`
  - `internal/inbox/lookups_test.go` `TestAClaimedLetterThatCannotBeReadIsNotOwedByNobody`
  - `internal/inbox/lookups_test.go` `TestATurnRecordThatCannotBeReadIsNotNone`
  - `internal/inbox/lookups_test.go` `TestTheSweepKeepsWhatAnUnreadableWaiterMayOwe`
  - `internal/inbox/lookups_test.go` `TestNothingButTestsCallsTheLossyWaiters`
  - `internal/inbox/lookups_test.go` `TestADamagedWaitRecordIsNotReadAsZeros`
  - `internal/inbox/unknown_test.go` `TestAClaimThatCannotBeReadHoldsTheLetter`
  - `internal/inbox/unknown_test.go` `TestAStatusThatCannotBeReadStopsTheLookup`
  - `internal/inbox/unknown_test.go` `TestAMarkOrAWaiterThatCannotBeReadIsNotNone`
  - `internal/inbox/records_test.go` `TestAFileOfNoKnownKindStopsTheMailbox`
  - `internal/inbox/records_test.go` `TestANonRegularEntryAtEveryRecordPathStops`
  - `internal/inbox/records_test.go` `TestUnlistedRecordsNamesAStrayDirectory`
  - `internal/inbox/journal_unknown_test.go` `TestAnUnreadableInterimStopsTheJournal`
  - `internal/cli/bridge_unknown_test.go` `TestARefusedReadIsReplayedInItsTurn`
  - `internal/cli/bridge_unknown_test.go` `TestALetterThatCannotBeLookedUpIsNotShown`
  - `internal/cli/bridge_unknown_test.go` `TestAWaiterThatCannotBeReadLeavesTheMarkOpen`
  - `internal/cli/bridge_unknown_test.go` `TestARecipientThatCannotBeLookedUpIsNotAnEndedRun`
  - `internal/cli/bridge_lookups_test.go` `TestAReadThatCouldNotSeeTheLetterFreezesNothing`
  - `internal/cli/bridge_lookups_test.go` `TestAKeptAnswerThatCannotBeReadIsNotDropped`
  - `internal/cli/bridge_lookups_test.go` `TestAnUnreadableResultIsNotFailedWhenTheNameMovesOn`
  - `internal/cli/bridge_lookups_test.go` `TestAClearingThatFailedIsFinishedBeforeTheNextReport`
  - `internal/inbox/confirm_test.go` `TestAnUnreadableKeptAnswerIsNotWrittenOver`
  - Gap: readers that only inform (`awaited`, `owed`, `list`) folding an unknown into their most cautious answer — closed in S3.

- **E7. Every effect has an immutable identity and a proven scope, and recovery advances
  it only from durable evidence tied to that identity.** (M1 rule 7, clauses identity,
  clock, journal and scope.)
  - A turn end is one operation — publish its reports, take the kept answer they carry,
    clear the waits they answer, record whether the end was interim — named by its run
    and its event, never by an attempt; everything it writes is named from that.
  - Its scope is fixed once, before its first effect, from evidence the event carries.
    The read boundary is a position on the run's read clock, which numbers every read
    and every hold; a hold reserves its position, keeps the answer, then commits the
    clock, and a position is never issued twice.
  - The turn's window on the boot clock picks the pending marks that make it interim,
    opening after the turn's start or the latest earlier end a journal records,
    whichever is later; no end removes a pending mark, which lives as long as its run.
  - Every 2.0 TurnBoundary names its event (a turn id), so the 1.x case of an end heard
    once with no event goes; an end that names an event and carries no boundary still
    takes no effect on any attempt, and its waits stay owed.
  - What the operation will do is written down first (the journal), and nothing but the
    journal publishes. A stop after an interruption goes on taking its held answer (the
    owner, September 30, 2026).
  Tests:
  - `internal/cli/turn_journal_retry_test.go` `TestALateRetryAfterTheSweepPublishesNothing`
  - `internal/cli/turn_journal_retry_test.go` `TestARetryAnswersOnlyWhatItsFirstAttemptSaw`
  - `internal/cli/turn_test.go` `TestARetriedReadReportsOnce`
  - `internal/cli/review_receipt_identity_test.go` `TestStoppedAndErrorRetriesSettleOriginalScopeOnce`
  - `internal/cli/turn_retry_test.go` `TestRetriedTurnDoesNotConsumeLaterWork`
  - `internal/cli/turn_retry_test.go` `TestPartialTurnRetryKeepsOriginalOutcome`
  - `internal/cli/turn_scope_test.go` `TestARetryWhoseFirstAttemptRecordedNothingKeepsItsScope`
  - `internal/cli/turn_scope_test.go` `TestARetryTakesOnlyTheAnswerItsFirstAttemptSaw`
  - `internal/cli/turn_scope_test.go` `TestAnEndWithNothingOwedKeepsItsScopeOnRetry`
  - `internal/cli/turn_journal_test.go` `TestARecoveredEndTakesTheAnswerItPublished`
  - `internal/cli/turn_journal_test.go` `TestALateRetryKeepsALaterAnswer`
  - `internal/inbox/journal_test.go` `TestAJournalTakesOnlyTheAnswerItCarried`
  - `internal/inbox/read_boundary_test.go` `TestReadBoundaryExcludesLaterReadsAndIncludesSameTurnSteering`
  - `internal/inbox/read_boundary_test.go` `TestReadBoundaryCaptureNeverWaitsForMailbox`
  - `internal/inbox/read_boundary_test.go` `TestReadBoundaryHighWatermarkPreventsReuseAfterCounterLoss`
  - `internal/inbox/read_boundary_test.go` `TestReadClockProcessWriter`
  - `internal/inbox/read_boundary_test.go` `TestReadBoundaryIsSharedWithAnotherReaderProcess`
  - `internal/inbox/confirm_test.go` `TestAKeptAnswerBelongsToItsRun`
  - `internal/inbox/confirm_test.go` `TestAKeptAnswerIsTakenOnlyAtOrBelowTheBoundary`
  - `internal/inbox/pending_test.go` `TestAMarkIsNeverTakenAway`
  - `internal/inbox/pending_test.go` `TestTheWindowOpensAfterItsStartAndClosesAtItsEnd`
  - `internal/inbox/pending_test.go` `TestAMarkOfAnotherRunIsIgnoredAndSwept`
  - `internal/inbox/pending_test.go` `TestTheLatestMarkInATurnWins`
  - `internal/inbox/pending_test.go` `TestAMarkIsWrittenOncePerCall`
  - `internal/inbox/pending_test.go` `TestAnUnreadableMarkInTheWindowIsAnError`
  - `internal/inbox/pending_test.go` `TestTheWindowOpensAfterTheRunsLatestEarlierEnd`
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
  - `internal/inbox/journal_test.go` `TestAnAbandonedJournalWriteIsNotAJournal`
  - `internal/cli/review_receipt_identity_test.go` `TestReviewScopedGapReceiptsRemainDistinct` — rebuilt in S5 on neutral completions
  - `internal/cli/review_receipt_identity_test.go` `TestReviewStoppedReceiptAllowsSameTurnFinal` — rebuilt in S5 on neutral completions
  - `internal/cli/turn_test.go` `TestTwoTurnEndsAtOnceReportOnce` — rebuilt in S5 on neutral completions
  - `internal/cli/error_report_test.go` `TestACodexNotifyReportsNothing` — rebuilt in S5 on neutral completions
  - Gap: a hold reserving its read-clock position, never issued twice, never covered by a boundary before it commits (`inbox/kept.go:70-80`; only reads are tested) — closed in S3.
  - Gap: every TurnBoundary names its event — closed in S10 with the adapter API.

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
  - The barrier runs under the mailbox lock before a turn end reads a wait: plan; collect
    the obligations proven publications closed; decide every other report — superseded,
    published, or unknown and stopping — and complete the journal. (The 1.x steps 2, 3
    and 5 — conversion, successor, held — go.)
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
  - `internal/inbox/journal_unknown_test.go` `TestAReportFoundWithoutItsMarkIsMarkedPublished`
  - `internal/inbox/journal_unknown_test.go` `TestAnUnknownPublicationMarkIsNeverPermission`
  - `internal/inbox/claims_test.go` `TestALetterIsPublishedOnceAcrossReadAndSweep`
  - `internal/inbox/claims_test.go` `TestAnIntentWithoutItsLetterIsWrittenAgain`
  - `internal/inbox/journal_test.go` `TestAJournalDoesNotRepublishASweptReport`
  - `internal/inbox/journal_test.go` `TestAReportWhoseEntryFailedIsNotRepublishedAfterTheSweep`
  - `internal/inbox/stop_writes_test.go` `TestAGateWhoseStopCannotBeWrittenStillStops` — its lift by a removed cause inverted in S3
  - `internal/inbox/stop_writes_test.go` `TestALiftOfAStopThatFailsReachesTheCaller` — its lift by a removed cause inverted in S3
  - `internal/inbox/answer_stop_test.go` `TestAStoppedMailboxDoesNotTakeAnAnswer` — its lift by a removed cause inverted in S3
  - `internal/cli/stop_gate_test.go` `TestAnUnreadableJournalStopsEveryCall` — its lift by a removed cause inverted in S3
  - `internal/cli/stop_gate_test.go` `TestAReadInPartsStopsWithTheMailbox`
  - `internal/cli/stop_gate_test.go` `TestUnknownEvidenceStopsTheCallsAfterTheEnd`
  - `internal/inbox/plan_faults_test.go` `TestEveryReadOfThePlanStopsBeforeTheFirstEffect`
  - `internal/inbox/journal_test.go` `TestADoneJournalIsKeptWhileItsRunLives`
  - `internal/cli/turn_journal_retry_test.go` `TestALateRetryAfterTheSweepPublishesNothing`
  - `internal/receipt/receipt_test.go` `TestSweepKeepsWhatARecoveryReads`
  - `internal/inbox/sweep_test.go` `TestAnOwedMessageOutlivesTheSweep`
  - `internal/inbox/sweep_test.go` `TestAnOwedMessageLeftUnreadOutlivesTheSweep`
  - `internal/inbox/evidence_test.go` `TestTheEvidenceOfEveryEffectIsReadBeforeTheFirst` — rebuilt in S2 on journal records only
  - `internal/inbox/effect_stop_test.go` `TestAnUnknownOnlyAnEffectMetStopsUntilTheBarrier` — rebuilt in S2 on journal records only
  - `internal/inbox/effect_stop_test.go` `TestAStopThatCouldNotBeRecordedNamesEveryFailedWrite` — rebuilt in S2 on journal records only
  - `internal/inbox/late_unknown_test.go` `TestEveryDurableStopOutlivesAFailedRetry` — rebuilt in S2 on journal records only
  - `internal/inbox/late_unknown_test.go` `TestEveryEvidencePathIsReadBeforeTheFirstEffect` — rebuilt in S2 on journal records only
  - `internal/inbox/conversion_moot_test.go` `TestAReportForAnEndedRunOfThisBuildIsMoot` — rebuilt in S2 on journal records only
  - `internal/inbox/conversion_moot_test.go` `TestAMootReportClearsWhatItAnswered` — rebuilt in S2 on journal records only
  - `internal/inbox/conversion_test.go` `TestReportsAreDecidedTogether` — rebuilt in S2 on journal records only
  - Gap: a stop lifts only on returning evidence (two tests say the opposite) — closed in S3.
  - Gap: a stop tells main once and lets letters from others arrive, whatever found it — closed in S3.
  - Gap: nothing that names no run is swept by age — closed in S3.
  - Gap: a publication's proof is retired while an attempt admitted to its run can still replay it (sweep.go:74-89, once.go:206-216), and by a sweeper that is not the live run — closed in S3.
  - Gap: a cause that recurs after its resolution is a new occurrence, lifted only by its own evidence — closed in S3.
  - Gap: the operator decision — closed in S18 (part B).

## How E8 lifts a stop

E8 says a stop the plan found goes once the plan finds its cause gone, and that a stop is
lifted only by returning evidence, never by removing its cause. Read literally, the first
lets the removal of an unreadable record lift a stop, which the second forbids. Stage 3
reads them together as correction 2 of [stage3.md](../v2/stage3.md#corrections-to-the-accepted-design),
agreed in review: a stop is one write-once record per occurrence of a cause; an
occurrence is resolved only by evidence about its effect — the effect proven done or proven
not done — never by its path becoming absent or merely readable; an effect-only cause is
resolved the same way, once a barrier has run its effect through to evidence, not by the
barrier having run; one resolution never resolves another, and a cause that recurs after
its resolution is a new occurrence. S3 builds it
([stage3-steps.md](../v2/stage3-steps.md#s3-a-stop-is-resolved-per-cause-by-evidence-codex)),
and the tests above that lift a stop by a removed cause are inverted there.
