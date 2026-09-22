# Check runner — proposal

September 21, 2026. This answers the research deliverable of
[check-runner.md](check-runner.md) and adds the requirement that arrived with it:
the suite must survive a third harness. Nothing here is implemented. The owner's
two goals, in their words on September 21, 2026, are that nothing in testing is
automated and that reading logs and tests burns agent context, and that whatever we
pick has to stay convenient as harnesses are added.

**What the suite is mainly for.** Not finding new behaviour — protecting the Codex path
from regressions when shared code changes. The owner's account of the two adapters,
September 21, 2026: Codex "was developed further and checked more", Claude Code "was
made first, then Codex" and "came easier, Codex took titanic effort", so "something may
not work right in Claude, but Codex must not be broken". Most of the mailbox is shared
(`internal/inbox`, `internal/cli`, `internal/wrap`), so a change made for one harness
lands under both. Hence the asymmetry: the Codex column is a regression gate, the
Claude Code column is where defects are expected. An **impl?** cell there means
*unverified*, never *works* — odd behaviour gets investigated, not attributed to the
harness. After September 21, 2026 one such cell remains: HF-19.

**The proposal in one paragraph.** Keep `go test` as the engine and add no framework.
A workflow scenario becomes an ordinary Go test in a new `test/workflow` package,
skipping itself unless switched on, so it never runs in the five required checks but
is still compiled and analysed by all of them. Each scenario is
written **once** against a harness-shaped fixture interface and run against every
registered harness through a table, the way a conformance suite runs one contract
against many implementations. A scenario's observations are declared as a struct, not
asserted ad hoc, so a missing observation is a failure rather than an omission. A
thin script over `go test -json` prints the short summary and writes `summary.json`
plus per-case artifact directories; successful runs print no logs. First
implementation is three scenarios, not the matrix; they and the harness matrix are in
[check-runner-scenarios.md](check-runner-scenarios.md), split out to keep both documents
under the project's line limit.

## What already exists

| Layer | What is there | What it proves | Gap |
| --- | --- | --- | --- |
| Five required checks | gofumpt, go vet, staticcheck, golangci-lint, `go test -race -shuffle=on` (AGENTS.md) | The tree builds, is formatted, and every unit test passes | Nothing about a real process, a real harness or a real message path |
| Unit and domain tests | 521 tests, 19 070 lines, across 14 packages; heaviest in `internal/harness/codex/gateway`, `internal/cli`, `internal/inbox` | Deterministic domain invariants, refusal reasons, receipt accounting, queue bounds | In-process doubles only |
| In-process integration | `internal/cli/gateway_integration_test.go` wires a real reservation to a real mailbox inside one process | Correlation across components | No process boundary, no harness binary |
| Cross-process helper | `internal/state/lock_test.go` re-execs the test binary through `TestMain` to prove a file lock holds between processes | That the pattern we need already works here, with no dependency | Used for exactly one invariant |
| Process fixtures | `internal/wrap/process_fixture_test.go`, `ownership_test.go`, `scripts/shim_test.go` | Wrapper lifecycle against spawned children | Children are shims, not harnesses |
| Hand-written acceptance | [inbox-acceptance.md](inbox-acceptance.md), [native-mailbox-acceptance.md](native-mailbox-acceptance.md), [claude-parity-2026-09-21.md](claude-parity-2026-09-21.md) | The live behaviour the marks in [harness-features.md](harness-features.md) rest on | Manual, unrepeatable, one run each |

Reusable from earlier packages: the re-exec helper shape, the sequenced local endpoint
that asserts it was called, and the isolation recipe of a private HOME/config/state per
case. Not reusable: anything in the ignored handoff tree, any fixture needing an
installed owner binary, any archive whose layout assumes its own directory. Those stay
one-off evidence; what is needed is re-created inside the repository.

**One inventory finding, since fixed.** A working copy can hold Git-ignored research
archives beside the source, and those archives contain standalone Go fragments. Being
ignored by Git does not remove them from the Go module, so the five checks in their
literal `./...` form were red on archived code (`undefined: stateDir` and similar)
rather than on anything in the project. A nested `go.mod` inside the ignored tree,
added September 21, 2026, drops it out of the module and makes `go vet`, `staticcheck`,
`golangci-lint` and the tests green in their documented form.

