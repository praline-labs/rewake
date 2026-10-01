# The CLI side of the mail tool

The first of three stages of [mail-bridge.md](mail-bridge.md), built September 30, 2026:
what the CLI does when it runs one tool call, and what changed for the shell with it.
The server (stage 2) is built and accepted; the launch injection (stage 3) is not, so nothing runs
under the tool yet: no harness starts the server, and a ticket the wrapper's context
endpoint did not issue is refused (below). Where the specification left a choice open, this document records the choice
and why; where it reads the specification one of two ways, it says which.

## The rules the code holds

The first acceptance of this stage (September 30, 2026) found eleven defects, most of
them breaking one of five properties; the second found the sixth missing, the fifth the
seventh, and the eighth the last; the ninth restated the last two as one invariant. They
are the rules below; each section after them says how the code keeps them. Rules 7 and 8
were restated on September 30, 2026, and the code keeping them in this form landed the
same day: [turn-end-recovery.md](turn-end-recovery.md) says how, record by record, and
[delivery-turn-end.md](delivery-turn-end.md) walks the path through the code.

1. **An operation whose effect is unknown is never discarded and never bypassed.** A
   receipt goes only when its effect is proven absent or proven done. A step not
   recorded as done is not proof it was not done: a crash between the effect and the
   journal leaves exactly that. Age decides nothing either: who may continue an
   operation is decided by its scope — the run, the native conversation and turn, the
   words — or by its receipt, never by how long ago it began.
2. **A lock is removed only by whoever holds it.** The sweep takes a record's lock
   before it removes the record and the lock file, and passes over a record whose lock
   another call holds; a call that took a lock checks that its file is still the one at
   the path, so a lock removed under it is taken again, not shared.
3. **Every check that decides an effect is made inside the critical section, right
   before the effect, and made again on retry.** The deadline, the letter's current
   state and version, the words' place on the tool's surface: a check made before a
   lock wait is stale by the time the lock is held, and a retry is a new call with its
   own deadline, its own transport and the same limits.
4. **A read's completion is one durable fact that every channel and every receipt
   shares.** A letter leaves `unread/` only by being read once a part of it was shown —
   nothing else may take a claimed letter out — and it never comes back. So a letter
   no longer in `unread/` is read, whoever read it; a late acknowledgment finds that
   and marks nothing, and never owes the report a second time.
5. **The size cap holds on the final encoded bytes of every answer, checked in one
   place.** Every branch — a part, a refusal, a diagnostic, the failure to keep an
   output — goes through the same bound before it leaves. Every text part names its
   letter id, its part and count, and its byte range.
6. **Every check that decides an effect has three outcomes: found, proven absent,
   unknown.** Only a lookup that answered "no such file" proves absence. Anything else
   — an I/O error, a directory that cannot be listed or searched, a record that cannot
   be read, an earlier effect recorded as uncertain — is unknown, and unknown always
   stops the effect and leaves the operation where a retry can take it up; it is never
   read as absence. The second acceptance (September 30, 2026) found this sharpening of
   rules 1 and 3 missing in three places: an uncertain heads-up that the shell could
   run again, a mailbox that could not be searched read as a letter never written, and
   a version check that failed read as a version unchanged.
