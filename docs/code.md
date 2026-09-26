# Source layout

Where each part of rewake lives, and the one interface every harness adapter
implements. Split out of [design.md](design.md), which keeps what the system is and why;
this file is what to open when looking for where something is in the tree. `go list
./...` names every Go package below; a new one is added here in the same change, and
`docs/layout_test.go` fails until it is.

```
cmd/rewake/                      entry point, top-level parsing
internal/cli/                    command table, parsing, overview, help, failures, printing
internal/state/                  directory: checks, paths, atomic writes
internal/buildtime/              durations a build may shorten through -ldflags, for the suite
internal/registry/               session record, name publishing, liveness, listing
internal/proc/                   /proc: identity, liveness and job-control state
internal/boottime/               the boot clock, comparable across processes and never set back
internal/inbox/                  message, status, sender-side write, servicing loop
internal/control/                a main's control request to a run and its answer, as files
internal/role/                   the role catalogue: flag, briefing line, reporting duty
internal/brief/                  text injected into an agent, independent of transport
internal/alias/                  launch aliases: a short name turned into launch arguments
internal/sessionstate/           optional, epoch-scoped observations of a harness (telemetry)
internal/harness/                the Harness interface, launch plans, notices, hooks, defaults
internal/harness/catalog/        the one list of harnesses that exist
internal/harness/claude/         launch arguments, environment, socket delivery
internal/harness/claude/telemetry/  what a Claude Code session says about itself, carried to its wrapper
internal/harness/codex/          owned app-server, WebSocket RPC, thread events and delivery
internal/harness/codex/gateway/  the terminal gateway: selection, reservation, native mailbox
internal/wrap/                   wrapper: launch, signals, lifecycle
internal/worktree/               checkouts rewake makes for a launch: git worktree add, records, removal
scripts/                         packaging scripts and the tests of the npm shim
docs/                            the documentation, and the tests that keep it true to the tree
test/workflow/                   the workflow suite: end-to-end scenarios against fixtures
test/workflow/record/            the record format a suite run prints and the summarizer reads
tools/checksummary/              summarizes a suite run into a few lines and summary.json
tools/harnesscache/              fetches and caches harness versions, runs them in a container
tools/harnesscache/cache/        resolving, downloading, verifying and keeping a version
tools/harnesscache/container/    the disposable container a cached version runs in
```

`cmd/` holds only what the project ships; `tools/` holds development programs that ship
with nobody, and `test/` and `docs/` hold packages that exist only for their tests.

## The harness interface

A harness adapter implements one interface, `internal/harness/plan.go`, shown here
without its comments:

```go
type Harness interface {
    ID() string
    Title() string
    Summary() string
    Examples() []string
    Notes() []string
    SingleUseFlags() []Flag
    Launch(request LaunchRequest) (LaunchPlan, error)
    Deliver(ctx context.Context, session registry.Session, message inbox.Message) inbox.Result
}
```

`SingleUseFlags` names the flags a harness takes at most once, so an alias and a typed
flag for the same parameter replace rather than repeat each other. `Deliver` sends the
notice for `message`, built by `harness.Notice`; it never sends `message.Text`.

A harness whose sessions take `rewake compact` and `rewake interrupt` also implements
`Steerable` — `CompactFocus() bool`, whether a compaction may carry a focus. One that
does not is refused before anything is sent ([remote-control.md](remote-control.md)).


A harness whose own worktree flag cannot work under rewake implements
`WorktreeHarness` — `WorktreeFlag()`, the spelling the launch command takes for itself;
`WorktreeRefusal(args)`, what the harness cannot do in a checkout of its own, asked
before anything is made (Codex: a continued conversation, which stays in the directory
it was started in, and the arguments its launch refuses anyway); and
`LaunchDirectory(args)`, the directory the launch would work in and the arguments
without what chose it. The launch command then makes the checkout in `internal/worktree`
and starts the session inside it ([launch.md](launch.md#a-worktree-for-a-launch)). Only
Codex implements it; Claude Code makes its own.
