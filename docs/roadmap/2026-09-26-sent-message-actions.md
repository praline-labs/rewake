# Actions on a sent message — September 26, 2026

The owner approved on September 26, 2026 one feature made of two queued ideas: an
addendum to a task already sent, kept from the dropped todo list, and withdrawing or
editing a message nobody has read, the owner's idea of September 23. The condition was
that it be convenient for a main session. The design came from a research round on the
review side, with the mechanism of every step located in the code; it was built as
proposed, in two parts. Mechanism: [delivery-sent.md](../delivery-sent.md).

**What was built.**

- `rewake withdraw <id>` takes back a message while it is unread, under the recipient's
  mailbox lock. With no readable copy yet nothing was announced, and the message goes
  silently. With one, a notice may have shown a preview, so a tombstone takes its place
  under the same id — a note with a `withdrawn` field saying who withdrew which kind —
  and a recall note tells the recipient at once not to act on the notice. A held
  message's line stays in Claude Code's approval queue, and the answer says so. A read
  message is final and the refusal gives the addendum's command line.
- `rewake edit <id> "..."` is a withdrawal and a new letter of the same kind under one
  lock, the replacement written first: the tombstone names its replacement, the
  replacement names what it replaces, and the answer is a send's with the new id. A
  question edit waits for its answer, and the old blocked send exits 1.
- `rewake send <name> "..." --to <id>` sends an addendum: a task with `addendumTo`,
  filed under the root task, announced at once and read into the same wait record, so
  one report settles the task and its addenda. `--owed` nests it under its task,
  `--awaited` lists it after its task, and a plain read marks its heading.
- For a main session: `send` prints the id on its own line, a question before it waits;
  the three commands take a unique prefix of the id or of its random tail, from four
  characters; an ambiguous one lists the candidates and keeps `<id>` in its ready line;
  every refusal names the next command as a ready line; the guide's examples show all
  three.

**Decisions.**

- A withdrawn message's status is `failed` with `withdrawn: true`: everything already
  waiting on a message treats `failed` correctly, and no new state is taught to every
  reader. It is final like `read` wherever a status is written — the recorded outcome,
  the settling, the last checks before a notice, the late words about a hold, and the
  "not delivered" note an expired hold would send. That finality was the main risk of
  the change, and the unit tests race each of those writers against a withdrawal.
- An edit is never an edit in place: the preview already showed the old first line, the
  message may be a hard link in two directories, and read is final only while an id's
  text never changes.
- Only the same run of the sender acts on a message, in any role; shell letters and an
  earlier run's are out of reach. An addendum from anyone but the task's sender is
  refused, since wait records are kept per sender.
- A withdrawal whose notice may have gone out also sends the recipient a recall note,
  announced outside the coalescing window. The design had none: the notice leads to
  `rewake inbox`, where the tombstone is. In a live run of the review an agent carried
  out the preview of a withdrawn task and never read its inbox, and main decided on
  September 26, 2026 that the recall goes out, and only then: a withdrawal before the
  notice stays silent, and a held one sends none.
- Not built, by the design: withdrawing someone else's mail, removing a line from Claude
  Code's approval queue, a report that waits for addenda still on the way, addenda to
  notes or reports, new message kinds.

**Tests.** Unit tests in `internal/inbox/withdraw_test.go`: a hold ending after a
withdrawal (released, expired, session ended), a late held or failed word, a withdrawal
during the notice, the last check before a notice, reading a tombstone, an edit under
one lock, and prefix resolution restricted to this run. `internal/cli/withdraw_test.go`
covers prefixes, the ambiguous listing, the read refusal pointing to `--to`, another
run's message, edit, addendum and the nested `--owed`. Mutants of `recordLocked`,
`settle`, `tellUndelivered`, `MarkRead` and `validAnnouncement` were each caught.

The workflow cases `withdraw-after-notice`, `edit-after-notice` and `addendum-owed` run
in both columns. A new fixture switch, `RW_SHIM_READ_GATE`, stops a worker between the
start of its turn and its read, which is the moment these commands exist for. Five
product mutants, on the Claude Code column: a withdrawal that leaves the task readable
and its status untouched, a withdrawal that sends no recall, an edit that sends the old
text again, a tombstone that does not name its replacement, and an `--owed` that does
not nest. Each breaks exactly the observations it names. A general session on Claude
Code is idle only after its report, so a compaction with a task still owed does not
arise there live; the fixture has the worker run `--owed` in its turn instead. Two
controls of earlier scenarios, `awaited-never-settled` and `replay`, had their mutant
text moved to follow a refactoring of the code they edit, and break what they broke
before.

**Review of the core.** review-claude reviewed the core as of 11:26 in a copy, with a
live run on Claude Code 2.1.280. Finality held against every late writer, with no double
notice and no "not delivered" after a withdrawal. Found and fixed:

- A withdrawal that failed after its status left the original readable by hard link; a
  retry answered `already`, and the reader was served the task and wrote `read` and a
  wait over the withdrawn status. The reader now shows a message with a withdrawn status
  as its tombstone, `MarkRead` never writes over one, and only a finished withdrawal
  answers `already`: the next call does the rest. The refusal says how far it got.
