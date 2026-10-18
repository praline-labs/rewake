# Testing rewake

The one place that says how this project is tested: what each tier runs, what it proves
and what it does not, how to run it, how to read what comes out, how to check a new
harness version before updating, and how to extend the suite. It describes the code as
it is. For depth it links to the documents where each decision was argued:
[check-runner.md](check-runner.md) for the evidence contract,
[check-runner-proposal.md](archive/check-runner-proposal.md) for why the suite has its shape,
[check-runner-scenarios.md](archive/check-runner-scenarios.md) for each scenario's invariant,
and the roadmap records named below for how each piece was built.

## The tiers

| Tier | Runs | Proves | Does not prove | Costs |
| --- | --- | --- | --- | --- |
| The step's checks | `gofumpt`, golangci-lint, one `go test -race -shuffle=on ./...` | unit invariants: parsing, publishing races, liveness, inbox order and expiry, the context endpoint's framing, the import rule; the suite's own classifier and summarizer | anything end to end: every workflow scenario skips itself | about ten minutes at `-p 4` on an idle machine — 9m50s on October 17, 2026 — nearly all of it the `-race` test run; no network, no harness, no container. The full set before a publication adds vet and staticcheck over the four build variants and two more `-race` runs with the tags, about half an hour. Each test run carries `-timeout 20m`: `core/mail`'s `TestEveryReadOfThePlanStopsBeforeTheFirstEffect` takes about 290 s alone and about 450 s under `-race` beside other work, and `go test`'s default ten minutes killed the untagged run at `-p 4` on October 16, 2026 |
| Workflow suite (**F**) | a built rewake end to end against a fixture harness, in one column since S9 | that the shared service code delivers, groups, steers and reports as each scenario claims | that the real harness parses, renders or behaves as its fixture does | about three minutes — 3m06s and 3m10s on September 25, 2026, 102 cases six at a time, against 19m22s one after another the same evening; 2m23s eight at a time. The longest cases then were `claude-steered`'s controls, about 30 s each. It fits in `go test`'s default ten minutes, which the documented commands name; no network |