7. **Every effect has an immutable identity and a proven scope, and recovery advances it
   only from durable evidence tied to that identity.** A turn end is one operation of
   several effects — publish its reports, take the kept answer they carry, clear the
   waits they answer, record whether the end was interim — and it is named by its cause,
   never by an attempt: its run and its event. Everything it writes is named from that —
   its journal, its reports, a notice about it — so a retry is the same operation, and a
   record found under that name is its own. Its scope is fixed once, before its first
   effect, and only from evidence the event itself carries. A read boundary is a
   position on the one sequence that numbers every read and every hold of the run (the
   read clock): what was read or kept at or below it is this end's, and nothing above it
   is. The turn's window on the boot clock picks the pending marks that make the end
   interim; it opens after the turn's start or after the latest earlier end of the run
   that a journal records, whichever is later, so a start that failed to be recorded
   widens nothing, and where two windows still overlap a mark makes both interim,
   closing nothing. The time it ended orders its interim record among the same run's
   other ends, never another run's. Nothing an end relies on is destroyed by another
   end: no end removes a pending mark, which is kept for its run's life, so a retry
   finds the marks its first attempt saw. An end heard once — a hook that names no event
   — has no retry, and its one attempt, under the mailbox lock, is its scope. An end
   that names an event and carries no boundary takes no effect on any attempt: a retry
   could not tell a record the first attempt wrote from one it wrote itself after the
   first failed, and what is there now is no evidence of what the first attempt saw; its
   waits stay owed. What the operation is about to do is written down before its first
   effect (the journal), and nothing but the journal publishes. The owner decided on
   September 30, 2026 that a stop after an Esc goes on taking its held answer. A hold
   therefore takes its place on the read clock as a read does, all under the mailbox
   lock: it reserves the next position durably, keeps the answer with that position, and
   only then commits the clock that boundaries are captured from. A position is never
   issued twice, even when the answer could not be kept, and a boundary never covers a
   hold that is not yet kept. The fifth acceptance (September 30, 2026) found the
   journal written after the reports, and a journal of an ended run dropped while its
   waits were taken over; the sixth, a prepared receipt that could still publish on its
   own; the seventh, the interim end recorded outside the journal, and a retry whose
   first attempt recorded nothing answering a question read since; the eighth, a retry
   whose receipt was never written doing the same, and a retry taking an answer kept
   after its first attempt; the ninth, a retry whose receipt and journal were both lost
   taking the answer kept now, with a read boundary that ordered reads but not holds, a
   late receipt fixing a retry's scope at its own moment, a later end removing the mark
   an earlier end's retry relied on, and a start never recorded letting later ends take
   a mark already used.
