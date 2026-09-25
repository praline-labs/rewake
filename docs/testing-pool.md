# The workflow suite side by side

How the workflow suite runs its cases at the same time, how a case's cleanup keeps to
its own processes, and which of the product's waits the suite's build shortens. The
commands are in the [Checks section of AGENTS.md](../AGENTS.md#checks); the tiers and
the rest of running the suite are in [testing.md](testing.md#running-it).

## Choosing the width

The suite reads go test's `-parallel` as the width of its own pool
(`test/workflow/pool_test.go`), six when it is not given. The pool starts the longest
scenarios first, so that none of them runs alone at the end; go test's own limit is
lifted, because its slots go out in whatever order paused tests happen to wake. A width
below one is refused before any case starts: go test's own check of the flag never runs
once the limit is lifted, and a pool of none would hold every case until `-timeout`.

Six is the default because it leaves room for another session's build or checks beside
the run. On a machine with nothing else running, `-parallel 8` is faster; a machine
already busy with another heavy run takes `-parallel 4` or waits. On September 25, 2026
the full suite took 3m06s and 3m10s at six and 2m23s at eight, and at eight added well
under a gigabyte of memory in use. A case mostly waits on a timer, so the width is
bounded by memory and by how far the timings of a loaded machine can stretch, not by
processors.

A scenario enters through `enterScenario`, which joins the pool. One that changes what
the whole test process shares — its environment through `t.Setenv`, a package variable
such as the termination budget — enters through `enterSerialScenario` and runs before
the pool starts, as does every test that is not a scenario. A test whose columns are
its subtests is made parallel itself and holds no slot, and a test under one that holds
a slot runs inside it: a parent waiting on subtests that wait on the parent's slot would
never finish.

## Owner labels

Every process a case starts carries a variable of its own in the environment,
`REWAKE_WORKFLOW_OWNER_<pid>_<n>=1`, which its descendants inherit through setsid and
re-parenting alike, and a case's cleanup ends only what carries its label
(`test/workflow/owner_labels_test.go`). The label is a name of its own rather than one
value of a shared name, because `os/exec` keeps only the last value of a repeated key
and a nested scope — a build inside a self-check child — needs both.

A process whose exit status a case's own Wait is to collect is in a registry, and no
neighbour's sweep reaps it. The registry does not cover a case's own cleanup of its own
group: `reapGroup` collects any member of the group it is ending, the leader included,
and on a path that is already red the leader's Wait can then answer with an error
instead of the status.

After the last case, `TestMain` sweeps whatever is still below the test process —
anything no case accounted for — ends it, and fails the run, whether or not it would
have ended at the first signal. So "nothing a case started
outlives the run" still holds, checked once at the end rather than by each case for all
of them. Each leftover is named by its command line and its owner labels, read before
it is signaled, because by the time anyone reads why the run failed the process is
gone; the finding goes into the run record, so `tools/checksummary` prints it as the
run's failure, and a sweep never signals the test process's own group, which is go
test's and its caller's as well.

## Waits the suite shortens

The suite's binary runs five of the product's waits shorter than a release, set at
build through `-ldflags -X` on string variables read by `internal/buildtime`, for the
binary under test and every mutant alike (`suiteFlags` in
`test/workflow/timings_test.go`). A release sets none of them, and the code is the same;
only these numbers differ, so the suite does not run these values at their real length:

| Value | Release | Suite | Where |
| --- | --- | --- | --- |
| coalescing quiet | 3 s | 2 s | `internal/inbox`, `builtQuiet` |
| coalescing cap | 4 s | 3 s | `internal/inbox`, `builtCap` |
| pickup of a compact or interrupt request | 5 s | 1.5 s | `internal/cli`, `builtPickup` |
| a compaction letter's wait for its other half | 3 s | 1.5 s | `internal/wrap`, `builtLetterWait` |
| main's scan for other sessions' notices | 1 s | 250 ms | `internal/wrap`, `builtNoticeScan` |

Each is waited out in dozens of cases, and at the real length they added minutes to a
run while proving nothing the shorter ones do not. The real values are held by the unit
tests, including one that keeps the cap a second inside `send`'s five-second wait. A
scenario that times something against one of them takes the suite's constant
(`suiteCap`, `suiteLetterWait`, …) rather than a number of its own. The linker ignores
`-X` for a variable that does not exist, so a test reads each target from its package's
source and fails when one has been renamed away.
