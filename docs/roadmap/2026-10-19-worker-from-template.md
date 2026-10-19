# A worker only from a template

October 19, 2026, by the owner's decision of that day ([worker-profile.md](../v2/worker-profile.md#decisions),
decision 8). **Done.** `rewake space open <name> --alias <alias> [--worktree[=<name>]] -- <harness args>`
is the only way to raise a worker: `--alias` is required, and `--harness` and `--role` are gone
from every build, refused as unknown flags; the role is the alias's. The `default` memory of a
worker opened without an alias went with them: a Claude Code launch with no template — only a
test build's direct worker launch — has no memory. Help, the guide's flow, main's briefing,
launch.md, launch-defaults.md, flow.md, roles.md, install.md and rules/space.md say so.
**Open.** The review.
