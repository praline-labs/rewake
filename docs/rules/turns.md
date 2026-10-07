# Rules: turn outcomes, O1–O2

What a turn's end reports when the turn was woken after `rewake pending`, failed or was
interrupted, in the full wording of [design-rules.md](../v2/design-rules.md#turn-outcomes).
The design gives these two rules no number; stage 3 numbers them **O1** and **O2** so
that they are checked like the rest ([stage3-tests.md](../v2/stage3-tests.md#how-a-rule-names-its-tests)).

## The rules

- **O1. An interim wake must not close a task early.** A turn woken after `rewake pending`
  that ends without a new mark, while its senders still run, is asked once before its
  end closes: the core keeps the answer and hands the session a reason to go on; the
  next end publishes that answer with its own, and every failure of the check falls to
  publishing, the behaviour without it (`cli/turn_hold.go:13-24`; held by
  `cli/turn_hold_test.go:80,122,149` and `test/workflow/pending_confirm_test.go:25`, with
  negative controls at 177 and 181). The mechanism is the adapter's TurnBoundary
  ([design-api.md](../v2/design-api.md#turnboundary)): since S5 an adapter that can hold
  an end confirms it (`cli/completion.go`, `ConfirmCompletion`). An end held once is
  known by its event: confirmed again, it gets the same reason and nothing is published
  or kept again, and once its continuation took the kept answer it is answered as
  published, never published as its own continuation.
  Tests:
  - `internal/inbox/confirm_test.go` `TestTheInterimRecordKeepsTheLastWord`
  - `internal/inbox/confirm_test.go` `TestAnInterimTieDoesNotDependOnOrder`
  - `internal/inbox/confirm_test.go` `TestTheInterimRecordOfAnotherRun`
  - `internal/inbox/pending_test.go` `TestAnInterimReportSettlesNothing`
  - `internal/inbox/pending_test.go` `TestAnInterimReportIsNotTakenByAWaitingQuestion`
  - `internal/inbox/journal_unknown_test.go` `TestAnInterimEndReplacesAnEndedRunsRecord`
  - `internal/cli/inbox_awaited_test.go` `TestAwaitedTakesTheLatestInterimOrStop`
  - `test/workflow/awaited_view_test.go` `TestAwaitedView`
  - `test/workflow/pending_report_test.go` `TestPendingReport`
  - `test/workflow/pending_report_test.go` `TestAnIgnoredPendingMarkFails`
  - `test/workflow/pending_report_test.go` `TestAPendingTurnEndThatSettlesFails`
  - `test/workflow/pending_report_test.go` `TestAPendingTurnEndWithoutItsTextFails`
  - `internal/cli/turn_hold_test.go` `TestAStopAfterAHoldCarriesTheHeldAnswer`
  - `internal/cli/turn_hold_test.go` `TestAnUnmarkedEndAfterAnInterimOneIsHeldOnce` — the Stop hook's case, goes in S9
  - `internal/cli/turn_hold_test.go` `TestAContinuationThatMarksPendingKeepsTheTaskOwed` — the Stop hook's case, goes in S9
  - `internal/cli/turn_hold_test.go` `TestAFailureAfterAHoldCarriesTheHeldAnswer` — the Stop hook's case, goes in S9
  - `internal/cli/turn_hold_test.go` `TestAnAnswerHeldAndNeverContinuedGoesWithTheNextEnd` — the Stop hook's case, goes in S9
  - `internal/cli/turn_hold_test.go` `TestATurnEndIsHeldOnlyAfterAnInterimOne` — the Stop hook's case, goes in S9
  - `test/workflow/pending_confirm_test.go` `TestPendingConfirm` — on the fixture column since S5; its Claude Code case goes in S9
  - `test/workflow/pending_confirm_test.go` `TestAnUnconfirmedPendingFails` — on the fixture column since S5; its Claude Code case goes in S9
  - `test/workflow/pending_confirm_test.go` `TestAConfirmationThatDropsTheAnswerFails` — on the fixture column since S5; its Claude Code case goes in S9
  - `internal/harness/fixture/turns_test.go` `TestAnEndIsConfirmedWhenItCanBeHeld`
  - `internal/cli/turn_confirm_test.go` `TestAConfirmedEndAfterAnInterimOneIsHeldOnce`
  - `internal/cli/turn_confirm_test.go` `TestAConfirmedContinuationThatMarksPendingKeepsTheTaskOwed`
  - `internal/cli/turn_confirm_test.go` `TestAFailedOrStoppedContinuationCarriesTheHeldAnswer`
  - `internal/cli/turn_confirm_test.go` `TestAnAnswerHeldAndNeverContinuedGoesWithTheNextConfirmedEnd`
  - `internal/cli/turn_confirm_test.go` `TestAConfirmedEndIsHeldOnlyAfterAnInterimOne`
  - `internal/cli/turn_confirm_test.go` `TestEveryFailureOfTheConfirmationCheckFallsToPublishing`
  - `internal/cli/turn_hold_senders_test.go` `TestOneUnreadableSenderAbandonsTheHold`
  - `internal/cli/turn_hold_senders_test.go` `TestAGoneSenderLeavesTheHoldToTheLiveOne`
  - `internal/cli/turn_confirm_test.go` `TestAConfirmedEndWithoutItsEventIsRefused`
  - `internal/cli/turn_confirm_repeat_test.go` `TestTheSameEndConfirmedAgainGetsTheSameAnswer`
  - `internal/cli/turn_confirm_repeat_test.go` `TestTwoEqualConfirmationsAtOnceHoldOnce`
  - `internal/cli/turn_confirm_repeat_test.go` `TestAHoldCutByACrashIsHeldOnceWhenConfirmedAgain`
  - `internal/cli/turn_confirm_repeat_test.go` `TestAHeldEndConfirmedAfterItsContinuationIsPublished`
  - `internal/inbox/held_end_test.go` `TestAJournalThatTookAHeldEndNamesItInEveryForm`
  - `internal/inbox/held_end_test.go` `TestAnUnreadableJournalIsNotTakenForNone`
  - `internal/inbox/held_end_test.go` `TestAHeldEndRaisesTheClockToItsPosition`

- **O2. Failed and interrupted turns** report as in 1.x (`docs/turn-outcomes.md`, "Failed
  turns", "Interim turn ends"), with the reason taken from the adapter's end event.
  Tests:
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
  - `test/workflow/stopped_routing_test.go` `TestStoppedRouting`
  - `test/workflow/stopped_routing_test.go` `TestAStoppedSentToMainFails`
  - `internal/cli/error_report_test.go` `TestFailedTurnsReachEveryWaitingSender` — its hook leg goes in S9
  - `internal/cli/error_report_test.go` `TestACodexNotifyReportsNothing` — the notify's case, goes in S8
  - `internal/cli/error_report_test.go` `TestUnclaimedFailuresReachMainAndMainKeepsItsOwn` — the hook's case, goes in S9
  - `internal/cli/error_report_test.go` `TestEmptyCompletionAfterWorkReportsAnErrorWithoutText` — the notify's case, goes in S8
  - `internal/cli/neutral_end_test.go` `TestNeutralFailedEndsReachEveryWaitingSender`
  - `internal/cli/neutral_end_test.go` `TestNeutralEndsWithoutTheirScopeReportNothing`
  - `internal/cli/neutral_end_test.go` `TestNeutralUnclaimedFailuresReachMainAndMainKeepsItsOwn`
  - `internal/cli/neutral_end_test.go` `TestNeutralAnEmptyEndAfterWorkReportsAnError`
  - Gap: the reason taken from the adapter's end event over a neutral TurnBoundary — closed in S10.
