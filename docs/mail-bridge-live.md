# Live checks of the launch injection

The plan of the live checks of stage 3 of [mail-bridge.md](mail-bridge.md): where they
run, how each case sets its conditions, how preservation is proven without touching the
person's configuration, and which gates of [mail-bridge-launch.md](mail-bridge-launch.md#gates)
and [mail-bridge-channel.md](mail-bridge-channel.md) each case closes. **Plan, October 4,
2026, run the same day** on build 916f888 — the results are in
[the run of October 4, 2026](#the-run-of-october-4-2026), the facts it learned about the
harnesses in [research-mail-tool.md](research-mail-tool.md).

## Where they run

The owner decided on October 4, 2026: the live checks run in the owner's own logged-in
harnesses, not in disposable logged-in homes, and the user configuration stays
read-only.

- **A folder per case**, `.scratch/m3-live/<case>/`, made its own git root with `git
  init`, so each harness takes it as a project root of its own: Codex's root marker is
  `.git`, Claude Code keys trust on the git root.
- **A state directory per case** under that folder as `REWAKE_DIR`, launches through
  `env -u REWAKE_SESSION -u REWAKE_EPOCH`, a tmux server of its own (`tmux -L m3live`),
  and a main of the case in the same state directory when the case needs one. The
  owner's sessions are never touched.
- **Instructions above the folder still load.** Claude Code reads every `CLAUDE.md` from
  the cwd up, inside another repository too, so this repository's instruction files
  reach a case folder; the caller's `--settings` of every Claude Code case carries
  `claudeMdExcludes` for them. Codex reads `AGENTS.md` up to its project root, which the
  folder's own `.git` stops.
- **Cost.** A cheap model and short messages; the owner is warned before Codex runs,
  which spend subscription quota. Checks with no model turn run in a scratch home as on
  October 1, 2026, and need no login.

## Where a case's conditions go

Never in the person's user configuration. A server named `rewake`, a deny rule, a limit,
an approval default go into the case folder or the caller's flags:

- **Claude Code**: the caller's `--settings <case>/settings.json` — `env`, permissions,
  hooks — and `--mcp-config <case>/servers.json`. The folder's `.claude/settings.json`
  is not used: in an untrusted folder its MCP approvals are ignored (Claude Code 2.1.196
  and later, documented), and trusting the folder writes `~/.claude.json`. A `.mcp.json`
  in the folder serves the name cases, since `mcp get` reports it approved, pending or
  rejected (*verified* October 1, 2026). An interactive case may answer the folder's
  trust dialog; that writes `hasTrustDialogAccepted` in the folder's own entry of
  `~/.claude.json`, the one trust field the comparison below allows to change.
- **Codex**: the folder's `.codex/config.toml`, trusted for that one launch by the
  caller's `-c 'projects={"<absolute path>"={trust_level="trusted"}}'`, or the caller's
  `-c` itself. The path goes in the value: Codex splits a `-c` key on every dot and keeps
  the quotes, so the dotted form `projects."<path>".trust_level` never names the project
  and the app-server then writes the trust itself (**S1**, October 4, 2026). An untrusted
  folder still serves the name cases: the check refuses a layer that is off.
- **Codex writes trust itself.** Its app-server makes a project trusted in its own
  configuration home when a thread request names a cwd, the project has no trust
  decision, and the effective permissions may write the cwd — `workspace-write` is
  enough — and it goes on with trust in memory if that write fails (*source*,
  `thread_processor.rs` 1363–1414 at `rust-v0.159.0`). In the owner's login that home is
  `~/.codex`. So no Codex case sends a cwd for a folder without a trust decision: rewake
  launches only, whose terminal names a cwd only when given `-C` or `--cd`
  (`thread_cwd_from_config`), never `-C` into an untrusted folder, and no plain `codex`
  in a case folder — its embedded terminal always names the cwd; baselines go through
  `rewake --no-mail-tool`. A case that needs a project layer active runs only once S1
  has shown, in a scratch `CODEX_HOME`, that the `-c` trust leaves that home's
  `config.toml` byte-identical under exactly these conditions. If S1 fails, such a case
  stays unrun; moving it to the caller's `-c` proves nothing about a trust write the
  app-server makes on its own.

## Proving preservation

**Without a live harness.** For generated combinations of the caller's arguments —
allowances under both spellings or none, `--strict-mcp-config`, a caller's `--settings`,
`--mcp-config` as files and inline, several values and repeated flags, `-c` in both
spellings and under other tables, `--command`, aliases and defaults, `--` — the plan's
arguments equal those of the adapter without stage 3, which already merges `--settings`
and adds its own flags, plus exactly the additions of
[mail-bridge-launch.md](mail-bridge-launch.md); the merged settings equal that adapter's
plus our hook entries. A launch against a fake harness writes nothing outside the state
directory: the configuration files of a temporary `HOME`, `CODEX_HOME` and
`CLAUDE_CONFIG_DIR` stay as they were. Every diagnostic surface is searched for sentinels
planted in env values, inline configuration, `-c` values and a check's output.

**Live**, the person's user configuration is read only, and a real harness keeps runtime
state, trust and history of its own; so the invariant is that rewake edits no
configuration, and the effective values, replies and denials of baseline and injected
runs are compared as well as the files. Before and after each run, the case records
digests, never contents (an `env` may hold secrets):

- **Byte for byte**: `~/.claude/settings.json`, `~/.claude/settings.local.json` when
  present, `~/.codex/config.toml`, and the case folder's own configuration files —
  rewake edits none of them either. When `CLAUDE_CONFIG_DIR` is set, a harness started
  from that shell inherits it, and its `settings.json` and `.claude.json` are the ones
  under test as well.
- **`~/.claude.json` as a whole, less named runtime fields.** The harness rewrites it
  itself — `pluginUsage` because of the `--plugin-dir` every rewake launch passes
  ([claude-plugin.md](claude-plugin.md)), a projects entry for each new folder,
  counters, caches — so it cannot stay byte-identical. The whole JSON is compared after
  removing an explicit list of runtime fields; a field that appears, disappears or
  changes outside the list is a difference, in every project's entry, the case's own
  included. The list is built from the diff of baseline runs (`--no-mail-tool`) on the
  version under test, and each field on it is named with its reason in the case record.
  The candidates known now: top-level `numStartups`, `pluginUsage`, `skillUsage`, the
  tip and nudge counters, and the `cached…` and `…Cache` fields the harness refreshes
  from its servers; per project, the `last…` statistics of the last session and
  `exampleFiles` with its time.
- **Never on that list**, in any project's entry: `mcpServers`, `disabledMcpServers`,
  `enabledMcpjsonServers`, `disabledMcpjsonServers`, `allowedTools`, `mcpContextUris`,
  `hasClaudeMdExternalIncludesApproved`, and the top-level `mcpServers`. The one trust
  field allowed to change is `hasTrustDialogAccepted` in the case's own entry, and only
  when the case answered the dialog.
- **The case's own entry** did not exist before its first run, so it is compared with
  the entry a baseline run creates in a twin folder: equal once the listed runtime
  fields are removed. The baseline shows where a runtime write comes from; it never
  admits a value of its own under a compared key.
- **The comparison is tested on fixture pairs**, not on the person's file: an allowance
  or a server entry added to the case's own entry fails it even when the baseline added
  that entry too; a counter, a cache or `pluginUsage` changing passes. It prints field
  paths only, never values.
- The owner's own sessions write these files too. A difference is re-run in a quiet
  moment, never explained away.

## What only the user layer can show

Project and caller layers add to the person's configuration; they cannot remove from
it. So these stay off the live runs:

1. **A `rewake` in user scope**, on either harness, and Claude Code's local scope, kept
   in `~/.claude.json` by directory: the name check for them stands on the scratch-home
   runs of `mcp get` and `config/read` of October 1, 2026 and on the fixture.
2. **Managed layers**: G6 in a disposable container, where the system path can be
   written; G4 from the source.
3. **The default interactive approval**: the case "our tool runs without a prompt"
   means something only when the person's user settings hold no rule naming
   `mcp__rewake__rewake` or a pattern covering it. The case reads them, read-only, and
   records the answer with the result; when they hold one, that part stays unproven.

## The cases

Before the run every gate was open, so no launch carried the tool: a case that needed it
— all but S1 and the gates' own checks — launched with `REWAKE_GATES_ASSUMED` naming the gates
it stands behind ([gates taken as closed](mail-bridge-launch.md#gates-taken-as-closed)),
and its record says which. Since the run, Claude Code 2.1.284 carries the tool with no
assumption (G7 closed); Codex still needs G2 assumed. A gate a case closes is entered in `closedGates` for the
harness version it ran against, and later cases on that version need not assume it. A
launch takes its harness's version as [the harness
version](mail-bridge-version.md) says, and an unknown one keeps every
gate open: a case on Claude Code that needs the tool runs with the native install's
`claude` on `PATH`, `--command` or not.

| # | Harness | What | Conditions through | Turns |
|---|---|---|---|---|
| S1 | Codex | `-c` trust skips the prompt, and a `thread/start` naming the folder as cwd under `workspace-write` leaves `config.toml` byte-identical | caller `-c`; a scratch `CODEX_HOME`, app-server level | 0 |
| L1 | both | preservation, new conversation: control servers, a disabled one, a denied tool and a person's hook keep their entries, replies and denials; only `rewake` is added | folder `.codex/config.toml`, caller `-c`; caller `--settings` and `--mcp-config` | Codex 1, Claude 1 |
| L2 | both | the same on cold resume, and with the caller's `--strict-mcp-config` | as L1 | Codex 1, Claude 1 |
| L3 | both | an occupied name refuses before anything is published, naming the layer: project on and off, caller `-c` and `--mcp-config` (second file, repeated flag, `=`), a thread in another folder, a thread request whose `config` names `rewake`, adds a leaf or lowers the output limit — refused, while the terminal's ordinary keys pass; secret sentinels in no output | folder, caller flags | 0 |
| L4 | both | our tool runs with no prompt under each Codex approval policy and Claude Code's default mode; a deny of `mcp__rewake__rewake` gives the native signal, the block, one notice to main and no shell advice; the block outlives a disconnect and a hello | caller `-c`; caller `--settings` deny | Codex 3, Claude 2 |
| L5 | both | a read's bytes in the harness's record equal the child's; the exact 4 KiB bound; the smallest allowed output limit and the nearest refused one | caller `-c`, caller `--settings` `env` | Codex 2, Claude 2 |
| L6 | both | `pending` and the turn's end; Esc right after `pending` | — | Codex 3, Claude 3 |
| L7 | both | observation before call over 20 calls and Codex's parallel calls | — | Codex 2, Claude 1 |
| L8 | both | a nested agent's call and the hook field that names it | — | Codex 1, Claude 1 |
| L9 | Claude | `/clear` and `/resume` in one process | — | 2 |
| L10 | both | the channel: a server killed mid-turn and between turns; a replaced binary; a tool commit whose answer is lost, then the briefing followed — one notify; the notices' first lines whole in the preview, alone and grouped, from a 32-character name; fail, read, recover, read, fail past the window — a new letter; a failure, a failed publication, a denial, the retry — no shell advice delivered; the end of a run: SIGTERM to the wrapper freezes the record at once and no notice is written after it, a server killed more than a heartbeat before the harness exits is told, one whose close falls within a heartbeat of an exit by itself is not ([the tool observation](mail-bridge-channel.md#the-tool-observation)); two denials, then a ticket issued between them — still blocked, one issued after the second — cleared; a denial within a heartbeat of a server's close — no shell advice written; two servers of one run, one killed — unchanged, the other killed — "server gone" at its close | main in the same state directory | Codex 2, Claude 2 |
| L11 | both | a harness timeout with the child alive: the channel unchanged, then `retry`, one effect | caller `--settings` `env`, caller `-c` | Codex 2, Claude 2 |
| G9 | Codex | the request parameters that reach a thread's layers, injection, limits or registrations; then, through rewake's gateway, a folder with no trust decision and a lowered cap, `thread/start` naming it under `workspace-write` with an empty `config` — refused before forwarding, the scratch home's `config.toml` unchanged; a trusted folder with ordinary settings still starts | the source of the version; a scratch `CODEX_HOME`, never the owner's | 0 |
| G1 | Codex | no MCP server starts in the check's app-server without a thread | sentinel servers in folder, `-c`; scratch home | 0 |
| G2 | Codex | a call listing a thread request's registrations, plugins it selects included, without starting them | a sentinel plugin in a scratch home | 0 |
| G3 | Codex | the cwd of a resume or fork whose request names none | two folders | 1 |
| G5, G7 | Claude | a parent's `.mcp.json`; a plugin server named `rewake` | folder; a scratch `CLAUDE_CONFIG_DIR` | 0 |
| G6 | Claude | `managed-mcp.json` and `mcp get` | a disposable container | 0 |
| G8a | Claude | the limits at PreToolUse and at the result, from caller and project settings, changed between calls and during a long one; our server's own `timeout` | caller `--settings`, the folder's settings under `-p` | 3 |
| G8b | Claude | the same from the user layer: set, then changed during the session | a scratch `CLAUDE_CONFIG_DIR` with no login, against the stand-in API | 0 |

G8b and G8's managed part need a stand-in API that answers with a call of our tool:
`tools/standin` ([testing.md](testing.md#the-stand-in-api)) replies with a `tool_use` of
`mcp__rewake__rewake` whenever the tool is offered, and a slow child stands in for a long
call. The user part runs in a scratch configuration directory,
whose user layer may be written, the managed part in the container of G6. The person's
user layer is never written for it, and its own values are not an experiment: they
cannot be changed during a session. A part that cannot be run stays unproven, and the
rule of the gate applies ([G8](mail-bridge-launch.md#gates)): a run whose layer of that
kind names either variable acknowledges no read on Claude Code.

## The run of October 4, 2026

Build 916f888, codex-cli 0.159.0 and Claude Code 2.1.284, the owner's logins, a cheap
model at low effort; about 20 Codex turns and about $1.6 of Claude Code. The owner's
configuration files were byte-identical before and after every case and at the end, and
both `.claude.json` files equal less runtime fields, the case entries equal to their
twins'. The facts behind each line are in [research-mail-tool.md](research-mail-tool.md).

| Case | Result |
|---|---|
| S1 | passes with the value form of the `-c` trust; the dotted form fails, and the app-server then writes trust |
| L1, L2 | pass on both: only `rewake` added, controls, denials and hooks kept; cold resume and `--strict-mcp-config` too. Finding: a folder's trust dialog holds Claude Code's servers past the 15 s hello timer, which then sends the session to the shell |
| L3 | the name refusals pass, live part; the thread-request part was not run live |
| L4 | no prompt under every policy passes; the native denial signal is absent on both — **L4 open** |
| L5 | a read's bytes equal the child's; the smallest output limit for a 4 KiB result is 861 on Codex (860 cuts), 2048 on Claude Code (2047 replaces the result with an error) — **L5 answered**; at the run the code turned reads off for any limit set, and since the revised rules were built it takes these bounds per version |
| L6 | passes on both: `pending` at a normal end, `stopped` after an Esc |
| L7 | passes: twenty calls on each, five parallel per response on Claude Code; Codex runs one server's calls serially |
| L8 | Claude Code: the nested call carries `agent_id` and `agent_type` and is refused. Codex: a sub-agent's call is refused by timeout and fails the parent's channel — a finding |
| L9 | passes |
| L10 | partly run. Finding: Claude Code 2.1.284 does not start a killed server again, so "not connected, no notice" leaves the session without its tool and untold. SIGTERM freezes the record; one of two servers killed leaves it unchanged |
| L11 | passes on Claude Code: one effect, the channel unchanged; Codex not run |
| G1 | **closed** for 0.159.0 |
| G2 | open: no call lists registrations without starting them |
| G3 | answered: a resume runs in the recorded cwd; the gateway checks another — open until the code follows |
| G5, G7 | **closed** for 2.1.284 |
| G6 | answered: a managed MCP file makes the harness refuse `--mcp-config` and exit 1 — open until rewake detects it |
| G8 | partly: a server's own `timeout` binds over `MCP_TOOL_TIMEOUT`; the output limit from caller settings applies; the layers and changes during a session not run — open |
| G9 | not closable as built: a request's `config` reaches the layers unfiltered |

## What stays unrun, and what it leaves open

- **G8b and G8's managed part** — the user and managed layers' limits, and a change
  during a session, against the stand-in API. G8 stays open, so no call on Claude Code
  acknowledges a read and tool reads there stay refused.
- **The rest of L10**: a replaced binary; a commit whose answer is lost, then the
  briefing followed; the notices' first lines from a 32-character name; fail, recover,
  fail past the window; a failed publication with a denial; the end within a heartbeat
  of a close; denials around a ticket. The channel's generated tests cover each by event
  time; what stays unshown live is that the harnesses produce those sequences as the
  tests assume. Denials wait for L4's signal in any case.
- **L11 on Codex** — a harness timeout with the child alive. The server of that run was
  killed in L10 first. Codex's `tool_timeout_sec` of 30 s above the ticket's 25 s is
  then unshown live, as is the `retry` after it.
- **L3's thread-request part** — a thread request whose `config` names `rewake`, adds a
  leaf or lowers the output limit. Step 0 is unshown live; it matters only once G2 lets
  Codex carry the tool.
- **G4** — Codex requirements on MCP servers. Requirements naming MCP keep the tool out
  regardless, so the gap costs the tool, never a wrong launch.

The revised rules of October 4, 2026, built the same day, need their own live checks,
each in the way its finding was found:

- **The version** (`wrapped-launch` is the scenario): a Claude Code launch carries the
  tool on 2.1.284 with no assumption, with `--command` or without, and runs nothing to
  learn the version; with no `claude` on `PATH`, or one whose path names no version, it
  starts without the tool and says why; a Codex launch runs its program's `--version`
  once.
- **G6 closed**: in the container of G6, `rewake claude` with the managed file present
  starts, without the tool, the channel showing the reason; with the file removed, the
  tool returns.
- **The hello timer on Claude Code**: a never-trusted folder, its dialog left open past
  15 s, then answered — no failure, a hello, and a call that works.
- **A server killed on Claude Code**: one notice to the worker and one to main, with the
  `/mcp` line; reconnected in `/mcp`, a call works and main is told so.
- **L8 on Codex again**: a sub-agent's call refused at once as an agent's, the parent's
  channel unchanged; the parent's server killed after the sub-agent's has called —
  "server gone" at the kill; killed before the sub-agent's first call — "server gone"
  placed at the kill once that call binds its connection; with a sub-agent server that
  never calls — unchanged, the open compromise of an unbound connection, recorded as
  such ([conversation connections](mail-bridge-channel-codex.md#conversation-connections)).
- **The version unconfirmed** (no model): a fixture whose `--version` names one
  version and whose `initialize` another, or none — the injection withdrawn, the
  channel "no tool" with `harness version not confirmed`, the wrapper run once more; and
  a version read whose holder is lost — the launch refused. *Built as unit tests on
  October 4, 2026, with no live run:* the unit fixture app-server answers any
  `userAgent` (`codex/server_version_test.go`, every row of the table), a holder killed
  mid-read refuses (`harness/gates_version_test.go`), and the wrapper's record of a
  withdrawal re-begins "no tool" (`wrap/mailtool_version_test.go`). The workflow shim
  answers one version to both, so no scenario withdraws.
- **L5 in the code**: a Codex launch with `tool_output_token_limit=861` keeps tool reads
  on, one with 860 turns them off.