- An edit wrote its tombstone, already naming the replacement, before the replacement;
  a failed write left a withdrawal pointing at nothing, and the refusal offered `rewake
  withdraw`. The replacement is now written first under the same lock and removed if the
  withdrawal fails; the refusal offers `rewake edit`.
- The ambiguous-prefix refusal filled the first candidate's id into its ready line — a
  tombstone, live. It keeps `<id>`, and a tombstone is listed as `task (withdrawn)`.
- The live run behind the recall, above.
- Low: a question printed no id (now before it waits); `--awaited` showed an addendum
  whose task was reported on as a plain task; `--peek` showed no link; "had gone out"
  became "may have gone out"; an edit of an addendum asks the checks of `--to` again.

Each of the first four came with a failing test first: `withdraw_failure_test.go` and
`recall_test.go` in `internal/inbox`, and the ambiguous line and the recall in
`internal/cli/withdraw_test.go`.

**What stays open.**

- A mutant of the check in `prepareDelivery` survives: without it, making the message
  readable fails on the missing waiting copy and the outcome is the same. The check is
  a second guard, not the only one.
- Between the last check and the write into a harness there are milliseconds, and a
  notice already handed over cannot be recalled by either harness's protocol; the
  tombstone and the recall note are the answer to such a notice.
- A recall stops only what has not been done: on Claude Code it reaches a running turn at
  a tool-call boundary, and a step already taken from the preview stays taken.
- Acceptance on the Codex side is still to come, since the delivery server and the
  Codex path are touched.

**Final review.** review-codex, with a probe of a real Codex mailbox notice, and
review-claude, live on Claude Code 2.1.280, converged on the notice the model sees. A
notice previewed only its newest letter, so a recall vanished behind a newer notify
(`Rewake: 2 new messages ↳ peer notify: build finished`) or behind a second recall; its
first wording, led by `Rewake: <sender> withdrew…`, was cut by the 96-cell bound before
its point; a Codex member carried the recall's id but not the withdrawn task's; and an
edit sent its recall in the same group as the replacement, whose preview then showed
the recall — the agent read "withdrawn", ended its turn and never read the replacement,
while the edit exited 0. Live, a notice reaches a running Claude Code turn only at a
tool-call boundary, and a one-step task done from the preview was not undone
([research.md](../research.md#a-notice-during-a-running-turn)). Main decided on
September 26, 2026, and it was built so:

- Every recall in a notice has a line of its own, first, on both harnesses; its text
  leads with the instruction, `Do not act on <kind> <short id> from <sender>
  (HH:MM:SS): withdrawn unread.`; a Codex member of a recall carries `recalls: <id>`.
- An edit sends no recall. Its replacement previews as `Replaces <short id>
  (withdrawn): <first line>`.
- What a recall can and cannot do is written down in the mechanism.
- Once the tombstone is written, removing the waiting copy is not a step of the
  withdrawal: the serving process clears a copy left behind, and a finished withdrawal is
  no longer sent back to a call that would be refused. The open item about a tombstone
  naming a replacement taken back is closed with it.

Tests: `notice_recall_test.go` in `internal/harness` (a recall beside a newer letter, two
recalls, a long one, a replacement), a Codex member test, the archive after a halfway
read (whose mutant on `archiveTombstone` had survived) and a waiting copy that will not
go in `withdraw_failure_test.go`, and an edit that sends no recall. End to end, the three
cases check the notice the worker was shown, the new `withdraw-mid-turn` sees the recall
steered into a held Codex turn, and three new mutants — recall-sender-first,
recall-unnamed, replacement-unmarked — each break only their own observation.

**Last round.** review-claude found nothing, live: the replacement is now seen and
done, and a recall stops work at the tool-call boundary. It named one surviving mutant:
removing the recall's line from a single delivery whose notice shows a newer unread
letter went uncaught. review-codex accepted the recall and found the replacement lost
the same way the recall had been: with a newer notify in its group the notice showed only
`Rewake: 2 new messages ↳ peer notify: build finished`, and no member linked the
replacement to the withdrawn id. Main decided on September 26, 2026 to generalise, and it
was built so: every correcting letter, a recall or a replacement, gets a line of its own,
first, oldest first, in a group and in a single delivery that previews a newer letter;
a replacement's line is `<sender> <kind>: Replaces <short id> (withdrawn): <first
line>`, and its Codex member carries `replaces: <id>`. An edit still sends no recall.

Tests: in `notice_recall_test.go` a replacement beside a newer letter, two replacements,
a recall with a replacement, and a single correction beside the newest unread, which kills
review-claude's mutant; a Codex member test for `replaces`, which fails without the
field. An end-to-end neighbour was not added: the scenario's replacement is a task, which
goes out at once rather than waiting in a group, so a newer notify arrives in a notice of
its own and would prove nothing (tried, both columns). `docs/testing.md` had passed 400 lines; the
case-by-case claims moved to `docs/testing-cases.md`.
