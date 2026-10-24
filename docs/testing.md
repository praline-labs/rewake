---
description: How rewake is tested — the tiers and what each proves, the fault build, the fixture column and its readiness exchange, running the suite side by side with its shortened waits, the checks before a publication, reading a result, extending the suite, and the traps it has paid for.
---

# Testing rewake

The one place that says how this project is tested: what each tier runs, what it proves and
what it does not, how to run it, how to read what comes out, how to check a new harness version
before updating, and how to extend the suite. The commands of a commit and of a step are in the
[Checks section of AGENTS.md](../AGENTS.md#checks); what each workflow case claims is in
[testing-cases.md](testing-cases.md).

## The tiers

| Tier | Runs | Proves | Does not prove | Costs |
| --- | --- | --- | --- | --- |
| The step's checks | the hook's formatters and linters, one `go test -race -shuffle=on ./...` | unit invariants: parsing, publishing races, liveness, inbox order and expiry, the context endpoint's framing, the import rule; the suite's own classifier and summarizer | anything end to end: every workflow scenario skips itself | about two minutes of the race run at `-p 4` on an idle machine, compilation aside; `core/mail` is the longest package. No network, no harness, no container |
| Workflow suite | a built rewake end to end against the fixture harness, in one column | that the shared service code delivers, groups, steers and reports as each scenario claims | that a real harness parses, renders or behaves as its fixture does | about half a minute for 24 scenarios, six cases at a time; no network |

Two tiers are not built. **P**, a real harness of a named version against a local responder
instead of a model provider, would catch a change of behaviour that a schema cannot see. **M**,
a real model through a real harness, paid, would show that a model actually reads its mail and
acts on it. Live checks of a harness are run by hand meanwhile and recorded in the research
documents. Whoever runs a real harness, by hand or in a future tier, keeps these rules:

- **The cheapest model, always** (owner, September 21, 2026). A run that starts a real harness
  with a real model sets, in that run's environment, the cheapest model available and the
  lowest reasoning effort. The values come from the environment of the run; they are not in
  this repository, and neither is any model name. Such a run exists to see whether a model read
  its mail and acted on it, not to judge how well it works: the weakest model answers that, and
  the cheaper it is, the more often a run can be afforded. So an unset value is a reason not to
  run the case, not to fall back to the harness's default, which picks an expensive model.
  rewake takes the model and effort of a launch from the environment
  ([launch-defaults.md](launch-defaults.md)), so a case sets two variables and touches no
  harness configuration.
- **Isolation per tier.** Every tier runs in a fresh private HOME, configuration and state
  directory, with no owner credentials, configuration or session mounts, no inherited identity
  or proxy, and verified binaries. The free tiers may put a shim for the harness on `PATH` in
  place of separate network, mount and PID boundaries: no real credentials are near and a
  substituted harness does not reach outside. Those boundaries stay mandatory for a paid run,
  where real credentials live alongside. Failing to establish the isolation a tier requires
  stops the case; it never falls back to the real account.
- **`not-run` is never acceptance.** A case not run, for any reason, is reported as such and
  never counted as passed. No typing proxies or automated input into a person's screen, and a
  protocol client driving a synthetic case is never labelled a real terminal.

### The fault build inside the test run

A binary built with the `rewaketest` tag reads `REWAKE_FAULT`
(`internal/infra/state/fault_build.go`) and logs, ends, fails or holds its process at a durable
step, or leaves behind a process that keeps its output open past a kill. A release build never
reads the variable.

The lease rig (`test/leaserig`) builds rewake twice the same way, the second with a build time
passed in, so the two builds share a version and differ in content, and races the room's build
lease between them and a byte-identical copy: each process held at a named step of the lease
([rules/state.md](rules/state.md)), released or killed by the test. It runs in every test run,
untagged, since a launch it starts never gets past its lease.

The tag also admits one narrow kind of test code into a production package. When a fault
schedule must cross a package boundary and neither an exported operation, a file fixture nor
`state.Fault` can build it, the package that owns the step may carry exported test operations
in a file built only under `rewaketest`, and the tests in the other package that use them carry
the same tag. They run in the additional tagged race run; an untagged build has no such
operation. The only such file is `internal/core/mail/faultops.go`: a waiting copy's removal
that fails.

### The fixture column, the gate

The suite runs one column: the fixture harness, a harness of its own built only under the
`rewaketest` tag, which the suite's rewake is built with
([its exchange](#the-fixtures-readiness-exchange)). It proves the core through the adapter
contract, and its program can withhold each capability. **The fixture column is the regression
gate**: a red there blocks, and an absent capability there is red too, because a gate that
stopped checking must not look green. A column of a real harness beside it would be a search
column — a red there a finding to investigate, an absent capability reported as `unsupported`
by name. The gate is a field of the column (`gate` in `test/workflow/column_test.go`), set on
exactly one; the summary names the column of every result and promotes none.

### The fixture's readiness exchange

The fixture harness (`internal/adapter/fixture`, id `fixture`) runs the program `fixture` the
suite puts on the run's `PATH`, with the launch description as flags it parses strictly; a
`fixture --version` below `1.0.0`, or unreadable, refuses the launch. What is live for a run is
what the program proved, and only that: a test switches liveness, never the offered set.

1. **Hello.** The adapter listens on the run's socket and passes its path as a flag; the
   program sends one hello — its version, pid, conversation and the capabilities it serves
   (Wake, TurnBoundary, Telemetry, Control). It is accepted only from the process the adapter
   started, by the peer's pid and start time, and only once.
2. **Probes.** One probe with no side effect per served capability, answered within the
   readiness bound: Wake a probe notice acknowledged without showing it, TurnBoundary the turn
   state, Telemetry the last sample or none, Control "ready" without acting. An answered probe
   makes its capability live; a late or wrong answer leaves it out, the others not.
3. **Withdrawal.** A closed socket withdraws every live capability; a reconnect is a fresh
   hello and fresh probes from the same process; the program's exit ends the run.

After the probes the adapter hands the program its briefing (`brief`), and a control request
reaches it as `control` while Control is live. The adapter reports the live set after the
exchange and at each change (`ReportLive`); the host records it in the session record, and a
worker's launch waits for its TurnBoundary up to the readiness bound. The suite's switches
(`test/workflow/fixtureshim_test.go`): `RW_SHIM_NO_WAKE`, `RW_SHIM_NO_BOUNDARY`,
`RW_SHIM_NO_TELEMETRY` and `RW_SHIM_NO_CONTROL` leave a capability out of the hello;
`RW_SHIM_FAIL_PROBE=<capability>` serves it and fails its probe; `RW_SHIM_DROP_AFTER=<turns>`
closes the socket after that many turns; `RW_SHIM_HELPER_REQUESTS` sends the hello from a
child, which the peer check refuses; `RW_SHIM_NO_HOLD` reports ends that cannot be held.

## Running it

Three levels (owner, October 17, 2026): a commit while a step goes on builds, vets and passes
the tests of the packages it touched; the last commit of a step, before it lands on `v2`,
passes one `go test -race` over the whole tree and the workflow scenarios the change touches;
the full set — every build variant, vet and staticcheck over each, the tagged `-race` runs and
the whole workflow suite — runs only before a publication to npm. A step is done when it works
live and breaks no existing test.

Formatting and linting run on every commit, by the pre-commit hook (`lefthook.yml`): Biome over
the staged TypeScript, `golangci-lint fmt` over the staged Go, then `go build`, `go vet` and
`golangci-lint run` over the module. `biome.json` sets the owner's style — spaces, indent 2,
line width 100, double quotes, the recommended rules, imports organized — over the shell, the
mod and pi's extension, leaving out the generated `protocol.gen.ts`, which stays as
`tools/protogen` writes it. Its three overrides each name the files they need: the shell's
`main.tsx` has no accessibility rule (OpenTUI has no accessibility tree or roles), the shell's
escape-sequence parsers may write control characters in a regular expression, and the mod's
test doubles may be `any`, standing in for the host's untyped objects.

**Every `go test`** clears `REWAKE_SESSION`, `REWAKE_EPOCH`, `REWAKE_DIR` and `REWAKE_ROOM`,
the workflow suite's included, because a test that inherits them reads the live session's state
as its own. The unit tests start no harness, no container and no network request, whatever else
the environment says: every real-harness step is behind the suite switch.

**The workflow suite** is switched on by `REWAKE_WORKFLOW=1`; without it every scenario skips
itself, and with it set and none selected the run fails. Run it through `tools/checksummary` to
read a result, which prints a few lines and writes the rest to a file; run it with `go test -v`
to watch it, and keep the `-v`: `go test` prints nothing a passing package printed, so without
it the line naming how many scenarios ran is invisible. `-count=1` keeps a cached pass from
standing in for a run. On the last commit of a step, run the scenarios the change touches,
selected with `-run`; the whole suite runs after a change to the fixture and before a
publication. `REWAKE_WORKFLOW_SELFCHECK` is not for people: the suite sets it on a child of
itself to prove that a scenario missing an observation turns the run red.

### Side by side

The suite reads go test's `-parallel` as the width of its own pool
(`test/workflow/pool_test.go`), six when it is not given. The pool starts the longest scenarios
first, so that none of them runs alone at the end; go test's own limit is lifted, because its
slots go out in whatever order paused tests happen to wake. A width below one is refused before
any case starts: go test's own check of the flag never runs once the limit is lifted, and a
pool of none would hold every case until `-timeout`.

Six leaves room for another session's build or checks beside the run. On a machine with nothing
else running, `-parallel 8` is faster; a machine already busy with another heavy run takes
`-parallel 4` or waits. A case mostly waits on a timer, so the width is bounded by memory and
by how far the timings of a loaded machine can stretch, not by processors.

A scenario enters through `enterScenario`, which joins the pool. One that changes what the
whole test process shares — its environment through `t.Setenv`, a package variable such as the
termination budget — enters through `enterSerialScenario` and runs before the pool starts, as
does every test that is not a scenario. A test whose columns are its subtests is made parallel
itself and holds no slot, and a test under one that holds a slot runs inside it: a parent
waiting on subtests that wait on the parent's slot would never finish.

### Owner labels

Every process a case starts carries a variable of its own in the environment,
`REWAKE_WORKFLOW_OWNER_<pid>_<n>=1`, which its descendants inherit through setsid and
re-parenting alike, and a case's cleanup ends only what carries its label
(`test/workflow/owner_labels_test.go`). The label is a name of its own rather than one value of
a shared name, because `os/exec` keeps only the last value of a repeated key and a nested scope
— a build inside a self-check child — needs both.

A process whose exit status a case's own Wait is to collect is in a registry, and no
neighbour's sweep reaps it. The registry does not cover a case's own cleanup of its own group:
`reapGroup` collects any member of the group it is ending, the leader included, and on a path
that is already red the leader's Wait can then answer with an error instead of the status.

After the last case, `TestMain` sweeps whatever is still below the test process — anything no
case accounted for — ends it, and fails the run, whether or not it would have ended at the first
signal. So "nothing a case started outlives the run" holds, checked once at the end. Each
leftover is named by its command line and its owner labels, read before it is signaled; the
finding goes into the run record, so `tools/checksummary` prints it as the run's failure, and a
sweep never signals the test process's own group, which is go test's and its caller's as well.

A case's own sweep is laxer: it names a descendant only if it still runs after the signal, so
one that ends at once is not a finding. Its regression test
(`TestACaseSweepLeavesItsNeighbourAlone`) gives its escaped descendant a fifth of a second to
end on SIGTERM; with a bare `sleep` it raced the kernel. A sweep calls the tree quiet only after
a pause of `straySettle` with nothing found, counted from its start, from the end of the case's
own group and from its last find: a descendant ending on the signal releases its memory before
it becomes a zombie, and in that moment `/proc/<pid>/environ` reads empty, so it carries no
label and is not found while it still runs.

### Waits the suite shortens

The suite's binary runs five of the product's waits shorter than a release, set at build
through `-ldflags -X` on string variables read by `internal/infra/buildtime`, for the binary
under test (`suiteFlags` in `test/workflow/timings_test.go`). A release sets none of them, and
the code is the same; only these numbers differ, so the suite does not run these values at
their real length:

| Value | Release | Suite | Where |
| --- | --- | --- | --- |
| coalescing quiet | 3 s | 2 s | `internal/host/delivery`, `builtQuiet` |
| coalescing cap | 4 s | 3 s | `internal/host/delivery`, `builtCap` |
| pickup of a compact or interrupt request | 5 s | 1.5 s | `internal/cli`, `builtPickup` |
| a compaction letter's wait for its other half | 3 s | 1.5 s | `internal/host`, `builtLetterWait` |
| main's scan for other sessions' notices | 1 s | 250 ms | `internal/host`, `builtNoticeScan` |

Each is waited out in dozens of cases, and at the real length they added minutes to a run while
proving nothing the shorter ones do not. A scenario that times something against one of them
takes the suite's constant (`suiteCap`, `suiteLetterWait`, …) rather than a number of its own.
The linker ignores `-X` for a variable that does not exist, so a target renamed away leaves the
suite's binary at the release value without a word.

### Before a publication to npm

The release has no build tag. The test-only `rewaketest` tag enables both the fault seam and
the fixture harness; no release needs either. The lease rig uses the same tag, though it stops
launches before their harness starts.

Run the ordinary checks over the whole module. Run the additional tagged vet, staticcheck and
race checks only over packages with tagged files — today `internal/adapter/catalog`,
`internal/adapter/fixture`, `internal/cli`, `internal/core/mail`, `internal/host/delivery` and
`internal/infra/state`. This includes tagged test files and negative build constraints, so the
release's catalogue without the fixture is checked in the ordinary run and the fixture's
presence in the tagged run. The locked-caller check in `internal` still loads the whole module
in both variants. The list is derived from build constraints, not maintained separately:

```bash
mapfile -t tagged_packages < <(
  rg -l '^//go:build .*rewaketest' --glob '*.go' --glob '!archive/**' |
    xargs -r -n 1 dirname | sort -u | sed 's|^|./|'
)
# A missing tag must not turn the additional check into a check of no packages.
((${#tagged_packages[@]})) || exit 1
go vet ./... && go vet -tags rewaketest "${tagged_packages[@]}"
staticcheck ./... && staticcheck -tags rewaketest "${tagged_packages[@]}"
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  go test -race -shuffle=on -timeout 20m -tags rewaketest "${tagged_packages[@]}"
```

Run the whole workflow suite as well. The tools install into `~/go/bin` and the active Node's
global `bin`, both on `PATH`; the hook's two are pinned, and `lefthook install` runs once per
clone:

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest mvdan.cc/gofumpt@latest \
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
go install github.com/evilmartians/lefthook/v2@v2.2.2
npm install --global @biomejs/biome@2.5.15
lefthook install
```

The release gate's `scripts/pack.sh` also needs [Bun](https://bun.sh), 1.4 or newer: it
compiles the shell for each platform package.

## Checking a new harness version before updating

Claude Code's launch reads the installed version before the claim and refuses one below its
minimum; a new version's behaviour is checked live, with the `mods` skill of that version for
the mod, and recorded in [research-claude-mod.md](research-claude-mod.md) and
[research.md](research.md).

pi is shipped with rewake at a pinned version (`PinnedVersion` in
`internal/adapter/pi/ship.go`, read by `scripts/pack.sh`), and a launch of the shipped copy
refuses any other. Before raising it, compare the new package's extension declarations and
changelog with the pinned one's and rerun the live checks of
[research-pi.md](research-pi.md#the-bundled-pi) on it, with the new copy installed where a
build finds it ([install.md](install.md#trying-20-beside-103)).

## Reading a result

The summarizer prints, and its exit code says, 0 green, 1 red, 2 a wrong call, 3 a stream it
could not read (`go run` reports every non-zero exit as 1 and prints the real one; build the
binary to keep them apart). A green run prints at most twelve lines, a budget its own test
holds; the rest — every case and every observation, with its outcome, duration and evidence
path — is in `summary.json`, so the next diagnostic is chosen from it without loading
successful transcripts:

```
workflow  24 scenarios, 24 cases: 23 pass, 1 fail   41.2s
against   scenarios against the suite's own harness programs in every column
FAIL  batch-arrival/fixture   fail
      fail        "the recipient can receive mail before the letters leave"
        worker-fixture never started listening
      evidence  /tmp/rewake-case-270015062
      stderr    worker.stderr: fixture: a delivery carried more than one line
summary   .rewake-checks/<time>/summary.json
```

The first line counts scenarios and cases by outcome. `against` says what the run was checked
against. An `unsupported` line names the case, the column and the capability it lacks. A
`FAIL` block names the case and column, each observation that did not pass with its detail, one
evidence directory, and a `stderr` line for each session that printed to its standard error, at
most four: the file and the last line in it. `run FAIL` is a failure of the run itself, such as
a binary that could not be built; `engine FAIL` means `go test` failed with no case to explain
it, and the failed tests are listed. `summary.json` lives under `.rewake-checks/`, which git
ignores.

A red case keeps its directory: `/tmp/rewake-case-*` with the private HOME, state directory and
shim records of that case. A green case removes its own. Read the evidence before deleting it.
Each session the case started wrote its standard error to `<name>.stderr` in that HOME — the
wrapper's refusals, the fixture's complaints — and a red case keeps those files cut to 64 KiB,
their first and last halves, so a session that looped on an error does not fill the disk.

**The outcomes**, in the order the classifier ranks a case. `fail`: an observation was made and
contradicted the claim, or cleanup failed, or the deadline expired — it wins over everything
below. `incomplete`: the case ran and a declared observation was never made, which includes one
recorded as `skip` or `not-run`; never green, because "we never looked" is not "we looked and
it was right". `unsupported`: the column lacks a capability an observation needs; acceptable on
a search column, red on the gate, where it would mean the regression check had quietly stopped
checking. `pass`: every declared observation was made and held. `skip` and `not-run` appear on
observations, never as a case's outcome: a declared observation nobody made is listed as
`not-run`, and a case that has not finished has no verdict yet. Exit 0 alone is never a
case's acceptance, and a case's result is final only after the processes it owns have finished
and its cleanup is checked.

## Extending the suite

**A scenario** declares its observations before it starts (`Start` with a `Spec`), so an
observation the code forgot to make is `incomplete`, not silently absent. It runs through
`runInColumns`, and anything a column cannot show goes through `col.unsupported` with a
capability registered by `capability(...)` — per observation, not per case. Waits are anchored
on a condition the case can observe (`Await`, `waitFor`), never on a sleep: a sleep makes a slow
machine look like a defect and a lost wakeup look like slowness. The suite has no product
mutants or controls; a new scenario does not bring them back.

**A fixture** is stricter than the harness it plays, never looser: a fixture that accepts what
the harness refuses lets a scenario pass on a product the harness would reject.

**A harness column** is a `column` value with its capabilities and a fixture; the scenarios do
not change. The fixture harness's program is `test/workflow/fixtureshim_test.go` and
`fixtureshim_turn_test.go`; its adapter, with its own tests of the readiness exchange, each
switch, the versions, L1, L2 and E3, is `internal/adapter/fixture`.

## Traps this suite has already paid for

- **A recipient started but not yet live.** A task goes only to a run whose turn boundary is
  live, and the record says so once the readiness exchange has run. Cases that sent from main
  to workers they had just started passed while main's first request came after the workers'
  exchanges, and failed once a worker's exchange finished later under load. A scenario waits
  with `sessionsUp` for each recipient's listening file, which its harness writes after the
  exchange, as a session's own sends do through `readinessSwitch`.
- **A shim rewritten while it runs.** Every session a case starts installed the shim scripts
  again, writing them in place, while an earlier session of the same case was executing them:
  an exec of a file open for writing fails with ETXTBSY. The scripts are written beside and
  renamed over.
- **Timers set at the edge of an idle machine.** With six cases at a time, sends started over a
  second late, and a window shortened past what it had to outlast announced a letter alone.
  Every shortened wait carries, beside its value, what it has to outlast.
- **A fixture session that outlives nothing.** A fixture session stops itself at an ordinary
  ceiling unless it serves a request directory, which keeps it up for 85 seconds. A worker the scenario asks
  nothing of, launched late in a long case, was gone when its last step came — and a step that
  expected a refusal took "no such session" for it, since both exit 2. Such a worker is
  launched with `staysUp`, and a finding that expects a refusal names its text, not only its
  exit code.
- **A zombie counted as a live session.** A fixture that exits while a child it started is
  finishing leaves that child an orphan, adopted by the suite as subreaper and dead but
  uncollected; the check for a live process group counted it, and the case waited out its
  deadline for a session that had ended. A zombie in the group is collected and not counted.
- **Inherited session variables.** A test run from inside a session read the live state as its
  own — hence `env -u` everywhere
  ([traps.md](traps.md#a-test-that-inherited-the-sessions-variables-declared-its-own-session-foreign)).
- **A fixture softer than the harness.** Findings were missed on a product with every test
  green, because the fixture accepted what the real harness refuses; a fixture checks a call as
  the harness does.
- **The gate losing an observation.** One line removed from the gate's table turned an
  observation `unsupported` and the run stayed green; unsupported is red there.
