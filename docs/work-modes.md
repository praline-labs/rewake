---
description: A session's one runtime work mode: its complete behavioral text, where a mode is defined, how it is selected, model and effort per mode, runtime switching and observed state, memory, and the contracts it touches.
---

# Work modes

A session has a role and one runtime work mode. The role owns orchestration, reporting and Git
authority; the mode owns behavioral and task discipline. Model and thinking level are selections
attached to a mode, not another behavioral axis. A mode's text is the **one** behavioral section a
session reads: a switch replaces it, and nothing is stacked beside it.

The owner's decision, November 3, 2026: review and advisor are prompt-only read-only guidance.
There is no tool-call guard, command-safety parser, active-tool restriction or filesystem sandbox,
and no plan mode anywhere. A mode is not a security boundary. Project instructions and the brief
still take precedence, and main remains the orchestrator in every mode.

## The built-ins

rewake ships four, as complete texts at `internal/core/workmode/builtin/<mode>.md`, embedded in the
build and naming no model, provider or harness:

- `advisor` — investigate a question and answer it with evidence; change nothing it studies.
- `build` — implement a scoped change and verify it, keeping the project's review and commit
  discipline; a mode grants no Git authority.
- `review` — report verified defects with precise files and lines; do not edit what you review or
  change Git metadata.
- `process` — carry out an authorised procedure and verify the state it leaves; a state-changing
  procedure is neither advice nor necessarily code, which is why it is its own mode.

Each text says what to read first, how to work, when the work is done and what the report holds,
and each defers to the project's own instructions. The owner's, October 21, 2026: a text names or
assumes no other mode or worker, only this worker, main, the brief, the project and the subject of
the work; `process`, `review` and `advisor` read right whether the subject is code, a document,
data, a configuration or a procedure, and `build` assumes no language, test framework or version
control workflow. Rewake's text is a page, not a manual: a file over 32 KiB is refused.

## Where modes come from

The nearest of one name wins:

1. the project's `.rewake/modes/<mode>.md`, in main's working directory;
2. the person's `~/.config/rewake/modes/<mode>.md`;
3. rewake's built-ins.

A nearer definition of a built-in's name replaces its text and keeps its built-in place in the
catalogue's order. A file is Markdown opening with a TOML front matter between two `+++` lines:

```markdown
+++
description = "Answer a question about the code; change nothing."
+++
Read first: …
```

`description` is required and one line; another field is refused, as is an empty text, a name other
than letters, digits, `.`, `_` and `-`, and a file that does not read. A file that does not read
refuses the catalogue, naming it, as an alias file does: a mode silently absent would raise a
worker other than main asked for. Model and effort are **not** in the Markdown; they are alias
TOML, below.

`rewake workers` lists the modes in catalogue order after the templates, each with its source, path
and description; `--json` carries them as `modes`.

## The old worker types

The owner's decision, November 3, 2026: the separate worker type is gone. What it carried is the
work mode. Old syntax in the files is skipped with a migration notice saying where it goes, never
read as if it were a type and never rewritten by rewake; a typed `--type` or `--no-type` is still
refused, since a call can be made again and a file cannot be rewritten by main's first launch:

| Old | New |
| --- | --- |
| `type = "code"` in an alias | `[alias.<name>.pi] start_mode = "build"` |
| `type = "analysis"` | `start_mode = "advisor"` |
| `type = "review"` / `"process"` | `start_mode = "review"` / `"process"` |
| no type | no `start_mode`: the merged global one, else advisor |
| `.rewake/types/<name>.md`, `~/.config/rewake/types/` | the same file under `modes/`; a custom `code.md` or `analysis.md` needs a name of its own, its text kept |
| `space open --type <type>`, `--no-type` | `--work-mode <mode>`, or `start_mode` |
| `[types] off = [...]` | removed: built-ins are not turned off; a `modes/<name>.md` of a built-in's name replaces its text |

