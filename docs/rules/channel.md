# Rules: the channel record, C1–C8

What the record of a run's mail channel holds and what it proves, in the full wording of
[design-rules.md](../v2/design-rules.md#the-channel-record-c1c8), from
`mail-bridge-channel.md` rules 1–8.

Core and harness-free (revision, R3): no branch on a harness id in `core/channel`; what
one adapter needs beyond the neutral events lives in the adapter.

## The rules

- **C1.** Per channel — tool, shell — one observation with its own evidence and time,
  plus a policy block; the state shown is derived, never stored apart.
  Tests:
  - `internal/channel/space_oracle_test.go` `TestEqualComparesEveryField`
  - `internal/wrap/channel_test.go` `TestTheShellObservationsAreFoldedAndTaken`
  - `internal/channel/space_test.go` `TestEveryEventSequenceKeepsTheRules`
  - `internal/channel/table_test.go` `TestEachFailurePointShowsAndTellsWhatTheTextSays`

- **C2. Connected is not working.** A transport's hello proves it reached the host; only
  a call that met its binding proves the tool carried one.
  Tests:
  - `internal/channel/connections_test.go` `TestTwoConnectionsAcrossATicketInEveryOrder`
  - `internal/channel/connections_test.go` `TestAHelloAtTheTimersEndIsInTime`
  - `internal/channel/connections_test.go` `TestATicketStopsTheTimer`
  - `internal/channel/connections_test.go` `TestALateHelloUndoesFailuresUntilTheLastClose`
  - `internal/wrap/channel_test.go` `TestAClosedServerFailsOnlyWhileItsHarnessLives`

- **C3. Silence proves nothing.** No hello, no call, no shell command are not failures;
  a wait names what was not observed, never what did not happen.
  Tests:
  - `internal/channel/silence_test.go` `TestSilenceProvesNothing`
  - `internal/channel/table_test.go` `TestEachFailurePointShowsAndTellsWhatTheTextSays`

- **C4.** Each channel has one kind of evidence: the tool's, a call bound (T2); the
  shell's, a mail operation that wrote under a lock or failed to reach it.
  Tests:
  - `internal/wrap/channel_test.go` `TestTheShellObservationsAreFoldedAndTaken`
  - `internal/channel/space_test.go` `TestEveryEventSequenceKeepsTheRules`

- **C5.** Events fold by when they happened, not by when they arrived.
  Tests:
  - `internal/channel/order_test.go` `TestEveryArrivalOrderFoldsAsEventTime`
  - `internal/channel/late_test.go` `TestALateTicketKeepsTheFirstFailureAfterIt`
  - `internal/channel/late_test.go` `TestALateTicketDoesNotUndoALaterClose`
  - `internal/wrap/channel_order_test.go` `TestACloseFoldedAfterLaterFailuresLandsWhereItHappened`
  - `internal/wrap/channel_order_test.go` `TestATicketToldAfterLaterFailuresFoldsInItsPlace`
  - `internal/wrap/channel_order_test.go` `TestACloseYoungerThanAHeartbeatWaits`
  - `internal/bridge/endpoint/channel_order_test.go` `TestAHelloDeliveredLateKeepsItsConnectionLive`
  - `internal/bridge/endpoint/channel_order_test.go` `TestARefusedHelloDeliveredAfterAHelloKeepsItsFailure`

- **C6.** A notice states an action first and fits the preview.
  Tests:
  - `internal/channel/preview_test.go` `TestEveryFirstLineFitsThePreview`
  - `internal/channel/notices_test.go` `TestTheKeyWindowHoldsARepeatUntilItEnds`
  - `internal/channel/notices_test.go` `TestNoMoreThanSixAnHourToOneRecipient`
  - `internal/channel/notices_test.go` `TestANewMainGetsTheCurrentCategory`
  - `internal/channel/notices_test.go` `TestAPublicationKeepsItsIdentityAndADropStays`
  - `internal/channel/windows_test.go` `TestTheWindowsOverEveryLandingHistory`
  - `internal/harness/notice_test.go` `TestNoticesPreviewOnlyTheAuthorsFirstLine`
  - `internal/harness/notice_test.go` `TestPreviewsAreBoundedAndCannotAddTerminalLines`
  - `internal/harness/notice_test.go` `TestWidePreviewFitsOneHundredColumns`
  - `internal/harness/notice_test.go` `TestGroupedNoticeKeepsCompactPreviewWithoutInstructions`
  - `internal/harness/notice_recall_test.go` `TestARecallIsShownBesideANewerLetter`
  - `internal/harness/notice_recall_test.go` `TestTwoRecallsAreBothShown`
  - `internal/harness/notice_recall_test.go` `TestALongRecallKeepsItsInstruction`
  - `internal/harness/notice_recall_test.go` `TestAReplacementNamesTheMessageItReplaces`
  - `internal/harness/notice_recall_test.go` `TestAReplacementIsShownBesideANewerLetter`
  - `internal/harness/notice_recall_test.go` `TestCorrectionsAreAllShownOldestFirst`
  - `internal/harness/notice_recall_test.go` `TestASingleCorrectionIsShownBesideTheNewestUnread`
  - `internal/wrap/channel_test.go` `TestANoticeLandsOnceAcrossHeartbeats`
  - `internal/wrap/channel_test.go` `TestMainIsToldUnlessItIsTheRun`
  - `internal/wrap/channel_order_test.go` `TestNoticesWaitForTheOneInFlight`

- **C7. A policy refusal is not routed around** (the owner, October 4, 2026) — a managed
  policy that keeps mods off included.
  Tests:
  - `internal/wrap/channel_test.go` `TestAdviceFixedBeforeTheBlockIsDropped`
  - `internal/wrap/channel_order_test.go` `TestADenialToldWhileACloseWaitsStopsAdviceAtOnce`
  - `internal/channel/space_test.go` `TestEveryEventSequenceKeepsTheRules`

- **C8.** Nothing is relaunched, granted or approved by the channel.
  Tests:
  - `internal/wrap/channel_test.go` `TestANoticeForAMainThatLeftIsDropped`
  - `internal/wrap/channel_test.go` `TestNothingIsToldAfterTheHarnessExits`
  - `internal/wrap/channel_order_test.go` `TestANoticeFixedBeforeTheExitIsNotWrittenAfterIt`
  - `internal/wrap/channel_order_test.go` `TestALostBackendFreezesTheRecordBeforeTheHarnessEnds`
  - `internal/wrap/channel_order_test.go` `TestAnAcceptedSignalFreezesTheRecord`
  - `internal/channel/space_test.go` `TestEveryEventSequenceKeepsTheRules`
  - `internal/wrap/channel_effects_test.go` `TestTheChannelGrantsAndApprovesNothing`
