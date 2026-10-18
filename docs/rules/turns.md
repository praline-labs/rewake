# Rules: turn outcomes, O1–O2

What a turn's end reports when the turn was woken after `rewake pending`, failed or was
interrupted, in the full wording of [design-rules.md](../v2/design-rules.md#turn-outcomes).
The design gives these two rules no number; stage 3 numbers them **O1** and **O2** so
that they are checked like the rest ([stage3-tests.md](../archive/v2/stage3-tests.md#how-a-rule-names-its-tests)).

## The rules

- **O1. An interim wake must not close a task early.** A turn woken after `rewake pending`
  that ends without a new mark, while its senders still run, is asked once before its
  end closes: the core keeps the answer and hands the session a reason to go on; the
  next end publishes that answer with its own, and every failure of the check falls to
  publishing, the behaviour without it (`core/mail/turn_hold.go:12-23`; held by
  `cli/turn_hold_test.go:80,122,149` and `test/workflow/pending_confirm_test.go`). The mechanism is the adapter's TurnBoundary
  ([design-api.md](../v2/design-api.md#turnboundary)): since S5 an adapter that can hold
  an end confirms it (`cli/completion.go`, `ConfirmCompletion`). An end held once is
  known by its event: confirmed again, it gets the same reason and nothing is published
  or kept again, and once its continuation took the kept answer it is answered as
  published, never published as its own continuation.
  Known limit, October 16, 2026, owned by stage 4's step of the confirmation with P3:
  Claude Code reports its turns without holding an end (S4-6, `HoldsEnd` false), so there
  an interim end without a new mark publishes its answer as final and closes the task early
  ([stage4-turns.md](../v2/stage4-turns.md#what-not-holding-costs)).
  Tests:
  - `internal/core/mail/confirm_test.go` `TestTheInterimRecordKeepsTheLastWord`
  - `internal/core/mail/confirm_test.go` `TestAnInterimTieDoesNotDependOnOrder`
  - `internal/core/mail/confirm_test.go` `TestTheInterimRecordOfAnotherRun`
  - `internal/core/mail/pending_test.go` `TestAnInterimReportSettlesNothing`
  - `internal/host/delivery/pending_test.go` `TestAnInterimReportIsNotTakenByAWaitingQuestion`
  - `internal/core/mail/journal_unknown_test.go` `TestAnInterimEndReplacesAnEndedRunsRecord`
  - `internal/cli/inbox_awaited_test.go` `TestAwaitedTakesTheLatestInterimOrStop`
  - `test/workflow/awaited_view_test.go` `TestAwaitedView`
  - `test/workflow/pending_report_test.go` `TestPendingReport`
  - `internal/cli/turn_hold_test.go` `TestAStopAfterAHoldCarriesTheHeldAnswer`
  - `internal/cli/turn_hold_test.go` `TestAnUnmarkedEndAfterAnInterimOneIsHeldOnce` — a holdable end handed to the core since S9
  - `internal/cli/turn_hold_test.go` `TestAContinuationThatMarksPendingKeepsTheTaskOwed` — a holdable end handed to the core since S9
  - `internal/cli/turn_hold_test.go` `TestAFailureAfterAHoldCarriesTheHeldAnswer` — a holdable end handed to the core since S9
  - `internal/cli/turn_hold_test.go` `TestAnAnswerHeldAndNeverContinuedGoesWithTheNextEnd` — a holdable end handed to the core since S9
  - `internal/cli/turn_hold_test.go` `TestATurnEndIsHeldOnlyAfterAnInterimOne` — a holdable end handed to the core since S9
  - `test/workflow/pending_confirm_test.go` `TestPendingConfirm` — on the fixture column since S5, the only one since S9
  - `internal/adapter/fixture/turns_test.go` `TestAnEndIsConfirmedWhenItCanBeHeld`
  - `internal/cli/turn_confirm_test.go` `TestAConfirmedEndAfterAnInterimOneIsHeldOnce`
  - `internal/cli/turn_confirm_test.go` `TestAConfirmedContinuationThatMarksPendingKeepsTheTaskOwed`
  - `internal/cli/turn_confirm_test.go` `TestAFailedOrStoppedContinuationCarriesTheHeldAnswer`
  - `internal/cli/turn_confirm_test.go` `TestAnAnswerHeldAndNeverContinuedGoesWithTheNextConfirmedEnd`
  - `internal/cli/turn_confirm_test.go` `TestAConfirmedEndIsHeldOnlyAfterAnInterimOne`
  - `internal/cli/turn_confirm_test.go` `TestEveryFailureOfTheConfirmationCheckFallsToPublishing`
  - `internal/cli/turn_hold_senders_test.go` `TestOneUnreadableSenderAbandonsTheHold`
  - `internal/cli/turn_hold_senders_test.go` `TestAGoneSenderLeavesTheHoldToTheLiveOne`
  - `internal/cli/turn_hold_senders_test.go` `TestAnUndecodableSenderIsNotTakenForGone`
  - `internal/cli/turn_confirm_test.go` `TestAConfirmedEndWithoutItsEventIsRefused`
  - `internal/cli/turn_confirm_repeat_test.go` `TestTheSameEndConfirmedAgainGetsTheSameAnswer`
  - `internal/cli/turn_confirm_repeat_test.go` `TestTwoEqualConfirmationsAtOnceHoldOnce`
  - `internal/cli/turn_confirm_repeat_test.go` `TestAHoldCutByACrashIsHeldOnceWhenConfirmedAgain`
  - `internal/cli/turn_confirm_repeat_test.go` `TestAHeldEndConfirmedAfterItsContinuationIsPublished`
  - `internal/core/mail/held_end_test.go` `TestAJournalThatTookAHeldEndNamesItInEveryForm`
  - `internal/core/mail/held_end_test.go` `TestAnUnreadableJournalIsNotTakenForNone`
  - `internal/core/mail/held_end_test.go` `TestAHeldEndRaisesTheClockToItsPosition`

- **O2. Failed and interrupted turns** report as in 1.x (`docs/turn-outcomes.md`, "Failed
  turns", "Interim turn ends"), with the reason taken from the adapter's end event.
  On Claude Code, from S4-6, the event is `turn.complete` of the session's own conversation:
  its reasons `answer`, `aborted`, `refusal` and `error` report as answered, aborted, refused
  and failed, and a subagent's turn, which carries `agentId`, reports nothing — neither its end
  nor its `turn.start`, which the harness raises for a subagent's turns too from 2.1.299
  ([stage4-turns.md](../v2/stage4-turns.md#the-mods-side)); the mod's own tests hold that in the
  harness's test kit (`internal/adapter/claude/mod/tests/rewake.test.ts`, "each reason reaches
  the run as its end reason, and a subagent's turn is not sent", and "a subagent's turn sends
  neither its start nor its end, an empty agent id included, while the main loop's pair goes").
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
  - `test/workflow/stopped_routing_test.go` `TestStoppedRouting`
  - `internal/cli/error_report_test.go` `TestFailedTurnsReachEveryWaitingSender` — a boundary end handed to the core since S9
  - `internal/cli/error_report_test.go` `TestUnclaimedFailuresReachMainAndMainKeepsItsOwn` — a boundary end handed to the core since S9
  - `internal/cli/neutral_end_test.go` `TestNeutralFailedEndsReachEveryWaitingSender`
  - `internal/cli/neutral_end_test.go` `TestNeutralEndsWithoutTheirScopeReportNothing`
  - `internal/cli/neutral_end_test.go` `TestNeutralUnclaimedFailuresReachMainAndMainKeepsItsOwn`
  - `internal/cli/neutral_end_test.go` `TestNeutralAnEmptyEndAfterWorkReportsAnError`
  - `test/workflow/turn_reasons_test.go` `TestTurnReasons` — the fixture's end frame to the report
  - `internal/cli/turn_end_api_test.go` `TestEachEndReasonReportsItsOutcome`
  - `internal/cli/turn_end_api_test.go` `TestATurnEndWithoutItsEventOrAKnownReasonIsRefused`
  - `internal/host/turn_boundary_test.go` `TestTheWrapperPassesEachEndOnWithItsEventAndReason`
  - `internal/adapter/fixture/turns_test.go` `TestAnEndIsConfirmedWhenItCanBeHeld` — the fixture's end frame to a `TurnEnd` and its reason
