package workflow

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// These tests run on every `go test ./...`, switch or no switch. The
// classifier is the one part of the suite whose failure is invisible from
// outside: a broken classifier does not crash, it paints red green. Checking
// it in the ordinary gate is what keeps that from rotting unnoticed.

func TestOnlyPassIsGreen(t *testing.T) {
	for _, outcome := range []Outcome{Fail, Skip, Unsupported, Incomplete, NotRun} {
		if outcome.Green() {
			t.Errorf("%s counted as a success", outcome)
		}
	}
	if !Pass.Green() {
		t.Error("pass did not count as a success")
	}
}

func TestObservationNeverMadeIsIncompleteNotPass(t *testing.T) {
	required := []string{"mailbox holds the message", "recipient read it"}
	made := map[string]observation{
		"mailbox holds the message": {Name: "mailbox holds the message", Outcome: Pass},
	}
	outcome, reason := classify(required, made, nil)
	if outcome != Incomplete {
		t.Fatalf("outcome=%s reason=%q, want incomplete", outcome, reason)
	}
	if !strings.Contains(reason, "recipient read it") {
		t.Errorf("reason does not name the missing observation: %q", reason)
	}
}

func TestContradictedObservationOutranksMissingOne(t *testing.T) {
	required := []string{"a", "b"}
	made := map[string]observation{"a": {Name: "a", Outcome: Fail, Detail: "epoch differed"}}
	outcome, reason := classify(required, made, nil)
	if outcome != Fail {
		t.Fatalf("outcome=%s, want fail", outcome)
	}
	if !strings.Contains(reason, "epoch differed") {
		t.Errorf("reason drops the detail: %q", reason)
	}
}

func TestAbsentCapabilityIsUnsupportedNotPass(t *testing.T) {
	required := []string{"delivery accepted", "acceptance acknowledged"}
	made := map[string]observation{
		"delivery accepted":       {Name: "delivery accepted", Outcome: Pass},
		"acceptance acknowledged": {Name: "acceptance acknowledged", Outcome: Unsupported, Detail: "socket adapter emits no ACK"},
	}
	outcome, _ := classify(required, made, nil)
	if outcome != Unsupported {
		t.Fatalf("outcome=%s, want unsupported", outcome)
	}
}

// An observation that exists but carries no evidence is not evidence. These
// outcomes cannot be recorded through the public methods today; the test is
// here because classify is deliberately separate from them, and the first
// method that records one must not find a green path already open.
func TestEvidencelessOutcomesCountAsMissing(t *testing.T) {
	for _, outcome := range []Outcome{Skip, Incomplete, NotRun} {
		made := map[string]observation{"a": {Name: "a", Outcome: outcome}}
		result, reason := classify([]string{"a"}, made, nil)
		if result != Incomplete {
			t.Errorf("an observation recorded as %s classified the case as %s, want incomplete", outcome, result)
		}
		if !strings.Contains(reason, string(outcome)) {
			t.Errorf("reason for %s does not say what was recorded: %q", outcome, reason)
		}
	}
}

// An outcome nobody has defined yet must land on the safe side. This is the
// property that makes the default branch in classify worth having: whatever a
// later change starts recording, it cannot arrive green by accident.
func TestUnknownOutcomeIsNotGreen(t *testing.T) {
	made := map[string]observation{"a": {Name: "a", Outcome: Outcome("something-new")}}
	outcome, reason := classify([]string{"a"}, made, nil)
	if outcome.Green() {
		t.Fatalf("an unknown outcome classified the case as %s", outcome)
	}
	if outcome != Incomplete {
		t.Errorf("outcome=%s, want incomplete", outcome)
	}
	if !strings.Contains(reason, "something-new") {
		t.Errorf("reason does not say what was recorded: %q", reason)
	}
}

func TestCleanupFailureFailsAnOtherwisePerfectCase(t *testing.T) {
	required := []string{"a"}
	made := map[string]observation{"a": {Name: "a", Outcome: Pass}}
	outcome, reason := classify(required, made, errors.New("socket still present"))
	if outcome != Fail {
		t.Fatalf("outcome=%s, want fail", outcome)
	}
	if !strings.Contains(reason, "socket still present") {
		t.Errorf("reason drops the cleanup error: %q", reason)
	}
}

// recorder stands in for *testing.T so a deliberately incomplete case can be
// finished and its verdict read without failing this test.
type recorder struct {
	cleanups []func()
	errors   []string
	fatals   []string
}

func (r *recorder) Helper()      {}
func (r *recorder) Failed() bool { return len(r.errors) > 0 }
func (r *recorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *recorder) Fatal(args ...any) { r.fatals = append(r.fatals, fmt.Sprint(args...)) }

func (r *recorder) Fatalf(format string, args ...any) {
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
}
func (r *recorder) Cleanup(fn func()) { r.cleanups = append(r.cleanups, fn) }

// finish runs the registered cleanups last-in-first-out, the way testing does,
// so the ordering the case relies on is the ordering under test.
func (r *recorder) finish() {
	for i := len(r.cleanups) - 1; i >= 0; i-- {
		r.cleanups[i]()
	}
}

func TestCaseWithAnUnmadeObservationEndsIncompleteAndRed(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "stub", Observations: []string{"looked at one thing", "looked at another"}})
	c.Observed("looked at one thing", "fine")
	// "looked at another" is deliberately never made.
	rec.finish()

	outcome, reason := c.Result()
	if outcome != Incomplete {
		t.Fatalf("outcome=%s reason=%q, want incomplete", outcome, reason)
	}
	if len(rec.errors) != 1 {
		t.Fatalf("errors=%v, want exactly one failure reported", rec.errors)
	}
	if !strings.Contains(rec.errors[0], "incomplete") {
		t.Errorf("failure does not say incomplete: %q", rec.errors[0])
	}
}

func TestCaseThatMadeEveryObservationEndsGreen(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "stub", Observations: []string{"only observation"}})
	c.Observed("only observation", "fine")
	rec.finish()

	if outcome, reason := c.Result(); outcome != Pass {
		t.Fatalf("outcome=%s reason=%q, want pass", outcome, reason)
	}
	if len(rec.errors) != 0 {
		t.Errorf("a complete case reported failures: %v", rec.errors)
	}
}

func TestCleanupChecksRunBeforeClassification(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "stub", Observations: []string{"only observation"}})
	c.Observed("only observation", "fine")
	c.CheckCleanup("state directory empty", func() error { return errors.New("one socket survived") })
	rec.finish()

	outcome, reason := c.Result()
	if outcome != Fail {
		t.Fatalf("outcome=%s, want fail: a case that leaves a socket behind has not finished", outcome)
	}
	if !strings.Contains(reason, "one socket survived") {
		t.Errorf("reason drops the cleanup detail: %q", reason)
	}
}

func TestResultIsNotRunUntilTheCaseFinishes(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "stub", Observations: []string{"only observation"}})
	if outcome, _ := c.Result(); outcome != NotRun {
		t.Fatalf("outcome=%s before finishing, want not-run", outcome)
	}
}