A `types/` directory, an alias `type` and a `[types]` section are each skipped with that guidance,
even when new syntax is present beside them: refused, they kept every launch in a project that
still carried one from starting, so main could not start to be told what to fix (met on
November 11, 2026). A template whose `type` is skipped is left out until it is fixed rather than
started under advisor or another default — read under a default it would work as something its
person did not write — so `rewake workers` lists it in no template and `space open --alias` refuses
it with the notice. The rest of the catalogue runs, and a launch carries the notices into the
session's briefing, where main reads the file, the line and what to write.

Only the old shape is skipped: a `type` holding a string, and a `[types]` section holding
`off = ["<name>", ...]` and nothing else. Any other value or field there is malformed and refuses
the file, as malformed TOML, an unknown field and a malformed current value do — recognizing an
obsolete key never swallows its value. A definition is checked against the whole current schema
before it is left out, at the catalogue and again when `rewake workers` lists or `space open` asks
for it: an obsolete `type` may leave a valid template out, never screen a broken `start_mode`, a
mode's model or effort, or a launch setting beside it.

The two alias files still replace one another whole, so the definition in force decides: a valid
project definition replaces an obsolete user one, and an obsolete project definition hides the
user's valid one until it is fixed. Only the definition in force is left out and named in a notice;
the other's type is not in force and is not reported. Personal configuration is never rewritten.

## Selecting the start

`rewake space open <name> --alias <alias> --work-mode <mode>` raises the worker in that mode.
Otherwise the template's `[alias.<name>.pi] start_mode` decides, then the merged global
`[pi] start_mode` (user then project), then `advisor`:

```toml
[pi]
start_mode = "advisor"

[pi.modes.build]
model = "provider/model-id"
effort = "high"

[alias.writer]
harness = "pi"
role = "write"
[alias.writer.pi]
start_mode = "build"
[alias.writer.pi.modes.review]
effort = "medium"
```

Type-derived defaults are gone: a template with no explicit mode starts in advisor. Unknown
fields, a `start_mode` or a `[pi.modes.<name>]` the catalogue does not hold, and an effort other
than `off`, `minimal`, `low`, `medium`, `high`, `xhigh` or `max` refuse the catalogue; a configured
model is an exact `provider/model-id`. `plan` is refused by name: there is no plan mode. Start
precedence is `--work-mode`, template `start_mode`, global `start_mode`, advisor; the role is not
inferred from the mode. `space open` resolves the mode in main's directory and passes its name and
complete text to the worker's launch as `REWAKE_SPACE_WORK_MODE` and
`REWAKE_SPACE_WORK_MODE_TEXT` beside `REWAKE_SPACE_TEMPLATE` ([space.md](space.md)); the launch
unsets them.

## Model, thinking and switching

Initial model/effort precedence is explicit harness flags, the starting mode's fields, then
existing launch defaults. Explicit model scope and a model's thinking suffix count as explicit
selections. Later switches apply only the destination mode's fields that are set; an unset field
preserves the current selection, subject to native model capability clamping. Exact registry
lookup or native model-selection failure visibly refuses the switch and restores the prior usable
model/effort before admitting work; if restoration fails, the session ends rather than running a
mixed state. An invalid or busy initial selection ends the launch instead of silently discarding
its instructions.

The mode is a launch decision: `--work-mode`, or the template's `start_mode`, never a key of the
person's. The runtime switch machinery stays for the wire (`mode-cycle`, and the face's
`workmode` input) and is kept compatible, but no key reaches it. A switch applies only idle and
not compacting, in catalogue order: review, advisor, build, process, then the custom-only names
sorted by name, then back to review. An overridden built-in keeps its built-in position. Busy
requests are refused, not queued for a later turn.

Shift+Tab is pi's own thinking key and stays so in rewake: natively it follows
`app.thinking.cycle`, and the face sends its thinking input, which moves the level as Alt+T does.
Alt+T cycles native thinking; Ctrl+T toggles thinking visibility. There is no model-callable
mode switch and no reviewer-only allowlist.

