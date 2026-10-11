package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// What the scenarios that answer by findings share, and the switches of the
// fixture harness's program they set. They were written beside the Claude Code
// column; that column retired in S9 until stage 4 gives Claude Code a turn
// boundary, and these stayed for the fixture's cases.

// telemetryFinding is one observation's answer. judged is false when the
// sessions' records could not be read at all, which is never "held". Named
// for the scenario it was first written for.
type telemetryFinding struct {
	observation string
	held        bool
	judged      bool
	detail      string
}

// show prints an optional value, or "unknown".
func show[T any](value *T) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprint(*value)
}

// runGateControl runs a scenario against a mutant on the gate column, where a
// control belongs since the gate's red is the one that blocks.
func runGateControl(t *testing.T, scenario string, play func(*testing.T, *Case, *Isolation) []telemetryFinding, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, gateColumn().harness, scenario, play, mutant, breaks...)
}

// runFindingsControlOn runs a scenario against a mutant on a column named by
// its harness, and requires exactly the named observations to break — judged,
// not merely unread — while every other one holds. A mutant that broke
// everything would otherwise pass as a control of whatever it was named for.
func runFindingsControlOn(t *testing.T, harness, scenario string, play func(*testing.T, *Case, *Isolation) []telemetryFinding, mutant mutation, breaks ...string) {
	t.Helper()
	name := scenario + "-control-" + mutant.name
	enterScenario(t, name)
	want := "the " + mutant.name + " mutant breaks " + strings.Join(breaks, "; ") + ", and nothing else"
	c := Start(t, Spec{
		Name:         name,
		Harness:      harness,
		Observations: []string{want},
		Deadline:     120 * time.Second,
	})
	binary, err := buildMutant(c, mutant)
	if err != nil {
		c.Contradicted(want, "the mutant could not be built: %v", err)
		return
	}
	findings := play(t, c, Isolate(t, c, binary))
	var wrong, broke []string
	for _, finding := range findings {
		expected := slices.Contains(breaks, finding.observation)
		switch {
		case !finding.judged:
			wrong = append(wrong, "could not judge "+finding.observation+": "+finding.detail)
		case expected && finding.held:
			wrong = append(wrong, finding.observation+" held anyway: "+finding.detail)
		case !expected && !finding.held:
			wrong = append(wrong, finding.observation+" broke too: "+finding.detail)
		case expected:
			broke = append(broke, finding.observation+": "+finding.detail)
		}
	}
	if len(broke) != len(breaks) && len(wrong) == 0 {
		wrong = append(wrong, fmt.Sprintf("the scenario made %d of the %d observations named", len(broke), len(breaks)))
	}
	if len(wrong) > 0 {
		c.Contradicted(want, "%s", strings.Join(wrong, "; "))
		return
	}
	c.Observed(want, strings.Join(broke, "; "))
}

// harnessArgs are the arguments the wrapper gave the harness: everything after
// the "--" the stand-in script puts between the test binary's own flags and
// them. Without that cut the strict parser read the test binary's path as an
// unknown option and refused every launch — which is how it was found.
func harnessArgs(args []string) []string {
	for index, arg := range args {
		if arg == "--" {
			return args[index+1:]
		}
	}
	// No separator means this was not started by the stand-in script, and an
	// empty list is then refused for its missing socket rather than guessed at.
	return nil
}

const (
	// shimInterruptFirst makes the session's first turn one a person
	// interrupts: the fixture harness's program reads its mail and ends that
	// turn interrupted.
	shimInterruptFirst = "RW_SHIM_INTERRUPT_FIRST_TURN"
	// shimPendingOnHold makes the continuation of the session's first held
	// turn run `rewake pending <text>` before it ends: a worker asked by the
	// hold that is still waiting on something. Later continuations only end
	// the turn.
	shimPendingOnHold = "RW_SHIM_PENDING_ON_HOLD"
)

// maxStopHolds is how many holds in a row the fixture honors in one turn
// before it ends the turn anyway — the bound Claude Code kept for its Stop
// hook (seen live on 2.1.280), kept so the fixture's holds end the same way.
const maxStopHolds = 8

// holdContinuation is what the fixture's model says when its turn was held
// and it has nothing to add.
const holdContinuation = "nothing to add after the hold"

var pendingOnHold sync.Once

// continueHeld is the model's turn after a hold: it may mark the turn, and
// answers its short last message.
func continueHeld() string {
	if text := os.Getenv(shimPendingOnHold); text != "" {
		pendingOnHold.Do(func() {
			out, err := exec.Command("rewake", "pending", text).CombinedOutput()
			line := "pending on hold ok"
			if err != nil {
				line = "pending on hold refused: " + err.Error() + ": " + string(out)
			}
			(&shimSession{}).recordTurn(line)
		})
	}
	return holdContinuation
}

// eventEnds counts the turn ends a session acted on that were named by their
// event: each leaves a journal named from its run and event, 32 hex digits,
// kept while the run lives; one that named no event has a drawn name
// (docs/turn-end-recovery.md#the-operation).
func eventEnds(iso *Isolation, name string) int {
	found, _ := filepath.Glob(filepath.Join(iso.StateDir, "rooms", "default", "inbox", name, "journal", "*"))
	count := 0
	for _, path := range found {
		if eventNamed.MatchString(strings.TrimSuffix(filepath.Base(path), ".done")) {
			count++
		}
	}
	return count
}

var eventNamed = regexp.MustCompile(`^[0-9a-f]{32}$`)
