package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// The live set on the fixture column (S11,
// docs/v2/stage3-steps-adapters.md#s11-the-host-on-the-live-set-codex): each
// run's record carries what its readiness exchange proved, and the decisions
// other sessions make about it read that, never the offer. Every absence goes
// through the program's own switch:
//
//   - full proves everything.
//   - wakeless withholds Wake: a note and a task to it are accepted at once,
//     exit 3, and wait readable in its inbox.
//   - dropper closes its connection after its first turn: the whole set is
//     withdrawn at once, and a task to it fails, exit 1.
//   - stuck withholds the boundary as a worker: its launch is refused.
//
// The steering refusal without a live Control is fixture-steered's bare.

const (
	obsLiveRecorded = "each run's record carries what its exchange proved: everything for a full run, nothing it withheld"
	obsLiveUnwoken  = "a letter to a run without a live Wake is accepted at once, exit 3, saying no notice can reach it, and waits readable in its inbox, a task as a note"
	obsLiveDropped  = "a connection lost after a turn withdraws the run's whole live set at once, and a task to it then fails, exit 1, with nothing sent"
	obsLiveLaunch   = "a worker whose run proves no turn boundary is refused at launch, exit 1, saying so, and leaves no session"

	liveTask   = "fixture-live: do the work and report"
	liveNote   = "fixture-live: a heads-up"
	liveSecond = "fixture-live: a second task"
)

var liveObservations = []string{obsLiveRecorded, obsLiveUnwoken, obsLiveDropped, obsLiveLaunch}

func TestFixtureLive(t *testing.T) {
	binary := enterScenario(t, "fixture-live")
	c := Start(t, Spec{
		Name:         "fixture-live",
		Harness:      fixtureColumn.harness,
		Observations: liveObservations,
		Deadline:     120 * time.Second,
	})
	for _, finding := range playLive(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

// liveRow is one session of main's listing: its record, with its live set.
type liveRow struct {
	registry.Session
}

// liveWorld is the case's sessions and main's view of them.
type liveWorld struct {
	c    *Case
	iso  *Isolation
	asks *requests
}

// rows is main's listing now, by name.
func (w liveWorld) rows() map[string]liveRow {
	code, out, ok := w.asks.ask(w.c, "list", "--json")
	var listed struct {
		Sessions []liveRow `json:"sessions"`
	}
	if !ok || code != 0 || json.Unmarshal([]byte(out), &listed) != nil {
		return nil
	}
	rows := map[string]liveRow{}
	for _, row := range listed.Sessions {
		rows[row.Name] = row
	}
	return rows
}

// await waits for a session's row to satisfy held, and answers the last row seen.
func (w liveWorld) await(window time.Duration, name string, held func(liveRow) bool) (liveRow, bool) {
	var last liveRow
	ok := waitFor(w.c, window, func() bool {
		row, found := w.rows()[name]
		last = row
		return found && held(row)
	})
	return last, ok
}

var fullLive = []string{"wake", "turn-boundary", "telemetry", "control"}

func playLive(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := fixtureColumn.harness

	full := startHarnessSession(t, c, iso, col, "full", "--general", staysUp(iso, "full"))
	defer stopSession(t, c, full)
	wakeless := startHarnessSession(t, c, iso, col, "wakeless", "--general", shimNoWake+"=1", staysUp(iso, "wakeless"))
	defer stopSession(t, c, wakeless)
	dropper := startHarnessSession(t, c, iso, col, "dropper", "--general", shimDropAfter+"=1", shimInboxJSON+"=1", staysUp(iso, "dropper"))
	defer stopSession(t, c, dropper)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)
	w := liveWorld{c: c, iso: iso, asks: asks}

	var out []telemetryFinding
	finding := func(observation string, held bool, detail string, args ...any) {
		out = append(out, telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)})
	}
	for _, session := range []*scenarioSession{full, wakeless, dropper, lead} {
		if !sessionReady(c, session) {
			for _, observation := range liveObservations {
				out = append(out, telemetryFinding{observation: observation, detail: session.name + " never became ready"})
			}
			return out
		}
	}

	code, said, refused := refusedLaunch(t, c, iso, col, "stuck", "--general", shimNoBoundary+"=1")
	_, stayed := w.rows()["stuck-"+col]
	finding(obsLiveLaunch, refused && code == 1 && strings.Contains(said, "did not make its turn boundary live") && !stayed,
		"exit %d (ended %t), left a session %t: %s", code, refused, stayed, said)

	rows := w.rows()
	recorded := slices.Equal(rows[full.name].Live, fullLive) && !slices.Contains(rows[wakeless.name].Live, "wake") &&
		slices.Contains(rows[wakeless.name].Live, "turn-boundary")
	finding(obsLiveRecorded, recorded, "full %v, wakeless %v", rows[full.name].Live, rows[wakeless.name].Live)

	unwoken, detail := w.unwoken(wakeless, rows[wakeless.name].Epoch())
	finding(obsLiveUnwoken, unwoken, "%s", detail)

	out = append(out, playDropped(w, dropper)...)

	return out
}

