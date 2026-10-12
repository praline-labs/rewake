# Rules: what a launch adds, L1–L3

What rewake adds to a harness's launch, in the full wording of
[design-rules.md](../v2/design-rules.md#what-a-launch-adds-l1l3), from
`mail-bridge-launch.md`.

## The rules

- **L1. The mail is added and nothing else changes.** The person's configuration is
  never edited; what a run needs is passed for that one launch. (`mail-bridge-launch.md`
  rule 1; until S9, on Claude Code, one `--plugin-dir`.)
  Tests:
  - `internal/adapter/claude/claude_test.go` `TestAddedFlagsStayBeforeTheTerminator`
  - `internal/adapter/defaults_test.go` `TestAnExplicitFlagIsNotReplaced`
  - `internal/adapter/fixture/launch_test.go` `TestALaunchLeavesThePersonsFilesAsTheyWere`
  - `internal/adapter/fixture/launch_test.go` `TestACallerCannotPassTheFixturesOwnFlags`

- **L2. Diagnostics say where, never what**: no configuration content or argument value
  is printed. (Rule 7.)
  Tests:
  - `internal/adapter/fixture/launch_test.go` `TestALaunchsDiagnosticsNeverCarryWhatTheyPointAt`
  - `internal/host/launch/version_test.go` `TestAVersionRefusalNeverCarriesWhatTheProgramPrinted`

- **L3. Nothing persists past the run**: what a launch writes lives in the run's
  directory and goes with it. (Rule 8.)
  Tests:
  - `internal/host/control_test.go` `TestTheControlDirectoryLivesAsLongAsTheSession`
  - `internal/host/control_test.go` `TestAHarnessThatServesNoRequestsGetsNoDirectory`
  - `internal/host/wrap_test.go` `TestSessionIsPublishedWhileRunningAndRemovedAfter`
  - `internal/host/ownership_test.go` `TestCleanupLeavesTheNextOwnerAlone`
  - `internal/host/ownership_test.go` `TestEachRunCleansOnlyItsOwnSocket`
  - `internal/adapter/claude/claude_test.go` `TestLiveSocketIsNeverRemoved`
  - `internal/adapter/claude/claude_test.go` `TestStaleSocketIsReplaced`
  - Gap: the run directory `run/<name>.<epoch>/` removed with the run — closed in S17.

## What returns with the Codex adapter

`mail-bridge-launch.md` rules 2, 4, 5 and 6 are about injecting an MCP server and checking
its name; they return, redesigned, with the Codex adapter (stage 5). Rule 5's guarantee
— the effective limits decide a read — is T6.
