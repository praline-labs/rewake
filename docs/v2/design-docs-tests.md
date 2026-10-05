# Design: documents and tests of 2.0

How the documentation and the tests are laid out, and how a rule stays tied to the tests
that hold it. What happens to each 1.x document and test is in
[revision-docs.md](revision-docs.md) and [revision-tests.md](revision-tests.md). The
overview is in [design.md](design.md).

## Documents

Four parts (the plan; answer 10 merges the overlaps):

```
docs/
  README.md            the root map: a short index of the parts, under 400 lines
  rules/               how 2.0 works, harness-free: flow, layout, the adapter API, mail,
                       turn ends and recovery (E), tools (T), channel (C), launch (L),
                       grants, roles, worktree, state, install, testing
  adapters/claude/     the mod: loading, tools, turns, telemetry, control, wake, limits
  adapters/codex/      stage 5
  research/            facts per harness, each dated with the version it was seen on:
                       claude/, codex/, and the general (worktree, prior art)
  archive-1.x/         the records of 1.x, moved unchanged: reviews, investigations,
                       live checks, acceptances, the 1.x roadmap, the 1.x design
  roadmap/             2.0's stages and review rounds, one file per entry
  v2/                  this design and the revision, until stage 7 folds them into
                       rules/ and the revision into the archive
```

- **Each part has its own `README.md`** listing its documents with what each holds;
  the root map lists the parts. `docs/map_test.go` already walks every directory below
  `docs/` and checks links and headings, so it carries unchanged; what changes is that
  each index stays under 400 lines, which the 507-line map of today does not.
- **Rules documents state rules, not history.** A rule carries its date where the owner
  decided it; what each review found goes to `roadmap/`, as `AGENTS.md` asks today.
- **The overlaps fold** (answer 10): the twelve `mail-bridge-*.md` become the tool rules
  (T), the effect rules (E) and, in stage 5, a Codex injection document; `delivery*.md`
  one delivery document plus the adapters' wake; `remote-control*.md` the command side in
  the rules and the served side per adapter; `check-runner*.md`, never built, goes to
  the archive.
- **A record is never edited when it moves** (`AGENTS.md`, "Keeping the documentation
  true"); only the links into it are rewritten.
- **Research is split by harness and by how a fact was obtained**, as today: what a
  binary answers, what the types or the schema state, what only a running session shows.
  The mod's API facts from the recon (`.scratch/v2-recon/claude-mods.md`) move into
  `research/claude/` with their marks, re-checked by the stage 4 probe.
- **`AGENTS.md` follows in stage 3**: the five checks with `internal/layout_test.go`,
  "Adding a harness" as capabilities plus one line in the catalogue's constructor, the
  rule that a change to the mod runs the workflow suite, and the reading order pointing
  at `docs/rules/`.

## Tests

### The tiers

They carry over (`docs/testing.md`): unit and table tests; generated spaces with an
oracle written from the rules, not the code; fault tests through the fault seam (build
tag `rewakefault`); the workflow suite in `test/workflow` with its columns, controls
(`mutant_test.go`, `docs/controls_test.go`) and the crosswise check; live checks run by
main. Heavy runs one at a time, `go test -p 2 -parallel 4`.

### Each rule names its tests

A rule in `docs/rules/` ends with the tests that hold it, by package and function. A new
documentation test, `docs/rules_test.go`, runs with the five checks and fails when a
numbered rule (E, T, C, L) names no test, or names one that does not exist in the
module (it parses the Go files; nothing is run). So a rule cannot lose its tests in a
move without the move going red, and the generated spaces travel with their rules:

| Rules | Tests carried with them |
|---|---|
| E1–E6 | `inbox/inbox_test.go` (`stateDir`), `records_test.go`, the M1 tests on the journal protocol (`plan_faults`, `plan_pairs`, `plan_writes`, `seam_probe`, `journal`, `journal_unknown`, `pending`, `read_boundary`, `confirm`, `records`, `stop_writes`) |
| E7–E8 | `cli/bridge_rules_test.go`, the turn-journal tests (`turn_journal`, `turn_journal_retry`, `turn_retry`, `turn_scope`), the four plan scenes that do not rest on the migration (`plan_scenes_test.go`), `late_unknown_test.go` with its lab rebuilt |
| T2–T8 | `bridge/endpoint/order_table_test.go`; the oracles of `bridge/server/fault_test.go:91`, `order_cut_test.go:23`, `order_gen_test.go:87`, made transport-neutral and run against every transport: the result lost, replaced or truncated, the transport failing after the core ran, a turn's end crossing the confirmation — each ending with no letter read that the model did not get whole |
| C1–C8 | `channel/space_test.go` with `space_oracle_test.go` (the alphabet without the Claude MCP letters), `order_test.go`, `late_test.go` |
| turn outcomes | `cli/turn_hold_test.go`, `test/workflow/pending_confirm_test.go` with its negative controls |
| host | `wrap/ownership_test.go`, `stop_follow_test.go`, `signals_test.go` |

The MCP transport's own cases (frames, the encoder, the child) return with the Codex
adapter in stage 5, and with them the selection space
(`channel/selection_space_test.go`), moved to the adapter or to neutral events.

### The workflow suite

Columns per adapter: a **fixture adapter** column from stage 3 (neutral scenarios, no
real harness), the **Claude Code** column on a mod host from stage 4 — the fixture
becomes a host that loads the generated module and plays `session.start`, `tool.call`,
`turn.start`, `classic.Stop`, `turn.complete`, `session.measure` to it, seeded by
`claudeshim_plugin_test.go` — and a **Codex** column from stage 5. Each tool of the set
has one scenario shared by every column that offers ToolTransport, so "the same tools on
every harness" (decision 9) is checked, not assumed. A scenario for the pair — a Claude
Code main with workers in both columns — is part of stage 5's acceptance.

### The review files

Answer 9: the test files named after a review round are renamed by subject, with their
`TestReview…` functions. The revision counted eleven (632 lines); there are fifteen,
1 059 lines: the six of `gateway`, three of `cli`, two of `inbox`, and also three in
`harness/codex` (`review_batch_liveness`, `review_consumed_grant`, `review_drain`) and
one in `wrap` (`review_notice_edges`). The nine Codex ones leave with the Codex code; the
other six move in stage 3:

| Today | 2.0 |
|---|---|
| `cli/review_observation_lock_test.go` | `cli/observation_lock_test.go` |
| `cli/review_receipt_identity_test.go` | `cli/receipt_scope_test.go` |
| `cli/review_shell_json_test.go` | `cli/list_json_test.go` |
| `inbox/review_socket_fallback_test.go` | `host/delivery_later_mail_test.go` |
| `inbox/review_waiting_inbox_test.go` | `host/delivery_arrivals_test.go` |
| `wrap/review_notice_edges_test.go` | `host/departure_notice_test.go` |

"Tidy the tests generally" (answer 9) means, beyond the names: a test lives in the
package of the code it holds, a helper shared by two packages lives in one test-only
package, and no test file passes 400 lines.
