package workflow

import (
	"slices"
	"testing"
	"time"
)

// TestCodexTerminalOfALaterShape is a Codex terminal that starts or resumes
// its conversation in the form 0.157.1 sends: no workspace roots, the
// terminal's configuration with its web_search mode instead, and on a resume
// the history and the path left null. Until September 26, 2026 the gateway
// took only the roots or the permissions for the terminal's selection, never
// selected such a conversation, and every delivery to it failed as
// unavailable — seen live on 0.157.1 in a container without a network
// (docs/research-codex.md).
//
//   - fresh starts its conversation so, and a task sent to it is finished;
//   - resumed resumes its conversation so, and a task sent to it is finished.
//
// What it does not prove: that a real 0.157.1 terminal sends these forms on
// every path — the probe drove the first launch, /new, /resume and a launch
// with resume, not a fork — or that a later version keeps them.
func TestCodexTerminalOfALaterShape(t *testing.T) {
	binary := enterScenario(t, "codex-tui-later-shape")
	c := Start(t, Spec{
		Name:         "codex-tui-later-shape",
		Harness:      codexColumn.harness,
		Observations: laterShapeObservations,
		Deadline:     60 * time.Second,
	})
	for _, finding := range playLaterShape(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsLaterStart  = "a terminal starting its conversation as 0.157.1 does is selected, and a task sent to it is finished"
	obsLaterResume = "a terminal resuming its conversation as 0.157.1 does is selected, and a task sent to it is finished"
	laterShapeTask = "later-shape: a task for a terminal of 0.157.1"
)

var laterShapeObservations = []string{obsLaterStart, obsLaterResume}

func playLaterShape(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := codexColumn.harness
	shape := shimTUIShape + "=0.157.1"
	fresh := startHarnessSession(t, c, iso, col, "fresh", "--general", shimInboxJSON+"=1", shape, staysUp(iso, "fresh"))
	defer stopSession(t, c, fresh)
	resumed := startHarnessSession(t, c, iso, col, "resumed", "--general", shimInboxJSON+"=1", shape, shimResume+"=1", staysUp(iso, "resumed"))
	defer stopSession(t, c, resumed)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)
	sg := steering{c: c, asks: asks, lead: lead}

	var out []telemetryFinding
	for _, worker := range []struct {
		session     *codexSession
		observation string
	}{{fresh, obsLaterStart}, {resumed, obsLaterResume}} {
		code, sent, _ := asks.ask(c, "send", worker.session.name, laterShapeTask)
		id := printedID(sent)
		var kinds []string
		finished := waitFor(c, 20*time.Second, func() bool {
			kinds = reportKinds(sg.about(worker.session, id))
			return slices.Contains(kinds, "finished")
		})
		out = append(out, judged(worker.observation, finished,
			"send exit %d, %q; main read of it %v; its status %q", code, firstLine(sent), kinds, statusState(iso, worker.session, id)))
	}
	return out
}

// The control: the gateway as it was, taking only the roots or the
// permissions for the terminal's selection.
var mutantRootsOnlyRecognition = mutation{
	name: "roots-only-recognition",
	file: "internal/harness/codex/gateway/state.go",
	edits: []edit{
		{`m.source == "user" && (m.roots || m.permissions || m.tuiConfig)`, `m.source == "user" && (m.roots || m.permissions)`},
		{` || m.tuiConfig && m.byID || m.reconnect)`, ` || m.reconnect)`},
	},
}

func TestATerminalOfALaterShapeUnrecognizedFails(t *testing.T) {
	runFindingsControlOn(t, codexColumn.harness, "codex-tui-later-shape", playLaterShape, mutantRootsOnlyRecognition, obsLaterStart, obsLaterResume)
}
