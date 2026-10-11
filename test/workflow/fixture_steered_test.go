package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestFixtureSteered is `rewake compact` and `rewake interrupt` end to end on
// the gate column: a main steers three fixture workers, and each request
// travels the command, the control directory, the program serving it, the
// session carrying it out, and back — the worker's compaction reaching main's
// wrapper through the adapter's snapshot as a letter.
//
//   - calm works its task to the end and is idle: a compaction with a focus is
//     answered started while it still runs, before the telemetry counts it;
//     its end reaches main as a letter with the counts and the compaction's
//     number, and main, which asked, gets no notice of it besides. An
//     interrupt is refused as no turn running. Then calm is killed in the
//     middle of a second compaction, and main still gets its letter, from its
//     own wrapper's record.
//   - busy is held in its first turn: a compaction is refused as in a turn and
//     the turn goes on; an interrupt ends it, and main reads stopped naming
//     itself, as its list of what it is owed does.
//   - bare serves no control, so its run never makes Control live: every
//     request is refused before anything is asked, exit 1.
//
// And a worker asking is a wrong call.
//
// The fixture is the only harness offering Control from S9 until stage 4 gives a
// real adapter Control again; Claude Code's served side left with its plugin
// module. What it does not prove: that any real harness carries these calls
// out as the program does; the refusals are the words of internal/core/control.
func TestFixtureSteered(t *testing.T) {
	binary := enterScenario(t, "fixture-steered")
	c := Start(t, Spec{
		Name:         "fixture-steered",
		Harness:      fixtureColumn.harness,
		Observations: steeredObservations,
		Deadline:     120 * time.Second,
	})
	for _, finding := range playSteered(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsCompactStarted = "main compacts an idle worker: the command answers started before the compaction ends, which the telemetry has not counted yet"
	obsCompactLetter  = "the compaction's end reaches main as a notify from the worker, with the token counts and the compaction's number"
	obsCompactBusy    = "main compacts a worker in a turn: refused as in a turn, and the turn goes on unreported"
	obsInterruptBusy  = "main interrupts a worker in a turn: done, and main reads stopped saying main interrupted it, as its awaited list does"
	obsCompactQuiet   = "main's own compaction: the telemetry counts it, and main gets no compaction notice of it"
	obsInterruptIdle  = "main interrupts an idle worker: refused as no turn running"
	obsCompactOrphan  = "a worker killed in the middle of a compaction main asked for: main still gets one letter of it"
	obsNotAnswering   = "a request to a worker whose run has no live Control is refused, exit 1, with nothing asked"
	obsNotMain        = "a worker that is not main asking for a compaction is a wrong call"
	steeredTask       = "fixture-steered: work the long task"
	steeredFocus      = "keep the plan"
)

var steeredObservations = []string{
	obsCompactStarted, obsCompactLetter, obsCompactQuiet, obsCompactBusy, obsInterruptBusy, obsInterruptIdle, obsCompactOrphan,
	obsNotAnswering, obsNotMain,
}

func playSteered(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := fixtureColumn.harness
	calmAsks := newRequests(iso, "calm")
	calm := startHarnessSession(t, c, iso, col, "calm", "--general", shimInboxJSON+"=1", shimCompactTakes+"="+compactTakes, calmAsks.env())
	defer stopSession(t, c, calm)
	// busy and bare are asked nothing, but the case outlives the ordinary
	// ceiling.
	busy := startHarnessSession(t, c, iso, col, "busy", "--general", shimInboxJSON+"=1", shimHoldForInterrupt+"=1", staysUp(iso, "busy"))
	defer stopSession(t, c, busy)
	bare := startHarnessSession(t, c, iso, col, "bare", "--general", shimInboxJSON+"=1", shimNoControl+"=1", staysUp(iso, "bare"))
	defer stopSession(t, c, bare)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	sg := steering{c: c, asks: asks, lead: lead}
	steer, about, row, kinds := sg.steer, sg.about, sg.row, reportKinds
	stoppedText := lead.name + " interrupted this turn with rewake interrupt"

	var sends []string
	if err := sessionsUp(c, calm, busy, bare); err != nil {
		sends = append(sends, err.Error())
	}
	for _, worker := range []*scenarioSession{calm, busy} {
		code, out, _ := asks.ask(c, "send", worker.name, steeredTask)
		sends = append(sends, fmt.Sprintf("to %s: exit %d, %s", worker.name, code, firstLine(out)))
	}
	var out []telemetryFinding

	// While the two work: bare's run proved no Control, so the command asks
	// nothing and says so.
	code, view, said := steer("compact", bare.name)
	out = append(out, finding(obsNotAnswering, code == 1 && view.Outcome == "" && strings.Contains(said, "control is not live") && strings.Contains(said, "nothing was asked"), "%s", said))

	tasks := map[*scenarioSession]string{}
	if !waitFor(c, 20*time.Second, func() bool {
		for _, worker := range []*scenarioSession{calm, busy} {
			message, read := messageCarrying(worker, "fixture-steered:")
			if !read {
				return false
			}
			tasks[worker] = message.ID
		}
		return slices.Contains(kinds(about(calm, tasks[calm])), "finished")
	}) {
		for _, observation := range steeredObservations[:len(steeredObservations)-2] {
			out = append(out, telemetryFinding{observation: observation, detail: fmt.Sprintf("the workers did not take their tasks: sends %v; calm's reports %v", sends, kinds(about(calm, tasks[calm])))})
		}
		code, refused, _ := calmAsks.ask(c, "compact", bare.name)
		return append(out, finding(obsNotMain, code == 2 && strings.Contains(refused, notMainRefusal), "exit %d, %s", code, firstLine(refused)))
	}

	// busy is in its held turn: a compaction is refused, and the turn is
	// still running afterwards, with nothing reported about its task.
	code, view, said = steer("compact", busy.name)
	busyRow, failure := row(busy)
	early := kinds(about(busy, tasks[busy]))
	out = append(out, finding(obsCompactBusy,
		code == 1 && view.Outcome == "refused" && view.Reason == "in a turn" && busyRow.Activity == "working" && len(early) == 0 && failure == "",
		"%s; then busy reads %q %s, and main read %v about its task", said, busyRow.Activity, failure, early))

	code, view, said = steer("interrupt", busy.name)
	stopped := sg.stoppedAbout(busy, tasks[busy])
	// The task is still owed after a stop, and main's list of what it is owed
	// says who stopped it.
	_, awaited, _ := asks.ask(c, "inbox", "--awaited")
	out = append(out, finding(obsInterruptBusy,
		code == 0 && view.Outcome == "done" && strings.Contains(stopped.Text, stoppedText) &&
			strings.Contains(awaited, " · stopped: "+stoppedText+"\n"),
		"%s; main read %v about the task, the stopped saying %q; its awaited list: %q", said, kinds(about(busy, tasks[busy])), firstLine(stopped.Text), awaited))

	code, view, said = steer("interrupt", calm.name)
	out = append(out, finding(obsInterruptIdle, code == 1 && view.Outcome == "refused" && view.Reason == "no turn running", "%s", said))

	out = append(out, sg.compactIdle(calm, fixtureTokensBefore, fixtureTokensAfter, steeredFocus)...)

	code, refused, _ := calmAsks.ask(c, "compact", bare.name)
	out = append(out, finding(obsNotMain, code == 2 && strings.Contains(refused, notMainRefusal), "exit %d, %s", code, firstLine(refused)))

	// calm's harness is killed while its second compaction runs: nothing
	// records the end, and the letter comes from main's own wrapper. A letter
	// without an outcome goes letterWait after the departure is seen.
	lettersOf := func() []string {
		var letters []string
		for _, message := range readMessages(lead) {
			if message.From == calm.name && (strings.HasPrefix(message.Text, "Rewake: the compaction of "+calm.name+" you asked for") ||
				strings.HasPrefix(message.Text, "Rewake: compacted "+calm.name)) {
				letters = append(letters, message.Kind+" "+message.Text)
			}
		}
		return letters
	}
	earlier := len(lettersOf())
	code, view, said = steer("compact", calm.name)
	killHarness(c, iso, calm)
	var letters []string
	// The letter goes letterWait after the worker is gone and waits up to
	// suiteCap for company; the control that sends none waits this out.
	waitFor(c, suiteLetterWait+suiteCap+3*time.Second, func() bool {
		letters = lettersOf()
		letters = letters[min(earlier, len(letters)):]
		return len(letters) > 0
	})
	out = append(out, finding(obsCompactOrphan,
		code == 0 && view.Outcome == "started" && len(letters) == 1 && strings.HasPrefix(letters[0], "notify "),
		"%s; then main read %q", said, letters))
	return out
}

// killHarness kills a session's harness outright, as a crash does, and waits
// for its wrapper to end the session: nothing of the harness writes anything
// afterwards, while the wrapper still cleans up after it.
func killHarness(c *Case, iso *Isolation, session *scenarioSession) {
	var record struct {
		HarnessPID int `json:"harnessPid"`
	}
	raw, err := os.ReadFile(filepath.Join(iso.StateDir, "rooms", "default", "sessions", session.name+".json"))
	if err != nil || json.Unmarshal(raw, &record) != nil || record.HarnessPID <= 0 {
		c.Note("no harness to kill for " + session.name)
		return
	}
	c.mu.Lock()
	session.process.endedByCase = true
	c.mu.Unlock()
	_ = syscall.Kill(record.HarnessPID, syscall.SIGKILL)
	waitFor(c, 10*time.Second, func() bool { return session.process.finished() && !session.process.groupAlive() })
}
