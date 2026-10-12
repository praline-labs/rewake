# Source layout

Where each part of rewake lives, and the one interface every harness adapter
implements. Split out of [design.md](design.md), which keeps what the system is and why;
this file is what to open when looking for where something is in the tree. `go list
./...` names every Go package below; a new one is added here in the same change, and
`docs/layout_test.go` fails until it is.

```
cmd/rewake/                                   entry point, top-level parsing
internal/                                     the layout test of 2.0: the import rule, harness names outside the adapters, no registry
internal/cli/                                 command table, parsing, overview, help, failures, printing
internal/infra/state/                         directory: checks, paths, atomic writes
internal/infra/buildtime/                     durations a build may shorten through -ldflags, for the suite
internal/core/registry/                       session record, name publishing, liveness and listing
internal/core/registry/registrytest/          for tests: the boot a session of this build carries
internal/infra/proc/                          /proc: identity, liveness, job-control state, lineage and namespaces
internal/infra/boottime/                      the boot clock, comparable across processes and never set back
internal/core/mail/                           message, status, sender-side write, the locked sweep and settlement, turn ends and the read's acknowledgment
internal/core/mail/mailtest/                  for tests outside mail: the paths and bytes of its records, each checked against mail
internal/core/receipt/                        the journal of mail operations run through the tool's words: keys, records, frozen reads
internal/bridge/                              what the CLI and the context endpoint share: the ticket, result bounds, cutting parts
internal/bridge/endpoint/                     the wrapper's context endpoint: hellos, tickets, completions, the gate against a turn's end, a transport's calls
internal/core/channel/                        the run's mail channel: tool and shell observations, the block, display and notices
internal/core/grant/                          which directories a task may grant, and the journal of what was granted
internal/core/grant/authority/                main's wrapper holding the grants its commands registered, and confirming them
internal/core/grant/authority/authoritytest/  for tests: a helper process that is a recipient's run, confirms its grants at delivery and ends
internal/core/control/                        a main's control request to a run and its answer, as files
internal/core/role/                           the role catalogue: flag, briefing line, reporting duty
internal/core/brief/                          text injected into an agent, independent of transport
internal/alias/                               launch aliases: a short name turned into launch arguments
internal/core/session/                        optional, epoch-scoped observations of a harness (telemetry)
internal/adapter/                             the adapter API: the Harness interface, the catalogue value, launch plans, version gates, flags, defaults, settings
internal/adapter/catalog/                     the one constructor of the catalogue, which cmd/rewake builds and hands to the CLI
internal/adapter/claude/                      launch arguments, environment, socket delivery
internal/adapter/fixture/                     the test harness behind rewakefixture: a scripted program over a socket, every contract interface
internal/wrap/                                wrapper: launch, signals, lifecycle
internal/host/launch/                         a launch's host side: the run's environment, the version read before the claim, the bounded probe
internal/host/delivery/                       the delivery server: the serving loop, notices and their batches, the coalescing window, outcomes and the sweep's lock
internal/worktree/                            checkouts rewake makes for a launch: git worktree add, records, land, removal
scripts/                                      packaging scripts and the tests of the npm shim
docs/                                         the documentation, and the tests that keep it true to the tree
test/workflow/                                the workflow suite: end-to-end scenarios against fixtures
test/workflow/record/                         the record format a suite run prints and the summarizer reads
test/toolrig/                                 the neutral rig: the tool path's oracles on the fixture's program and the host endpoint; the delivery server against the fixture's reservation, the test playing the program
tools/checksummary/                           summarizes a suite run into a few lines and summary.json
tools/release/                                the release gate: checks, builds with pack.sh, publishes only when told
```

`cmd/` holds only what the project ships; `tools/` holds development programs that ship
with nobody, and `test/` and `docs/` hold packages that exist only for their tests.

## The adapter API

An adapter implements the capabilities of [design-api.md](v2/design-api.md), declared in
`internal/adapter`. Launch, which every adapter has, is the `Harness` interface of
`internal/adapter/plan.go`, shown here without its comments:

```go
type Harness interface {
    ID() string
    Title() string
    Summary() string
    Examples() []string
    Notes() []string
    SingleUseFlags() []Flag
    ProtectedDirs() Protected
    Version() VersionGate
    EnvNotInherited() []string
    Offers() Offer
    Launch(request LaunchRequest) (LaunchPlan, error)
}
```

`SingleUseFlags` names the flags a harness takes at most once, so an alias and a typed
flag for the same parameter replace rather than repeat each other. `ProtectedDirs` names
the harness's own configuration, which no task may grant a session to write
([grants.md](grants.md)): `Dirs`, absolute directories outside any project, and `Names`,
the directories it keeps inside one. `Version` is the gate the host reads before the
claim, and `EnvNotInherited` the variables the host leaves out of `LaunchRequest.Env`.

