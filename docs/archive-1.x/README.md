# The archive of 1.x

The documents of 1.x code that stage 3 of 2.0 removes, moved here unchanged by the
step that removed their code (`docs/v2/stage3.md#documents`). An archived document is
never edited: it records how 1.x worked, and the records that link to it keep their
bytes too.

A moved document keeps its links as they were written, from the directory it left. The
documentation tests resolve them from there and resolve a record's link to a moved path
through the table below; a forge's file view does not, so a link inside an archived
document may open a path that is no longer there. The table says where each document
was.

| Document | Path before the move | Step | SHA-256 of its bytes |
|---|---|---|---|
| [protocol-cutover.md](protocol-cutover.md) | `protocol-cutover.md` | S2 | `0d7c397ca98c219d51bd411a8bde5f7614fe0aa777bf9c4e718f20ac23fdac38` |
| [gateway.md](gateway.md) | `gateway.md` | S8 | `3804775843d02ce28ecf830bcfb0fbd24cbe599374270c83b1a26c9b18a150a8` |
| [codex-publication.md](codex-publication.md) | `codex-publication.md` | S8 | `57996fbbf64af33620d2f8c877547db9b76f8f4c94073a6546d4da9f49cab37b` |
| [delivery-conversation.md](delivery-conversation.md) | `delivery-conversation.md` | S8 | `8277a521bb2a0c1fd848ed2ce4ad31355cb19556781d939551481993f416cd4c` |
| [startup-transport.md](startup-transport.md) | `startup-transport.md` | S8 | `aae68b0aca6e592e4e96f5e8d4b72517e04214224c48460cfe9148f1a6504b05` |
| [native-mailbox.md](native-mailbox.md) | `native-mailbox.md` | S8 | `f5e05990997138a79614dd0349f85c8d3c0e201862667b4e84bed58600b38490` |
| [native-mailbox-ui.md](native-mailbox-ui.md) | `native-mailbox-ui.md` | S8 | `49896611c6fdb4718efd002e11538bd7ba9f175a899a6a6bdd33ec2a0f6b1ec9` |
| [remote-control-codex.md](remote-control-codex.md) | `remote-control-codex.md` | S8 | `282155d0390decafe5db15abd232918c67c10b0fdbdb996380cca73ac575808b` |
| [remote-control-codex-limits.md](remote-control-codex-limits.md) | `remote-control-codex-limits.md` | S8 | `2ff75f75c108651a20dcfffe680b4b5050fbe9c242ed9a4ab21a1a8025215a09` |
| [mail-bridge-server.md](mail-bridge-server.md) | `mail-bridge-server.md` | S8 | `21678cef0f5d27a3242f289a00280f2c48d57b9377a4c4404f3c41c1719563d9` |
| [mail-bridge-launch.md](mail-bridge-launch.md) | `mail-bridge-launch.md` | S8 | `856fe6240e6a572c79c682053eed82571a794868867f70fefca086245278687f` |
| [mail-bridge-launch-codex.md](mail-bridge-launch-codex.md) | `mail-bridge-launch-codex.md` | S8 | `e66620d47d2b1d478a3f785617f948bccb70e251bf03474372f32ad9b210b883` |
| [mail-bridge-version.md](mail-bridge-version.md) | `mail-bridge-version.md` | S8 | `36b5dde6b5d86bdc932272e4530ec84132f70967fde27148ad091b2901c424b6` |
| [mail-bridge-channel-codex.md](mail-bridge-channel-codex.md) | `mail-bridge-channel-codex.md` | S8 | `9c7ed43e38ddfa5950a007fa67bbb64860c0c03c095acc735d457d3c0893ad9b` |

- [protocol-cutover.md](protocol-cutover.md) — how a mailbox passed from the protocol of
  builds before September 30, 2026 to the one of turn-end journals: the launch order, the
  states of the successor, run records named by boot and epoch, the upgrade the
  automatic cutover was bounded by and the look for earlier-build writers within it, and
  what a build refused or held for a run of an earlier build. Removed in S2 with the
  conversion and the cutover; open it to read a record that names them.

Moved in S8, October 8, 2026, with Codex, the MCP injection and the mail server they
served; their code is kept beside them as a reference in
[archive/1.x/codex](../../archive/1.x/codex/README.md). Their links into documents that
stay may name text since rewritten for the neutral alphabet.

- [gateway.md](gateway.md) — the Codex delivery gateway from the inside: how the primary
  terminal connection and its conversation are selected and reserved before mail goes
  in, the ledger of admitted work, report publication, forks and side conversations, and
  the regression and acceptance coverage. Open it only when working on the gateway; it
  is dense with evidence, not an overview.
