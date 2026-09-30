# A failed resume holds deliveries until the launch's conversation or the person's word

Stage A of the fix for the defect seen September 29, 2026
([work-queue.md](../work-queue.md#now-after-100)): a worker relaunched with a resume of a
conversation another Codex held went on in a new, empty one, rewake delivered main's task
there, and the worker could not read it, its sandbox leaving rewake's state directory
read-only. Built the same day on the owner's decisions of that day; the conversation
marker is stage B. How it works is in [delivery-conversation.md](../delivery-conversation.md).

## What was found

- The gateway did not know what the launch asked for: a refused resume let the binding
  go, and the next recognized `thread/start` bound a new conversation ready for delivery.
- Nothing checked that the worker could reach its mail before a notice went out.
- From Codex's source (0.159.0, 0.157.1): the terminal answers an active-writer refusal
  with a read-only view of the same conversation, not a new one; a remote terminal's
  resume drops permission overrides; `thread/read` states no permissions, while lifecycle
  replies and `thread/settings/updated` do ([research-codex.md](../research-codex.md#a-conversation-another-program-holds),
  [research-codex-conversation.md](../research-codex-conversation.md#what-states-a-conversations-permissions)).
  How the incident's new conversation began stays unestablished.

## What was done

- The launch's intent reaches the gateway: `resume <uuid>` names the conversation, and
  `--last`, a name or the picker leave it to the terminal's first resume, pinned before
  its reply. Fresh launches and forks are unchanged.
- Until the first successful resume of it, a reservation of any other conversation is
  refused with a hold the Codex adapter passes on as `inbox.ErrNotYet`: the message stays
  `pending`, nothing becomes readable, `rewake send` exits 3 naming both conversations.
  `/new` does not lift it. A resume refused with nothing selected after it holds as well,
  across connections, and one still on its way keeps the message pending.
- `rewake accept <session> <conversation>`, from the person's shell outside any session,
  through the run's control directory, taken only for the conversation selected now,
  checked and lifted in one step.
- `rewake inbox` refuses while the wrapper's own record holds the run's mail: written
  before the terminal starts, rewritten before the hold ends, read under the mailbox lock.
- main gets one notice per hold and per change of its cause; `rewake list` shows it,
  `--json` as `deliveryHold`; the terminal gets the server's `warning` notification once
  per hold.
- A check before each delivery that judged the conversation's sandbox from the
  permissions the server stated, and added rewake's state directory as a root where the
  policy allowed, was built with the rest and taken out after the third acceptance round
  ([below](#acceptance-round-3)). Its findings in rounds 1 and 2 are kept here as they
  were; the code and the tests they name went out with it.

## Acceptance, round 1

review-codex did not accept the first version, with seven findings, each confirmed by a
probe that went red; the probes are kept, adapted, as the tests named below.

1. A stale `rewake accept` could lift the hold of another binding: the selection was read
   before the intent's lock and not checked again. Now the check and the lift are one
   step under the connection's and the gateway's locks (`TestAcceptChecksAndLiftsInOneStep`,
   which stops the acceptance at the intent's lock and finds the connection locked).
2. `rewake inbox` trusted the published snapshot, written every 250 ms and read as "no
   hold" when missing, outside the mailbox lock. Now the wrapper's own record decides
   (`sessionstate.HoldMail`, `MailHeld`), read under the lock; an unreadable one refuses
   (`TestInboxWaitsForTheAcceptance`, `TestAnUnreadableAdmissionRefusesTheMail`,
   `TestAResumeLaunchHoldsTheMail`, `TestTheLiftAdmitsTheMailFirst`, the inbox replay).
3. A refused resume with nothing selected after it failed the delivery on Reserve's
   deadline and was told to no one. Now it holds at once, across connections, published
   and shown to the terminal; a resume on its way keeps the delivery pending
   (`TestARefusedResumeWithNothingSelectedHolds`, `TestAReadOnlyResumeHolds`,
   `TestAnOwedResumeKeepsTheDeliveryPending`).
4. A late reply to an earlier lifecycle request overwrote the current statement. Now
   statements keep the order of the writes they answer
   (`TestALateReplyDoesNotReplaceALaterStatement`).
5. Runtime roots kept from a reply stayed proof after a turn could replace them. Now no
   roots are kept; the current ones are read (`TestRootsAReplyStatedAreNotCurrent`,
   `TestTheCurrentRootsDecide`).
6. The acknowledgment of `thread/settings/update` was taken for its application. Now the
   permissions are changing until `thread/settings/updated`, and the notice waits
   (`TestAnAcknowledgedSettingsUpdateLeavesPermissionsChanging` and its neighbours).
7. main was not told when a hold's cause changed under the same reason, and waits for idle
   or for the roots did not record the hold. Now the notice's key has the detail, and
   every wait is the binding's hold (`TestAChangedCauseIsToldAgain`,
   `TestTheStateDirectoryWaitsForAnIdleSession`, `TestUnreadableRootsAreTheBindingsHold`).

## Acceptance, round 2

review-codex closed findings 1, 3, 4, 5 and 7 of round 1 and found four more, each
confirmed by a probe that went red; the probes are kept, adapted, as the tests named below.
Three of them were the class of round 1's finding 6 again, so the rule came first: it was
written as the property the code held, "Permissions known", in delivery-conversation.md,
and the three windows were closed by following it rather than one by one. The rule went
out with the check after round 3.

1. A failed write of the admission record left the hold in place, and still let a
   delivery into the intended conversation, while the inbox stayed closed and main was not
   told. Now the intended conversation selected before the record is written is the hold
   `mail-closed`, naming the error, and every publication tries the write again
   (`TestTheLiftAdmitsTheMailFirst`).
2. The statement of an earlier update ended a later one. Now a statement ends the open
   requests only when it holds what they ask, taken together
   (`TestAnEarlierStatementDoesNotEndALaterUpdate`).
3. A lifecycle reply to a request sent after the acknowledgment was taken for the update's
   application; a resume reads the settings as they are. Now such a reply ends a change
   only when it holds what was asked (`TestAResumeDoesNotProveAQueuedUpdate`); an update
   never stated ends with a turn the terminal starts after its acknowledgment, since the
   session takes that turn only after the update
   (`TestATurnAfterTheAcknowledgmentEndsAnUnstatedUpdate`).
4. Policies compared as raw JSON, so one spelled with its defaults read as another, and an
   acknowledgment after the statement opened the change again for good. Now policies
   compare as typed values with their defaults, a change is open from the request rather
   than the acknowledgment, and an acknowledgment opens nothing
   (`TestAStatementBeforeTheAcknowledgmentEndsTheChange`).

The class is `TestTwoUpdatesInEveryOrder`: two updates in a row, answered in every order
the server may use — acknowledgments on either side of the statements, a first update
refused, a second changing nothing, a policy spelled otherwise, and a resume reading any of
the states on the way — and at no point may a delivery go with permissions the queue does
not leave. `TestATurnAskingForPermissionsEndsWithItsStart` and
`TestARefusedUpdateEndsItsChange` cover a `turn/start` that asks and a refused update.

## Acceptance, round 3

review-codex accepted the fix of round 2's first finding — a failed admission write is the
hold `mail-closed`, and each publication tries the write again — and did not accept the
permission tracking, with three findings, each confirmed by a probe that went red:

1. A statement that matches what the open requests ask does not prove the queue is
   through: three updates, read-only, full access, read-only, were all ended by the first
   one's notification, and the next, full access, read as known while the last read-only
   was still queued; an old resume snapshot equal to the start ended a queue the same way.
2. A turn's id does not prove a new start: `turn/start` into a running turn answers with
   that turn's id (`turn_processor.rs:672–715`, 0.159.0), so an earlier `turn/started` of
   it ended a change it did not follow.
3. The permissions stated are those of the turns to come; a running turn keeps the ones it
   started with (`core/src/session/mod.rs:1898–1899`, `session/turn_input.rs:193–201`),
   and a delivery into it runs under those.

The owner decided on September 30, 2026 to take the mail-reach check out rather than go on
closing windows the protocol does not state: mail will not depend on the sandbox, the
wrapper doing its writes outside it ([work-queue.md](../work-queue.md#now-after-100), the
bullet "Mail that does not depend on the sandbox"). Stage A keeps what review-codex
accepted — the launch's intent, the hold and `rewake accept`, the admission record with the
`mail-closed` hold, the notices and the terminal's warning, the refused resume held across
connections — and lost the check before delivery, the permission tracking kept only for
it, the root it added, and their tests. The source facts read for it stay true and stay in
[research-codex-conversation.md](../research-codex-conversation.md).

## Tests

Each part has a test that goes red without it, checked by removing the part:
`gateway/intent_test.go` (the incident replayed at the gateway: a resume refused with
`already has an active writer`, a `thread/start`, a reservation held; `/new` held; accept
and the intended resume lifting; pinning for `--last`; the warning once),
`codex/resume_intent_test.go`, `cli/gateway_intent_test.go` (the incident through the
inbox: the task pending with both conversations named, nothing unread, no delivery thread,
delivered into the accepted one), `cli/accept_test.go` (the command, its refusals, the
inbox refusal, `rewake list`), `wrap/hold_notices_test.go`.

The five checks green; the workflow suite against the schema from Codex 0.159.0: 137
scenarios, 163 cases, 160 pass, 3 unsupported as before — the same after the fixes of
acceptance rounds 1 and 2, and after the mail-reach check was taken out.

## What stays open

- A conversation whose sandbox closes rewake's state directory still takes the notice, and
  its worker cannot read the mail: mail that does not depend on the sandbox is the next
  change.
- The `warning` notification is read in the source, not seen in a live terminal.
- The live chain of the incident — what started the new conversation — is not established.
- Not accepted live on Codex yet; the Codex-side acceptance is for the review chain.
