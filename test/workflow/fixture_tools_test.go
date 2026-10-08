package workflow

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The mail tool through the fixture's own process, one scenario per tool of
// the set (docs/v2/stage3-fixture.md): a main sends a worker a task, and the
// worker's first turn makes its calls through the tool — reported to the
// adapter, run by the wrapper's endpoint, their results reported back —
// before the turn ends and reports. What each call did is judged by what the
// other sessions read and what the state directory holds, not only by the
// answer the program was handed.
//
// The fixture column alone: the transport is the fixture's own, and the other
// columns' tools are their harnesses' servers, held by their own scenarios.

const (
	toolTaskText = "tool-scenario: do the work and report"
	toolNoteText = "tool-scenario: a heads-up through the tool"
	toolMarkText = "tool-scenario: the build runs in the background"
	toolLead     = "lead"
)

// toolScenario is one tool's case: the calls the worker's turn makes, given
// the lead's session name, and the judgment of what they did.
type toolScenario struct {
	name         string
	observations []string
	calls        func(lead string) []fixtureToolCall
	judge        func(run toolRun) []telemetryFinding
}

// toolRun is what a scenario judges: the sessions, the task the lead sent,
// and the worker's calls as its turn log recorded them.
type toolRun struct {
	c            *Case
	iso          *Isolation
	lead, worker *codexSession
	task         reportView
	calls        []fixtureToolRecord
}

// from is what the lead read from the worker.
func (r toolRun) from() []reportView {
	var out []reportView
	for _, message := range readMessages(r.lead) {
		if message.From == r.worker.name {
			out = append(out, message)
		}
	}
	return out
}

// owed lists the worker's open obligations to the lead.
func (r toolRun) owed() []string {
	found, _ := filepath.Glob(filepath.Join(r.iso.StateDir, "rooms", "default", "inbox", r.worker.name, "awaiting", "*", toolLead+"-*"))
	return found
}

// answer is a call's whole text.
func (record fixtureToolRecord) answer() string {
	return strings.Join(record.Texts, "")
}

func (record fixtureToolRecord) String() string {
	return fmt.Sprintf("%s %v: error %v %q %s", record.Tool, record.Arguments, record.IsError, record.answer(), record.Error)
}

func runToolScenario(t *testing.T, scenario toolScenario) {
	runParallel(t)
	col := fixtureColumn
	t.Run(col.harness, func(t *testing.T) {
		binary := enterScenario(t, scenario.name)
		c := Start(t, Spec{
			Name:         scenario.name,
			Harness:      col.harness,
			Observations: scenario.observations,
			Deadline:     90 * time.Second,
		})
		for _, finding := range playToolScenario(t, c, Isolate(t, c, binary), col, scenario) {
			if finding.held {
				c.Observed(finding.observation, finding.detail)
			} else {
				c.Contradicted(finding.observation, "%s", finding.detail)
			}
		}
	})
}

