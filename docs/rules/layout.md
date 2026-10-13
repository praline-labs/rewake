# Rules: the layout

What each layer of the tree may import, where a harness may be named, and how adapters
reach the program, held by one test over the whole module, `internal/layout_test.go`,
with the five checks. The layout and its reasons are
[design.md](../v2/design.md#the-layout); where each package lives is
[code.md](../code.md), whose tree `docs/layout_test.go` holds equal to the module's
packages. The rules have no numbers: each section below is one rule, ends with the tests
that hold it, and is known to the rules test by its heading.

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
decision (`AGENTS.md`, "Code"). A forbidden edge fails naming both packages and the file
that makes it, and the build tag when only a tagged build has it; a package in no layer
fails; a directory with a module of its own is no package of this one. The mail
functions that run under a mailbox lock their caller holds keep the callers the test
lists, since the lock is not reentrant and nothing at run time would say a caller is
outside its section.

Tests:
- `internal/layout_test.go` `TestImportsFollowTheLayers`
- `internal/layout_cases_test.go` `TestAForbiddenEdgeFailsNamingBothPackagesAndTheFile`
- `internal/layout_cases_test.go` `TestAnEdgeOnlyATagBuildsFailsNamingTheTag`
- `internal/layout_cases_test.go` `TestAPackageInNoPlaceFails`
- `internal/layout_cases_test.go` `TestOneAdapterMayNotImportAnother`
- `internal/layout_discovery_test.go` `TestEveryImportReachesTheRule`
- `internal/layout_discovery_test.go` `TestARaceOnlyFileIsJudged`
- `internal/layout_discovery_test.go` `TestADirectoryWithItsOwnModuleIsNoPackage`
- `internal/layout_callers_test.go` `TestTheLockedEntryPointsKeepTheirCallers`
- `internal/layout_callers_test.go` `TestCallerProblemsSeeEveryMismatch`
- `docs/layout_test.go` `TestEveryPackageIsInTheLayout`

## Harness names outside the adapters

No Go file under `core`, `tool`, `host`, `cli` or `infra`, tests included and whatever
its build constraint, names a harness in an identifier, a string literal, read by its
value, or a comment: the ids and titles the catalogue declares, compared without case,
and the words that belong to one harness (`mcp__`, `CLAUDE_`, `app-server`, `.claude`,
`.codex`, `.agents`). A comment counts on purpose: a comment that explains core code by
one harness's behaviour is the first step toward a branch on it. A harness built only for
tests is found by its title anywhere and by its id as a whole string literal, since its id
is also the plain noun the core's tests use for their fakes; every shipped harness keeps
the whole rule. A word that contains another fails once.

Tests:
- `internal/layout_test.go` `TestNoHarnessIsNamedOutsideTheAdapters`
- `internal/layout_cases_test.go` `TestANameFailsInACommentAStringAndAnIdentifier`
- `internal/layout_cases_test.go` `TestAStringIsReadByItsValue`
- `internal/layout_cases_test.go` `TestAWordContainingAnotherFailsOnce`
- `internal/layout_discovery_test.go` `TestEveryFileIsReadWhateverItsConstraint`
- `internal/layout_names_policy_test.go` `TestATestOnlyHarnessIsFoundByItsTitleAndItsIDAsALiteral`
- `internal/layout_names_policy_test.go` `TestAShippedHarnessIsFoundByItsIDInEveryText`
- `internal/layout_names_policy_test.go` `TestTheTestOnlyIDIsALiteralAndNotAWord`

## No registry

No package under `adapter` declares `func init`, and no package-level variable of an
adapter type, or of a slice or map of them, exists outside `adapter/catalog`'s
constructor, in a test file as in any other; two declarations on one line are two.
`cmd/rewake` builds the catalogue value and passes it down, so a third harness is one
more argument there.

Tests:
- `internal/layout_test.go` `TestNoAdapterIsRegistered`
- `internal/layout_cases_test.go` `TestAnInitThatRegistersAnAdapterFails`
- `internal/layout_cases_test.go` `TestAPackageLevelVariableHoldingAdaptersFails`
- `internal/layout_discovery_test.go` `TestARegistryInATestFileFails`
- `internal/layout_discovery_test.go` `TestTwoDeclarationsOnOneLineAreTwo`

## The exception tables

What one of the rules above finds and the tree still has to keep is an entry in the
test's own tables — an import, a name, a registration — with its reason, the number of
findings it excuses and a digest of which, so one more beside them fails, and an entry
nothing matches fails. Every table is empty, the transition and file-group tables of
stage 3's moves with them; an entry is a decision recorded there, never a finding let
through.

Tests:
- `internal/layout_test.go` `TestTheTablesShrinkWithTheMoves`
- `internal/layout_cases_test.go` `TestAnExceptionNothingMatchesFails`
- `internal/layout_cases_test.go` `TestATransitionEntryAtItsPlaceOrGoneFails`
- `internal/layout_cases_test.go` `TestAFileGroupWithAFileGoneFails`
- `internal/layout_cases_test.go` `TestAPackageThatStaysImportsNothingAStepRemoves`
- `internal/layout_discovery_test.go` `TestANameExceptionAdmitsNoNewMention`
- `internal/layout_discovery_test.go` `TestAnInitExceptionAdmitsNoSecondInit`
- `internal/layout_discovery_test.go` `TestOnlyTheBuildOrderNamesSteps`