Two tiers are planned and not built. **P**, a real harness of a named version against a
local responder instead of a model provider, would catch a change of behaviour that a
schema cannot see; the fetch and the container it would need left with
`tools/harnesscache` in S8 and are kept in
[archive/1.x/codex](../archive/1.x/codex/README.md). **M**, a real model through a real
harness, paid, would show that a model actually reads its mail and acts on it; it runs on
the cheapest model only, as [check-runner.md](check-runner.md) requires. The full ladder,
including the owner's own eye on a terminal, is in
[check-runner.md](check-runner.md#evidence-tiers-and-matrix).

### The fault build inside the test run

A binary built with the `rewakefault` tag reads `REWAKE_FAULT`
(`internal/infra/state/fault_build.go`) and logs, ends, fails or holds its process at a durable
step, or leaves behind a process that keeps its output open past a kill. The neutral rig
that took its fault cases from the logged steps of clean runs, `test/toolrig`, left with
the mail tool on October 18, 2026. A release build never reads the variable.

The lease rig (`test/leaserig`) builds rewake twice the same way, the second with a build
time passed in, so the two builds share a version and differ in content, and races the
room's build lease between them and a byte-identical copy: each process held at a named
step of the lease ([stage3-state.md](v2/stage3-state.md#one-build-per-room)), released or
killed by the test. It runs in every test run, untagged, since a launch
it starts never gets past its lease.

The decision rig (`test/decisionrig`) builds rewake with the tag and the fixture harness,
and launches sessions whose harness is the rig's own test binary as the fixture's program
([stage3-decision.md](v2/stage3-decision.md#authority)). The program's two halves start
askers on command — below the wrapper, in a group or a user namespace of their own, with
the session's variables changed or cleared, or through a double fork that leaves the tree
— each asking for a decision through the client, straight at the address, or classifying
itself as the person. A wrapper is held at the named steps of a decision's way and killed
or ended there, an asker at the client's step after it checked the wrapper. It runs only
with the fixture's tag, since a launch needs the fixture harness.

The tag also admits one narrow kind of test code into a production package. When a fault
schedule must cross a package boundary and neither an exported operation, a file
fixture nor `state.Fault` can build it, the package that owns the step may carry exported
test operations in a file built only under `rewakefault`, and the tests in the other
package that use them carry the same tag. They run in the `rewakefault,rewakefixture`
race run; an untagged build has no such operation. The only such file is
`internal/core/mail/faultops.go`, decided by S15 for `core/mail` ([stage3-moves.md](archive/v2/stage3-moves.md#s15-the-delivery-server-to-hostdelivery-codex)):
a waiting copy's removal that fails, and the listing of a mailbox's stops observed.

### Three columns, three meanings

Since S8, October 8, 2026, there were two: the Codex column left with its adapter. Since
S9, October 11, 2026, there is one: the column of a fixture that played a Claude Code
session left with the hooks and the plugin it played
([testing-cases.md](testing-cases.md#the-retired-claude-code-column)), and stage 4 brings
a Claude Code column back. Shared scenarios run across the columns; adapter-specific
scenarios run on the columns they exercise. The column left is the
fixture harness — a harness of its own, built
only under the `rewakefixture` tag, which the suite's rewake is built with
([stage3-fixture.md](v2/stage3-fixture.md)). **The fixture column is the regression
gate** since S6 ([stage3-fixture.md](v2/stage3-fixture.md#the-gate-across-the-steps)):
a red there blocks, and an absent capability there is red too, because a gate that
stopped checking must not look green. It proves the core through the adapter contract,
and its program can withhold each capability. Until S9 **Claude Code was a search
column**: a red there was a finding to investigate, and an absent capability was
reported as `unsupported`, by name; a search column returned with stage 4 takes that
meaning again. The gate is a field of the
column (`gate` in `test/workflow/column_test.go`), set on exactly one. The summary
names the column of every result and promotes none; ending the asymmetry is the owner's
decision, recorded in [harness-features.md](archive/harness-features.md).

## Running it

The commands of a commit and of a step live in the [Checks section of
AGENTS.md](../AGENTS.md#checks), which every agent session loads; the full set before a
publication is [below](#before-a-publication-to-npm). Three levels, by the owner's
decision of October 17, 2026: a commit while a step goes on builds, vets and passes the
tests of the packages it touched; the last commit of a step, before it lands on `v2`, passes `gofumpt`, golangci-lint, one
`go test -race` over the whole tree and the workflow scenarios the change touches; the full
set — every build variant, vet and staticcheck over each, the tagged `-race` runs and the whole
workflow suite — runs only before a publication to npm. A step is
done when it works live and breaks no existing test.
What follows is what their variables and flags do, and when each run is the right one.

**Every `go test`** clears `REWAKE_SESSION`, `REWAKE_EPOCH`, `REWAKE_DIR` and
`REWAKE_ROOM`, the workflow suite's included, because a test that inherits them reads the
live session's state as its own. The unit tests start no harness, no container and no
network request, whatever else the environment says: every real-harness step is behind the
suite switch.

**The workflow suite** is switched on by `REWAKE_WORKFLOW=1`; without it every scenario
skips itself. Run it through `tools/checksummary` to read a result, which prints a few
lines and writes the rest to a file; run it with `go test -v` to watch it, and keep the
`-v`: `go test` prints nothing a passing package printed, so without it the line naming
how many scenarios ran is invisible. `-count=1` keeps a cached pass from standing in for
a run. On the last commit of a step, run the scenarios the change touches, selected with
`-run`; the whole suite runs after a change to the fixture and before a publication.

**Cases run side by side**, six at a time unless `-parallel` says otherwise, and the
suite's binary runs seven of the product's waits shorter than a release.
[testing-pool.md](testing-pool.md) says how to choose the width, how a case's cleanup
leaves its neighbours alone, and which values the suite does not run at their real
length.

`REWAKE_WORKFLOW_SELFCHECK` is not for people: the suite sets it on a child of itself
to prove that a scenario missing an observation turns the run red.

### Before a publication to npm

The full set runs every build variant, since production code also builds under two tags:
`rewakefault` adds the fault seam of the state directory, `rewakefixture` the fixture
harness the workflow suite runs ([stage3-fixture.md](v2/stage3-fixture.md)):

```bash
go vet ./... && go vet -tags rewakefault ./... && go vet -tags rewakefixture ./... &&
  go vet -tags rewakefault,rewakefixture ./...
staticcheck ./... && staticcheck -tags rewakefault ./... && staticcheck -tags rewakefixture ./... &&
  staticcheck -tags rewakefault,rewakefixture ./...
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  go test -race -shuffle=on -timeout 20m -tags rewakefixture ./...
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  go test -race -shuffle=on -timeout 20m -tags rewakefault,rewakefixture ./...
```

with the whole workflow suite. The tools install into `~/go/bin`, on `PATH`:

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest mvdan.cc/gofumpt@latest \
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

### The stand-in API

Until S8, October 8, 2026, `tools/standin` answered for the model API a Claude Code
harness talks to, replying with a call of the 1.x mail tool's MCP name, so a live check of
that tool could run in a scratch configuration with no login. It served the live checks of
stage 3 ([mail-bridge-live.md](archive/mail-bridge-live.md)). The fixes of S8's review moved it to
the 1.x archive with the MCP contract it answered
([archive/1.x/codex](../archive/1.x/codex/README.md)); no product launch offers that tool.
The stand-in stage 4 was to rebuild for the mod's live checks, answering with a call of
the mod's tool, has nothing to answer since the mail tool left on October 18, 2026.

## Checking a new harness version before updating

Until S8, October 8, 2026, the suite checked a Codex version before an update: it fetched
the named version into a harness cache, generated its protocol schema in a container and
held the fixture's messages to it. That check, `REWAKE_CODEX_VERSION` and
`tools/harnesscache` left with the Codex adapter and are kept in
[archive/1.x/codex](../archive/1.x/codex/README.md). Claude Code's launch reads the
installed version before the claim and refuses one below its minimum; a new version's
behaviour is checked live, as [research.md](research.md) records. Stage 5 brings a
version check back with the Codex adapter.

## Reading a result

The summarizer prints, and its exit code says, 0 green, 1 red, 2 a wrong call, 3 a
stream it could not read (`go run` reports every non-zero exit as 1 and prints the real
one; build the binary to keep them apart):

```
workflow  24 scenarios, 36 cases: 33 pass, 3 unsupported   2m0.7s
against   scenarios against the suite's own harness programs in every column
          unsupported  mid-turn/fixture  observes-mid-turn-arrival
FAIL  batch-arrival/fixture   fail
      fail        "the recipient can receive mail before the letters leave"
        worker-fixture never started listening
      evidence  /tmp/rewake-case-270015062
      stderr    worker.stderr: fixture: a delivery carried more than one line
summary   .rewake-checks/<time>/summary.json
```

The first line counts scenarios and cases by outcome. `against` says what the run was
checked against. An `unsupported` line names the case, the column and the capability it
lacks. A `FAIL` block names the case and column, each observation that did not pass
with its detail, one evidence directory, and a `stderr` line for each session that
printed to its standard error, at most four: the file and the last line in it.
`run FAIL` is a failure of the run itself, such as a binary that could not be built; `engine FAIL` means `go test` failed with no case to explain it, and the
failed tests are listed. `summary.json` holds all of it,
every case and every observation, under `.rewake-checks/`, which git ignores.

A red case keeps its directory: `/tmp/rewake-case-*` with the private HOME, state
directory and shim records of that case. A green case removes its own. Read the
evidence before deleting it. Each session the case started wrote its standard error to
`<name>.stderr` in that HOME — the wrapper's refusals, the fixture's complaints — and a
red case keeps those files cut to 64 KiB, their first and last halves, so a session
that looped on an error does not fill the disk. They were added on September 27, 2026,
after a launch that exited 1 said why only in a rerun with the capture added by hand.

**The outcomes**, in the order the classifier ranks a case. `fail`: an observation was
made and contradicted the claim, or cleanup failed, or the deadline expired — it wins
over everything below. `incomplete`: the case ran and a declared observation was never
made, which includes one recorded as `skip` or `not-run`; never green, because "we never
looked" is not "we looked and it was right". `unsupported`: the column lacks a
capability an observation needs; acceptable on a search column, red on the gate,
where it would mean the regression check had quietly stopped checking. `pass`: every
declared observation was made and held. `skip` and `not-run` appear on observations,
never as a case's outcome: a declared observation nobody made is listed as `not-run`,
and a case that has not finished has no verdict yet.

## Extending the suite

**A scenario** declares its observations before it starts (`Start` with a `Spec`), so
an observation the code forgot to make is `incomplete`, not silently absent. It runs
through `runInColumns`, and anything a column cannot show goes through
`col.unsupported` with a capability registered by `capability(...)` — per observation,
not per case. Waits are anchored on a condition the case can observe (`Await`, `waitFor`),
never on a sleep: a sleep makes a slow machine look like a defect and a lost wakeup look
like slowness.

**Controls** — product mutants and switches of the fixture's world, each proving that a
scenario could fail — and the crosswise check that ran each control's observations in
every other control's world left the suite with the test diet of October 18, 2026; a new
scenario does not bring them back.

**A fixture** is stricter than the harness it plays, never looser: a fixture that
accepts what the harness refuses lets a scenario pass on a product the harness would
reject.

**A harness column** is a `column` value with its capabilities and a fixture; the
scenarios do not change. The fixture harness's program is `test/workflow/fixtureshim_test.go`
and `fixtureshim_turn_test.go`; its adapter, with its own tests of the readiness
exchange, each switch, the versions, L1, L2 and E3, is `internal/adapter/fixture`. What building the second one taught is in
[2026-09-22-fixture-claude-code.md](roadmap/2026-09-22-fixture-claude-code.md).

### The cases

What each case claims is in [testing-cases.md](testing-cases.md):
launching and reporting, the worktrees, actions on a sent message, conversations and the inbound gate, and the Claude Code column retired in
S9 with the telemetry budgets and the plugin's cases it ran.

## Traps this suite has already paid for

- **A recipient started but not yet live.** Since S11 a task goes only to a run whose
  turn boundary is live, and the record says so once the readiness exchange has run. Ten
  cases sent from main to workers they had just started, waiting for nothing or only
  for the record's service pid; main is started last, so its first request usually came
  after the workers' exchanges, and the full run after the step's second commit was
  green. The next run, eight cases at a time, had a worker's exchange finish after main's
  and the task refused. A scenario now waits with `sessionsUp` for each recipient's
  listening file, which its harness writes after the exchange, as a session's own sends
  do through `readinessSwitch` (October 11, 2026).
- **A shim rewritten while it runs.** Every session a case starts installed the shim
  scripts again, writing them in place, while an earlier session of the same case was
  executing them: an exec of a file open for writing fails with ETXTBSY. Seen once while
  eight copies of the suite ran at a time; the scripts are now written beside and renamed over
  (September 25, 2026).
- **Timers set at the edge of an idle machine.** Shortening the widened-window mutant's
  collection interval from ten seconds to five broke its control in both columns: the
  first two sends wait out `send`'s five-second wait for a status before the third letter
  leaves, so the widened window has to outlast that, not only `laterSendDelay`. And with
  six cases at a time, the sends writing batch-arrival's heads-ups started over a second
  late: at 1.5 s of quiet and a 2 s cap, one crosswise run in four had the third heads-up
  announced alone. The suite's window is 2 s and 3 s since, which cost about 25 s of a
  full run. Every shortened wait now carries, beside its value, what it has to outlast
  (September 25, 2026).
- **A fixture session that outlives nothing.** A fixture session stops itself after 25
  seconds unless it serves a request directory, which keeps it up for 85. A worker the
  scenario asks nothing of, launched late in a long case, was gone when its last step
  came — and a step that expected a refusal took "no such session" for it, since both
  exit 2. When the coalescing window made heads-ups wait, three steered and interrupted
  cases crossed the line. Such a worker is launched with `staysUp`, and a finding that
  expects a refusal names its text, not only its exit code (September 25, 2026).
- **A record read between two of its writes.** The Claude Code fixture records a
  delivery and then the overview it read for it; batch-arrival's wait for a delivery
  once landed in between under a full run and failed the case in half a second. A wait
  now waits on through that gap; a judgement after its anchor still fails on it
  (September 23, 2026).
- **A zombie counted as a live session.** A fixture that exits while a hook it started
  is finishing leaves that hook's shell an orphan, adopted by the suite as subreaper and
  dead but uncollected; the check for a live process group counted it, and the case
  waited out its deadline for a session that had ended — two of eight parallel runs of
  pending-report. A zombie in the group is now collected and not counted (September 23,
  2026).
- **Inherited session variables.** A test run from inside a session read the live state
  as its own — hence `env -u` everywhere
  ([traps.md](traps.md#a-test-that-inherited-the-sessions-variables-declared-its-own-session-foreign)).
- **A fixture softer than the harness.** One review round found two high findings on a
  product with every test green, because the fixture accepted what the real server
  refuses ([research-launch.md](research-launch.md)). It happened again with the
  Claude Code plugin: the fixture's `$.process.run` took an options object the harness
  refuses, and the module that passed every case was silent live; the fixture now
  checks the call as the binary does
  ([2026-09-23-claude-plugin.md](roadmap/2026-09-23-claude-plugin.md)).
- **The gate losing an observation.** One line removed from the gate's table turned an
  observation `unsupported` and the run stayed green; unsupported is now red there.
- **The five checks reaching outside.** The direct shape tests were not behind the suite
  switch, so every ordinary `go test ./...` ran the installed Codex with the owner's HOME,
  and with a version named it contacted the registry and started a container; they need
  the switch now, and the suite's self-check child no longer inherits a named version
  ([2026-09-23-harness-versions.md](roadmap/2026-09-23-harness-versions.md)).

How each scenario was built, and what it does not prove, is in its own record:
[task-report](roadmap/2026-09-21-scenario-task-report.md),
[batch-arrival](roadmap/2026-09-22-scenario-batch-arrival.md),
[mid-turn](roadmap/2026-09-22-scenario-mid-turn.md), and the summarizer in
[2026-09-22-suite-summarizer.md](roadmap/2026-09-22-suite-summarizer.md).
