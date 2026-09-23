package workflow

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// The telemetry scenario's controls: three product mutants, each taking down
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

// runTelemetryControl runs the scenario against a mutant and requires exactly
// the named observations to break — judged, not merely unread — while every
// other one holds. A mutant that broke everything would otherwise pass as a
// control of whatever it was named for.
func runTelemetryControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	name := "claude-telemetry-control-" + mutant.name
	enterScenario(t, name)
	want := "the " + mutant.name + " mutant breaks " + strings.Join(breaks, "; ") + ", and nothing else"
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
	findings := playClaudeTelemetry(t, c, Isolate(t, c, binary))
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
