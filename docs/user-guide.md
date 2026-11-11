---
description: The person's guide — the few things only the person does: install, logins, starting main, the worker templates, a project's .rewake/, the shell's keys, stopping and restarting, where things live, upgrading.
---

# The person's guide

Main does most of the setting up: it reads the templates, raises the workers, hands out the
work and answers the workers' requests. What is left to you is below. Every command named here
prints its own help with `--help`; `rewake guide` is the map of all of them.

## Install

```bash
npm install -g @praline-labs/rewake
rewake --version
```

Linux on x64 or arm64. The package carries rewake, its shell and the one pi rewake runs, at the
version rewake was checked with; pi runs on Node.js 22.19 or newer, and a pi of your own on
`PATH` is not used. Claude Code is yours to install, 2.1.287 or newer: a launch reads its version
first and refuses an older one.

## Log in

- **Main on Claude Code** is your own Claude Code, with your own login and settings.
- **Existing Claude Code workers** keep their login and configuration in
  `~/.config/rewake/claude/worker/`. New workers are pi only; nothing ends or migrates an
  existing session.
- **pi**, main and workers alike, keeps its login, settings, extensions and sessions in
  `~/.config/rewake/pi/agent`, apart from your own `~/.pi`. Log in there once with `/login` in
  the first pi session.

## Start main

From your own terminal, in the project's directory:

```bash
rewake claude    # main-claude
rewake pi        # or main-pi
```

That launch is always main, and the directory is its room: one main per directory, and another
directory is another room with sessions of its own. The harness's own flags go after the word,
as with the plain command: `rewake claude --effort high`, `rewake claude --continue`.
`rewake claude --worktree=<name>` starts main in a new checkout of its own.

A launch from a shell inside a rewake session is refused: start main from a terminal of yours,
not from one an agent runs. You never start a worker yourself; ask main for one.

## Worker templates

Main raises each worker from a template, which names its harness, role and launch settings;
`rewake workers` lists them, and rewake ships none. Ask main to write them for you, or put this
in `~/.config/rewake/aliases.toml` (mode `0600`) and change it to taste:

```toml
[alias.write]
harness = "pi"
role = "write"
description = "Writes and commits a change in a checkout of its own"
worktree = true
args = []
[alias.write.pi]
start_mode = "build"

[alias.write.launch]
effort = "high"

[alias.review]
harness = "pi"
description = "Reviews a change and reports; edits nothing"
[alias.review.pi]
start_mode = "review"

[alias.helper]
harness = "pi"
description = "Answers a question about the code"
[alias.helper.pi]
start_mode = "advisor"
```

- `harness` is `pi` for new workers; Claude Code still launches main. Old Claude Code aliases
  stay in your files, but are omitted from `workers` and refused by name on `space open`,
  even with a `command` wrapper. `role` is `write`, a worker that may commit within its own
  permissions, or `general`, the default.
