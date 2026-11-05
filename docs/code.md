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
internal/infra/build/                         the build id (SHA-256 of the image), the room's build lease and its own copy of its build
internal/infra/build/buildtest/               for tests: a room made and its build recorded the way a launch of the test binary does
internal/infra/buildtime/                     durations a build may shorten through -ldflags, for the suite
internal/infra/parts/                         rewake's parts: the manifest beside an install, the digests, and the plan of what to restart
internal/core/registry/                       session record, name publishing, liveness and listing
internal/core/registry/registrytest/          for tests: the boot a session of this build carries
internal/infra/proc/                          /proc: identity, liveness, job-control state, lineage, namespaces, owner and initial environment
internal/infra/boottime/                      the boot clock, comparable across processes and never set back
internal/infra/proxyenv/                      for tests: the proxy variables of an environment, carried into one built from nothing
internal/infra/memfile/                       files in memory alone (memfd), for what an image hands the next across an exec
internal/core/mail/                           message, status, sender-side write, turn ends; `Box` is one run's own mail (room, name, epoch), the one handle a command opens on it: its read, its pending mark, what it owes, its acts on the letters it sent (send, edit, withdraw, addendum, recall), each taking the lock itself, and what it is told of them, and `Locked` the operations that run under its mailbox lock, settling a letter with a result or by its status among them
internal/core/mail/mailtest/                  for tests outside mail: the paths and bytes of its records, each checked against mail
internal/core/control/                        a main's control request to a run and its answer, as files; the words of an agent's refusal code and the interrupt's attribution and settling
internal/core/ask/                            a worker's permission requests and questions, held in its wrapper while main answers them
internal/core/role/                           the role catalogue: flag, briefing line, reporting duty
internal/core/brief/                          text injected into an agent, independent of transport
internal/core/worktype/                       the kinds of work a worker is raised for: the built-in texts and the person's and project's own
internal/core/workmode/                       pi's closed runtime stances, configuration overlays, type defaults and additive instructions; never permissions or Git authority
internal/core/session/                        optional, epoch-scoped observations of a harness (telemetry): the Observer every adapter reports to, which merges them into the snapshot, and the activity verdict on it
internal/core/sessiontools/                   the session tools every adapter offers — the brief and worktree tools' names, descriptions, input schemas, which role is offered which, the check of an input into a rewake command's argv and variables or a refusal, bounds and the wording of a raw outcome; the agents get the table as data and hold no copy
internal/core/mirror/                         the mirror: the model of a session's conversation the face draws, its socket, and the decoder of the neutral events the mod's feed sends (the feed itself is the mod's, mod/hooks/feed.ts)
internal/core/screen/                         a window as cells for `rewake screen`: the frame, its text, its PNG with the embedded font, the picture's file with its refusals
internal/core/space/                          the record of each space a main opens: its states, its steps, the kept checkouts' reminder
internal/core/work/                           a run's room work as its input shows it: the open briefs bound to it with their ticks and the reports it waits on
internal/adapter/                             the adapter API: the Harness interface, the catalogue value, launch plans, version gates, flags, defaults, settings
internal/adapter/catalog/                     the one constructor of the catalogue, which cmd/rewake builds and hands to the CLI
internal/adapter/claude/                      launch arguments, environment, socket delivery, the mod embedded from its mod/ directory and written per run
internal/adapter/claude/modhost/              the wrapper's end of the mod's protocol: the HTTP server on the run's mod socket, each request checked by its peer, hello, a turn's start and end, mod/ended, the tools' plan and the wording of their raw end; the types tools/protogen writes from internal/adapter/claude/mod/protocol.schema.json
internal/adapter/pi/                          launch arguments, embedded TypeScript extension and management, peer-checked socket for notices, settled ends, telemetry and control, the tools' plan and the wording of their raw end, the translator of pi's raw events into the mirror's (translate*.go) and the extension's bounded stream of them (stream.ts, forward.ts)
internal/adapter/fixture/                     the test harness behind rewaketest: a scripted program over a socket, every contract interface
internal/host/                                wrapper: one run as a value through its phases — claim, prepare, serve, start, deliver — entered as a launch or as the adoption of a run an earlier image handed over (entry.go, adoption.go, the hold of one that fails in unadopted.go), its re-execution, signals and lifetime
internal/host/launch/                         a launch's host side: the run's environment, the version read before the claim, the bounded probe, the worker aliases main opens, a worker raised from one (`Set.Raise`: its worktree and type resolved, every conflicting request refused) and the worktree flag read for every launch
internal/host/herdr/                          main reporting itself to herdr, the terminal manager it runs in: its state from the telemetry, its resume command, the release
internal/host/delivery/                       the delivery server: the serving loop, notices and their batches, the coalescing window, when an outcome is recorded and settled and the memory of it, and the sweep's lock
internal/host/terminal/                       a program's pseudo-terminal by the standard library, whether a file is a terminal, the person's terminal's modes, the screen the program draws with its input modes, keyboard protocols and title, and the raw stream with its preamble for a late client, the one package the emulator library enters
internal/host/reviewcheckout/                 a reviewer's checkout from the brief's assignment to its release: create, bind, release the one before, and whose checkout it is at a brief's close and a run's end
internal/host/stream/                         a host's relay between its program's pseudo-terminal, its emulator and its socket: the raw stream and the snapshot, shared by the space host and main's host
internal/host/mainhost/                       main's host, the pane's process of a main at a terminal: main's pseudo-terminal, the shell as a replaceable child, the versioned channel between them, restart, crash and close told apart, main alone when no shell starts, the door rewake restart asks, the shell it runs as a part, and its handover to its next image in a switch of the room's build
internal/host/spacehost/                      the space host: the worker's pseudo-terminal and screen, served through internal/host/stream; and opening a space for main (`Open`): the record, the host started, the wait for the worker's registration, the rollback
internal/host/switching/                      a switch of the running room to another build between its leader, rewake restart rewake, and the room's wrappers and hosts: asking, the drop, go or resume, and a parked image's wait (rules/state.md#switching-a-running-room)
internal/host/worktree/                       checkouts rewake makes for a launch: git worktree add, records, land, removal and what keeps one, a worker's release at its close; a reviewer's detached checkout
shell/src/                                    the shell (shell.md): life.ts its life, the host's channel from hello to exit and each pane's stream, with everything it acts on handed in; main.tsx the entry that wires it to the process, the renderer and the room's poll; the panes, the bar, the strip, the briefs and the faces beside
scripts/                                      packaging scripts and the tests of the npm shim
test/workflow/                                the workflow suite: end-to-end scenarios against fixtures
test/workflow/record/                         the record format a suite run prints and the summarizer reads
test/launch/                                  a space's worker command checked against every registered harness, with and without a worktree
test/leaserig/                                the room's build lease raced between real builds of rewake, each paused at a named step
tools/checksummary/                           summarizes a suite run into a few lines and summary.json
tools/release/                                the release gate: checks, builds with pack.sh, writes each build's manifest, publishes only when told, waits for the registry with --wait
tools/land/                                   lands an accepted branch on v2: merges, runs a step's last-commit checks, pushes, installs rewake2's build and its manifest and says what to restart, or resets on red
tools/protogen/                               writes the Go and TypeScript types of three schemas, the mod protocol's, the shell's room view (internal/cli/room_view.schema.json) and the mirror's, whose TypeScript goes to the face (face/src/mirror.gen.ts); its test fails when a checked-in file differs
```

`cmd/` holds only what the project ships; `tools/` holds development programs that ship
with nobody, and `test/` holds packages that exist only for their tests.
