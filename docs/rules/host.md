# Rules: the host

The host is the wrapper a session runs under: its launch lifetime, the claim of its name,
signals, availability, departure, the notices it sends, the delivery loop, its worktree,
the launch aliases and the endpoint the adapters reach
([design.md](../v2/design.md#the-layout)). Its rules have no numbers of their own: they
are the 1.x rules of [launch.md](../launch.md) and [delivery.md](../delivery.md) the
wrapper carries, and each block below names the tests that hold them
([stage3-tests-tcl.md](../v2/stage3-tests-tcl.md#the-host)). The tests are in
`internal/host` since S16 moved them there from `internal/wrap`.

## The wrapper

The wrapper's ownership of its run, signals and its following of a stop, control, availability,
compaction letters, departure, notices, naming, roles, the room's context and the
delivery window, as 1.x holds them; and the launch's version read, through the adapter's
gate before the claim, in a bounded probe whose output is never shown, refusing below the
minimum or when no version can be read
([design-api.md](../v2/design-api.md#minimum-versions)), in `internal/host/launch` since
S10.

Tests:
- `internal/host/ownership_test.go` `TestCleanupLeavesTheNextOwnerAlone`
- `internal/host/ownership_test.go` `TestShutdownOnlyRefusesItsOwnMail`
- `internal/host/ownership_test.go` `TestHarnessSharesTheTerminalGroup`
- `internal/host/ownership_test.go` `TestSignalIsNotRepeatedToAHarnessThatGotIt`
- `internal/host/ownership_test.go` `TestSignalAimedAtTheWrapperIsPassedOn`
- `internal/host/ownership_test.go` `TestInterruptDoesNotEndTheSession`
- `internal/host/ownership_test.go` `TestWrapperStopsWithTheHarness`
- `internal/host/ownership_test.go` `TestEachRunCleansOnlyItsOwnSocket`
- `internal/host/stop_follow_test.go` `TestAStoppedHarnessStaysStoppedWithItsWrapper`
- `internal/host/signals_test.go` `TestKeyboardSignalsAreNotIgnoredInTheHarness`
- `internal/host/signals_test.go` `TestAStopAlreadyOverIsNotFollowed`
- `internal/host/signals_test.go` `TestAStopInProgressIsFollowed`
- `internal/host/wrap_test.go` `TestExitCodeOfTheHarnessIsReturned`
- `internal/host/wrap_test.go` `TestSignalledHarnessReportsShellStyleCode`
- `internal/host/wrap_test.go` `TestSessionIsPublishedWhileRunningAndRemovedAfter`
- `internal/host/wrap_test.go` `TestMailboxIsServedWhileTheHarnessRuns`
- `internal/host/wrap_test.go` `TestMailOfAPreviousSessionIsRefused`
- `internal/host/wrap_test.go` `TestSessionEnvironmentReachesTheHarness`
- `internal/host/wrap_test.go` `TestWhatTheAdapterSaysNotToInheritStaysOut`
- `internal/host/wrap_test.go` `TestTakenNameIsRefusedOnlyWhenExplicit`
- `internal/host/wrap_test.go` `TestTheRoleIsRecorded`
- `internal/host/control_test.go` `TestTheControlDirectoryLivesAsLongAsTheSession`
- `internal/host/control_test.go` `TestAHarnessThatServesNoRequestsGetsNoDirectory`
- `internal/host/availability_test.go` `TestAvailabilityMainOnlyReadinessEpochAndRetryDedup`
- `internal/host/availability_test.go` `TestLateMainDiscoversExistingWorkersInItsRoomOnly`
- `internal/host/availability_test.go` `TestAvailabilityWaitsForServiceAndTransportAndToleratesStorageFailure`
- `internal/host/availability_test.go` `TestAvailabilityUsesNormalWrapperNotifyDelivery`
- `internal/host/availability_batch_test.go` `TestLateMainDiscoveryPassGetsOneGroupedWake`
- `internal/host/availability_observation_test.go` `TestAvailabilityScanDoesNotCleanDeadPeerUnderNameLock`
- `internal/host/compaction_letters_test.go` `TestACompactionMainAskedForEndsInOneLetter`
- `internal/host/compaction_letters_test.go` `TestARefusedOrFailedCompactionIsALetterToo`
- `internal/host/compaction_letters_test.go` `TestALetterWaitsForItsOtherHalfOnlyAMoment`
- `internal/host/compaction_letters_test.go` `TestALetterComesWhenTheWorkerLeavesWithoutAWord`
- `internal/host/compaction_letters_test.go` `TestALetterComesAtTheBoundWithoutAWord`
- `internal/host/compaction_letters_test.go` `TestAnEarlierRunsRequestIsClosedByTheNext`
- `internal/host/compaction_letters_hold_test.go` `TestARecordItsCommandHoldsIsLeftAlone`
- `internal/host/compaction_letters_hold_test.go` `TestAKilledCommandsRecordIsClosed`
- `internal/host/compaction_letters_hold_test.go` `TestHelperHoldsARecord`
- `internal/host/compaction_letters_running_test.go` `TestACompactionStillRunningIsReportedByItsEnd`
- `internal/host/compaction_letters_running_test.go` `TestACompactionStillRunningAtTheBoundSaysSo`
- `internal/host/compaction_letters_running_test.go` `TestAStartedRecordedAfterTheEndDoesNotHideIt`
- `internal/host/departure_evidence_test.go` `TestDepartureUnknownRetainsWorkerUntilConfirmedDeath`
- `internal/host/departure_evidence_test.go` `TestDepartureConfirmedProcessEvidence`
- `internal/host/departure_evidence_test.go` `TestDepartureRevalidationAndUncertainRetry`
- `internal/host/departure_evidence_test.go` `TestDepartureRevalidationRejectsAliveAndForeignNamespace`
- `internal/host/session_notices_test.go` `TestCompactionNoticesDedupAndLateMain`
- `internal/host/session_notices_test.go` `TestACompactionMainAskedForGetsNoNotice`
- `internal/host/session_notices_test.go` `TestKnownDepartureRetainsOldStateAndDeduplicates`
- `internal/host/session_notices_test.go` `TestDepartureIgnoresUnknownLivenessStaleStateAndOldAbsence`
- `internal/host/session_notices_test.go` `TestDeparturePublicationRetriesWithoutDuplicate`
- `internal/host/session_notices_test.go` `TestObservedCompactionSurvivesDepartureBeforeNextScan`
- `internal/host/session_notices_test.go` `TestStateNoticeObserverStaysInRoomAndDoesNotNotifyItself`
- `internal/host/hold_notices_test.go` `TestMainIsToldOncePerHold`
- `internal/host/hold_notices_test.go` `TestAChangedCauseIsToldAgain`
- `internal/host/naming_test.go` `TestLaunchNamesFollowSelectedRoleAndHarness`
- `internal/host/naming_test.go` `TestExplicitPrefixControlsNameButNotRole`
- `internal/host/naming_test.go` `TestNamePrefixAndAssembledBoundaries`
- `internal/host/naming_test.go` `TestGenericHarnessAndAutomaticSuffixLength`
- `internal/host/naming_test.go` `TestConcurrentAutomaticNamesUseDefaultGeneral`
- `internal/host/naming_test.go` `TestHarnessLengthLeavesRoomForAPrefix`
- `internal/host/role_selection_test.go` `TestARoomReservesExplicitMainAndHonorsOtherRoles`
- `internal/host/role_selection_test.go` `TestAnExplicitWorkerCanStartBeforeMain`
- `internal/host/role_selection_test.go` `TestConcurrentDefaultLaunchesStayGeneral`
- `internal/host/role_selection_test.go` `TestRolePublicationWaitsForTheRoomLock`
- `internal/host/room_context_test.go` `TestTheHarnessReceivesItsRoomAndDefaultGeneral`
- `internal/host/room_context_test.go` `TestADeadMainDoesNotSelectAnImplicitMain`
- `internal/host/room_context_test.go` `TestEachRoomHasItsOwnMain`
- `internal/host/window_test.go` `TestWrapperGathersNotesApartIntoOneNotice`
- `internal/host/thread_test.go` `TestTheWrapperPinsDeliveryToTheHarnessThread`
- `internal/host/thread_test.go` `TestTheWrapperPinsDeliveryToTheObservedThread`
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

The wait before the start holds for a boundary the run proves before its harness starts. One
the run proves only once its harness runs (`AfterStart`, Claude Code's mod from S4-6) is not
awaited before the start: the harness starts, the set is Wake alone until the proof, and the
run's availability waits for the boundary up to its own bound, past which availability goes
ahead with the boundary not live. What a mod proved is withdrawn when its heartbeat lapses and
made live again by a later admitted hello; a hello of a load a later one superseded, or with a
lower load number than the current load's — the number each load takes from its run's counter
before its first hello — is refused and changes nothing, however late it arrives, and each
report carries the number of the transition it follows, so a report of an earlier one changes
nothing. An end that arrives after a
withdrawal is still taken: a withdrawn boundary refuses new tasks, it drops no report
([stage4-turns.md](../v2/stage4-turns.md#the-live-set)). Held since S4-6, October 16, 2026:
the start and availability since its part A, the withdrawal, the load numbers, the
generations, the reports' transitions and the late end since its part B.

Tests:
- `internal/core/registry/live_test.go` `TestModifyChangesTheRecordAsItStands`
- `internal/host/live_test.go` `TestTheLiveSetIsRecordedAndADropLeavesTheRun`
- `internal/host/live_test.go` `TestTheLiveSetDoesNotWriteAnotherRunsRecord`
- `internal/host/live_test.go` `TestAWithdrawalThatFailsToLandIsWrittenAgain`
- `internal/host/live_test.go` `TestAReportingRoleWaitsForItsBoundary`
- `internal/host/live_test.go` `TestABoundaryNeverReportedIsRefusedAtTheBound`
- `internal/host/after_start_test.go` `TestABoundaryProvenAfterTheStartStartsTheHarnessAndAvailabilityWaitsForIt`
- `internal/host/after_start_test.go` `TestAvailabilityGoesAheadPastTheBoundWithTheBoundaryNotLive`
- `internal/adapter/claude/mod_live_test.go` `TestTheSetIsWakeUntilTheHelloThenTheBoundaryAndSpacesOnMain`
- `internal/adapter/claude/mod_live_test.go` `TestALapseLeavesWakeAndANewHelloRestoresTheSet`
- `internal/host/after_start_test.go` `TestABoundaryWithdrawnAfterAvailabilityLeavesWakeUntilProvenAgain`
- `internal/adapter/claude/modhost/alive_test.go` `TestALapseWithdrawsWhatTheModProvedAndANewHelloRestoresIt`
- `internal/adapter/claude/modhost/alive_test.go` `TestAHelloRetriedAfterItsAnswerWasLostKeepsItsGeneration`
- `internal/adapter/claude/modhost/alive_test.go` `TestAnEarlierLoadsHelloAfterALaterOnesIsRefused`
- `internal/adapter/claude/modhost/alive_test.go` `TestAHelloOfAnotherLoadWithTheCurrentNumberBindsAsTheLater`
- `internal/adapter/claude/modhost/alive_test.go` `TestAPollDelayedAcrossANewHelloOfTheSameLoadIsRefused`
- `internal/adapter/claude/modhost/alive_test.go` `TestALapseTimerOfAnEarlierTransitionDoesNothing`
- `internal/adapter/claude/modhost/alive_test.go` `TestAnEndAfterALapseIsTakenAndMakesNothingLive`
- `internal/adapter/claude/modhost/alive_test.go` `TestALapseAfterTheCloseReportsNothing`
- `internal/adapter/claude/modhost/alive_test.go` `TestAHeldPollHoldsNoLock`
- `internal/adapter/claude/mod_test.go` `TestTheModsTestsPass`
- `internal/host/live_test.go` `TestARunThatReportsNoneHasItsPlanLive`
- `internal/host/live_test.go` `TestADeliveryWithoutALiveWakeWaits`
- `internal/host/live_test.go` `TestWithoutALiveWakeNothingIsReservedOrPinned`
- `internal/host/delivery/reservation_test.go` `TestNoReservationTakesThePlainPath`
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
- `internal/host/grants_test.go` `TestAGrantIsConfirmedWithTheMainThatSentIt`
- `internal/host/grants_test.go` `TestMainHoldsTheConversationTheLetterWasPinnedTo`
- `internal/host/grant_checks_test.go` `TestAGrantWhoseDirectoryBecameALinkIsRefusedBeforeMainIsAsked`
- `internal/host/grant_checks_test.go` `TestAGrantFromAnInvalidSenderNameIsRefused`
- `internal/host/grant_checks_test.go` `TestAGrantWhoseMainHasEndedFailsNamingThatMain`
