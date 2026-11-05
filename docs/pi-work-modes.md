---
description: pi runtime review, advisor and build stances: prompt-only instructions, optional model and effort fields, native/face keys, observed display and compatibility.
---

# pi work modes

A pi session has one runtime stance: **review**, **advisor** or **build**. There is no plan
mode. Modes add short Go-authored instructions to the existing role and work-type briefings
before each model request (`internal/core/workmode/mode.go`). They grant no permissions or
Git authority. Main remains an orchestrator in every mode, even without a writer template.

- Review inspects the requested change against its brief and project rules, reports
  correctness/regression findings with precise paths/lines, and runs relevant requested
  checks. It does not edit the code under review or change Git metadata; rewake coordination
  remains allowed.
- Advisor answers, analyzes and compares with concrete recommendations, without product edits
  or Git metadata changes; rewake coordination remains allowed.
- Build implements the named scope, verifies it, keeps project review/commit discipline and
  reports evidence and assumptions honestly. It does not change the role's authority.

**Owner decision, November 3, 2026:** review/advisor are prompt-only read-only guidance. There
is no tool-call guard, command-safety parser, active-tool restriction or filesystem sandbox.
The mode is not a security boundary. Role keeps mail/reporting and Git authority, work type
keeps task discipline, and mode changes instructions and optional model/thinking only.

## Configuration and startup

The existing user `~/.config/rewake/aliases.toml` and project `.rewake/aliases.toml` accept:

```toml
[pi]
start_mode = "advisor"

[pi.modes.review]
model = "provider/model-id"
effort = "high"

[alias.writer.pi]
start_mode = "build"

[alias.writer.pi.modes.review]
effort = "medium"
```

Unknown fields, modes and effort values refuse the catalogue. Prompts and the three-mode
catalogue are built in, not editable TOML strings. Configured models use exact
`provider/model-id`; effort is `off`, `minimal`, `low`, `medium`, `high`, `xhigh` or `max`,
subject to the selected native model's supported levels.

Global fields merge user then project, field by field; the selected template overlays that
result. Whole-template replacement is unchanged: a project template replaces the user's of
the same name, including its `pi` section. Start precedence is template `start_mode`, then
review/code type defaults (review/build), then merged global `start_mode`, then advisor.
The global fallback covers main and otherwise-advisor launches, never displacing review/code.
Mode-specific template fields overlay globals regardless of where the start came from.
The role is not inferred from the mode.

Initial model/effort precedence is explicit harness flags, starting-mode fields, then existing
launch defaults. Explicit model scope and a model's thinking suffix count as explicit
selections. Later cycles apply only destination fields that are set; an unset field preserves
the current selection, subject to native model capability clamping. Exact registry lookup or
native model-selection failure visibly refuses the switch before changing mode or effort.
An invalid configured startup selection ends the launch rather than continuing with a silent
substitute.

## Keys and observed state

Shift+Tab cycles `review -> advisor -> build -> review`, only idle and not compacting. Busy
requests are refused, not queued for a later turn. Alt+T cycles native thinking; Ctrl+T toggles
thinking visibility. There is no model-callable mode switch and no reviewer-only allowlist.

Native pi uses a documented `CustomEditor` subclass: only focused editor Shift+Tab/Alt+T are
intercepted, with all other editing, paste and rendering delegated to native pi. Dialogs keep
their own keys. Cleanup removes the replacement. This bare-pi integration does not chain a
replacement editor installed by another extension; the last installed factory wins. It does
not change personal keybinding/settings files, install a global raw input hook or draw a
parallel editor. Source evidence is dated in [research-pi.md](research-pi.md#work-mode-native-apis-november-3-2026-140).

Go owns parsing, precedence, instructions, mode destinations, refusal/status text and state
projection. TypeScript resolves/calls native model/thinking APIs, injects the supplied section,
publishes observed values and renders Go's status. A successful native application publishes
`workMode`; planning a switch does not change observed state.

Native status uses `ctx.ui.setStatus`. The face's input rule shows mode at its left; model and
thinking remain on the strip. Its explicit `workmode` input is separate from permission mode,
and keys never reach the pty. Ctrl+T hides/shows the face's thinking rows without changing
thinking effort. Only verified main readers see telemetry: `rewake list` has a separate Mode
cell and optional JSON `mode`, leaving Role and Type unchanged; snapshots and room-view rows
carry the same observed value. Wrapper handover preserves the live current mode, not merely
the configured start value; reconnecting the extension restores that mode's instructions.

## Compatibility

- pi extension wire: **4**; pi adapter handover: **4**.
- Host/wrapper handover: **6** (effective mode configuration and observed snapshot).
- Mirror wire and model handover: **2**, with `WorkMode`, `stance` lines and `workmode` input.
- Face/shell messages: **6**; room-view: **2**, advertised by the executable and required by
  the loaded shell.

Existing build-switch preflight compares the loaded extension/handover contracts. An old
loaded extension is not adopted into this incompatible build: the room needs a restart,
rather than breaking a running old extension. A face of a different contract is refused by
the existing hot-reload handshake, keeping the running face. No stored role/type format changes.
