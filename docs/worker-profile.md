---
description: The Claude Code workers' profile: the service folder, login, onboarding and trust, rewake's defaults in settings.json and their upgrades, the run's own settings, memory.
---

# The Claude Code workers' profile

Every Claude Code launch but main's sets `CLAUDE_CONFIG_DIR` to rewake's service folder and adds
one `--settings` with what the run alone gets. The profile is the worker role's, not an alias's:
a worker raised from any alias gets it, and main is exactly the person's Claude Code. Under
`--command` it is not applied, with a launch note: the wrapper decides the configuration Claude
Code starts with, its `CLAUDE_CONFIG_DIR` among it, rewake sets none and `space open` asks after
no login. The rest of a Claude Code launch is in [launch.md](launch.md#claude-code); the probes
behind the facts here are in
[research-launch.md](research-launch.md#a-configuration-directory-of-the-workers).

## The service folder

`~/.config/rewake/claude/worker/`, is the workers' Claude Code
configuration directory, so a worker's conversations stay out of the person's `--resume`
and `/resume`. What Claude Code reads from it (2.1.302):

| What | as |
| --- | --- |
| `settings.json` | the user's settings: its `env` reaches the process, its hooks run |
| `CLAUDE.md` | the user's memory file |
| `skills/<skill>/` | user skills, named as their folders |
| `agents/`, commands, plugins installed there | the user's own |
| `.claude.json` | onboarding, folder trust, user MCP servers, counters |
| `projects/`, `sessions/`, history, `.credentials.json` | the workers' transcripts and login |

What of it is missing a worker's launch makes — the folder itself (0700), an empty
`CLAUDE.md` and an empty `skills/`. A file or directory already there is the person's or
Claude Code's and is never changed, but for `settings.json` (below) and two keys of
`.claude.json`. rewake never reads `~/.claude`, `~/.claude.json` or a project's files; the
person's own `~/.claude/CLAUDE.md`, read as an ancestor's under another configuration
directory too, is excluded by `claudeMdExcludes`. What stays the person's in a worker: the
project's and local settings, the project's `CLAUDE.md`/`AGENTS.md`, skills, agents and
`.mcp.json`, and managed policy.

## The login

The login is the person's, once, in a worker's pane. `space open` of a Claude Code template without `--command` first runs `claude auth
status --json` under that variable; when it answers `loggedIn: false` (exit 1) the worker is
opened all the same, and the answer adds a line — `notice` under `--json` — telling main the
workers' Claude Code is not logged in and the person is to type `/login` once in this
worker's pane; main passes it on. A status that cannot be read, or does not answer within ten seconds — two
more for its output, should a wrapper's child hold it open —, adds nothing. Not logged in,
Claude Code shows no login screen of its own: the worker starts to its prompt, the footer
saying "Not logged in · Run /login", its mod live. A task sent before the login is delivered,
its turn stops at once on "Not logged in · Please run /login", and the task stays unread —
`rewake inbox --awaited` lists it as `unread`, nothing reports it failed — until the worker
runs `rewake inbox` on a later notice (2.1.302). The account is the person's,
logged in a second time.

## Onboarding and trust

Before each launch rewake makes sure the folder's `.claude.json`
holds `hasCompletedOnboarding: true` and `projects[<root>].hasTrustDialogAccepted: true` for
the worker's project — the main repository's root, as for its memory, under its path as
given and resolved, or the launch directory outside Git — adding what is missing and
keeping every other key: the person's own sessions there already trust it. A fresh directory otherwise stops on the
theme picker, then on the trust dialog, with nobody at the worker's screen; a trusted root
covers its worktrees and subfolders, a trusted parent of a repository does not
([research-launch.md](research-launch.md#a-configuration-directory-of-the-workers)). The file
is replaced in one step, 0600; Claude Code reads it again before its own saves. Known limit:
a save between rewake's read and its rename drops the entry, and the worker shows the dialog
in its pane until the next launch adds it again. A `.claude.json` rewake cannot read refuses
the launch, naming it, and stays as it is.

## Skills, plugins and MCP servers

A skill is attached by placing its folder, or a symlink to it, in `skills/`: Claude
Code loads it as a user skill named as the folder. A plugin is installed with the
variable set (`claude plugin install`, or `/plugin` in a worker's pane); `plugins/` is Claude
Code's own directory. `mcp.json` becomes `--mcp-config`.

## settings.json: the workers' user settings

`settings.json` is the workers' user settings, read by Claude Code itself; precedence is user < project < local < flag <
policy, so a project's settings can override one of rewake's defaults that is a single
value, while deny lists merge. rewake writes its defaults there on the first launch,
with `"rewake": {"defaults": <n>}`, the version of rewake's defaults the file is at;
Claude Code ignores that key. The defaults of version 3: `claudeMdExcludes` naming
`~/.claude/CLAUDE.md` as an absolute path; the owner's off-list —
`permissions.deny` of `Agent` (every subagent, the built-in `Explore` and `Plan` among
them), `ScheduleWakeup`, `SendMessage`, `ListAgents`, `ExitWorktree`,
`EnterPlanMode`, `PushNotification`, `RemoteTrigger` and `Artifact`; `disableWorkflows:
true`, `remoteControlAtStartup`, `promptSuggestionEnabled`, `awaySummaryEnabled`,
`idleCompaction` and `spinnerTipsEnabled` false; `disableClaudeAiConnectors: true`, which
drops only the person's claude.ai connectors — the project's `.mcp.json` servers and the
folder's `mcp.json` still load; `env` with `DISABLE_AUTOUPDATER=1`,
`DISABLE_FEEDBACK_COMMAND=1`, `CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY=1`,
`CLAUDE_CODE_AUTO_CONNECT_IDE=false` and `CLAUDE_CODE_DISABLE_CRON=1`. Kept: web fetch and
search, `AskUserQuestion`, `ExitPlanMode`, the terminal title, auto-compact, background
tasks and Claude Code's own messaging socket, which carries rewake's wake.

## Retired defaults

Version 1 had `autoMemoryEnabled: false`; version 2 retires it, so a file written
by the build before loses it where it still holds rewake's `false`. Version 3 takes
`EnterWorktree` out of `permissions.deny`: a deny in the user's settings is not lifted by a
run's allow, so each run's `--settings` denies it unless its template enters (below).

## The run's --settings

The run's `--settings` is a JSON object of what this run alone gets, merged by Claude
Code over the folder's settings: the worker's memory and `EnterWorktree`, allowed for a
template with `enter_worktree = true`, denied for every other run, and, for a run with a mod,
rewake's `statusLine` ([claude.md](claude.md#the-cache-ttl)). `--settings` is single-use — a
second one replaces the first, not merges — so the profile owns it: a worker's `--settings`
or `--setting-sources` after `--` is refused, naming the profile. An `env` value in the
user settings beats both `--autocompact` and the process environment, and `env` in
`--settings` beats the user settings, so a window variable in the folder's `settings.json`
beats a template's `launch.context` ([launch-defaults.md](launch-defaults.md)).

## Main's checkout is readable

The run's
`permissions.allow` holds `Read(//<root>/**)`, `<root>` the main repository's root as the
memory names it below, where main keeps the briefs and reviews a worker reads. A worker in a
checkout of its own stopped on auto mode's one-time prompt for a read outside its working
directories, with nobody at its screen; under the rule the read passes and no prompt shows
([research-launch.md](research-launch.md#a-read-outside-the-working-directories)). A `Read`
rule covers Read, Grep and Glob and grants no edit. `--add-dir` is not used: it makes main's
checkout a working directory, where edits pass as in the worker's own and its `.claude/`
loads.

## The worker's memory

A worker's memory is Claude Code's own
auto-memory, `MEMORY.md` loaded into the context and kept by the model as main's is, but
one per worker template and project:
`~/.config/rewake/claude/worker/memory/<project>/<template>/`, made 0700 before the launch.
`<project>` is the main repository's root, named as a room is named for its directory —
the last element as a readable slug and 12 hex digits of the SHA-256 of the resolved path
— so every worktree of one repository, and any directory in it, shares it, as Claude Code
keys its own; outside Git it is the launch directory. `<template>` is the worker's type for a
worker raised for one, else the alias the space was opened with, which every space has; an alias that cannot name a
directory (only letters, digits, dot, dash and underscore, not starting with a dot)
refuses the launch. Parallel workers of one template share the directory; the last write
of a fact wins. The run's `--settings` carries `autoMemoryEnabled: true` and
`autoMemoryDirectory: <that absolute path>`; Claude Code ignores a relative
`autoMemoryDirectory` ([research-launch.md](research-launch.md)). A person's own `"autoMemoryEnabled": false` in
`settings.json` is respected: the run's settings then carry no memory.

## A template that enters a checkout

A template with `enter_worktree = true` gets `EnterWorktree` in the
run's `permissions.allow`, and the run gets `REWAKE_WORKTREE_ENTER=1`. A run with no
directory for rewake's mod, or whose folder settings deny `EnterWorktree` (the person put
it back), gets no entry, noted
([worktree-tools.md](worktree-tools.md#a-workers-entry-enter_worktree--true-and-worktree_enter)).

## The person edits the file

A default is turned back on by changing or removing it —
`"Agent"` taken out of `permissions.deny` gives workers their subagents back,
`"idleCompaction": true` its idle compaction — and anything else of Claude Code's settings
is added beside them. rewake does not write the file again while it is at this build's
version, or a newer one.

## An upgrade adds only the newer defaults

A launch of a build whose defaults are newer
than the file's version runs only the steps after it: a default brought since is added
where the file has no value for it, into a list when absent from it; one retired since is
taken out only where the file still holds rewake's value; then the version is raised. What
the person took out of an older version stays out. A file with no `rewake` key is at
version 0 and gets every default, the person's values kept: the `{}` the folder held
before is such a file. A write replaces the file in one step, through a temporary file in
its directory, the target of a symlink when it is one; the keys come back in order and
indented, the values as they were. The launch notes that it wrote.

## A file rewake cannot read

A file rewake cannot read — not JSON, not one object, a `rewake` key other than
`{"defaults": <whole number>}` — refuses the launch, naming the file and the line, and is
never replaced. So does, at any version and naming the key, a key one of rewake's defaults
owns holding another kind than the default's: a string for `autoMemoryEnabled`, an object
in `permissions.deny`, a list for an `env` value, `env` or `permissions` not an object.
Claude Code drops a whole settings file over one such value ("These settings files did not
load", 2.1.302), every default with it. Keys no default owns are the person's, unchecked.
A `settings.json` that is a symlink to nothing is refused, naming its target, and the link
left as it is. A `rewake.keep` key is refused, naming `permissions.deny` as the way. An entry of the wrong kind in the folder refuses the launch naming the path.
