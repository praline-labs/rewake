# The terminal of Codex 0.157.1 recognized — September 26, 2026

A live probe of the same day ran real terminals and app-servers of Codex 0.155.1 and
0.157.1 in containers without a network. On 0.157.1 the session registered and the
server answered every lifecycle request writable, but the gateway selected nothing:
after the first launch, `/new`, `/resume` in the same process and `codex resume <id>`,
`rewake send` refused with `selected conversation is not ready`. 0.155.1 delivered
after all four ([research-codex.md](../research-codex.md#the-terminals-selection-on-01571)).

## Why

`recognized` (`internal/harness/codex/gateway/state.go`) took a start or a resume for
the terminal's own only when it carried `runtimeWorkspaceRoots` or `permissions`. In
remote mode 0.157.1 sends the roots null, and the permissions were null on both
versions. Nothing else in the request was read as the terminal's mark, and a
configuration merely present was deliberately not enough: the terminal's helpers and
its `PreserveExistingThread` resume carry one too.

## What was done

- **The terminal's configuration is its mark.** The ordinary builder of the terminal's
  overrides always writes `web_search`, one of `disabled`, `cached`, `indexed` and
  `live`; the projection (`metadata.go`, `tuiConfig`) takes a `config` object holding
  that key as one of those four strings, and nothing else of the configuration. Other
  keys beside it are allowed: 0.155.1 sends a personality, and a launch's `-c` keys come
  along.
- **Start**: the id families and `threadSource: "user"` gate as before, and the
  configuration is a third alternative to the roots and the permissions.
- **Resume**: a numeric id and a thread, and a new branch — the configuration with no
  history and no path (`byID`), which is the by-id resume the terminal sends.
  `excludeTurns` is not required: it is left out when false.
- **Fork** takes the same alternative under its existing guards, from the source only.
- **An ordinary resume is not a reconnect.** `reconnectIntent` (`connections.go`) took
  any numeric resume without roots on a fresh connection for the terminal rejoining;
  a 0.157.1 `/resume` of the last selected thread was then allowed no backfill, and its
  first read of another loaded conversation undid the selection. A resume carrying the
  configuration is excluded, as one carrying roots was.
- **Duplicates refused**: `web_search` inside the configuration, and `history`, `path`
  and `excludeTurns`, as the other fields recognition reads.
- **The fixture's terminal speaks 0.157.1** under `RW_SHIM_TUI_SHAPE`, and reads its
  goal after a resume as the real one does; the shim serves the goal query. The schema
  case checks both forms and the goal reply.
- Documents: [research-codex.md](../research-codex.md#the-terminals-selection-on-01571),
  [research-protocol.md](../research-protocol.md#the-terminals-lifecycle-requests-on-01571),
  [gateway.md](../gateway.md#compatibility-and-limits),
  [gateway-native-evidence.md](../gateway-native-evidence.md),
  [thread-ownership-investigation.md](../thread-ownership-investigation.md),
  [traps.md](../traps.md), [testing.md](../testing.md) and
  [testing-cases.md](../testing-cases.md).

## Evidence

- `internal/harness/codex/gateway/tui_paths_test.go` replays the probe's five recorded
  paths (`testdata/tui-paths`) through a real gateway and checks the selection at the
  probe's checkpoints: startup, `/new` and `/resume` on both versions, and a separate
  resume launch. On the old rule six checkpoints failed, every 0.157.1 one that should
  select.
- `tui_config_test.go`: the four modes and the negatives — an empty, null or string
  configuration, `web_search` null, a number, a list, an unknown mode or nested; helper
  ids and a subagent source; a resume with a history or a path, with an empty
  configuration, of defaults only, or with only the terminal's tool server; duplicates;
  a helper after a selection; a fork. The reconnect test went red without the change
  in `connections.go`.
- The workflow case `codex-tui-later-shape`: a fresh and a resumed worker of the
  0.157.1 form each report a task `finished`. Its mutant `roots-only-recognition`,
  the old rule, breaks both observations.
- The schema case green against the 0.155.1 and the 0.157.1 schemas.
- The full workflow suite with the new case: 124 cases, 121 pass and 3 unsupported for
  a named capability, 4m19s.

## What stays open

- **No fork and no `PreserveExistingThread` resume of 0.157.1 was driven live.** The
  fork rule rests on the source. A resume of defaults is still no selection, except as
  a correlated reconnect; the terminal's tool transport may add its own server to that
  configuration, and its code lies outside the trees read, so a transport that also
  wrote `web_search` would make such a resume a selection.
- **The mark is a fingerprint of the terminal's builder**, not a protocol statement: a
  later version that stops writing `web_search` would fall back to unavailable, as
  0.157.1 did, and the suite would not notice until its terminal form is added.
- The helpers' paths were read in both tags, not driven: no model call ran.
