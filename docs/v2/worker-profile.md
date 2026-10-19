# The worker profile and `rewake claude` as main

A design; all three steps built October 19, 2026 (below). The owner's decisions of October 19, 2026: rewake carries its own
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
| carries the profile's settings; precedence is user < project < local < flag < policy | `--settings` of the service folder's `settings.json` | yes |
| the profile's skills, as a plugin (skills appear as `<plugin>:<skill>`) | `--plugin-dir` of the profile's plugins folder | yes |
| the profile's MCP servers | `--mcp-config` | from help |
| the worker's instructions | folded into the one `--append-system-prompt` after rewake's briefing | from help |
| the auto-compact window | `--autocompact <auto\|100k–1M>` | yes |

What stays the person's: the login and account, transcripts and `/resume`, folder trust, the
project's and local settings, the project's `CLAUDE.md`/`AGENTS.md`, skills, agents and
`.mcp.json`, and managed policy. The claude.ai connectors are dropped by the profile's
`disableClaudeAiConnectors: true` (the owner's decision below), which leaves the project's
servers; `--strict-mcp-config` would drop both.

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
`DISABLE_FEEDBACK_COMMAND=1` (no survey in an unattended pane); `Bash(rewake:*)` as today;
and the owner's off-list of October 19, 2026 — subagents (`Agent`), `ScheduleWakeup` and cron,
`SendMessage`, `ListAgents`, the worktree and plan-mode tools, `PushNotification`,
`RemoteTrigger`, `Artifact`, workflows, remote control, prompt suggestions, away summaries, idle
compaction, spinner tips, the feedback survey, IDE auto-connect and the claude.ai connectors —
listed setting by setting in [launch.md](../launch.md#claude-code). They are written into the
service folder's `settings.json`, which the person edits to turn one back on — `"Agent"` taken
out of `permissions.deny` — and which a newer build brings up to its defaults without undoing
the person's changes (below).

**The person's profile**, optional, in rewake's own directory beside pi's,
`~/.config/rewake/claude/worker/`, never in the state directory. A worker's launch makes what
of it is missing — the folder (0700), an empty `CLAUDE.md`, `skills/` and
`.claude-plugin/plugin.json` naming the plugin `worker` — and never changes what is there but
`settings.json` (the owner's decisions, October 19, 2026); the folder itself is passed as
`--plugin-dir`, so a skill placed or symlinked into `skills/` loads as `worker:<skill>`. An entry
of the wrong kind refuses the launch naming the path:

- `settings.json` — the whole of the worker's settings, passed as the run's one `--settings`
  as the file it is (the owner's decision, October 19, 2026, in place of a JSON string of the
  defaults merged with the person's file). rewake writes its defaults there on the first launch
  and marks the file `"rewake": {"defaults": <n>}`; a build with newer defaults runs only the
  steps after the file's version — adding what each brings where the file has no value,
  taking out what each retires where the file still holds rewake's value — so a default the
  person removed stays removed. The rule, the refusals and the write are in
  [launch.md](../launch.md#claude-code). Where the person's permissions allow-list, `env`,
  model per effort, `skillOverrides` go, beside rewake's defaults.
- `CLAUDE.md` — the worker's instructions, after rewake's briefing.
- `skills/` — the worker's own skills, each folder or symlink one skill of the plugin `worker`.
- `plugins/` — each child a whole plugin, loaded with `--plugin-dir`.
- `mcp.json` — passed with `--mcp-config`.

rewake reads and makes only its own directory and never `~/.claude`, `~/.claude.json` or a project's
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
  19 characters (`dir` when nothing is left), the name then fitting rewake's 32, then the first 12 hex digits of the SHA-256 of the resolved path, so two
  checkouts named alike stay apart and the name stays readable. `rewake list` shows the room as
  that resolved path, kept in the room's directory by the bootstrap that makes it — a new file
  of the state format, so step 3 takes Codex's acceptance. A session finds its room in
  `REWAKE_ROOM` as today; a command outside a session takes the room of its working directory.
  Workers main raises belong to main's room whatever their checkout: the space host passes main's
  room, and a worktree elsewhere changes nothing.
- **A second `rewake claude` in the same directory** is refused, exit 1, before anything
  starts: `<dir> already has main <name>: one main per directory. Use that one, or start rewake
  in another directory, which is another room.` One
  in another directory is another room.
- **Aliases are only main's worker catalogue.** `rewake <alias>` as a launch is removed, and with
  it `main = true` and main's aliases. An alias keeps `harness`, `command`, `args`, `description`
  and `launch`; its `rewake` list becomes one field `role = "write" | "general"`, since nothing
  else in it means anything for a space. A file with the old fields is refused naming them.
  `rewake workers` and `space open --alias | --harness [--role]` stay; the user/project rule
  stays: a project alias names no role and no command.
- **How the space host still launches a worker.** It runs `rewake [--command <wrapper>]
  <harness> <args>` with `REWAKE_SPACE=<room>/<space id>` and no name, room or role flag; the
  launch is that worker only when the variable names a space that is `opening`, or `open` on the
  launch's own run, whose host is its parent process — taking the name, role and room from the
  record — and refuses otherwise, pointing to `rewake space open`. The space record is not
  changed.
  The workflow suite, which starts workers directly in about forty places, keeps that form in the
  `rewaketest` build only.

## Decisions

Main's, October 19, 2026, on the design's questions; the room's identity is the owner's (above).

1. The shell by default whenever standard input and output are a terminal; no `--no-shell`.
2. A second `rewake claude` in the same directory is refused, exit 1.
3. The workflow suite's direct worker launches stay, in the `rewaketest` build only.
4. A worker's auto-memory is off by default.
5. The claude.ai connectors stay for workers: no `--strict-mcp-config` by default. Superseded the
   same day by the owner: they are off for workers, through `disableClaudeAiConnectors`, and the
   project's `.mcp.json` servers stay.
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
3. **The launch surface** — built ([its entry](../roadmap/2026-10-19-worker-profile-step3.md)):
   main always, the shell by default, the flags removed for the person, the second main refused,
   `REWAKE_SPACE`, the room by directory, the service folder made, the off-list. Checked live
   through a built rewake in a private `REWAKE_DIR`, `HOME` unchanged for the login, haiku:

   | check | seen |
   | --- | --- |
   | `rewake claude --model haiku` at a terminal | main-claude in the shell, room `proj-c219f3571dbf`, "rewake <harness> always starts main, one per directory" |
   | a second `rewake claude` there | exit 1, "/tmp/rewake-live/proj already has main main-claude: one main per directory. …" |
   | `rewake claude -p` in another directory | room `other-d46b52a25619`, its `directory` file naming it |
   | main runs `space open api --harness claude --role write` | api-claude in main's room, "raised by main with rewake space open as write"; the service folder made, 0700, the person's `skills/pskill` symlink kept |
   | main's task to the worker | woken by mail with `SendMessage` and `ListAgents` denied, report `finished` at its turn end |
   | the worker's tools | no `Agent`, `SendMessage`, `ListAgents`, `ScheduleWakeup`, worktree, plan-mode, push, trigger, `Artifact` or workflow tool; main has `Agent`, `ScheduleWakeup` and the connectors |
   | the worker's `/context` and `/skills` | MCP tools: 1, the project's `.mcp.json` server `probe`, no claude.ai connector; `worker:pskill`, plugin `worker` |
   | the terminal closed | the shell, main, the worker and its space host ended; the space closed "main's run ended" |

   `--alias` was not run: the owner's `aliases.toml` is still in the old form, which step 2
   refuses, and is rewritten with main at landing.
4. **The settings as a file** — built ([its entry](../roadmap/2026-10-19-worker-settings-file.md)):
   the folder's `settings.json` passed as it is, rewake's defaults written and migrated by
   version, `rewake.keep` and the merge gone. `claudeMdExcludes` lives in the file: the one
   path that needs it is `~/.claude/CLAUDE.md` of the home the folder lies in, and a
   `CLAUDE_CONFIG_DIR` elsewhere is not read with the user source off (probed on 2.1.302), so
   no launch-time addition is left. Checked live with haiku, a private HOME and `REWAKE_DIR`:

   | check | seen |
   | --- | --- |
   | a worker over a copy of the owner's `{}` | the file rewritten with every default at version 1, the launch noting it |
   | its tools and `/context` | no `Agent`, `ScheduleWakeup`, `SendMessage`, `ListAgents`, worktree, plan-mode, push, trigger or `Artifact`; System tools 2.7k, main's 6.7k |
   | `"Agent"` taken out of the file, the worker started again | `Agent` among its tools, System tools 4.5k; the file byte for byte the same |
   | Claude Code at a terminal with the file | no warning of the `rewake` key |
