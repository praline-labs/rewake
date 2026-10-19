# The worker profile, step 1

October 19, 2026, on Claude Code 2.1.302. Every Claude Code launch but main's gets rewake's
worker profile as launch options: `--setting-sources project,local`, one `--settings` JSON with
auto-memory off, the person's `CLAUDE.md` excluded (also as an ancestor's) and the updater and
feedback off, and the person's `~/.config/rewake/claude/worker/` merged in when present.
`launch.context` applies as `--autocompact`. A `--command` wrapper keeps its own configuration.
Live through a built rewake with haiku ([worker-profile.md](../v2/worker-profile.md#steps)).
Open: a worker opened in a pane; steps 2 (the catalogue) and 3 (the launch surface, the room
by directory).
