# A worker's memory, one per template and project

October 19, 2026, step 5 of [the worker profile](../v2/worker-profile.md), by the owner's
decision of that day. **Done.** A Claude Code worker has Claude Code's own auto-memory in
`~/.config/rewake/claude/worker/memory/<project>/<template>/`: the project is the main
repository's root, named as a room is, so worktrees share it; the template is the space's alias,
`default` without one, passed by the space host as `REWAKE_SPACE_TEMPLATE`. The launch lays the
two memory keys over the folder's settings in a copy in the run's directory; a person's `false`
holds. Defaults version 2 retires `autoMemoryEnabled: false`. Live with haiku: two templates in two
worktrees of one repository, each reading its own seeded `MEMORY.md`. **Open.** The review.
