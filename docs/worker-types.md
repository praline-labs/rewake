# Worker types

A type is the kind of work a worker is raised for — writing code, answering a question,
carrying out a procedure, reviewing a change — given as a short text that follows its role's
briefing. The role says what the worker may do and how its mail goes; the type says how to
work on this task. Built October 21, 2026 on main's decisions of that day
([the step](roadmap/2026-10-21-worker-types.md)).

## Where types come from

The nearest of one name wins:

1. the project's `.rewake/types/<type>.md`, in main's working directory;
2. the person's `~/.config/rewake/types/<type>.md`;
3. rewake's built-in `code`, `analysis`, `process` and `review`, in the source at
   `internal/core/worktype/builtin/<type>.md`, embedded in the build.

The built-in texts name no model, provider or harness, so they read the same to every
harness, and each defers to the project's own instructions. They say what to read first, how
to work, when the work is done and what the report holds; the owner edits them in the source.

A type file is Markdown opening with a TOML front matter between two `+++` lines:

```markdown
+++
description = "Answer a question about the code; change nothing."
+++
Read first: …
```

`description` is required and one line; another field is refused, as is an empty text, a name
other than letters, digits, `.`, `_` and `-`, or a file over 32 KiB. A file that does not read
refuses the catalogue, naming it, as an alias file does: a type silently absent would raise a
worker other than main asked for. `rewake workers` lists the types after the templates, each
with its source and description; `--json` carries them as `types`.

## Turning built-in types off

The owner's, October 21, 2026: `[types] off = ["process", "analysis"]` in
`~/.config/rewake/aliases.toml`, in the project's `.rewake/aliases.toml`, or in both. The two
lists are joined. A built-in type off in either is gone from `rewake workers`, and `space open
--type` refuses it, naming the file or files that turned it off. A type file of the same name,
the person's or the project's own text, still stands. A name in `off` that is no built-in type
refuses the catalogue, as an unknown field does: a typo would leave on what was meant off.

## Raising a worker for a type

`rewake space open <space> --alias <alias> --type <type>` raises the worker for that type;
an alias's `type = "<type>"` is its default, and `--no-type` raises one with none. A type
that is not there or is turned off is refused before anything starts; a template whose
default is, too, with the reason, unless `--type` or `--no-type` says otherwise. `rewake
workers` shows such a template's type with why it would be refused. The type's name shows on
the answer of `space open`.

`space open` resolves the type in main's directory and the space host passes its name and text
to the worker's launch as `REWAKE_SPACE_TYPE` and `REWAKE_SPACE_TYPE_TEXT` beside
`REWAKE_SPACE_TEMPLATE` ([launch.md](launch.md#a-space-for-a-worker)); the launch unsets them.
Nothing of the type is recorded in the space's record: a type changes the worker's briefing,
not the room's state. The briefing ends, after the role's sections, with
`YOUR KIND OF WORK: <type>`, a line saying the text changes how the worker works and not what
it may do — the role's rules stand and the project's own instructions win where they speak —
and the text. On Claude Code it travels in the one `--append-system-prompt`, on pi in the
briefing's own.

## Memory

Main's decision, October 21, 2026: a Claude Code worker raised for a type keeps the memory of
that type in that project, `~/.config/rewake/claude/worker/memory/<project>/<type>/`, whatever
template opened it; one with no type keeps its template's, as before
([v2/worker-profile.md](v2/worker-profile.md)). Nothing is moved. A template named like a type
shares that type's memory directory — a known limit, left until it bites.

## The project's `.rewake/`

A project's aliases moved from `.rewake.toml` to `.rewake/aliases.toml` the same day, beside
its types; the old file refuses the catalogue, naming the new place
([launch-defaults.md](launch-defaults.md#the-worker-catalogue)). A project template still may
not set `role` or `command`; it may set `type`.

## Later

`rewake list` showing a worker's type is not built.
