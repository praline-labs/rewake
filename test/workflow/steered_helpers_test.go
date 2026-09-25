package workflow

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// What the steered cases of both columns share: a main asking through its
// session, and reading back what it asked and what it was sent.

// steeredView is what `rewake compact` and `rewake interrupt` print under
// --json.
type steeredView struct {
	Outcome      string `json:"outcome"`
	Reason       string `json:"reason"`
	Detail       string `json:"detail"`
	TokensBefore *int64 `json:"tokensBefore"`
	TokensAfter  *int64 `json:"tokensAfter"`
}

type steeredRow struct {
	Activity    string  `json:"activity"`
	Compactions *uint64 `json:"completedCompactions"`
}

// steering is a main running rewake compact and rewake interrupt through its
// session, and reading what it was sent.
type steering struct {
	c    *Case
	asks *requests
	lead *codexSession
}

// steer runs one command as main and reads what it printed.
func (s steering) steer(args ...string) (int, steeredView, string) {
	code, out, ok := s.asks.ask(s.c, append(args, "--json")...)
	var view steeredView
	if !ok || json.Unmarshal([]byte(out), &view) != nil {
		return code, view, fmt.Sprintf("exit %d, %q", code, firstLine(out))
	}
	return code, view, fmt.Sprintf("exit %d, %s %s: %s", code, view.Outcome, view.Reason, view.Detail)
}

// about is the messages main read from a worker about its task.
func (s steering) about(worker *codexSession, task string) []reportView {
	var found []reportView
	for _, message := range readMessages(s.lead) {
		if message.From == worker.name && slices.Contains(message.InReplyTo, task) {
			found = append(found, message)
		}
	}
	return found
}

// row is the worker's telemetry as main's listing shows it.
func (s steering) row(worker *codexSession) (steeredRow, string) {
	code, machine, ok := s.asks.ask(s.c, "list", "--json")
	var listed struct {
		Sessions []struct {
			Name      string     `json:"name"`
			Telemetry steeredRow `json:"telemetry"`
		} `json:"sessions"`
	}
	if !ok || code != 0 || json.Unmarshal([]byte(machine), &listed) != nil {
		return steeredRow{}, fmt.Sprintf("the listing did not come back: exit %d, %q", code, firstLine(machine))
	}
	for _, entry := range listed.Sessions {
		if entry.Name == worker.name {
			return entry.Telemetry, ""
		}
	}
	return steeredRow{}, worker.name + " is not listed"
}

// stoppedAbout waits for the stopped message about a worker's task.
func (s steering) stoppedAbout(worker *codexSession, task string) reportView {
	var stopped reportView
	waitFor(s.c, 10*time.Second, func() bool {
		for _, message := range s.about(worker, task) {
			if message.Kind == "stopped" {
				stopped = message
				return true
			}
		}
		return false
	})
	return stopped
}

func reportKinds(messages []reportView) []string {
	var out []string
	for _, message := range messages {
		out = append(out, message.Kind)
	}
	return out
}

// compactTakes is how long the fixture's compaction runs after its start in
// the steered cases: long enough that a command which waited for the end
// could neither answer within startedWithin nor before the telemetry counted
// it.
const compactTakes = "5s"

// startedWithin is how soon a command that did not wait for the end answers,
// with room for the module's poll and the pickup under a loaded machine.
const startedWithin = 3 * time.Second

// compactIdle is main compacting an idle worker whose compaction takes
// compactTakes: the command answers once it has started, the end comes as a
// letter with the counts, and no notice besides. The fixture's worker has
// compacted nothing before. A focus, where the column takes one, goes with the
// request.
func (s steering) compactIdle(worker *codexSession, before, after int64, focus ...string) []telemetryFinding {
	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	asked := time.Now()
	code, view, said := s.steer(append([]string{"compact", worker.name}, focus...)...)
	took := time.Since(asked)
	early, failure := s.row(worker)
	uncounted := early.Compactions != nil && *early.Compactions == 0 && failure == ""
	out := []telemetryFinding{finding(obsCompactStarted,
		code == 0 && view.Outcome == "started" && took < startedWithin && uncounted,
		"%s in %s; the telemetry then counts %s compactions %s", said, took.Round(time.Millisecond), show(early.Compactions), failure)}

	want := fmt.Sprintf("Rewake: compacted %s: %d tokens before, %d after (compaction 1).", worker.name, before, after)
	var letter reportView
	waitFor(s.c, 15*time.Second, func() bool {
		for _, message := range readMessages(s.lead) {
			if message.From == worker.name && strings.HasPrefix(message.Text, "Rewake: compacted") {
				letter = message
				return true
			}
		}
		return false
	})
	out = append(out, finding(obsCompactLetter,
		letter.Text == want && letter.Kind == "notify",
		"main read %s %q, want notify %q", letter.Kind, letter.Text, want))

	// main's wrapper looks for compactions once a second, and the telemetry
	// case sees its notice within five: nothing in five means none was sent.
	// No later event can stand in for the wait — the absence is the finding.
	const notice = "Rewake: context compacted (compaction 1)."
	noticed := waitFor(s.c, 5*time.Second, func() bool { return strings.Contains(s.lead.mailboxRead(), notice) })
	late, failure := s.row(worker)
	out = append(out, finding(obsCompactQuiet,
		late.Compactions != nil && *late.Compactions == 1 && !noticed,
		"the telemetry counts %s compactions %s; main read %q: %v", show(late.Compactions), failure, notice, noticed))
	return out
}
