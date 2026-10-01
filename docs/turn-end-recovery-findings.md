# Turn-end recovery: what the reviews found

The findings of the acceptances and reviews of the turn-end recovery, each with the
clause of rules 7 and 8 that closes it, and what the probes of the earlier
acceptances expect now. The rules are in [mail-bridge-cli.md](mail-bridge-cli.md); how
recovery keeps them is in [turn-end-recovery.md](turn-end-recovery.md), whose clause
names (7-journal, 8-stop and the rest) this document uses.

## What each earlier finding meets

| Found | Closed by |
|---|---|
| A failed clear of a wait answered again (fourth acceptance) | 7-journal: the clear is a step of the journal, completed before a wait is read again |
| The journal written after the reports (fifth) | 7-journal |
| A journal of an ended run dropped while its waits were taken over (fifth) | 8-reconcile before adoption; 8-retention |
| A prepared receipt publishing on its own (sixth) | 7-journal: nothing but the journal publishes |
| A kept answer left behind by recovery, or a later one dropped by a late retry (sixth) | 7-clock and 8-evidence: taken by version, only below the boundary |
| A journal outliving its only proof of publication (sixth) | 8-proof: marks for the recipient run's life |
| The interim end recorded outside the journal (seventh) | 7-journal: interim is a journal step |
| A late retry after a day's retention preparing again (seventh) | 8-retention: journals kept while the run may retry |
| An earlier-build receipt completed as if empty (seventh) | 8-origin |
| A report whose entry failed published again after the sweep (seventh) | 8-proof |
| A retry whose first attempt recorded nothing answering a question read since (seventh) | 7-scope |
| A retry whose receipt was never written doing the same (eighth) | 7-scope: the event's boundary, or no effect |
| A retry taking an answer kept after its first attempt (eighth) | 7-clock |
| An earlier-build receipt bypassed by every end but its own (eighth) | 8-reconcile: one barrier for every format |
| That receipt swept because it named no run (eighth) | 8-retention |
| Its done mark taken as proof its kept answer was gone (eighth) | 8-cutover: an ended run's kept answer is never taken |
| Its report found without a mark, published again after the sweep (eighth) | 8-proof: sender-side proof first |
| An unreadable interim record read as permission (eighth) | 8-stop |
| An unknown publication mark read as permission (eighth) | 8-stop |
| Both writes lost, the retry taking the answer kept now (ninth) | 7-clock |
| Equal text taken for a kept answer's identity (ninth) | 8-cutover: no text is compared; an ended run's answer is never taken |
| An earlier build's publication repeated once its proof was swept, or its new proof failed (ninth) | 8-evidence and 8-stop: never proven unsent; the person settles it |
| Overlapping earlier-build receipts replayed one by one (ninth) | 8-reconcile: one conversion journal decides them together |
| A retry's late receipt fixing the scope of its moment (ninth, rules review) | 7-scope: an id without a boundary is refused |
| One decision for a whole operation with several recipients (ninth, rules review) | 8-settle: one decision per report |
| A late interim end over a later one, and pending marks not in the journal (ninth, rules review) | the interim and pending tables |
| The absence of a matching text taken as history (ninth, rules review) | 8-cutover: text is not read at all |
| The mixed run (ninth) | 8-cutover: this build does not act for an earlier-build run |
| A later pending end removing the mark an earlier end's retry needed (ninth, second rules review) | 7-scope: no end removes a mark; marks live for their run's life |
| A report partly superseded while unknown handed to the next end (ninth, second rules review) | 8-reconcile: the mailbox stops |
| The protocol and the successor of a run proven only by the registry (ninth, second rules review) | 8-cutover: run records and the successor record, written before the session record |
| A person's decision dropped with the done journal (ninth, second rules review) | 8-retention: the conversion journal is kept whole for good |
| Equal `Ended` and equal `At` ordered by the order of retries (ninth, second rules review) | the interim and pending tables: fixed tie orders |
| A start never recorded letting later ends take a mark already used (ninth, third rules review) | 7-scope: a window opens after the latest end a journal records |
| An epoch that recurs after a restart naming a permanent run record (ninth, third rules review) | 8-cutover: runs are named by boot and epoch |
| Writers found stopped without proof that the list of them was whole (ninth, third rules review) | 8-cutover: the conditions of a complete look, or a refusal |
| A look for writers that needs no thread to start anywhere meanwhile (ninth, fourth rules review) | 8-cutover: the supported upgrade bounds it; the look is over earlier-build rewake processes |
| A successor still launching taken for one that has ended (ninth, fourth rules review) | 8-cutover: bound, starting, ready and gone told apart; only gone makes a held report moot |
| The outcomes with no times dropped along with a real end's unknown `Ended` (ninth, fourth rules review) | the interim record: they are published without a place among the ends |
| An unknown mark, kept answer or interim record found only after a report went out, and the stop then forgotten (twelfth review) | 8-stop: the whole mailbox and every effect's marks read first; the stop on record ([mailbox-records.md](mailbox-records.md)) |
| A moot note to main lost when its publication failed (twelfth review) | the note owed with the moot decision, sent until it is out |
| A mark behind a file taken for none, a directory where a record belongs, a newly chosen successor's mark read after earlier reports (thirteenth review) | 8-stop: only a missing file is no mark; directories checked against the list; the successor chosen now read first |
| An effect's stop lifted by a retry that did not run its effect (thirteenth review) | 8-stop: lifted only once a barrier ran every effect through |
| The letter in the recipient's stages, the mark of a note the barrier creates, and an unreadable read clock found only by an effect (fourteenth review) | 8-stop: the effects run first as a plan through one file seam; a test fails every read it makes |
| An effect's stop replaced by a different cause the plan after the failure found, and lifted with it (fifteenth review) | 8-stop: the effect's stop is recorded first, a later cause beside it |
| An effect's stop recorded without its cause when the first write of it failed and a later one went through (sixteenth review) | 8-stop: the barrier holds the effect's cause and writes it into every record of the stop it makes |
| The effect's cause missing from the answer when neither write of the stop went through and the plan after it found another cause (seventeenth review) | 8-stop: the answer names the effect's cause beside the plan's, as the record does |

