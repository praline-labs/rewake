package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/iiiokojiadbi/rewake/test/workflow/record"
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
	Totals        map[string]int `json:"totals"`
	Wall          string         `json:"wall"`
	EngineExit    *int           `json:"engineExit"`
	PackageFailed bool           `json:"packageFailed"`

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
	if s.PackageFailed {
		return false
	}
	if s.EngineExit != nil && *s.EngineExit != 0 {
		return false
	}
	for _, one := range s.Cases {
		if one.Outcome != outcomePass && one.Outcome != outcomeUnsupported {
			return false
		}
	}
	return true
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
	s.renderUnsupported(to)
	s.renderFailures(to)
	if s.EngineExit != nil && *s.EngineExit != 0 && !s.anyRed() && !s.ranNothing() {
		// The engine failed and no case says why: a build error, a panic, a
		// package timeout. Saying so is the whole diagnostic a reader gets —
		// unless the line above already said it. A run that was switched on
		// and ran nothing fails the engine by design, and printing both is two
		// refusals for one cause.
		_, _ = fmt.Fprintf(to, "engine    FAIL  go test exited %d with no case reporting a failure\n", *s.EngineExit)
	}
	_, _ = fmt.Fprintf(to, "summary   %s\n", path)
}

// ranNothing is the suite's own red line: the switch was set and no scenario
// ran.
func (s *summary) ranNothing() bool { return s.Run.Enabled && len(s.Run.Scenarios) == 0 }

func (s *summary) anyRed() bool {
	for _, one := range s.Cases {
		if one.Outcome != outcomePass && one.Outcome != outcomeUnsupported {
			return true
		}
	}
	return false
}

// renderUnsupported names each absent capability with the column it is absent
// from. A cell that is unsupported and unexplained reads as a defect that
// swallowed the evidence.
func (s *summary) renderUnsupported(to io.Writer) {
	for _, one := range s.Cases {
		if one.Outcome != outcomeUnsupported {
			continue
		}
		_, _ = fmt.Fprintf(to, "          unsupported  %s  %s\n", column(one), one.Reason)
	}
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
		if one.Outcome == outcomePass || one.Outcome == outcomeUnsupported {
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

// excerptLines is the per-case ceiling from the same contract.
const excerptLines = 20

func failureLines(one record.Case) []string {
	lines := []string{fmt.Sprintf("FAIL  %s/%s   %s", one.Case, column(one), one.Outcome)}
	for _, observation := range one.Observations {
		if observation.Outcome == outcomePass || observation.Outcome == outcomeUnsupported {
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
