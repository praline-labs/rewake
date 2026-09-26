# Launch defaults and aliases

What a launch gets when the person did not type it: a model and a reasoning effort from
the environment or a settings file, and a whole set of arguments under one alias name.
The launch sequence itself, the room and role flags and the per-harness details are in
[launch.md](launch.md).

## Launch defaults from the environment

Sessions multiply, and one that quietly picks an expensive model costs real money
for work that did not need it. So rewake can supply a default model and reasoning
effort — as flags for a single launch, never by editing anyone's configuration.

The values are named one pair per harness, because the harnesses neither name their
models the same way nor take the reasoning effort the same way:

| Variable | What it sets |
| --- | --- |
| `REWAKE_CODEX_MODEL` | the model for a Codex launch |
| `REWAKE_CODEX_EFFORT` | its reasoning effort |
| `REWAKE_CLAUDE_MODEL` | the model for a Claude Code launch |
| `REWAKE_CLAUDE_EFFORT` | its reasoning effort |

No model names appear in this repository. Which model is cheap, and what it is
called, belongs to whoever runs the sessions.

They can be set in three places, and rewake reads the files itself — a setting that
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

**A file may set those four names and nothing else.** Not a prefix — a list. That is
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

## Naming a whole launch

A launch that states a role, a name, a model and a configuration override is long
enough that people stop typing it and start approximating it — and that is how a
session comes up as something other than what was meant. An alias is the opposite of
approximating: one name, one recorded set of arguments.

```toml
# ~/.config/rewake/aliases.toml, or .rewake.toml in the working directory
[alias.wcodex]
harness = "codex"
rewake  = ["--write", "--name", "writer"]
args    = ["--model", "some-model", "-c", "model_reasoning_effort=high"]
```

`rewake wcodex` is then the whole launch. TOML, because this is a structure rather
than a list of names and values; a separate file from `settings`, because that one is
`KEY=VALUE` and a file cannot be both.

**Three fields, not one list.** What rewake reads, which harness to start, and what
that harness gets are not interchangeable, and a single flat list would have to be cut
at the harness name for anyone — a person editing the file included — to know which
flag belongs to whom. **Lists, not strings**, for the same reason everywhere: a string
would have to be split into words, and splitting words means quoting rules.

An alias states a default, and a flag typed on the line *replaces* the alias's copy of
it — dropped, not merely outranked. So `rewake --name reviewer wcodex --model another`
launches as `reviewer` on `another`, and the command that reaches the harness names the
model once.

Dropping it is the only thing that works, and only for some flags. Codex refuses to
parse a repeated `--model` rather than taking the last one, so leaving both would end
the launch with a complaint about a flag the person wrote once. But a repeated
`--add-dir` is how a second directory is added, and a repeated `-c` key means one thing
for a plain value — the last wins — and another for a structured setting, which merges.
Replacing either would remove something nobody asked to remove, and sorting the two
kinds of setting apart means understanding the value, which the harness already does.

So the flags that get replaced are a closed list, published by each harness and applied
only to launches of that harness. Each entry names one parameter by all its spellings —
a typed `-m` replaces an alias's `--model` — and says whether it carries a value, which
is what keeps a dropped `--search` from taking the prompt standing behind it. Everything
else is appended and left to the harness, which is the safe answer for anything
unlisted; `-c` is never collapsed.

The reason differs by harness, and that difference is the point. Codex refuses to parse
a repeated flag, so replacement is what keeps the launch alive. Claude Code accepts one
and takes the last occurrence, so replacement there only keeps the command readable and
the two behaving alike. Both lists live in their adapters
([research-launch.md](research-launch.md) records what each was read from).

Two things this deliberately does not try to do. `--enable X` with `--disable X` is not
"the last one wins" — Codex applies every enable and then every disable — and flags that
conflict under different names, an approval policy against a sandbox mode, are not found
by comparing names at all. Resolving those is the harness's business, and an alias that
sets up such a pair is a mistake the person has to see rather than one rewake silently
half-fixes.

**Where the flags end.** `--` ends them for everything that follows it in the command
being built, not just in the half it was written in. A terminator you type ends your own
arguments; a terminator the alias carries ends its own tail *and* everything the line
adds behind it, because that is where those words land. So an alias ending in `--` is one
whose arguments are fixed: what you add after it is input for the harness, and a word of
a prompt can never take a setting out of the alias. Checking each half separately let
exactly that happen.

**Where a flag goes:** rewake's own flags before the alias name, the harness's after it.
`rewake --name reviewer wcodex --model another` sets the session name and the model;
writing `--name` after the alias would hand it to the harness, which knows nothing about
session names. That is the general rule of the command line — everything after a launch
word belongs to the harness — and an alias does not change it.

Environment defaults still fill in what the alias did not name.

An alias names arguments to rewake and nothing else. It cannot begin with a command
word, so it cannot turn `rewake <name>` into some other command — the same boundary
the settings file draws by having no substitutions.

**A project file may not choose a role, or the program.** `--main`, `--write` and
`--general` belong in the user's own file, and so does `command` (or `--command` in its
`rewake` list); an alias in `.rewake.toml` that carries one is refused by name. The
program is whatever runs with the person's credentials, the same concern as the role. The
role decides more than the room does — a main session sees everyone's telemetry and may
ask for repository access — and a file sitting in whatever directory somebody happens to
be in should not decide that for them. The user file is also held to `0600`, and a
launch says so when it is not: it can name a role, which is more than the settings file
beside it can do.

Refusals say what happened rather than falling through: a name that is not an alias
and not a command lists both, an alias with no harness or an unknown one says so and
names the file it came from, and a file that is present but unreadable produces a note
instead of silence. A launch that quietly runs something other than what was asked for
is the failure this feature exists to prevent, so it is the one outcome that is never
allowed.

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
- **Codex** takes the model as `--model` or `-m`, apart, with `=`, or joined
  (`-mname`) — and also as the configuration key `model`. It has no flag for the
  reasoning effort at all: that is the configuration key `model_reasoning_effort`.
  Keys are recognized through `-c`/`--config` in every spelling, with whitespace
  around the key allowed, because the CLI trims it.

Not consulted for these defaults, deliberately: the harnesses' own configuration files,
and settings passed with `--settings` or a Codex profile. (The Claude Code status line
is read from those files for another purpose, [in launch.md](launch.md#claude-code);
nothing else of them is.) rewake neither reads nor edits a person's configuration, and cannot see into a file it was handed — so a model set
there and a variable set for rewake would be two answers with no way to compare them.
The variable wins, being the one set for rewake specifically. A Codex launch with a
profile is refused outright before any of this, as are `--oss` and
`--local-provider`.
