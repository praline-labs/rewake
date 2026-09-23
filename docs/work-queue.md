# Work queue

The order of what comes next, as the owner set it on September 21, 2026. It lives
apart from [the roadmap](roadmap/README.md) because that file records what has happened —
milestones closed, reviews held, decisions taken — and this one records what has not
happened yet. Mixing the two makes both harder to read, and the roadmap had already
passed the project's 400-line limit.

An entry leaves this file when the work is done: the record of it goes to the roadmap,
and what was learned goes to the research documents.

## Now: the workflow suite

What exists is in the roadmap's [suite entry](roadmap/2026-09-21-workflow-suite.md):
`test/workflow` with its isolation, owned process groups and the shim that plays the
Codex app-server; the Codex session taken to an accepted conversation; the first
selected scenario, `task-report`, with its negative controls (`8b7004b`); the second,
`batch-arrival`, with four controls that mutate the product through a build overlay;
the third, `mid-turn`, a letter steered into a turn the recipient holds open; and the
second-terminal case, which has no reachable control and says so.

What remains, from [check-runner-scenarios.md](check-runner-scenarios.md) and
[check-runner-proposal.md](check-runner-proposal.md): the scenario `ack-recovery`;
and the paid tier, which has not run under the suite. The summarizer the proposal asks for is built
([the record](roadmap/2026-09-22-suite-summarizer.md)); there is still no separate
runner command, by decision.

## Also: the arrival-display stand, rebuilt on the suite

The Python fixture that checked the native arrival row was deleted with its research
package on September 22, 2026; what it did is written down in
[native-mailbox-ui-check.md](native-mailbox-ui-check.md). Everything around the
visible row is something `test/workflow` already does — an isolated case, a shim that
plays the harness, a scripted endpoint, delivery and a read performed by the session
itself, cleanup that is checked rather than assumed.

What stays manual is the one step the fixture could not automate either: a person
looking at the screen and saying whether the row appeared. The suite would carry the
case up to that point, leave the observation to the owner, and record it — the same
division the fixture used, with the machine half no longer in a second language.

## Then: a named harness version against a local responder

The first half of pinning harness versions is done
([the record](roadmap/2026-09-23-harness-versions.md)): a Codex version named in
`REWAKE_CODEX_VERSION` is fetched once into a cache, and the schema case generates the
protocol schema from it in a disposable container. That catches changes of *shape* — a
new required field, a field that disappeared, a type that changed — before the owner
updates their installation, which is what the owner asked for on September 21, 2026.

What remains is the half a schema cannot see: changes of *behaviour* — a different
order of events, a different moment of readiness, a different reaction to a refusal.
That needs a tier with a real harness, of a named version, against a local responder
instead of a model provider, for both harnesses. The pieces it would reuse exist:
`tools/harnesscache/cache` already fetches and keeps both Codex and Claude Code, and
`tools/harnesscache/container` runs either one with a private HOME and a network mode
of the caller's choosing. What does not exist is the responder, and a way to point
each harness at it for one launch without editing its configuration.

## Then: what Claude Code's one-way channel still leaves out

The Codex wrapper talks to its own server: it sends requests and receives events. With
Claude Code the channel runs one way — a line is written into a socket and nothing
comes back. The telemetry half of that gap is closed: since September 23, 2026 a
collector fed by background hooks and a status-line tap reports model, effort, context,
activity and compactions, observed live the same day
([claude-telemetry.md](claude-telemetry.md); HF-11 and HF-22 in the
[feature map](harness-features.md#capability-map)).

Conversation tracking is built as well and passes its fixture case
([the record](roadmap/2026-09-23-claude-thread-tracking.md)); HF-10 waits only for a
`/clear` seen live between a delivery and its report.

What stays open is listed in the feature map's [parity queue](harness-features.md#open):
a keyboard interruption that produces `stopped` (HF-06), for which the harness was shown
on September 23, 2026 to offer no signal, so it waits on the owner's decision. The
harness's own `/clear` and `/resume` were observed working under rewake the same day
(HF-19). Two telemetry
limits remain beside them: activity stays `working` after Esc, and the waiting state is
unknown during a turn ([session-state.md](session-state.md#claude-code-source)).

## Then: Codex's `--worktree`

rewake refuses Codex's `--worktree` today (`internal/harness/codex/codex.go`, the check
before the launch plan; [launch.md](launch.md)). The session-owned app-server starts in
the launch directory, while `--worktree` moves the conversation into a managed worktree
that Codex creates itself. A plain git worktree, created first and launched from,
already works.

Two routes, to be chosen by research rather than built now. If the terminal creates the
worktree and hands its directory to the server in `thread/start`, the refusal may be
unnecessary and the flag can pass through the gateway. If not, rewake would create the
worktree itself before starting the server — which copies Codex's allocation logic
([research-permissions.md](research-permissions.md#managed-worktrees-and-continuation-permissions))
and drifts with every Codex version.

The research: where the installed and the latest Codex handle `--worktree` in their
source, which fields `thread/start` takes in the generated schema, and one probe in the
disposable container without a model call. It is the Codex path, so the launch does not
change until the owner has seen the findings.

## Then: the parity queue

The remaining entries of [harness-features.md](harness-features.md), in its order.

## Also queued, not scheduled

**An orchestrator starts a worker in the background.** The owner's idea for later,
recorded September 23, 2026, after honest delivery status for Claude Code landed:
an orchestrating session starts its worker itself, without a terminal of its own.
Claude Code offers the means — `--bg` starts a session under a supervisor and `claude
attach <id>` opens it in a terminal later
([research-launch.md](research-launch.md#whether-claude-code-has-a-client-server-split-to-sit-between)).
A background worker will likely run without permission prompts, and that is exactly
the class of receiver whose inbound gate holds every rewake line
([traps.md](traps.md#a-message-reported-delivered-was-held-by-claude-code--the-status-is-now-honest)),
so a sender that can tell held from delivered is its prerequisite. Nothing to build yet.

**A view for main of what others owe it.** Proposed with `rewake inbox --owed`, not
asked for by the owner yet ([the record](roadmap/2026-09-23-owed-reread.md)): `--owed`
refuses a main session, whose reads record no obligation, yet after its own compaction
main needs the opposite list — the tasks it waits on from others, found through the other
sessions' `awaiting/` records naming main's run, plus what they have not read yet. It
answers a different question and reads other sessions' mailboxes, so it would be its own
view rather than a mode of `--owed`.

**A slot in the room for heavy test runs.** The owner's idea of September 23, 2026, not
built. Before a heavy run an agent checks whether the slot is free and takes it
explicitly, then runs its tests. An agent that wants to run while the slot is busy takes
a number in a queue and may go idle without ending its task — like a `rewake pending`
mark, waiting in the queue does not close the task. The holder releases the slot when
its tests are done; rewake also releases it when the holder's turn finishes, as a safety
net that agents are not told to rely on. The next one in the queue, by its number, gets
a waking notice that it now holds the slot. Once built, it replaces the manual rule in
the Checks section of `AGENTS.md`.

**Parsing the Codex configuration.** Today rewake looks for a mention of a key in the
text of the file and substitutes nothing when it finds one — crude, and crude on
purpose, because the hand-written parser was removed. With a library this can be done
properly. Its own task, because it touches the Codex adapter: careful, and accepted by
the Codex-side reviewer.

**Launch defaults from the environment** stay as they are, but are not worth
developing further: an alias ([launch.md](launch.md), "Naming a whole launch") states
the choice explicitly, which is what the defaults were approximating.