## What the earlier probes now expect

Three probes of the seventh and eighth acceptances describe the same bytes: an
earlier-build receipt prepared and not done, whose report has no letter and no mark.
`TestReview7ReceiptPreparedBeforeM1/prepared` and
`TestReview7ExtraLegacyReceiptRecoveredByNextEnd/false`
(`.scratch/m1-review-7/round7_cli_test.go`) expected it published;
`TestReview8LegacyPublicationUnknownAfterSweep`
(`.scratch/m1-review-8/round8_inbox_test.go`) expected it not published; and
`TestAnEarlierBuildsReceiptIsRecoveredByTheNextEnd/done=false`
(`internal/cli/turn_legacy_receipt_test.go`) published it too. All of them now expect
the same: the receipt belongs to a run that has ended, and a run of this build meets it
at adoption; nothing is published and no wait is cleared; the mailbox stops with the
journal and each unknown report named; main gets the note once; `--undelivered`
publishes that report once and `--delivered` publishes nothing and clears its waits;
the same words again change nothing. A probe that ends a turn of the receipt's own run
now expects the refusal of 8-cutover and no change at all. The branches with a proven
publication or a done mark still expect completion without a second copy.

The rest of those probes changed only where they named what no longer exists, each
change marked in the adapted copy under `.scratch/m1-review-10/`, which records why: an
end that names an event carries its boundary; a fault in `turns/` moves to the journal,
the one record this build writes; a probe that expected a mark consumed expects it still
in the window; a receipt of the ending run's own end becomes one of its earlier-build
run; and the proof of an earlier-build publication, a once mark in the recipient's
mailbox before, is now the entry in the sender's conversion journal, so a fault on that
proof moves to the sender's journal directory and, once the letter is swept, stops the
mailbox rather than publishing again.