8. **Each effect is proven done, proven not done, or unknown; an unknown stops every
   change to its mailbox until evidence or a person settles it; and every record of a
   mailbox, in every format the code still reads, is reconciled before any of them is
   replayed.** Done is proven only by evidence written after the effect under the
   operation's name: the journal's entry, the recipient's `published` mark, the letter
   itself in the recipient's mailbox, a journal completed, and the done mark of an
   earlier build's receipt for the reports that receipt lists. Not done is proven only
   where a protocol makes absence visible: a report of the journal protocol with no
   mark, or an `intent` mark and no letter, since the mark is written before the letter
   and settled before the letter is swept; a kept answer or an interim record still in
   place under the identity the journal names. Everything else is unknown: a record
   without its done mark, a letter or mark missing for a report written before marks
   existed, an age, a record that cannot be read, a state that is none of the known
   ones. Neither a retry, nor converting a record to a newer format, nor a text found or
   not found settles an unknown. An unknown stops every change to the mailbox, as a
   receipt that cannot be read stops it: the records are kept, nothing is published,
   cleared, adopted or read from it, and the stop names its exact cause. Three changes
   go on, deciding no effect: letters from others arrive, recovery writes its own
   records — the stop, the conversion, a person's decision — and main is told. What a
   stop is decided by is read before any effect, by the code that decides it: the
   barrier, and every call that would change the mailbox, first walks the whole mailbox
   against one list of the record kinds it holds (`internal/inbox/records.go`), then runs
   the barrier's own effects as a plan that writes nothing, through the one file seam
   both passes use (`internal/inbox/access.go`); only a plan that meets no unknown lets
   the same code run again for real. A test fails every read the plan makes, in turn,
   and finds what it reads by watching the seam rather than from a list
   ([mailbox-records.md](mailbox-records.md#the-plan-and-the-seam)). The stop is itself
   a record of the mailbox (`stopped`). One the plan found is found again by the same
   plan every call makes, and goes once that plan finds its cause gone. One only an
   effect met, which no plan shows, is answered from the record by every later call, and
   goes only once a barrier has run every effect through, whatever a canceled or failed
   retry, or the plan after a failure, met. So a record or mark that could not be read
   waits for a later look. One whose evidence is gone is irrecoverable: a report of the
   earlier build with neither letter nor mark was never written, or written, read and
   swept, and the bytes are the same either way. Main is told once, and the person settles
   it report by report with `rewake settle`: the owner decided on September 30, 2026 that
   such an unknown stops the whole mailbox and is resolved by one command, with no fence
   on single tasks. A report that closes nothing — a stop, an interim note — is the one
   exception: unknown, it is withheld for good, and the journal records that decision so
   that no later barrier weighs it again, since losing a note repeats nothing; the kept
   answer, the waits and the interim record of the same operation are still decided by
   their own evidence. The proof that an effect is done lives as long as anything could
   replay it — the journal's entry and the recipient's marks, while the recipient's run
   lives; a report found without a mark is recorded in the sender's journal first and
   marked for the recipient after. A journal is kept while its run may retry it, an
   unfinished one for good, a person's decisions for good beside the records they
   settled, and a record that names no run is never swept by age. Recovery is one
   barrier under the mailbox lock, before any turn end reads a wait and before adoption
   takes one over. It takes every record at once and first establishes which obligations
   proven publications closed: an obligation is a task or question of one sender's run,
   answered by a closing report (finished or failed), and one report id says nothing
   about another report closing the same obligation. Only then does it decide the rest:
   an effect on obligations another proven publication closed is superseded and does
   nothing; one proven not done is done, unless another publication closed part of its
   obligations: then it is not published and the rest stays owed; one unknown stops the
   mailbox, and so does one partly superseded, since whether it answered the rest is
   unknown too. The owner decided on September 30, 2026 how a mailbox changes protocol:
   a session started by an earlier build stays on that build's protocol for the rest of
   its run. This build changes nothing in that session's mailbox — not for the session's
   own hooks and calls, not for another session's send or report — and refuses instead,
   saying that rewake was upgraded and the session must be restarted by resuming its
   conversation, and main is told. That holds for main too: until it is resumed its mail
   stands still, and reports to it wait at their senders. Nothing is restarted
   automatically. Which build a run followed is proven by a record that outlives the
   registry: a run of this build is named by the machine's boot and its epoch, since an
   epoch alone recurs after a restart, and writes its run record under both, with the
   build stamp, before its session record is published; run records are never swept, and
   a run named by its epoch alone is an earlier build's. The first run of this build
   under a name binds itself, as durably, as the successor of the name's earlier-build
   runs, and stays it once ended; what is held for them waits while it starts, and is
   moot only once it is proven gone. The new protocol takes a mailbox over only once
   every earlier-build writer of it has stopped — its wrapper and every rewake process
   acting for that run, hooks too, not merely the harness the session record calls alive
   — by a look over the person's rewake processes, complete where the upgrade replaced
   the binary at the paths the earlier runs use, which bounds the automatic cutover;
   otherwise the launch refuses; and what that build left unfinished is met by the new
   run's barrier at adoption. Its receipts keep their origin when converted: a missing
   mark never proves their reports unpublished, and a done mark proves the reports it
   lists and nothing after them. A kept answer, a pending mark or an interim record of a
   run that has ended is never taken by another run's end, whatever its text. Every
   record and state, and what recovery does with each, is in
   [turn-end-recovery.md](turn-end-recovery.md); the cutover in
   [protocol-cutover.md](protocol-cutover.md). What each acceptance from the fourth on
   found, and what closes it now, is in
   [turn-end-recovery-findings.md](turn-end-recovery-findings.md#what-each-earlier-finding-meets).

The third acceptance (September 30, 2026) found five more, and found that they could not
be closed one call site at a time: the mail tool decides by the same lookups the older
delivery paths do, so the line between the two cannot be drawn by the age of the code.
Rule 6 is therefore kept by the shared lookups in `internal/inbox`, not by their
callers. Each answers found, proven absent, or a Go error for unknown, and only "no such
file" is absence; a record that exists and cannot be read or does not parse is an
error, and so is a wait record whose times or sequences do not parse; the record that
names the run alone, from before times were kept, still reads. That covers the listing
of a mailbox directory (so `PeekUnread` and `AvailableUnread`), a message's status
(`ReadStatus`, `Answered`), whether a directory holds a message, the waiter records
(`ReadWaiters`, recording a read into one, and clearing one once it is reported to),
the claimed letters, what a run sent and where each stands (`Awaited`, `SentMatching`), the
addenda of a task, the records of a turn end (the pending mark, the interim record, the
kept answer), and the owed and retained sets the sweep keeps by. The fourth acceptance
(September 30, 2026) found three unknowns still folded behind these: a status the
sender's wait could not read, a wait record left uncleared after a report, and a wait
record whose numbers did not parse. Each is described below.

Each caller decides what an unknown stops, and it always stops the effect: a read, an
acknowledgment, a withdrawal, a mark, an announcement, a status written over one that
could not be read, a removal by the sweep, the adoption of an earlier run's waits. The
operation stays where a retry can take it up, and a delivery stays pending. A few
readers only inform and change nothing, and they fold an unknown into their most
cautious answer instead: the thread check on a report (no mismatch, so no advice to
send again), a predicate on grants (the task still open, not settled), and the hold on an
unmarked turn end (no hold, which is the behavior without it; a kept answer that
cannot be read is never written over). The wait of a sender for its status goes on
past a status it cannot read, and `send` then reads the status once more, so a result
that cannot be read stays unknown whoever holds the name by then. A wait record that
could not be cleared after a report stays in the turn end's journal, completed before
a wait is read again (rule 7, [delivery-turn-end.md](delivery-turn-end.md)). A note that a delivery failed is withheld while the status cannot be
read; the sender still finds the failure with `rewake inbox --awaited`. `inbox.Waiters`
drops the error of `ReadWaiters` and exists for tests only; a test fails when code
outside tests calls it.

## Bridge mode

A CLI process runs a tool call when `REWAKE_BRIDGE_TICKET_FD` is set
(`internal/bridge`). The variable names an inherited descriptor carrying the call's
ticket as JSON: the per-launch capability, the native conversation, turn and call id,
the call time and deadline on the boot clock, the digest of the normalized words and
the transport. The descriptor is read once, within a second, at most 4 KiB; a ticket
missing its conversation, turn, call, digest or a deadline after its call time is
refused. The process then:

1. checks the words against the surface again, with the real parser and without
   aliases, which could turn a word into a launch;
2. refuses a ticket issued for other words, then asks the wrapper whether it issued it
   (`bridge.Validator`, the interface the server's endpoint will fill). The default,
   `bridge.NoEndpoint`, refuses every ticket, so an invented one authorizes nothing;
3. refuses a call whose deadline has passed;
4. runs the command and bounds its answer (below).

Nothing in the words — an id, a time, a name — is authority by itself. The server will
call `cli.ToolWords` for the same check before it starts the CLI, and the digest both
sides compare is taken over the normalized words: the command, its flags sorted with
their values inline, then `--` and the positionals, texts exactly as given. Two
spellings of one call are one call; two texts are two.

The surface is the table in the specification, with its limits made exact: at most
12 words and 32 KiB in all; help for an allowed command; `--wait` of 0 to 5 seconds,
further cut to the call's deadline less half a second. Main's wider commands stay
shell calls. `whoami` carries `mailChannel`, `tool` or `shell`, for the call itself;
what the wrapper observed of either channel comes with the server.

`rewake retry` under the tool checks the words its receipt recorded against the same
surface (rule 3): a receipt a shell call left — a heads-up whose text came from stdin,
say — is finished from the shell, and the tool refuses it.

## Bounded output

The encoded size of an answer is taken as the server will encode it: stdout and stderr
each as a JSON string, escapes included, plus 256 bytes for the server's framing
(`bridge.EncodedSize`). Every answer under the tool leaves through `boundedAnswer`,
which holds it to 4 KiB that way (rule 5):

- a diagnostic whose encoded size passes 1 KiB is cut to it, on a UTF-8 boundary, with
  the shell named for the full refusal;
- a longer answer that marks nothing — help, `--owed`, `--peek`, `--awaited` — is kept
  as an output record and handed out in parts with `inbox --next`; a part under
  `--json` is a whole envelope carrying its slice, and no part carries more than 2 KiB;
- whatever still does not fit, the failure to keep an output included, is replaced by
  one short line naming the shell.

A read's part is sized when the read is frozen and checked again against the cap before
the part is recorded as shown, so a part the bound would have to change is never one
the read counts.

## Reads in parts

An explicit read under the tool — `inbox` or `inbox --message` — freezes its batch at
the first call, under the mailbox lock: which letters, each in its version (a digest of
the stored letter), each cut into parts on UTF-8 boundaries. A response carries one
part; after a letter's last part, whole single-part letters that follow ride along while
the text stays within 2 KiB and the result fits. Every part, in text as in JSON, names
the letter id, the part and count, the byte range and total; the response names the
receipt and the exact next words (`["inbox","--next","<receipt>.<letter>.<part>"]`).
Partial text says the letter is not read yet. Letters that arrive later are not added;
`--peek` and `--message` reach them.

Before any part of a letter is shown for the first time — the first part or any other a
continuation names — the letter is looked up under the mailbox lock (rule 3). A letter
withdrawn since it was frozen shows as its tombstone, from its first part; one no longer
unread at all, read through another call or taken out by its delivery's end, shows a line
saying so instead of its text and is not marked again. The deadline is checked in the
same critical section, before the part is recorded as shown: past it, nothing is shown
or claimed. The lookup reads the letter's own file and its status, not a listing that
passes over a file it cannot read, and one that fails shows nothing of that letter
(rule 6): the text frozen may be one withdrawn since. The call fails, nothing is
recorded as shown, and the same words show the letter once the mailbox reads again.
A claim that cannot be read counts as standing.

Nothing becomes read by being printed. A part counts once the wrapper saw the result
carrying it reach the model whole, and says so through `cli.AcknowledgeRead(dir, name,
epoch, receipt, bridge.Exposure)`. It is a Go function the observer calls, not a command:
there is no word an agent could run to the same effect. The evidence must name the call,
be direct, successful, not shortened and inside 4 KiB. Once every part of a letter's
version is acknowledged, the letter goes through the ordinary `MarkRead`, with its waiter
and status — unless it is no longer in `unread/`, which means it was read (rule 4), and
then nothing is marked or owed again.

The first part shown of an unread letter writes a claim, `inbox/<name>/claims/<id>`.
While it stands, `withdraw` and `edit` refuse (`inbox.ErrReadInProgress`, a kind of
already-read), the sweep keeps the letter however old, and the server's own outcome for
it — a delivery that failed late, say — is recorded as delivered, since part of the text
is in front of the agent; nothing but a read takes it out of `unread/`. `MarkRead`
removes the claim.

The same words in the same turn are the same read, even after it ended: the frozen batch
again, from its first part, with a line saying so once any part was shown, and naming
`--peek` and `--message` for what came since. A read that found nothing answers the same
nothing to those words for the rest of the turn, and one that was refused, the same
refusal; a read the lock or the disk kept from freezing froze nothing, and the same
words try again.

## The shell

Plain `rewake inbox` in a shell is unchanged: whole output, read once printed. Parts are
the tool's. The shell can continue a read or an output with `inbox --next` and show
either again with `rewake retry <receipt>`, and its trust stays what it was: a part the
shell printed counts as acknowledged. A part only the tool showed stays unacknowledged
until the observer confirms it or the shell shows it again, so a read the tool lost is
finished with `rewake retry <receipt>` and `--next` to the end.

## Receipts

A heads-up (`send --notify`) and a pending mark run under a receipt, from the tool and
from the shell alike (`internal/receipt`). The journal is `inbox/<name>/receipts/<epoch>/`:
one record per operation, `<token>.json` with a 24-hex token, held by a lock file of its
own; an index file per scoped key points at its record. The key is the run, the native
conversation and turn, and the digest of the normalized words. The record is written
before anything else and holds each step and, when finished, the answer, which a repeat
gets again with a line saying so. `rewake retry <receipt>` finishes an open operation or
replays a finished one; it cannot reach another run's journal.

- **A heads-up** fixes its letter id, its text and the recipient's run in the record
  before it publishes; a retry takes the text from there, never from stdin again.
  Publication happens under the recipient's mailbox lock, against a mark in
  `inbox/<to>/once/<to-epoch>/<id>`: `intent` before the letter is written, `published`
  after. The deadline is checked under that lock, just before the mark and the letter
  are written. The sweep turns an intent into published before it removes the letter it
  names, and keeps the letter when that write fails, so an intent without a letter
  always means the letter was never written, and a retry a day later writes nothing
  twice. Taking the recipient's lock is new for a heads-up; it is held only for state
  changes, and the wait is bounded by ten seconds or the call's deadline.
- **A heads-up whose recipient's run ended** before the record said it was published
  is resolved from what that run's mailbox shows: the letter there, or a `published`
  mark, is a heads-up published to the run that ended; an `intent` mark without the
  letter is one never written. Either answer is final and replayed. When neither is left
  — the marks of an ended run are swept — the effect is unknown: the answer says so, the
  record is kept as long as its run lives (rule 1), and nothing goes to the session that
  took the name since. The mailbox is read under its lock, so the letter and its mark are
  seen as the sweep left them. A lookup that fails — the recipient's record, the letter's
  stages, the mark — is no answer (rule 6): the operation stays open for `rewake retry`.
  Nothing is published on a guess either: a stage of the mailbox that cannot be searched
  may hold the letter, so the publication writes neither the intent nor a second copy.
- **A pending mark** records its text and its time before marking: the ticket's call
  time under the tool, the process start in a shell. The deadline is checked under the
  mailbox lock, just before the mark. A retry marks with the recorded time, and never
  over a newer mark of the same run (`inbox.MarkPendingUnlessNewer`), so it cannot
  stretch into a later turn. Letters owing a report that a read is showing in parts
  count among the waits: their acknowledgment comes before the turn ends. "Nothing is
  owed" is an answer only when every wait record and claim was read; one that could not
  be, or a mark in place that could not, leaves the operation open (rule 6).
- **A refusal** is an answer like any other: the same words in the same turn replay it.
  Its effect is proven absent, so the sweep may remove it a day later with the rest.
- **A deadline** or a busy mailbox before a commit leaves the operation open, the call
  exits 1 naming `rewake retry <receipt>`, and nothing was committed. A record busy in
  another call exits 3 with the same words.

**The shell and an unresolved operation.** A shell knows no verified turn, so its own
receipts are never joined by words: two shell calls with the same words are two
operations, as before. The specification allows adopting "only an unambiguous
unresolved operation with verified scope"; a shell has no verified scope. So a shell
call whose words match an operation of this run still open — whatever its age, from
the tool or from the shell — does not run: it exits 3, the call accepted and not done
yet, naming the receipt, and `rewake retry` finishes that operation. One whose effect is
recorded as unknown stops the same words too, finished or not (rule 6): it exits 1
naming the receipt, for as long as the run lives, and a message in other words is a new
operation. So does a journal that cannot be read. An operation that finished with its
effect known stands in the way of nothing: the shell's same words are then a new
operation.

**Sweep.** A finished record, and a read with nothing left in progress, go with the
finished mail a day after their last change; an open record, and one whose effect is
unknown, stay while their run lives. Another run's journal goes once it has been idle a
day, open records included: no call can reach it — a receipt names a record of its own
run only. That is the clean-up of a journal nobody can reach, not proof that its effects
are absent. A journal whose age cannot be read is kept. Each record goes under its own
lock (rule 2).
Once-marks of runs other than the live one go at every sweep.

## Left to the next stages

- The context endpoint that validates tickets, and the observers that call
  `AcknowledgeRead` with evidence (stage 2). Forgery and reuse of a valid ticket are
  proven only with that endpoint; the tests here stand in for it.
- Tying a delayed acknowledgment to the turn that exposed the part, with that turn's
  read boundary: the CLI records which call showed which part and when, and the turn
  association is the observer's. Pending counts letters in progress as waits; waiting
  for an acknowledgment still in flight belongs with the observer too.
