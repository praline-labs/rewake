# The workflow suite in three minutes instead of twenty — September 25, 2026

The full suite had grown to about twenty minutes, and the owner waited on it every
round. A measurement before the change (review-claude, on a copy of a59a42d) found why:
the cases ran one after another and spent most of their time waiting on timers of the
product or the fixture, and eight copies of the suite run as separate processes finished
in about two minutes with every case passing.

## What was done

- **Cases run side by side, in one process.** A scenario enters through `enterScenario`,
  which joins a pool (`test/workflow/pool_test.go`). The pool reads go test's `-parallel`
  as its width, six by default, and lifts go test's own limit, whose slots go out in the
  order paused tests happen to wake; the pool starts the longest known scenarios first
  instead. A test whose columns are its subtests is made parallel itself and holds no
  slot. A scenario that changes process-wide state enters through `enterSerialScenario`
  and runs before the pool starts: the schema case, which sets the session's variables
  with `t.Setenv`. The tests that shorten the termination budget are not scenarios and
  were serial already. The crosswise checks enter through `enterScenarioAround`, and
  each pair joins the pool as a case of its own.
- **Owner labels instead of descent.** Cleanup used to take every descendant of the test
  process as the running case's, which is only true one case at a time. Every process a
  case or a bounded preparation starts now carries a variable of its own name in its
  environment — a name rather than one value of a shared name, because `os/exec` keeps
  only the last value of a repeated key, and a nested scope needs both — and a sweep
  ends only the descendants carrying its label (`owner_labels_test.go`). The processes
  the suite started and whose exit status their own Wait collects are in a registry, and
  no sweep reaps one of them; a case's cleanup of its own group still can, on paths
  that are red already. After the last case `TestMain` sweeps everything still
  below the test process, names it and fails the run: the guarantee that nothing a case
  started survives the run moved there, and is not gone.
- **The shim scripts are replaced, not rewritten.** A case installs its shims again for
  every session it starts, while an earlier one runs them; a script open for writing
  fails an exec with ETXTBSY. They are now written beside and renamed over.
- **Five product waits are shorter in the suite's build**, set with `-ldflags -X` through
  the new `internal/buildtime`, with no test branch in the product: the coalescing quiet
  and cap (already so, now 2 s and 3 s instead of 1.5 s and 2 s, below), the pickup of a
  steering request (5 s to 1.5 s), a compaction
  letter's wait for its other half (3 s to 1.5 s), and main's scan for notices (1 s to
  250 ms). [testing-pool.md](../testing-pool.md#waits-the-suite-shortens) lists them.
  The pickup and the letter's wait are 1.5 s rather than the 1 s the measurement
  suggested, because the cases now run on a loaded machine. A test reads every `-X`
  target from its package's source, since the linker ignores one that names nothing.
- **Fixture and test windows**: the steered fixture's compaction takes 3 s instead of 5,
  with the command's answer due within 2 s instead of 3; the absence of main's compaction
  notice is waited for a scan, the cap and 1.5 s instead of 5 s; the interrupted case's
  and the orphaned letter's fifteen-second ceilings are now the waits they bound plus a
  margin; the inbound wait after a final word is 2 s plus the cap instead of 3 s plus the
  cap; the mid-turn hold window is 4 s instead of 10.
- The widened-window mutant stays at ten seconds. Cut to five, it broke its control in
  both columns: its first two sends wait out `send`'s five seconds for a status before
  the third letter leaves, so the window has to outlast that.
- The suite's coalescing window grew from 1.5 s of quiet and a 2 s cap to 2 s and 3 s.
  With six cases at a time, the sends that write batch-arrival's heads-ups, 600 ms apart,
  started over a second late — their ids carry the time — and one crosswise run in four
  had the third heads-up miss the cap and go alone. Three crosswise runs after the change
  were green; the full run paid about 25 s for it.
- `TestNotesSecondsApartAreAnnouncedOnce` in `internal/inbox` failed once in the five
  checks: under `-race` beside the other packages its 700 ms sleep took 1.9 s, past the
  test's own one-second quiet. The gap is now 1.1 s under a 2 s quiet and a 3.5 s cap,
  so a stall has room before it closes the window early.
- Mutant builds are not shared between cases. The measurement put the saving at about
  four seconds of case time in a full run; with the builds overlapping other cases'
  waits, that is under a second of wall time, not worth a suite-wide cache and its
  locking.

## Evidence

On September 25, 2026, each run started with no other suite or checks in the process
list; the baseline ran while this session edited files and ran vet and staticcheck.

| Run | Before (1e37250) | After |
| --- | --- | --- |
| full suite, 102 cases | 19m22.2s, 99 passed and 3 unsupported | 3m10.2s and 3m06.0s at six, 2m23.3s at eight, 99 passed and 3 unsupported each time |
| sum of case durations | 1155 s | 1054–1065 s |
| crosswise, 75 cases | 7m01.9s | 1m17.4s, 1m17.5s and 1m17.8s at six, 75 passed each time |
| memory in use, peak rise | — | 0.4–0.7 GB at six, 0.7 GB at eight |

The first full run after the change was red only on the widened-window control, cut to
five seconds as above; with it back at ten the run was green. With the window at 1.5 s
and 2 s the full run took 2m41s and 2m46s at six and 2m09s at eight, and the crosswise
check 1m16s, but was red once in four runs, as above. Two tests in the five
checks hold the labels: one case's sweep ends its own escaped descendant, leaves its
neighbour's running and lets the neighbour's Wait collect its exit status; an unlabeled
process is left by every case's sweep and ended by the final one. Both fail with the
label check taken out.

## Review

review-claude reviewed the change with no blockers: two full runs of 3m05s and 3m09s and
a crosswise run of 1m25s, all green; neighbours' processes untouched; the shorter timers
only tighten what the cases assert. Four findings, fixed:

- A process that outlived every case failed the run while barely naming it: the
  summarizer printed only that go test exited 1 with no case reporting a failure, the
  summary file said nothing, and go test's own output gave a pid that was dead by then.
  A sweep now describes each stray by its command line and owner labels before
  signaling it, and the final sweep's finding goes into the run record, which the
  summarizer prints as the run's failure. A test in `tools/checksummary` plants an
  unlabeled process in a real run of the suite's package and reads its command line in
  the summary. Writing it turned up an older hazard: a stray started without a group of
  its own shares the test process's group, which is also go test's and its caller's,
  and the sweep signaled that group — the first run of the test ended the shell that
  started it. A sweep no longer signals its own process's group. And the final sweep
  counted a find only if it was still running after the signal, so a leftover that died
  at SIGTERM was ended without being named and the run passed — the label test failed
  on it once under load. The final sweep now names whatever was running when found.
- `-parallel 0` held every case until `-timeout`: go test's own check of the width never
  runs once the pool has lifted the limit. A width below one is refused before the run.
- `testing.md` passed 400 lines; the pool, the owner labels and the shortened waits moved
  to [testing-pool.md](../testing-pool.md).
- The wording above about which waits are 1.5 s, and the registry's comment, which
  promised more than a case's cleanup of its own group keeps.

## What stays open

- The pool's order is a list of scenario-name prefixes with the durations measured that
  day; a scenario that grows long is not moved forward until it is added there.
- The shortened product values are held at their real length by the unit tests only.
