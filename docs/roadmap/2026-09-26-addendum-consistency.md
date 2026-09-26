# Addenda kept with their task through a withdrawal and an edit — September 26, 2026

A read-only health review of the code on September 26, 2026 found three places where an
addendum (`rewake send --to`) and the actions on a sent message disagreed with each
other. Main set the fix the same day; the behaviour is in
[delivery-sent.md](../delivery-sent.md).

## What was found

- **A withdrawn task left its addenda standing.** `rewake withdraw` of a task touched
  nothing else, so each unread addendum stayed a task naming a withdrawn one: read on its
  own, it owed a report on work its sender took back.
- **An edited task lost its addenda in the views.** They named the old id, so `--owed`
  and `--awaited` showed them apart from the replacement, and `--to` or an edit given
  either id was refused as adding to a withdrawn task.
- **The check before an addendum ran without the lock.** `send --to`, and `rewake edit` of
  an addendum, asked whether a report on the task could still come, then wrote without
  asking again; a report landing in between let an addendum through that the check
  existed to refuse.
- **An edited notify waited in the coalescing window.** `canWait` let only a recall skip
  it, so a notify's replacement, which sends no recall, waited up to four seconds.

## What was done

- **Withdraw takes the addenda along**, under the same lock, each the way the task is
  withdrawn, with its own recall where its notice may have gone out. A read addendum
  stays owed, and the task is recalled for it even when its own notice never went out.
  The answer and `--json` (`addenda`) say what became of each.
- **An edit keeps them**, without rewriting any: `inbox.CurrentTask` follows the old id's
  tombstone to its replacement, through every edit, for the views, `--to`, an edit of an
  addendum and a later withdrawal. The answer names the addenda that came along.
  Withdrawing them with the old text was the other choice; it would make an edit silently
  undo corrections the sender still wants.
- **A second look under the recipient's mailbox lock**, with the addendum or the edit
  written under the same hold. The turn end that writes a report holds that lock.
- **An edit's replacement skips the window** whatever its kind (`internal/inbox/window.go`).

## Evidence

Each behaviour has a test that fails without it, checked by mutation: without the second
look, `TestAnAddendumIsAskedAgainUnderTheLock` and
`TestAnAddendumsEditIsAskedAgainUnderTheLock` fail; without the cascade,
`TestWithdrawingATaskTakesItsAddendaAlong` and `TestAReadAddendumStaysAndItsTaskIsRecalled`;
without the task's recall for a read addendum, the latter alone; with `CurrentTask`
following nothing, or the views not asking it, `TestAnEditsReplacementKeepsTheAddenda`
(`internal/cli/addendum_test.go`); with `canWait` as before,
`TestAReplacedNotifyIsAnnouncedAtOnce` (`internal/inbox/recall_test.go`), whose notice went
out 3.15 s after it was written instead of at once.

## Review

review-claude accepted it with one change and two small ones, all made the same day:

- **The old id after an edit met its tombstone.** `send --to` by it reached the
  replacement, but `rewake withdraw` by it answered `already withdrawn` with exit 0 while
  the replacement stood, and `rewake edit` by it refused with advice to send the task
  again. Main decided that the old id names the replacement everywhere: withdraw and edit
  act on the letter that stands now (`currentSent`, `internal/cli/sent_current.go`), and
  the answer says so on its first line, with `named` in `--json`.
- **A withdrawal's lines contradicted each other.** A task withdrawn unseen whose addendum
  was read said the recipient "saw nothing" and then that it was told the task was
  withdrawn; it now says the recipient read an addendum to it. Withdrawn again, the read
  addendum's line no longer claims a recall that is not sent.
- **`delivery-sent.md`** said an edit given either id adds to the replacement; it now
  describes the old id naming the replacement for every action.

Each is a test that fails without it (`internal/cli/sent_current_test.go`, and the
contradicting line in `TestAReadAddendumStaysAndItsTaskIsRecalled`), checked by mutation:
with `currentSent` leading nowhere, both old-id tests and the edit test in
`withdraw_test.go` fail; with the old first line, the read-addendum test; with the old
line on a second withdrawal, `TestWithdrawingAgainSendsNoSecondRecall`.

## What stays open

Acceptance on the Codex side, since this touches delivery on both harnesses.
