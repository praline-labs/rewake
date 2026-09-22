# Launch defaults from the environment and from settings files — done, September 21, 2026

A launch may take its model and reasoning effort from `REWAKE_CODEX_MODEL`,
`REWAKE_CODEX_EFFORT`, `REWAKE_CLAUDE_MODEL` and `REWAKE_CLAUDE_EFFORT`, supplied as
flags for that one launch and announced in a launch note. Strongest first: a flag on
the line, a variable already in the environment, `.rewake.env` in the working
directory, then `~/.config/rewake/settings`. An unset value means no default and no
guess; the set of names a file may decide is closed, so a file found in whatever
directory somebody is in cannot steer the state directory or the room. No model name
is in the repository.

Where it lives: `internal/harness/defaults.go` for the substitution and the note,
`internal/harness/settings.go` for the files and their order, and in each adapter
the pair of defaults that harness takes. Documented in [launch.md](../launch.md) under "Launch defaults
from the environment"; what each harness accepts for a model and an effort, and in
which spelling, is in [research-launch.md](../research-launch.md). The workflow suite's
paid tier relies on it: a case sets two variables and touches no harness
configuration ([check-runner.md](../check-runner.md)).

Not to be developed further ([work-queue.md](../work-queue.md)): an alias states the
choice explicitly, which is what a default was approximating.
