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

The model's twelve dimensions do not all interact. A product of all of them would test
the same schedule many times over, so they are split into four partitions, each a product
of the dimensions that do interact, with the rest held at one value:

| Partition | Holds fixed | Dimensions | Product | Feasible | Pairs | Pair cover |
|---|---|---|---|---|---|---|
| `decision` | one report in J, no resend run, nothing else unknown | R's truth, R's observation at the decision, R's observation later, the choice, a second command, who decides, activity in r's mailbox, the sender's life | 6480 | 1590 | 252 | 21 |
| `resend` | R decided "did not arrive" by r, nothing else unknown | the truth of R' with its evidence, the second choice, r's end along J''s path, the sender's life | 729 | 111 | 115 | 41 |
| `mailbox` | R unknown and decided, r alive, the sender alive | the rest of the mailbox, the second report in J, the choice | 64 | 52 | 53 | 32 |
| `generations` | every copy decided by r, every unknown at a mark | the generations with the truth of R' and the second choice, R's truth | 12 | 12 | 12 | 12 |

**Feasible** is the clean worlds: 1765 in all. **Pairs** is how many pairs of values some
feasible world of the partition holds; **pair cover** is how many worlds the generated
set has that holds every one of them — 106 in all, the set the five checks run. It is
chosen greedily in a fixed order from the feasible worlds and verified (`Covers`), never
written by hand.

## Excluded combinations

Never built, never accepted; each is a clause of a partition's `Feasible`:

- evidence that disagrees with the truth: "published" for a copy that did not land,
  "absence proven" for one that did;
- a retired proof without r ended and the next run of its name having dropped r's marks;
- a second command, or any effect, after evidence returned before D's write: no D exists;
- r reading or the sweep removing a letter that never landed, and the sweep promoting a
  mark that does not read;
- a new run of r's name while r lives — and so a verified decision by r after r ended;
- a decision on R' unless its evidence is unknown, and none missing on an unknown R';
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
