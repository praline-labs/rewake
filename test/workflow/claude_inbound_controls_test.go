package workflow

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// The inbound scenario's controls: product mutants, each naming every
// observation it must break and requiring the others to hold.

// The inbox server delivers the first notice without waiting for the session.
var mutantUngated = mutation{
	name:  "ungated",
	file:  "internal/inbox/serve.go",
	edits: []edit{{"\tif s.gated() {\n", "\tif false && s.gated() {\n"}},
}

// The first notice waits for SessionStart instead of the status line: the
// order the owner first asked for, which live runs showed is too early.
var mutantGateOnSessionStart = mutation{
	name:  "gate-on-session-start",
	file:  "internal/harness/claude/telemetry/collector.go",
	edits: []edit{{"\t\tif event.Kind == StatusLine {\n", "\t\tif event.Kind == SessionStart {\n"}},
}

// A held receipt is read as a delivery, which is what rewake said before it
// listened for receipts.
var mutantHeldAsDelivered = mutation{
	name:  "held-as-delivered",
	file:  "internal/harness/claude/receipt.go",
	edits: []edit{{"\t\treturn inbox.Result{State: inbox.Held, Via: \"socket\"", "\t\treturn inbox.Result{State: inbox.Delivered, Via: \"socket\""}},
}

// A refusal is read as a delivery.
var mutantRefusalAsDelivered = mutation{
	name:  "refusal-as-delivered",
	file:  "internal/harness/claude/receipt.go",
	edits: []edit{{"inbox.Result{State: inbox.Failed, Via: \"socket\", Detail: because(\"the session refuses", "inbox.Result{State: inbox.Delivered, Via: \"socket\", Detail: because(\"the session refuses"}},
}

// A word about a line counted delivered from silence is dropped, as it was
// while rewake trusted its 300 ms window.
var mutantLateWordDropped = mutation{
	name:  "late-word-dropped",
	file:  "internal/harness/claude/lane.go",
	edits: []edit{{"	if !waiting.at.IsZero() && time.Since(waiting.at) > inbox.LateWordWindow {\n", "	if !waiting.at.IsZero() {\n"}},
}

// A held task that expires is settled, and its sender is never told.
var mutantExpiryUnannounced = mutation{
	name:  "expiry-unannounced",
	file:  "internal/inbox/held.go",
	edits: []edit{{"\t\ts.tellUndelivered(message, outcome.Detail)\n", "\n"}},
}

func TestAnUngatedFirstNoticeFails(t *testing.T) {
	runInboundControl(t, mutantUngated, obsStartAccepted)
}

func TestAGateOnSessionStartFails(t *testing.T) {
	runInboundControl(t, mutantGateOnSessionStart, obsStartAccepted)
}

func TestAHeldNoticeReportedDeliveredFails(t *testing.T) {
	runInboundControl(t, mutantHeldAsDelivered, obsReleaseReported, obsHeldReported, obsExpiryTold, obsExpiryUnsettled, obsLateHoldTold)
}

func TestAnUnannouncedExpiryFails(t *testing.T) {
	runInboundControl(t, mutantExpiryUnannounced, obsExpiryTold, obsLateHoldTold)
}

func TestARefusalReportedDeliveredFails(t *testing.T) {
	runInboundControl(t, mutantRefusalAsDelivered, obsRefusalReported)
}

func TestADroppedLateWordFails(t *testing.T) {
	runInboundControl(t, mutantLateWordDropped, obsLateHoldTold)
}

func runInboundControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	name := "claude-inbound-control-" + mutant.name
	enterScenario(t, name)
	want := "the " + mutant.name + " mutant breaks " + strings.Join(breaks, "; ") + ", and nothing else"
	c := Start(t, Spec{Name: name, Harness: claudeColumn.harness, Observations: []string{want}, Deadline: 150 * time.Second})
	binary, err := buildMutant(c, mutant)
	if err != nil {
		c.Contradicted(want, "the mutant could not be built: %v", err)
		return
	}
	var wrong, broke []string
	for _, finding := range playClaudeInbound(t, c, Isolate(t, c, binary)) {
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
	if len(wrong) == 0 && len(broke) != len(breaks) {
		wrong = append(wrong, fmt.Sprintf("%d of the %d named observations were made", len(broke), len(breaks)))
	}
	if len(wrong) > 0 {
		c.Contradicted(want, "%s", strings.Join(wrong, "; "))
		return
	}
	c.Observed(want, strings.Join(broke, "; "))
}
