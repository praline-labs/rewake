# Rules: the layout

What each layer of the tree may import, where a harness may be named, and how adapters
reach the program. The import rule and the registry rule are held by one test over the
whole module, `internal/layout_test.go`. The layout and its reasons are
[design.md](../v2/design.md#the-layout); where each package lives is
[code.md](../code.md). The rules have no numbers: each section below is one rule.

## The import rule

Every package of the module, its test imports included, in every build variant — with
and without `-race`, `rewakefault`, `rewakefixture` and both — imports only what its layer
may. A layer always imports itself, and an adapter its own parts:

| Layer | May import |
|---|---|
| `infra/*` | the standard library and other `infra` packages, acyclic |
| `core/*` | `infra` |
| `tool` | `core`, `infra` |
| `adapter` | `core`, `infra` — the values the interfaces carry |
| `adapter/<name>` | `adapter`, `tool`, `core`, `infra`; never another adapter |
| `adapter/catalog` | `adapter` and every `adapter/<name>` — the only package that does |
| `host` | `adapter` (the interfaces), `tool`, `core`, `infra`; never an adapter |
| `cli` | `host`, `adapter`, `tool`, `core`, `infra`; never `adapter/catalog` or an adapter |
| `cmd/rewake` | `cli`, `adapter/catalog` |

Every layer but `infra` may import another module's package, each dependency its own
decision (`AGENTS.md`, "Code"). A module taken as an exception to that rule may be
confined to the one package it was taken for: `github.com/charmbracelet/` is imported by
`internal/host/terminal` alone ([stage4-space.md](../v2/stage4-space.md#the-terminal-library)),
and an import of it anywhere else fails naming the file and the package that may. A forbidden edge fails naming both packages and the file
that makes it, and the build tag when only a tagged build has it; a package in no layer
fails; a directory with a module of its own is no package of this one. The mail
functions that run under a mailbox lock their caller holds keep the callers the test
lists, since the lock is not reentrant and nothing at run time would say a caller is
outside its section.

Tests:
- `internal/layout_test.go` `TestImportsFollowTheLayers`
- `internal/layout_cases_test.go` `TestAForbiddenEdgeFailsNamingBothPackagesAndTheFile`
- `internal/layout_cases_test.go` `TestAnEdgeOnlyATagBuildsFailsNamingTheTag`
- `internal/layout_cases_test.go` `TestAConfinedLibraryIsImportedByItsPackageAlone`
- `internal/layout_cases_test.go` `TestAPackageInNoPlaceFails`
- `internal/layout_cases_test.go` `TestOneAdapterMayNotImportAnother`
- `internal/layout_discovery_test.go` `TestEveryImportReachesTheRule`
- `internal/layout_discovery_test.go` `TestARaceOnlyFileIsJudged`
- `internal/layout_discovery_test.go` `TestADirectoryWithItsOwnModuleIsNoPackage`
- `internal/layout_callers_test.go` `TestTheLockedEntryPointsKeepTheirCallers`
- `internal/layout_callers_test.go` `TestCallerProblemsSeeEveryMismatch`

## Harness names outside the adapters

No Go file under `core`, `tool`, `host`, `cli` or `infra` names a harness — its id or
title, or a word that belongs to one harness such as `mcp__`, `CLAUDE_`, `app-server`,
`.claude`, `.codex`, `.agents` — in an identifier, a string or a comment: a comment that
explains core code by one harness's behaviour is the first step toward a branch on it.
Since the owner's decision of October 17, 2026 no test reads the words; the import rule
keeps every adapter package out of the core, and review holds the rest.

## No registry

No package under `adapter` declares `func init`, and no package-level variable of an
adapter type, or of a slice or map of them, exists outside `adapter/catalog`'s
constructor, in a test file as in any other, whatever the file's build constraint; two
declarations on one line are two.
`cmd/rewake` builds the catalogue value and passes it down, so a third harness is one
more argument there.

Tests:
- `internal/layout_test.go` `TestNoAdapterIsRegistered`
- `internal/layout_cases_test.go` `TestAnInitThatRegistersAnAdapterFails`
- `internal/layout_cases_test.go` `TestAPackageLevelVariableHoldingAdaptersFails`
- `internal/layout_discovery_test.go` `TestARegistryInATestFileFails`
- `internal/layout_discovery_test.go` `TestEveryFileIsReadWhateverItsConstraint`
- `internal/layout_discovery_test.go` `TestTwoDeclarationsOnOneLineAreTwo`
