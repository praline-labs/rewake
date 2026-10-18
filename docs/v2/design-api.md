# Design: the adapter API

What an adapter offers the core, as named capabilities, and what the core does when an
adapter — or one run of it — lacks one. The overview is in [design.md](design.md).

## Why capabilities

1.x grew twenty-two optional interfaces beside `harness.Harness`, each found by a type
assertion where it is used: `backend.go` (fifteen, from `GitGrantHarness` to
`ToolWithdrawer`), `mailtool.go` (two), `plan.go` (`Steerable`, `Accepting`),
`worktree.go` and `thread.go` (three) **[code]**. Which harness can do what is spread
over the call sites, and the core learns of a harness by asserting its types. 2.0 groups
them into the eight capabilities of the plan, each optional, each a small interface in
`internal/adapter` carrying narrow values — never `registry.Session` whole, which
`Deliver` hands an adapter today (revision, layering faults). Stage 4 adds a ninth, Spaces,
and states in the offer whether a launch can name its conversation
([below](#conversation-names)).

Rests on: the plan's architecture direction and decision 7 (the third harness is added
without touching the core).

## Two levels: offered and live

A capability is **offered** by the adapter — static, known from the catalogue before a
launch. It is **live** for one run once the run proves it: the Claude Code adapter
offers TurnBoundary, but a run whose mod did not load does not have it
([design-claude.md](design-claude.md#when-the-mod-does-not-load)). The host records the
live set in the session record, and every
decision another session makes about this one — may a task be sent to it, may it be
compacted — reads the live set, not the offered one. Some proofs come only from the running
harness — Claude Code's mod says hello once its harness runs — so an offer says when its
boundary is proven after the start (`TurnBoundaryOffer.AfterStart`), and a launch then does
not wait for it before starting the harness
([stage4-turns.md](stage4-turns.md#readiness-and-admission)).

Rests on: the channel record's rules (a tool can fail to load,
[mail-bridge-channel.md](../archive/mail-bridge-channel.md) rules 1–4), applied to every
capability; the channel itself left with the mail tool on October 18, 2026.

## The capabilities

| Capability | What the adapter does | Without it, the core |
|---|---|---|
| Launch | required: describes and starts one run | — an adapter without Launch is not an adapter |
| Wake | brings a notice to the session: wakes it idle, steers it busy where it can | writes the letter; `send` exits 3, "accepted, no notice can reach this session"; the session reads when it runs `rewake inbox` |
| TurnBoundary | reports a turn's start and end, with the end's answer, and confirms an end before it closes | publishes no report on its own; see [below](#turnboundary) |
| Telemetry | reports context, limits and cost | `list` and main see the state as unknown |
| Control | compacts, interrupts, clears | `rewake compact`, `interrupt`, `clear` to that session exit 1 naming the missing verb; `clear` is designed, not built ([conversation-reset.md](../conversation-reset.md)) |
| PersonUI | shows the person the session's mail state | nothing beyond the notices; stage 6 |
| Spaces | hosts spaces from a main run: a worker in a pseudo-terminal the wrapper owns, shown inside the harness ([stage4-space.md](stage4-space.md)) | `rewake space open` refuses, naming the missing level; see [below](#spaces) |

ToolTransport and Permissions left the design by the owner's decision of October 18, 2026,
with the mail tool and grants ([below](#tooltransport), [below](#permissions)).

### Launch

Everything the catalogue says about a harness today, less the registry: `ID`, `Title`,
`Summary`, `Examples`, `Notes`, `SingleUseFlags` (non-empty, a catalogue test holds it).
Plus:

- **The minimum version** and how to read the installed one ([below](#minimum-versions)).
- **The plan of one run**: the argv and environment for the harness, given a neutral
  description — name, role, room, the briefing text, the working directory or
  worktree, a conversation to resume, the endpoint's address — and the run directory
  the host owns, where the adapter may write what lives exactly as long as the run.
- **Briefing words**: the sentences the role texts need about this harness (how it
  commits). The core's role texts take them from here; they
  name no harness ([design.md](design.md#what-holds-it)).
- **Resume and worktree**, as `Resumer` and `WorktreeHarness` do today.

### The harness process

A service that serves the harness process itself — an endpoint whose every request is
checked by its peer — implements `HarnessWatcher`: the host names the process it started,
with its start time, as soon as `exec` returns, before it waits for anything else. Until
then a request is held, within a bound, and refused after it; from then on a peer that is
not that process, alive with that start, is refused. A launch through `--command` names the
wrapper script it started, so a harness the script runs as a child is not the named process
and is refused: such a run's mod is never admitted.

### Handlers that write the room

A service whose request handlers write the room — a record in the run's directory —
implements `WorkerCounter`: before it starts, the host hands it the run's `Workers`, and
each handler is admitted among the run's workers and dismissed when it returns. Admission
fails once the run begins to end, and the request is refused. The service's close is
bounded: it stops admission, waits for the handlers in flight within its bound and then cuts
the rest, and a handler that outlasts it — stalled in the filesystem, say — stays counted,
so the run's end holds the room's lease and its directory until that handler's last write
rather than the close holding the wrapper's exit
([stage3-rundir.md](../archive/v2/stage3-rundir.md#where-it-is-and-when-it-goes)). Built in S4-5's review
fix, October 13, 2026.

### Wake

Given a notice — its kind line, preview and members — and the run, it delivers and
answers delivered, held (with the harness's reason) or failed, and later, for a held
one, the outcome. The host's delivery loop (inbox `Server`, batches, window, watch, now
in `host`) decides what to send and when; the adapter only carries it. Whether a busy
session is steered or queued is part of the answer, because the notice texts promise
"active work is steered, idle work is woken" (the guide, MAIL).

### ToolTransport

Offered the mail tools to the model and carried their calls under the tool rules. It left
the design with the mail tool by the owner's decision of October 18, 2026; a session uses
the shell commands, and the guide is its interface. What it was is in
[rules-tools.md](../archive/rules-tools.md) and [mail-bridge.md](../archive/mail-bridge.md).

### TurnBoundary

Reports, for the session's own conversation (never a subagent's): a turn started (its
id, and when on the harness's own clock where the harness says); a turn ended (its id, the reason — answered, aborted, refused, failed — and the
final answer); and asks, before an end closes, whether it may: the core answers "go on"
when the end would close a task early ([design-rules.md](design-rules.md#turn-outcomes)).
An adapter that cannot hold an end open says so in what it offers; the core then
cannot keep a task open across an unmarked interim end, and that limit is written in
the adapter's document rather than discovered.

The boundary's reports reach the host on their own, whatever else the run serves. A timed start is the evidence a pending mark of an earlier turn is judged by
when that turn's end was lost
([mail-bridge-turns.md](../archive/mail-bridge-turns.md#a-pending-mark-at-its-turns-end)); the
host records it once.

Without a live TurnBoundary a session's turn ends report nothing, and a worker's
contract — "end your turn; your final reply goes to the sender" — does not hold. The
owner decided (answer 3 of [design.md](design.md#the-owners-answers)): a launch of a
role that must report refuses when the boundary cannot be live — a known cause, named;
once running, the session record says the boundary is not live, so a task or question
sent to it exits 1, naming why; no new word `report` is added. A boundary that was live
and is lost — the mod unloaded, its stream gone — is withdrawn from the live set as soon as
the host sees the loss; on Claude Code that is the lapse of the mod's heartbeat, within 30 s
of its last poll ([the live set](stage4-turns.md#the-live-set)). A withdrawal refuses new
tasks; an end the harness still sends is taken. Notifies and reads still work through
the shell; the shell never stands in for the boundary. On Claude Code the boundary
reaches the host by the mod's checked requests, so it depends on P4
([design-claude.md](design-claude.md#the-socket-and-its-peer)).

### Telemetry

Context tokens, window and percent; rate limits; cost; folded into the session-state
snapshot (`core/session`) by event time, a late event never overwriting a newer one
(`telemetry/state_test`'s property, carried). Reading a transcript is not a source.

### Control

Three verbs, each answered on its own: compact (with optional instructions), interrupt
the running turn, clear the conversation. Requests travel as files in the run's control
directory (`core/control`, carried); the adapter decides how it hears them. A verb the
adapter does not offer is refused by the CLI before a file is written.

### Permissions

Applied a grant — a directory, the Git metadata — to the run. Grants left the code by the
owner's decision of October 18, 2026 ([grants.md](../archive/grants.md)); a session writes
and commits within its own permissions.

### PersonUI

The session's mail state shown to the person (a pane, a status). Stage 6 (decision 8).

### Spaces

A main run that hosts spaces: it opens a worker in a pseudo-terminal its wrapper owns and
shows it inside the harness ([stage4-space.md](stage4-space.md)). Three levels decide it, and
a refusal names the one that is missing:

- **The role.** `Role.Spaces` says a session of the role opens spaces: main alone. Opening one
  from another role is a wrong call, exit 2.
- **The offer.** `Offer.Spaces` says a run of the harness can host them. A harness that does not
  offer them is a wrong call too, exit 2, naming the harness; so is a session recorded by a build
  whose catalog has no such harness.
- **The live set.** Spaces are never planned: a run proves them, so an adapter that offers them
  has its plan carry a `LiveReporter`, and a catalog test holds it. A run of a role that opens
  spaces reports `spaces` once it can host them and withdraws it when it loses them. A main
  session without them live is refused as a target, exit 1: its run has not proven them or has
  lost them, which a later run may not repeat.

On Claude Code the proof is the mod's hello: an admitted hello on a main run makes Spaces live,
and its answer says `hostsSpaces`, so the mod serves the spaces' side of the protocol. A mod
that is lost takes them out of the live set when the host sees the loss — the lapse of its
heartbeat, the signal that withdraws its turn boundary, built in S4-6
([the live set](stage4-turns.md#the-live-set)). A run whose mod did not load never has them.
The fixture offers none until S4-7 gives
it spaces through the host's neutral seam, never through another adapter's mod. The refusal is
`spacesRefusal` in `internal/cli`, which `rewake space open` asks before anything is written;
the command itself is S4-7's. Built in S4-5, October 13, 2026.

### Conversation names

A space's record keeps its worker's conversation id, so the worker can be found and restored
([stage4-space.md](stage4-space.md#4-the-workers-launch)). rewake chooses the id, a UUID,
before the launch and hands it in `LaunchRequest.Conversation`; the adapter's Launch passes it
with the harness's flag. Every adapter answers in its offer whether it can, either way —
`ConversationOffer.Flag`, or `Unnamed` with the reason there is none — because the zero value
would read as both "cannot" and "forgot to say". A catalog test holds the answer: exactly one
of the two; a flag taken once with a value, carried by Launch with the id; and, without one,
Launch refusing an id rather than dropping it. Claude Code answers `--session-id <uuid>`
(`claude --help`, 2.1.296) and refuses rewake's id beside arguments that already name or continue
a conversation; the fixture answers that it takes no id. Built in S4-5, October 13, 2026; the
host chooses the id when S4-7 opens a space.

## The catalogue

`adapter/catalog` exposes one constructor that returns the catalogue value — the list
of adapters in the order the guide shows them. `cmd/rewake` calls it and hands the
value to `cli`; `cli` hands each launch its adapter and the host the interfaces. The
guide, each harness's help page, the step in FLOW and the `harnesses` field of the
machine form all derive from that value, as today. Adding a harness stays one package
plus one line, now in the constructor instead of an `init`
(`harness/harness.go:13-23` today).

## Minimum versions

Each adapter declares its **minimum version** and how to read the installed one. A
launch reads it before the claim (as 1.x does since `aee71ea`):

- **below the minimum**: exit 1, "Claude Code 2.1.280 is installed; rewake needs
  2.1.287 or later" — the installed and the minimum named;
- **unreadable**: exit 1, saying the version could not be read and how it was asked
  (the owner's answer 6);
- **at or above it**: the launch goes on. A version newer than the last one a live
  check ran on is not refused; the launch notes it once, as the pin in
  `codex/server_version.go:31` notes today.

Exact versions in gates go (answer 8): a gate is a minimum or nothing. Claude Code's
minimum is 2.1.287 (decision 1); Codex's is set in stage 5.

**The legacy mark stays as a mechanism with an empty table** (answer 11): the rules of
`docs/legacy.md` and `docs/legacy_test.go` move to 2.0 unchanged, with no marks and the
table of oldest supported versions holding the minimums above. The table and the
adapters' declared minimums are one fact: the test reads both and fails when they
differ. The first raise of a minimum in 2.x turns the marks below it red, as designed.

The mod API promises no stability: the `mods` skill states that it changes in patch
releases too, and the skill is kept current for that reason. A stage 4 run starts by
comparing the skill's version with the installed one; each probe result is written with
the version it ran on.
