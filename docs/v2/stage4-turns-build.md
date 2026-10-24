# Stage 4: the rules and the build of S4-6

The second half of [stage4-turns.md](stage4-turns.md), written October 16, 2026 on branch
`s4-6`: the clauses S4-6 brings into `docs/rules/`, the three parts of its build with the tests
each adds, and what the live acceptance runs. The design itself is the first half.

## The rules

What S4-6 brings into `docs/rules/`, each clause in the group it belongs to; a clause no test
holds yet is stated in its rule's text as a known limit, dated, owned by S4-6's build, since
the rules test refuses a `Gap:` line and a named test that does not exist. The build moves
each into its rule's `Tests:` list with the test that holds it.

- **O1** (`turns.md`): the limit of an unheld interim end on Claude Code.
- **O2** (`turns.md`): the reasons mapped from `turn.complete`, a subagent's turn not sent.
- **E7** (`effects.md`): a retried end reuses its first capture; one with the same id and
  another digest is refused; a run proven gone answers no receipt, and an ownership that could
  not be read is retried.
- **The live set** (`rules/adapter-api.md`): a boundary proven after the start is not awaited before it;
  availability waits for it within its bound; a lapse withdraws what the mod proved, a new hello
  restores it, a hello of a superseded or earlier load and a report of an earlier transition
  change nothing; an end after a lapse is taken.
- **The capabilities** (`adapter-api.md`): a reporting role launches on Claude Code; a known
  cause refuses it; a launch through `--command` that does not exec is never admitted.
- **SP4** (`space.md`): the peer checked before each effect of a start, an end and a poll.
- **L3** (`launch.md`): the outbox of a killed run kept in the harness's store until a later
  load proves the run ended.

## The build

Three parts, each closed by its acceptance; the steps' table carries them
([stage4-space-steps.md](stage4-space-steps.md#the-build-steps)).

**Part A — the turns.** The schema's endpoints and bounds; `modhost/turns.go`, the dispatch
table and `modhost/hello.go`; `modRun` as `TurnBoundary`; the offer and `AfterStart`; the known
causes and the launch's refusal; availability waiting for a boundary proven after the start;
ownership read three ways and `ErrRunGone`; the mod's `turn.start`, `turn.complete`, outbox,
its one sender, retry chain, cut and the store's cleanup by proof. Tests it adds:

- `modhost`: each reason maps to its end reason; an end without an admitted hello is refused;
  a failed `Publish` answers 503 and the retry under the same `endId` hands the core the same
  capture — boundary, `Started`, `Ended`, event; a lost receipt and a retry after later reads
  and marks answer only what the first capture saw; two copies of one end at once capture once;
  a known `endId` with another digest is refused; a request delayed across `Close`, new or of a
  cached capture, publishes nothing; a begun `Publish` holds the lease; a body past
  `maxEndBodyBytes` is refused 413; a start records no timed start; `mod/ended` answers only
  the runs `ObserveRun` proves ended.
- `core`: ownership read as owned, proven not owned or unknown; a run whose name passed to a
  later run gets `ErrNameLost` and no receipt, the mod's end answered 410; a record that cannot
  be read answers 503, and the retry once it can be read publishes once; an end taken whose
  receipt was lost is answered from the journal; the fixture's program sees `ErrRunGone` as it
  saw the nil.
- `adapter/claude`: the offer reports turns and does not hold; the known causes refuse a
  reporting role and leave a silent role with a note; the set is Wake until the hello and Wake
  with TurnBoundary after it, and with Spaces on main.
- `host`: a reporting role on a boundary proven after the start starts its harness, and a task
  to it fails until the hello; availability waits for the boundary within its bound; the
  fixture's wait before the start is unchanged, with its negative control.
- `cli`: the refusal of a reporting role still covered by a test harness that reports none, now
  that no catalog harness lacks a boundary.
- The mod's own tests in `claude plugin test`: a subagent's `turn.complete` is not sent, nor,
  since October 17, 2026, its `turn.start` (an empty agent id included), while the main loop's
  pair goes; the
  first attempt is awaited; an end that comes while an older one is in flight queues behind it;
  a 503 and a 200 that is not the end's receipt are retried, a 409 and a 410 dropped; a long
  answer is cut with its line at the exact bound, for quotes, backslashes and characters outside
  ASCII; the outbox survives another session's start and its own reload, and a reload during
  the store's cleanup loses no end; only the keys of runs `mod/ended` answers ended are removed.

**Part B — the heartbeat.** `mod/alive`, loads and generations, the lapse watch and the
withdrawal. Tests it adds: a lapse withdraws TurnBoundary, and Spaces on main, leaving Wake; a
new hello restores them; load A's hello, held before it binds while B's hello with a higher
`loadSeq` binds, is refused 410 when released, and so is A's first hello arriving only after B's,
and B stays current; a hello of another load id with the current number is bound as the later; a hello retried after its answer was lost keeps the
generation and reports nothing; a poll delayed across a new hello of the same load is refused
409 and its chain ends; a poll of a superseded load is refused 410; a lapse timer and a live
report of an earlier transition do nothing after a later one; an end after a lapse is taken and
makes nothing live; a lapse after the close reports nothing; the mod's chain follows the
heartbeat's table of answers, each load takes the next `loadSeq` of its run's counter and keeps
it across its hello's retries, and a load refused 410 that holds the highest number it can read
takes the next and says hello once more.

**Part C — live**, on the installed Claude Code, by main, in a separate `REWAKE_DIR`: main sends
a task to a worker started by hand with `rewake claude --write` and reads its report; the same
through a wrapper script given with `--command` that ends in `exec`, and one that does not,
whose worker refuses the task; a worker whose mod stops polling — its module in the run's
directory replaced by one that registers nothing, which an interactive session's hot reload of a
`--plugin-dir` loads (the skill, `official/troubleshoot.md`) — has its boundary withdrawn within
30 s and a task to it refused, and has it back once the module is restored; an interrupted turn
reported as stopped; a subagent's turn not reported; whether the engine runs `turn.complete`'s
hooks before the next turn starts; that the heartbeat's chain keeps polling between turns and
through a reload; how long the hello takes after the harness starts, which sets
`AfterStartBound`.

Part C passed on October 16, 2026, on Claude Code 2.1.296, by main and the writer, with
`.scratch/v2-s4-build/live-s4-6c.sh`; what it saw is recorded in
[research-claude-mod.md](../research-claude-mod.md#s4-6-a-workers-turns-through-the-mod), and
the hooks' order at a turn's end stayed unobserved.

The workflow suite's columns stay the fixture's alone (`test/workflow/column_test.go`), and the
confirmation scenarios keep the fixture, which holds an end; Claude Code's turns are held by
the tests above and by part C.
