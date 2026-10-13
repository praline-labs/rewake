package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// TestFixtureDecideSandboxed is a recipient whose shell cannot connect() to
// any socket deciding a report it owes nobody can prove arrived
// (docs/v2/stage3-decision.md#the-command). The worker api owes main web a
// report; while api's turn has read the task and not yet ended, the
// publication mark of that report in web's mailbox is made unreadable, so
// the end meets an unknown and api's mailbox stops on it. Then:
//
//   - web's shell, under a seccomp filter that refuses connect, runs
//     rewake decide: the wrapper cannot be asked, the command exits 1 naming
//     the tool, and nothing is decided — a wrapper out of reach is never
//     taken for the person;
//   - web's tool call decides with the same words: the call runs in a child
//     of web's wrapper, outside the shell's sandbox, the wrapper checks it
//     and writes the decision verified as web's run;
//   - web's own view then lists its task closed by the decision, and api's
//     mailbox has nothing left to decide.
//
// Both sessions are fixtures. The sandbox is the program's (fixtureshim_sandbox_test.go),
// written for the architectures it names; on any other the case is
// unsupported for that capability.
func TestFixtureDecideSandboxed(t *testing.T) {
	binary := enterScenario(t, "fixture-decide-sandboxed")
	c := Start(t, Spec{
		Name:         "fixture-decide-sandboxed",
		Harness:      fixtureColumn.harness,
		Observations: decideObservations,
		Deadline:     120 * time.Second,
	})
	if !sandboxSupported() {
		for _, observation := range decideObservations {
			c.UnsupportedCapability(observation, "seccomp-connect-filter", "the program has no connect filter for this architecture")
		}
		return
	}
	for _, finding := range playDecideSandboxed(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsDecideShell  = "a decide from the recipient's shell that cannot connect exits 1 naming the tool, and nothing is decided"
	obsDecideTool   = "the same decide through the recipient's tool call is recorded verified, as the recipient's run"
	obsDecideClosed = "the recipient's own view then lists its task closed by the decision, and the sender's mailbox has nothing left to decide"
	decideTaskText  = "decide-sandboxed: do the work and report"
)

var decideObservations = []string{obsDecideShell, obsDecideTool, obsDecideClosed}

func playDecideSandboxed(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := fixtureColumn.harness
	room := iso.Room("default")
	endFile, goFile := filepath.Join(iso.Home, "api.end"), filepath.Join(iso.Home, "web.go")
	api := startHarnessSession(t, c, iso, col, "api", "--general", shimHoldEnd+"="+endFile, staysUp(iso, "api"))
	defer stopSession(t, c, api)
	calls, err := json.Marshal([]fixtureToolCall{{
		Tool: "decide", AfterFile: goFile,
		Arguments: map[string]any{"name": api.name, "report": "{after}", "arrived": true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	asks := newRequests(iso, "web")
	web := startHarnessSession(t, c, iso, col, "web", "--main", shimSendTo+"="+api.name, shimSendText+"="+decideTaskText,
		readinessSwitch(api), asks.env(), shimToolCalls+"="+string(calls))
	defer stopSession(t, c, web)

	// The wait api's read recorded for web gives the report's id before the
	// end publishes it.
	var report, webRun string
	var waits []mail.Waiter
	if !waitFor(c, 40*time.Second, func() bool {
		record, err := registry.Load(room, api.name)
		if err != nil || record.ServicePID == 0 {
			return false
		}
		waits, err = mail.ReadWaiters(room, api.name, record.Epoch())
		for _, wait := range waits {
			if err == nil && wait.Name == web.name {
				report, webRun = mail.ReportID(api.name, record.Epoch(), wait), wait.Epoch
				return true
			}
		}
		return false
	}) {
		return unjudgedAll(decideObservations, "api never recorded the wait for web's task: %+v", waits)
	}
	mark := mailtest.OncePath(room, web.name, webRun, report)
	if err := os.MkdirAll(filepath.Dir(mark), 0o700); err != nil {
		return unjudgedAll(decideObservations, "the mark's directory: %v", err)
	}
	if err := os.WriteFile(mark, []byte("damaged"), 0o600); err != nil {
		return unjudgedAll(decideObservations, "the unreadable mark: %v", err)
	}
	if err := os.WriteFile(endFile, nil, 0o600); err != nil {
		return unjudgedAll(decideObservations, "releasing api's end: %v", err)
	}
	if !waitFor(c, 30*time.Second, func() bool { return openSubject(room, api.name, report) }) {
		listing, err := mail.ListDecisions(room, api.name)
		return unjudgedAll(decideObservations, "api's mailbox never stopped on report %s: %+v %v", report, listing, err)
	}

	var out []telemetryFinding
	code, printed, ok := asks.askWith(c, shimRequest{Args: []string{"decide", api.name, report, "--arrived"}, Sandboxed: true})
	decided := decisionOn(room, api.name, report)
	out = append(out, judged(obsDecideShell,
		ok && code == 1 && decided == nil && strings.Contains(printed, "rewake tool's decide") && strings.Contains(printed, "operation not permitted"),
		"exit %d, answered %v, a decision %s: %q", code, ok, decisionText(decided), printed))

	if err := os.WriteFile(goFile, []byte(report), 0o600); err != nil {
		return append(out, unjudgedAll([]string{obsDecideTool, obsDecideClosed}, "the call's words: %v", err)...)
	}
	var records []fixtureToolRecord
	if !waitFor(c, 40*time.Second, func() bool {
		records, err = toolRecords(web)
		return err == nil && len(records) == 1
	}) {
		return append(out, unjudgedAll([]string{obsDecideTool, obsDecideClosed}, "web's turn made no call through the tool (%v): %s", err, web.acceptedTurns())...)
	}
	decided = decisionOn(room, api.name, report)
	verified := decided != nil && decided.By.Kind == mail.ByRecipient && decided.By.Verified && decided.By.Name == web.name && decided.By.Run == webRun
	out = append(out, judged(obsDecideTool, records[0].Error == "" && !records[0].IsError && verified,
		"the call %s; the decision %s", records[0], decisionText(decided)))

	var awaited string
	closed := decided != nil && waitFor(c, 20*time.Second, func() bool {
		_, awaited, _ = asks.ask(c, "inbox", "--awaited")
		return strings.Contains(awaited, "closed by decision "+decided.ID)
	})
	settled := !openSubject(room, api.name, report)
	out = append(out, judged(obsDecideClosed, closed && settled, "closed %v, api's mailbox settled %v; web's view: %q", closed, settled, awaited))
	return out
}

// openSubject says the report is a subject of the mailbox nobody decided yet.
func openSubject(room, name, report string) bool {
	listing, err := mail.ListDecisions(room, name)
	if err != nil {
		return false
	}
	for _, subject := range listing.Subjects {
		if subject.Subject.Report == report && subject.Decided == nil {
			return true
		}
	}
	return false
}

// decisionOn is the decision on the report, nil when there is none that
// reads.
func decisionOn(room, name, report string) *mail.Decision {
	views, err := mail.Decisions(room, name)
	if err != nil {
		return nil
	}
	for _, view := range views {
		if view.Report == report {
			return view.Decision
		}
	}
	return nil
}

// decisionText names a decision and its decider, without the observations
// its record keeps.
func decisionText(d *mail.Decision) string {
	if d == nil {
		return "none"
	}
	return d.ID + " " + d.Choice + " by " + d.By.Kind + " " + d.By.Name + " run " + d.By.Run + " verified " + strconv.FormatBool(d.By.Verified)
}
