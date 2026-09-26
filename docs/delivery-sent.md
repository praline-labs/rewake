# Actions on a sent message

What a sender can still do with a message after `rewake send` has accepted it: take it
back while nobody has read it (`rewake withdraw`), replace its text (`rewake edit`), or
add to a task already under way (`rewake send --to`). All three name the message by its
id, which `send` prints on its own line. The owner approved the design on September 26,
2026, on the condition that it is convenient for a main session; the ids, the short
forms and the ready command lines in every refusal come from that condition.

[Back to the delivery document](delivery.md).

## Who, and by which id

Only the run that sent a message acts on it: `from` is the caller's session and
`fromEpoch` its run, in any role. A message from a plain shell has no run and one from an
earlier run of the name is out of reach; a call outside a session is refused with exit 2.
The role adds nothing: other senders' mail is out of reach either way, and workers
delegate too. The environment is the same trust boundary as `from` on a send.

A reference is the full id, a prefix of it, or a prefix of its random tail — the part
after the dash, which is what `--awaited` prints for an addendum and what a person copies,
as with git's short hashes. It takes at least four characters (`inbox.MinReference`),
and an exact id always wins. Only this run's tasks, questions and notes are searched,
tombstones included, in every mailbox of the room and every stage (`inbox/`, `unread/`,
`done/`); reports and rewake's own notices, the recall below among them, are not a
sender's letters. No match is exit 1 pointing at `rewake inbox --awaited`; more than one
lists each candidate — id, kind and recipient, time, first line, and a tombstone as
`task (withdrawn)` without its tombstone text — and ends with the command as a ready line
whose `<id>` stays a placeholder. It once carried the first candidate's id, and a live
run on September 26, 2026 showed that candidate to be a tombstone: an agent copying the
line would have acted on whichever message happened to sort first.

## Withdraw (`rewake withdraw <id>`)

Under the recipient's mailbox lock, waited for as long as a reader waits. The readable
copy in `unread/` is made before any notice (delivery.md, the servicing process), which
decides the outcome:

- **No readable copy** — no notice went out and the recipient saw nothing. The waiting
  copy goes, the tombstone is archived in `done/`, and the answer says `before its notice
  went out; <name> saw nothing.`
