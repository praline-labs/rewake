package workflow

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/test/workflow/record"
)

// switchEnv turns the scenarios on. They skip themselves without it so that
// the five required checks stay cheap and stay at five commands, while the
// code is still compiled and analyzed by every one of them.
//
// A person or the orchestrator sets it explicitly:
//
//	REWAKE_WORKFLOW=1 go test ./test/workflow/...
const switchEnv = "REWAKE_WORKFLOW"

// suite is the state TestMain owns: whether scenarios are on, the rewake
// binary built for this run, and which scenarios actually ran.
var suite struct {
	enabled bool
	binary  string

	mu  sync.Mutex
	ran []string
	// schema is where the schema comes from, decided before any case
	// starts, and schemaUsed whether a case asked for it; see
	// harness_version_test.go.
	schema         schemaSource
	schemaPrepared bool
	schemaUsed     bool
}

func TestMain(m *testing.M) {
	// Re-executed as `codex` by a scenario: be the harness and nothing else.
	// Checked before anything else because the ordinary path would otherwise
	// build a binary and run a suite inside the shim.
	if os.Getenv(shimEnv) != "" {
		recordShimCall(os.Args)
		if os.Getenv(shimHarness) == "claude" {
			os.Exit(runClaudeShim(os.Args))
		}
		os.Exit(runShim(os.Args))
	}
	enabled, err := switchedOn(os.Getenv(switchEnv))
	if err != nil {
		fmt.Fprintf(os.Stderr, "workflow: %v\n", err)
		os.Exit(2)
	}
	suite.enabled = enabled
	// Parsed here rather than by m.Run, because the fetch before the cases
	// takes its budget from -timeout.
	flag.Parse()
	os.Exit(run(m))
}

// switchedOn reads the switch. An unset variable is off and "0" is off too:
// reading any value as "on" would turn REWAKE_WORKFLOW=0 into a switch that
// means the opposite of what it says. An unrecognized value stops the run
// rather than guessing, because guessing here is silent.
func switchedOn(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "off":
		return false, nil
	case "1", "true", "yes", "on":
		return true, nil
	default:
		return false, fmt.Errorf("%s=%q is neither on (1, true, yes, on) nor off (0, false, no, off)", switchEnv, value)
	}
}

func run(m *testing.M) int {
	// Before anything is started: orphaned descendants are re-parented here
	// rather than to init, which is the only way a process that left its
	// group can still be found. See adopted_test.go.
	if err := becomeSubreaper(); err != nil {
		fmt.Fprintf(os.Stderr, "workflow: %v\n", err)
		return 1
	}
	if suite.enabled {
		dir, err := os.MkdirTemp("", "rewake-workflow-")
		if err != nil {
			fmt.Fprintf(os.Stderr, "workflow: temporary directory: %v\n", err)
			return 1
		}
		defer func() { _ = os.RemoveAll(dir) }()
		binary, err := buildRewake(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "workflow: %v\n", err)
			return 1
		}
		suite.binary = binary
		suite.schema, suite.schemaPrepared = prepareSchemaSource()
		if suite.schema.err != nil {
			// Stated before the cases run, and again in the run record: a
			// named version that could not be had is the run's failure
			// whether or not the schema case is among those selected.
			fmt.Fprintf(os.Stderr, "workflow: %v\n", suite.schema.err)
		}
	}

	if err := takePoolWidth(); err != nil {
		fmt.Fprintf(os.Stderr, "workflow: -parallel: %v\n", err)
		return 2
	}
	code := m.Run()
	var failures []string
	if suite.schema.err != nil {
		failures = append(failures, suite.schema.err.Error())
	}
	// Every case has finished and swept what carried its label. Whatever is
	// still below this process was accounted for by no case — an unlabeled
	// descendant, or one a sweep could not read — and the run fails on it:
	// the guarantee that nothing a case started survives it now ends here.
	// It goes into the run record as well, because no case record will say
	// it, and a summary would otherwise report a red engine and nothing more.
	if left := finalSweep(); len(left) > 0 {
		leftover := "processes outlived their cases: " + joinStrings(left, "; ")
		fmt.Fprintf(os.Stderr, "workflow: %s\n", leftover)
		failures = append(failures, leftover)
		if code == 0 {
			code = 1
		}
	}
	// Published on both paths. A failing run needs its scenario count as much
	// as a passing one — more, because the first question about a red run is
	// whether the case that failed is the only thing that ran — and the
	// earlier version reported it only when everything had passed.
	scenarios := ranScenarios()
	against := againstForRun()
	failure := joinStrings(failures, "; ")
	publishRun(suite.enabled, scenarios, against, failure)
	if code != 0 {
		return code
	}
	if failure != "" {
		return 1
	}
	return reportScenarios(scenarios, against)
}

