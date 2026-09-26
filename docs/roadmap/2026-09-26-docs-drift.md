# Documentation that had drifted from the code — September 26, 2026

A review of the documentation against the code, held on September 26, 2026, found
lists that repeated what the code declares and had fallen behind it. Each was either
dropped in favour of the source that cannot go stale, or put under a test.

## What was found

- **The controls in `docs/testing.md`.** "Sixty-five controls, fifty-six mutants" against
  sixty-one mutants and nine fixture switches in the suite: `claude-steered` listed eleven
  of fourteen and named `compaction-uncounted`, which does not exist, for
  `letter-uncounted`; `codex-steered` listed five of seven.
- **The command list in `docs/design.md`.** Four commands (`withdraw`, `edit`, `pending`,
  `guide`) and eight flags (`--version`; `send`'s `--to`, `--grant-git`, `--notify`;
  `inbox`'s `--peek`, `--message`, `--owed`, `--awaited`) were missing.
- **The source tree in `docs/code.md`.** `internal/buildtime`, `internal/boottime` and
  `internal/harness/claude/telemetry` were missing from a tree that claims to be complete.
- **The session record in `docs/design.md`.** `messagingReadyAt`, `ownsSocket`,
  `codexHome` and `pidNamespace` were absent from the example.
- **Smaller ones.** Two "Now:" sections in `docs/work-queue.md` and a stale list of
  built scenarios there; the map's line for the work queue silent on Codex's
  `--worktree` and the dropped todo list; the suite's length given as "a little over
  two" minutes in `test/workflow/harness_version_test.go` against about three elsewhere.

## What was done

- **Controls: a table under a test.** The list moved from prose in `testing.md` to a
  table by scenario in [testing-cases.md](../testing-cases.md#every-control), without a
  count. `docs/controls_test.go` parses the suite and requires the mutants column to equal
  its `mutation` values both ways, and every listed fixture switch to be a name the suite
  spells. A switch has no single shape in the code — a field of a control struct, an
  argument of a helper — so a switch added without the table is left to review.
- **Commands: no copy.** `design.md` points to `internal/cli/registry.go`, `rewake guide`
  and `rewake <command> --help`, which the tool derives from one table; a second copy
  could only go stale again.
- **Source tree: under a test.** `docs/layout_test.go` walks the module for directories
  holding Go files, skipped as the go command skips them, and requires the tree in
  `code.md` to equal them both ways. The three packages were added.
- **The rest by hand.** The session record gained its four fields with a line each; the
  second "Now:" became "Then:", as only research remains of it, and the built scenarios
  point to `testing-cases.md`; the map's line and the comment were corrected.

## Evidence

- `TestEveryControlIsListed` over a table copied from the old list failed on exactly the
  seven discrepancies above: six mutants unlisted, `compaction-uncounted` listed and
  absent. A switch renamed or moved to the wrong column failed as well.
- `TestEveryPackageIsInTheLayout` over the old tree failed on the three missing packages.

## What stays open

Nothing.