- **A readable copy** — a notice may have gone out: the copy is made before the notice,
  and stays after a notice the harness refused or the last check stopped. Its preview can
  itself be an instruction. Removing the text silently would leave the agent with a
  preview it cannot check, so a tombstone takes its place in `unread/` under the same id:
  the message rewritten as kind `notify`, with `withdrawn: {kind, at}` and the text
  `Rewake: <sender> withdrew its <kind> of HH:MM:SS before you read it; disregard its
  notice.` A note owes nothing, so reading the tombstone records no wait and nothing is
  reported. The answer says `its notice may have gone out`.

  And the recipient is told: a note from the sender's own run, marked `recall: {id, kind}`,
  `Do not act on <kind> <short id> from <sender> (HH:MM:SS): withdrawn unread.` The
  design had no second notice, on the reasoning that the first leads the agent to
  `rewake inbox` and the tombstone; a live run on September 26, 2026 showed an agent
  carrying out the preview without reading its inbox at all, and main decided the same
  day that the recall goes out. It is announced at once, outside the coalescing window.
  The answer's `recall` field names it; a failure to write it is said on a line of its
  own with the `send --notify` to send instead.

  **How a notice shows it.** A notice otherwise previews one letter, the newest, and both
  reviewers found the recall lost that way on September 26, 2026: behind a newer notify in
  the same group (a real Codex notice read `Rewake: 2 new messages ↳ peer notify: build
  finished`), behind a second recall, or cut by the 96-cell bound before its point, as
  the first wording, which led with `Rewake: <sender> withdrew…`, was. Main decided the
  same day: every recall in a notice gets a line of its own, placed first, oldest first,
  on both harnesses (`harness.Notice`); the newest other letter keeps its line after
  them; and the text leads with the instruction, so the bound cuts the sender and the
  time, never which message not to act on. On Codex each member entry of a recall carries
  `recalls: <id>`, the withdrawn message's id, beside the recall's own. An edit's
  replacement is shown the same way, below.

  **What a recall can and cannot do.** It stops work that has not happened yet. On Claude
  Code a notice reaches a running turn only at a tool-call boundary, so the model sees
  the recall after the step in progress; a step already taken from the preview — a
  one-command task done in one go — is not undone, and a turn that ended first gets the
  recall as a turn of its own (live, 2.1.280, [research.md](research.md#a-notice-during-a-running-turn)).
  On Codex the recall is steered into the running turn and reaches the model at its next
  step. Nothing makes a withdrawal after the notice safe for work that cannot wait; a
  task whose first line is itself a destructive instruction is better withdrawn before
  its notice, or not written that way.
- **Held** on Claude Code — the line waits in the harness's approval queue, which rewake
  cannot empty. The tombstone replaces the readable copy all the same, and the answer
  says the line is still queued: approved later, it leads to the tombstone. No recall is
  sent: the agent has not seen the preview, and the recall would queue behind it.
- **Read** — final. Exit 1: the text is with the agent, and the answer gives the ready
  line for an addendum, `rewake send <name> "..." --to <id>`.
- **Failed** or no longer kept — exit 1, nothing to take back. Withdrawn already — exit 0,
  said so.

**A task goes with its addenda.** An addendum is a task that only makes sense beside
the one it adds to; left standing after it, it would be read on its own and owe a report
on work its sender took back. So under the same lock, once the task is withdrawn — or
found withdrawn already, which lets a retry finish the rest — each of its addenda from
the same run is withdrawn the same way (`inbox.AddendaOf`, which follows edits, below),
and the answer adds a line for each: `withdrew its addendum <id> too`, with the recall
its own notice calls for. A read addendum is final like any read message: it stays owed,
the answer says so, and the task is recalled for it even when the task's own notice never
went out, since the addendum named it. `--json` lists them under `addenda`, each with its
`result` — `unseen`, `announced`, `held`, `already`, `read`, `undelivered` or `failed` —
and its `recall`; one that did not finish makes the exit 1 and names the withdraw that
finishes it. Chosen on September 26, 2026, from the health review's finding that
a withdrawn task left its addenda owing reports.

The status is written first, before the tombstone: `failed`, detail `withdrawn by
<sender>`, `withdrawn: true`. `failed` is deliberate: whoever already waits on the message
behaves correctly — a blocked `send --question` exits 1, `--awaited` stops listing it —
and no new state has to be taught to every reader.

**A withdrawal that fails halfway.** With the status written and the tombstone not, the
original stays in `unread/` by hard link. The review on September 26, 2026 found that
then a retry answered `already` while the reader was served the task and wrote `read`
over the withdrawn status, with a wait record behind it: the recipient did the work its
sender believed withdrawn. Three things close it. The reader shows any message whose
status says withdrawn as its tombstone (`PeekUnread`), and archives the tombstone rather
than the original. `MarkRead` writes no `read` status and no wait record over a withdrawn
status on disk, whatever copy it was handed. And only a finished withdrawal answers
`already`: while `unread/` holds the original, or the waiting copy is there with no
tombstone written, the next withdraw or edit does the steps again. The refusal says how
far it got, from the status on disk: marked withdrawn (the recipient reads it so; finish
with `rewake withdraw <id>`), or nothing changed. Removing the waiting copy is not a step:
once the tombstone is written a copy that will not go is left to the serving process,
which removes it as it settles the withdrawn status (`settle`), and the withdrawal counts
as finished. The second review found that counting it as a step sent the next call to a
refusal: the edit it suggested met the tombstone and was refused as withdrawn already.

**Withdrawn is final, like read.** The notice is sent outside the lock, so a withdrawal
can land while it is on the way, and the harness's word can come after. From the moment
the status is written, nothing the server learns about the notice changes it: the
recorded outcome keeps `withdrawn` (`recordLocked`), settling it removes only the waiting
copy and never archives the tombstone (`settle`), a message withdrawn before its notice
is not made readable again and a group notice whose member was withdrawn is rebuilt
(`prepareDelivery`, `validAnnouncement`), a late held or failed word leaves it withdrawn
(`takeBack`, `stillHeld`, the batch pass), and a hold that ends after the withdrawal
sends its sender no "not delivered" note (`settleHeld`, `tellUndelivered`, which reads the
status from disk). Reading the tombstone moves it to `done/` and writes no `read` status
over the withdrawn one. Internally the server carries the outcome as the unwritten state
`withdrawn` (`Status.final`, `isFinal`), so every place that already stopped at `read`
stops here too.

What cannot be closed: between the last check before a notice and the write into the
socket or `turn/start` there are milliseconds, and a notice already handed to a harness
cannot be recalled by either protocol. The tombstone and the recall note are the answer
to that notice, within what a recall can do (above).

## Edit (`rewake edit <id> <text>`)

A withdrawal and a new letter of the same kind under one lock, never an edit in place:
the message may be a hard link in two directories, the preview has shown its first line
already, and read is final only while an id's text never changes. The old id becomes a
tombstone with `withdrawn.replacedBy` and the text `Rewake: <sender> replaced its <kind>
of HH:MM:SS before you read it; disregard its notice and read the replacement, <id>.`;
the new letter carries `replaces: <old id>`, keeps the kind and any `addendumTo`, and
keeps `grantGit` only while the caller is still the main that could grant it. It is
announced as any letter is, with its own preview, and the reader finds the tombstone and
the replacement together, in id order. The replacement is written first, under the same
lock, so the server cannot make it readable before the tombstone is in place; if the
withdrawal then fails, the replacement is removed. So a failed edit leaves either the old
message as it was or — status written, tombstone not — the old one withdrawn and nothing
in its place, which the refusal says, with `rewake edit <id> "..."` to finish; it never
leaves a tombstone pointing at a replacement that was not sent.

An edit sends no recall. The first version did, and live on Claude Code 2.1.280 on
September 26, 2026 the recall and the replacement went out in one group whose preview
showed the recall: the agent read "withdrawn", ended its turn and never read the
replacement, while the edit had exited 0. Main decided the same day that the
replacement's own preview carries it instead: it begins `Replaces <short id>
(withdrawn): ` and goes on with the replacement's first line (`harness.Notice`), so one
line both sets the old work aside and shows the new. review-codex then found that line
lost the way the recall's had been: a replacement followed by a newer notify in one group
previewed only the notify. Main decided the same day that every correcting letter, a
recall or a replacement, gets a line of its own, first, oldest first: the replacement's
reads `<sender> <kind>: Replaces <short id> (withdrawn): <first line>`, and so does a single
delivery whose notice shows a newer unread letter. On Codex its member entry carries
`replaces: <id>`, the withdrawn message's id. The answer is a send's, with the new id on
its own line and the same exit codes; a question edit waits for its answer as `send
--question` does, and the old blocked send exits 1. `-` reads the text from stdin. What
refuses a withdrawal refuses an edit, and so does a message withdrawn already or a
recipient run that has ended. An addendum's replacement adds to the same task, and the
checks of `--to` are asked again first, under the recipient's mailbox lock, as for an
addendum (below): the task may have been reported on since.

**An edited task keeps its addenda.** They are not rewritten — a letter's content never
changes once written — and go on naming the old id; the old id's tombstone names the
replacement (`withdrawn.replacedBy`), and every place that asks what an addendum adds to
follows that link, through as many edits as there were (`inbox.CurrentTask`): `--owed`,
`--awaited`, a plain read and `--peek` show the addendum under, or as adding to, the
replacement, and `--to` given either id adds to the replacement. The answer
names them — `your addendum <id> now adds to <new id>`, with the withdraw that takes it
back — and `--json` lists them under `addenda`, since a correction the sender meant to
drop with the old text would otherwise go on being read. Withdrawing the edit's
replacement later takes them along. The other choice, withdrawing them with the old
text, would make an edit silently undo corrections the sender still wants; one withdraw
per addendum is the cheaper mistake to fix. Chosen on September 26, 2026.

**The old id names the replacement.** After an edit the id main was printed first is
still the name it has for the task, so every action by it acts on the letter that stands
now, through as many edits as there were (`currentSent`): `rewake withdraw <old id>`
withdraws the replacement with its addenda, and `rewake edit <old id>` replaces the
replacement. The answer says so first — `Rewake: <old id> was replaced by <new id>;
withdrawing <new id>.`, or `editing` — and `--json` gives the letter acted on as `id` for
a withdrawal and `replaces` for an edit, with the id the call named as `named`. The
withdrawal looks the replacement up under the recipient's mailbox lock, so the letter it
finds is the one it withdraws. Until review found it the same day, the old id met its
tombstone: withdraw answered `already withdrawn` with exit 0 while the replacement stood,
and edit refused with advice to send the task again, which would have left two. Decided
by main on September 26, 2026. A replacement withdrawn without a new one is withdrawn
already, by either id.

**What a withdrawal says of a read addendum.** A task whose notice never went out but
whose addendum was read is not one the recipient "saw nothing" of: the first line says
it has read an addendum to it, so it is told not to act on the task. Withdrawn a second
time, the read addendum's line says the recipient was told when the task was withdrawn,
since no second recall goes out.

An edit's replacement is announced at once whatever its kind, a notify's included
(`canWait`): it sends no recall, so its own preview is what sets the old notice aside.
Before September 26, 2026 a notify's replacement waited in the coalescing window like
any notify, up to four seconds.

## Addendum (`rewake send <name> <text> --to <id>`)

A separate letter of kind `task` with `addendumTo: <root id>`; the task itself is never
touched, since it may be read and final. An addendum to an addendum is filed under the
root. A task, not a note, on purpose: it is announced at once, and read into the same
`awaiting/` record of the same sender run, so one report's `inReplyTo` settles the task
and every addendum read with it. An addendum read after that report is an obligation of
its own until the next turn end; a report never waits for addenda still on the way.

It is refused with exit 1, each with a ready line, when the task went to another
recipient, is withdrawn, is a note, was already reported on (send it as a new task),
failed, or its recipient's run has ended; and with exit 2 beside `--notify`, `--question`
or `--grant-git`. An id that `rewake edit` replaced adds to its replacement.

The checks are made twice: once without a lock, for a quick refusal, and again under the
recipient's mailbox lock, with the addendum written under the same hold. The turn end
that writes a report holds that lock too, so a report lands either before the second
look, which then refuses, or after the addendum is in the mailbox. The health review of
September 26, 2026 found the second look missing: a report landing between the check and
the write let through an addendum the check existed to refuse. An addendum in the mailbox
before the report is read with the task and settled by it, or read after it and owed on
its own, as above.

How it is shown: plain `rewake inbox` and `--peek` add `· addendum to <id>` to the
heading (and an edit's replacement `· replaces <id>`); `--owed` places it under its task as
`+ addendum from <sender> · HH:MM:SS`, and `--awaited` as `  + <id> · addendum to
<short id> · …`, or, when its task is no longer listed, on its own as `task, addendum to
<short id>` ([delivery-owed.md](delivery-owed.md)).

## Tests

The finality races are unit tests beside the held ones, in
`internal/inbox/withdraw_test.go`; the halfway failures of a withdrawal and an edit in
`internal/inbox/withdraw_failure_test.go`, with the archive holding the tombstone after a
halfway read and a waiting copy that will not go; the recall's place outside the window in
`internal/inbox/recall_test.go`; the recall's lines and the replacement's in a notice in
`internal/harness/notice_recall_test.go`, and the member entries of a recall and of a
replacement in `internal/harness/codex/mailbox_test.go`; the references, refusals, the recall and the
edit that sends none in `internal/cli/withdraw_test.go`; a task withdrawn with its
addenda, a read addendum that stays, an edit's replacement keeping them and the second
look under the lock in `internal/cli/addendum_test.go`; withdraw and edit by an id an edit
replaced, and a second withdrawal that sends no second recall, in
`internal/cli/sent_current_test.go`; an edited notify announced at once
beside the recall in `internal/inbox/recall_test.go`. End to end,
`withdraw-after-notice`, `edit-after-notice` and `addendum-owed` run in both columns and
check the notice the worker was shown, and `withdraw-mid-turn` holds a Codex worker's
turn open and sees the recall steered into it ([testing-cases.md](testing-cases.md#actions-on-a-sent-message)).
