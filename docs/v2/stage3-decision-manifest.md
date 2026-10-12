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
name; a world is that order with its values.

- `decision`: J writes R or not; R's evidence turns unknown; r ends, when it ends before
  the decision; the decider writes D at its linearization point, verified by r only while
  r lives; D is installed in the next barrier, J' with it and not run (the `resend`
  partition runs it); a second command, when there is one, after D; R's later evidence
  after D, or before D's write when it returns before it, and then no D; r's end after D,
  when it ends after it, past D's installation; the activity in r's mailbox after D,
  except that r reads R at any time it lives and a new run of its name comes only after r
  ended; the sender ends before D's effects are installed or after, or not at all.
- `resend`: R's evidence unknown; the decider writes D "did not arrive" — the person,
  when r ended before it — and the next admitted barrier installs and runs J'; r ends at
  the point of J''s path its value names, or not at all; R' is decided only once it is
  unknown, the second choice in the barrier after; the sender at its point; R's own
  evidence stays unknown and r's mailbox sees no activity.
- `mailbox`: R's evidence unknown; r, verified and alive, writes D; the rest of the
  mailbox is ordered before or after J as its value names and settled after D or left;
  r and the sender live throughout; R' publishes on its first attempt unless the rest
  names it unknown.
- `generations`: each copy turns unknown at its mark and is decided by r, verified and
  alive, before the next copy runs in an admitted barrier; r and the sender live
  throughout.

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

Every feasible world runs clean, then once per durable write of its clean run's seam log
(`rewakefault`), the process killed there and once more with that write's directory
flush failed, the barrier run again and the oracle checked. The cut count of a world is
the number of durable writes its clean log holds, so it is measured, not assumed: it is
published here, per partition, from the matrix's first run in S18's third part, with the
clean count above as its base. The five checks run the pair cover with its cuts; the full
set runs before S18's final acceptance and at the stage's. If the full set does not fit
where it is required, the measured cost and a proposed partition go to main before the
gate changes.
