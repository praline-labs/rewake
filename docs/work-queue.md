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
selected scenario, `task-report`, with its negative controls (`8b7004b`); and the
second-terminal case, which has no reachable control and says so.

What remains, from [check-runner-scenarios.md](check-runner-scenarios.md) and
[check-runner-proposal.md](check-runner-proposal.md): the scenarios `batch-arrival` and
`mid-turn`, then `ack-recovery`; a fixture for the Claude Code column, which has none;
the summarizer over `go test -json` that gives the short console output and the summary
file the proposal asks for — there is no separate runner command, by decision; and the
paid tier, which has not run under the suite.

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

## Then: pinning harness versions

Take an arbitrary version of Codex into a disposable environment, generate the protocol
schema from it, and run our messages against that schema. The point is to learn what
broke **before** the owner updates their installation: the owner asked, on September
21, 2026, that new versions be downloaded and checked rather than tested on their
live setup.

This is the one place in the project where docker is warranted — a disposable container
is cleaner than temporary directories, and the owner's installation is not touched at
all. For the rest of the suite it would be overhead.

The boundary, stated honestly: a schema catches changes of *shape* — a new required
field, a field that disappeared, a type that changed. It does not catch changes of
*behaviour* — a different order of events, a different moment of readiness, a different
reaction to a refusal. Those need a tier with a real harness against a local responder,
which does not exist yet.

## Then: a two-way channel for Claude Code

The Codex wrapper talks to its own server: it sends requests and receives events. With
Claude Code the channel runs one way — a line is written into a socket and nothing
comes back. Four rows of the [feature map](harness-features.md) are missing for that
single reason: telemetry, compactions, interruption, and following the conversation.

The leads are known. The Agent SDK reports the available models and can report context
usage. And there is a channel where the harness calls a command *we* name and hands it
data about the session — the same shape by which the end of a turn already reaches us
(`internal/harness/hooks.go`). The question for the research is whether Claude Code has
a two-way path structured like the Codex one, and what it would give.

## Then: the parity queue

The remaining entries of [harness-features.md](harness-features.md), in its order.

## Also queued, not scheduled

**Parsing the Codex configuration.** Today rewake looks for a mention of a key in the
text of the file and substitutes nothing when it finds one — crude, and crude on
purpose, because the hand-written parser was removed. With a library this can be done
properly. Its own task, because it touches the Codex adapter: careful, and accepted by
the Codex-side reviewer.

**Launch defaults from the environment** stay as they are, but are not worth
developing further: an alias ([launch.md](launch.md), "Naming a whole launch") states
the choice explicitly, which is what the defaults were approximating.
