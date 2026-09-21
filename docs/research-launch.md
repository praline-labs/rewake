# Research: what a session is launched with

Split out of [research.md](research.md) on September 21, 2026, when that file passed
the project's 400-line limit. The division is by how a fact is obtained, because that
is also how it ages: everything here is learned by running an installed binary and
reading what it answers — which model names it takes, which efforts it offers, what a
catalogue on disk holds, and which shapes of argument it accepts at all.

Tags: **[verified live]** — live on the named versions; **[source]** — read in the
source; **[docs]** — official pages. Facts here age with harness versions: recheck
before touching an adapter.

## Models, efforts and their catalogues

**[verified live; Codex CLI 0.155.1; September 21, 2026]** What a model catalogue can
be read from, and at what cost.

`codex debug models` prints it as JSON: model names, descriptions, the effort levels
each model offers — they differ per model — context windows, and visibility. It makes
no model request, but refreshes through the network under the current account when its
cache has expired; the cache is `~/.codex/models_cache.json`. The refreshed catalogue
and the one built into the binary are not the same: `--bundled`, which skips the
refresh and dumps what ships inside, lists nine entries where an ordinary call returns
seven. That is what the experiment shows. That the list varies with the account follows
from the code, not from this comparison.

The same information, minus context windows, comes from the protocol method
`model/list`, which is sturdier: it belongs to the protocol this adapter already
speaks, where `codex debug` is a debug group with no such promise.

Context windows need arithmetic, not quoting. Each model carries `context_window`, the
value in effect, `max_context_window`, the ceiling, and
`effective_context_window_percent`, the share of the result that is actually usable.
The configuration key `model_context_window` does not merely narrow: it *replaces* the
value in effect, clamped to the ceiling only when a ceiling is known — so it can raise
the window as well as lower it (reference `e29eceb75`,
`models-manager/src/model_info.rs:19`).

The whole rule, in order. With the key set, the window is the smaller of the key and
the ceiling when a ceiling exists, and the key alone when it does not. With no key, it
is the value in effect, or the ceiling if that is missing. The usable window is that
result multiplied by the percentage — not a share of the ceiling. The catalogue is
printed before the key is applied, which is why this has to be computed rather than
read off.

**[verified live; Claude Code 2.1.270; September 21, 2026]** There is no command that
lists models: the full list of subcommands in the help was read through, and none of
them does that.

The model flag does not validate its value: an unknown name passes in silence, where an
unknown effort is answered immediately with the permitted set. What happens to that
name afterwards was not observed — seeing it would need a real request — so the
expectation that it fails later, at the provider, is an expectation, not a finding.

Effort levels are obtainable reliably: an invalid value makes the harness print the
permitted set itself, and the same set appears in the help.

Models do exist in an internal catalogue cache in the Claude home. Its file name
carries an identifier and a hash, and its contents follow the current login — so the
catalogue is cached per login rather than per machine. It is undocumented, has an
expiry, and may simply be absent on a fresh machine. No context window is recorded
anywhere in that file: every key of it was examined.

**That is a statement about the subcommands that were read and the catalogue cache,
and about nothing else.** The status line is part of the CLI too, so "the CLI has no
source" would already claim more than was looked at. Elsewhere
there is more: the Agent SDK reports the available models together with their effort
levels and can report context usage, the size of the window appears in a model's usage
data, and the status line receives the context window size directly. This
reconnaissance read the list of subcommands and the files on disk, not the SDK and not
what the status line is handed, so it cannot say there is no source — only that it did
not look where those are. What was not found anywhere is a ready-made catalogue of windows for
all models without starting a session.

**Three of those sources are unofficial**: the debug subcommand group, the internal
cache, and the text of a warning message. `model/list` is part of an interface, but of
the Codex protocol specifically. On the Claude Code side the documented route is the
Agent SDK rather than the CLI, and it was not examined here. Anything built on the
unofficial three has to treat their disappearance as ordinary, not exceptional.

## Passing a model and an effort for one launch

**[verified live; Codex CLI 0.155.1 and Claude Code 2.1.270; September 21, 2026]**
How each harness takes a model and a reasoning effort for one launch, read from the
installed binaries and checked by running them.

**Read this before trusting a `--help` probe.** Appending `--help` to a command is the
cheap way to ask "does this CLI accept that spelling", and on these two harnesses it
answers different questions — or none:

| Command | Exit | What it actually tells you |
| --- | --- | --- |
| `claude --definitely-not-a-flag x --help` | 0 | nothing: Claude Code takes any unknown flag beside `--help` and prints the help |
| `codex --unknown --help` | 2 | the spelling is rejected — the useful form |
| `codex --help --unknown` | 0 | nothing: `--help` before the flag short-circuits |
| `codex -c not-a-setting --help` | 0 | nothing about the *setting*: only the flag's spelling was checked |

So on Codex the probe works in exactly one arrangement — the flag first, `--help` last
— and only for how a flag is written, never for what a configuration setting contains.
On Claude Code it does not work at all, and a form has to be checked by running the
command without `--help`: `claude -m x` answers "unknown option '-m'", which is how it
is known that Claude Code has no short form for either flag. Claude Code takes both
as flags: `--model <model>` and `--effort <level>`, where the levels it lists are
low, medium, high, xhigh and max. Codex takes the model as `-m`/`--model <MODEL>`,
but has no flag for the reasoning effort: it is the configuration key
`model_reasoning_effort`, set for a single launch through the repeatable
`-c key=value` override. The key name is confirmed in the reference tree at
`e29eceb75` (`codex-rs/core/src/config/edit.rs:227`). Codex also accepts the model as
the configuration key `model`, so a person can state that choice either way. Its
override parser splits a setting on the first `=` and trims both halves
(`codex-rs/utils/cli/src/config_override.rs`), which is why `-c 'model = "x"'` and
`-c model="x"` are the same setting — verified live on 0.155.1, both accepted. Neither harness needs its
configuration file touched for either setting.
