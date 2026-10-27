package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestBriefGivenWorkedAndRead is a brief's main path end to end
// (docs/briefs.md): main makes a brief and sends its task with --brief, the
// worker's inbox prints the brief with the task, the worker ticks item 1 by
// the edition it read and comments; main gives it to a reviewer with
// --review, whose tick is refused and whose comment is taken; main's brief
// show sees the tick and both comments, each with its author's capacity.
//
// Each session runs its own commands when the scenario asks: a brief's rights
// are main's run and the run it was given to, which a test process would not
// prove.
func TestBriefGivenWorkedAndRead(t *testing.T) {
	runInColumns(t, "brief", func(t *testing.T, col column) {
		binary := enterScenario(t, "brief")
		c := Start(t, Spec{
			Name:         "brief",
			Harness:      col.harness,
			Observations: briefObservations,
			Deadline:     90 * time.Second,
		})
		for _, finding := range playBrief(t, c, Isolate(t, c, binary), col) {
			if finding.held {
				c.Observed(finding.observation, finding.detail)
			} else {
				c.Contradicted(finding.observation, "%s", finding.detail)
			}
		}
	})
}

const (
	obsBriefGiven  = "the worker's inbox prints the brief with the task that gives it"
	obsBriefWorked = "the worker's tick by the edition it read and its comment are accepted"
	obsBriefReview = "the reviewer given the brief with --review is refused a tick, and its comment is accepted"
	obsBriefSeen   = "main's brief show has item 1 done, and the worker's and the reviewer's comments with their capacities"

	briefTaskText = "brief-probe: do the brief"
	briefText     = "# Brief probe\n\n- [ ] first item\n- [ ] second item\n"
	briefComment  = "brief-probe comment: first item done"
	reviewTask    = "brief-probe: review the brief"
	reviewComment = "brief-probe review: second item not done"
)

var briefObservations = []string{obsBriefGiven, obsBriefWorked, obsBriefReview, obsBriefSeen}

// briefShown is the machine form of brief show, as far as the scenario
// judges it.
type briefShown struct {
	ID      int `json:"id"`
	Edition int `json:"edition"`
	Done    int `json:"done"`
	Items   []struct {
		Done bool `json:"done"`
	} `json:"items"`
	Comments []struct {
		From     string `json:"from"`
		Text     string `json:"text"`
		Capacity string `json:"capacity"`
	} `json:"comments"`
}

func playBrief(t *testing.T, c *Case, iso *Isolation, col column) []telemetryFinding {
	t.Helper()
	works := newRequests(iso, "worker")
	worker := startHarnessSession(t, c, iso, col.harness, "worker", "--general", shimInboxJSON+"=1", works.env())
	defer stopSession(t, c, worker)
	reviews := newRequests(iso, "reviewer")
	reviewer := startHarnessSession(t, c, iso, col.harness, "reviewer", "--general", shimInboxJSON+"=1", reviews.env())
	defer stopSession(t, c, reviewer)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col.harness, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	unjudged := func(detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range briefObservations {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
	}
	// brief show in a session's own run, parsed.
	show := func(r *requests, id int) (briefShown, string) {
		code, out, _ := r.ask(c, "brief", "show", fmt.Sprint(id), "--json")
		var shown briefShown
		if code != 0 || json.Unmarshal([]byte(out), &shown) != nil {
			return briefShown{}, fmt.Sprintf("exit %d: %s", code, out)
		}
		return shown, out
	}

	if err := sessionsUp(c, worker, reviewer); err != nil {
		return unjudged(err.Error())
	}
	code, out, _ := asks.ask(c, "brief", "new", "--json", "--", briefText)
	var made briefShown
	if code != 0 || json.Unmarshal([]byte(out), &made) != nil || made.ID < 1 {
		return unjudged(fmt.Sprintf("main's brief new answered exit %d: %s", code, out))
	}
	id := fmt.Sprint(made.ID)
	if code, out, _ := asks.ask(c, "send", worker.name, briefTaskText, "--brief", id); code != 0 {
		return unjudged(fmt.Sprintf("main's send --brief answered exit %d: %s", code, out))
	}
	if !waitFor(c, 30*time.Second, func() bool {
		_, read := messageCarrying(worker, "brief-probe")
		return read
	}) {
		return unjudged("the worker never read the task")
	}

	// The machine form of the worker's read, indented: the letter's brief
	// object carries its title and its items.
	read := worker.mailboxRead()
	given := strings.Contains(read, briefTaskText) && strings.Contains(read, `"title": "Brief probe"`) &&
		strings.Contains(read, `"text": "first item"`)
	findings := []telemetryFinding{finding(obsBriefGiven, given, "the worker's inbox read %q", read)}

	// The worker's own reading names the edition its tick takes.
	reread, rereadOut := show(works, made.ID)
	tick, tickOut, _ := works.ask(c, "brief", "tick", id, "1", fmt.Sprintf("--edition=%d", reread.Edition))
	comment, commentOut, _ := works.ask(c, "brief", "comment", id, "--", briefComment)
	accepted := reread.Edition == 1 && tick == 0 && comment == 0 &&
		strings.HasPrefix(tickOut, "Rewake: brief "+id+" item #1 done: first item.")
	findings = append(findings, finding(obsBriefWorked, accepted, "the worker's show: %s; tick exit %d: %s; comment exit %d: %s",
		rereadOut, tick, firstLine(tickOut), comment, firstLine(commentOut)))

	// The reviewer reads and comments, and is refused the tick that is the
	// worker's.
	if code, out, _ := asks.ask(c, "send", reviewer.name, reviewTask, "--brief", id, "--review"); code != 0 {
		return append(findings, finding(obsBriefReview, false, "main's send --brief --review answered exit %d: %s", code, out))
	}
	if !waitFor(c, 30*time.Second, func() bool {
		_, read := messageCarrying(reviewer, "review the brief")
		return read
	}) {
		return append(findings, finding(obsBriefReview, false, "the reviewer never read the task"))
	}
	refused, refusedOut, _ := reviews.ask(c, "brief", "tick", id, "2", fmt.Sprintf("--edition=%d", reread.Edition))
	reviewed, reviewedOut, _ := reviews.ask(c, "brief", "comment", id, "--", reviewComment)
	findings = append(findings, finding(obsBriefReview, refused == 1 && strings.Contains(refusedOut, "ticking is the worker's") && reviewed == 0,
		"the reviewer's tick exit %d: %s; comment exit %d: %s", refused, firstLine(refusedOut), reviewed, firstLine(reviewedOut)))

	shown, shownOut := show(asks, made.ID)
	seen := shown.Done == 1 && len(shown.Items) == 2 && shown.Items[0].Done && !shown.Items[1].Done &&
		len(shown.Comments) == 2 &&
		shown.Comments[0].From == worker.name && shown.Comments[0].Text == briefComment && shown.Comments[0].Capacity == "work" &&
		shown.Comments[1].From == reviewer.name && shown.Comments[1].Text == reviewComment && shown.Comments[1].Capacity == "review"
	return append(findings, finding(obsBriefSeen, seen, "main's brief show printed %s", shownOut))
}
