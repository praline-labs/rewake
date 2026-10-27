---
description: Where each part of rewake lives in the tree, package by package; the adapter interface is in rules/adapter-api.md.
---

# Source layout

Where each part of rewake lives. [design.md](design.md) keeps what the system is and why,
and the interface every harness adapter implements is
[rules/adapter-api.md](rules/adapter-api.md); this file is what to open when looking for
where something is in the tree. `go list
./...` names every Go package below; a new one is added here in the same change.

```
cmd/rewake/                                   entry point, top-level parsing
internal/                                     the layout test of 2.0: the import rule, harness names outside the adapters, no registry
internal/cli/                                 command table, parsing, overview, help, failures, printing
internal/infra/state/                         directory: checks, paths, atomic writes
internal/infra/build/                         the build id (SHA-256 of the image) and the room's build lease
internal/infra/build/buildtest/               for tests: a room made and its build recorded the way a launch of the test binary does
internal/infra/buildtime/                     durations a build may shorten through -ldflags, for the suite
internal/core/registry/                       session record, name publishing, liveness and listing
internal/core/registry/registrytest/          for tests: the boot a session of this build carries
internal/infra/proc/                          /proc: identity, liveness, job-control state, lineage, namespaces, owner and initial environment
internal/infra/boottime/                      the boot clock, comparable across processes and never set back
internal/core/mail/                           message, status, sender-side write, turn ends; `Box` is one run's own mail (room, name, epoch) and `Locked` the operations that run under its mailbox lock
internal/core/mail/mailtest/                  for tests outside mail: the paths and bytes of its records, each checked against mail
internal/core/control/                        a main's control request to a run and its answer, as files
internal/core/ask/                            a worker's permission requests and questions, held in its wrapper while main answers them
internal/core/role/                           the role catalogue: flag, briefing line, reporting duty
internal/core/brief/                          text injected into an agent, independent of transport
internal/core/worktype/                       the kinds of work a worker is raised for: the built-in texts and the person's and project's own
internal/core/session/                        optional, epoch-scoped observations of a harness (telemetry) and the activity verdict on them
internal/core/space/                          the record of each space a main opens: its states, its steps, the kept checkouts' reminder
internal/adapter/                             the adapter API: the Harness interface, the catalogue value, launch plans, version gates, flags, defaults, settings
internal/adapter/catalog/                     the one constructor of the catalogue, which cmd/rewake builds and hands to the CLI
internal/adapter/claude/                      launch arguments, environment, socket delivery, the mod embedded from its mod/ directory and written per run
internal/adapter/claude/modhost/              the wrapper's end of the mod's protocol: the HTTP server on the run's mod socket, each request checked by its peer, hello, a turn's start and end, mod/ended; the types tools/protogen writes from internal/adapter/claude/mod/protocol.schema.json
internal/adapter/pi/                          launch arguments, embedded TypeScript extension and management, peer-checked socket for notices, settled ends, telemetry and control
internal/adapter/fixture/                     the test harness behind rewaketest: a scripted program over a socket, every contract interface
internal/host/                                wrapper: launch, signals, lifecycle
internal/host/launch/                         a launch's host side: the run's environment, the version read before the claim, the bounded probe, the worker aliases main opens, a worker raised from one (`Set.Raise`: its worktree and type resolved, every conflicting request refused) and the worktree flag read for every launch
internal/host/herdr/                          main reporting itself to herdr, the terminal manager it runs in: its state from the telemetry, its resume command, the release
internal/host/delivery/                       the delivery server: the serving loop, notices and their batches, the coalescing window, outcomes and the sweep's lock
internal/host/terminal/                       a program's pseudo-terminal by the standard library, whether a file is a terminal, the screen the program draws with its input modes, keyboard protocols and title, and the raw stream with its preamble for a late client, the one package the emulator library enters
internal/host/spacehost/                      the space host: the worker's pseudo-terminal and screen, the space socket's raw stream for the shell; and opening a space for main (`Open`): the record, the host started, the wait for the worker's registration, the rollback
internal/host/worktree/                       checkouts rewake makes for a launch: git worktree add, records, land, removal and what keeps one, a worker's release at its close
scripts/                                      packaging scripts and the tests of the npm shim
test/workflow/                                the workflow suite: end-to-end scenarios against fixtures
test/workflow/record/                         the record format a suite run prints and the summarizer reads
test/launch/                                  a space's worker command checked against every registered harness, with and without a worktree
test/leaserig/                                the room's build lease raced between real builds of rewake, each paused at a named step
tools/checksummary/                           summarizes a suite run into a few lines and summary.json
tools/release/                                the release gate: checks, builds with pack.sh, publishes only when told
tools/protogen/                               writes the mod protocol's Go and TypeScript types from its schema; its test fails when a checked-in file differs
```

`cmd/` holds only what the project ships; `tools/` holds development programs that ship
with nobody, and `test/` holds packages that exist only for their tests.