`gofumpt` is the exception a runner must handle: it walks the filesystem rather than
the module, so a nested `go.mod` does not hide those files from it. The form that
matches the other four is `gofumpt -l $(go list -f '{{.Dir}}' ./...)`. Any runner
executing the five required checks has to treat the module — not the directory tree —
as the definition of the project. A fresh checkout has no such archives, which is
precisely why the runner must not depend on them either.

## What to run it with

Four options, each costed against the same four questions.

### A. `go test` with a skip switch and shared helpers — recommended

Scenarios live in `test/workflow`. Each one begins by checking a switch — an
environment variable or a flag — and calls `t.Skip` with a reason when it is off. A
scenario is a Go function taking a fixture; the harness comes from a table.

- *New harness:* one row in the harness table plus whatever that harness's fixture
  needs to launch and observe. Scenarios are untouched.
- *New scenario:* one function, one entry in the scenario list. It runs against every
  harness immediately.
- *Code before the first scenario runs:* more than `go test` alone suggests. It gives
  process isolation, `-race`, `-shuffle`, subtests and machine-readable output; the
  fixture still has to build the binary, place the shims, establish the private
  environment, express per-case deadlines and define the observation struct.
- *Parallelism:* `go test` offers it, and the suite does not take it. A case is
  identified by descent from the test process, so two cases at once would each see
  the other's processes as unaccounted for — signalling a neighbour's work and
  recording strays against a case that never started them. Using it needs a real
  boundary per case, a cgroup or a pid namespace, which is not worth it for a suite
  whose cases are measured in seconds.
- *Harness version bump:* fixture-local; a changed native surface changes one
  implementation, not the scenarios.

**The fixture drives a real binary, not internals** (orchestrator decision,
September 21, 2026). A scenario is a black-box test over an assembled `rewake`: the
fixture builds it into a temporary directory with an ordinary `go build` from
`TestMain` — standard library, no dependency — and puts a shim on `PATH` in place of
the harness. Three things follow. The fixture tier stops depending on what happens to
be installed on the machine. The Stop hook then invokes the binary that was built,
rather than the test binary that `os.Executable()` would otherwise return
(`internal/harness/hooks.go`). And the seam stays outside every adapter.

Cost, stated plainly: building the binary, writing and maintaining the shims, and the
isolation below. The summary and `summary.json` are ours too — `go test -json` gives
events, not the evidence contract of [check-runner.md](check-runner.md). Its `-timeout`
is per package and kills the process, so per-case deadlines with classification after
subprocesses end are separate work. The package is compiled on every `go test ./...`,
a few milliseconds for a standard-library package whose tests skip — and that
compilation is what keeps the code from rotting. One condition follows from the skip
form: nothing needing an installed harness may run at package initialisation.

The shape to copy is the re-exec helper in `internal/state/lock_test.go`, which already
runs a second process to prove a cross-process invariant. Not
`internal/cli/gateway_integration_test.go`: it lives inside package `cli` and assembles
a reservation through unexported helpers, which an external `test/workflow` package
cannot reproduce.

### B. A separate runner binary in `cmd/`

Adding a harness or a scenario costs the same as A once it exists, and a harness bump
is equally fixture-local. What differs is the start: process supervision, parallel
scheduling and case naming have to be written, and all three are already in `go test`.
Failure capture and result classification are not a difference — A writes those too.
Rejected — it buys control
over output, which a Go summarizer over `go test -json` also buys, at the price of
reimplementing a test runner. Revisit only for orchestration `go test` cannot express,
such as a scenario spanning several machines.

### C. Scenarios as data, one interpreter

A table or file per scenario (txtar-style), executed by a shared engine.

A new scenario is then a file and no Go code, which is attractive once scenarios
repeat. The start is the most expensive of the four: the interpreter needs a vocabulary
for everything a scenario can observe, and our observations are not stdout matching —
they are message IDs, epochs, receipt state, admission ACKs and report correlation.
Encoding that in a mini-language is a second product. Rejected for now, kept as the
destination; revisit past roughly a dozen scenarios, when the Go functions start
looking like each other.

### D. An external scenario engine (`rogpeppe/go-internal/testscript`)

