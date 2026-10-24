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
| `REWAKE_PI_MODEL` | the model for a pi launch |
| `REWAKE_PI_EFFORT` | its thinking level |

No model names appear in this repository. Which model is cheap, and what it is
called, belongs to whoever runs the sessions.

Claude Code's two can be set in three places, pi's only in the environment, and rewake
reads the files itself — a setting that
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

**A file may set `REWAKE_CLAUDE_MODEL` and `REWAKE_CLAUDE_EFFORT` and nothing else**
(`readable` in `internal/adapter/settings.go`). Not a prefix — a list. That is
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
configured means nothing substituted — behavior without configuration is exactly what
it was before — and an **empty** variable is a choice rather than an absence: it turns
the default off for that launch, and the files are not consulted behind it. And
whatever is substituted is announced in a launch note naming both the variable and
where its value came from, because a silently swapped model is the worst kind of
surprise, and the second worst is not knowing where to change it.

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

```toml
# ~/.config/rewake/aliases.toml, or .rewake/aliases.toml in the working directory
[alias.claude-write]
harness = "claude"
role = "write"
args = ["--permission-mode", "acceptEdits"]
command = "claude-worker"
description = "A patch worker"
type = "code"
[alias.claude-write.launch]
model = "chosen-model"
effort = "high"
context = "400k"
```

| Field | Meaning |
| --- | --- |
| `harness` | The worker's harness, by its id; required. |
| `role` | `write` or `general`; `general` when absent. User file only. |
| `args` | Arguments handed to the harness as written. |
| `command` | The program started in place of the harness, a wrapper that runs it; becomes the worker launch's `--command`. User file only. |
| `description` | One line saying when to choose the worker. |
| `type` | The kind of work its worker does unless `space open --type` or `--no-type` says otherwise ([worker-types.md](worker-types.md)). |
| `[alias.<name>.launch] model` | Applied by the adapter: `--model` on Claude Code and pi. |
| `launch.effort` | Applied as Claude Code's `--effort` or pi's `--thinking`. |
| `launch.context` | The auto-compact window, applied on Claude Code as `--autocompact`: `auto`, or 100k–1M tokens such as `300k` or `1M`, any other value refused. Shown only on a harness without such a flag (pi). On Claude Code the strip and `rewake list` show it as the window and measure the fill against it, since the mod reports only the model's. |
| `launch.label` | How the strip and `rewake list` name the model, such as `Opus 5.5`; kept in the space record's `worker.label` and used only where the harness gives no display name of its own (Claude Code gives none to a mod; pi gives its registry's `name`). Without either, the model's id is shown. |

TOML, because this is a structure rather than a list of names and values; a separate file
from `settings`, because that one is `KEY=VALUE` and a file cannot be both. **Lists, not
strings**: a string would have to be split into words, and splitting words means quoting
rules. A field rewake does not know is refused, not skipped, and the removed ones are named
with what replaced them — `rewake` (the role is `role`; the space names the worker and its
room), `main` (main is started with `rewake <harness>`), `launch.apply` (settings always
apply; flags that should not go in `args`). The whole file is refused then, so it is
rewritten once.

**The project file's place, since October 21, 2026** (the owner's decision): a project's aliases
live in `.rewake/aliases.toml`, beside its types in `.rewake/types/`. A `.rewake.toml` in the
working directory, where they lived before, refuses the catalogue, naming the new place: read
silently or skipped, either would open workers other than the project says. Nothing is moved
for the person. Either alias file may also hold `[types] off = [...]`, the built-in types it
turns off ([worker-types.md](worker-types.md)).

**A project file may not choose a role, or the program.** `role` and `command` belong in the
user's own file; an alias in `.rewake/aliases.toml` that carries one is refused by name. The program is
whatever runs with the person's credentials, and a file sitting in whatever directory somebody
happens to be in should not decide that or what a worker may do. The user file is held to
`0600`, and a wider one produces a note.

`rewake workers` is a view available to any role, even outside a session. It reads the user
file and the current directory's `.rewake/aliases.toml`, a project alias replacing the user's of
the same name, and shows name, harness, role, command (the harness when none), description, type
and each setting's value marked applied or shown only, then the types `space open --type` takes;
`--json` returns `workers`, whose `model`, `effort` and `context` each have `value` and
`applied`, and `types`. A file that cannot be read refuses the
catalogue rather than silently omit workers; TOML errors include the source path, line and
column. An alias that makes no sense — no harness or an unknown one, a role that is not a
worker's, settings on a harness with no flags for them — refuses it too, naming the alias.

`rewake space open api --alias claude-write -- <more harness args>` starts the alias's harness,
role, command, arguments and applied settings; `--alias` is required, since a worker is raised
only from a template (October 19, 2026), and there is no `--harness` or `--role` beside it. The
space names the worker, and its room is main's whatever the checkout. Applied
settings replace older copies in `args`, and flags after `--` replace those in turn; environment
defaults fill what neither supplied. Under a `command` wrapper whose settings set
`CLAUDE_CODE_AUTO_COMPACT_WINDOW`, that value wins over `--autocompact`; without one a worker's
user settings no longer reach the run ([worker-profile.md](worker-profile.md)). A worktree
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