// playDropped sends dropper its task, waits for the report and the drop, and
// judges what the record and a second task show.
func playDropped(w liveWorld, dropper *scenarioSession) []telemetryFinding {
	unjudged := []telemetryFinding{{observation: obsLiveDropped}}
	code, said, _ := w.asks.ask(w.c, "send", dropper.name, liveTask)
	if code != 0 && code != 3 {
		for i := range unjudged {
			unjudged[i].detail = fmt.Sprintf("the first task was not accepted: exit %d, %q", code, firstLine(said))
		}
		return unjudged
	}
	gone, withdrawn := w.await(30*time.Second, dropper.name, func(r liveRow) bool { return len(r.Live) == 0 })
	code, said, _ = w.asks.ask(w.c, "send", dropper.name, liveSecond)
	sent := inboxHolds(w.iso, dropper.name, liveSecond)
	return []telemetryFinding{
		{
			observation: obsLiveDropped, judged: true,
			held:   withdrawn && code == 1 && strings.Contains(said, "turn boundary is not live") && !sent,
			detail: fmt.Sprintf("live after the drop %v (withdrawn %t); the second task: exit %d, %q, in its inbox %t", gone.Live, withdrawn, code, firstLine(said), sent),
		},
	}
}

// inboxHolds says whether any letter in a session's inbox carries the text.
func inboxHolds(iso *Isolation, name, text string) bool {
	found, _ := filepath.Glob(filepath.Join(iso.Room(iso.HomeRoom()), "inbox", name, "*.json"))
	for _, path := range found {
		if raw, err := os.ReadFile(path); err == nil && strings.Contains(string(raw), text) {
			return true
		}
	}
	return false
}

// refusedLaunch launches a session expected to be refused and answers its
// exit code, its standard error, and whether it ended within the bound; one
// that did not is asked to stop.
func refusedLaunch(t *testing.T, c *Case, iso *Isolation, harness, name, role string, controls ...string) (int, string, bool) {
	t.Helper()
	session, launch := sessionLaunch(t, c, iso, "", harness, name, role, nil, nil, controls...)
	release := c.captureStderr(launch, iso.Home, name)
	process, err := c.start(launch, true)
	release()
	if err != nil {
		t.Fatalf("launching %s: %v", name, err)
	}
	session.process = process
	ended := waitFor(c, 20*time.Second, process.finished)
	if !ended {
		_ = session.stop(c)
	}
	_ = c.wait(process)
	stderr, _ := os.ReadFile(filepath.Join(iso.Home, name+".stderr"))
	code := -1
	if process.cmd.ProcessState != nil {
		code = process.cmd.ProcessState.ExitCode()
	}
	return code, string(stderr), ended
}

// unwoken sends a note and a task to a run without a live Wake and says
// whether each was accepted at once, exit 3, with the promise that the
// session reads it, and whether the promise holds once the run's server has tried it: the
// letter readable in the inbox, its stored status still waiting.
func (w *liveWorld) unwoken(session *scenarioSession, epoch string) (bool, string) {
	room := w.iso.Room(w.iso.HomeRoom())
	held, details := true, []string{}
	for _, text := range []string{liveNote, liveTask} {
		args := []string{"send", session.name, text, "--json"}
		if text == liveNote {
			args = append(args, "--notify")
		}
		code, said, _ := w.asks.ask(w.c, args...)
		var accepted struct {
			ID     string `json:"id"`
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal([]byte(said), &accepted)
		var status mail.Status
		var unread []mail.Message
		tried := accepted.ID != "" && waitFor(w.c, 8*time.Second, func() bool {
			status, _, _ = mail.ReadStatus(room, session.name, accepted.ID)
			unread, _ = mail.PeekUnread(room, session.name, epoch)
			// The coalescing window keeps a note pending before its try.
			return status.State == mail.Failed || strings.Contains(status.Detail, "no notice can reach this session")
		})
		readable := slices.ContainsFunc(unread, func(m mail.Message) bool { return m.ID == accepted.ID })
		// The answer is send's own, decided by the live set before any try:
		// a send that waited for the server's try would carry its detail.
		decided := strings.HasPrefix(accepted.Detail, "accepted; no notice can reach this session")
		ok := code == 3 && decided && tried && readable && status.State == mail.Pending
		held = held && ok
		details = append(details, fmt.Sprintf("%q: exit %d, answered %q, readable %t, stored %s %q", text, code, accepted.Detail, readable, status.State, status.Detail))
	}
	return held, strings.Join(details, "; ")
}