Turn admission starts closed until initial instructions and native selections commit. Each mode or
thinking application closes it before its first native mutation, through commit or restoration.
Native input and run preparation await it; face prompts wait without being submitted early. Wakes
receive the existing pending receipt for the host to retry. Remote compact/interrupt requests
refuse with `busy` while a selection is changing; thinking cycling need not wait for an idle turn,
but refuses while selection or compaction is in progress.

Native pi uses a documented `CustomEditor` subclass: only focused editor Alt+T is intercepted,
with all other editing, paste and rendering delegated to native pi; Shift+Tab is left to pi's own
`app.thinking.cycle`. This bare-pi integration does not chain a replacement editor installed by
another extension; the last installed factory wins. It does not change personal
keybinding/settings files, install a global raw input hook or draw a parallel editor. Source
evidence is dated in
[research-pi.md](research-pi.md#work-mode-native-apis-november-3-2026-140).

## The native boundary

Pi 1.5.2 has no native named build/plan/work-mode registry: its `--mode` selects text, json or rpc
output, not a work mode, and it is not used for one. All rewake modes are equal extension-owned
modes using the existing Go-owned plans, prompt sections, model/thinking APIs and focused-editor
integration; there is no native mode emulation and no loaded plan-mode example. Go owns parsing,
the catalogue, precedence, prompt text, mode destinations, refusal/status wording and state
projection. TypeScript resolves and calls native model/thinking APIs, injects the supplied
section, publishes observed values and renders Go's status. Its `Modes` module owns
initialization, selection/restoration and admission privately; callers use initialization, the
retained wire cycle, thinking, face prompt, wake and selection-side control operations, plus the
observed mode name.
A successful native application publishes `workMode`; planning a switch does not change observed
state.

Native status uses `ctx.ui.setStatus`. The face's input rule shows the observed mode at its left,
`◆ build`, with no cycle hint, since the person chooses it at launch; model and thinking remain on
the strip. The explicit `workmode` wire input is separate from permission mode and retained for
compatibility, but no key sends it. Keys never reach the pty. Ctrl+T hides/shows the face's
thinking rows without changing thinking effort. Only verified main readers see telemetry:
`rewake list` has a separate Mode cell and optional JSON `mode`. Snapshot and room-view rows
carry the same observed value. Wrapper handover preserves the live current mode, not the
configured start value; a reconnect keeps an already applied mode, and a new extension instance
receives the observed mode's instructions in its hello.
A harness that cannot switch at runtime reads the mode's section in its launch briefing instead;
pi's launch briefing never carries one, so a switch cannot leave two contradictory sections.

## Memory

Main's decision, October 21, 2026: a Claude Code worker keeps the memory of its work mode in that
project, `~/.config/rewake/claude/worker/memory/<project>/<mode>/`, whatever template opened it
([worker-profile.md](worker-profile.md)). Nothing is moved automatically; a mode's map is where the
old type's was when the names agree. A template named like a mode shares that mode's memory
directory — a known limit, left until it bites.

## In `rewake list`

`rewake list` shows the harness's observed runtime work mode in a `Mode` column, only for a reader
who proved to be main's shell and only when some session reports one; `--json` carries it as
`mode`, omitted when empty. The worker type column is gone.

## Compatibility

- pi extension wire: **4**; pi adapter handover: **4**.
- Host/wrapper handover: **7** (the runtime work mode and the whole resolved catalogue travel
  with their texts in place of the worker type).
- Space host handover: **2** (the worker's mode and its text in the spec).
- Mirror wire: **3**, `workMode` a mode name rather than a three-name enum.
- Mirror's handed model: **2**, its shape unchanged by this change.
- Face/shell messages: **7**; room-view: **3**, advertised by the executable and required by the
  loaded shell.

A build-switch preflight compares the loaded extension/handover contracts: an incompatible loaded
extension or space host requires a room restart. A face of another contract is refused by the
existing hot-reload handshake, keeping the running face. New space records omit the worker type;
old records still read, their obsolete field ignored, and no state migration is needed. Existing
sessions are not restarted or rewritten.
