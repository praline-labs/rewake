# Turn-end recovery and the protocol cutover, built

Round 10 of the mail-tool stage: the recovery rules accepted on September 30, 2026 —
rules 7 and 8 of [mail-bridge-cli.md](../mail-bridge-cli.md),
[turn-end-recovery.md](../turn-end-recovery.md) and
[protocol-cutover.md](../protocol-cutover.md) — built into the code the same day. How the
path runs through the code is in [delivery-turn-end.md](../delivery-turn-end.md); the
launch's part is in [launch.md](../launch.md#launching-a-harness).

## What was found

- Where code and rules disagreed, the code changed; no rule was found wrong while
  building.
- The look for earlier-build writers first cleared an unreadable executable of this
  user only for capabilities beyond this process's permitted set. On a real machine the
  per-user service manager's helper still blocked every launch: its inheritable set
  carried a capability and its files belonged to root. Main's decision of the same day
  widened the rule to the permitted or inheritable set, or a process that is not
  dumpable; the ground and the evidence are in
  [protocol-cutover.md](../protocol-cutover.md#proving-the-earlier-writers-stopped).
- Two launch tests had passed only because the look ran against the machine's own
  processes: their fixture records carried no pid namespace, so a record was out of
  sight and refused once the look judged it.
- The workflow suite had read what the earlier build left: three scenarios counted the
  receipts in `turns/`, which this build no longer writes, and two controls named text
  the journal replaced. They count the journals named by an event now — every end leaves
  a journal, and only one named by its event is the old receipt's counterpart — and the
  controls change what the journal clears and where the mark is found. The wrapped
  launch compared socket paths by their epoch; with the boot id in it the path passes
  103 bytes and takes its digest form, which the comparison now normalizes too.
- Mutants of the working tree, 34 of them through `go test -overlay`, left five
  survivors. One was equivalent and was replaced by two that reach the code; four were
  killed by new tests: a done receipt swept by age, a found letter recorded before the
  sweep, the window opened after the run's latest earlier end, and a report for a
  replaced run of this build.

## What was done

- A run is named by its pid, start and boot; runs of this build keep a run record and
  bind one successor per name. A call for a run of the earlier build is refused with
  exit 1, and main is told once; a send to one still running is refused.
- A turn end is one journal named by its run and event, written before its first
  effect; an end named by an id carries its read boundary; pending marks are files of
  the run's life, and a turn's window opens after the run's latest earlier end; the
  interim record keeps the later end of the same run.
- The barrier converts the earlier build's receipts into one conversion journal and
  decides every report on its evidence together; an unknown report stops the mailbox
  until `rewake settle`, which goes through the command table.
- A launch looks for earlier-build writers once, before the room lock, and refuses,
  exit 1, naming each process as an unknown user, an unknown executable, an earlier
  rewake, out of sight, or an earlier run. The look takes its process tree as a value:
  the cli tests and the workflow suite's build give it an empty one, so neither depends
  on the machine's processes.
- The earlier probes run under overlays, their changed expectations written down in
  [turn-end-recovery-findings.md](../turn-end-recovery-findings.md#what-the-earlier-probes-now-expect).
- The session record moved to [session-record.md](../session-record.md) and a resumed
  conversation to [delivery-conversation.md](../delivery-conversation.md#a-resumed-conversation),
  so that design.md and delivery.md stay under 400 lines.

## The review of October 1

The reviewer's eleventh round found eight places where the code did not follow the
rules; no rule changed.

- A conversion that died after saving its journal left receipts the next barrier took
  for new ones and stopped on. A receipt the journal holds byte for byte is now removed
  and the conversion goes on; one that reads otherwise stops.
- A successor recorded as a held report's recipient was trusted on the next attempt,
  so one that had ended was counted as published and main was not told. It is looked
  at again every time: gone makes the report moot, anything short of ready keeps it.
- An unreadable journal or wait stopped the barrier but not `pending`, `inbox` or a
  read acknowledgment, and the barrier published before reading the other journals.
  The barrier now reads every record before any effect, and every call that would
  change the mailbox asks the same reading; the sweep of other effect paths added a
  question taking its answer, a later part of a read in parts, and a wait of a run of
  this build without its place on the read clock.
- A worker could settle its own mailbox; settle from inside a session now needs a
  verified main.
- A moot closing report left the waits it answered owed; they are cleared.
- A launch went on over an unreadable successor record, and pruned the dead record of
  its name before the look for earlier writers could see it. Everything before that
  look now only reads, and an unreadable successor record refuses the launch.

Each fix has a test and a mutant it kills.

## The second review of October 1

The twelfth round confirmed the eleventh round's fixes and found two defects.

- The reading before any effect covered journals and waits but not the evidence the
  effects decide by: an unknown publication mark, kept answer or interim record was
  found only after a report went out, and the stop was then forgotten, so the next
  `pending` went on. It was the third round in a row to find an unknown of that class
  late, so the class is closed by construction (refuted by the fourth review)
  ([mailbox-records.md](../mailbox-records.md)): one list in code of every kind of file
  a mailbox holds, a walk of the whole mailbox against it and a reading of every
  remaining effect's marks before any effect, a stop kept on record in the mailbox, and
  a test that fails when a writer of the package leaves a path the list does not know.
- A moot held report's note to main was lost when its publication failed, since the
  moot decision was saved first and a retry skipped the note. The note is now owed in
  the journal with the decision and sent by every later barrier until it is out; its
  id is fixed, so a retry publishes it once.

The design met three existing tests, which pass unchanged with two consequences made
explicit: a read that met a stop does not keep the stop as its answer, and a path
through a file that is no directory holds no mark (taken back in the third review). One test changed how it stages a
failed move — a closed archive rather than a file of no kind in its place, which the
reading now stops on before the move is reached.

## The third review of October 1

The thirteenth round accepted the two kinds of stop, the note owed to main and the
changed acknowledgment test, and found four more unknowns that arrived late.

- A path to a publication mark through a file where its directory belongs was taken for
  no mark, so an earlier report went out and the gate stayed open. Only a missing file
  is no mark (rule 6); the exception is gone, and two retry tests that blocked a
  recipient with a file in place of its mailbox now close it to writes instead, a plain
  failure a retry clears.
- The walk skipped every directory, so an empty directory where `kept.json` belongs
  passed the reading and a report went out before the effect met it. A directory is now
  allowed only on the way to a kind's files, as the list's patterns say.
- A stop an effect met was lifted by the next clean reading of the barrier, before its
  effects ran, and replaced by any cause a later reading found. It now stands until a
  barrier has run every effect through, and a later cause is recorded beside it.
- The mark of a successor chosen during the barrier was read only when the report came
  to it. The reading and the publication now decide the successor by one function, and
  the reading reads its mark before any effect.

The completeness test grew with them: a directory and a link at every kind's path, a
file in place of a directory and a directory closed to reading on every evidence path,
and a canceled, stopped or failing retry against every durable stop
([mailbox-records.md](../mailbox-records.md#the-tests-that-hold-it)).

## The fourth review of October 1

The fourteenth round confirmed the thirteenth round's fixes and found three more
unknowns that arrived late: the letter searched for in the recipient's unread and done
stages, the mark of a moot note the barrier itself was about to create, and a read clock
only checked for its shape, never read. Each was a dependency the hand-written reading
before the effects did not list, as in the three rounds before it, so the list itself
went.

- The barrier plans first: after the walk it runs its own effects with every write
  doing nothing, so what they decide by is read by the code that decides it. Only a
  plan with no unknown lets the same code run again for real, on fresh reads; an
  unknown met only there is the effect's stop. The gates ask the same plan.
- Every file operation of the barrier and the gates goes through one seam
  (`internal/inbox/access.go`); the walk reads every file it opens, the read clock
  among them, and something other than a regular file where a letter belongs is
  unknown. The registry stays outside the seam: live by nature, read-only in the plan.
- A test watches the seam instead of a list: every read of the plan in ten scenes,
  failed in turn with every fault that applies, in both passes and in the effects
  alone, and each scene once more with the mailboxes moved where only the seam finds
  them, which a read around it cannot.
- The note to main at a conversion stop is an effect like any other: one whose mark is
  unknown stops the mailbox. The test also found an effect stop that outlived a
  barrier which ran every effect up to a conversion stop; that conversion stop is now
  what the mailbox answers.

Main decided on October 1, 2026 that the effects re-run the decision code rather than
execute the plan as data, that the registry stays outside the seam, and that the note at
a conversion stop is no longer best effort.

## The fifth review of October 1

The fifteenth round accepted the plan, the seam and the test over it, and found one
more way an effect's stop was lost. When an effect failed on an unknown, the barrier
planned again to explain the failure, and a different cause that plan found was
recorded in place of the effect's; once that cause went, a gate lifted the stop before
the effect had run through. The effect's stop is now recorded as soon as the effect
fails, and a cause the plan after it finds is kept beside it, as a retry's always was.
The test over the seam composes the two: each read the effects make fails them, then
each read of the plan fails the plan after them, and the stop must outlive the second
cause. A scene of two journals sharing one wait, the kept answer and the interim line
joined the others. The text that counted a settle among the calls that plan first was
wrong: a decision of main or the person is written first, in a stopped mailbox too.

## The sixth review of October 1

The sixteenth round accepted that fix and found the same loss through a failed write.
The barrier recorded the effect's stop, and when that write failed and the plan after
it found another cause, the next write learned of the effect only from the record that
was never written, so it recorded a plain stop that went with the other cause. The
barrier now holds the effect's cause until it returns and writes it into every record
of the stop: once when the effect fails, and once more after the plan, so one failed
write leaves the next one whole; a stop no write could record is answered with every
failure named. The test over the seam now also breaks every write the barrier makes in
each scene, once, in the scene as it is, with an effect stopping, and with the plan
after it stopping too. The document of the turn-end recovery reached its size limit,
and what the reviews found moved to
[turn-end-recovery-findings.md](../turn-end-recovery-findings.md).

## The seventh review of October 1

The seventeenth round accepted that fix and the broken writes, and found the effect's
cause lost from the answer. When neither write of the stop went through and the plan
after the effect found another cause, the barrier answered that cause and the two
failed writes; the effect's cause, kept nowhere else, was gone. The answer now names
the effect's cause beside the plan's whenever the plan found one, as the record does,
whether or not the record was written. The test over the seam checks that answer for
every pair of causes a scene produces, and runs each pair once more with both writes of
the stop failing.

## What stays open

What the rules leave open is in
[turn-end-recovery.md](../turn-end-recovery.md#what-stays-open). The stage was accepted
with the seventeenth round and landed in one commit
([the stage's entry](2026-10-01-mail-tool-cli-stage.md)).
