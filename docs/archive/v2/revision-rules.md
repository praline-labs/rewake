# Revision: the numbered rules

Which rules of the 1.x documents survive into 2.0. A rule that survives is restated in
the 2.0 rules documents of stage 2, worded without a harness where it binds every
adapter; a rule that goes is left in the archive of 1.x with the document that holds it.
The overview is in [revision.md](revision.md).

Three reasons make a rule go: the 1.x migration (decision 3), the Claude Code MCP
injection (decision 1), and an exact harness version that 2.0 no longer supports.

## mail-bridge-cli.md, rules 1–8

M1's rules on effects, in `docs/mail-bridge-cli.md` ("The rules the code holds"); the
clauses of 7 and 8 are named in `docs/turn-end-recovery.md:5-12`.

| Rule | Verdict | Notes |
|---|---|---|
| 1 an unknown effect is neither dropped nor bypassed | carry over | |
| 2 a lock is released only by its holder | carry over | |
| 3 the check that decides an effect runs inside the critical section, again on a retry | carry over | |
| 4 the completion of a read is one durable fact | carry over | |
| 5 the size bound is on the final bytes, in one place | carry over | |
| 6 a check has three outcomes: found, proven absent, unknown | carry over | |
| 7-identity, 7-clock, 7-journal | carry over | |
| 7-scope | change | the row "a hook without an event (Claude Stop/StopFailure, once)" becomes the mod's turn events; the row "an id without a boundary (Codex notify through `turn-ended`)" goes, rewake installs no notify |
| 8-evidence, 8-stop, 8-proof, 8-retention | carry over | "earlier build" leaves the evidence table |
| 8-reconcile | change | steps 2 (conversion), 3 (successor) and 5 (held) go |
| 8-withheld | drop (1.x migration) | an unknown without evidence arises only in conversion |
| 8-settle, 8-cutover, 8-origin | drop (1.x migration) | |

## mail-bridge-server.md, rules 1–11

The server rules in `docs/mail-bridge-server.md` ("The rules"). They are written for
the Codex MCP server, but what they guarantee is not a property of stdio MCP: it binds
every tool, the mod's included, whatever its transport. Stage 2 chooses the mechanism —
the wrapper endpoint with tickets, or something else; the guarantees below are carried
and not open to that choice. The column "binds the mod" names them.

| Rule | Verdict | Binds the mod |
|---|---|---|
| 1 the server is a transport; it decides no mail | carry over, `tool` | yes: the mod decides no mail either |
| 2 a call runs only under a ticket the wrapper issued after it saw the same call | carry over, `tool` | yes, as its guarantee: a trusted binding to the call, conversation and turn; the model's arguments are never the source of authority. A ticket is one way to hold it |
| 3 every effect happens in the child, under its receipt; the server never repeats | carry over, `tool/mcp` | the receipt part, yes; the child is the `rewake` process either way |
| 4 a call is bound to its operation before its first effect | carry over, `tool` | yes |
| 5 everything read and written is bounded, in one place each | carry over, `tool` | yes; the mod's own result bound is still unmeasured |
| 6 a part is read only on its call's own answer | carry over, `tool` | yes: a letter is read only on proof that the call's own whole result reached the model, within the effective limits (`cli/read_ack.go:17-37`) |
| 7 a call's commits stop at its turn's end | carry over, `tool` | yes; the end comes from `turn.complete` |
| 8 an end's boundary is a cut between commits | carry over, `tool` | yes |
| 9 every wait has a bound, and nothing waits holding what it waits for | carry over | yes |
| 10 one build per run | change: one build per room, with no cutover behind it | yes |
| 11 a failing tool never weakens the mail; the shell takes the work | carry over | yes |

