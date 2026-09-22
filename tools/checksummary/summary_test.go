package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/test/workflow/record"
)

// What the summarizer must get right, driven by recorded streams rather than
// by running anything. Each case here is a shape the suite really produces,
// and each has a way of being wrong that would not look wrong: a run that
// summarized only the part it could read, an unsupported cell counted as a
// pass, a red run reported green because no case said so.

// stream builds a `go test -json` stream out of output lines, the way the
// engine writes one.
func stream(t *testing.T, lines []string, packageFailed bool) string {
	t.Helper()
	var out strings.Builder
	for _, line := range lines {
		encoded, err := json.Marshal(map[string]any{
			"Action": "output", "Package": "workflow", "Test": "TestSomething", "Output": line + "\n",
		})
		if err != nil {
			t.Fatal(err)
		}
		out.Write(encoded)
		out.WriteByte('\n')
	}
	action := "pass"
	if packageFailed {
		action = "fail"
	}
	encoded, err := json.Marshal(map[string]any{"Action": action, "Package": "workflow"})
	if err != nil {
		t.Fatal(err)
	}
	out.Write(encoded)
	out.WriteByte('\n')
	return out.String()
}

func runRecordLine(scenarios ...string) string {
	encoded, _ := json.Marshal(record.Run{Enabled: true, Scenarios: scenarios})
	return record.RunMark + string(encoded)
}

func caseLine(t *testing.T, one record.Case) string {
	t.Helper()
	encoded, err := json.Marshal(one)
	if err != nil {
		t.Fatal(err)
	}
	return record.CaseMark + string(encoded)
}

func summarize(t *testing.T, text string) *summary {
	t.Helper()
	found, err := parseStream(strings.NewReader(text))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	return found
}

func TestGreenRunIsGreenAndSaysWhatRan(t *testing.T) {
	text := stream(t, []string{
		caseLine(t, record.Case{Case: "task-report", Harness: "codex", Outcome: outcomePass}),
		runRecordLine("task-report"),
	}, false)
	found := summarize(t, text)
	if !found.green() {
		t.Error("a run whose only case passed was not green")
	}
	rendered := render(t, found)
	if !strings.Contains(rendered, "1 scenario, 1 case: 1 pass") {
		t.Errorf("the line does not say what ran:\n%s", rendered)
	}
	if strings.Contains(rendered, "FAIL") {
		t.Errorf("a green run printed a failure:\n%s", rendered)
	}
}

// A failing case must bring its name, its column, the observation that failed
// and one path. Without the path the next read is a search.
func TestRedRunNamesTheObservationAndTheEvidence(t *testing.T) {
	text := stream(t, []string{
		caseLine(t, record.Case{
			Case: "batch-arrival", Harness: "codex", Outcome: "fail",
			Reason: "observation not satisfied",
			Observations: []record.Observation{
				{Name: "two close letters are one group", Outcome: "fail", Detail: "the delivery named three"},
				{Name: "an overview consumed nothing", Outcome: outcomePass},
			},
			Evidence: []string{"/tmp/rewake-case-1"},
		}),
		runRecordLine("batch-arrival"),
	}, true)
	found := summarize(t, text)
	if found.green() {
		t.Error("a run with a failing case was green")
	}
	rendered := render(t, found)
	for _, want := range []string{"FAIL", "batch-arrival/codex", "two close letters are one group", "the delivery named three", "/tmp/rewake-case-1"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the failure does not carry %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "an overview consumed nothing") {
		t.Errorf("the excerpt lists observations that passed:\n%s", rendered)
	}
}

// Unsupported is green for the run and visible in its own right: a harness
// that cannot offer a capability has not failed and has not passed.
func TestUnsupportedIsGreenAndNamed(t *testing.T) {
	text := stream(t, []string{
		caseLine(t, record.Case{
			Case: "mid-turn", Harness: "claude-code", Outcome: outcomeUnsupported,
			Reason: "claude-code declares no observes-mid-turn-arrival",
		}),
		runRecordLine("mid-turn"),
	}, false)
	found := summarize(t, text)
	if !found.green() {
		t.Error("an unsupported case made the run red")
	}
	rendered := render(t, found)
	for _, want := range []string{"unsupported", "claude-code", "observes-mid-turn-arrival"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the unsupported line does not carry %q:\n%s", want, rendered)
		}
	}
}

