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
	Case         string        `json:"case"`
	Harness      string        `json:"harness"`
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
}

// Run is the record of the run itself: whether the scenarios were switched on
// at all, and which of them ran. A run that was switched on and ran nothing is
// the suite's own red line, and a summary that counted only cases could not
// see it — there would be no cases to count.
type Run struct {
	Enabled   bool     `json:"enabled"`
	Scenarios []string `json:"scenarios"`
}
