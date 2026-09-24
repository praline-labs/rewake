package workflow

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// What the steered cases of both columns share: a main asking through its
// session, and reading back what it asked and what it was sent.

// steeredView is what `rewake compact` and `rewake interrupt` print under
// --json.
type steeredView struct {
	Outcome      string  `json:"outcome"`
	Reason       string  `json:"reason"`
	Detail       string  `json:"detail"`
	TokensBefore *int64  `json:"tokensBefore"`
	TokensAfter  *int64  `json:"tokensAfter"`
	Compaction   *uint64 `json:"compaction"`
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
