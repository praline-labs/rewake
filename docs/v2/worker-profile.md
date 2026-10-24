# The worker profile and `rewake claude` as main

A design; its steps built October 19, 2026, and the configuration directory of its own
October 23 (below). The owner's decisions of October 19, 2026: rewake carries its own
Claude Code worker profile, so a worker does not carry everything of the person's own Claude
Code, and the person's wrapper made for that is deleted; the person starts everything with
`rewake claude` and Claude Code's own flags, which always makes main; workers are raised only
by main (`rewake space open`), later also from the shell's UI. Facts below were probed on
Claude Code 2.1.302 with `claude -p --no-session-persistence "/context"`, which makes no model
call, in a private HOME where the person's files would have answered instead.

## A configuration directory of its own

The owner's decision, October 21–23, 2026: a worker's conversations sat beside the person's,
so `claude --resume`, `--continue` and `/resume` offered them as the person's own. Since
October 23 the service folder `~/.config/rewake/claude/worker/` is the workers' Claude Code
configuration directory, `CLAUDE_CONFIG_DIR`, which every worker's launch sets and the person
logs in once. What the design of October 19 weighed against it is answered: the login is the
person's once, in the first worker's own pane with `/login` (the owner's decision, October 23:
`space open` opens the worker and its answer tells main the person is to log in there, rather
than refusing and naming a command typed elsewhere); onboarding and the trust of
the repository main runs in are marked by rewake in the folder's `.claude.json` before each
launch (main's decision on the owner's behalf, October 23: the person's own sessions there
already have that trust); separate history is the point. The account is the same, logged in a
second time: the same plan and usage. Probed on 2.1.302, October 23, in private HOMEs with no
login ([research-launch.md](../research-launch.md#a-configuration-directory-of-the-workers)):

| What Claude Code reads from the folder | as |
| --- | --- |
| `settings.json` | the user's settings: its `env` reaches the process, its hooks run |
| `CLAUDE.md` | the user's memory file (`/context`: *User*) |
| `skills/<skill>/` | user skills, named as their folders |
| `agents/`, commands, plugins installed there | the user's own |
| `.claude.json` | onboarding, folder trust, user MCP servers, counters |
| `projects/`, `sessions/`, history, `.credentials.json` | the workers' transcripts and login |

What the launch still adds: `--settings` with what this run alone gets, a JSON object of the
worker's memory, `EnterWorktree` and, for a run with a mod, rewake's `statusLine` (the cache's
ttl: [design-claude.md](design-claude.md#the-cache-ttl); the person's own `statusLine` in the folder's
settings is run by it and printed) — allowed for a template that enters, denied for any
other; `mcp.json` as `--mcp-config`; rewake's briefing in `--append-system-prompt`; the mod.
Gone: `--setting-sources project,local`, the whole file as `--settings`, the folder as the plugin
`worker` and its `CLAUDE.md` folded into the system prompt. What stays the person's: the
project's and local settings, the project's `CLAUDE.md`/`AGENTS.md`, skills, agents and
`.mcp.json`, and managed policy; the person's own `~/.claude/CLAUDE.md`, read as an ancestor's
under another configuration directory too, stays excluded by `claudeMdExcludes`. rewake's
defaults moved from the flag layer to the user layer, so a project's settings can override one
of them that is a single value; deny lists merge, so the off-list's denies hold. The mod's
`$.store` lies in the folder's `plugins/store/` now; its keys are per run.

Until October 23 a worker kept the person's configuration directory and launch options laid
the profile over it, for the reasons the owner then set aside: a separate directory's own
login, trust and history. Two facts of that build still hold.

`--settings` is single-use — a second one replaces the first,
not merges (probed) — so the profile owns it: a worker's `--settings` or `--setting-sources`
after `--` is refused, naming the profile file; one leaving out the user layer would now drop
rewake's defaults with it. And an `env` value in user settings beats both `--autocompact` and
the process environment, while `env` in `--settings` beats user settings: since October 23 the
folder's `settings.json` is the user layer, so a window variable set in its `env` beats a
template's `launch.context`.

## The profile

**Every Claude Code worker gets it; main never.** It is the worker role's, not an alias's: a
worker raised from any alias gets it; main is exactly the person's Claude Code.

**rewake's defaults**, with no file of the person's: the folder as the user layer and the
ancestor exclude above; the auto-memory, off until October 19, 2026 and since then one per template and project,
since October 21 one per type and project for a worker raised for a type (step 5); `env` with
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
of it is missing — the folder (0700), an empty `CLAUDE.md` and `skills/` — and never changes
what is there but `settings.json` (the owner's decisions, October 19, 2026) and two keys of
Claude Code's `.claude.json` (October 23). Claude Code reads the folder as the user's own since
October 23; until then it was passed as the plugin `worker`, whose `.claude-plugin/` a folder
made then still holds, unread. An entry of the wrong kind refuses the launch naming the path:

- `settings.json` — Claude Code's user settings of the workers, which it reads itself (October
  23, 2026; passed as the run's one `--settings` from October 19). rewake writes its defaults
  there on the first launch and marks the file `"rewake": {"defaults": <n>}`; a build with
  newer defaults runs only the steps after the file's version — adding what each brings where
  the file has no value, taking out what each retires where the file still holds rewake's
  value — so a default the person removed stays removed. The rule, the refusals and the write
  are in [launch.md](../launch.md#claude-code). Where the person's permissions allow-list, `env`,
  model per effort, `skillOverrides`, a status line go, beside rewake's defaults.
- `CLAUDE.md` — the workers' instructions, Claude Code's user memory file.
- `skills/` — the workers' own skills, each folder or symlink one skill, named as the folder
  (`mods`; `worker:mods` until October 23).
- `agents/`, and plugins installed with `CLAUDE_CONFIG_DIR` set to the folder (`claude plugin
  install`, or `/plugin` in a worker's pane) — Claude Code's own. `plugins/` is Claude Code's
  directory there; until October 23 each child of it was a plugin rewake passed.
- `mcp.json` — passed with `--mcp-config`; user servers added with `claude mcp add -s user`
  under the variable work too.
- `memory/` — rewake's: each worker's auto-memory directory (step 5).

rewake reads and makes only its own directory and never `~/.claude`, `~/.claude.json` or a project's
files — the folder's own `.claude.json` is the workers', not the person's; it does not know where the person's instructions and skills are built from. Filling the
profile is the person's: the owner's installers get a target writing these paths.

**Context per worker.** The alias field `launch.context` is applied on Claude Code as
`--autocompact <value>` for the workers main raises with it — aliases are main's worker
catalogue only, so main has none — checked by rewake as `auto` or 100k–1M
written `300k`, `1M` or `200000` — Claude Code itself reads the leading number of anything —
and stays shown-only on a harness with no such flag (`AppliesContext`). `launch.apply = false`
went with the wrapper it served (step 2).

**Under `--command`** the profile is not applied, with a launch note: the wrapper decides the
configuration Claude Code starts with, as it did before, its `CLAUDE_CONFIG_DIR` among it: rewake
sets none, and `space open` asks after no login. A wrapper that moves `CLAUDE_CONFIG_DIR` keeps its own
settings, whose `env` then beats `--autocompact`.

## `rewake claude [claude flags]`

- **Always main.** `rewake [--command <program>] [--no-intro] <harness> [flags]` is main on that
  harness, pi's too; `--worktree` stays. **Removed** from the person's launch: `--main`,
  `--write`, `--general`, `--name` (main is `main-<harness>`, one per room), `--shell` and
  `--room`.
- **The shell by default** whenever standard input and output are both terminals and the
  adapter offers spaces, with no flag to turn it off; otherwise main runs plain — a script or a
  pipe. `-p` does not decide it: what
  counts is whether both streams are terminals. This changes "the shell,
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
  `rewake workers` and `space open --alias` stay — the only way to raise a worker since
  `--harness` and `--role` went (decision 8); the user/project rule stays: a project alias names
  no role and no command. Since October 20, 2026 an alias may say `worktree = true`
  ([worktree-tools.md](../worktree-tools.md#a-templates-worktree--true)), or, on Claude Code,
  `enter_worktree = true`, which takes `EnterWorktree` off the off-list for that template's runs
  alone ([worktree-tools.md](../worktree-tools.md#a-workers-entry-enter_worktree--true-and-worktree_enter)).
- **How the space host still launches a worker.** It runs `rewake [--command <wrapper>]
  <harness> <args>` with `REWAKE_SPACE=<room>/<space id>` and `REWAKE_SPACE_TEMPLATE=<alias>`,
  the alias that names the worker's memory, and no name, room or role flag; the
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
4. A worker's auto-memory is off by default. Superseded the same day by the owner: Claude
   Code's own auto-memory, one per worker template and project (step 5).
5. The claude.ai connectors stay for workers: no `--strict-mcp-config` by default. Superseded the
   same day by the owner: they are off for workers, through `disableClaudeAiConnectors`, and the
   project's `.mcp.json` servers stay.
6. `rewake pi` is main too, in the shell at a terminal and raising workers, as `rewake claude` is
   (main's decision, October 24, 2026, under the owner's rule that every feature is weighed for
   both harnesses).
7. One profile for every Claude Code worker; named profiles only when needed.
8. The owner's, October 19, 2026: a worker is raised only from a template. `space open` requires
   `--alias`; `--harness` and `--role` are gone, the role is the alias's, and the `default`
   memory of a worker opened without an alias with them
   ([the entry](../roadmap/2026-10-19-worker-from-template.md)).

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
5. **The worker's memory** — built ([its entry](../roadmap/2026-10-19-worker-memory.md)):
   `autoMemoryEnabled: false` retired in defaults version 2; each launch lays
   `autoMemoryEnabled: true` and `autoMemoryDirectory` over the folder's settings in a copy in
   the run's directory, the directory
   `~/.config/rewake/claude/worker/memory/<project>/<template>/` ([launch.md](../launch.md#claude-code));
   the alias reaches the launch as `REWAKE_SPACE_TEMPLATE` from the space host, no new record
   field. Since October 21, 2026 (main's decision) a worker raised for a type keeps the type's
   memory, `…/memory/<project>/<type>/`, whatever template opened it; one with no type keeps
   its template's, and nothing is moved ([worker-types.md](../worker-types.md)). Checked live with haiku, a private HOME and `REWAKE_DIR`, a `-p` main in a probe
   repository opening two spaces with `--worktree`:

   | check | seen |
   | --- | --- |
   | a version-1 file with `autoMemoryEnabled: false` and only `Agent` denied | version 2, the key gone, the deny list as the person left it |
   | `space open wa --alias haiku-a --worktree`, `wb --alias haiku-b --worktree` | two checkouts; each worker's `--settings` a run-directory copy, `autoMemoryDirectory` `…/memory/repo-d9ecc4f0fee9/haiku-a` and `…/haiku-b`, both made 0700 |
   | `/context` with each worker's settings in its checkout | `AutoMem …/haiku-a/MEMORY.md` and `…/haiku-b/MEMORY.md`, each seeded with one line |
   | a haiku call asking each for the code word in its memory | `HERON` for template A, `OSPREY` for B |
6. **A configuration directory of its own** — built
   ([its entry](../roadmap/2026-10-23-worker-config-dir.md)): the folder as `CLAUDE_CONFIG_DIR`,
   its files read natively, the run's `--settings` a JSON object of memory and `EnterWorktree`
   (no run-directory copy), defaults version 3 taking `EnterWorktree` out of the file,
   onboarding and trust in the folder's `.claude.json`, the login notice of `space open`; pi's
   workers keep their sessions in `worker-sessions/` of its agent directory. Probed without a
   login ([research-launch.md](../research-launch.md#a-configuration-directory-of-the-workers));
   the live check with a worker waits for the owner's login.
