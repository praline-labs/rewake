# Work queue

The order of what comes next, as the owner set it on September 21, 2026. It lives
apart from [roadmap.md](roadmap.md) because that file records what has happened —
milestones closed, reviews held, decisions taken — and this one records what has not
happened yet. Mixing the two makes both harder to read, and the roadmap had already
passed the project's 400-line limit.

An entry leaves this file when the work is done: the record of it goes to the roadmap,
and what was learned goes to the research documents.

## Now: the workflow suite

The delivery scenario and its controls are committed (`8b7004b`). What remains of that
package is in [check-runner-scenarios.md](check-runner-scenarios.md): two more
scenarios — `batch-arrival` and `mid-turn` — and the runner itself, described in
[check-runner-proposal.md](check-runner-proposal.md).

## Next: launch aliases, and the first dependency

The owner's decision of September 21, 2026: the rule that this project uses only the
standard library is lifted. Dependencies are allowed, one deliberate decision at a
time; the criterion is no transitive dependencies and live maintenance, and one library
that covers several places beats three that each cover one.

The first is `pelletier/go-toml/v2`, chosen on reconnaissance: actively maintained,
rewritten this year, faster, current specification — and, like its main alternative,
with no transitive dependencies of its own. That was the deciding property.

What it enables first: **launch aliases**. A short name instead of a long command —
`rewake wcodex` in place of a launch carrying a role, a name, a model and a
configuration override. An alias expands into arguments and the launch proceeds as
usual: environment defaults still fill what the alias did not name, and an explicit
flag on the command still wins over the alias. An unknown name is a refusal listing
the names that exist, and an alias that expands into something unusable is a refusal
showing what it expanded to. An alias names arguments to rewake and nothing else: it
cannot run a command of its own, for the same reason the settings file has no
substitution.

Arguments are a list, not a string. A string would have to be split into words, and
that means quoting, spaces inside values, and every other place this project has
already been caught today.

Why this became possible only now: a hand-written TOML parser here broke on valid TOML
through two review rounds and was removed. The format was not the problem; writing the
parser was.

## Then: a role-shaped first page

An agent launched through rewake should have the minimal order of actions for its own
role within reach. Two ways, and they do not exclude each other.

The briefing at launch explains what rewake is and points at the guide; it could also
carry a short flow — what an executor does with a task it has just read, and how to
finish. And the guide could recognize who is calling it: the session variables are
already in its environment and the role is known, so it can show the section for that
role — task-setting, telemetry and grants for a main session; reading mail, finishing
by ending the turn, and writing mid-work only when an answer is needed, for an
executor — beside the part everyone needs.

What makes it worth doing: on September 21, 2026 an executor sent every report by hand
*and* let the turn report it, all day, because no instruction said the report sends
itself. Both documents described the mechanism; neither told anyone what to do.

The condition: the briefing and the guide must not say different things. The briefing
arrives first and will be believed, so either one source of text feeds both, or the
split between them is explicit about which covers what. Roughly half a day.

## Then: pinning harness versions

Take an arbitrary version of Codex into a disposable environment, generate the protocol
schema from it, and run our messages against that schema. The point is to learn what
broke **before** the owner updates their installation, in the owner's words: "download
and check new versions, so as not to test on me live."

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
developing further: an alias states the choice explicitly, which is what the defaults
were approximating.
