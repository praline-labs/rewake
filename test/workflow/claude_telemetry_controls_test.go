package workflow

import (
	"testing"
	"time"
)

// The telemetry scenario's controls: two product mutants, each taking down
// one observation. There is no crosswise run over them yet — each breaks a
// different observation of a single session, and the other observations are
// read from the same listing in the same run, so the scenario's own
// observations stand in for the cross.

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

func TestATapWithoutTheOwnersLineFails(t *testing.T) {
	runTelemetryControl(t, mutantTapWithoutOwner, obsOwnerStatusShown)
}

func TestAnUncountedCompactionFails(t *testing.T) {
	runTelemetryControl(t, mutantUncountedCompaction, obsCompactionCount)
}

// runTelemetryControl runs the scenario against a mutant and requires the
// named observation to break — judged, not merely unread.
func runTelemetryControl(t *testing.T, mutant mutation, observation string) {
	t.Helper()
	name := "claude-telemetry-control-" + mutant.name
	enterScenario(t, name)
	want := "the " + mutant.name + " mutant breaks: " + observation
	c := Start(t, Spec{
		Name:         name,
		Harness:      claudeColumn.harness,
		Observations: []string{want},
		Deadline:     120 * time.Second,
	})
	binary, err := buildMutant(c, mutant)
	if err != nil {
		c.Contradicted(want, "the mutant could not be built: %v", err)
		return
	}
	for _, finding := range playClaudeTelemetry(t, c, Isolate(t, c, binary)) {
		if finding.observation != observation {
			continue
		}
		switch {
		case !finding.judged:
			c.Contradicted(want, "could not judge: %s", finding.detail)
		case finding.held:
			c.Contradicted(want, "held anyway: %s", finding.detail)
		default:
			c.Observed(want, finding.detail)
		}
		return
	}
	c.Contradicted(want, "the scenario made no such observation")
}
