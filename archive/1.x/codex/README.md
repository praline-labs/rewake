# The 1.x Codex adapter, kept as a reference

This directory holds the code of rewake 1.x that connected it to Codex, and the mail
tool's MCP injection that went with it. Stage 3 of 2.0 removed both from the product in
step S8 ([stage3-steps-adapters.md](../../../docs/v2/stage3-steps-adapters.md)). The
files are copied byte for byte from commit `cf7f430`, the last commit of `v2` before S8.

**It is not live code.** The directory is a Go module of its own (`go.mod` here), so the
main module's `go list ./...`, its build, its five checks, the layout test, the rules
test and the legacy-mark test never reach it. Nothing here is built or tested. Its
imports still name the main module's internal packages, which a nested module may not
import, and several of those packages have since changed or gone. Read it; do not try to
build it.

**Why it is kept.** The 1.x Codex adapter took a long time to get right, and the owner
decided on October 8, 2026 to keep it in the repository rather than only in git history.
Stage 5 of 2.0 designs the Codex adapter afresh on the 2.0 skeleton: the core, the
adapter API, and a place for every part. It reads this code as its reference, and does
not port it ([design-codex.md](../../../docs/v2/design-codex.md)). Which parts come back,
and in what form, is stage 5's decision.

## What was where

The paths below are the paths these files had in the main module. The same paths exist
under this directory.

- **`internal/harness/codex/`** — the Codex harness. It launched Codex with a private
  `codex app-server`, built the launch settings and the briefing, and delivered letters
  as turns and steers. The design is in
  [gateway.md](../../../docs/archive-1.x/gateway.md),
  [delivery-conversation.md](../../../docs/archive-1.x/delivery-conversation.md),
  [codex-publication.md](../../../docs/archive-1.x/codex-publication.md) and
  [startup-transport.md](../../../docs/archive-1.x/startup-transport.md).
- **`internal/harness/codex/gateway/`** — the gateway between the person's Codex
  terminal and the private app-server. It relayed JSON-RPC over WebSocket. It owned the
  selection and acceptance of the conversation, injected rewake's notices, observed
  turns and their ends, and served the remote control. See
  [gateway.md](../../../docs/archive-1.x/gateway.md),
  [native-mailbox.md](../../../docs/archive-1.x/native-mailbox.md),
  [native-mailbox-ui.md](../../../docs/archive-1.x/native-mailbox-ui.md),
  [remote-control-codex.md](../../../docs/archive-1.x/remote-control-codex.md) and
  [remote-control-codex-limits.md](../../../docs/archive-1.x/remote-control-codex-limits.md).
- **`internal/bridge/server/`** — `rewake bridge-serve`, the stdio MCP server that
  Codex and Claude Code started for the mail tool. It took a ticket from the wrapper's
  endpoint for each call, ran one rewake command per call in a bounded child, and
  answered within the harness's tool timeout. See
  [mail-bridge-server.md](../../../docs/archive-1.x/mail-bridge-server.md).
- **`internal/bridge/bridge.go`, `internal/bridge/endpoint/events.go`,
  `channel.go`** — the transport names `codex-mcp` and `claude-mcp`, the parser of
  Codex's item events into the endpoint's neutral input, and the endpoint's channel
  observations: thread binding, the primary thread and the server's start failures.
  With them are their tests: `events_test.go`, `bind_test.go`, `channel_test.go` and
  `channel_order_test.go`.
- **`internal/harness/check*.go`, `gates*.go`, `mailtool.go`** — the launch injection's
  checks. A bounded run of the person's own program asked whether a server named rewake
  was already configured. The gates G1–G8 were recorded per harness version, with the
  rule each one left open. See
  [mail-bridge-launch.md](../../../docs/archive-1.x/mail-bridge-launch.md),
  [mail-bridge-launch-codex.md](../../../docs/archive-1.x/mail-bridge-launch-codex.md)
  and [mail-bridge-version.md](../../../docs/archive-1.x/mail-bridge-version.md).
  2.0 keeps only the bounded version read, renamed by subject.
- **`internal/harness/claude/mailtool*.go`, `managed.go`** — the same injection on
  Claude Code: one `--mcp-config` holding only rewake's server, its allowance, the hooks
  around each call, and the managed MCP file that leaves the tool out.
- **`internal/wrap/mailtool*.go`, `channel*.go`** — the wrapper's side: it opened the
  run's endpoint, chose the transport, let the server's children run under the harness,
  and kept the channel record and its notices. 2.0 keeps the keeper, cut to the neutral
  alphabet. The 1.x file is here whole.
- **`internal/channel/events.go`, `selection*.go`, `table_codex_test.go`** — the
  channel record's Codex and MCP letters, conversation selection on Codex, and the
  failure points that only Codex reached. See
  [mail-bridge-channel-codex.md](../../../docs/archive-1.x/mail-bridge-channel-codex.md).
- **`internal/cli/bridge_serve.go`, `accept.go`, `turn_payload.go`** — the
  `bridge-serve` and `bridge-hook` commands; `rewake accept`, the person's word on a
  resumed Codex launch that ended up in another conversation; and the decoder of Codex's
  notify payload beside the hook's.
- **`test/workflow/codexshim*`, `websocket_test.go`, `schema*_test.go`,
  `harness_version_test.go`, `codex_*_test.go`** — the suite's Codex column. It held a
  shim app-server over WebSocket, checked the shim's answers against the schema of a
  real Codex, and ran the Codex-only scenarios. The neutral scenarios that once carried
  the `codex_` prefix stayed in the suite under names by subject.
- **`tools/harnesscache/`** — fetched a named Codex version into a cache, so that the
  schema check could run against it.

[mail-bridge-live.md](../../../docs/archive-1.x/mail-bridge-live.md) records the live run
of October 4, 2026 that exercised the injection on both harnesses.

## The cli tests on the gateway

S8 sorted the `internal/cli` tests that drove the gateway as follows. All of them are
archived here whole.

- **They go:** `gateway_wire_test.go`, and `gateway_side_test.go` with its fixtures and
  `gateway_notice_display_test.go`. They test the 1.x gateway's wire and nothing else.
- **Codex later:** these hold behaviour that stage 5 has to decide for the new adapter.
  - `gateway_intent_test.go:25`: a resume is refused until it is accepted.
  - `gap_advisory_test.go:18` and `:74`: the gateway's advisories for a run without
    proof of work.
  - The selection half of `gateway_integration_test.go:50`.
- **Rebuilt neutral in S5, so the gateway copies leave:**
  - the neutral half of `gateway_integration_test.go:50`;
  - `review_receipt_identity_test.go:50` and `:82`;
  - `error_report_test.go:77` and `:126`.
- **Stays in the product:** `review_receipt_identity_test.go:109`, renamed by subject.
  The rest of `error_report_test.go` stays as well.

`accept_test.go` went with `accept.go`.
