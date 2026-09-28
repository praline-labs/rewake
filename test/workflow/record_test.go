package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/praline-labs/rewake/test/workflow/record"
)

// The record a case leaves for whoever summarizes the run.
//
// One line per case, on standard output, in the one place a verdict is
// published. A summarizer reads these and nothing else: the contract in
// docs/check-runner.md asks for per-observation outcomes, evidence paths and
// durations, and none of that survives a regular expression over free text.

func (c *Case) publishRecord(outcome Outcome, reason string, made map[string]observation, keptEvidence bool) {
	if !c.scenario {
		return
	}
	c.mu.Lock()
	dirs := append([]string(nil), c.dirs...)
	stderr := append([]string(nil), c.stderr...)
	c.mu.Unlock()
	if !keptEvidence {
		// A green case removes its directories, so naming them would send a
		// reader to a path that is not there.
		dirs = nil
	}
	published := record.Case{
		Case:    c.spec.Name,
		Harness: c.spec.Harness,
		Gate:    isGate(c.spec.Harness),
		// The one conversion: the suite's Outcome is a type of the test files,
		// and the shared record carries plain strings so a program outside can
		// read it.
		Outcome:    string(outcome),
		Reason:     reason,
		Evidence:   dirs,
		Stderr:     stderr,
		DurationMs: time.Since(c.started).Milliseconds(),
	}
	for _, name := range c.spec.Observations {
		entry, ok := made[name]
		if !ok {
			published.Observations = append(published.Observations, record.Observation{Name: name, Outcome: string(NotRun)})
			continue
		}
		published.Observations = append(published.Observations, record.Observation{
			Name: name, Outcome: string(entry.Outcome), Detail: entry.Detail, Capability: entry.Capability,
		})
	}
	encoded, err := json.Marshal(published)
	if err != nil {
		// A record that cannot be written is a case the summary would not know
		// about, which is worse than a loud failure here.
		c.t.Errorf("workflow: case %q could not publish its record: %v", c.spec.Name, err)
		return
	}
	fmt.Println(record.CaseMark + string(encoded))
}

func publishRun(enabled bool, scenarios []string, against []record.Against, failure string) {
	encoded, err := json.Marshal(record.Run{Enabled: enabled, Scenarios: scenarios, Against: against, Failure: failure})
	if err != nil {
		fmt.Fprintf(os.Stderr, "workflow: the run record could not be written: %v\n", err)
		return
	}
	fmt.Println(record.RunMark + string(encoded))
}
