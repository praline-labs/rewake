# Where the permission-request facts were checked

The sources behind [permission-requests.md](permission-requests.md), one entry per fact
it rests on.

Checked on September 29, 2026 against the installed Claude Code 2.1.280 binary (its bundled source,
by the minified names), the older Claude Code tree kept as reference (`openagent`), and the
Codex tree at the release commits `be2951ea3` (0.155.1) and `36650394c` (0.157.1); lines
are 0.157.1's unless the two differ. Where the old tree and the binary disagree, the
binary is what runs.

- **The hook's payload** — `executePermissionRequestHooks` (`QCe`) sends the common
  fields, `tool_name`, `tool_input`, `permission_suggestions` and, for an MCP tool,
  `mcp_server`; no `tool_use_id` in either tree (`utils/hooks.ts:4157`), though the help
  text of both claims one (`hooksConfigManager.ts:166`). The common fields add `agent_id`
  and `agent_type` in a subagent.
- **Its answer** — `allow` with optional `updatedInput` and `updatedPermissions`, `deny`
  with optional `message` and `interrupt`; the same schema in both
  (`coreSchemas.ts:875`); `addDirectories` is one kind of `updatedPermissions` entry
  (`PermissionUpdateSchema.ts:68`). A deny's message, or `Permission denied by hook`, becomes the
  tool's error (`PermissionContext.ts:252`, `permissions.ts:453`); 2.1.280 also drops an input-less allow for a
  tool that needs the person (`requiresUserInteraction`).
- **Timeouts** — 600 s by default (`Ia=600000`; `TOOL_HOOK_EXECUTION_TIMEOUT_MS`), the
  hook's own `timeout` in seconds replaces it, with no ceiling in the schema.
- **The race** — hooks run beside the dialog unless `awaitAutomatedChecksBeforeDialog`,
  and a decision counts only if nothing settled the dialog first
  (`interactiveHandler.ts:411`; in 2.1.280 `if(!S)(async()=>{… s.runHooks(…) …})`). The
  flag is set for a background subagent that may show prompts (`runAgent.ts:436`); in
  2.1.280 that is every background subagent of an interactive session.
- **A "no"** — without feedback on the main thread it aborts the turn
  (`PermissionContext.ts:154`, `Kcn` in 2.1.280); a refused call returns before any
  `PostToolUse*` hook (`toolExecution.ts:995`).
- **Auto mode** — the classifier's refusal is a `deny` with reason `classifier`
  (`permissions.ts:903`); `PermissionDenied` hooks get it, with `tool_use_id`, and may
  answer `retry` (`toolExecution.ts:1073`); after 3 refusals in a row or 20 in all the
  harness prompts (`denialTracking.ts:12`, `HN={maxConsecutive:3,maxTotal:20}` in 2.1.280).
  What still prompts: safety checks the classifier may not approve, tools that need the
  person, and the fallbacks when the classifier cannot judge — a transcript too long for it, or,
  failing open, unavailable (`permissions.ts:818-875`).
- **MCP and `WebFetch`** — an MCP tool passes to the rules and asks by default
  (`MCPTool.ts:56`, `services/mcp/client.ts:1814`); `WebFetch` asks for a host neither
  preapproved nor ruled on (`WebFetchTool.ts:112-176`).
- **Codex requests** — `common.rs:1765-1801` in 0.157.1 (`1756-1792` in 0.155.1); the
  decisions `accept`, `acceptForSession`, `acceptWithExecpolicyAmendment`,
  `applyNetworkPolicyAmendment`, `decline`, `cancel`, the same in both (`v2/item.rs:66`,
  `:115`); `cancel` aborts the turn.
- **Codex answering** — first reply wins, any connection (`outgoing_message.rs:467`,
  `:552`); `serverRequest/resolved` (`thread_lifecycle.rs:864`), dismissed in the terminal
  (`tui/src/app/app_server_events.rs:237`); no timeout (`bespoke_event_handling.rs:1924`,
  `:1971`); open requests cleared on turn start, completion and abort (`:157`, `:187`,
  `:1206`); `steer_input` (`core/src/session/turn_input.rs:624`) and the drain before the
  next request (`core/src/session/turn.rs:416`); the decline text
  (`core/src/tools/events.rs:446`).
- **Codex hooks** — hooks before guardian and the person
  (`core/src/tools/approvals.rs:500`, `:493` in 0.155.1); deny wins
  (`hooks/src/events/permission_request.rs`); unsupported fields and the deny message
  (`hooks/src/engine/output_parser.rs:401`); timeout (`hooks/src/engine/discovery.rs:762`);
  trust (`discovery.rs:712`, `hooks/src/config_rules.rs:23`); on by default
  (`features/src/lib.rs:1212`, `:1171` in 0.155.1).
