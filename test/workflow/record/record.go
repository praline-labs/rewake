// Package record is the contract between a workflow suite run and whatever
// summarizes it.
//
// It exists because the two ends cannot share anything else. The suite lives
// in test files, which no program can import, and the summarizer is an
// ordinary program — so the shape of what one prints and the other reads was
// declared twice, in two places no compiler connected. A field renamed on one
// side emptied a column on the other, and the end-to-end run only looked at
// two fields of eight.
//
// So the shape lives here, in one package both ends import: the suite writes
// these structures, the summarizer reads them, and a rename is a compile
// error rather than a quiet gap. Plain strings rather than the suite's own
// Outcome type, because that type belongs to the test files as well; the
// conversion happens once, where a case publishes.
//
// It is not under cmd/ and nothing shipped imports it: `go list ./...` finds
// it, so the five checks cover it, and that is the whole of its reach.
package record

import "strings"

// The tags that open a record line. They are looked for at the start of a line
// of test output, so they have to be something no ordinary message begins
// with.
const (
	CaseMark = "workflow-result "
	RunMark  = "workflow-run "
)

// Case is one finished case: what it claimed, how it came out, and where the
// evidence is if it kept any.
type Case struct {
	Case    string `json:"case"`
	Harness string `json:"harness"`
	// Gate is true for the column whose red blocks: there an unsupported
	// outcome is a failure too, and a summarizer must not count it as green
	// even if a record says unsupported.
	Gate         bool          `json:"gate"`
	Outcome      string        `json:"outcome"`
	Reason       string        `json:"reason"`
	Observations []Observation `json:"observations"`
	Evidence     []string      `json:"evidence"`
	DurationMs   int64         `json:"durationMs"`
}

// Observation is one observation of a case. A declared observation that was
// never made appears here as well, as not-run: an absent line and an
// observation nobody looked at would otherwise be the same silence.
type Observation struct {
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	Detail  string `json:"detail"`
	// Capability names what an unsupported observation lacks.
	Capability string `json:"capability,omitempty"`
}

// Run is the record of the run itself: whether the scenarios were switched on
// at all, and which of them ran. A run that was switched on and ran nothing is
// the suite's own red line, and a summary that counted only cases could not
// see it — there would be no cases to count.
type Run struct {
	Enabled   bool     `json:"enabled"`
	Scenarios []string `json:"scenarios"`
	// Against names what the run was checked against: where the schema came
	// from, and what the scenarios ran against. A green run that does not say
	// has answered a question nobody can check.
	Against []Against `json:"against"`
	// Failure is a failure of the run itself, before or around the cases —
	// a named harness version that could not be fetched, for one. A run
	// that carries one is red whatever its cases say.
	Failure string `json:"failure,omitempty"`
}

// Against is one part of a run and what it ran against: What is the part
// ("schema", "scenarios"); Harness and Version name a real harness when one
// was used, and How says how it was obtained, or what was used instead.
type Against struct {
	What    string `json:"what"`
	Harness string `json:"harness,omitempty"`
	Version string `json:"version,omitempty"`
	How     string `json:"how"`
}

// DescribeAgainst is the one-line form both ends print, so the -v line of a
// run and the summarizer's line read the same.
func DescribeAgainst(against []Against) string {
	parts := make([]string, 0, len(against))
	for _, one := range against {
		if one.Harness == "" {
			parts = append(parts, one.What+" "+one.How)
			continue
		}
		parts = append(parts, one.What+" from "+one.Harness+" "+one.Version+" ("+one.How+")")
	}
	if len(parts) == 0 {
		return "nothing recorded"
	}
	return strings.Join(parts, "; ")
}