- `args` reach the harness as written; `[alias.<name>.launch]` takes `model`, `effort`,
  `context` (Claude Code's auto-compact window, such as `300k`) and `label`. A template with no
  model or effort takes the launch defaults (below), else the harness's own.
- `[alias.<name>.pi] start_mode` is the runtime work mode the worker starts in: rewake's built-in
  `advisor`, `build`, `process` and `review`, or one of your own (below). Without one the worker
  takes `[pi] start_mode` from either alias file, else `advisor`.
- `worktree = true` starts the worker in a new checkout of its own, which main lands with
  `rewake worktree land` and the close removes once nothing in it would be lost.

`rewake workers --help` lists every field. Main runs `rewake workers` and
`rewake space open <name> --alias <template>`, and the worker `<name>-<harness>` joins its room.

**Launch defaults.** A default model and effort for every launch that names none, the workers'
included, are `KEY=VALUE` lines in `~/.config/rewake/settings`, or in `.rewake.env` in a
project's directory: `REWAKE_CLAUDE_MODEL`, `REWAKE_CLAUDE_EFFORT`, `REWAKE_CLAUDE_CONTEXT`,
`REWAKE_PI_MODEL`, `REWAKE_PI_EFFORT`. The environment beats both files, and a typed flag beats
everything. A Claude Code main keeps the model's own window unless
`REWAKE_CLAUDE_CONTEXT` or a typed `--autocompact` names one; `auto` keeps the model's own.

## A project's `.rewake/`

A project can carry its own catalogue beside its code, read from the directory main runs in:

- **`.rewake/aliases.toml`** — the project's templates, in the same form. A project template
  with your template's name replaces yours. It may not set `role` or `command`: those stay in
  your own file.
- **`[briefs] dir = ".shift/briefs"`**, in that file, names where main's new briefs go, relative to
  main's directory: each in `<dir>/brief-<id>/`. Briefs in the room's state are lost when the
  temporary directory is cleared; a folder of the project is not. `rewake brief new --dir <path>`
  puts one brief elsewhere, in a folder that exists. Your own file may not set it
  ([briefs.md](briefs.md#where-a-brief-lives)).
- **`[skills] never = ["keychain"]`**, in either file, bars those of your skills from briefs. Main
  gives a worker skills with a brief by a line of its text, `Skills: mods, mp-tdd`: rewake copies
  each from where main's harness reads your skills into the brief, the worker reads the copies,
  and closing the brief takes them away; a skill one of them links, `../<name>/…`, comes along. This
  is the only way a worker gets your skills. A barred or missing skill the line names refuses the
  brief; a barred one a skill links is left out, and the letter says so
  ([briefs.md](briefs.md#skills)).
- **`[aliases] off = ["helper"]`** in that file hides those of your templates in this project
  alone. A name your catalogue no longer has is already off and is ignored, so a project that
  still lists a template you deleted later launches as before; the notice asks you to drop the
  name. With no launchable `write` template left, main still delegates unless the project says
  otherwise with the next setting; `rewake workers` states both facts separately.
- **`[main] implements = true`**, in that file only, lets main implement product changes itself
  in this project, subject to the task, its work mode and the project's rules. Missing or false
  is the default and keeps main delegating; there is no user-global form, because whether main
  writes code is one project's workflow. The value is read when main starts and frozen for that
  run, so editing the file changes the next main, not the running one
  ([roles.md](roles.md#main-implementation)).
- **`.rewake/modes/<mode>.md`** — a runtime work mode of the project's own, which also replaces a
  built-in's text of the same name; yours go in `~/.config/rewake/modes/`. A mode file opens with
  a TOML front matter holding one `description` line:

  ```markdown
  +++
  description = "Edit the project's documentation; leave the code alone."
  +++
  Read the project's documentation map first. Change only the documents the brief names.
  ```

**Templates stay on your machine; the project shares the rest.** A template is a whole recipe —
harness, role, command, launch settings, and the model and effort its modes select — so the
project carries its modes, its `[aliases] off`, its `[main] implements` and its brief folder, and
its documentation names the local templates it expects. Your own file holds the complete
templates, with role and command, and model IDs only from your machine's own pi registry; a
project definition of the same name replaces yours whole, so a shared recipe with a machine-local
model would need an overlay that rewake does not have. [launch-defaults.md](launch-defaults.md#keeping-templates-on-the-machine)
has the two examples. Nothing is migrated or rewritten for you.

Older settings of the separate worker type do not block a launch: a `type = "..."` in a template,
a `[types]` section and a `types/` directory are skipped, and every command that reads the
catalogue prints a notice naming the file, the line and what to write instead. A template whose
`type` is skipped stays out of `rewake workers` and cannot be opened until it is fixed; the rest
of the catalogue runs, and main reads the notices in its briefing and can repair the file
([work-modes.md](work-modes.md#the-old-worker-types)). Only the old shape is skipped: a `type`
that is not a string, or a `[types]` section that is not `off = ["...", ...]`, refuses the file,
and so does a broken setting beside an old `type` — the template is checked before it is left out.
Your own file and the project's still replace one another whole, so only the definition in force
is reported. A file rewritten by hand is the only way: malformed TOML, an unknown field and a
malformed value still refuse the catalogue.

## The workers' own settings

An existing Claude Code worker reads `~/.config/rewake/claude/worker/` as its user configuration:
`CLAUDE.md` for instructions every worker gets, and `settings.json`. Skills do not go there: a
worker gets them with a brief, from your own skill folders (`[skills] never` above). rewake writes its defaults into `settings.json` on the first launch —
subagents, scheduling and other features an unattended worker should not use, turned off — and
you edit the file to change them: taking `"Agent"` out of `permissions.deny` gives the workers
their subagents back. A newer rewake adds only its newer defaults and keeps your changes. Your
project's own settings, `CLAUDE.md` and MCP servers reach the workers as they reach you.

## The shell

On a terminal, main runs in rewake's shell. From 120 columns wide, main is on the left and the
chosen worker on the right; narrower, one of them takes the window. The bar at the bottom names
main and each worker, the bottom line says where your keys go, and each worker's pane ends with
a line of its activity, model and context fill. A click on a pane or a button focuses it.

The prefix is **Ctrl+\\**; within two seconds, press:

| Key | Does |
| --- | --- |
| `1`–`9` | choose and focus that worker |
| `0` or `m` | focus main |
| `n` / `p` | the next or previous tab |
| ← / → | move the focus to main or to the chosen worker |
| `z` | zoom the focused pane to the whole window, or end the zoom |
| Ctrl+\\ | send Ctrl+\\ itself to the focused program |

Any other key cancels. Every other key goes to the focused program, so typing in a worker's
pane is typing to that worker: you may answer its dialog there before main does.

## Stop and restart

- **Stop**: end main as its harness ends — `/exit` in Claude Code, `/quit` in pi — or close the
  terminal. Main's end ends every worker; nothing runs on unseen. Main closes one worker with
  `rewake space close <name>`.
- **Restart**: run `rewake claude` again in the same directory, with the harness's own
  `--continue` or `--resume` to go on with main's conversation. Workers are not brought back:
  main raises new ones from the templates. `rewake space list` shows the spaces that closed,
  with their session ids, and a checkout a worker left with unlanded work stays, listed by
  `rewake worktree ls`.
- **Who is up**: `rewake list` in the project's directory lists the room's sessions.

## Where things live

| Where | What |
| --- | --- |
| `~/.config/rewake/aliases.toml` | your worker templates |
| `~/.config/rewake/settings` | your launch defaults |
| `~/.config/rewake/modes/` | your own runtime work modes |
| `~/.config/rewake/claude/worker/` | the Claude Code workers' configuration: login, conversations, `settings.json`, `CLAUDE.md`, their memory |
| `~/.config/rewake/pi/agent/` | rewake's pi: login, settings, extensions, sessions |
| `.rewake/` and `.rewake.env` in a project | the project's templates, modes and launch defaults |
| `$REWAKE_DIR/v2/`, by default `rewake-<uid>/v2/` in the system's temporary directory | the rooms: sessions, mail, spaces |
| `$REWAKE_WORKTREES`, else `$XDG_DATA_HOME/rewake/worktrees/`, else `~/.local/share/rewake/worktrees/` | the checkouts made for `--worktree` |

rewake never edits your own harness configuration: what a session needs is passed as options of
that one launch.

## Upgrading

**rewake itself**: `npm install -g @praline-labs/rewake` again. Sessions already running keep the
build they started with, and every session of a room is one build: end the room's main, and with
it its workers, before starting main with the new build there; a launch into a room another
build holds is refused, saying so.

**From 1.x**: end every 1.x session first. rewake 2.0 reads none of 1.x's state and carries none
of its mail over; the old tree under the state directory can be removed by hand. What a person
changes:

- Start only main, `rewake claude` or `rewake pi` in the project's directory; the launch flags
  `--main`, `--write`, `--general`, `--name` and `--room` are refused, since the directory is the
  room and main raises the workers.
- Rewrite the templates file: a template is `[alias.<name>]` with `harness` and `role`, not a
  `rewake` list or `main = true`, and a file of the old form is refused, naming each field. A
  project's templates live in `.rewake/aliases.toml`; a `.rewake.toml` is refused, naming that
  place.
- The harnesses are Claude Code and pi.

More on installing, building from source and running a build beside an installed one:
[install.md](install.md).
