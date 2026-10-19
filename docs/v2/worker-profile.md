# The worker profile and `rewake claude` as main

A design, not built. The owner's decisions of October 19, 2026: rewake carries its own
Claude Code worker profile, so a worker does not carry everything of the person's own Claude
Code, and the person's wrapper made for that is deleted; the person starts everything with
`rewake claude` and Claude Code's own flags, which always makes main; workers are raised only
by main (`rewake space open`), later also from the shell's UI. Facts below were probed on
Claude Code 2.1.302 with `claude -p --no-session-persistence "/context"`, which makes no model
call, in a private HOME where the person's files would have answered instead.

## Why launch options and not a configuration directory

A separate `CLAUDE_CONFIG_DIR` — what the wrapper does — swaps the whole global layer, and with
it the login, the transcripts, `~/.claude.json` with its folder-trust decisions, and history. A
worker then logs in on its own, asks for trust in every new checkout with nobody at its screen,
and keeps conversations main's account cannot resume. It also does not do its job: Claude Code
reads `CLAUDE.md` and `.claude/CLAUDE.md` of every directory above the working one, so for any
checkout under the home directory the person's `~/.claude/CLAUDE.md` loads anyway, as a
*Project* memory (probed: with the user source off or another configuration directory, the file
still shows as `Project`). Launch options do all of it and keep the person's directory as is:

| What the profile does | Option | Probed |
| --- | --- | --- |
| drops the person's user layer: `settings.json` (hooks, status line, plugins, permissions, env, model, language), user skills, user MCP servers of `~/.claude.json`, the user `CLAUDE.md` | `--setting-sources project,local` | yes |
| drops the person's `CLAUDE.md` read as an ancestor's | `claudeMdExcludes: ["<config dir>/CLAUDE.md"]` in `--settings` | yes |
| carries the profile's settings; precedence is user < project < local < flag < policy | `--settings <run dir>/worker-settings.json` | yes |
| the profile's skills, as a plugin (skills appear as `<plugin>:<skill>`) | `--plugin-dir` of the profile's plugins folder | yes |
| the profile's MCP servers | `--mcp-config` | from help |
| the worker's instructions | folded into the one `--append-system-prompt` after rewake's briefing | from help |
| the auto-compact window | `--autocompact <auto\|100k–1M>` | yes |

What stays the person's: the login and account, transcripts and `/resume`, folder trust, the
project's and local settings, the project's `CLAUDE.md`/`AGENTS.md`, skills, agents and
`.mcp.json`, managed policy, and the claude.ai connectors (only `--strict-mcp-config` drops them,
and it drops the project's servers with them).

Two facts that shape the build. `--settings` is single-use — a second one replaces the first,
not merges (probed) — so the profile owns it: a worker's `--settings` or `--setting-sources`
after `--` is refused, naming the profile file. And an `env` value in user settings beats both
`--autocompact` and the process environment, while `env` in `--settings` beats user settings;
with the user source off, `--autocompact` holds unless the project's settings set the variable.

## The profile

**Every Claude Code worker gets it; main never.** It is the worker role's, not an alias's: a
`space open --harness claude` and an alias both get it; main is exactly the person's Claude Code.

**rewake's defaults**, with no file of the person's: the user layer off and the ancestor exclude
above; `autoMemoryEnabled: false`, since a worker is short-lived and the auto-memory of a project
is main's (sharing the directory with main otherwise mixes them); `env` with
`DISABLE_AUTOUPDATER=1` (a worker must not update Claude Code under a running room) and
`DISABLE_FEEDBACK_COMMAND=1` (no survey in an unattended pane); `Bash(rewake:*)` as today.

**The person's profile**, optional, in rewake's own directory beside pi's,
`~/.config/rewake/claude/worker/`, made private by the launch, never in the state directory:

- `settings.json` — merged over rewake's defaults (objects key by key, the person's value wins;
  `claudeMdExcludes` is appended to, never replaced), written as the run's one `--settings` file.
  Where the person's permissions allow-list, `env`, model per effort, `skillOverrides` go.
- `CLAUDE.md` — the worker's instructions, after rewake's briefing.
- `plugins/` — each child a plugin, loaded with `--plugin-dir`; the worker's skills live here.
- `mcp.json` — passed with `--mcp-config`.

rewake reads only its own directory and never `~/.claude`, `~/.claude.json` or a project's
files; it does not know where the person's instructions and skills are built from. Filling the
profile is the person's: the owner's installers get a target writing these paths.

**Context per worker.** The alias field `launch.context` becomes applied on Claude Code as
`--autocompact <value>`, validated as the flag's range, and stays shown-only on a harness with no
such flag. `launch.apply = false` goes with the wrapper it served.

## `rewake claude [claude flags]`

- **Always main.** `rewake [--room <room>] [--command <program>] [--no-intro] <harness> [flags]`
  is main on that harness; `--worktree` stays. **Removed** from the person's launch: `--main`,
  `--write`, `--general`, `--name` (main is `main-<harness>`, one per room) and `--shell`.
- **The shell by default** when standard input and output are both terminals and the adapter
  offers spaces; otherwise main runs plain (`claude -p`, a script, pi until it offers spaces), as
  an ordinary launch. This changes "the shell, chosen at launch" in `project.md`: the owner's
  decision, recorded there in the same step.
- **A second `rewake claude` in the same room** is refused, exit 1, before anything starts:
  `room <room> has main <name>; start another with rewake --room <other> claude`.
- **Aliases are only main's worker catalogue.** `rewake <alias>` as a launch is removed, and with
  it `main = true` and main's aliases. An alias keeps `harness`, `command`, `args`, `description`
  and `launch`; its `rewake` list becomes one field `role = "write" | "general"`, since nothing
  else in it means anything for a space. A file with the old fields is refused naming them.
  `rewake workers` and `space open --alias | --harness [--role]` stay; the user/project rule
  stays: a project alias names no role and no command.
- **How the space host still launches a worker.** It runs the same launch it runs today with
  `--name` and the role, plus `REWAKE_SPACE=<id>`; the launch takes a role or a name only when that
  variable names an `opening` space of the room with that name whose host is its parent process,
  and refuses them otherwise, pointing to `rewake space open`. The space record is not changed.
  The workflow suite, which starts workers directly in about forty places, keeps that form in the
  `rewaketest` build only.

## Steps

1. **The profile in the Claude Code adapter**, for every role but main, roles untouched.
   Live check: main opens `space open p --harness claude`; `/context` in the worker's pane shows no
   `~/.claude/CLAUDE.md` (neither User nor Project), the profile's skills and the window
   `launch.context` named; main's own `/context` is unchanged. Unit tests: the merge, the refusal
   of `--settings`/`--setting-sources` for a worker, the exclude path under `CLAUDE_CONFIG_DIR`.
2. **The catalogue**: `role`, `launch.context` applied, `apply`, `main` and the direct alias launch
   removed. Main rewrites the owner's `aliases.toml` with him and deletes the wrapper and its
   `command` lines with him, once step 1 is live; the wrapper's directory stays his.
3. **The launch surface**: main by default, the shell by default, the flags removed, the second
   main refused, `REWAKE_SPACE`; examples, the guide, briefings, `launch.md`, `launch-defaults.md`,
   `roles.md`, L1 of `design-rules.md` (a worker's profile is added beside the mail) and
   `project.md` in the same commit.