The mature form of C: txtar scripts, a command vocabulary, used to test the `go`
command itself. Rejected on two counts — the module's first dependency against the
standard-library-only rule, and a partial fit, since testscript is strongest at "run a
command, match its output" while our hardest invariants concern what a *second process*
observed. Reasons and the condition for revisiting are in **What was rejected**.

### The cost none of the options may hide

If an option needs the Codex adapter changed to become testable, that is not a small
price: that path cost milestone 10 and several review rounds, and its gateway is the
most intricate code in the tree. Option A needs nothing from it, because the fixture
stays outside every adapter — it drives an assembled binary through the CLI, and the
only interfaces it depends on are the ones both harnesses share, `harness.Harness` and
`LaunchPlan` (`internal/harness/plan.go`). `harness.Backend` is not one of them:
`internal/harness/codex/codex.go` is the only place it is constructed, while the
Claude Code adapter returns a plan without it and delivers through `Harness.Deliver`
into its socket. Option B is no cheaper here. Options C and D would likely want new
observation hooks inside the adapter for a script to assert on; that hidden cost alone
keeps them later rather than first. Any future scenario that cannot be written without
an adapter change is raised as a question before the change, not written as a
convenience.

## Harness as a parameter, not a copy

The pattern to borrow is the conformance suite: write the contract's tests once against
a factory and run them against every implementation — what `connectrpc/conformance` does
across Connect/gRPC, and what Go projects do with `contract`-style packages taking a
`*testing.T` and a constructor. Here the implementations are harnesses, already
enumerated in `internal/harness/catalog/catalog.go`, exactly as
[harness-features.md](harness-features.md) gives each one a column.

Concretely, a scenario never names Codex or Claude Code. It asks its fixture to launch
an isolated session of role R, deliver message M, report whether the session consumed
it and what outcome it reported, then tear down and report cleanliness. Each harness
supplies that fixture plus a capability set; a scenario the harness cannot support
yields `unsupported` with a reason.

The capability set is the seam that keeps a third harness cheap: the scenario asks
"can you observe a mid-turn arrival?", not "are you Codex?". A capability that no
harness supports is a scenario that never silently passes.

## What the agent sees

Success is a handful of lines. The [check-runner.md](check-runner.md) budget — at most
twelve summary lines for five checks plus three workflows, failure excerpts at most
twenty lines and 8 KiB combined — is realistic for the shape below and should stay.

```
checks    5/5 ok           12.4s
workflow  3 scenarios × 2 harnesses: 5 pass, 1 unsupported   64.1s
          unsupported  mid-turn/claude-code  no interruption source (HF-06)
summary   .rewake-checks/2026-09-21T00-41/summary.json
```

A failure adds the case name, the failing observation by name, and a path:

```
FAIL  task-report/codex   observation "report correlated to read task" not satisfied
      expected report for message 1789…97, sender saw 1789…12
      evidence  .rewake-checks/2026-09-21T00-41/task-report-codex/
```

The summary is produced by a small Go program reading `go test -json`, not by a shell
pipeline. A `jq`-based script would make the runner depend on whatever happens to be
installed; Go is available by definition, since without it there is nothing to run.
Its location — `cmd/` or `tools/` — is an implementation choice, not a design one.

Two rules make the budget hold rather than state it: the summary is generated from the
result records, so no added print can grow it; and artifacts go per case into a
directory named by the case, so the agent's next read is one path, never a listing.

`summary.json` carries the evidence contract fields already specified in
[check-runner.md](check-runner.md) — result category, harness, tier, per-observation
pass/fail, durations, source and build identity, and who performed each read. One field
to add: `capability`, naming why a cell is `unsupported` and linking its HF row.

## What can and cannot be automated

The boundary is the same tier ladder as [check-runner.md](check-runner.md), restated as
a decision about who pays:

| Tier | Automatable? | Cost |
| --- | --- | --- |
| Pure Go domain invariants | Yes, already | Free, in the five checks |
| Protocol fixtures — local doubles for the native surface | Yes | Free; the dominant tier for the new suite |
| Real harness binary against a local scripted endpoint | Yes | Free of account, but needs the harness installed and version-pinned; isolation must hold or the case stops |
| Real-model semantic — the model itself chooses to read the inbox | No | Paid per run; an explicit opt-in target, never in the default suite (three conditions below) |
| Owner TUI — what a person sees on screen | No, permanently | Manual acceptance; no typing into anyone's terminal |

