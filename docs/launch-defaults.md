---
description: Where a launch's model, effort and arguments come from: worker aliases, environment variables, the settings files and their precedence.
---

# Launch defaults and aliases

What a launch gets when the person did not type it: a model and a reasoning effort from
the environment or a settings file; and the catalogue of workers main opens by alias name.
The launch sequence itself — `rewake <harness>` always main, in the room of its directory —
the worker profile and the per-harness details are in [launch.md](launch.md).

## Launch defaults from the environment

Sessions multiply, and one that quietly picks an expensive model costs real money
for work that did not need it. So rewake can supply a default model and reasoning
effort — as flags for a single launch, never by editing anyone's configuration.

The values are named one pair per harness, because the harnesses neither name their
models the same way nor take the reasoning effort the same way:

| Variable | What it sets |
| --- | --- |
| `REWAKE_CLAUDE_MODEL` | the model for a Claude Code launch |
| `REWAKE_CLAUDE_EFFORT` | its reasoning effort |
| `REWAKE_CLAUDE_CONTEXT` | its auto-compact window ([below](#the-context-window)) |
| `REWAKE_PI_MODEL` | the model for a pi launch |
| `REWAKE_PI_EFFORT` | its thinking level |

No model names appear in this repository. Which model is cheap, and what it is
called, belongs to whoever runs the sessions.

Each harness's two can be set in three places, and rewake reads the files itself — a setting that
has to be loaded by hand before every launch is not a setting but a ritual, and the
one time it is forgotten a session comes up on something else without saying so:

| Where | For |
| --- | --- |
| the environment | this launch, or this shell |
| `.rewake.env` in the working directory | one repository |
| `~/.config/rewake/settings` | everything this person launches |

Both files hold `KEY=VALUE` lines: blank lines and `#` comments are skipped, spaces
around the name and the value are trimmed, and one matching pair of quotes is removed.
Nothing more — no `$VAR` expansion, no commands, no line continuation. A file that can
run things is a different and much larger promise, and this is a settings file.

**A file may set these five, `REWAKE_CLAUDE_MODEL`, `REWAKE_CLAUDE_EFFORT`,
`REWAKE_CLAUDE_CONTEXT`, `REWAKE_PI_MODEL` and `REWAKE_PI_EFFORT`, and nothing else** (`readable` in `internal/adapter/settings.go`); each
launch page names its own harness's keys, beside the one line on where the files are. Not a prefix — a list. That is
a boundary rather than tidiness: a file sitting in whatever directory somebody happens
to be in must not become a way to set arbitrary variables for the harness launching
there, and names like the state directory or the room are spelled with the same prefix.
Everything else in the file is ignored without comment, so the file can serve other
purposes too. `export KEY=VALUE` is accepted, since that is how such a file is usually
written, and a `#` after whitespace begins a comment that runs to the end of the line
— inside quotes it is part of the value.

The strongest source wins, and strength is nearness to the launch: a flag, then a
variable already in the environment, then the project file, then the user file. A
variable that is already set is never replaced by a file. `.rewake.env` in the working
directory is the only project file consulted — no walking up the tree, where a parent
directory could decide how a session launches.

Trouble is said, not swallowed: a file that exists but cannot be read, one that is not
an ordinary file at all, a line that is not `KEY=VALUE`, or a user file with
permissions wider than the `0600` its convention expects — each produces a launch note,
and none of them stops the launch. The permission warning is for the user file only: a
project file created under the usual umask is `0644`, and warning about that on every
launch would teach people to skip these notes.

Three rules, all about not surprising anyone. An explicit flag always wins: a launch
that already names a model or an effort gets nothing added and no note. Nothing
configured means nothing substituted, but for the context window below — behavior without configuration is exactly what
it was before — and an **empty** variable is a choice rather than an absence: it turns
the default off for that launch, and the files are not consulted behind it. And
whatever is substituted is announced in a launch note naming both the variable and
where its value came from, because a silently swapped model is the worst kind of
surprise, and the second worst is not knowing where to change it.

### The context window

New Claude Code launches are main only. The worker defaults below describe retained profile
internals, not a way to raise another worker.

The one default that lives in the code: a Claude Code worker's launch keeps
auto-compaction to 272k tokens (`--autocompact 272k`; the owner's decision, October 27, 2026,
corrected the same day to workers only), so nothing needs configuring for it. Main's launch
gets no `--autocompact` from rewake and keeps the model's own window, unless a step below names one. From the nearest to the farthest, the window is:

1. a typed `--autocompact` (main's launch line, or after `--` on `space open`);
2. a worker template's `launch.context`;
3. `REWAKE_CLAUDE_CONTEXT`, from the environment, the project file or the user file;
4. the code's 272k, for a worker only.

`auto` at any step is the opt-out: the model's own window, whatever it is. A value is `auto`
or 100k–1M tokens such as `300k`; a bad one in a file or the environment refuses the launch,
naming the variable and where it was read, unless a nearer step made it unused. An empty
variable is no answer here, so the code's default applies; write `auto` to turn the limit off. pi has no
flag for its window and is untouched. The strip and `rewake list` measure the fill against
the window the launch passed ([claude.md](claude.md#telemetry)).

"Already named" means every way of naming it, because the way a person writes a
choice must not decide whether it is heard. Checked against the installed binaries,
not from memory:

- **Claude Code** takes both as flags, `--model <model>` and `--effort <level>`,
  apart or with `=`. Neither has a short form.
- **pi** takes `--model` and `--thinking`. An explicit `--models` scope also suppresses
  the model default; a thinking suffix on `--model` or `--models` suppresses the effort
  default. Facts checked on 1.2.0 are in [research-pi.md](research-pi.md).
- **Codex** takes the model as `--model` or `-m`, apart, with `=`, or joined
  (`-mname`) — and also as the configuration key `model`. It has no flag for the
  reasoning effort at all: that is the configuration key `model_reasoning_effort`.
  Keys are recognized through `-c`/`--config` in every spelling, with whitespace
  around the key allowed, because the CLI trims it.

Not consulted for these defaults, deliberately: the harnesses' own configuration files,
and settings passed with `--settings` or a Codex profile. rewake neither reads nor edits a person's configuration, and cannot see into a file it was handed — so a model set
there and a variable set for rewake would be two answers with no way to compare them.
The variable wins, being the one set for rewake specifically. A Codex launch with a
profile is refused outright before any of this, as are `--oss` and
`--local-provider`.

## The worker catalogue

**Owner decision, October 19, 2026** ([worker-profile.md](worker-profile.md)): aliases are
only main's catalogue of workers. `rewake workers` lists them and `rewake space open <space>
--alias <name>` starts one; `rewake <alias>` is no launch — the word is refused, naming the
`space open` that opens it. Since October 18 the aliases had been launches and the catalogue at
once; the `rewake` list, `main = true` and `launch.apply` went with that.

**Owner decision, November 11, 2026:** new workers are pi only. Claude Code's adapter offers
`MainOnly`: its aliases may stay in the files but `workers` omits them, and `space open`
refuses one specifically by alias, saying its harness launches main only, before creating a
space. `command` replaces the executable, not the harness's launch policy. Existing sessions
are not ended or migrated, and rewake edits no alias file.

```toml
# ~/.config/rewake/aliases.toml, or .rewake/aliases.toml in the working directory
[alias.pi-write]
harness = "pi"
role = "write"
args = []
command = "pi-worker"
description = "A patch worker"
[alias.pi-write.pi]
start_mode = "build"
[alias.pi-write.launch]
model = "chosen-model"
effort = "high"
```

| Field | Meaning |
| --- | --- |
| `harness` | The worker's harness, by its id; required and must permit workers (pi today). |
| `role` | `write` or `general`; `general` when absent. User file only. |
| `args` | Arguments handed to the harness as written. |
| `command` | The program started in place of the harness, a wrapper that runs it; becomes the worker launch's `--command`. User file only. |
| `description` | One line saying when to choose the worker. |
| `[alias.<name>.pi] start_mode` | The runtime work mode its worker starts in unless `space open --work-mode` says otherwise ([work-modes.md](work-modes.md#selecting-the-start)). |
| `[alias.<name>.launch] model` | Applied by the adapter: `--model` on Claude Code and pi. |
| `launch.effort` | Applied as Claude Code's `--effort` or pi's `--thinking`. |
| `launch.context` | The auto-compact window, applied on Claude Code as `--autocompact`: `auto`, or 100k–1M tokens such as `300k` or `1M`, any other value refused. Shown only on a harness without such a flag (pi). On Claude Code the strip and `rewake list` show it as the window and measure the fill against it, since the mod reports only the model's. |
| `launch.label` | How the strip and `rewake list` name the model, such as `Patch writer`; kept in the space record's `worker.label` and used only where the harness gives no display name of its own (a Claude Code run's status line gives `model.display_name` once it has run, and that wins, less a trailing parenthesis; pi gives its registry's `name`). Without either, the model's id is shown. |

TOML, because this is a structure rather than a list of names and values; a separate file
from `settings`, because that one is `KEY=VALUE` and a file cannot be both. **Lists, not
strings**: a string would have to be split into words, and splitting words means quoting
rules. A field rewake does not know is refused, not skipped, and the removed ones are named
with what replaced them — `rewake` (the role is `role`; the space names the worker and its
room), `main` (main is started with `rewake <harness>`), `launch.apply` (settings always
apply; flags that should not go in `args`), under "fields whose place is elsewhere". The whole
file is refused then, so it is rewritten once. The worker type's old settings are the one
exception: they are recognized as obsolete, skipped with a migration notice, and main starts on
the rest ([work-modes.md](work-modes.md#the-old-worker-types)). Only the old shape is skipped — a
`type` holding a string and a `[types]` section holding `off = ["<name>", ...]` — and a definition
is checked against the current fields before it is left out: a malformed value there, and a broken
`start_mode`, mode or launch setting beside an old `type`, still refuses the file. A template the
old `type` leaves out follows the same whole-template rule as any other: the user's and the
project's definitions replace one another whole, only the definition in force is omitted, and its
notice names that file.

**The project file's place, since October 21, 2026** (the owner's decision): a project's aliases
live in `.rewake/aliases.toml`, beside its modes in `.rewake/modes/`. A `.rewake.toml` in the
working directory, where they lived before, refuses the catalogue, naming the new place: read
silently or skipped, either would open workers other than the project says. Nothing is moved
for the person. The old `[types] off = [...]` and a `types/` directory are skipped with a notice
saying where a mode file goes ([work-modes.md](work-modes.md#the-old-worker-types)); built-in
modes are not turned off.

**A project turns templates off** (the owner's, October 26, 2026): `[aliases] off = ["patch-write"]`
in `.rewake/aliases.toml` hides those templates of the merged catalogue, the user's included, for
that project only. A template off is gone from `rewake workers` (lines and `--json`) and from
main's catalogue, and `space open --alias <it>` refuses: `the alias "x" is turned off in the project
alias file ([aliases] off); available: ...`. `off` is subtraction from the merged catalogue: a name
it does not hold is already off, so a project that still lists a template removed later from the
user's or its own file hides nothing, launches as before, and is told to drop the name (met on
November 11, 2026, when a project's list kept two obsolete templates and refused every launch
there; the launch-blocking refusal is gone). Two refusals of the
whole catalogue remain, as an unknown field is: a name the same file defines (`defines [alias.x] and
turns it off in [aliases] off; keep one`, a contradiction, not hidden); and `[aliases] off` in the
user's file (`belongs in a project's .rewake/aliases.toml, and to drop a template everywhere delete
it from this file`). A migration notice — the name with its file — is printed by every command
that reads the catalogue, and a launch carries it into the session's briefing, so main can repair
the file itself.
With no launchable `write` template left, `rewake workers` says `No launchable write-role
templates.` — inventory alone — and states main's authorship policy apart from it. With the default
it adds that implementation must be delegated when it is requested, and with the project's
`[main] implements = true` that main may implement under its work mode and the project's rules,
asking for no writer template. `--json` keeps `noWriter` as the inventory boolean and adds
`mainImplements` and `mainPolicySource` (`session` for a live main's frozen snapshot,
`configuration` for what a fresh launch in the command's directory would take; see
[roles.md](roles.md#main-implementation)).

**Main's authorship is a project's policy**, in `.rewake/aliases.toml` alone:

```toml
[main]
implements = true
```

Missing or false is the default: main delegates. True permits main to implement product changes
itself, subject to the task, its work mode and the project's rules; it grants no permission and
is read once when main starts, then frozen for that run. A user file that carries `[main]` is
refused. This is not a role and does not change reporting, Git authority or a worker's limits.

[Work modes](work-modes.md) add optional startup model/effort fields before ordinary launch
defaults: explicit harness flags win, then the starting mode, then these defaults. Global `[pi]`
fields merge user then project; `[alias.<name>.pi]` overlays that result, without changing
whole-template replacement. Start precedence is `space open --work-mode`, template `start_mode`,
global `start_mode`, then advisor, main included; type-derived defaults are gone.

**A project file may not choose a role, or the program.** `role` and `command` belong in the
user's own file; an alias in `.rewake/aliases.toml` that carries one is refused by name. The program is
whatever runs with the person's credentials, and a file sitting in whatever directory somebody
happens to be in should not decide that or what a worker may do. The user file is held to
`0600`, and a wider one produces a note.

### Keeping templates on the machine

A project shares what is true for everyone — its working modes, its `[aliases] off`, its
`[main] implements` and its brief folder — and names in its own documentation the local templates
its workflow expects. The templates themselves stay on each machine, because a template is the
whole recipe: harness, role, command, launch settings and the model and effort its modes select.
Only the user file may set `role` and `command`; a project definition replaces the user's template
of the same name **whole**, so a shared definition without `role` would make even a user writer
general. There is no field-level overlay and no named model profile: a shared recipe with a
machine-local model would need one, and inheriting a credentials-bearing command or a write role
into a materially different project recipe is worse than repeating three small definitions on each
machine.

A fresh machine's `~/.config/rewake/aliases.toml`, permissions 0600. The model strings are
placeholders, not models to install: each person replaces them with exact IDs from this machine's
own rewake pi registry, and effort may be added beside a model that supports it.

```toml
[alias.writer]
harness = "pi"
role = "write"
worktree = true
description = "Implements an assigned change"
[alias.writer.pi]
start_mode = "build"
[alias.writer.pi.modes.build]
model = "local-provider/writer-id"

[alias.reviewer]
harness = "pi"
role = "general"
description = "Reviews or advises; does not edit the subject"
[alias.reviewer.pi]
start_mode = "review"
[alias.reviewer.pi.modes.review]
model = "local-provider/reviewer-id"
[alias.reviewer.pi.modes.advisor]
model = "local-provider/reviewer-id"

[alias.helper]
harness = "pi"
role = "general"
description = "Investigates general questions"
[alias.helper.pi]
start_mode = "advisor"
[alias.helper.pi.modes.advisor]
model = "local-provider/helper-id"
```

The shared `.rewake/aliases.toml` of a project whose main reviews and implements, on a machine
that defines those names:

```toml
[main]
implements = true

[pi]
start_mode = "build"

[aliases]
off = ["writer", "helper"]
```

`off` is subtraction, not an allowlist: other locally installed templates stay available, and a
machine dedicated to this workflow may instead define only `reviewer`. A machine with no templates
gets the empty catalogue and cannot raise an invented worker. The starting-mode fields are the
existing exact model selection ([work-modes.md](work-modes.md#model-thinking-and-switching)), not
another precedence tier; a template's `launch.model` remains valid and local. Nothing is migrated
or rewritten: choosing local templates and removing machine IDs from a shared file is the
person's edit, and it changes the next launch, never a running session.

`rewake workers` is a view available to any role, even outside a session. It reads the user
file and the current directory's `.rewake/aliases.toml`, a project alias replacing the user's of
the same name and the project's `[aliases] off` removing templates, and shows name, harness, role, command (the harness when none), description, its `start_mode`,
the starting mode's own `start_model` and `start_effort` when it configures them (marked as
overriding the template's launch settings), each setting's value marked applied or shown only, and
the label, omitting main-only harnesses without rejecting the other templates, then the mode
catalogue in cycle order with each mode's name, source and description, never its text; `--json`
returns `workers`, whose `model`, `effort` and `context` each have `value` and `applied`, with
`label` when one is set, `startModeSettings` when the starting mode configures a model or effort,
and `modes`. rewake ships no templates: an empty
or main-only catalogue answers `No launchable worker templates: define [alias.<name>] for a worker-capable harness in ...`, and `space open --alias`
with none defined refuses with `no templates are defined; rewake workers --help says where` in
place of the empty `available:` list. The fields are listed once, on `rewake workers --help`; a
launch page only points there. Both files are read from the directory the command runs in, not
the room's, so the help tells main to run `workers` and `space open` from the project's top. A file that cannot be read refuses the
catalogue rather than silently omit workers; TOML errors include the source path, line and
column. An alias that makes no sense — no harness or an unknown one, a role that is not a
worker's, settings on a harness with no flags for them — refuses it too, naming the alias.

`rewake space open api --alias pi-write -- <more harness args>` starts the alias's harness,
role, command, arguments and applied settings; `--alias` is required, since a worker is raised
only from a template (October 19, 2026), and there is no `--harness` or `--role` beside it. The
space names the worker, and its room is main's whatever the checkout. Applied
settings replace older copies in `args`, and flags after `--` replace those in turn; environment
defaults fill what neither supplied. Existing Claude Code worker-profile internals stay in
place ([worker-profile.md](worker-profile.md)), but no new worker uses them. A worktree
named in the alias is recorded as the space's checkout; an explicit space `--worktree` replaces
it. rewake neither reads nor edits the person's wrapper or harness settings.

**Which typed flags replace the alias's.** A flag after `--` replaces the alias's copy of it —
dropped, not merely outranked — so the command that reaches the harness names the model once.
Dropping it is the only thing that works, and only for some flags. A harness may refuse to
parse a repeated `--model` rather than take the last one. But a repeated `--add-dir` is how a
second directory is added, and a repeated `-c` key means one thing for a plain value — the last
wins — and another for a structured setting, which merges. Replacing either would remove
something nobody asked to remove, and sorting the two kinds of setting apart means understanding
the value, which the harness already does.

So the flags that get replaced are a closed list, published by each harness and applied only to
workers of that harness. Each entry names one parameter by all its spellings — a typed `-m`
replaces an alias's `--model` — and says whether it carries a value, which is what keeps a
dropped `--search` from taking the prompt standing behind it. Everything else is appended and
left to the harness; `-c` is never collapsed. Claude Code accepts a repeated flag and takes the
last occurrence, so replacement there only keeps the command readable; the lists live in their
adapters ([research-launch.md](research-launch.md) records what each was read from). Two things
this deliberately does not try to do: `--enable X` with `--disable X`, and flags that conflict
under different names, are the harness's business, and an alias that sets up such a pair is a
mistake the person has to see.

**Where the flags end.** `--` ends them for everything that follows it in the command being
built, not just in the half it was written in. A terminator the alias carries ends its own tail
*and* everything typed behind it, because that is where those words land: what is added after it
is input for the harness, and a word of a prompt can never take a setting out of the alias.
