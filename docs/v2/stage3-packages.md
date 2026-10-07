# Stage 3: where every 1.x package goes

Each package of `internal/` and `cmd/rewake` at `b6ed4ed`, by file group — and, where a
file splits, by declaration — with its 2.0 place or its removal and the step that does
it ([stage3-steps.md](stage3-steps.md), [stage3-moves.md](stage3-moves.md)). The
verdicts rest on [revision.md](revision.md#every-part-in-one-line); where these rules
decide something the revision left open, the row says why. Paths under `internal/` drop
the prefix; file names drop `.go` and their tests go with them unless
[stage3-tests.md](stage3-tests.md) says otherwise.

## Infra and core

| Package: files | 2.0 place | Step | Note |
|---|---|---|---|
| `state`: all but the two below | `infra/state` | S12 | the fault seam (`fault*.go`, tag `rewakefault`) with it; the root APIs change in S17 ([stage3-state.md](stage3-state.md)) |
| `state`: `ToolConfigPath` (`paths.go:70-73`) | removed | S8 | the Claude Code `--mcp-config` path |
| `state`: `KeeperAddress` (`paths.go:104`) | removed | S9 | goes with the keeper |
| `proc`, `boottime`, `buildtime` | `infra/proc`, `infra/boottime`, `infra/buildtime` | S12 | `buildtime` stays the human version line; the build id is new in S17 |
| `receipt` | `core/receipt` | S13 | |
| `registry`: `registry`, `names`, `liveness`, `observe` | `core/registry` | S13 | the `Build` field goes in S2; the live capabilities join in S11 |
| `registry`: `run` — `Epoch`, `RunEpoch`, `ParseEpoch`, `ParseRun`, `ObserveRun`, `EpochAlive`, `CurrentBoot`, `isCurrentBoot` | `core/registry` | S13 | every run's identity and liveness |
| `registry`: `run` — `BuildStamp`, `Session.EarlierBuild`, `EarlierBuildEpoch`; `runs` whole | removed | S2 | the migration's; callers in [stage3-steps.md](stage3-steps.md#s2-the-1x-migration-leaves-codex) |
| `registry`: `CodexHome` | removed | S8 | |
| `registry/registrytest` | `core/registry/registrytest` | S13 | |
| `sessionstate` | `core/session` | S13 | its legacy branch goes in S2 [unverified: located by the revision, `store.go`] |
| `role`, `brief` | `core/role`, `core/brief` | S13 | the harness words come from the adapter in S10 |
| `control` | `core/control` | S13 | |
| `grant` | `core/grant` | S13 | the harness directories of `rules.go:317` come from `ProtectedDirs` in S10 |
| `grantauth`: `grantauth`, `server`, `resume` | `core/grant/authority` | S13 | a subpackage, so the move stays a rename |
| `grantauth`: `keeper` | removed | S9 | serves only the Claude Code permission hook S9 removes; stage 6 redesigns the answer in the mod and reads the keeper from history (the revision's "waits for stage 6") |
| `grantauth/grantauthtest` | `core/grant/authority/authoritytest` | S13 | |
| `channel`: `record`, `history`, `display`, `notices` | `core/channel` | S13 | |
| `channel`: `selection` | removed | S8 | the Codex conversation selection; returns as the Codex adapter's in stage 5 |
| `channel`: `events` | `core/channel` | S8 cut, S13 move | the Claude MCP and Codex letters go in S8 |
| `cutover` | removed | S2 | |

## Mail: `inbox`

| Files or declarations | 2.0 place | Step | Note |
|---|---|---|---|
| the mail model: `inbox`, `addenda`, `adopt`, `answer*`, `awaited`, `claims`, `grants`, `interim`, `kept`, `local`, `marks`, `notices`, `once`, `owed`, `read_boundary`, `recall`, `reservation`, `sent`, `state_notice`, `status`, `unread`, `waiters`, `withdraw` | `core/mail` | S13 | |
| the journals and seam: `journal`, `journal_ends`, `journal_held` (publish or moot), `records`, `access`, `reconcile`, `stop`, `sweep` | `core/mail` | S13 | their migration branches go in S2; `stop` changes in S3, the decision joins in S18 |
| `turn_records`: `sweepTurnRecords`, `recordEpoch` | `core/mail` (`journal_retention`) | S2 rename, S13 move | the retention of completed journals of ended runs |
| `turn_records`: `TurnsPath`; `conversion`, `conversion_decide`, `held_take`; `Settle` | removed | S2 | the migration |
| the `(*Server)` methods in mail files: `followEarlierRun` (`adopt:121`), `checkGrants`, `failGranted` (`grants:43,80`), `prepareDelivery` (`reservation:23`), `sweepFinished`, `sweepFinishedLocked` (`sweep:16,25`) | `host/delivery` | S15 | moved to delivery files by S15's preparatory commit |
| delivery-declared, used by mail: `Availability`, `UndeliveredNotice`, `removeWaiting`, `answerLifetime`, `keepFinished`, `deliveryThread`, `ErrThreadUnavailable`, `ErrNotYet` | `core/mail` | S15 | moved to mail files by the same commit ([stage3-moves.md](stage3-moves.md#s15-the-delivery-server-to-hostdelivery-codex)) |
| the delivery server: `serve`, `batch`, `window`, `watch_linux`, `retention`, `announcement`, `availability` (less the type), `thread`, `outcome`, `held` | `core/mail` in transit, then `host/delivery` | S13, S15 | `held` follows the Claude Code messaging line it reads; its fate is P1's in stage 4 |
| the review files | renamed by subject | S16 | answer 9 |

## Tool and host

| Package: files | 2.0 place | Step | Note |
|---|---|---|---|
| `bridge`: `bridge`, `parts` | `tool` | S16 | the ticket, the parts, `ResultCap`; `CodexTransport` goes in S8 |
| `bridge/endpoint`: `gate` | `tool` | S16 | the end gate orders acknowledgments against turn ends; its interface is declared again in core in S4 |
| `bridge/endpoint`: `endpoint`, `protocol`, `client`, `config`, `calls`, `channel` | `host/endpoint` | S16 | the neutral input and the per-request peer check are written in S7 |
| `bridge/endpoint`: `events` | `host/endpoint` | S7 neutral, S8 and S9 cut, S16 move | the Codex parser goes in S8, the hook client in S9 |
| `bridge/endpoint`: `limits` | removed | S9 | the Claude Code hook's raw limits (`limits.go:1-3`) |
| `bridge/server` | removed | S8 | its oracles run on the neutral rig from S7 (correction 6) |
| `wrap`: `wrap`, `claim`, `names`, `signals`, `departure`, `availability`, `session_notices`, `hold_notices`, `compaction_letters`, `grants`, `grant_resume`, `serving`, `backend` | `host` | S16 | `serving` and `backend` call capabilities from S10; the live set from S11; the child's `REWAKE_DIR` from S17 |
| `wrap`: `channel` | `host` | S8 cut, S16 move | the MCP server lifecycle goes in S8 |
| `wrap`: `mailtool`, `mailtool_launch` | removed | S8 | |
| `wrap`: `cutover` | removed | S2 | |
| `alias` | `host/launch` | S16 | `harness.Flag` becomes the adapter API's in S10 |
| `worktree` | `host/worktree` | S16 | reads 1.x worktree records unchanged |

## Adapters

| Package: files or declarations | 2.0 place | Step | Note |
|---|---|---|---|
| `harness`: `plan`, `backend`, `thread`, `worktree`, `harness` | `adapter` (rewritten as capabilities) | S10 | `backend`'s interfaces become the capabilities; the registry of `harness.go:13-23` goes |
| `harness`: `flags` (`Flag`, `AddFlags`, `HasFlag`, `FlagValues`, `BeforeTerminator`, `MatchFlag`, `WithoutFlag`), `defaults` (`ApplyDefaults`, `Default`), `settings` | `adapter` | S10 | standard library only; the Claude Code adapter, `alias` and `cli/launch_worktree` call them, which only the API package allows all three |
| `harness`: `environment` — `SessionEnv` | `host` | S10 | the host builds the child's environment; an adapter declares as data what its children must not inherit |
| `harness`: `environment` — `CheckEnv` | `host/launch` | S10 | `wrap` calls it |
| `harness`: `notice`, `preview` | `core/mail` | S10 | the notice text is the core's; `NoticeID` and `ShellQuote`, Claude Code's only, go to its adapter |
| `harness`: `hooks` | removed | S9 | the hook constants and their users |
| `harness`: `check`, `check_diagnostic`, `check_holder`, `gates`, `gates_version`, `mailtool` | removed | S8 | the name checks and gates of an injected MCP server |
| `harness/catalog` | `adapter/catalog` | S10 | a constructor, no `init` |
| `harness/claude`: `claude`, `protected`, `resume`, `worktree` | `adapter/claude` (Launch) | S10 | |
| `harness/claude`: `socket`, `lane`, `lane_interrupt`, `receipt`, `replysweep`, `notice` | `adapter/claude` (Wake) | S10 | the 1.x messaging line, kept until P1 decides it |
| `harness/claude`: `settings`, `statusline`, `plugin` (+ `plugin.js`), `permission` | removed | S9 | the hook machinery; the mod replaces it in stage 4 |
| `harness/claude`: `mailtool`, `mailtool_check`, `managed` | removed | S8 | the MCP injection |
| `harness/claude/telemetry` | removed | S9 | the mod reports telemetry in stage 4 |
| `harness/codex`, `harness/codex/gateway` | removed | S8 | stay on `main` and in history (answer 5) |
| — | `harness/fixture`, then `adapter/fixture` | S5, S10 | new, behind `rewakefixture` ([stage3-fixture.md](stage3-fixture.md)) |

## The CLI and `cmd/rewake`

| `cli` files or declarations | 2.0 place | Step | Note |
|---|---|---|---|
| the table and printing: `registry`, `registry_internal`, `registry_send`, `model`, `render`, `output`, `output_parts`, `errors`, `args`, `naming`, `run`, `version` | `cli` | — | each command form is a view or a change from S17 ([stage3-state.md](stage3-state.md#what-is-a-writer)); `registry_internal` shrinks to nothing in S9 |
| the commands: `inbox*`, `send*`, `sent_*`, `pending`, `retry`, `withdraw*`, `edit`, `sessions`, `session_table`, `session_state`, `steer`, `worktree*`, `launch*` | `cli` | — | `launch` takes the catalogue value from `cmd/rewake` in S10 |
| the tool receipts of a command: `journal`, `journal_steps`, `shell_channel`, `bridge_run` | `cli` | — | a CLI child runs them |
| `bridge_surface` (`ToolWords`, the allowlist) | `cli` | S7 | becomes the descriptors |
| `turnended`: `endTurnContext`, `completeTurnContext`, `turnMark`, `hookLockWait` | `core/mail` | S14 | |
| `turnended`: `handleTurnEnded`, `readPayload`, `isTerminal` | removed | S9 | the hook command |
| `turn_result`: the neutral value and `kind()` | `core/mail` as `TurnEnd` | S4 in place, S14 move | |
| `turn_result`: `completedTurn` | `cli/turn_payload`, then removed | S4, S8 and S9 | the hook and notify decoder |
| `turn_reports`, `turn_hold` (less `printHold`), `pending_turn` (less `inOwnTurn` and `attemptScope`), `read_ack` (less the shim) | `core/mail` | S14 | the inventory in [stage3-moves.md](stage3-moves.md#s14-the-turn-end-and-read-code-to-coremail) |
| `completion` (`ReportCompletion`, `ConfirmCompletion` from S5), `read_ack` (`AcknowledgeRead`), `printHold`, `inOwnTurn`, `attemptScope`, `completeTurn` | `cli` | — | shims and the shell's side |
| `self` | `cli` | S2 cut | the `errUpgraded` branches go |
| `telemetry` (`observe`, `status-tap`), `granthook` | removed | S9 | |
| `bridge_serve` | removed | S8 | |
| `accept` | removed | S8 | Codex-only (correction 5) |
| `settle` | removed | S2 | |
| — `decide` | `cli` | S18 | new; its word is answer 2 ([stage3-decision.md](stage3-decision.md#the-command)) |

`cmd/rewake` stays; from S10 it builds the catalogue value with `adapter/catalog`'s
constructor and hands it to `cli`.

## Outside `internal/`

`tools/standin` loses its MCP answer in S8 and gains a fixture-tool answer in S7 [its
use beyond the suite unverified]; `tools/checksummary` stays. `tools/harnesscache` and
`REWAKE_CODEX_VERSION` fetch and run only Codex, so under answer 5 they leave in S8 with
the Codex column, and the commands of `AGENTS.md` that name them with it; they return in
stage 5. `test/toolrig` is new in S7. `test/workflow` as
[stage3-fixture.md](stage3-fixture.md) says.