- [codex-publication.md](codex-publication.md) — which Codex turn outcomes reach the
  waiters: proof of work, the advisory report of a turn without it or of a run that
  passed unseen, a compaction's turn reporting nothing, and why no outcome is dropped
  and no compaction settles a task. Open it when a Codex report is missing, advisory
  when it should settle, or settles when it should not.
- [delivery-conversation.md](delivery-conversation.md) — why a Codex message stays
  pending in a conversation the launch did not ask for: the launch's intent, `rewake
  accept`, the record that keeps the worker's inbox closed meanwhile, and how the sender,
  main and the person at the terminal are told; and why a worker whose sandbox closes
  rewake's state directory is not handled there yet; and, for either harness, how a
  resumed run takes over the waits of the earlier runs in its conversation. Open it when a
  Codex delivery waits after a resume, a worker cannot read its mail, or a resumed run's
  report does not settle what an earlier run read.
- [startup-transport.md](startup-transport.md) — the Codex transport's startup failures
  and their repair: a request queue that overflowed in startup bursts, a message size
  limit hit later, the sizes chosen and the live acceptance. Open it for why those limits
  are what they are.
- [native-mailbox.md](native-mailbox.md) — how the Codex adapter delivers a notice as the
  output of a `rewake_mailbox_notice` tool call in a turn start: its JSON, the briefing
  text that explains it, and the workspace-write permission it needs. Open it when a
  Codex notice arrives wrong or not at all.
- [native-mailbox-ui.md](native-mailbox-ui.md) — the arrival row a Codex terminal shows
  (`rewake notice --display-only`): where it is inserted, why it bypasses admission and
  telemetry, that it is not persisted, and its installed acceptance. Open it for what the
  person at a Codex terminal sees when mail arrives.
- [remote-control-codex.md](remote-control-codex.md) — how a Codex session serves those
  requests through its gateway: the three safety properties, when a compaction is
  refused as busy, uncertain or as nothing to compact, the open operations kept across
  reconnects, the mark that ties main's compaction to its turn and what happens to it
  when the request fails, the terminal's `/compact` answered while main's runs, and the
  interrupt naming main. Open it when a Codex compaction or interrupt from main
  misbehaves.
- [remote-control-codex-limits.md](remote-control-codex-limits.md) — the known limits of
  that service: a goal's turn compacting before main's compaction, a reply after its
  turn's end, a request left unanswered, a compaction lost sight of or ending unseen,
  work accepted and lost, a running turn whose id is unknown. Open it when main's
  compaction is refused as uncertain, or its answer or letter looks wrong.
- [mail-bridge-server.md](mail-bridge-server.md) — the server of the mail tool, built and
  started by a launch only once stage 3's gates allow: its eleven rules and where each lives in the code, how a
  call is matched to a native observation and given a ticket, the call's binding to its
  operation, the child it runs, the bounded answer, the failure points of a call and the
  channel record left to stage 3. Open it before changing the server or the context
  endpoint.
- [mail-bridge-launch.md](mail-bridge-launch.md) — stage 3 of the mail tool's design, built, live
  checks open:
  how each harness's launch injects the server and approves only its tool, which sources
  of a server named `rewake` each check covers before the run is published and what is
  done where coverage is unknown, what a diagnostic may show, how the person's
  configuration is proven untouched, the gates with the action each takes while open, and
  `REWAKE_GATES_ASSUMED` for the live checks. Open it before changing the injection.
- [mail-bridge-launch-codex.md](mail-bridge-launch-codex.md) — the Codex part of stage 3:
  the `-c` values the launch adds, the name check by a separate app-server before the
  claim, the injection check at start and at every thread with its step 0 for the thread
  request itself (the terminal's keys, the trust rule), and the output limit. Open it
  before changing the Codex injection or the gateway's thread check.
- [mail-bridge-version.md](mail-bridge-version.md) — the harness version a launch takes
  for the closed gates and the L5 bounds: when it is taken, from where without running
  anything extra (Codex's one `--version` read, Claude Code's install path), what an
  unknown one means, and a read whose cleanup failed; built, with how the code reads
  it. Open it before changing `gates_version.go` or how a launch chooses the tool.
- [mail-bridge-channel-codex.md](mail-bridge-channel-codex.md) — the channel's
  conversations on Codex, where every thread runs its own server: which connections
  serve the conversation, how a call's `_meta.threadId` binds one, and how selecting
  another conversation moves the timer, held events and an open interval; built, with
  the choices the code made. Open it before changing how the endpoint, the gateway's
  selection steps or the keeper count Codex connections.
