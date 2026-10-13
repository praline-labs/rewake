# Stage 3: the decision's worlds

The worlds S18's tests build, enumerated before any matrix is coded (Q14 of
[stage3-decision-build.md](stage3-decision-build.md#q14-acceptance-in-three-parts)) and
apart from the code that records a decision. The dimensions are those of
[the model](stage3-decision-model.md#the-world) as the recovery document's table lists
them ([the tests](stage3-decision-recovery.md#the-tests)); their values, the excluded
combinations, the partitions and the outcome each world must have are code, in
`internal/core/mail/decisionmodel`, and `TestTheManifestCountsAreTheModels` holds the
numbers below equal to what that package enumerates.

## Partitions

The model's dimensions do not all interact. A product of all of them would test the same
schedule many times over, so they are split into four partitions, each a product of the
dimensions that do interact, with the rest held at one value and its events in the fixed
order the next section names:

| Partition | Holds fixed | Dimensions | Product | Feasible | Pairs | Pair cover |
|---|---|---|---|---|---|---|
| `decision` | one report in J, no resend run, nothing else unknown | R's truth, R's observation at the decision, R's observation later, the choice, a second command, who decides, r alive or ended at the decision, r living on or ending after it, activity in r's mailbox, the sender's life | 25920 | 3198 | 354 | 23 |
| `resend` | R decided "did not arrive", nothing else unknown | the truth of R' with its evidence, the second choice, who decided R, r's end along J''s path, the sender's life | 1458 | 219 | 162 | 41 |
| `mailbox` | R unknown and decided, r alive, the sender alive | the rest of the mailbox, the second report in J, the choice | 64 | 52 | 53 | 32 |
| `generations` | every copy decided by r, every unknown at a mark | the generations with the truth of R' and the second choice, R's truth | 12 | 12 | 12 | 12 |

**Feasible** is the clean worlds: 3481 in all. **Pairs** is how many pairs of values some
feasible world of the partition holds; **pair cover** is how many worlds the generated
set has that holds every one of them — 108 in all, the set the five checks run. It is
chosen greedily in a fixed order from the feasible worlds and verified (`Covers`), never
written by hand.

## The schedules held fixed

Each partition fixes the order of its events, and its dimensions place only what they
name; a world is that order with its values. The plays are `playDecision`, `playResend`,
`playMailbox` and `playGenerations` in `internal/core/mail/decision_matrix_*_test.go`.

- `decision`: J writes R or not; R's evidence turns unknown; r ends, when it ends before
  the decision; the decider writes D at its linearization point, verified by r only while
  r lives; D is installed in the barrier `decide` runs after it, J' with it, and runs
  there; a second command, when there is one, after D; R's later evidence after D, or
  before D's write when it returns before it; r's end after D, when it ends after it,
  past D's installation; the activity in r's mailbox after D, except that r reads R at
  any time it lives and a new run of its name comes only after r ended; the sender ends
  before D's effects are installed or after, or not at all. Evidence that returns before
  D's write leaves no D, r alive or ended: S3 reads a held report's evidence without
  admitting an ended r, so `decide` refuses it as proven and the barrier settles R —
  published, or moot when r ended before and R was never written.
- `resend`: R's evidence unknown; the decider writes D "did not arrive" — the person,
  when r ended before it — and the barrier `decide` runs installs and runs J'; r ends at
  the point of J''s path its value names, or not at all; R' is decided only once it is
  unknown, in the barrier after, by r while it lives and by the person once it ended; the
  sender at its point; R's own evidence stays unknown and r's mailbox sees no activity.
  Each point is placed on the step the product reaches, not on a count of steps, so a cut
  before it moves the event with the path: J' installed (r ends before its liveness
  read), the first liveness `stat` of R' (inside the section), the first write of R''s
  mark or letter (before its intent), the published mark after the letter, the save of
  `Published`, the rename of J' to `.done`. An unknown R' is set up as J' is installed —
  an intent with no letter, a mark that does not read beside the letter or not, an
  intent beside the letter or not in a mailbox whose stat of that letter fails — or made
  as the attempt goes: the mark turned unreadable and its write failed, after the letter
  or at it.
- `mailbox`: R's evidence unknown; r, verified and alive, writes D; the rest of the
  mailbox as its value names, settled after D or left; r and the sender live throughout;
  R' publishes on its first attempt unless the rest names it unknown. A barrier records
  the first unknown it meets — a journal that does not read before any step, then the
  steps by journal name, J' before J — so the unrelated journal turns unreadable, named
  before J (`aaa`) or after it (`zzz`), once every report the world decides is on record:
  before D when J holds R alone, after D when it holds a second report too, since that
  report is met only once R is decided. The second report goes to another run, unknown
  the same way, decided by the person; an unknown child is R''s mark unreadable as J' is
  installed, settled by the mark repaired to its intent or by r deciding R' "arrived".
  The occurrences left open are some of those the world leaves, never one more.
- `generations`: each copy turns unknown at its mark — one that lands at its published
  mark after its letter, one that does not at its letter — and is decided by r, verified
  and alive, before the next copy runs in the barrier `decide` runs; the copy after the
  last decision publishes on its first attempt; r and the sender live throughout. `one`
  is R decided "did not arrive" and R' published; each copy of the chain of 40 after R
  does not land.

## Excluded combinations

Never built, never accepted; each is a clause of a partition's `Feasible`:

- evidence that disagrees with the truth: "published" for a copy that did not land,
  "absence proven" for one that did;
- what r does after D when r ended before it, and a verified decision by r once r ended;
- a retired proof without r ended — at D or after it — and the next run of its name having
  dropped r's marks;
- a second command, or any effect, after evidence returned before D's write: no D exists;
- r reading or the sweep removing a letter that never landed, and the sweep promoting a
  mark that did not read at D with no repair since — its evidence returning;
- a new run of r's name while r lives, at D or after it;
- a decision on R' unless its evidence is unknown, and none missing on an unknown R';
- a resend of a decision verified by r when r ended before the decision;
- R' landing when r ended before its first attempt read r live (moot, nothing written),
  R' not landing when r ended once an attempt had admitted it live, and an unknown R'
  with r ended anywhere but after R' turned unknown;
- an unknown child R' unless R was decided "did not arrive".

## Clean runs and crash cuts

Every world runs clean, then once per durable step of its clean run with the process
killed there, and once more for each step a flush can fail at, with that flush failed. A
durable step is a rename, a removal or a directory or file flush of the product, or a
point inside one of its writes and publications, taken while a command of the schedule
runs. A write or a publication is four points, each a step of its own
(`state.Points`, `infra/state/write.go`): its temporary file written, that file's
contents flushed, the file placed under its name — renamed for a write, linked, failing
when the name is taken, for a publication — and the directory flushed. A kill at a point
stops the process before it: at the first three the name stays as it was and the
temporary file stays behind, as a stopped process leaves it; at the directory's flush the
record is in place and nobody learns of it. A failed flush answers an error without being
made: the temporary file's leaves the name as it was, the directory's leaves the record
visible, the error answered and the name taken. A rename or a removal flushed is made
and answers an error. The temporary file and the placing are only ever killed — the
flushes around them are the points that fail. The seam is the file access of the mail
package, put in place for one world's state directory alone (`matrixAccess`,
`decision_matrix_access_test.go`) rather than the process-wide fault seam, so worlds run
side by side; it logs the steps, makes one its cut, counts every letter written into a
mailbox by id from the moment it is in place — a letter read or swept, or whose
directory's flush failed, still counts — and plays the faults a world's evidence names on
reads. Every clean log is held to the four points of each write and publication, in
order and nothing between them (`checkWritePoints`), so a write cut only around the whole
call fails the run. After a kill the stopped process is gone: every later step of
it fails until the command runs again in one that is not — the barrier again, or the
same words of the deciding command — and the rest of the schedule follows. Before it
does, every decision, record beside one and installed journal in the room is whole or
absent (`checkWhole`). Each run is then held to the model by the play's checks
(`TestDecisionDurabilityAtEveryCut`); a chain's every generation has names as long as
the first's.

The cuts of a world are its clean log's durable steps killed, and those a flush can fail
at flushed, measured on the matrix's runs in the fix round of S18's third part, the
feasible worlds by the full run:

| Partition | Pair-cover worlds | Their cuts | Feasible worlds | Their cuts |
|---|---|---|---|---|
| `decision` | 23 | 999 killed, 565 flushed | 3198 | 177902 killed, 99112 flushed |
| `resend` | 41 | 3650 killed, 2022 flushed | 219 | 23280 killed, 12804 flushed |
| `mailbox` | 32 | 2831 killed, 1535 flushed | 52 | 4532 killed, 2460 flushed |
| `generations` | 12 | 1080 killed, 596 flushed; the chain of 40 clean | 12 | 5974 killed, 3246 flushed |

The full run on the final code of the fix round, on October 13, 2026: all 3481 feasible
worlds, 211688 cuts killed and 117622 flushed — 336277 passing tests and subtests, none
failing or skipped, in 1 hour 55 minutes.

Before the internal points, on the code of S18's third part, a step was a whole write or
publication, killed before it or made with an error answered after it: 3481 feasible
worlds and 70589 steps, each killed and flushed, 148145 passing tests and subtests in 41
minutes. That run left the temporary file's flush and the visible record whose
directory's flush failed out of every world (the review of part 3, G1). A world's room is made on the file system in
memory when the system has one (`/dev/shm`): every durable write of a room flushes, and
on a disk the same pair cover took four to six times as long. The flushes the seam logs
are not made — no world loses power, so they change nothing a world can see. For the same
reason a flush whose failure were ignored would leave the same outcome, so every run's log
is also held to the order a decision's durable marker stands on (`checkFlushedMarkers`):
the marker's publication begins right after the decision's file, its directory and its
mailbox were flushed, and never right after one of those flushes failed. Worlds and their cuts
run in parallel.

**What the five checks run.** The pair cover with every point of every write: the
internal points are not narrowed for them. Measured on October 13, 2026 with no other
heavy run on the machine, the pair cover of `decision`, `resend` and `mailbox` and the
ten short worlds of `generations`, every cut, takes 40 s, and 197 s under `-race`; before
the internal points it took 20 s and 98 s. Measured on October 12, 2026, before them: the
chain of 40 took 2.1 s clean, and its 764 steps were some 3000 cut runs — about 13
minutes without `-race`; cutting only its first two generations and its last still took
200 s, and under `-race` some 25 minutes for each of the three runs of the package the
five checks make. Main decided on October 12, 2026: the five checks run the pair cover
with every cut, except the chain of 40, which runs clean, every copy, decision, note and
closure checked; `REWAKE_DECISION_MATRIX=full` runs every feasible world and every cut,
the chain of 40 included, in the background before S18's final acceptance, with its log
and counts in the report. That decision chose which worlds the five checks cut, not
which points. The runner names the chain of 40 as the one world the five checks leave
uncut (`cleanOnly`) rather than taking a long log for it: with the internal points,
ordinary worlds pass the length only the chain passed before.

## Behind a link

A world apart from the partitions (`TestDecisionWorldsBehindLinks`): R's occurrence on
record, then api's mailbox or its stops moved aside and a link put in its place,
dangling or to the records themselves, or the directory of every mailbox replaced by a
dangling link. The stops read as absent — no subject — or as unreadable: `decide`
refuses, writes no decision on either side of the link and publishes nothing, and with
the directory back the same words decide. A link above the mailbox to the mailbox's own
records is not such a world: the lock and the listings refuse a link at the mailbox's own
level, not at every ancestor, so `decide` then decides on the records the link reaches.
Nothing promises more (part 2's review); the observation is recorded with S18's report.
