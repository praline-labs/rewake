# Injecting the mail tool on Codex

The Codex part of [mail-bridge-launch.md](mail-bridge-launch.md), whose rules, launch
order, refusal, diagnostics and [gates](mail-bridge-launch.md#gates) apply here: the
values the launch adds, the name check by a separate app-server before the claim, the
injection check the startup probe and the gateway run at every thread, and the output
limit. Facts are marked as in that document: *verified*, *source*, *probe*, *gate*.

## What is added

As `-c` values to the owned app-server and to the terminal (probe 2):

| Leaf under `mcp_servers.rewake` | Value |
|---|---|
| `command`, `args` | the resolved executable; `["bridge-serve"]` |
| `env.REWAKE_DIR`, `env.REWAKE_ROOM`, `env.REWAKE_SESSION`, `env.REWAKE_EPOCH`, `env.REWAKE_BRIDGE_CAPABILITY` | the run's values, one leaf each |
| `enabled_tools` | `["rewake"]` |
| `tool_timeout_sec` | `30`: the ticket's deadline of 25 s falls inside it |
| `default_tools_approval_mode` | `"approve"` |
| `omit_tools_from` | `["code_mode"]` |

The caller's `-c` values keep their order and come first. The capability is on both
command lines, inside the boundary the server already draws
([what the server knows](mail-bridge-server.md#what-the-server-knows-at-start)).

## The name check: a separate app-server before the claim

Main's decision of October 1, 2026, costing about one second per launch. Rewake starts
`<program> <the caller's -c values> app-server --listen unix://<socket>` — the launch's
program, so a `--command` wrapper sets the same Codex home — with none of our values, in
the directory the real server will use (`-C`, else the launch directory), with the
harness's environment and `CODEX_INTERNAL_APP_SERVER_REMOTE_CONTROL_DISABLED=1`, its
socket and discarded stderr in a private 0700 directory under the state root. It initializes and asks:

- `config/read` with that cwd and `includeLayers: true`: no layer and no effective
  `mcp_servers` may hold `rewake`;
- the registrations outside the configuration (below), by the means the gate settles.

Then it closes the connection, ends the process group (SIGTERM, then SIGKILL a second
later), reaps it and removes the directory. The whole check is bounded at 10 s. No thread
is started, and the gate G1 shows that no MCP server starts without one.

| Source of a name | Covered by | Known how | Unknown, and the action |
|---|---|---|---|
| packaged defaults | the effective `mcp_servers` of `config/read` | *source*: `config_manager_service.rs` 157–174 leaves this layer out of `layers` and `origins` | — |
| managed: `system`, `mdm`, `enterpriseManaged`, legacy managed file | its layer in `layers` | schema; *source* | — |
| `user` (`$CODEX_HOME/config.toml`) | its layer | *probe*, schema | — |
| each `project` from the cwd to the repository root, trusted or not | its layer, with `disabledReason` when off | schema | a layer off today is on once the folder is trusted, so it refuses all the same |
| the caller's `-c` (`sessionFlags`) | its layer; rewake parses the keys only to say which argument | *verified* (`mcp get` honours them); schema | — |
| a profile | — | rewake already refuses `--profile` | — |
| plugins (discovered and thread-selected), extensions, hosted apps, compatibility servers | the registrations a thread will have, named `rewake` | *source*: `codex-mcp/src/catalog.rs` registers them in the same name space, configuration first, so ours would hide theirs | **gate G2**: a call that lists them for a given thread request — the plugins it selects included — without starting them |
| each thread: `/new`, resume or fork, in any directory | the injection check, below, at every thread | — | **gate G3**: the cwd of a resume or fork whose request names none |
| a thread request's own `config` and other parameters | step 0 of the injection check | *source*: `thread.rs` 100, 410, 604 take `config`; `config_manager.rs` 446–457 lays it over the server's `-c` for that thread, while `config/read` (481–497) never sees it | **gate G9**: which parameters reach the layers, the injection, the limits or the registrations; until it closes, only the list below passes |

A project entry named `rewake` would not replace ours but merge into it field by field
(*source*: `mcp_servers` merge per server name and field), which is why a covered layer
naming it refuses whatever its values.

## The injection check, at start and at every thread

The startup probe of the real
server, and the gateway at each `thread/start`, `thread/resume` and `thread/fork` before
forwarding it, run one check on the real server with the thread's cwd — the request's,
else the server's own for a start, else the gate G3 — bounded at 5 s:

0. **The request itself**, at a thread only, since `config/read` cannot see it. Which
   keys the terminal sends is read in the source; that any value of them leaves the
   layers and limits steps 1–4 read as they were is not proven, so the two are kept
   apart.
   - *Keys.* Every top-level key of its `config` must be one the terminal sends
     (*source*, `config_request_overrides_from_config` in `tui/src/app_server_session.rs`):
     `allow_login_shell`, `default_permissions`, `features`, `network`, `permissions`,
     `sandbox_workspace_write`, `shell_environment_policy`,
     `suppress_unstable_features_warning`, `model_reasoning_effort`,
     `model_reasoning_summary`, `model_verbosity`, `personality`, `web_search`,
     `bypass_hook_trust`. `features`, which may reach registrations, passes only when
     each leaf equals the caller's `-c` the preflight checked. Any other key —
     `mcp_servers` in any form, even equal to ours, `tool_output_token_limit`,
     `profile`, `projects`, a key unknown — refuses, and so does
     `runtimeWorkspaceRoots`, which the terminal leaves unset under `--remote`.
   - *Trust.* A request changes the active layers with no key at all when it names a
     cwd whose project has no trust decision and its effective permissions — the
     `sandbox` and `permissions` fields, the keys above, managed constraints — may write
     that cwd: the app-server then trusts the project in its configuration home, or in
     memory when that write fails, and reloads (*source*, `thread_processor.rs`
     1340–1414). The gateway cannot compute those permissions. So a request naming a cwd
     passes only when the effective configuration already holds a trust decision,
     trusted or untrusted, under the key Codex uses for that cwd or its repository root
     (G9); otherwise it is refused. A request names a cwd when its `cwd` holds a string,
     an empty one included: the server takes a relative path from its own directory and
     its trust branch asks only whether one was given (*source*, `thread_processor.rs`
     1363 and 1684, `core/src/config/mod.rs` 1502–1506 at `rust-v0.159.0`); so the
     check resolves it the same way, and only an absent or null `cwd` names none. A
     request naming no cwd does not reach that branch, and the terminal names one only
     when given `-C` or `--cd` (`thread_cwd_from_config`).
     rewake never raises trust or edits a file to let a request pass.
   - *Version.* The list is the terminal's of 0.159.0; ordinary settings of a thread
     are on it.
1. `config/read` with `includeLayers: true`. A layer other than `sessionFlags` that names
   `mcp_servers.rewake` refuses whatever it holds, an empty table included — `user`, a
   `project` on or off, a managed one — even when its values equal ours or a higher
   layer hides them: the effective table cannot say where a value came from. From the
   `sessionFlags` layer it removes exactly the leaves of our injection, each by its key
   and our value; any leaf left there, the person's `-c`, refuses, and an empty table,
   at the top or nested, counts as a leaf.
2. The effective `mcp_servers.rewake` must hold our leaves and nothing else but the
   defaults the reply fills in (`environment_id`, `enabled` and the like, as probe 2
   recorded them); `omit_tools_from` must be in effect, else tool reads are off.
3. The registrations of that thread, by G2's call, hold no `rewake` but ours.
4. The output limits and requirements below.

At start a failure refuses the launch and removes the record as after any failed start.
At a thread it answers that request with the refusal and forwards nothing, so no server
of that thread starts and the session goes on without it. A cwd that cannot be known, a
read that fails, an answer not recognized or the bound passing count as a failure: an
unknown source is never an empty one. So the check is the same at every thread, in the
launch directory or another, and the preflight above is what tells the person's entry
apart from ours before ours exists.

## The output limit

Two values decide it (*source*): `tool_output_token_limit`
(`core/src/config/mod.rs`) and the per-tool `mcp_servers.rewake.tools.rewake.output_token_limit`
(`codex-mcp/src/binding.rs` 267–276). The second can come only from a layer naming
`rewake`, which step 1 refuses. The first, read by the injection check, must be unset,
the default probe 2 calibrated; any value turns tool reads off until the gate L5
calibrates the smallest allowed one. Managed requirements are read with
`configRequirements/read`; a requirement on MCP servers that does not admit ours means
no tool (**gate G4**: its exact fields).
