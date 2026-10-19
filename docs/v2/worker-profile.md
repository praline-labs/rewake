# The worker profile and `rewake claude` as main

A design; step 1 built October 19, 2026 (below). The owner's decisions of October 19, 2026: rewake carries its own
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
`~/.config/rewake/claude/worker/`, never in the state directory; rewake reads it and never
writes or makes it, and an entry of the wrong kind refuses the launch naming the path:

- `settings.json` — merged over rewake's defaults (objects key by key, the person's value wins;
  `claudeMdExcludes` is appended to, never replaced), passed as the run's one `--settings`, a
  JSON string, so nothing is written.
  Where the person's permissions allow-list, `env`, model per effort, `skillOverrides` go.
- `CLAUDE.md` — the worker's instructions, after rewake's briefing.
- `plugins/` — each child a plugin, loaded with `--plugin-dir`; the worker's skills live here.
- `mcp.json` — passed with `--mcp-config`.

rewake reads only its own directory and never `~/.claude`, `~/.claude.json` or a project's
files; it does not know where the person's instructions and skills are built from. Filling the
profile is the person's: the owner's installers get a target writing these paths.

**Context per worker.** The alias field `launch.context` is applied on Claude Code as
`--autocompact <value>`, for main and workers alike, checked by rewake as `auto` or 100k–1M
written `300k`, `1M` or `200000` — Claude Code itself reads the leading number of anything —
and stays shown-only on a harness with no such flag (`AppliesContext`). `launch.apply = false`
went with the wrapper it served (step 2).

**Under `--command`** the profile is not applied, with a launch note: the wrapper decides the
configuration Claude Code starts with, as it did before. A wrapper that moves
`CLAUDE_CONFIG_DIR` keeps its own settings, whose `env` then beats `--autocompact`.

## `rewake claude [claude flags]`

- **Always main.** `rewake [--command <program>] [--no-intro] <harness> [flags]` is main on that
  harness, pi's too; `--worktree` stays. **Removed** from the person's launch: `--main`,
  `--write`, `--general`, `--name` (main is `main-<harness>`, one per room), `--shell` and
  `--room`.
- **The shell by default** whenever standard input and output are both terminals and the
  adapter offers spaces, with no flag to turn it off; otherwise main runs plain (`claude -p`, a
  script, and pi, which without spaces has no shell and no workers). This changes "the shell,
  chosen at launch" in `project.md`: the owner's decision, recorded there in step 3.
- **The room is the launch directory** (the owner's decision, October 19, 2026). Its identity is
  the working directory resolved with symlinks followed; the person never names a room. On disk
  it is `v2/rooms/<base>-<hash>`: the directory's last element reduced to `[a-z0-9-]` and cut to
  24 characters, then the first 12 hex digits of the SHA-256 of the resolved path, so two
  checkouts named alike stay apart and the name stays readable. `rewake list` shows the room as
  that resolved path, kept in the room's directory by the bootstrap that makes it — a new file
  of the state format, so step 3 takes Codex's acceptance. A session finds its room in
  `REWAKE_ROOM` as today; a command outside a session takes the room of its working directory.
  Workers main raises belong to main's room whatever their checkout: the space host passes main's
  room, and a worktree elsewhere changes nothing.
- **A second `rewake claude` in the same directory** is refused, exit 1, before anything
  starts: `<dir> has main <name>; one main per directory: start it in another directory`. One
  in another directory is another room.
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

## Decisions

Main's, October 19, 2026, on the design's questions; the room's identity is the owner's (above).

1. The shell by default whenever standard input and output are a terminal; no `--no-shell`.
2. A second `rewake claude` in the same directory is refused, exit 1.
3. The workflow suite's direct worker launches stay, in the `rewaketest` build only.
4. A worker's auto-memory is off by default.
5. The claude.ai connectors stay for workers: no `--strict-mcp-config` by default.
6. `rewake pi` is main too; without spaces it runs without the shell and without workers.
7. One profile for every Claude Code worker; named profiles only when needed.

## Steps

1. **The profile in the Claude Code adapter** — built (`internal/adapter/claude/profile.go`),
   roles untouched. Checked live through a built rewake, `-p --no-session-persistence
   "/context"` with haiku, in a private HOME and `REWAKE_DIR`, the profile there holding a
   plugin with one skill, `CLAUDE.md` and `settings.json`, the alias with `context = "250k"`:

   | run | memory | skills | window |
   | --- | --- | --- | --- |
   | worker | only the project's `CLAUDE.md` | `wtools:wskill` (Plugin) | 250k |
   | worker, `CLAUDE_CONFIG_DIR` another directory | only the project's | `wtools:wskill` | 250k |
   | main | User `~/.claude/CLAUDE.md` and the project's | the user skill | 300k, its user `env` |
   | worker under `--command` moving `CLAUDE_CONFIG_DIR` | that directory's, `~/.claude`'s as Project | that directory's | its `env`, over `--autocompact` |

   A haiku call quoted the profile's `CLAUDE.md` from the system prompt. Not seen live: a
   worker through `space open` in a pane; it is the same launch.
2. **The catalogue** — built ([its entry](../roadmap/2026-10-19-worker-catalogue.md)): `role`,
   `launch.context` applied, `apply`, `main` and the direct alias launch removed; an old file is
   refused naming each field, its line and what replaced it. Main rewrites the owner's `aliases.toml` with him and deletes the wrapper and its
   `command` lines with him, once step 1 is live; the wrapper's directory stays his.
3. **The launch surface**: main by default, the shell by default, the flags removed, the second
   main refused, `REWAKE_SPACE`; examples, the guide, briefings, `launch.md`, `launch-defaults.md`,
   `roles.md` and `project.md` in the same commit; the room by directory with its acceptance.