The role texts and the CLI's help name no harness. A help text that needs one takes it
from the catalogue it was built from (`internal/cli/registry_words.go`): which harnesses
lack a grant, the metadata names a grant refuses, a launch to show. A briefing that
needs a sentence about its own harness — how a session on it commits — gets it from the
adapter that builds it, as `brief.Context.Harness`, a section after the role's own; the
Claude Code adapter tells a role that may commit that it does so within its own
permissions.

What an adapter offers beyond Launch is a value, `Offers()` (`internal/adapter/offer.go`),
known before any launch: whether it wakes a session, the tool transport it carries,
whether it reports turn ends and can hold one, whether it collects telemetry, which
Control verbs it takes (`rewake compact`, `rewake interrupt`, a compaction's focus), and
which Permissions it serves (a directory into a running session, Git metadata, a main's
grant reaching its wrapper). The CLI asks this value, through `Catalog.Offers(id)`, before
it writes anything: a verb or a grant the target's adapter does not offer is refused
there ([remote-control.md](remote-control.md), [grants.md](grants.md#who-can-grant)).
That an adapter offers a capability says nothing of one run; what a run serves is its
`LaunchPlan`, and what it made live in its exchange it reports through `LiveReporter`
(`internal/adapter/run.go`): the whole set once the exchange has run and again at every
change, a lost capability at once. The host records each set in the session record
(`registry.Session.Live`, written under the name's lock by `registry.Modify`), and every
decision another session makes about the run reads it there — a send, a task or a
question, a steer — never the offer (S11). A run whose plan carries no reporter has live
what its plan carries, from its start.

The plan carries one run's capabilities as interfaces the wrapper calls and never
asserts on the adapter: `Service`, started before the harness and closed with it, a
`ProcessService` when it owns a process; `Wake`, which delivers the notice built by
`mail.Notice` and never `message.Text`, a `Reserver` and a `Receiver` where the run
answers before the letter; `TurnBoundary`, handed a `TurnHandler` before the service
starts; `Telemetry`; and `Thread`, the run's conversation. A run without a `Wake`
answers each delivery that no notice can reach the session. A tool transport is the
service's `ToolTransport`, generic over the descriptor so the API names no tool-layer
type. An adapter that follows the conversation from the outside is a `ThreadTracker`,
given the run's process; one that continues a conversation a launch names is a
`Resumer`.

Every turn end a `TurnBoundary` reports is a `TurnEnd` (`internal/adapter/turn.go`):
the event it came from, the end's reason — answered, aborted, refused or failed — and
the answer in the harness's words, with the scope captured from the run's read clock.
The core refuses an end that names no event or no reason it knows, and derives the
report from the reason: answered reports finished, aborted stopped, failed and refused
an error ([turn-outcomes.md](turn-outcomes.md)). An adapter that can hold an end
confirms it through `TurnHandler.Confirm` rather than publishing it, so the core can
continue the turn instead of closing it. A turn's start goes to `TurnHandler.Started`,
tool or none: the wrapper records a timed start (`mail.RecordTurnStart`) and, where the
run has a mail tool, opens the turn in the endpoint's table from the same call, so the
tool's input (`ToolInput`) carries a turn's end, the calls and their results, and no
start.

The adapters reach the CLI as one value. `catalog.New()` returns an `adapter.Catalog` in
guide order; `cmd/rewake` builds it and passes it to `cli.Run`, which derives the command
table from it for that invocation. No package holds adapters at package level and none
registers one from `init` (rule 3 of [design.md](v2/design.md#the-test-that-holds-it)),
so a test builds a catalogue of its own. `Catalog.Protected()` is the set the grant rules
receive: every adapter's `ProtectedDirs`, and what the catalogue declares for harnesses
this build has no adapter for — `~/.codex`, `.codex` and `.agents` until stage 5's
adapter declares them itself.

The fixture offers every capability the core has a part in, in a build that carries it,
so the workflow suite holds the core's part of control, grants and worktrees on its gate
column ([testing-cases.md](testing-cases.md)); Claude Code offers Wake and a main's grant
reaching its wrapper.

A harness that gives rewake's worktree to a launch implements `WorktreeHarness` —
`WorktreeFlag()`, the spelling the launch command takes for itself;
`WorktreeRefusal(args)`, what the harness cannot do in a checkout of its own, asked
before anything is made (a continued conversation, which stays in the directory it was
started in; for Claude Code its own `-w` and `--tmux` beside rewake's); and
`LaunchDirectory(args)`, the directory the launch would work in and the arguments
without what chose it. The launch command then
makes the checkout in `internal/worktree` and starts the session inside it
([worktree.md](worktree.md)). Claude Code implements it, keeping `-w` for its own.