**Isolation differs by tier** (orchestrator decision, September 21, 2026). The
requirement in [check-runner.md](check-runner.md) — private HOME/config/state, no owner
credentials, network/mount/PID boundaries, a local-only endpoint, verified binaries —
was written for every tier at once, and `go test` supplies none of the network, mount
or PID boundaries. Narrowing it is allowed, but only stated and with a reason. For the
free tiers, private HOME/config/state, a shim on `PATH` and a local endpoint suffice:
no real credentials are nearby, no network is used, and a substituted harness does not
reach outside. The network/mount/PID boundaries remain mandatory for the paid tier,
where real credentials do live alongside.

**The pinned version.** `internal/harness/codex/server.go` runs `codex` from `PATH` and
compares the output against one constant, warning when it differs. That constant was
0.154.0 while 0.155.1 was installed, so the note printed on every launch; it moved to
0.155.1 on September 21, 2026 (below). One consequence stands regardless: the fixture
tier is reachable only through a `PATH` shim, which is why the fixture places one.

Three conditions govern the paid tier, decided September 21, 2026:

1. **No account access by default, as a property rather than a promise.** Isolation is
   established before any harness launches, per [check-runner.md](check-runner.md).
   A failed isolation stops the case; it never falls back to the real account.
2. **A person or the orchestrator starts a paid run explicitly.** The runner never
   decides that spending quota is warranted, including when retrying a failed case.
3. **A paid result does not close a scenario.** It stays evidence of its own tier and
   lives in a separate region of the summary, so that a free green can never be read
   as proof that a model actually read the mailbox and acted.

The third tier is where most value sits and where the discipline has to be hardest,
because it is the tier that has repeatedly produced false green. The record from earlier
rounds, kept here as requirements rather than anecdotes:

- **A correct refusal for the wrong reason.** Replay and oversize guards passed because
  a reservation had expired, not because the guard fired. A scenario sets up a live
  precondition, asserts the specific refusal, and its negative control removes that one
  guard.
- **Passing before the process finished.** A failed terminal, a cleanup failure under
  exit 0, an endpoint error after an early pass. Results are classified only after owned
  processes end, handlers join and cleanup is verified.
- **The controller read the mailbox and the report said the agent did.** Every read
  records who performed it; a controller-driven read is never reported at the real-model
  tier.
- **A synthetic client reported as a terminal.** Fixture evidence stays fixture
  evidence; `not-run` never becomes acceptance.

Each of these is a property of the *result record*, which is why the observation
struct matters more than the assertions: an observation that was never made is
`incomplete`, and `incomplete` is not green.

The scenarios chosen for that first implementation, their invariants, observations and
negative controls, and the scenario × harness matrix are in
[check-runner-scenarios.md](check-runner-scenarios.md).

## Patterns read outside the project

Read, not verified here; each is cited for the idea taken, and the fit noted.

