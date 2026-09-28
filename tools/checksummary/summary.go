package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/praline-labs/rewake/test/workflow/record"
)

// The summary built from the records a run publishes.
//
// The records themselves are declared once, in test/workflow/record, and both
// ends import it: the suite writes those structures and this program reads
// them, so a renamed field is a compile error rather than an empty column.

// The outcome names come from docs/check-runner.md. Only pass is green, and
// unsupported is green *for the run* while staying visible in its own column:
// a harness that cannot offer a capability has not failed, and has not passed
// either.
const (
	outcomePass        = "pass"
	outcomeUnsupported = "unsupported"
)

type summary struct {
	Run   record.Run    `json:"run"`
	Cases []record.Case `json:"cases"`
	// Totals are the cases by outcome, written into the file from the same
	// function the console line uses. A consumer that had to recount them
	// would be a second implementation of the one question this file answers.
	Totals map[string]int `json:"totals"`
	// FailedTests are the tests the engine reported failed, whether or not a
	// case record explains them.
	FailedTests   []string `json:"failedTests"`
	Wall          string   `json:"wall"`
	EngineExit    *int     `json:"engineExit"`
	PackageFailed bool     `json:"packageFailed"`

	sawRun bool
}

// green reports whether the whole run may be presented as a success. Three
// things can take it away, and each of them has been a real failure here: a
// case that did not pass, a run that was switched on and ran nothing, and an
// engine that failed for a reason no case recorded — a build error, a panic, a
// package timeout.
func (s *summary) green() bool {
	if s.ranNothing() {
		return false
	}
	if s.PackageFailed || s.Run.Failure != "" {
		return false
	}
	if s.EngineExit != nil && *s.EngineExit != 0 {
		return false
	}
	for _, one := range s.Cases {
		if !acceptable(one) {
			return false
		}
	}
	return true
}

// acceptable is the run's rule for one case: a pass, or an absent capability
// off the gate. On the gate an unsupported case is red whatever it says — the
// suite converts it already, and this holds even for a record that did not.
func acceptable(one record.Case) bool {
	return one.Outcome == outcomePass || one.Outcome == outcomeUnsupported && !one.Gate
}

func (s *summary) encode() ([]byte, error) {
	_, s.Totals = s.counts()
	return json.MarshalIndent(s, "", "  ")
}

// counts are the cases by outcome, in a stable order.
func (s *summary) counts() ([]string, map[string]int) {
	byOutcome := map[string]int{}
	for _, one := range s.Cases {
		byOutcome[one.Outcome]++
	}
	names := make([]string, 0, len(byOutcome))
	for name := range byOutcome {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, byOutcome
}

// render writes the few lines an agent reads. The budget in
// docs/check-runner.md is twelve lines for a green run, and the failures that
// follow are bounded rather than trusted to be short.
func (s *summary) render(to io.Writer, path string) {
	scenarios := len(s.Run.Scenarios)
	names, byOutcome := s.counts()
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%d %s", byOutcome[name], name))
	}
	summaryOfCases := strings.Join(parts, ", ")
	if summaryOfCases == "" {
		summaryOfCases = "no cases"
	}
	switch {
	case !s.Run.Enabled:
		_, _ = fmt.Fprintf(to, "workflow  scenarios were switched off\n")
	case scenarios == 0:
		_, _ = fmt.Fprintf(to, "workflow  FAIL  the switch was set and no scenario ran\n")
	default:
		_, _ = fmt.Fprintf(to, "workflow  %d %s, %d %s: %s   %s\n",
			scenarios, plural(scenarios, "scenario"), len(s.Cases), plural(len(s.Cases), "case"), summaryOfCases, s.Wall)
	}
	if s.Run.Enabled && len(s.Run.Against) > 0 {
		// What the run was green or red against. A version is the first
		// thing to doubt about a harness, and a summary that leaves it out
		// cannot be compared with the next one.
		_, _ = fmt.Fprintf(to, "against   %s\n", record.DescribeAgainst(s.Run.Against))
	}
	if s.Run.Failure != "" {
		_, _ = fmt.Fprintf(to, "run       FAIL  %s\n", firstChars(s.Run.Failure, 300))
	}
	s.renderUnsupported(to)
	s.renderFailures(to)
	if s.EngineExit != nil && *s.EngineExit != 0 && !s.anyRed() && !s.ranNothing() && s.Run.Failure == "" {
		// The engine failed and no case says why: a build error, a panic, a
		// package timeout. Saying so is the whole diagnostic a reader gets —
		// unless the line above already said it. A run that was switched on
		// and ran nothing fails the engine by design, and printing both is two
		// refusals for one cause.
		_, _ = fmt.Fprintf(to, "engine    FAIL  go test exited %d with no case reporting a failure\n", *s.EngineExit)
		for index, name := range s.FailedTests {
			if index == failedTestLines {
				_, _ = fmt.Fprintf(to, "          … %d more in the summary file\n", len(s.FailedTests)-index)
				break
			}
			_, _ = fmt.Fprintf(to, "          failed test  %s\n", name)
		}
	}
	_, _ = fmt.Fprintf(to, "summary   %s\n", path)
}

