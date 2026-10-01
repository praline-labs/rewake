# Mail through one tool, outside the sandbox

**Stages 1 and 2 of 3 built and accepted on October 1, 2026.** Stage 1 is the CLI side
([mail-bridge-cli.md](mail-bridge-cli.md), [entry](roadmap/2026-10-01-mail-tool-cli-stage.md));
stage 2 the server, its context endpoint and the adapters
([mail-bridge-server.md](mail-bridge-server.md), [mail-bridge-turns.md](mail-bridge-turns.md),
[mail-bridge-checks.md](mail-bridge-checks.md),
[entry](roadmap/2026-10-01-mail-tool-server-stage.md)); the launch injection, stage 3, is not built,
so no harness starts the server yet. This is the build specification for the mail
transport chosen on September 30, 2026 ([work queue](work-queue.md#now-after-100)). The CLI implements mail
once; a local stdio MCP server runs it outside the shell sandbox. The agent uses one
`rewake` tool with its ordinary CLI words. Reading remains explicit: delivery alone
neither marks a letter read nor creates a report obligation. The owner accepts bounded
continuation calls for a long letter, without searching for its text in files.

The live facts below come from two probes on September 30, 2026: Codex 0.159.0 and
Claude Code 2.1.284. Probe 2 uses an owned app-server and a `--remote` terminal for Codex,
and `-p` new/resumed conversations for Claude Code. Both use a stub, not this unbuilt
integration. Evidence stays outside git, in the repository's ignored `.scratch/`:
`mcp-probe-report.md`, `mcp-probe-2-report.md`, `mcp-probe/` and `mcp-probe-2/README.md`,
with scripts, request logs and captured native events beside them. An observation below
establishes that harness's behaviour on that version, not acceptance of the build.

## One implementation and a narrow surface

The wrapper injects a launch-local stdio server named `rewake`. It advertises exactly
one tool, also `rewake`, taking one argument:

```json
{"words":["send","main-claude","the check needs an owner decision","--notify"]}
```

The server validates the words with the CLI's real parser and a shared capability
predicate, resolves trusted call context, and starts the resolved rewake executable with
that argument array. There is no shell, PATH lookup, command concatenation or stdin
message input. The CLI performs selection, rendering, reads, pending, send, chunking and
receipt recovery. The server transports bounded stdout, stderr, exit code and any
receipt/continuation. It never implements those operations a second time.

| Allowed words | Scope |
|---|---|
| `inbox` | This run's available unread mail, under existing admission checks. |
| `inbox --peek` | Preview only; no read or obligation. |
| `inbox --message ID` | One available unread letter, with existing ID validation. |
| `inbox --owed` | Existing full reread and unread-work count; no new obligation. |
| `inbox --awaited` | This run's sent work, with existing ended/resumable meanings. |
| `inbox --next TOKEN` | Continue this run's frozen output; a new CLI form. |
| `pending TEXT` | This authenticated current turn, with existing role/wait checks. |
| `send NAME TEXT --notify` | A live session in this room; literal nonempty text. |
| `whoami` | This run's identity and observed mail channel. |
| `retry TOKEN` | Reconcile this run's CLI receipt; a new CLI form. |

Help for these forms and `--json` are allowed. Existing inbox modes remain exclusive;
`--next` retains the original mode. Send accepts only `--notify`, `--json` and `--wait`
of 0–5 seconds. It refuses text `-`, `--question`, `--to`, grants and every other
modifier. An empty words array refuses; the executable name is not a word. Extra words
never start another command. Continuation and retry tokens are opaque record selectors,
not paths or authority to another run.

All other commands refuse before a child starts, including launches, plain task send,
edit/withdraw, grants, accept, compact/interrupt, worktrees, administration and internal
callbacks. New CLI commands stay denied until reviewed for this surface. Main gets the
same restriction through this tool; its wider commands remain ordinary CLI calls.
The CLI checks the predicate again in authenticated bridge mode. Argument counts, bytes
and result size are bounded before effects. A letter containing shell syntax remains
literal text; the tool has no arbitrary file, URL, redirection or subprocess facility.

The wrapper keeps its native observer, report publisher and a private context endpoint.
The endpoint provides call correlation and completion evidence, not another mail API.
Observers invoke a private CLI finalizer for durable changes; they do not call MarkRead
themselves. No daemon, sandbox exception or change to the person's configuration is
part of the bridge.

## Identity and the calling turn

The wrapper fixes `REWAKE_DIR`, `REWAKE_ROOM`, `REWAKE_SESSION`, `REWAKE_EPOCH`, the
resolved executable and context endpoint at launch. The server builds child environments
from these saved values, never from words. The CLI rechecks the registry's live epoch.
Native conversation IDs are not rewake epochs, and the server's startup environment
does not prove which conversation or turn made a later call.

Each invocation carries a private wrapper-issued ticket: conversation, turn, native call
ID, boot-clock call time, deadline, normalized-word digest and transport. An inherited
descriptor carries it, with the descriptor number in a server-set environment variable;
the CLI validates it with the wrapper. No model-supplied identity, call ID, timestamp,
path or environment assignment is accepted as authority.

The endpoint is a private Unix socket of the run, 0600 under the state directory's socket
directory ([where it lives](mail-bridge-server.md#what-the-server-knows-at-start)),
with peer-process checks and a per-launch capability. These prevent cross-wiring; same
UID or possession of an environment token alone does not authenticate a native call.
The wrapper matches the exact tool, arguments and native observation before issuing a
ticket. An invented metadata object cannot authorize a call. A person controlling the
harness/state under the same UID remains outside this isolation boundary. Ordinary shell
CLI retains its existing trust model.

### Codex

Join `_meta.threadId`, `_meta.callId` and `_meta.x-codex-turn-metadata.turn_id` with the
owned app-server's `mcpToolCall` item, its enclosing thread/turn, primary binding,
`server`, `tool` and normalized `arguments`. Do not treat MCP JSON-RPC request numbers
as native IDs. Missing or conflicting correlation refuses execution.

**Live, probe 2:** under `--remote`, `item/started` and `item/completed` contain
`mcpToolCall`; the item's `id` equals `_meta.callId`, and the stream supplies `turnId`,
server, tool, arguments and status. The stub is a child of the app-server. A `pending`
call's metadata turn equals the `turn/completed` observed 2.5 seconds later. The
complete result in `item/completed` is not proof of complete model-facing output.

### Claude Code

Match `_meta["claudecode/toolUseId"]` with `tool_use_id` from narrow PreToolUse,
PostToolUse and PostToolUseFailure hooks for `mcp__rewake__rewake`. Take conversation
from `session_id` and turn from `prompt_id`, correlated with UserPromptSubmit and Stop.
PreToolUse is acknowledged before execution; best-effort background telemetry is not
enough. Preserve all the person's hooks. Reject a missing start, conflicting session,
nested agent or ambiguous ordering rather than using the latest observed turn.

**Live, probe 2:** PreToolUse precedes the server call by 6 ms, with matching tool ID,
`session_id` and `prompt_id`; the latter stays equal from UserPromptSubmit through Stop.
`pending` runs eight seconds before that Stop. Metadata alone has no turn ID. This
replaces the draft's plugin-derived turn generation with the hooks' native prompt ID.
The inherited `CLAUDE_CODE_SESSION_ID` is not authority after a conversation change.
Fresh hooks rebind each call; interactive `/clear` and `/resume` remain acceptance cases.

Both adapters apply the existing incoming-mail admission and primary-selection rules.
`--awaited` retains its availability during a launch hold. Pending uses the call's
boot-clock time and authenticated turn, never the long-lived server's process start.
The report publisher orders admitted reads and pending against that turn's completion.

## Injection, approval and an occupied name

**Owner decision, September 30, 2026:** if the person already has an MCP server named
`rewake`, the launch refuses, names that server and explains how to proceed. It never
renames either server or picks another name on its own. The refusal tells the person to
rename or remove their conflicting MCP entry themselves and retry; it identifies the
configuration layer or launch argument where known. Rewake edits none of those files.

Check the effective configuration before injection, using the harness's listing in the
same cwd/environment plus the caller's launch layers/overrides. A disabled entry still
occupies its name. If absence cannot be established, refuse the launch with that reason
and a next step to inspect the effective configuration. A collision is a launch refusal,
not a quiet fallback to shell. Publish no ready session before this check succeeds.

**Live, probe 2:** `codex mcp list --json` and `claude mcp list` see the conflict before
launch. Blind injection silently merges command/args and environment into the person's
server on Codex, and replaces their whole server on Claude Code. An alternate name
worked in the experiment; automatic alternate names are excluded by the owner decision.

### Per-launch settings

For Codex, add only leaves under the absent `mcp_servers.rewake` table: resolved
`command`, internal bridge-launch `args`, explicit launch `env`,
`enabled_tools=["rewake"]`, a bounded `tool_timeout_sec`,
`default_tools_approval_mode="approve"` and `omit_tools_from=["code_mode"]`.
The server offers only this one tool, so its default allowance approves exactly that
tool. Pass these overrides to the owned app-server when deriving its arguments, not
only to the remote terminal. Preserve the caller's `-c` values and precedence outside
this table. Do not replace `mcp_servers`, config files, configuration home, global
approval/sandbox policy, global code mode or global output limits.

**Live, probe 2:** this allowance works under `approval_policy=never`, on new and cold
resumed conversations. With `omit_tools_from`, the model calls rewake directly while
the control servers still use code mode, also after resume. Source at rust-v0.159.0,
`codex-rs/config/src/overrides.rs` and `merge.rs`, explains the recursive dotted-table
merge; the preservation evidence is the live comparison below. Per-tool
`tools.rewake.approval_mode` and `output_token_limit` are not this verified recipe.

For Claude Code, append one `--mcp-config` layer holding only our server and exactly
`mcp__rewake__rewake` to existing `--allowedTools`/`--allowed-tools` values. Preserve
every existing config layer, hook, allowance and denial. Do not add
`--strict-mcp-config`; if the caller already uses it, retain their explicit set and add
ours to that set. Do not change permission mode, ToolSearch, global output/timeout
variables, or persist MCP registration. Explicit denials and managed restrictions are
capability refusals, never policies to override.

**Live, probe 2:** repeated `--mcp-config` and `--allowedTools` both apply under `-p`,
including resume. The person's hook runs beside injected hooks, and their project deny
still hides its tool. Caller-supplied strict mode loads only their explicit server plus
ours. An untrusted project's `permissions.allow` is ignored in `-p` with or without
injection; the probe therefore supplies its control allowance through launch arguments.
Interactive approval behaviour is not yet verified.

### Preservation check

The build repeats this check in disposable configurations/state, keeping effective
values and hashes without secrets. The difference is exactly our server, its one tool
allowance and required observer hooks; a list of names alone is insufficient.

1. Establish a baseline with sentinel servers in normal configuration layers and one
   through caller arguments, plus a disabled server, a denied tool, a person's hook and
   recognizable unrelated settings. Record effective command/args/env, timeouts,
   allowances, denials, active tool set and file hashes; call the enabled sentinels.
2. Inject rewake with an unoccupied name. Require unchanged old entries and unrelated
   settings, identical sentinel replies/denials, no disabled-server start, and our tool
   working without a new approval. Repeat on cold resume and with caller overrides;
   on Claude Code also exercise caller-supplied strict configuration.
3. Put a person's server at `rewake`; require a launch refusal before injection, with
   no replacement, alternate name or persistent write. Exercise explicit policy denial.
4. Compare persistent files byte for byte and retain the effective-runtime comparison.

**Live, probe 2:** Codex controls in project/caller layers keep their entries in all
seven captured `config/read` replies, including timeout 17, disabled server and tool
filter; the pre-existing user-layer server keeps the same digest. Control replies and
an approval refusal persist after cold resume. Claude Code retains project/caller
servers, deny and hook, and the caller's strict explicit set. Recorded user settings
files are unchanged. A shared `.claude.json` changes while the probe is idle or running
only Codex; the probe uses a separate configuration directory for Claude Code. This is
not evidence that every file touched by concurrent sessions remains unchanged. The build
still needs the isolated full comparison and its own conflict-refusal test.

## Bounded output and the read boundary

The CLI limits each chunk to **2 KiB of body and 4 KiB of the complete encoded tool
result**, whichever binds first. The latter includes headers, escapes, stderr, receipt
and continuation; diagnostics are bounded too. No server-side clipping follows a CLI
read commit. Chunking happens before the CLI marks anything read, including for `--json`.
JSON chunks are complete envelopes; UTF-8 characters are never split.

**Live, probe 2, default output settings:** Codex direct results of 46,986 ASCII,
47,083 Cyrillic and 46,985 emoji bytes arrive whole; a 54,000-byte ASCII result shrinks
to 48,024 bytes with its middle replaced, leaving the end marker intact. Claude Code
delivers 60,004 Cyrillic bytes whole; 70,000 ASCII bytes become a 2 KiB preview with a
`<persisted-output>` file. Larger results around 85,000 characters and above produce a
maximum-token error. These are observed bounds, not exact universal thresholds. The
4 KiB cap leaves more than tenfold margin below the smaller observed complete result.

Completion events alone prove neither bound: Codex `item/completed` carries 54,745
bytes for the truncated letter; Claude Code PostToolUse carries 70,914 bytes for a
preview. Seeing END proves nothing about omitted middle bytes. A read acknowledgment
therefore requires a correlated successful direct call **and** its entire result below
the calibrated bound for that output path/settings. Truncation, persistence, errors or
uncertain exposure never acknowledge a chunk. Lower configured limits need a smaller
verified cap; an unverified path is unavailable for tool reads. No global setting is
raised to fit mail. Calibration covers ASCII, Cyrillic, emoji, escapes and dense tokens.

Codex exposes this server outside code mode. Per-call caps cannot bound an outer script
concatenating arbitrary results; inner completion does not prove the script printed them.
If a mode cannot expose our direct/deferred tool under `omit_tools_from`, use fallback,
without disabling code mode globally. Supporting an outer script needs separate verified
output evidence. Model comprehension is not something the bridge claims to prove.

An explicit inbox call freezes available members, versions and text. Each response names
message ID, chunk index/count, byte range, total length, receipt and exact `nextWords`,
such as `["inbox","--next","opaque-token"]`. Partial human output says the letter is
not yet read. New arrivals do not enter that frozen batch; no spill-file search is needed.
The CLI finalizer records each qualifying acknowledgment once and marks a letter read
only after all parts of that version were exposed. There is no public ACK or mark-read
command. Peek, owed and awaited never consume. A completed earlier letter can be committed
while a later letter in the batch still needs parts; incomplete letters create no report
obligation.

Selection and durable changes hold the mailbox lock, not waits between chunks. A durable
claim is shared with CLI read/edit/withdraw, status writes and cleanup. Withdrawal can
win before first exposure; partial exposure is `read_in_progress`, not safely replaceable
unread text. Uncertain exposure retains the claim/receipt for recovery rather than
expiring into permission to replace shown text. Finish with the existing waiter, status
and archive order. Replaying a chunk creates no second read sequence or obligation.

Each admitted operation carries a causal ticket. A turn's end takes its read boundary at
its own event and waits for no call that has not begun to write. An acknowledgment that
meets the end — on record, or captured by the wrapper that runs it — commits nothing,
and one already writing is taken in whole: the boundary is its own snapshot under the
mailbox lock. A pending mark that meets the end's journal marks nothing, and only an
attempt in its own turn marks. So after the end is noted, no acknowledgment that has
not passed its check may begin writing; the acknowledgment already registered is
included through its closing snapshot, and no read by a subsequent mailbox holder can
extend that boundary. No mark is written after the end's journal. The two stage 1 limits that bound this
promise — a turn the server starts before the gateway reads the last completion, and a
turn start that failed to be recorded — are in
[mail-bridge-turns.md](mail-bridge-turns.md#a-turns-end-meets-its-calls). Recording failure or
uncertainty blocks ordinary success for that operation and tells main. Pending counts
the letters a read is still showing among its waits (stage 1).

## Receipts, retries and deadlines

The CLI journals before mutation. The idempotence key is **epoch + native conversation +
native turn + hash of normalized CLI words**. Normalization comes from the parsed call,
preserves literal text, and does not merge commands merely because their effects seem
similar. Native call IDs correlate observations and detect conflicting metadata; they
are not the retry key. Identical words in the same turn replay/join even after success.
To continue a frozen read use its next token; new arrivals can be selected by message ID.
A fresh identical operation belongs to a later verified turn, not a new native call ID,
and only once no operation of those words is unfinished or of unknown effect: it was
decided on October 1, 2026 that such words stop at that operation, as the shell's do.

**Live, probe 2:** retries get new `callId`/`tool_use_id` but the same turn/prompt ID and
words. No native field links original and retry. This supersedes the draft's call-ID key
and its permission to repeat successful identical words within the same turn.

Receipts persist outcomes, progress and stable outgoing IDs across server restart.
`retry TOKEN` resolves the original record in this run; it cannot retarget pending to
another turn. Receipt-aware shell fallback joins the same record. If a hidden token
requires matching words, only an unambiguous unresolved operation with verified scope
can be adopted; uncertain turn correlation refuses a fresh mutation and names recovery.
Different words, conversation or epoch never silently adopt another operation.

Notify journals the outgoing ID before publication, pins the recipient epoch, and
deduplicates under the recipient lock. A tombstone outlives message cleanup so delayed
retry cannot republish a swept notify. Pending replays its recorded outcome without
overwriting a newer mark. Reads resume recorded steps without recreating settled waits.

Each child has a bounded deadline from trusted context, shorter than the transport's
where possible; past it a hard bound kills and reaps it. Check the deadline before commit;
a cancellation drops the answer and leaves the child to that check, and a committed effect
is never called undone. Uncertainty returns its phase and receipt;
never start concurrent fallback on the assumption that timeout means no effect.
**Live, probe 2:** Codex times out at five seconds without cancelling the stub, which
finishes later. Claude Code sends PostToolUseFailure and `notifications/cancelled`; the
stub still finishes its first operation, and the retry is killed when `-p` ends. Both
outcomes require durable recovery.

## Fallback and what main sees

The briefing adds this transport sentence; the worker playbook otherwise keeps one
vocabulary:

> Run `rewake <words>` through the `rewake` tool when you have it, otherwise in the shell.

Use the tool first. Definite absence or a transport refusal before effects permits shell
CLI with the same words. Timeout/disconnect uses the matching receipt-aware CLI to
reconcile first, never an older binary to repeat an uncertain mutation. Semantic or policy
refusals, admission holds and forbidden commands are not failures to route around.
Continuation/retry words address the same journal in either channel.

The wrapper records tool availability, confirmed tool/shell use and failures, exposed
through whoami, existing state reporting and notices to main and the worker. Report
transitions once: tool failure says “tool unavailable; shell fallback unconfirmed”
immediately; successful shell use confirms shell. If both fail, give both diagnostics.
An unobservable shell failure stays unconfirmed, never “both failed”. The wrapper sends
this notice itself; a worker need not send it through broken mail. With no live main,
keep the diagnostic locally for a later main. No automatic relaunch or permission grant.

Probe 1 observed Codex leaving a dead MCP server down and Claude Code restarting it on
the next call. Durable state therefore belongs to the CLI journal, never server memory.
Existing wrappers keep their shell path; new wrappers use a matching bridge and CLI.
Receipts/claims are additive and versioned; old CLI mutation alongside active new claims
is unsupported. Ended epochs cannot replay into replacements. Committed waits retain
existing resume recovery; incomplete reads retain diagnostics through departure without
becoming read. Unsupported harnesses use shell under their existing restrictions.

CLI result codes stay 0 done, 1 target refusal/failure, 2 wrong invocation, 3 accepted but
pending. MCP schema/transport errors remain distinct. Neither pending nor uncertainty
means “not sent”; an incomplete letter never means “read”.

## What stays open before acceptance

- The launch injection and the live checks: stage 3. The context endpoint, the observer
  and both adapters are built and accepted as stage 2, and the CLI predicate, receipts,
  chunk claims and the finalizer as stage 1, all tested in-process; no harness has run
  them yet.
- Repeat preservation and exact-tool approval through rewake itself, including interactive
  Claude Code, cold resume, live `/clear` and `/resume`, nested calls and explicit denials.
- Test the occupied-name refusal against effective configuration and caller overrides;
  establish a reliable preflight for every supported configuration layer.
- Calibrate encoded chunks and lower output limits; verify no read on truncation, preview,
  loss or error. The exact Claude Code boundary between 60 and 70 KB and Codex per-tool
  output-limit behaviour remain unmeasured; neither is needed to raise the conservative cap.
- Exercise real pending at normal end, interruption immediately after pending, compaction,
  late read finalization and overlapping turns; no report may collect a read committed
  after its end's capture.
- Close stage 1's window before the capture: a turn the Codex server starts on its own
  before the gateway reads the last `turn/completed` can commit a read inside that end's
  boundary. It is older than stage 2, which does not widen it; until it is closed, no
  report is promised clean of such a turn's reads.
- Exercise timeout followed by tool/shell retry, Esc, server death during a remote turn,
  restart, and crashes at each receipt/read step; test concurrent CLI read/withdraw and
  replay after report publication. Verify main's notices when either or both channels fail.

These are implementation and verification gates, not unresolved owner choices. The chosen
surface, preserving settings, explicit reads, chunking, fallback and refusal on name
collision are settled. If neither a verified tool path nor shell works, report that
concrete limitation to main without weakening the read boundary.