// A line the summarizer cannot read fails the run. Skipping it would leave a
// summary that describes the part it happened to understand.
func TestAnUnparseableEventFailsTheRun(t *testing.T) {
	text := stream(t, []string{runRecordLine("stub")}, false) + "{not json at all\n"
	if _, err := parseStream(strings.NewReader(text)); err == nil {
		t.Error("an unreadable event was skipped instead of failing the run")
	}
	// And a record that is marked but malformed, which is the likelier drift.
	broken := stream(t, []string{record.CaseMark + `{"case":`, runRecordLine("stub")}, false)
	if _, err := parseStream(strings.NewReader(broken)); err == nil {
		t.Error("an unreadable case record was skipped")
	}
}

// The suite's own red line: the switch was set and nothing ran.
func TestASwitchedOnRunWithNoScenariosIsRed(t *testing.T) {
	found := summarize(t, stream(t, []string{runRecordLine()}, false))
	if found.green() {
		t.Error("a run that was switched on and ran nothing was green")
	}
	if !strings.Contains(render(t, found), "no scenario ran") {
		t.Error("the line does not say that nothing ran")
	}
}

// A stream with no run record at all is not a suite run, and saying so beats
// printing an empty summary that looks like a clean one.
func TestAStreamWithoutARunRecordIsRefused(t *testing.T) {
	if _, err := parseStream(strings.NewReader(stream(t, nil, false))); err == nil {
		t.Error("a stream with no run record was accepted")
	}
}

// An engine that failed with no case reporting it — a build error, a panic —
// must not come out green because there is nothing red to point at.
func TestAFailingEngineWithNoRedCaseIsStillRed(t *testing.T) {
	found := summarize(t, stream(t, []string{
		caseLine(t, record.Case{Case: "stub", Outcome: outcomePass}),
		runRecordLine("stub"),
	}, false))
	code := 2
	found.EngineExit = &code
	if found.green() {
		t.Error("a failing engine was reported green")
	}
	if !strings.Contains(render(t, found), "go test exited 2") {
		t.Error("the engine failure is not named")
	}
}

func TestSummaryFileHoldsTheRecords(t *testing.T) {
	found := summarize(t, stream(t, []string{
		caseLine(t, record.Case{Case: "stub", Harness: "codex", Outcome: outcomePass, DurationMs: 7}),
		runRecordLine("stub"),
	}, false))
	into := t.TempDir()
	path, err := writeSummary(into, found)
	if err != nil {
		t.Fatalf("writing: %v", err)
	}
	if filepath.Base(path) != "summary.json" {
		t.Errorf("the file is at %s", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var back summary
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("the file is not readable JSON: %v", err)
	}
	if len(back.Cases) != 1 || back.Cases[0].Case != "stub" || back.Cases[0].DurationMs != 7 {
		t.Errorf("the records did not survive the file: %#v", back.Cases)
	}
}

func render(t *testing.T, found *summary) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "rendered")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	found.render(file, "/tmp/summary.json")
	raw, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The engine splits a long output line across several events, and a record of
// a case with a dozen observations is long. Reading each event as a line cut
// the first real run's records in half — this is that, in miniature.
func TestARecordSplitAcrossEventsIsReassembled(t *testing.T) {
	whole := caseLine(t, record.Case{Case: "mid-turn", Harness: "codex", Outcome: outcomePass})
	half := len(whole) / 2
	text := chunks(t, []string{whole[:half], whole[half:] + "\n", runRecordLine("mid-turn") + "\n"})
	found := summarize(t, text)
	if len(found.Cases) != 1 || found.Cases[0].Case != "mid-turn" {
		t.Fatalf("the split record did not survive: %#v", found.Cases)
	}
	// And the other half of the rule: a record the stream ended in the middle
	// of is refused, not guessed at.
	cut := chunks(t, []string{runRecordLine("mid-turn") + "\n", whole[:half]})
	if _, err := parseStream(strings.NewReader(cut)); err == nil {
		t.Error("a record cut short by the end of the stream was accepted")
	}
}

// chunks writes output events verbatim, without adding newlines, so a caller
// can split a line wherever it likes.
func chunks(t *testing.T, pieces []string) string {
	t.Helper()
	var out strings.Builder
	for _, piece := range pieces {
		encoded, err := json.Marshal(map[string]any{
			"Action": "output", "Package": "workflow", "Test": "TestSomething", "Output": piece,
		})
		if err != nil {
			t.Fatal(err)
		}
		out.Write(encoded)
		out.WriteByte('\n')
	}
	encoded, _ := json.Marshal(map[string]any{"Action": "pass", "Package": "workflow"})
	out.Write(encoded)
	out.WriteByte('\n')
	return out.String()
}
