# Work queue

The order of what comes next, as the owner set it on September 21, 2026. It lives
apart from [the roadmap](roadmap/README.md) because that file records what has happened —
milestones closed, reviews held, decisions taken — and this one records what has not
happened yet. Mixing the two makes both harder to read, and the roadmap had already
passed the project's 400-line limit.

An entry leaves this file when the work is done: the record of it goes to the roadmap,
and what was learned goes to the research documents.

## Now: the workflow suite

What exists began with the roadmap's [suite entry](roadmap/2026-09-21-workflow-suite.md)
and has grown with nearly every change since: `test/workflow` with its isolation, owned
process groups and the fixtures that play Codex's app-server and Claude Code, and
scenarios across launching, reports, delivery, actions on a sent message, conversations,
the inbound gate and the plugin. The cases and their controls are listed in
[testing-cases.md](testing-cases.md) rather than here, where a list went stale with each
new scenario; the second-terminal case still has no reachable control and says so.

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

What stays open is listed in the feature map's [parity queue](harness-features.md#open).
A keyboard interruption now produces `stopped` (HF-06) through rewake's function-hooks
plugin, stage 1, observation only, built on September 23, 2026 and accepted live by the
owner on September 24, 2026 with an Esc and a Ctrl+C at the keyboard
([claude-plugin.md](claude-plugin.md)); the same plugin ends "working after Esc". The
harness's own `/clear` and `/resume` were observed working under rewake on September 23,
2026 (HF-19). One telemetry limit remains: the waiting
state is unknown during a turn ([session-state.md](session-state.md#claude-code-source)).
Stage 2 of the plugin acts on the session: `rewake compact` and `rewake interrupt`,
built on the Claude Code side on September 24, 2026 ([remote-control.md](remote-control.md))
with its workflow case, `claude-steered`, and accepted live the same day; the Codex side,
part B, was built the same day and accepted live on September 25.

## Then: stage 2 of the plugin, the rest

What [remote-control.md](remote-control.md) describes is built and accepted live on both
harnesses: Claude Code on September 24, 2026, Codex (part B) on September 25 after nine
review rounds ([2026-09-24-remote-control-codex.md](roadmap/2026-09-24-remote-control-codex.md)).
The command returning at once and the worker rules in the role briefing followed on
September 25, 2026 ([2026-09-25-compact-non-blocking.md](roadmap/2026-09-25-compact-non-blocking.md),
[roles.md](roles.md)). What stays open:

**Dropped: a main's own failed turn reporting to nobody.** The owner decided on
September 26, 2026 to leave it as it is, because the error letter a failing main keeps is
unread and unannounced and wakes nobody; the contract in
[turn-outcomes.md](turn-outcomes.md#failed-turns) stays.

**A focus for a Codex compaction, to be researched.** The owner decided on September 24,
2026 that `rewake compact <codex session> <focus>` is refused for now: Codex's
`thread/compact/start` has no field for it, and `compact_prompt` replaces the whole
prompt for the whole conversation. Two routes are to be researched later, not built: a
proper one, if a later protocol version adds a field or a per-request override; and an
emulation through the conversation's history — the focus put in as an item before the
compaction, which unlike instructions stays in the history afterwards.

## Dropped: a todo list sent with a task

The owner asked on September 24, 2026 for a todo list that main sends with a task and
the worker's harness shows as its own checklist. On September 26, 2026 the owner decided
not to build it — neither a list on `rewake send` nor turning the harnesses' checklist
tools on at launch: rewake is about delivery between sessions, and task lists belong to
the harness and to the owner's task trackers. What the research learned stays recorded:
Claude Code's task tools in [research-claude-actions.md](research-claude-actions.md#the-task-list),
Codex's plan tool in [research-codex.md](research-codex.md#the-plan-tool) and
[research-protocol.md](research-protocol.md#the-plan-over-the-protocol).

## Then: the parity queue

The remaining entries of [harness-features.md](harness-features.md), in its order.

## Now: what the directory grants and the worktrees left open

Both stages of the directory grant landed on September 27, 2026 ([grants.md](grants.md);
the roadmap entries of that day), and so did the worktree lifecycle
([worktree.md](worktree.md)). What they left, in the owner's order of September 27:

1. **A grant restored after a cold resume — done September 27, 2026.** Main's wrapper
   confirms the grant again for the run that resumes the conversation, the journal's copy
   follows the conversation, and a resumed run takes over the task's wait
   ([grants-resume.md](grants-resume.md),
   [the record](roadmap/2026-09-27-grant-resume.md)). Left open: a grant whose main has
   ended is not restored, and a copy a worker deletes leaves a Codex root it restored
   un-journaled ([grants-resume.md](grants-resume.md#what-it-does-not-cover)).
2. **A grant confirmed for a Codex main.** Since September 27, 2026 a Codex main cannot
   grant, `--grant-git` included: its sandbox refuses `connect()` on the unix socket
   its `rewake send` registers a grant through ([grants.md](grants.md#who-can-grant)).
   The owner's idea: main's wrapper already sees, in its own app-server's stream, every
   command its model runs (`commandExecution` items); a `rewake send` seen there with
   the grant's arguments may be the confirmation, without a socket. To research first.
3. **Does Codex resolve a root again on each command?** If it does, a granted directory
   whose parent the worker can write can be swapped for a link after delivery; rewake
   refuses the cases it can see ([grants.md](grants.md#what-a-grant-does-not-stop)). A
   probe on the real sandbox settles which.
4. **`land`, `finish` and the checks before `ls` and `rm` beside a foreign `git worktree
   add`.** Creating and removing are serialized per repository
   ([worktree.md](worktree.md#launches-at-once)); these are not, and during someone
   else's `add` — by hand, or Claude Code's own `-w` — they may fail with git's message
   and have to be run again.
5. **An owner file of extra rules for the grant tiers**, when a case needs it.

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
Its first step is research, not verified yet: `claude -p` with stream-json input and
output, where rewake would own the input rather than a terminal, and could then learn of
an interruption (`terminal_reason` `aborted_*`), ask for a compaction and send the other
commands a terminal session does not take from outside
([research-claude-control.md](research-claude-control.md)).

**A slot in the room for heavy test runs.** The owner's idea of September 23, 2026, not
built. Before a heavy run an agent checks whether the slot is free and takes it
explicitly, then runs its tests. An agent that wants to run while the slot is busy takes
a number in a queue and may go idle without ending its task — like a `rewake pending`
mark, waiting in the queue does not close the task. The holder releases the slot when
its tests are done; rewake also releases it when the holder's turn finishes, as a safety
net that agents are not told to rely on. The next one in the queue, by its number, gets
a waking notice that it now holds the slot. Once built, it replaces the manual rule in
the Checks section of `AGENTS.md`.

**A: Codex's own worktree scheme — to study later.** Recorded September 26, 2026, when
the owner chose variant B for `rewake codex --worktree`: rewake makes its own checkout
with the public `git worktree add` and keeps its own record
([launch.md](launch.md#a-worktree-for-a-launch)). Variant A would repeat what Codex's
terminal does in a local launch — its directory layout under
`$CODEX_HOME/worktrees/<id>/<repository>`, `<id>` the first four hex characters of a random UUID, a detached `--no-checkout` add,
`config.worktree`, and the binding of the checkout to the thread in `codex-thread.json`
([research-codex.md](research-codex.md#--worktree-with-a-remote-terminal),
[research-permissions.md](research-permissions.md#managed-worktrees-and-continuation-permissions)).
It is deferred because that scheme is private to the terminal, not a contract: a copy
would drift from a new Codex version without a word, and the checkout would stop being
one Codex recognizes. Worth studying when Codex exposes its worktrees through the
app-server or a subcommand, or accepts `--worktree` beside `--remote`; until then B
gives the substance without the dependency.

**Recording the directory a session actually works in: `claude -w` and `codex -C`.**
Found September 26, 2026. A session registers the working directory its wrapper
started in, while the harness may work elsewhere: Claude Code's `-w` moves it into a
worktree of its own ([research-launch.md](research-launch.md#a-worktree-at-launch)), and
Codex's `-C` into the directory named. So `rewake worktree rm`
does not see a session started with `-C <checkout>` from outside the checkout, and a
clean checkout can go from under it ([launch.md](launch.md#a-worktree-for-a-launch)).
The same record would serve both: the directory each harness reports once it has
started, not the wrapper's.

**A turn cut short to send a message is not a stop — to research.** Observed on
September 28, 2026 on Claude Code 2.1.280: the owner typed a message into a worker's
running turn and sent it at once with Ctrl+Enter. The harness ended the running turn to
take the message, the plugin saw `turn.complete` with reason `aborted`, and main got
`stopped` — "the person at the keyboard stopped this turn" — although the person had
redirected the worker, not stopped it, and the worker went on working. A sender reading
`stopped` does not resend and waits for the person
([turn-outcomes.md](turn-outcomes.md#keyboard-stops)), so the label sends it the wrong
way. To research: what the harness tells a plugin or a hook when an interruption carries a
new message — a prompt submitted right after the abort, a distinct reason, an ordering
of events — and whether rewake can hold a `stopped` briefly to see it and report
something truer ("redirected at the keyboard; still working"). Codex's side of the same
question as well: a steer from the terminal during a turn.

**Parsing the Codex configuration.** Today rewake looks for a mention of a key in the
text of the file and substitutes nothing when it finds one — crude, and crude on
purpose, because the hand-written parser was removed. With a library this can be done
properly. Its own task, because it touches the Codex adapter: careful, and accepted by
the Codex-side reviewer.

**Launch defaults from the environment** stay as they are, but are not worth
developing further: an alias ([launch-defaults.md](launch-defaults.md#naming-a-whole-launch)) states
the choice explicitly, which is what the defaults were approximating.
