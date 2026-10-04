# Live checks of the launch injection

The plan of the live checks of stage 3 of [mail-bridge.md](mail-bridge.md): where they
run, how each case sets its conditions, how preservation is proven without touching the
person's configuration, and which gates of [mail-bridge-launch.md](mail-bridge-launch.md#gates)
and [mail-bridge-channel.md](mail-bridge-channel.md) each case closes. **Plan, October 4,
2026; nothing run yet** — the stage's code is built and accepted, and these checks come
next. The checks themselves are recorded in the stage's roadmap entry when they run.

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
  caller's `-c 'projects."<absolute path>".trust_level="trusted"'`, or the caller's `-c`
  itself. That `-c` trust skips the prompt and writes nothing is read in the source, not
  seen; **S1** checks it first. An untrusted folder still serves the name cases: the
  check refuses a layer that is off.
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

Before and after each run, the case records digests, never contents (an `env` may hold
secrets):

- **Byte for byte**: `~/.claude/settings.json`, `~/.claude/settings.local.json` when
  present, `~/.codex/config.toml`, and the case folder's own configuration files —
  rewake edits none of them either.
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

As built, every gate is open, so no launch carries the tool: a case that needs it — all
but S1 and the gates' own checks — launches with `REWAKE_GATES_ASSUMED` naming the gates
it stands behind ([gates taken as closed](mail-bridge-launch.md#gates-taken-as-closed)),
and its record says which. A gate a case closes is entered in `closedGates` for the
harness version it ran against, and later cases on that version need not assume it: a
launch reads its harness's version whenever the table names one for it, and one it cannot
read keeps every gate open.

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
