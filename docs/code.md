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
internal/proc/                   /proc: identity, liveness, job-control state, lineage and namespaces
internal/boottime/               the boot clock, comparable across processes and never set back
internal/inbox/                  message, status, sender-side write, servicing loop
internal/grant/                  which directories a task may grant, and the journal of what was granted
internal/grantauth/              main's wrapper holding the grants its commands registered, and confirming them; a worker's wrapper keeping its own grants for its permission hook
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
internal/worktree/               checkouts rewake makes for a launch: git worktree add, records, land, removal
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
    ProtectedDirs() []string
    Launch(request LaunchRequest) (LaunchPlan, error)
    Deliver(ctx context.Context, session registry.Session, message inbox.Message) inbox.Result
}
```

`SingleUseFlags` names the flags a harness takes at most once, so an alias and a typed
flag for the same parameter replace rather than repeat each other. `ProtectedDirs` names
the harness's own configuration, which no task may grant a session to write
([grants.md](grants.md)). `Deliver` sends the
notice for `message`, built by `harness.Notice`; it never sends `message.Text`.

A harness whose sessions take `rewake compact` and `rewake interrupt` also implements
`Steerable` — `CompactFocus() bool`, whether a compaction may carry a focus. One that
does not is refused before anything is sent ([remote-control.md](remote-control.md)).


A harness that takes a directory into a running session for one task implements
`DirGrantHarness` — `SupportsDirGrant()`; `rewake send --grant-dir` to a session of any
other harness is refused with exit 1 ([grants.md](grants.md)). A harness whose launch
takes arguments rewake has a way of its own for implements `LaunchRefuser` —
`RefuseLaunch(args)`, asked before anything else of the launch; Codex refuses
`--add-dir` and a `writable_roots` override there. A harness whose main can grant
implements `GrantIssuer` — `ReachesWrapper()`, whether its commands reach their wrapper's
socket; only Claude Code does, and a grant from a main of any other harness is refused
with exit 1 ([grants.md](grants.md#who-can-grant)). A harness that takes a grant through
a permission hook rather than at delivery implements `HookGranter` — `GrantCall(payload)`,
the part of a hook's payload the wrapper needs, and `DecideGrant(call, entries)`, the
answer from the grants the wrapper keeps; the hidden `rewake grant-hook` passes one to
the other over the wrapper's keeper socket ([grants.md](grants.md#claude-code)).

A harness that gives rewake's worktree to a launch implements `WorktreeHarness` —
`WorktreeFlag()`, the spelling the launch command takes for itself;
`WorktreeRefusal(args)`, what the harness cannot do in a checkout of its own, asked
before anything is made (a continued conversation, which stays in the directory it was
started in; for Codex the arguments its launch refuses anyway, for Claude Code its own
`-w` and `--tmux` beside rewake's); and `LaunchDirectory(args)`, the directory the
launch would work in and the arguments without what chose it. The launch command then
makes the checkout in `internal/worktree` and starts the session inside it
([worktree.md](worktree.md)). Codex and Claude Code both implement it: Codex's terminal
cannot make its worktree under `--remote`, and Claude Code keeps `-w` for its own.