- **testscript / txtar** ([pkg.go.dev](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript),
  [encore.dev](https://encore.dev/blog/testscript-hidden-testing-gem)) — scripts as archives
  run by a small engine. It is an extracted sibling of the mechanism `cmd/go` uses on
  itself through its own internal copy; the `go` command does not depend on this package.
  *Takeaway:* the destination shape for option C. *Misfit:* stdout matching, plus a
  dependency.
- **Conformance suites** ([connectrpc/conformance](https://github.com/connectrpc/conformance),
  [interface-contract pattern](https://pnguyen.au/posts/testing-interface-contracts/)) — one
  suite, a factory per implementation. *Takeaway:* the core of this proposal, and a close
  fit — our harnesses genuinely implement one contract.
- **Sequenced mock endpoints with call expectations** (a reference Codex tree at
  `e29eceb75` builds its app-server integration tests as one aggregating binary, with
  helpers and scenarios in sibling directories and a mock responder asserting the exact
  number of calls it received). *Takeaway:* the one-binary/helpers/scenarios layout, and
  that a double which *expects* to be called turns "the process never got there" from a
  silent pass into a failure.
- **Deterministic synchronization instead of sleeps** — replace fixed sleeps with
  readiness events, acknowledgements, or bounded condition-polling that states its
  deadline and reports the last observed state. *Takeaway:* our `waitIntegration` polls
  every 5 ms for 3 s with no diagnostic; scenarios should wait on observable state and
  say what they last saw. *Note:* `testing/synctest` is in the standard library at Go
  1.25 and gives a fake clock, but only within one process — no help across the process
  boundaries these scenarios need.
- **Compact run summaries from machine output** ([gotestsum](https://github.com/gotestyourself/gotestsum)
  reads `go test -json` and formats both a human summary and a JSON artifact).
  *Takeaway:* not the tool but its split — the event stream is the source of truth, the
  console form is derived, the artifact written once.
- **TUI testing through a pseudo-terminal** ([teatest](https://charm.land/blog/teatest/),
  VT-emulating harnesses asserting on a screen grid). *Takeaway:* confirmation of a
  boundary already set — that is how the owner TUI tier would be automated, and the
  project forbids typing into a person's screen, so it stays manual.

## What was rejected, and why

Options B, C and D are costed above; the rest are the alternatives that did not reach
that comparison.

- **An external scenario engine (testscript)** — the fit is partial: it is strongest at
  matching command output, while our invariants concern a second process's observed
  state. That is the whole reason now. The second reason recorded here on September 21,
  2026 — that this project took no dependencies — was lifted by the owner the same day;
  a dependency is now an ordinary decision, judged on transitive dependencies and
  maintenance. Revisit once there are more than roughly a dozen scenarios with visible
  repetition between them. That is a concrete case, not a workaround.
- **Extending the in-process integration test instead of a new location** — it shares
  the package under test, so it cannot exercise a real binary, and growing it blurs the
  unit and workflow tiers inside one `go test ./...`.
- **Running workflows inside the five required checks** — slows the commit gate and
  makes it depend on installed harnesses. The gate stays as it is.
- **Reviving packaged fixtures from the ignored tree** — a fresh checkout must run the
  suite, and those fixtures assume their own layout and an installed owner binary.
- **A full matrix as the first deliverable** — it follows a demonstrated gap, not the
  availability of fixtures.

## Decisions and what is still open

**The owner's acceptance criterion, September 21, 2026**, quoted as given:

> да, планируйте вместе как все разложить, решиние должно быть просто поддерживать

The implementation is judged on how easy it is to maintain, ahead of how much it
covers. That is a criterion, not a preference: a design that covers more
rows but is harder to keep working loses to one that covers fewer and stays simple.
Every choice below was made against it, and so should the ones that follow — layout
included.

Settled on September 21, 2026, between the orchestrator and this session: the standard
library holds and testscript is not taken (above); `go test` is the engine; the summary
parser is a Go program, not a shell pipeline; the paid tier is an opt-in target under
the three conditions above; and the nested `go.mod` described in the inventory was
added, fixing the literal form of four of the five checks.

Two more were settled the same day:

- **The suite lives in `test/workflow`, outside `internal`.** A scenario that can reach
  into package internals will eventually assert on implementation rather than
  behaviour, and that would quietly devalue the whole suite. Reusing unexported helpers
  is not worth it.
- **No build tag.** The question of whether a sixth command should compile tagged code
  disappears if there is no tag: scenarios are ordinary tests that skip themselves
  unless switched on. The code is compiled and analysed by all five checks, the gate
  stays at five commands, and a separate way to run the suite is preserved. The cost —
  compiling the package on every gate run — is a few milliseconds and is itself the
  rot check.

**The transport pin moved to 0.155.1** on September 21, 2026, on the owner's decision
("пин можно сделать", the pin may be moved) and on two probes of that day: the
ordinary path, and steer into an active turn. It now lives in one constant, `lastObservedServerVersion` in
`internal/harness/codex/server.go`, so the real-harness tier no longer runs the
mismatch branch. Not observed on that version, and not to be claimed by the suite:
conversation selection after `/new`, stale-target refusal, `stopped` from an
interruption, and a group arriving during an active turn.

Open: **the standard-library rule**, and only if the scenario count later makes
testscript worth raising with the owner.
