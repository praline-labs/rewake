package workflow

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// The telemetry scenario's controls: four product mutants, each taking down
// one observation. There is no crosswise run over them yet — each breaks a
// different observation of one pair of sessions, read in the same run, so
// the scenario's own observations stand in for the cross.

// The tap does not look for the person's command: the status line goes blank.
var mutantTapWithoutOwner = mutation{
	name:  "tap-without-owner",
	file:  "internal/harness/claude/telemetry/tap.go",
	edits: []edit{{"\tcommand := ownerCommand(sources, caller)\n", "\tcommand := \"\"\n"}},
}

// The collector stops counting completed compactions.
var mutantUncountedCompaction = mutation{
	name:  "uncounted-compaction",
	file:  "internal/harness/claude/telemetry/state.go",
	edits: []edit{{"\t\ts.count++\n", "\n"}},
}

// The collector counts compactions but hands on no cue: the count stands and
// the main is never told.
var mutantSilentCompaction = mutation{
	name:  "silent-compaction",
	file:  "internal/harness/claude/telemetry/state.go",
	edits: []edit{{"\t\tsnapshot.CompactionEvents = append([]sessionstate.CompactionEvent{}, s.events...)\n", "\n"}},
}

// The collector shows the model's window whatever limit the person set: the
// listing and the header read the status line's 200K and 25%.
var mutantModelWindow = mutation{
	name:  "model-window",
	file:  "internal/harness/claude/telemetry/state.go",
	edits: []edit{{"\tcontext = capped(context, configured(s.limit, s.autocompact))\n", "\n"}},
}

func TestTheModelWindowInPlaceOfTheLimitFails(t *testing.T) {
	runTelemetryControl(t, mutantModelWindow, obsLimitWindow, obsHeader)
}

func TestASilentCompactionFails(t *testing.T) {
	runTelemetryControl(t, mutantSilentCompaction, obsCompactionNotice)
}

func TestATapWithoutTheOwnersLineFails(t *testing.T) {
	runTelemetryControl(t, mutantTapWithoutOwner, obsOwnerStatusShown)
}

// An uncounted compaction reads zero everywhere the count is shown: in the
// listing, in the header, and in the notice that never has a count to send.
func TestAnUncountedCompactionFails(t *testing.T) {
	runTelemetryControl(t, mutantUncountedCompaction, obsCompactionCount, obsHeader, obsCompactionNotice)
}

func runTelemetryControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControl(t, "claude-telemetry", playClaudeTelemetry, mutant, breaks...)
}

// runFindingsControl runs a Claude Code scenario against a mutant and requires
// exactly the named observations to break — judged, not merely unread — while
// every other one holds. A mutant that broke everything would otherwise pass
// as a control of whatever it was named for.
func runFindingsControl(t *testing.T, scenario string, play func(*testing.T, *Case, *Isolation) []telemetryFinding, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, claudeColumn.harness, scenario, play, mutant, breaks...)
}

// runFindingsControlOn is runFindingsControl on a column named by its harness.
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
