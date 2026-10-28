---
description: What a harness binary answers at launch: models, efforts, repeated flags, the settings order, the workers' configuration folder.
---

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
not look where those are. What the status line is handed was read on September 23, 2026
([research.md](research.md#telemetry-sources-the-status-line-and-hooks)): the running
model's window, not a catalogue. What was not found anywhere is a ready-made catalogue of
windows for all models without starting a session.

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

## Whether Claude Code has a client-server split to sit between

**[help and the binary's bundled source, cached 2.1.280, no session started; September
23, 2026]** Codex splits its interactive terminal from a session server over a
documented protocol, and rewake owns that server. Claude Code offers nothing of that
shape that a local program could stand between. Remote Control — `--remote-control
[name]` and the subcommand spelled `remote-control`, `rc`, `remote`, `sync` or `bridge` —
refuses to start without a claude.ai subscription login and talks to the vendor's
service: the source names `wss://bridge.claudeusercontent.com`, `environments/bridge`
and `/v1/sessions/<id>/events`. `CLAUDE_CODE_BRIDGE_SESSION_ID` is set from that bridge
session's id. The one local split is background sessions: `--bg` starts a
session under a supervisor, and `claude attach <id>` opens it in a terminal through a
per-session `<id>.pty.sock` served by a pty host, with `claude logs` printing recent
terminal output. What crosses that socket is a terminal, not a structured session
protocol, it is undocumented, and relaying a terminal is outside rewake's boundaries.
The structured protocol Claude Code does document is `--print` with
`--input-format stream-json` and `--output-format stream-json`, the Agent SDK's
transport. It runs without the interactive terminal, so a person would lose the screen
they work in.

**[the bundled source of 2.1.310, read with `strings`, no session started; October 28, 2026]**
Still no local split. The mods API's "surfaces" (`desktop`, `mobile`, `vscode`) and its
`session.attach` are a render-tree RPC carried as `control_request` subtypes (`ui_attach`,
`ui_render`, `ui_press`, `ui_input`, `ui_select`, …) over the SDK's stream-json channel or the
Remote Control bridge: a client that already has the conversation asks the mods to draw a
component and sends presses back; it is no mirror of a session. An interactive terminal session
takes such requests only through the bridge; `--sdk-url` refuses hosts other than the vendor's.
The VS Code and Desktop apps are SDK hosts that spawn `claude` headless. `--messaging-socket-path`
is inbound only. The transcript file gets one line per finished content block — no streaming
text, no permission dialog, no question until answered. What a local program can get from a live
interactive session is what a mod sees: the `ui.render` sites' props (UserMessage,
AssistantMessage, ToolUse, ToolResult, ToolGroup, ToolProgress, Spinner, AskUserQuestion, …),
`$.session.messages()`, the turn events and `tool.check`, pushed out by `$.http.fetch`; not seen:
streaming deltas (unverified whether `ui.render` fires per delta) and the permission dialog's own
text.

## A prompt draft at launch

**[verified live; Claude Code 2.1.280, private HOME, fake API; September 23, 2026]**
`--prefill <text>`, and `--prefill-b64` with the text in base64, puts the text into the
input box without submitting it and shows `Pre-filled prompt · review before pressing
Enter`. It fills the first prompt only; a draft for a session already running needs a
plugin ([research.md](research.md#interrupting-a-turn-commands-and-the-stop-hook)).

## A worktree at launch

**[verified live; Claude Code 2.1.280, rewake 0ee310e, private HOME, stand-in API; September 26, 2026]** `rewake --room probe claude -w probe` launched, registered and took
deliveries as any session: a line from a plain shell and one signed by main both woke the
idle session and started a turn. Claude Code made the worktree
`.claude/worktrees/probe` on a branch `worktree-probe` and ran there — its process's
working directory and the path in its TUI — while the session record, and so `rewake
list`, kept the directory rewake was launched from
([design.md](design.md#session-record)). After `/exit` Claude Code removed the worktree
and the branch itself; `git worktree list` showed only the main checkout. A report at
the turn's end was not seen: the stand-in API calls no tools, so the session never ran
`rewake inbox`, and a message not read owes no report.

What rewake's own worktree takes from Claude Code's — the flags that continue a
conversation, trust, `.worktreeinclude` — is in [research-worktree.md](research-worktree.md).

## Hook options and settings order that rewake's launch layer relies on

**[the binary's bundled source, cached 2.1.280; September 23, 2026]** A command hook
takes `"async": true` — "runs in background without blocking": the harness writes the
payload to its stdin, closes it and goes on. It also takes an exec form, `args` beside
`command`, which skips the shell; rewake does not use it, since a version that did not
know the key could refuse the whole layer. The status line has neither: `type`,
`command`, `padding`, `refreshInterval`, `hideVimModeIndicator`. The settings layers are
merged in the order of the harness's own list, `userSettings, projectSettings,
localSettings, flagSettings, policySettings`, later over earlier; project and local are
read under the working directory, the user layer under the configuration directory, and
the machine policy from `/etc/claude-code/managed-settings.json` with a
`managed-settings.d` beside it. `--setting-sources` names which of user, project and
local are read, and `--restricted` reads none of them (`claude --help`).

**[`claude --help`, installed 2.1.280; September 27, 2026]** The allow list for a launch
has two spellings, `--allowedTools, --allowed-tools <tools...>`, as does
`--disallowedTools, --disallowed-tools`. Rewake adds its `Bash(rewake:*)` rule only when
neither spelling of the allow list is given.

## What the worker profile relies on

**[`claude -p --no-session-persistence "/context"`, installed 2.1.302, no model call; October
19, 2026]** `/context` lists the memory files, skills, MCP tools and the window without a model
call, in a private HOME as well as the person's. What the worker profile
([worker-profile.md](worker-profile.md)) takes from it:

- `--setting-sources project,local` leaves out the user settings, user skills, the user MCP
  servers of `~/.claude.json` and the user `CLAUDE.md`. The claude.ai connectors stay; only
  `--strict-mcp-config` drops them, with every server not named by `--mcp-config`.
- `CLAUDE.md` and `.claude/CLAUDE.md` of each directory above the working one load as
  *Project* memory, so `~/.claude/CLAUDE.md` loads for any checkout under the home directory —
  with the user source off, and under another `CLAUDE_CONFIG_DIR`. `claudeMdExcludes` in
  `--settings` (absolute paths or globs; user, project and local memory) leaves it out.
- `--settings` takes a file or a JSON string and is single-use: given twice, only the last
  applies. Its `skillOverrides`, `autoMemoryEnabled` and `env` apply. A top-level key Claude
  Code does not know, `"rewake": {"defaults": 1}`, is ignored: the file's other settings apply,
  and neither `-p` nor the interactive start warns (October 19, 2026).
- `autoMemoryDirectory` is read from `--settings` (policy settings first, then the flag's,
  then local and project when the folder is trusted, then user); only an absolute or `~/` path
  counts — a relative one is ignored for the default key, the main repository's root shared by
  its worktrees. Claude Code makes the directory; `/context` lists its `MEMORY.md` as
  `AutoMem`, and the model answers from it. `autoMemoryEnabled` defaults to true; the user source
  off does not turn it off (October 19, 2026, with `/context` and one haiku call).
- With the user source off, `$CLAUDE_CONFIG_DIR/CLAUDE.md` of a directory that is not an
  ancestor's `.claude` does not load (a private `CLAUDE_CONFIG_DIR` holding one, `/context`
  listing no memory file; October 19, 2026).
- An `env` value in the user settings beats `--autocompact` and the process environment; one in
  `--settings` beats the user settings. `--autocompact` takes `auto` or 100k–1M (`300k`, `1M`,
  `200000`) and reads the leading number of anything else: `300K (note)` is 300k, `250k5` is
  250k; `99k`, `2M` and `1.5M` are refused.
- `--plugin-dir` of a folder whose children are plugins loads each; a plugin's skills show as
  `<plugin>:<skill>`, source `Plugin`.

## A configuration directory of the workers

**[verified live, installed 2.1.302, no login, no model call; October 23, 2026]** Run in private
HOMEs with `env -i` — no `CLAUDE_CONFIG_DIR`, `HOME` or `REWAKE_*` of the calling session —
through `claude -p "/context"`, a `SessionStart` hook, or an interactive start in a
pseudo-terminal killed after a few seconds. What the workers' own directory
([worker-profile.md](worker-profile.md#the-service-folder)) rests on:

- `claude auth status` with a fresh `CLAUDE_CONFIG_DIR` prints JSON with `loggedIn: false`,
  `configDirectory` and `projectsDirectory`, and **exits 1**, in about 0.16 s with no network;
  `--text` says "Not logged in. Run claude auth login". It makes the directory's `.claude.json`.
- An interactive start in a fresh directory stops on onboarding's theme picker. With
  `.claude.json` holding `hasCompletedOnboarding: true` it stops next on the folder-trust dialog —
  before any login. Trusted and not logged in, it starts to the prompt, the footer saying "Not
  logged in · Run /login", with a banner "Auto mode is now Claude Code's default permission
  mode" and auto mode on: no dialog.
- `projects[<path>].hasTrustDialogAccepted: true` on a repository's root trusts the root, its
  subfolders and a linked worktree elsewhere on disk; on a folder outside Git, its subfolders.
  On a parent of a repository it does not reach the repository or its worktree.
- The directory's `CLAUDE.md` loads as *User* memory, `skills/<s>/SKILL.md` as the skill `<s>`,
  source *User*, `agents/<a>.md` as a custom agent, *User*, and `settings.json` as the user's
  settings — its `env` reached the process and its `SessionStart` hook ran. With
  `--setting-sources project,local` all of them are gone.
- `--settings '{"autoMemoryEnabled": true, "autoMemoryDirectory": "<absolute>"}'` over that
  `settings.json` gives the AutoMem at that path and keeps the file's `env` and hook: a flag's
  settings merge with the user's. rewake's worker `settings.json`, copied in as the user's, drew
  no warning at an interactive start.
- A session running in the directory while another process replaced `.claude.json` with a copy
  holding one more trusted project (read, change, rename) saved at its exit (`lastGracefulShutdown`)
  and the other entry survived: Claude Code reads the file again before it saves (one run). It
  holds a directory `.claude.json.lock` while it writes.
- One `-p --session-id` run in the person's directory (`$HOME/.claude`) and one in the workers',
  same working directory: each transcript under its own `projects/<slug>/`; from the person's,
  `--resume <the worker's id>` answers "No conversation found" and `--continue` takes the
  person's own; from the workers', the id resumes.
- Claude Code makes in a directory `.claude.json`, `backups/`, `cache/`, `projects/`,
  `sessions/` and `session-env/`, and `plugins/` for installed plugins and a mod's `$.store`
  (`plugins/store/<name>_inline-<hash>.json`, from the `mods` skill).

**[verified live, installed 2.1.302, no login, no model call; October 23, 2026]** A worker of a
test build of rewake started directly in a private HOME, its folder holding only what rewake
writes, on a pseudo-terminal, then sent a task from a plain shell:

- Not logged in, after rewake's onboarding and trust keys, Claude Code shows no login screen: it
  starts to its prompt with the auto-mode banner, the footer saying "Not logged in · Run
  /login"; it neither exits nor waits on a dialog. The mod loads and its hello lands: `wake`,
  `turn-boundary`, `telemetry` and `control` live.
- The task is delivered through the socket and announced; the turn ends at once with "Not
  logged in · Please run /login". The task stays in the worker's `unread/` with status
  `delivered`, so a sender's `--awaited` lists it `unread`; the failed turn, with nothing read,
  writes an `error` letter to the worker's own mailbox and no report to the sender.
- A rewake main in the same private HOME, not logged in and run inside the shell, did not get
  its mod's hello within a minute (`wake` alone live) and so opened no space; not pursued, since
  main runs on the person's own login.

Not verified until the workers' directory is logged in: that `/login` in a running worker's pane serves that session without a restart; what a logged-in first start shows
beyond the above; the account, plan and usage against the person's; whether Claude Code keeps
the unknown `rewake` key when it rewrites `settings.json`; the interactive `/resume` picker,
which by construction reads the directory's `projects/`.

## A read outside the working directories

**[verified live, installed 2.1.302, a cheap model, October 23, 2026]** Interactive `claude` in a
pseudo-terminal, `CLAUDE_CODE_NO_FLICKER=1`, a private HOME and a private copy of the workers'
folder (its login, `.claude.json` and `settings.json`) without `hasSeenAutoModeOutsideReadPrompt`,
started in a fresh repository and asked to read a file in a sibling directory. Auto mode is the
default there.

- Without a rule: the dialog "Read outside the working directories", four choices (keep allowing,
  block from now on, no and ask again, yes and ask again), waiting for the person. The owner's
  first choice in the workers' real folder set `hasSeenAutoModeOutsideReadPrompt: true` in its
  `.claude.json` and wrote nothing to `settings.json`.
- With `--settings '{"permissions":{"allow":["Read(//<dir>/**)"]}}'` the same read passed, the
  check's verdict `allow` naming that rule, and no dialog showed.
- What the mod sees of the dialog is in
  [research-claude-mod.md](research-claude-mod.md#a-workers-dialogs-in-auto-mode).

## Where Claude Code reads skills

**[the binary's bundled source, installed 2.1.310, October 28, 2026]** What a brief's skills are
copied from ([briefs.md](briefs.md#skills)). The loader (`lAr` in that build) reads, in order:
the managed folder; the user folder, `$CLAUDE_CONFIG_DIR/skills`, else `~/.claude/skills`; then
`.claude/skills` in the working directory and each parent up to the repository's root (`Gte`,
which walks the parents, breaks at the directory holding `.git` and never reads the home
directory itself); then the `--add-dir` folders, synced skills and plugins. A skill is a folder,
or a link to one, holding `SKILL.md`, named after the folder; `skills/synced/` is Claude Code's
own sync folder, not a skill. The owner's skills are links into a folder of their own, with no
links inside them; the largest, `mods`, is 5.0 MB in 1,695 files.

A worker's read of a brief kept in the room is a read outside its working directories: in 2.1.310
auto mode asks its one-time question there while `.claude.json` lacks
`hasSeenAutoModeOutsideReadPrompt`, as [above](#a-read-outside-the-working-directories), and the
run's `Read(//<room>/briefs/**)` rule lets it through. Not probed live on 2.1.310: a fresh,
logged-in folder needs credentials; the workers' real folder has the flag set already.

## Claude Code's cross-session inbound gate

**[the binary's bundled source, installed 2.1.280; September 23, 2026]** Every line that
reaches a session's inbox socket passes a receive-side gate before it joins the queue. What
it does to rewake's line in a running session is in
[research.md](research.md#the-inbound-gate-on-rewakes-line); this is what the code states.

The setting, in the settings schema's own words: `crossSessionInbound` — "'accept'
delivers them, 'hold' parks them for your review without letting Claude act, 'refuse'
opts this session out. An explicit value always wins. Unset (mode parity): a message
auto-delivers only when the sending session's permission-mode class matches yours
(bypass↔bypass or prompting↔prompting); a mismatched sender's message is held for your
approval; a sender that asserts no class is held only while this session bypasses
permission prompts."

**Which source decides.** `policySettings`, `flagSettings` and `userSettings` are read in
that order and the first that sets the key wins, so a `--settings` layer overrides the
user's file and managed policy overrides both. `localSettings` and `projectSettings` may
then only raise the value along `accept < hold < refuse` ("a repo may only tighten"): a
repository's `hold` beats a `--settings` `accept`. A source switched off by
`--setting-sources` is skipped. An unrecognised value in any file holds everything while
it is present (cause `invalid-setting`). A remote kill switch refuses regardless.

**Unset: parity.** In order:

- the permission-mode getter is not wired yet, or the mode is none of `acceptEdits`,
  `auto`, `bypassPermissions`, `default`, `dontAsk`, `plan` — hold, cause `mode-unknown`;
- the sender is a descendant process of the receiving session — accept (the process
  tree is read only while the receiver bypasses; rewake's wrapper is the session's
  parent, not a descendant, so this never applies to it);
- the receiver's class is `bypass` for `bypassPermissions`, and for `plan` when bypass is
  available to the session (started with `--dangerously-skip-permissions` or
  `--allow-dangerously-skip-permissions`) and it is interactive; every other mode —
  `default`, `acceptEdits`, `auto`, `dontAsk`, `plan` without bypass — is `prompting`;
- a sender that asserted a class is accepted when it matches and held as
  `mode-mismatch` when it does not;
- a sender that asserted none is held as `no-mode-asserted` when the receiver is
  `bypass`, and accepted otherwise.

**Where a class comes from.** On the socket the class is not a field of the line. It is
an attribute of a wrapper around the content: `<cross-session-message from="…"
from-session="…" hop-chain="…" from-name="…" from-mode="bypass|prompting"
from-plugin="…">`, a newline, the body, a newline, `</cross-session-message>`, every
attribute optional. It is parsed with a regular expression and kept only when
serialising the parsed parts gives back exactly the same text. Nothing signs it; the
protocol's own description of `from` reads "Sender-asserted on the socket lane: a label,
not an identity proof", and `from-name` is "A claim, like `from`". `SendMessage` writes
this wrapper with its own session's class. rewake's `<task-notification>` content has no
wrapper, so it asserts no class and no display name.

**Holding.** At most 100 messages are held; the oldest is evicted as expired. The held
set is re-evaluated when the permission mode changes (reason `mode-changed`), when the
settings change (`policy-accepts`), and one message at a time on approval (`approved`);
the screen then says `Released N held cross-session message(s) to Claude's queue
(<reason>)`, the reason drawn as "permissions are prompting again", "crossSessionInbound
now accepts" or "you approved it". The getter is wired as the interface mounts, which
counts as a mode change, so a message held at startup is released with the first of
those texts although nothing was ever in bypass. A released or approved message passes
the inbox guard — rate limit, duplicate, queue cap — once more and can be dropped there.

Only `mode-mismatch` and `no-mode-asserted` get an approval prompt and a deadline:
`dialogExpiry` (`60s`, `5m`, `10m` or `never`; read from trusted sources only, never a
repository file) or `CLAUDE_CODE_USER_DIALOG_TIMEOUT_MS`, five minutes by default; at the
deadline the message is dropped. Every other cause has no deadline: the message waits
for a release, an eviction or the end of the session, and at the end every held message
is settled as expired.

**Receipts.** The receiver reports a message's fate by opening a new connection to the
address in the line's top-level `from`, never by writing back on the connection the line
came on. The address must be `uds:<path>`, and the path a `.sock` file in the same
directory as the receiver's own socket, or a pid-shaped name in a default `cc-socks`
directory. Before writing, the receiver checks that the listening process is the one that
wrote the message — same pid as the writer's peer credentials, same uid, same process
start — and refuses otherwise. The receipt is one line:

```json
{"type":"control","action":"peer_message_status","status":"held","reason":"…","from":"uds:<receiver's socket>","orig_msg_id":"<the message's msg_id>","msgV":1,"msg_id":"<fresh>"}
```

`orig_msg_id` is present only when the message carried a UUID `msg_id`. Statuses: `held`;
`delivered` once a held message is released or approved; `expired`; `denied`; a refusal,
sent as `"status":"expired","status_detail":"refused"`; and `dropped`, with `drop_reason`
and `dropped_msg_ids`. A message accepted outright gets no receipt. The `reason` texts
are fixed per status — `held` always reads "Your message is held for the recipient
user's approval before it reaches their Claude session (permission-mode parity).",
whatever the cause.

**Authentication.** A first line `{"type":"auth","token":"…"}` with the token from
`CLAUDE_CODE_MESSAGING_TOKEN` is optional on Linux and macOS and required on Windows,
where an unauthenticated line closes the connection.
