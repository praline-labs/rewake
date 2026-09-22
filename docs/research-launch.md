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

## Which flags may be repeated

**The difference that matters first:** the two harnesses disagree about what repeating a
flag even means. Codex treats a repeat as a malformed command line; Claude Code takes
the last occurrence. So dropping a duplicate is a necessity on one and a convenience on
the other, and anything built on "the last value wins" is true of only one of them.

**[verified live; Codex CLI 0.155.1; September 21, 2026]** Codex refuses a repeated
`--model` outright — the launch ends with a parse error rather than the last value
winning. Four more were tried one after another with the same result, `--strict-config`
among them. The flags that *choose* something behave this way: `--model`, `--cd`,
`--sandbox`, `--ask-for-approval`, `--profile`, `--local-provider`, `--remote`,
`--remote-auth-token-env`, and the switches `--search`, `--oss`, `--worktree`,
`--strict-config`, `--no-alt-screen`, `--approve-for-me`,
`--dangerously-bypass-approvals-and-sandbox`, `--dangerously-bypass-hook-trust`. Short
and long spellings are one parameter: `-m` and `--model`, `-C` and `--cd`, `-s` and
`--sandbox`, `-a` and `--ask-for-approval`, `-p` and `--profile`, `-i` and `--image`.

Repeating is how other flags are *used*: `--image`, `--add-dir`, `--enable` and
`--disable` accumulate, and `--enable`/`--disable` carry a feature name, so two of them
are the same flag about different things. `-c key=value` repeats are accepted too.

Five of those refusals were observed directly — `--model`, `--strict-config` and three
more tried one after another. The rest of the list is read from the declarations rather
than probed: each is declared as a single-valued option or a switch, which is the same
shape as the five.

**[verified live; Claude Code 2.1.270; September 21, 2026]** A repeated `--model` or
`--effort` is accepted without complaint and the last occurrence decides; an effort
level is validated on every occurrence, so an invalid one is refused even when a later
occurrence would have replaced it. Its list-valued flags — `--add-dir`, `--plugin-dir`,
`--plugin-url`, `--mcp-config` and the others spelled `<values...>` in its help — are
repeated to add entries. There are no short spellings for `--model` or `--effort`.

**[source: reference tree `e29eceb75`; September 21, 2026]** A flag can answer to more
than one name, and **the help never says so**. `--dangerously-bypass-approvals-and-sandbox`
is also `--yolo`, and `--approve-for-me` is also `--not-so-yolo`; both aliases are
declared beside the long name in `codex-rs/utils/cli/src/shared_options.rs` and appear
in neither the short nor the long help.

How to find them, since half an hour went into learning that the help does not know:
read the declarations — an alias sits next to its long name, so one pass through the
file shows them all — and confirm by running the installed binary with both spellings
at once, which is refused as the same argument used twice. Use a subcommand that spends
no quota, such as `completion` — and put the flags **before** the subcommand, because
after it they belong to the subcommand and come back as "unexpected argument", which
looks like the same refusal and is not:

```
codex --approve-for-me --not-so-yolo completion bash   # exit 2: cannot be used multiple times
codex --approve-for-me completion bash                 # exit 0
codex completion bash --approve-for-me                 # exit 2: unexpected argument
```

Those two are the only aliases among the flags of an interactive launch;
`--permission-profile`, which also has one, belongs to the `sandbox` subcommand.

**[source: reference tree `e29eceb75`; September 21, 2026]** Two things about Codex that
running it does not show. The order of `--enable` and `--disable` does not decide the
outcome: every enable is applied and then every disable, so `--enable X --disable X`
leaves X disabled whichever came last. And a repeated `-c` key does not always mean the
same thing: a plain value is replaced by the last occurrence, while a structured setting
merges, so part of an earlier one can survive a later one that does not restate it.
Dropping an earlier `-c` because a later one names the same key can therefore lose
settings, and telling the two cases apart means understanding the value.

## When a check through the help text can be trusted

Moved from the handoff set on September 22, 2026: these are facts about a harness
obtained by running the binary, and this is their place rather than the handoff
directory.

**[verified live; Claude Code 2.1.270; September 21, 2026]** The help swallows any
unknown flag: `claude --definitely-not-a-flag x --help` prints the help and returns
zero. Checking argument spellings through the help is pointless there — it takes a
run without `--help`.

**[verified live; Codex CLI 0.155.1; September 21, 2026]** In Codex the result depends
on the placement: `codex --unknown --help` refuses (exit 2), `codex --help --unknown`
accepts (exit 0), `codex -c not-a-setting --help` accepts too. So the technique works
in one placement — the flag first, the help last — and only for the spelling of a
flag, never for the content of a setting.

**[verified live; Codex CLI 0.155.1; September 21, 2026]** Flag aliases are not shown
in the help at all: neither `--yolo` nor `--not-so-yolo` is there. How to find them
and how to check them is in the section on which flags may be repeated, above.

**[source: reference tree `e29eceb75`; September 17, 2026]** The real server does not
accept an empty cursor and does not send a conversation-started event on connection. A
fixture that does both is softer than the original — and tests on it go green on a
broken product: one review round found two High findings that way, with 289 tests
green.

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

## How each harness is published on npm

**[verified by download; Codex CLI 0.155.1 and 0.156.0, Claude Code 2.1.280; September
23, 2026]** What `tools/harnesscache` fetches, and why each harness is fetched
differently. Both publish a sha512 `dist.integrity` for every version, which the cache
checks before a version gets its name.

**Codex.** `@openai/codex` publishes the platform builds as versions of the *same*
package with a suffix: `0.155.1-linux-x64`, `0.155.1-linux-arm64`, and so on; the plain
`0.155.1` is a node wrapper (`bin/codex.js`) whose optional dependencies alias those
suffixed versions. The `linux-x64` tarball is 142 MB compressed and 370 MB unpacked, 46
files. The binary is `package/vendor/x86_64-unknown-linux-musl/bin/codex`, a
static-pie musl executable, so it runs in any Linux container without node or libc;
`codex-code-mode-host` shares its `bin` directory, and the rest sits in sibling
directories under the same vendor triple: `rg` in `codex-path`, and `bwrap`, a zsh and
a voice host with its own libraries in `codex-resources`. Neither package contains a
symbolic link (`find -type l` over all three cached versions is empty), which is why the
cache refuses links outright. The dist-tags carry the platform too (`linux-x64` names the latest
platform build), and `latest` on the plain package names the version.

**Claude Code.** `@anthropic-ai/claude-code` is a 184 KB wrapper: `bin/claude.exe` is a
placeholder that a `postinstall` script (`install.cjs`, run by node) overwrites with a
binary copied out of a platform package listed in `optionalDependencies`. Fetching the
wrapper alone gives no harness. The platform packages are separate names:
`@anthropic-ai/claude-code-linux-x64` (glibc) and `-linux-x64-musl`, with arm64 and
other platforms beside them, each versioned exactly like the wrapper. The cache
fetches the musl one directly and runs no install script: 103 MB compressed, 227 MB
unpacked, the executable at `package/claude`. `readelf` lists `libc.musl` as its only
needed library, and `claude --version` answers `2.1.280 (Claude Code)` in a bare
`alpine:3.22` container.

The `--version` answers the cache's `installed` selector reads: Codex prints
`codex-cli 0.155.1`, Claude Code `2.1.280 (Claude Code)`.
