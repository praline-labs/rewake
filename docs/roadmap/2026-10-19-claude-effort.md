# Claude Code's effort in list

October 19, 2026, on Claude Code 2.1.302. `rewake list` showed effort unknown for every Claude
Code session. The mod now sends the effort of the main loop's latest model request
(`turn.step` without `agentId`) as a new optional `effort` of `StateRequest`; the host keeps the
last one seen across loads. Before one, the launch's own `--effort` (typed, an alias's `[launch]`
or `REWAKE_CLAUDE_EFFORT`) shows marked `(launch)`, the snapshot's new `reasoningEffortSource`;
with neither it stays unknown. Live with haiku ([research](../research-claude-mod.md#effort)).
Open: a change made with `/effort` shows from the next request; acceptance of the new fields.
