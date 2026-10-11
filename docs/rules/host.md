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
delivery window, as 1.x holds them; and the launch's version read, through the adapter's
gate before the claim, in a bounded probe whose output is never shown, refusing below the
minimum or when no version can be read
([design-api.md](../v2/design-api.md#minimum-versions)), in `internal/host/launch` since
S10.

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
- `internal/wrap/wrap_test.go` `TestWhatTheAdapterSaysNotToInheritStaysOut`
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
- `internal/wrap/thread_test.go` `TestTheWrapperPinsDeliveryToTheHarnessThread`
- `internal/wrap/thread_test.go` `TestTheWrapperPinsDeliveryToTheObservedThread`
- `internal/host/launch/version_test.go` `TestAVersionBelowTheMinimumIsRefused`
- `internal/host/launch/version_test.go` `TestAnUnreadableVersionIsRefused`
- `internal/host/launch/version_test.go` `TestAGateWithoutAMinimumReadsNothing`
- `internal/host/launch/probe_test.go` `TestAProbePastItsBoundIsEnded`
- `internal/host/launch/probe_test.go` `TestWhatAProbeLeftInASessionOfItsOwnIsEnded`
- `internal/host/launch/probe_test.go` `TestAProbeLeavesOtherChildrenAlone`

## The live set

What a run has proven of its adapter's capabilities is recorded in its session record as
it changes — set by the readiness exchange, withdrawn at once when lost — and every
decision another session makes about the run reads it, never the offer
([design-api.md](../v2/design-api.md#two-levels-offered-and-live)): a letter to a run
without a live Wake is accepted, exit 3, and waits in its inbox; a task or a question to
one without a live TurnBoundary fails, exit 1, nothing sent; a compaction or an interrupt
of one without a live Control fails, exit 1, nothing asked; a launch of a role that owes
reports waits for its TurnBoundary up to the readiness bound and is refused after.
Liveness never changes the run: a capability withdrawn and made live again leaves its
epoch, name, role and grants as they were. A set whose write fails is not taken for
recorded: the run's own view takes it at once, and the record is written again, once at
once and then with retries, until it lands or the run ends — never once the record has
become a later run's. Since S11.

Tests:
- `internal/core/registry/live_test.go` `TestModifyChangesTheRecordAsItStands`
- `internal/wrap/live_test.go` `TestTheLiveSetIsRecordedAndADropLeavesTheRun`
- `internal/wrap/live_test.go` `TestTheLiveSetDoesNotWriteAnotherRunsRecord`
- `internal/wrap/live_test.go` `TestAWithdrawalThatFailsToLandIsWrittenAgain`
- `internal/wrap/live_test.go` `TestAReportingRoleWaitsForItsBoundary`
- `internal/wrap/live_test.go` `TestABoundaryNeverReportedIsRefusedAtTheBound`
- `internal/wrap/live_test.go` `TestARunThatReportsNoneHasItsPlanLive`
- `internal/wrap/live_test.go` `TestADeliveryWithoutALiveWakeWaits`
- `internal/wrap/live_test.go` `TestWithoutALiveWakeNothingIsReservedOrPinned`
- `internal/core/mail/reservation_test.go` `TestNoReservationTakesThePlainPath`
- `internal/cli/live_set_test.go` `TestASendToARunWithoutALiveWakeIsAcceptedUnwoken`
- `internal/cli/live_set_test.go` `TestATaskToARunWithoutALiveBoundaryFails`
- `internal/cli/live_set_test.go` `TestTheLiveBoundaryDecidesNotTheOffer`
- `internal/cli/live_set_test.go` `TestABoundaryWithdrawnWhileStdinIsReadRefusesTheTask`
- `internal/cli/live_set_test.go` `TestSteeringARunWithoutALiveControlFails`
- `internal/adapter/fixture/live_test.go` `TestTheRunReportsItsLiveSetAsItChanges`
- `internal/adapter/fixture/live_test.go` `TestTheFirstReportLeavesOutWhatTheExchangeDidNotProve`
- `internal/adapter/fixture/live_test.go` `TestTheBriefingFollowsTheLiveTool`
- `internal/adapter/fixture/live_test.go` `TestControlIsCarriedOnlyWhileLive`
- `test/workflow/fixture_live_test.go` `TestFixtureLive`

## Grants

The three steps of the unforgeable grant carry over unchanged (`docs/grants-authority.md`,
"The scheme": at send, at delivery, when main does not answer), in `core/grant`
with the wrapper's side in `host`. Applying a grant is the Permissions capability.
What no grant reaches comes from the catalogue: every adapter's own configuration, and
what the catalogue declares for a harness the build has no adapter for — `~/.codex`,
`.codex` and `.agents` until stage 5 (decided 2026-10-11) — so the core names none.

Tests:
- `internal/adapter/catalog/catalog_test.go` `TestTheCatalogueProtectsTheHarnessesItHasNoAdapterFor`
- `internal/cli/send_dir_test.go` `TestAGrantRefusesWhatTheCatalogueProtects` — a grant into a checkout still refuses those names there, and those directories under the home
- `internal/core/grant/authority/grantauth_test.go` `TestAGrantRegisteredFromBelowIsConfirmed`
- `internal/core/grant/authority/grantauth_test.go` `TestARegistrationFromOutsideTheWrapperIsRefused`
- `internal/core/grant/authority/grantauth_test.go` `TestAConfirmationSettlesOnlyForItsRun`
- `internal/core/grant/authority/grantauth_test.go` `TestAConfirmationFromAnotherProcessIsRefused`
- `internal/core/grant/authority/grantauth_test.go` `TestAWrapperNotThereIsUnreachable`
- `internal/core/grant/authority/grantauth_test.go` `TestAFullAuthorityRefusesAndKeepsWhatItHolds`
- `internal/core/grant/authority/grantauth_test.go` `TestAGrantOutlivingItsWaitIsForgotten`
- `internal/core/grant/authority/grantauth_test.go` `TestClosingFreesTheAddress`
- `internal/core/grant/authority/grantauth_test.go` `TestABoundAddressIsNotTakenOver`
- `internal/core/grant/authority/grantauth_test.go` `TestAConfirmationFromOtherNamespacesIsRefused`
- `internal/core/grant/authority/checks_test.go` `TestAnAnswerFromAnotherLiveProcessIsRefused`
- `internal/core/grant/authority/checks_test.go` `TestGrantsDifferingOnlyInBreadthOrGitAreNotTheSame`
- `internal/core/grant/authority/checks_test.go` `TestARunInASandboxOfItsOwnIsNotItsWrapper`
- `internal/core/grant/authority/checks_test.go` `TestAnAnswerNamingAnotherGrantIsNotTaken`
- `internal/core/grant/authority/helper_test.go` `TestARegistrationFromOtherNamespacesIsRefused`
- `internal/core/grant/authority/helper_test.go` `TestARegistrationGoesOnlyToTheCallersWrapper`
- `internal/core/grant/authority/resume_test.go` `TestAGrantIsHeldWhileItsTaskIsOpen`
- `internal/core/grant/authority/resume_test.go` `TestAGrantGoesOnlyIntoTheConversationItWasDeliveredTo`
- `internal/core/grant/authority/resume_test.go` `TestAConfirmByAnotherProcessDeliversNothing`
- `internal/core/grant/authority/resume_test.go` `TestAGrantConfirmedWithNoConversationIsNotHandedOver`
- `internal/wrap/grants_test.go` `TestAGrantIsConfirmedWithTheMainThatSentIt`
- `internal/wrap/grants_test.go` `TestMainHoldsTheConversationTheLetterWasPinnedTo`
- `internal/wrap/grant_checks_test.go` `TestAGrantWhoseDirectoryBecameALinkIsRefusedBeforeMainIsAsked`
- `internal/wrap/grant_checks_test.go` `TestAGrantFromAnInvalidSenderNameIsRefused`
- `internal/wrap/grant_checks_test.go` `TestAGrantWhoseMainHasEndedFailsNamingThatMain`
- Gap: step 3's "the task fails, naming the main that sent it" — closed in S9.
