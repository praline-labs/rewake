---
description: Acting on a message already sent — rewake withdraw, rewake edit and rewake send --to — before and after the reader saw it, with the recall note, tombstones, addenda and what is final.
---

# Actions on a sent message

What a sender can still do with a message after `rewake send` has accepted it: take it back
while nobody has read it (`rewake withdraw`), replace its text (`rewake edit`), or add to a
task already under way (`rewake send --to`). All three name the message by its id, which
`send` prints on its own line, and every refusal carries a ready command line, so they are
convenient for a main session.

## Who, and by which id

Only the run that sent a message acts on it: `from` is the caller's session and `fromEpoch`
its run, in any role. A message from a plain shell has no run and one from an earlier run of
the name is out of reach; a call outside a session is refused with exit 2. The role adds
nothing: other senders' mail is out of reach either way, and workers delegate too. The
environment is the same trust boundary as `from` on a send.

A reference is the full id, a prefix of it, or a prefix of its random tail — the part after
the dash, which is what `--awaited` prints for an addendum and what a person copies, as with
git's short hashes. It takes at least four characters (`mail.MinReference`), and an exact id
always wins. Only this run's tasks, questions and notes are searched, tombstones included, in
every mailbox of the room and every stage (`inbox/`, `unread/`, `done/`); reports and rewake's
own notices, the recall below among them, are not a sender's letters. No match is exit 1
pointing at `rewake inbox --awaited`; more than one lists each candidate — id, kind and
recipient, time, first line, and a tombstone as `task (withdrawn)` without its tombstone text
— and ends with the command as a ready line whose `<id>` stays a placeholder, so an agent
copying it cannot act on whichever candidate happened to sort first.

## Withdraw (`rewake withdraw <id>`)

