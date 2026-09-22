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

## Когда проверке через справку можно верить

Перенесено из набора передачи 22 сентября 2026: это факты о harness, добытые
запуском бинаря, и место им здесь, а не в каталоге передач.

**[verified live; Claude Code 2.1.270; September 21, 2026]** Справка проглатывает
любой неизвестный флаг: `claude --definitely-not-a-flag x --help` печатает справку и
возвращает ноль. Проверять формы записи аргументов через справку там бессмысленно —
нужен запуск без `--help`.

**[verified live; Codex CLI 0.155.1; September 21, 2026]** У Codex результат зависит
от расстановки: `codex --unknown --help` отвергает (код 2), `codex --help --unknown`
принимает (код 0), `codex -c not-a-setting --help` тоже принимает. То есть прием
работает в одной расстановке — флаг первым, справка последней — и только про
написание флага, никогда про содержимое настройки.

**[verified live; Codex CLI 0.155.1; September 21, 2026]** Псевдонимы флагов в
справке не показываются вовсе: ни `--yolo`, ни `--not-so-yolo` в ней нет. Как их
находить и как проверять — разделом ниже, про повторяемые флаги.

**[source: reference tree `e29eceb75`; September 17, 2026]** Настоящий сервер не
принимает пустой курсор и не шлет событие о начале беседы при подключении. Фикстура,
которая делает и то и другое, мягче оригинала — и тесты на ней зеленеют при сломанном
продукте: круг ревью нашел так две High при 289 зеленых тестах.

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