**A child process is not a way round 6–8.** `rewake inbox` marks a letter read right
after writing it to stdout (`cli/inbox.go:114-119`). Under the mod's `tool.call` that
stdout is not yet shown to the model: the mod can fail before it returns the result,
and a large result reaches the model as persisted output with a 2 KB preview (seen on
2.1.289, `.scratch/v2-recon/claude-mods.md:52`). The letter would be read by the records
and never seen, and a task would owe a report nobody read. So whatever transport stage
2 picks, a mod's reading tool that cannot yet prove its whole result reached the model
refuses before its first effect; the explicit shell call stays the fallback it already
is. The fault cases that hold this carry over to the mod
([tests](revision-tests.md#generated-spaces-fault-tests-and-tables)).

The call sequence in the same document (steps 1–8: call, ticket, match, child,
authorization, command, answer, observer) is Codex-only. Step 1 changes with decision 9:
a `tools/call` names one of the tools and carries its own arguments, not
`{"words": [...]}` for one tool `rewake`.

## mail-bridge-launch.md, rules 1–8

The injection rules in `docs/mail-bridge-launch.md` ("The rules").

| Rule | Verdict | Notes |
|---|---|---|
| 1 the tool is added, and nothing else changes | carry over, every adapter | the mod is loaded for one launch by `--plugin-dir`; the person's configuration is never edited |
| 2 the name is proven free, source by source, before the run is published | Codex-only, change | G2 adds plugin manifests and `.mcp.json` as a source; the Claude sources (`--mcp-config`, parent `.mcp.json`, `mcp get`, managed MCP) go |
| 3 rewake approves exactly `mcp__rewake__rewake` | change | Codex approves the set `mcp__rewake__<name>` of decision 9; on Claude Code a tool the mod answers in `tool.call` needs no approval |
| 4 the server is injected only where it can work | Codex-only | the mod's equivalent — tools registered only when the wrapper is reachable — is a stage 2 rule |
| 5 the limits that decide a read are the harness's effective ones | carry over as a guarantee, every adapter; the mechanism Codex-only | the Codex measurement stays Codex's; the Claude MCP output limits (G8) go; the mod's effective result limit is a probe question (L5 below) |
| 6 checks run the person's programs only as the harness would, bounded | Codex-only | |
| 7 diagnostics say where, never what | carry over, every adapter | |
| 8 nothing persists past the run | carry over, every adapter | the per-launch mod directory included |

"The launch, in order" (steps 1–7) is Codex-only, without the Claude section.

**Gates.** G1, G2, G3, G4, G9 and S1 are Codex's and stay until stage 2 replaces exact
versions with minimums (G2 becomes the manifest reading of decision 4). G5, G6, G7 and
G8 are Claude Code's MCP injection and go. L4 (the native signal of a policy denial)
and L5 (output limits that deliver a 4 KiB result whole) keep their Codex halves; the
mod needs its own answer to both, which is a probe question.

Open for stage 2: a person's own Claude Code MCP server named `rewake` would show its
tools to the model as `mcp__rewake__<tool>`, the same prefix as the mod's tools. Whether
the two can collide, and what the launch does then, is not checked.

## mail-bridge-channel.md, rules 1–8

The channel record rules in `docs/mail-bridge-channel.md` ("The rules"). The record is
core in 2.0 (a mod's tools can fail to load as an MCP server can), so the rules carry
over, worded for any tool transport.

| Rule | Verdict |
|---|---|
| 1 two observations and a block, one derived display | carry over |
| 2 connected is not working | carry over; the mod's hello replaces the MCP server's |
| 3 silence proves nothing | carry over |
| 4 each channel has one kind of evidence | carry over |
| 5 events are ordered by when they happened, not by when they were folded | carry over |
| 6 a notice states an action first and fits the preview | carry over |
| 7 a policy refusal is not routed around (owner, October 4, 2026) | carry over; it covers a managed policy that keeps mods off |
| 8 nothing is relaunched, granted or approved by the channel | carry over |

The Claude rows go: the hello timer from the first session start, the call seen at
`PreToolUse`, "server gone" and the `/mcp` reconnect notice. The Codex conversation
rows (`mail-bridge-channel-codex.md`) are Codex-only; the failure table
(`mail-bridge-channel-failures.md`) is redone per adapter.

## turn-end-recovery.md

Carry over: "The operation" (without the third row of its table), "The read clock",
"Pending marks", "The interim record", "Reconciliation" steps 1, 4 and 6, and in "Every
record and state" the journal, `kept.json`, the pending mark, the interim record, the
once marks and the wait record. Drop (1.x migration): "The stop and `rewake settle`"
except the definition of the stop, "The cutover", the earlier-build receipt table, the
conversion journal, the run and successor records, and the open items about earlier
builds, held reports, the settle gate and the cutover.

## mailbox-records.md, turn-outcomes.md, mail-bridge-turns.md

- `mailbox-records.md`: carry over "The list" without `turns/*` and
  `journal/conversion`, "The plan and the seam", "The tests that hold it", "The stop on
  record"; drop "Notes owed to main", which rests on moot for held reports, and the
  conversion paragraph.
- `turn-outcomes.md` has no numbered rules. "Failed turns" and "Interim turn ends" carry
  over. "Confirmation on Claude Code" carries over as semantics: an interim wake after
  `pending` that ends without a new mark must not close the task early
  (`cli/turn_hold.go:16`; held by `cli/turn_hold_test.go:80,122,149` and
  `test/workflow/pending_confirm_test.go:25` with its negative controls at 177 and 181).
  Its mechanism moves to the mod's `classic.Stop`, which fires before `turn.complete`
  even without a settings hook and may answer `block` with a reason, re-prompting the
  turn (types of 2.1.289: `.scratch/v2-recon/cc-2.1.289.d.ts:1161,1205,1285,1313`;
  `StopHookInput.stop_hook_active` at 11565; the order seen live,
  `.scratch/v2-recon/claude-mods.md:115`; `plugin.js:289` already listens to it). The
  types allow it; that the whole chain works is not yet shown live (stage 4 probe).
  "Keyboard stops" is rewritten on `turn.complete` with `isAborted`.
- `mail-bridge-turns.md`: the rule that no commit passes its turn's end carries over;
  "The turn a call belongs to" (Claude `prompt_id`, the Stop hook) is rewritten with the
  `turnId` of `turn.start` and `turn.complete`; the Codex part carries over.

## Other numbered lists

- `remote-control-codex.md`, the three safety properties (never a compaction into a
  running or accepted turn; no outcome lost silently; no task closed by a compaction
  turn): Codex-only, carry over.
- `grants-authority.md`, the three steps of the unforgeable grant (on send, on
  delivery, when main does not answer): carry over.
- `design.md`, "Owner decisions, September 16, 2026": carried into the 2.0 design with
  their dates; the decisions of October 5, 2026 are added beside them.
- `conversation-reset.md` and `work-queue.md` number open questions and decisions, not
  rules; the first is redone for the Control capability, the second goes to the archive.
- `docs/legacy.md`: every current mark goes (decision 3), and with them the table of
  marks and of oldest supported versions. Whether the mark's rules and
  `docs/legacy_test.go` stay as a mechanism with an empty table, ready for the next
  minimum-version raise, is a proposal to the owner ([revision.md](revision.md#proposals-for-the-owner)).