Under the recipient's mailbox lock, waited for as long as a reader waits. The readable copy in
`unread/` is made before any notice ([delivery.md](delivery.md#servicing-process-the-wrapper)),
which decides the outcome:

- **No readable copy** — no notice went out and the recipient saw nothing. The waiting copy
  goes, the tombstone is archived in `done/`, and the answer says `before its notice went
  out; <name> saw nothing.`
- **A readable copy** — a notice may have gone out: the copy is made before the notice, and
  stays after a notice the harness refused or the last check stopped. Its preview can itself
  be an instruction. Removing the text silently would leave the agent with a preview it cannot
  check, so a tombstone takes its place in `unread/` under the same id: the message rewritten
  as kind `notify`, with `withdrawn: {kind, at}` and the text `Rewake: <sender> withdrew its
  <kind> of HH:MM:SS before you read it; disregard its notice.` A note owes nothing, so
  reading the tombstone records no wait and nothing is reported. The answer says `its notice
  may have gone out`.

  And the recipient is told: a note from the sender's own run, marked `recall: {id, kind}`,
  `Do not act on <kind> <short id> from <sender> (HH:MM:SS): withdrawn unread.` An agent may
  carry out a preview without reading its inbox at all, so the recall goes out as a notice of
  its own, at once, outside the coalescing window. The answer's `recall` field names it; a
  failure to write it is said on a line of its own with the `send --notify` to send instead.

  **How a notice shows it.** A notice otherwise previews one letter, the newest, and a recall
  could be lost behind a newer letter in the same group, behind a second recall, or cut by the
  96-cell bound. So every recall in a notice gets a line of its own, placed first, oldest
  first (`mail.Notice`); the newest other letter keeps its line after them; and the text leads
  with the instruction, so the bound cuts the sender and the time, never which message not to
  act on. An edit's replacement is shown the same way, below.

  **What a recall can and cannot do.** It stops work that has not happened yet. On Claude Code
  a notice reaches a running turn only at a tool-call boundary, so the model sees the recall
  after the step in progress; a step already taken from the preview — a one-command task done
  in one go — is not undone, and a turn that ended first gets the recall as a turn of its own
  ([research.md](research.md#a-notice-during-a-running-turn)). On pi the recall is steered in
  after the running tool batch. Nothing makes a withdrawal after the notice safe for work that
  cannot wait; a task whose first line is itself a destructive instruction is better withdrawn
  before its notice, or not written that way.
- **Held** on Claude Code — the line waits in the harness's approval queue, which rewake cannot
  empty. The tombstone replaces the readable copy all the same, and the answer says the line
  is still queued: approved later, it leads to the tombstone. No recall is sent: the agent has
  not seen the preview, and the recall would queue behind it.
- **Read** — final. Exit 1: the text is with the agent, and the answer gives the ready line for
  an addendum, `rewake send <name> "..." --to <id>`.
- **Failed** or no longer kept — exit 1, nothing to take back. Withdrawn already — exit 0, said
  so.

**A task goes with its addenda.** An addendum is a task that only makes sense beside the one it
adds to; left standing after it, it would be read on its own and owe a report on work its
sender took back. So under the same lock, once the task is withdrawn — or found withdrawn
already, which lets a retry finish the rest — each of its addenda from the same run is
withdrawn the same way (`mail.AddendaOf`, which follows edits, below), and the answer adds a
line for each: `withdrew its addendum <id> too`, with the recall its own notice calls for. A
read addendum is final like any read message: it stays owed, the answer says so, and the task
is recalled for it even when the task's own notice never went out, since the addendum named
it. `--json` lists them under `addenda`, each with its `result` — `unseen`, `announced`,
`held`, `already`, `read`, `undelivered` or `failed` — and its `recall`; one that did not
finish makes the exit 1 and names the withdraw that finishes it.

The status is written first, before the tombstone: `failed`, detail `withdrawn by <sender>`,
`withdrawn: true`. `failed` is deliberate: whoever already waits on the message behaves
correctly — a blocked `send --question` exits 1, `--awaited` stops listing it — and no new
state has to be taught to every reader.

**A withdrawal that fails halfway.** With the status written and the tombstone not, the
original stays in `unread/` by hard link, and a reader must not be served the task its sender
withdrew. So the reader shows any message whose status says withdrawn as its tombstone
(`PeekUnread`), and archives the tombstone rather than the original; `MarkRead` writes no
`read` status and no wait record over a withdrawn status on disk, whatever copy it was handed;
and only a finished withdrawal answers `already`: while `unread/` holds the original, or the
waiting copy is there with no tombstone written, the next withdraw or edit does the steps
again. The refusal says how far it got, from the status on disk: marked withdrawn (the
recipient reads it so; finish with `rewake withdraw <id>`), or nothing changed. Removing the
waiting copy is not a step: once the tombstone is written a copy that will not go is left to
the serving process, which removes it as it settles the withdrawn status (`Locked.Settle`), and
the withdrawal counts as finished.

**Withdrawn is final, like read.** The notice is sent outside the lock, so a withdrawal can
land while it is on the way, and the harness's word can come after. From the moment the status
is written, nothing the server learns about the notice changes it: the recorded outcome keeps
`withdrawn` (`recordLocked`), settling it removes only the waiting copy and never archives the
tombstone (`Locked.Settle`), a message withdrawn before its notice is not made readable again
and a group notice whose member was withdrawn is rebuilt (`prepareDelivery`,
`validAnnouncement`), a late held or failed word leaves it withdrawn (`takeBack`, `stillHeld`,
the batch pass), and a hold that ends after the withdrawal sends its sender no "not
delivered" note (`settleHeld`, `tellUndelivered`, which reads the status from disk). Reading
the tombstone moves it to `done/` and writes no `read` status over the withdrawn one.
Internally the server carries the outcome as the unwritten state `withdrawn` (`Status.Final`,
`IsFinal`), so every place that already stopped at `read` stops here too.

What cannot be closed: between the last check before a notice and the write into the socket
there are milliseconds, and a notice already handed to a harness cannot be recalled. The
tombstone and the recall note are the answer to that notice, within what a recall can do.

## Edit (`rewake edit <id> <text>`)

A withdrawal and a new letter of the same kind under one lock, never an edit in place: the
message may be a hard link in two directories, the preview has shown its first line already,
and read is final only while an id's text never changes. The old id becomes a tombstone with
`withdrawn.replacedBy` and the text `Rewake: <sender> replaced its <kind> of HH:MM:SS before
you read it; disregard its notice and read the replacement, <id>.`; the new letter carries
`replaces: <old id>` and keeps the kind and any `addendumTo`. It is announced as any letter
is, at once whatever its kind (`canWait`), with its own preview, and the reader finds the
tombstone and the replacement together, in id order. The replacement is written first, under
the same lock, so the server cannot make it readable before the tombstone is in place; if the
withdrawal then fails, the replacement is removed. So a failed edit leaves either the old
message as it was or — status written, tombstone not — the old one withdrawn and nothing in
its place, which the refusal says, with `rewake edit <id> "..."` to finish; it never leaves a
tombstone pointing at a replacement that was not sent.

An edit sends no recall: a recall and a replacement in one group would preview the recall,
and an agent reading "withdrawn" could end its turn without reading the replacement. The
replacement's own preview carries it instead: it begins `Replaces <short id> (withdrawn): `
and goes on with the replacement's first line (`mail.Notice`), so one line both sets the old
work aside and shows the new. Like a recall, it gets a line of its own, first, in a group:
`<sender> <kind>: Replaces <short id> (withdrawn): <first line>`. The answer is a send's, with the new id on
its own line and the same exit codes; a question edit waits for its answer as `send
--question` does, and the old blocked send exits 1. `-` reads the text from stdin. What
refuses a withdrawal refuses an edit, and so does a message withdrawn already or a recipient
run that has ended. An addendum's replacement adds to the same task, and the checks of `--to`
are asked again first, under the recipient's mailbox lock, as for an addendum (below): the
task may have been reported on since.

**An edited task keeps its addenda.** They are not rewritten — a letter's content never
changes once written — and go on naming the old id; the old id's tombstone names the
replacement (`withdrawn.replacedBy`), and every place that asks what an addendum adds to
follows that link, through as many edits as there were (`mail.CurrentTask`): `--owed`,
`--awaited`, a plain read and `--peek` show the addendum under, or as adding to, the
replacement, and `--to` given either id adds to the replacement. The answer names them — `your
addendum <id> now adds to <new id>`, with the withdraw that takes it back — and `--json` lists
them under `addenda`. Withdrawing the edit's replacement later takes them along. Withdrawing
them with the old text instead would make an edit silently undo corrections the sender still
wants; one withdraw per addendum is the cheaper mistake to fix.

**The old id names the replacement.** After an edit the id main was printed first is still the
name it has for the task, so every action by it acts on the letter that stands now, through as
many edits as there were (`currentSent`): `rewake withdraw <old id>` withdraws the replacement
with its addenda, and `rewake edit <old id>` replaces the replacement. The answer says so first
— `Rewake: <old id> was replaced by <new id>; withdrawing <new id>.`, or `editing` — and
`--json` gives the letter acted on as `id` for a withdrawal and `replaces` for an edit, with
the id the call named as `named`. The withdrawal looks the replacement up under the
recipient's mailbox lock, so the letter it finds is the one it withdraws. A replacement
withdrawn without a new one is withdrawn already, by either id.

**What a withdrawal says of a read addendum.** A task whose notice never went out but whose
addendum was read is not one the recipient "saw nothing" of: the first line says it has read
an addendum to it, so it is told not to act on the task. Withdrawn a second time, the read
addendum's line says the recipient was told when the task was withdrawn, since no second
recall goes out.

## Addendum (`rewake send <name> <text> --to <id>`)

A separate letter of kind `task` with `addendumTo: <root id>`; the task itself is never
touched, since it may be read and final. An addendum to an addendum is filed under the root. A
task, not a note, on purpose: it is announced at once, and read into the same `awaiting/`
record of the same sender run, so one report's `inReplyTo` settles the task and every addendum
read with it. An addendum read after that report is an obligation of its own until the next
turn end; a report never waits for addenda still on the way.

It is refused with exit 1, each with a ready line, when the task went to another recipient, is
withdrawn, is a note, was already reported on (send it as a new task), failed, or its
recipient's run has ended; and with exit 2 beside `--notify` or `--question`. An id that
`rewake edit` replaced adds to its replacement.

The checks are made twice: once without a lock, for a quick refusal, and again under the
recipient's mailbox lock, with the addendum written under the same hold. The turn end that
writes a report holds that lock too, so a report lands either before the second look, which
then refuses, or after the addendum is in the mailbox. An addendum in the mailbox before the
report is read with the task and settled by it, or read after it and owed on its own, as
above.

How it is shown: plain `rewake inbox` and `--peek` add `· addendum to <id>` to the heading
(and an edit's replacement `· replaces <id>`); `--owed` places it under its task as
`+ addendum from <sender> · HH:MM:SS`, and `--awaited` as `  + <id> · addendum to <short id> ·
…`, or, when its task is no longer listed, on its own as `task, addendum to <short id>`
([delivery-owed.md](delivery-owed.md)). End to end, the workflow scenarios
`withdraw-after-notice`, `edit-after-notice` and `addendum-owed` check the notice the worker
was shown ([testing-cases.md](testing-cases.md#actions-on-a-sent-message)).