// ranScenarios is what actually ran, in a stable order.
func ranScenarios() []string {
	suite.mu.Lock()
	ran := append([]string(nil), suite.ran...)
	suite.mu.Unlock()
	sort.Strings(ran)
	return slices.Compact(ran)
}

// reportScenarios says how many scenarios ran, and refuses a green run that
// exercised nothing. "Switched the suite on and got green" must not be
// indistinguishable from "switched it on and it ran nothing".
//
// The zero case is caught by the exit code and is always visible. The count
// itself is only visible under `go test -v`, because `go test` shows neither
// stdout nor stderr of a package that passed — which is why the documented
// command in AGENTS.md carries -v. Without it, three scenarios where four were
// expected still pass in silence.
func reportScenarios(ran []string, against []record.Against) int {
	if !suite.enabled {
		fmt.Printf("workflow: scenarios skipped, set %s=1 to run them\n", switchEnv)
		return 0
	}
	if len(ran) == 0 {
		fmt.Fprintf(os.Stderr, "workflow: %s is set but no scenario ran\n", switchEnv)
		return 1
	}
	fmt.Printf("workflow: %d scenario(s) ran: %v\n", len(ran), ran)
	// Beside the count, on purpose: a green run says what it was green
	// against, and the version is the first thing to doubt about a harness.
	fmt.Printf("workflow: %s\n", record.DescribeAgainst(against))
	return 0
}

// buildTimeout bounds the build. It runs before m.Run, so `go test`'s own
// timer has not started yet and a wedged build would otherwise hang the run
// with no diagnostic at all.
const buildTimeout = 3 * time.Minute

// buildRewake builds the binary under test into dir. Scenarios must never find
// `rewake` on the developer's PATH: a green result obtained against somebody
// else's build, of unknown age, proves nothing about this working tree.
func buildRewake(dir string) (string, error) {
	root, err := moduleRoot()
	if err != nil {
		return "", err
	}
	ctx, stop := context.WithTimeout(context.Background(), buildTimeout)
	defer stop()
	binary := filepath.Join(dir, "rewake")
	flags, err := buildFlags(dir)
	if err != nil {
		return "", err
	}
	// No VCS stamp: the suite asserts nothing about the build line, and a
	// copy without .git would fail on the stamp.
	build := exec.Command("go", append(append([]string{"build", "-buildvcs=false"}, flags...), "-o", binary, "./cmd/rewake")...)
	build.Dir = root
	build.Env = os.Environ()
	// Through the same group machinery a case uses: `go build` starts
	// compilers and linkers of its own, and any of them can hold the output
	// pipe open past the context if only the immediate child is killed.
	out, err := outputBounded(ctx, "go build", build)
	if err != nil {
		return "", fmt.Errorf("building rewake: %w: %s", err, out)
	}
	return binary, nil
}

// moduleRoot finds the repository root from the package directory, and checks
// it rather than trusting the relative path: a moved package would otherwise
// build whatever happened to be two directories up.
func moduleRoot() (string, error) {
	here, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(filepath.Join(here, "..", ".."))
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", fmt.Errorf("no go.mod at %s, so the suite cannot tell what to build: %w", root, err)
	}
	return root, nil
}

// enterScenario gates a scenario on the switch and records that it ran. It
// returns the path of the binary built for this run, which is the only rewake
// a scenario may execute.
//
// The scenario runs in the pool, beside other cases (pool_test.go).
func enterScenario(t *testing.T, name string) string {
	t.Helper()
	binary := enterSerialScenario(t, name)
	joinPool(t, name)
	return binary
}

// enterSerialScenario is enterScenario for a scenario that must not overlap
// another case: one that changes what the whole test process shares, such as
// its environment through t.Setenv. It runs in the serial phase, before the
// pool starts.
func enterSerialScenario(t *testing.T, name string) string {
	t.Helper()
	if !suite.enabled {
		t.Skipf("scenario %q skipped: set %s=1 to run the workflow suite", name, switchEnv)
	}
	if suite.binary == "" {
		t.Fatalf("scenario %q has no binary to run: the build in TestMain did not happen", name)
	}
	suite.mu.Lock()
	suite.ran = append(suite.ran, name)
	suite.mu.Unlock()
	return suite.binary
}
