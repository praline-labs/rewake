# Rules: the host

The host is the wrapper a session runs under: its launch lifetime, the claim of its name,
signals, availability, departure, the notices it sends, the delivery loop, its worktree,
the launch aliases and the endpoint the adapters reach
([design.md](../v2/design.md#the-layout)). Its rules have no numbers of their own: they
are the 1.x rules of [launch.md](../launch.md) and [delivery.md](../delivery.md) the
wrapper carries, and each block below names the tests that hold them
([stage3-tests-tcl.md](../v2/stage3-tests-tcl.md#the-host)). The tests move from
`internal/wrap` to `host` in S16.

## The wrapper

The wrapper's ownership of its run, signals and its following of a stop, control, availability,
compaction letters, departure, notices, naming, roles, the room's context and the
delivery window, as 1.x holds them.

Tests:
- `internal/wrap/ownership_test.go` `TestCleanupLeavesTheNextOwnerAlone`
- `internal/wrap/ownership_test.go` `TestShutdownOnlyRefusesItsOwnMail`
- `internal/wrap/ownership_test.go` `TestHarnessSharesTheTerminalGroup`
- `internal/wrap/ownership_test.go` `TestSignalIsNotRepeatedToAHarnessThatGotIt`
- `internal/wrap/ownership_test.go` `TestSignalAimedAtTheWrapperIsPassedOn`
- `internal/wrap/ownership_test.go` `TestInterruptDoesNotEndTheSession`
- `internal/wrap/ownership_test.go` `TestWrapperStopsWithTheHarness`
- `internal/wrap/ownership_test.go` `TestEachRunCleansOnlyItsOwnSocket`
- `internal/wrap/stop_follow_test.go` `TestAStoppedHarnessStaysStoppedWithItsWrapper`
- `internal/wrap/signals_test.go` `TestKeyboardSignalsAreNotIgnoredInTheHarness`
- `internal/wrap/signals_test.go` `TestAStopAlreadyOverIsNotFollowed`
- `internal/wrap/signals_test.go` `TestAStopInProgressIsFollowed`
- `internal/wrap/wrap_test.go` `TestExitCodeOfTheHarnessIsReturned`
- `internal/wrap/wrap_test.go` `TestSignalledHarnessReportsShellStyleCode`
- `internal/wrap/wrap_test.go` `TestSessionIsPublishedWhileRunningAndRemovedAfter`
- `internal/wrap/wrap_test.go` `TestMailboxIsServedWhileTheHarnessRuns`
- `internal/wrap/wrap_test.go` `TestMailOfAPreviousSessionIsRefused`
- `internal/wrap/wrap_test.go` `TestSessionEnvironmentReachesTheHarness`
- `internal/wrap/wrap_test.go` `TestTakenNameIsRefusedOnlyWhenExplicit`
- `internal/wrap/wrap_test.go` `TestTheRoleIsRecorded`
- `internal/wrap/control_test.go` `TestTheControlDirectoryLivesAsLongAsTheSession`
- `internal/wrap/control_test.go` `TestAHarnessThatServesNoRequestsGetsNoDirectory`
- `internal/wrap/availability_test.go` `TestAvailabilityMainOnlyReadinessEpochAndRetryDedup`
- `internal/wrap/availability_test.go` `TestLateMainDiscoversExistingWorkersInItsRoomOnly`
- `internal/wrap/availability_test.go` `TestAvailabilityWaitsForServiceAndTransportAndToleratesStorageFailure`
- `internal/wrap/availability_test.go` `TestAvailabilityUsesNormalWrapperNotifyDelivery`
- `internal/wrap/availability_batch_test.go` `TestLateMainDiscoveryPassGetsOneGroupedWake`
- `internal/wrap/availability_observation_test.go` `TestAvailabilityScanDoesNotCleanDeadPeerUnderNameLock`
- `internal/wrap/compaction_letters_test.go` `TestACompactionMainAskedForEndsInOneLetter`
- `internal/wrap/compaction_letters_test.go` `TestARefusedOrFailedCompactionIsALetterToo`
- `internal/wrap/compaction_letters_test.go` `TestALetterWaitsForItsOtherHalfOnlyAMoment`
- `internal/wrap/compaction_letters_test.go` `TestALetterComesWhenTheWorkerLeavesWithoutAWord`
- `internal/wrap/compaction_letters_test.go` `TestALetterComesAtTheBoundWithoutAWord`
- `internal/wrap/compaction_letters_test.go` `TestAnEarlierRunsRequestIsClosedByTheNext`
- `internal/wrap/compaction_letters_hold_test.go` `TestARecordItsCommandHoldsIsLeftAlone`
- `internal/wrap/compaction_letters_hold_test.go` `TestAKilledCommandsRecordIsClosed`
- `internal/wrap/compaction_letters_hold_test.go` `TestHelperHoldsARecord`
- `internal/wrap/compaction_letters_running_test.go` `TestACompactionStillRunningIsReportedByItsEnd`
- `internal/wrap/compaction_letters_running_test.go` `TestACompactionStillRunningAtTheBoundSaysSo`
- `internal/wrap/compaction_letters_running_test.go` `TestAStartedRecordedAfterTheEndDoesNotHideIt`
- `internal/wrap/departure_evidence_test.go` `TestDepartureUnknownRetainsWorkerUntilConfirmedDeath`
- `internal/wrap/departure_evidence_test.go` `TestDepartureConfirmedProcessEvidence`
- `internal/wrap/departure_evidence_test.go` `TestDepartureRevalidationAndUncertainRetry`
- `internal/wrap/departure_evidence_test.go` `TestDepartureRevalidationRejectsAliveAndForeignNamespace`
- `internal/wrap/session_notices_test.go` `TestCompactionNoticesDedupAndLateMain`
- `internal/wrap/session_notices_test.go` `TestACompactionMainAskedForGetsNoNotice`
- `internal/wrap/session_notices_test.go` `TestKnownDepartureRetainsOldStateAndDeduplicates`
- `internal/wrap/session_notices_test.go` `TestDepartureIgnoresUnknownLivenessStaleStateAndOldAbsence`
- `internal/wrap/session_notices_test.go` `TestDeparturePublicationRetriesWithoutDuplicate`
- `internal/wrap/session_notices_test.go` `TestObservedCompactionSurvivesDepartureBeforeNextScan`
- `internal/wrap/session_notices_test.go` `TestStateNoticeObserverStaysInRoomAndDoesNotNotifyItself`
- `internal/wrap/hold_notices_test.go` `TestMainIsToldOncePerHold`
- `internal/wrap/hold_notices_test.go` `TestAChangedCauseIsToldAgain`
- `internal/wrap/naming_test.go` `TestLaunchNamesFollowSelectedRoleAndHarness`
- `internal/wrap/naming_test.go` `TestExplicitPrefixControlsNameButNotRole`
- `internal/wrap/naming_test.go` `TestNamePrefixAndAssembledBoundaries`
- `internal/wrap/naming_test.go` `TestGenericHarnessAndAutomaticSuffixLength`
- `internal/wrap/naming_test.go` `TestConcurrentAutomaticNamesUseDefaultGeneral`
- `internal/wrap/naming_test.go` `TestHarnessLengthLeavesRoomForAPrefix`
- `internal/wrap/role_selection_test.go` `TestARoomReservesExplicitMainAndHonorsOtherRoles`
- `internal/wrap/role_selection_test.go` `TestAnExplicitWorkerCanStartBeforeMain`
- `internal/wrap/role_selection_test.go` `TestConcurrentDefaultLaunchesStayGeneral`
- `internal/wrap/role_selection_test.go` `TestRolePublicationWaitsForTheRoomLock`
- `internal/wrap/room_context_test.go` `TestTheHarnessReceivesItsRoomAndDefaultGeneral`
- `internal/wrap/room_context_test.go` `TestADeadMainDoesNotSelectAnImplicitMain`
- `internal/wrap/room_context_test.go` `TestEachRoomHasItsOwnMain`
- `internal/wrap/window_test.go` `TestWrapperGathersNotesApartIntoOneNotice`
- `internal/wrap/thread_test.go` `TestTheWrapperPinsDeliveryToTheHarnessThread` — recast on the adapter API's interfaces in S10
- `internal/wrap/thread_test.go` `TestTheWrapperPinsDeliveryToTheObservedThread` — recast on the adapter API's interfaces in S10

## Grants

The three steps of the unforgeable grant carry over unchanged (`docs/grants-authority.md`,
"The scheme": at send, at delivery, when main does not answer), in `core/grant`
with the wrapper's side in `host`. Applying a grant is the Permissions capability.

Tests:
- `internal/grantauth/grantauth_test.go` `TestAGrantRegisteredFromBelowIsConfirmed`
- `internal/grantauth/grantauth_test.go` `TestARegistrationFromOutsideTheWrapperIsRefused`
- `internal/grantauth/grantauth_test.go` `TestAConfirmationSettlesOnlyForItsRun`
- `internal/grantauth/grantauth_test.go` `TestAConfirmationFromAnotherProcessIsRefused`
- `internal/grantauth/grantauth_test.go` `TestAWrapperNotThereIsUnreachable`
- `internal/grantauth/grantauth_test.go` `TestAFullAuthorityRefusesAndKeepsWhatItHolds`
- `internal/grantauth/grantauth_test.go` `TestAGrantOutlivingItsWaitIsForgotten`
- `internal/grantauth/grantauth_test.go` `TestClosingFreesTheAddress`
- `internal/grantauth/grantauth_test.go` `TestABoundAddressIsNotTakenOver`
- `internal/grantauth/grantauth_test.go` `TestAConfirmationFromOtherNamespacesIsRefused`
- `internal/grantauth/checks_test.go` `TestAnAnswerFromAnotherLiveProcessIsRefused`
- `internal/grantauth/checks_test.go` `TestGrantsDifferingOnlyInBreadthOrGitAreNotTheSame`
- `internal/grantauth/checks_test.go` `TestARunInASandboxOfItsOwnIsNotItsWrapper`
- `internal/grantauth/checks_test.go` `TestAnAnswerNamingAnotherGrantIsNotTaken`
- `internal/grantauth/helper_test.go` `TestARegistrationFromOtherNamespacesIsRefused`
- `internal/grantauth/helper_test.go` `TestARegistrationGoesOnlyToTheCallersWrapper`
- `internal/grantauth/resume_test.go` `TestAGrantIsHeldWhileItsTaskIsOpen`
- `internal/grantauth/resume_test.go` `TestAGrantGoesOnlyIntoTheConversationItWasDeliveredTo`
- `internal/grantauth/resume_test.go` `TestAConfirmByAnotherProcessDeliversNothing`
- `internal/grantauth/resume_test.go` `TestAGrantConfirmedWithNoConversationIsNotHandedOver`
- `internal/wrap/grants_test.go` `TestAGrantIsConfirmedWithTheMainThatSentIt`
- `internal/wrap/grants_test.go` `TestMainHoldsTheConversationTheLetterWasPinnedTo`
- `internal/wrap/grant_checks_test.go` `TestAGrantWhoseDirectoryBecameALinkIsRefusedBeforeMainIsAsked`
- `internal/wrap/grant_checks_test.go` `TestAGrantFromAnInvalidSenderNameIsRefused`
- `internal/wrap/grant_resume_test.go` `TestAQuietMainIsAskedAgain`
- Gap: step 3's "the task fails, naming the main that sent it" — closed in S9.
