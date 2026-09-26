# Launch aliases, on the project's first dependency — done, September 21, 2026

`rewake <alias>` is a whole launch: `internal/alias` reads `[alias.<name>]` tables from
`~/.config/rewake/aliases.toml` and `.rewake.toml` in the working directory, each with
three fields — the harness to start, a string; the flags rewake reads and the
arguments the harness gets, two lists — and the launch proceeds as if the arguments
had been typed. For a flag the harness takes at most once, the ones it names in
`SingleUseFlags`, a copy typed on the line replaces the alias's rather than merely
outranking it, so such a flag reaches the harness once; everything else, `--add-dir`
and `-c` among them, is appended, because repeating those is how a second value is
added. An unknown name is a refusal listing the names that exist; an alias
that expands into something unusable is a refusal showing the expansion; an alias can
name arguments to rewake and nothing else, for the reason the settings file has no
substitution. Lists, not strings: a string would have to be split into words, and
splitting words means quoting rules. Documented in [launch-defaults.md](../launch-defaults.md) under
"Naming a whole launch" and in the launch help; what was
learned about repeatable flags is in [research-launch.md](../research-launch.md).

The file is read by `pelletier/go-toml/v2`, the project's first dependency. The
owner's decision of September 21, 2026 lifted the standard-library-only rule: a
dependency is allowed one deliberate decision at a time, judged on having no
transitive dependencies of its own and on live maintenance, and one library that
covers several places beats three that each cover one. The rule is in `AGENTS.md`
and in [design.md](../design.md). This library was chosen on reconnaissance — actively
maintained, rewritten this year, faster, current specification, and, like its main
alternative, with no transitive dependencies of its own, which was the deciding
property. Why the alias came only now: a hand-written TOML parser here broke on valid
TOML through two review rounds and was removed. The format was not the problem;
writing the parser was.

Open: parsing the Codex configuration with the same library instead of looking for a
mention of a key in the file's text, queued in [work-queue.md](../work-queue.md).
