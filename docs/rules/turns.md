---
description: O1–O2, an interim wake that must not close a task early and how failed and interrupted turns report; open before changing what a turn end reports.
---

# Rules: turn outcomes, O1–O2

What a turn's end reports when the turn was woken after `rewake pending`, failed or was
interrupted. The behaviour in full is [turn-outcomes.md](../turn-outcomes.md).

## The rules

- **O1. An interim wake must not close a task early.** A turn woken after `rewake pending`
  that ends without a new mark, while its senders still run, is asked once before its
  end closes: the core keeps the answer and hands the session a reason to go on; the
  next end publishes that answer with its own, and every failure of the check falls to
  publishing, the behaviour without it (`core/mail/turn_hold.go`). The mechanism is the
  adapter's turn boundary ([design-api.md](../v2/design-api.md#turnboundary)): an adapter
  that can hold an end confirms it (`cli/completion.go`, `ConfirmCompletion`). An end held
  once is known by its event: confirmed again, it gets the same reason and nothing is
  published or kept again, and once its continuation took the kept answer it is answered as
  published, never published as its own continuation.
  Known limit: only the fixture holds an end. Claude Code and pi report their turns without
  holding one (`HoldsEnd` false), so there an interim end without a new mark publishes its
  answer as final and closes the task early, and the briefing alone asks for the mark
  ([stage4-turns.md](../v2/stage4-turns.md#what-not-holding-costs)).
  Tests:
  - `internal/core/mail/confirm_test.go` `TestTheInterimRecordKeepsTheLastWord`
  - `internal/core/mail/confirm_test.go` `TestAnInterimTieDoesNotDependOnOrder`
  - `internal/core/mail/confirm_test.go` `TestTheInterimRecordOfAnotherRun`
  - `internal/core/mail/pending_test.go` `TestAnInterimReportSettlesNothing`
  - `internal/host/delivery/pending_test.go` `TestAnInterimReportIsNotTakenByAWaitingQuestion`
  - `internal/cli/inbox_awaited_test.go` `TestAwaitedTakesTheLatestInterimOrStop`
  - `test/workflow/awaited_view_test.go` `TestAwaitedView`
  - `test/workflow/pending_report_test.go` `TestPendingReport`
  - `internal/core/mail/turn_hold_test.go` `TestAStopAfterAHoldCarriesTheHeldAnswer`
  - `internal/core/mail/turn_hold_test.go` `TestAnUnmarkedEndAfterAnInterimOneIsHeldOnce`
  - `internal/core/mail/turn_hold_test.go` `TestAContinuationThatMarksPendingKeepsTheTaskOwed`
  - `internal/core/mail/turn_hold_test.go` `TestAFailureAfterAHoldCarriesTheHeldAnswer`
  - `internal/core/mail/turn_hold_test.go` `TestAnAnswerHeldAndNeverContinuedGoesWithTheNextEnd`
  - `internal/core/mail/turn_hold_test.go` `TestATurnEndIsHeldOnlyAfterAnInterimOne`
  - `test/workflow/pending_confirm_test.go` `TestPendingConfirm`
  - `internal/adapter/fixture/turns_test.go` `TestAnEndIsConfirmedWhenItCanBeHeld`
  - `internal/cli/turn_confirm_test.go` `TestAConfirmedEndAfterAnInterimOneIsHeldOnce`
  - `internal/core/mail/turn_hold_failure_test.go` `TestEveryFailureOfTheConfirmationCheckFallsToPublishing`
  - `internal/cli/turn_hold_senders_test.go` `TestOneUnreadableSenderAbandonsTheHold`
  - `internal/cli/turn_hold_senders_test.go` `TestAGoneSenderLeavesTheHoldToTheLiveOne`
  - `internal/cli/turn_hold_senders_test.go` `TestAnUndecodableSenderIsNotTakenForGone`
  - `internal/cli/turn_confirm_repeat_test.go` `TestTheSameEndConfirmedAgainGetsTheSameAnswer`
  - `internal/cli/turn_confirm_repeat_test.go` `TestTwoEqualConfirmationsAtOnceHoldOnce`
  - `internal/core/mail/held_end_test.go` `TestAJournalThatTookAHeldEndNamesItInEveryForm`
  - `internal/core/mail/held_end_test.go` `TestAnUnreadableJournalIsNotTakenForNone`
  - `internal/core/mail/held_end_test.go` `TestAHeldEndRaisesTheClockToItsPosition`

- **O2. A failed or interrupted turn reports its outcome, with the reason the adapter's end
  event carries.** An end the harness reports as answered gives `finished`; failed or
  refused gives `error`, to every live waiter of the run, or to the room's main when none
  waits; aborted gives `stopped`, to the current waiters only, their waits kept for the
  turn that finishes the work. An empty answer after the turn read work is an error with
  empty text. A reason is never guessed: an end without its event or a known reason is
  refused. On Claude Code the event is the mod's `turn/end` of the session's own
  conversation, its reasons `answered`, `aborted`, `refused` and `failed`; a subagent's turn,
  which carries `agentId`, reports nothing — neither its end nor its start
  ([stage4-turns.md](../v2/stage4-turns.md#the-mods-side)). On pi, `agent_settled` reports
  aborted, a final `stop` answered, and other stop reasons failed
  ([research-pi.md](../research-pi.md)).
  Tests:
  - `internal/adapter/claude/modhost/turns_test.go` `TestEachReasonReachesTheCoreAsItsEndReason`
  - `internal/adapter/claude/mod_test.go` `TestTheModsTestsPass`
  - `internal/cli/stopped_test.go` `TestStoppedKeepsWorkForTheHumanContinuation`
  - `internal/cli/stopped_test.go` `TestStoppedWithNobodyWaitingSendsNothing`
  - `internal/cli/stopped_test.go` `TestAStoppedQuestionReturnsAndLeavesTheLaterResultReadable`
  - `internal/cli/pending_test.go` `TestAMarkDoesNotSurviveAnInterruptedTurn`
  - `internal/cli/pending_test.go` `TestAFailedOrStoppedTurnIgnoresTheMark`
  - `internal/cli/turn_journal_retry_test.go` `TestARecoveredPendingEndKeepsItsInterimOnRecord`
  - `internal/cli/turn_retry_test.go` `TestPartialTurnRetryKeepsOriginalOutcome`
  - `internal/cli/error_report_test.go` `TestReadingAnErrorOwesNothing`
  - `internal/cli/error_report_test.go` `TestIdentifiedFailuresAreNotRoutedAgainAfterTheirWaitsClear`
  - `internal/cli/error_report_test.go` `TestQuestionsReturnAnErrorOutcome`
  - `internal/cli/error_report_test.go` `TestFailedTurnsReachEveryWaitingSender`
  - `internal/cli/error_report_test.go` `TestUnclaimedFailuresReachMainAndMainKeepsItsOwn`
  - `test/workflow/stopped_routing_test.go` `TestStoppedRouting`
  - `internal/cli/neutral_end_test.go` `TestNeutralFailedEndsReachEveryWaitingSender`
  - `internal/cli/neutral_end_test.go` `TestNeutralEndsWithoutTheirScopeReportNothing`
  - `internal/cli/neutral_end_test.go` `TestNeutralUnclaimedFailuresReachMainAndMainKeepsItsOwn`
  - `internal/cli/neutral_end_test.go` `TestNeutralAnEmptyEndAfterWorkReportsAnError`
  - `test/workflow/turn_reasons_test.go` `TestTurnReasons`
  - `internal/cli/turn_end_api_test.go` `TestEachEndReasonReportsItsOutcome`
  - `internal/cli/turn_end_api_test.go` `TestATurnEndWithoutItsEventOrAKnownReasonIsRefused`
  - `internal/host/turn_boundary_test.go` `TestTheWrapperPassesEachEndOnWithItsEventAndReason`