// ranNothing is the suite's own red line: the switch was set and no scenario
// ran.
func (s *summary) ranNothing() bool { return s.Run.Enabled && len(s.Run.Scenarios) == 0 }

func (s *summary) anyRed() bool {
	for _, one := range s.Cases {
		if !acceptable(one) {
			return true
		}
	}
	return false
}

// renderUnsupported names each absent capability with the case and the column
// it is absent from. A cell that is unsupported and unexplained reads as a
// defect that swallowed the evidence, and three lines naming only a column
// cannot be told apart.
func (s *summary) renderUnsupported(to io.Writer) {
	for _, one := range s.Cases {
		if one.Outcome != outcomeUnsupported || !acceptable(one) {
			continue
		}
		_, _ = fmt.Fprintf(to, "          unsupported  %s/%s  %s\n", one.Case, column(one), capabilities(one))
	}
}

// capabilities are the distinct capabilities a case's unsupported
// observations name, or its reason when none names one.
func capabilities(one record.Case) string {
	var names []string
	for _, observation := range one.Observations {
		if observation.Capability != "" && !contains(names, observation.Capability) {
			names = append(names, observation.Capability)
		}
	}
	if len(names) == 0 {
		return one.Reason
	}
	return strings.Join(names, ", ")
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// renderFailures writes the bounded excerpt for each case that did not pass:
// the case and its column, the failing observation by name with its detail,
// and one path to its evidence.
//
// The column is named because a red result does not mean the same thing in
// both: Codex is the regression gate and Claude Code is the search column, and
// check-runner-scenarios.md forbids this program from promoting the second on
// its own. It says which column, and stops there.
func (s *summary) renderFailures(to io.Writer) {
	budget := excerptBudget
	for _, one := range s.Cases {
		if acceptable(one) {
			continue
		}
		lines := failureLines(one)
		for _, line := range lines {
			if budget -= len(line) + 1; budget < 0 {
				_, _ = fmt.Fprintf(to, "          … further failures are in the summary file\n")
				return
			}
			_, _ = fmt.Fprintln(to, line)
		}
	}
}

// excerptBudget is the 8 KiB the evidence contract allows for failure excerpts
// together. Beyond it the file has everything, and the console has a pointer
// to the file.
const excerptBudget = 8 << 10

// failedTestLines bounds the tests named when no case explains a red engine.
// The innermost failing test is usually the answer and the parents follow it,
// so a handful is enough to point the next read.
const failedTestLines = 5

// excerptLines is the per-case ceiling from the same contract.
const excerptLines = 20

func failureLines(one record.Case) []string {
	lines := []string{fmt.Sprintf("FAIL  %s/%s   %s", one.Case, column(one), one.Outcome)}
	for _, observation := range one.Observations {
		// An unsupported observation is the failure itself on the gate, and
		// only a note elsewhere.
		if observation.Outcome == outcomePass || observation.Outcome == outcomeUnsupported && !one.Gate {
			continue
		}
		lines = append(lines, fmt.Sprintf("      %-11s %q", observation.Outcome, observation.Name))
		if observation.Detail != "" {
			lines = append(lines, "        "+firstChars(observation.Detail, 200))
		}
		if len(lines) >= excerptLines-2 {
			lines = append(lines, "      … the rest of this case is in the summary file")
			break
		}
	}
	if len(lines) == 1 && one.Reason != "" {
		// A case can fail without any single observation failing: a cleanup
		// error, an expired deadline. The reason is then the whole diagnostic.
		lines = append(lines, "      "+firstChars(one.Reason, 200))
	}
	for _, evidence := range one.Evidence {
		lines = append(lines, "      evidence  "+evidence)
		break
	}
	// Where a session that exited without a word said why; within the same
	// per-case ceiling.
	for _, stderr := range one.Stderr {
		if len(lines) >= excerptLines {
			break
		}
		lines = append(lines, "      stderr    "+firstChars(stderr, 200))
	}
	return lines
}

func plural(count int, noun string) string {
	if count == 1 {
		return noun
	}
	if noun == "case" {
		return "cases"
	}
	return noun + "s"
}

// column is the harness a case ran against, or a plain marker when it ran
// against none — several cases of this suite test the suite itself.
func column(one record.Case) string {
	if one.Harness == "" {
		return "no-harness"
	}
	return one.Harness
}