func playToolScenario(t *testing.T, c *Case, iso *Isolation, col column, scenario toolScenario) []telemetryFinding {
	t.Helper()
	unjudged := func(detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range scenario.observations {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
	}
	calls := scenario.calls(toolLead + "-" + col.harness)
	encoded, err := json.Marshal(calls)
	if err != nil {
		t.Fatal(err)
	}
	worker := startHarnessSession(t, c, iso, col.harness, "worker", "--general", shimInboxJSON+"=1", shimToolCalls+"="+string(encoded))
	defer stopSession(t, c, worker)
	lead := startHarnessSession(t, c, iso, col.harness, toolLead, "--main",
		shimSendTo+"="+worker.name, shimSendText+"="+toolTaskText, shimInboxJSON+"=1", readinessSwitch(col, worker))
	defer stopSession(t, c, lead)

	run := toolRun{c: c, iso: iso, lead: lead, worker: worker}
	var problem error
	if !waitFor(c, 40*time.Second, func() bool {
		run.calls, problem = toolRecords(worker)
		return problem == nil && len(run.calls) == len(calls)
	}) {
		return unjudged(fmt.Sprintf("the worker's turn made %d of %d calls through the tool (%v): %s", len(run.calls), len(calls), problem, worker.acceptedTurns()))
	}
	// The turn's end: the lead reads the worker's answer about its task.
	if !waitFor(c, 30*time.Second, func() bool {
		for _, message := range run.from() {
			if len(message.InReplyTo) > 0 {
				run.task = reportView{ID: message.InReplyTo[0]}
				return true
			}
		}
		return false
	}) {
		return unjudged(fmt.Sprintf("the lead never read the worker's answer about its task; calls %v", run.calls))
	}
	return scenario.judge(run)
}

// toolRecords are the calls the worker's turn log recorded.
func toolRecords(worker *codexSession) ([]fixtureToolRecord, error) {
	events, err := worker.turnEvents()
	if err != nil {
		return nil, err
	}
	var records []fixtureToolRecord
	for _, event := range events {
		if event.Kind != toolEventKind {
			continue
		}
		var record fixtureToolRecord
		if err := json.Unmarshal([]byte(event.Detail), &record); err != nil {
			return nil, errUnreadableRecord(worker.turns, event.Detail)
		}
		records = append(records, record)
	}
	return records, nil
}

func toolFinding(observation string, held bool, detail string, args ...any) telemetryFinding {
	return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
}

const (
	obsToolInboxShows   = "the tool's inbox answer shows the task"
	obsToolInboxReads   = "the task read through the tool is owed, and the turn's end reports it and settles it"
	obsToolSendOnce     = "the lead reads the tool's heads-up once, from the worker"
	obsToolPendingMark  = "the tool's pending mark is accepted, and the lead reads an interim message with its line while the task stays owed"
	obsToolWhoamiNames  = "the tool's whoami answer names the worker and says the call came through the tool"
	obsToolRetryJoins   = "the tool's retry of a heads-up answers that heads-up's own receipt, and the lead reads it once"
	obsToolListSessions = "the tool's list answer names the worker and the lead"
)

func TestToolInbox(t *testing.T) {
	runToolScenario(t, toolScenario{
		name:         "tool-inbox",
		observations: []string{obsToolInboxShows, obsToolInboxReads},
		calls: func(string) []fixtureToolCall {
			return []fixtureToolCall{{Tool: "inbox"}}
		},
		judge: func(run toolRun) []telemetryFinding {
			read := run.calls[0]
			finished := slices.ContainsFunc(run.from(), func(message reportView) bool {
				return message.Kind == "finished" && slices.Contains(message.InReplyTo, run.task.ID)
			})
			settled := waitFor(run.c, 5*time.Second, func() bool { return len(run.owed()) == 0 })
			return []telemetryFinding{
				toolFinding(obsToolInboxShows, !read.IsError && read.Error == "" && strings.Contains(read.answer(), toolTaskText), "%s", read),
				toolFinding(obsToolInboxReads, finished && settled, "a finished report on %s: %v; settled: %v; the worker's turns: %s", run.task.ID, finished, settled, run.worker.acceptedTurns()),
			}
		},
	})
}

func TestToolSend(t *testing.T) {
	runToolScenario(t, toolScenario{
		name:         "tool-send",
		observations: []string{obsToolSendOnce},
		calls: func(lead string) []fixtureToolCall {
			return []fixtureToolCall{{Tool: "send", Arguments: map[string]any{"notify": true, "wait": "0", "name": lead, "text": toolNoteText}}}
		},
		judge: func(run toolRun) []telemetryFinding {
			notes := func() int {
				n := 0
				for _, message := range run.from() {
					if message.Kind == "notify" && strings.Contains(message.Text, toolNoteText) {
						n++
					}
				}
				return n
			}
			waitFor(run.c, 10*time.Second, func() bool { return notes() > 0 })
			return []telemetryFinding{toolFinding(obsToolSendOnce, notes() == 1 && run.calls[0].Error == "", "%d notes read; %s", notes(), run.calls[0])}
		},
	})
}

func TestToolPending(t *testing.T) {
	runToolScenario(t, toolScenario{
		name:         "tool-pending",
		observations: []string{obsToolPendingMark},
		calls: func(string) []fixtureToolCall {
			return []fixtureToolCall{{Tool: "pending", Arguments: map[string]any{"text": toolMarkText}}}
		},
		judge: func(run toolRun) []telemetryFinding {
			mark := run.calls[0]
			var first reportView
			for _, message := range run.from() {
				if slices.Contains(message.InReplyTo, run.task.ID) {
					first = message
					break
				}
			}
			owed := run.owed()
			held := !mark.IsError && mark.Error == "" && first.Kind == "pending" && strings.HasPrefix(first.Text, toolMarkText) && len(owed) > 0
			return []telemetryFinding{toolFinding(obsToolPendingMark, held, "%s; the first answer on the task was %s %q; owed %v", mark, first.Kind, first.Text, owed)}
		},
	})
}

func TestToolWhoami(t *testing.T) {
	runToolScenario(t, toolScenario{
		name:         "tool-whoami",
		observations: []string{obsToolWhoamiNames},
		calls: func(string) []fixtureToolCall {
			return []fixtureToolCall{{Tool: "whoami"}}
		},
		judge: func(run toolRun) []telemetryFinding {
			who := run.calls[0]
			held := !who.IsError && strings.Contains(who.answer(), run.worker.name) && strings.Contains(who.answer(), "came through the rewake tool")
			return []telemetryFinding{toolFinding(obsToolWhoamiNames, held, "%s", who)}
		},
	})
}

func TestToolRetry(t *testing.T) {
	runToolScenario(t, toolScenario{
		name:         "tool-retry",
		observations: []string{obsToolRetryJoins},
		calls: func(lead string) []fixtureToolCall {
			return []fixtureToolCall{
				{Tool: "send", Arguments: map[string]any{"notify": true, "json": true, "wait": "0", "name": lead, "text": toolNoteText}},
				{Tool: "retry", Arguments: map[string]any{"receipt": "{receipt}", "json": true}},
			}
		},
		judge: func(run toolRun) []telemetryFinding {
			sent, retried := run.calls[0], run.calls[1]
			found := toolReceipt.FindStringSubmatch(sent.answer())
			notes := 0
			waitFor(run.c, 10*time.Second, func() bool {
				notes = 0
				for _, message := range run.from() {
					if message.Kind == "notify" && strings.Contains(message.Text, toolNoteText) {
						notes++
					}
				}
				return notes > 0
			})
			held := found != nil && retried.Error == "" && strings.Contains(retried.answer(), found[1]) && notes == 1
			return []telemetryFinding{toolFinding(obsToolRetryJoins, held, "the send: %s; the retry: %s; %d notes read", sent, retried, notes)}
		},
	})
}

func TestToolList(t *testing.T) {
	runToolScenario(t, toolScenario{
		name:         "tool-list",
		observations: []string{obsToolListSessions},
		calls: func(string) []fixtureToolCall {
			return []fixtureToolCall{{Tool: "list"}}
		},
		judge: func(run toolRun) []telemetryFinding {
			list := run.calls[0]
			held := !list.IsError && strings.Contains(list.answer(), run.worker.name) && strings.Contains(list.answer(), run.lead.name)
			return []telemetryFinding{toolFinding(obsToolListSessions, held, "%s", list)}
		},
	})
}
