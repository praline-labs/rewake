# Where the permission-request facts were checked

The facts behind [permission-requests.md](permission-requests.md). The September 29, 2026 record
of the settings hooks `PermissionRequest` on Claude Code 2.1.280 and of Codex 0.157.1, which the
first design rested on, is in git history before October 18, 2026: the build uses neither.

## The live probe on Claude Code 2.1.302, October 18, 2026

An isolated interactive `claude --plugin-dir` in a pseudo-terminal the probe owned, not through
rewake: `--permission-mode manual`, `--setting-sources ''`, `--strict-mcp-config`, a per-launch
settings layer asking for `Write`, a private home with a private copy of the login, a cheap
model; most cases were mod commands without a model turn, three were model turns. No setting of
the person's was changed. Its mod, driver and evidence were kept in the room's scratch directory,
not in the repository.

- **The check.** `tool.check` awaits `next(e)`, the engine's verdict, which runs nothing; a
  genuine ask from the per-launch rule reached it. The 2.1.302 CLI calls the manual mode
  `manual`, not `default`.
- **The question.** A direct `$.tool.call` of `AskUserQuestion` is refused by the host
  (`runs the AskUserQuestion tool: that is $.ui.ask`). `$.ui.ask` opens the engine's own dialog
  as a `tool.call` of `AskUserQuestion` through the mod's hook on it, and resolves to a string:
  the label, labels joined with a comma and a space, or the text under Other. It rejects when
  the hook answers `{ deny }`.
- **Holding.** The hook started the dialog with `next(e)` and awaited successive host-held
  `$.http.fetch` polls: one permission call was held 42 seconds, polls answered pending at 15
  and 30 seconds, with no own-time skip and no fetch cut. No sleep or timer promise was in the
  hook.
- **The first answer.** A remote answer returned as `{ result: { questions, answers } }`, keyed
  by question text, closed the shown dialog, and the pending `next` then settled as a native
  rejection that did not replace the chosen result. A keyboard answer at 2 seconds — Allow once,
  and Refuse — won over a remote answer scheduled for 12, and the native result's event woke the
  host's poll. A model's own `AskUserQuestion` answered remotely reached the model as its
  result, descriptions and all.
- **The deadline.** A shortened 8-second deadline answered `{ deny }`, `$.ui.ask` rejected, the
  check's handler refused, the dialog closed, and nothing was written; the same for a model's
  own write.
- **What it did not test.** Mail, `rewake answer`, the identity check, session allowances, the
  full 120 seconds, restarts, subagents, other modes, races under load. The mode a check runs in
  is read from the harness's classic events (`permission_mode`), as the 2.1.302 declarations
  give it, not seen live.

## pi 1.2.0

No built-in tool-permission gate and no model question tool: the extension examples
`question.ts` and `permission-gate.ts` are not loaded by default; `ui_prompt_start/end` carry a
kind and a title, neither the choices nor a way to answer ([research-pi.md](research-pi.md)).
