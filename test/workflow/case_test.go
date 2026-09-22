package workflow

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// Spec is what a scenario declares before it starts. The observation names are
// declared up front on purpose: a case that forgets to look is Incomplete, and
// that is only detectable if the list of what it owed exists independently of
// the code that was supposed to make the observations.
type Spec struct {
	// Name identifies the case in reports and in its artifact directory.
	Name string
	// Harness is the harness under test, empty while the suite is harness-free.
	Harness string
	// Observations are the names this case must make before it may be green.
	Observations []string
	// Deadline bounds the case. It is per case rather than per package: the
	// `go test` timeout kills the whole process, which leaves nothing to
	// classify and no diagnostic about where the case had got to.
	Deadline time.Duration
}

// caseT is the part of *testing.T a case uses. It exists so the classifier can
// be driven with a stand-in and its verdict inspected: a case that reports a
// failure cannot be examined through a real *testing.T without failing the
// test doing the examining.
type caseT interface {
	Helper()
	Errorf(format string, args ...any)
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Cleanup(func())
	// Failed reports whether anything has already gone wrong. A case whose
	// test has reported a failure cannot be classified green, however tidy its
	// own observations look.
	Failed() bool
}

// Case accumulates observations and publishes one classified result.
type Case struct {
	t        caseT
	spec     Spec
	deadline time.Time
	ctx      context.Context
	stop     context.CancelFunc

	started time.Time
	// scenario is true for a case a real scenario started through Start. The
	// suite's own tests drive cases through a stand-in to examine the
	// classifier, and several of those are *meant* to fail — publishing their
	// records would put a dozen deliberate failures into the summary of a run
	// that went perfectly.
	scenario  bool
	mu        sync.Mutex
	made      map[string]observation
	progress  string
	checks    []cleanupCheck
	dirs      []string
	processes []*owned
	result    Outcome
	reason    string
}

type cleanupCheck struct {
	what  string
	check func() error
}

const defaultDeadline = 60 * time.Second

// Start begins a case and arranges for its classification. The finaliser is
// registered first so that, with t.Cleanup running last-in-first-out, it runs
// after every process, directory and socket cleanup a scenario registers
// later: the result is published only once the case has actually finished.
func Start(t *testing.T, spec Spec) *Case {
	t.Helper()
	c := newCase(t, spec)
	c.scenario = true
	return c
}

func newCase(t caseT, spec Spec) *Case {
	t.Helper()
	if spec.Name == "" {
		t.Fatal("workflow: case needs a name")
	}
	if len(spec.Observations) == 0 {
		t.Fatalf("workflow: case %q declares no observations, so it could never be more than not-run", spec.Name)
	}
	if spec.Deadline == 0 {
		spec.Deadline = defaultDeadline
	}
	deadline := time.Now().Add(spec.Deadline)
	ctx, stop := context.WithDeadline(context.Background(), deadline)
	c := &Case{t: t, spec: spec, deadline: deadline, started: time.Now(), ctx: ctx, stop: stop, made: map[string]observation{}, result: NotRun}
	t.Cleanup(c.finish)
	return c
}

// Context expires at the case's deadline. Subprocesses are started with it, so
// an external command that hangs is killed by this case rather than by the
// package timeout of `go test`, which would take the whole run down with it and
// leave nothing to classify.
//
// It bounds polling and subprocesses — not arbitrary code. A scenario blocking
// on its own socket read inside this package has no context to cancel, and
// would still run into the package timeout.
func (c *Case) Context() context.Context { return c.ctx }

// Observed records that a required observation was made and satisfied.
func (c *Case) Observed(name, detail string) { c.record(name, Pass, detail) }

// Contradicted records that an observation was made and came out wrong. It
// does not stop the case: the remaining observations still carry information,
// and the classifier decides the outcome once everything has been collected.
func (c *Case) Contradicted(name, format string, args ...any) {
	c.record(name, Fail, fmt.Sprintf(format, args...))
}

// Unsupported records that the harness cannot offer this observation at all.
// A capability the harness lacks is not a license to stay silent: the case
// still has to say so by name, or a missing mechanism reads as a defect that
// swallowed it.
func (c *Case) Unsupported(name, reason string) { c.record(name, Unsupported, reason) }

// UnsupportedCapability is Unsupported with the capability named as a field,
// so whoever reads the record learns which one without parsing the reason.
func (c *Case) UnsupportedCapability(name, capability, reason string) {
	c.record(name, Unsupported, reason)
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.made[name]
	entry.Capability = capability
	c.made[name] = entry
}

func (c *Case) record(name string, outcome Outcome, detail string) {
	c.t.Helper()
	if !c.declares(name) {
		c.t.Fatalf("workflow: case %q recorded undeclared observation %q", c.spec.Name, name)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if previous, ok := c.made[name]; ok {
		c.t.Fatalf("workflow: observation %q already recorded as %s: %s", name, previous.Outcome, previous.Detail)
	}
	c.made[name] = observation{Name: name, Detail: detail, Outcome: outcome}
}

func (c *Case) declares(name string) bool {
	for _, declared := range c.spec.Observations {
		if declared == name {
			return true
		}
	}
	return false
}

// Note records where the case has got to. It is what a deadline diagnostic
// prints, so that a timeout says which state was last reached rather than only
// that time ran out.
func (c *Case) Note(state string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.progress = state
}

// Await waits for an observable condition instead of sleeping for a guess. A
// sleep makes a slow machine look like a defect and a lost wakeup look like
// slowness; waiting on the condition with a stated deadline keeps the two
// apart.
func (c *Case) Await(state string, condition func() bool) {
	c.t.Helper()
	c.Note("waiting for " + state)
	for {
		if condition() {
			c.Note(state)
			return
		}
		if time.Now().After(c.deadline) {
			c.t.Fatalf("workflow: case %q ran out of time waiting for %s; last observed: %s",
				c.spec.Name, state, c.lastProgress())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Expired reports whether the case is past its deadline, for loops that own
// their own waiting.
func (c *Case) Expired() bool { return time.Now().After(c.deadline) }

func (c *Case) lastProgress() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.progress == "" {
		return "nothing"
	}
	return c.progress
}

// CheckCleanup registers something that must be true once the case is over —
// no surviving process, no leftover socket, no registry entry. Cleanup is
// verified rather than assumed, and its failure fails the case.
func (c *Case) CheckCleanup(what string, check func() error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks = append(c.checks, cleanupCheck{what: what, check: check})
}

// RemoveOnFinish hands a directory to the case to delete once it is over. The
// case owns the removal rather than t.TempDir because cleanup is *checked*:
// t.TempDir deletes at its own point in the cleanup order, which is before the
// checks run, and a check cannot look for a leftover socket in a directory
// that has already gone. A case that did not end green keeps its directory and
// says where it is, so the evidence outlives the run.
func (c *Case) RemoveOnFinish(dir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dirs = append(c.dirs, dir)
}

// Result reports the classified outcome and its reason. It is empty until the
// case has finished, which is what makes the classifier testable on its own.
func (c *Case) Result() (Outcome, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.result, c.reason
}

// finish publishes the verdict, and only once the case has really ended:
// processes joined, cleanup checked, directories dealt with. Everything that
// can still turn a green case red happens before the result is stored, because
// a Result that says pass while the run is red is precisely the disagreement
// this package exists to remove.
func (c *Case) finish() {
	// Ends and joins first: a result published while a process is still
	// running is a result published before the case finished.
	cleanupErr := c.joinProcesses()
	if checkErr := c.runCleanupChecks(); cleanupErr == nil {
		cleanupErr = checkErr
	}

	c.mu.Lock()
	made := make(map[string]observation, len(c.made))
	for name, r := range c.made {
		made[name] = r
	}
	c.mu.Unlock()

	outcome, reason := classify(c.spec.Observations, made, cleanupErr)
	outcome, reason = c.applyRunState(outcome, reason)
	if outcome == Unsupported && isGate(c.spec.Harness) {
		// The gate column cannot lose an observation. An absent capability
		// there is not a fact about the harness but a hole in the check that
		// blocks regressions — and the review that found it removed one line
		// from the column table and watched the run stay green.
		outcome, reason = Fail, "the gate column reported a capability absent: "+reason
	}

	kept := ""
	if c.acceptable(outcome) {
		if err := c.removeDirs(); err != nil {
			outcome, reason = Fail, "could not remove the case directory: "+err.Error()
			kept = c.evidence()
		}
	} else {
		kept = c.evidence()
	}
	c.stop()

	c.mu.Lock()
	c.result, c.reason = outcome, reason
	c.mu.Unlock()

	// The machine-readable record, printed for every case whatever its
	// outcome. It is the only thing a summary is built from: prose is for a
	// person reading the run, and a summary assembled by reading prose grows
	// with every print somebody adds.
	c.publishRecord(outcome, reason, made, kept != "")

	if !c.acceptable(outcome) {
		// One message, not two: the verdict and where to look belong together,
		// or the output budget of a failing run doubles for no information.
		c.t.Errorf("case %q: %s: %s%s", c.spec.Name, outcome, reason, kept)
	}
}

// acceptable reports whether an outcome leaves the run intact for this case.
// Pass always does. Unsupported does only off the gate column: a capability a
// younger harness lacks is a fact about that harness, and a column that lacks
// one could otherwise never finish a run; on the gate it is a check that no
// longer happens. Everything else — contradicted, never made, skipped — means
// somebody has to look.
func (c *Case) acceptable(outcome Outcome) bool {
	return outcome == Pass || outcome == Unsupported && !isGate(c.spec.Harness)
}

// applyRunState folds in what happened around the observations: a deadline
// that expired, or a failure the test reported before classification. Either
// one means the case did not finish cleanly, whatever its own records say.
//
// An expired deadline is stated first even when something else already failed
// the case, because it usually explains that something else — the processes
// the case killed, the work it never finished. Burying it under a downstream
// symptom sends the reader after the wrong thing.
func (c *Case) applyRunState(outcome Outcome, reason string) (Outcome, string) {
	if c.ctx.Err() != nil {
		expired := fmt.Sprintf("the case deadline of %s expired; last observed: %s", c.spec.Deadline, c.lastProgress())
		if !c.acceptable(outcome) {
			expired += "; also: " + reason
		}
		return Fail, expired
	}
	if !c.acceptable(outcome) {
		return outcome, reason
	}
	if c.t.Failed() {
		return Fail, "the test reported a failure before the case was classified"
	}
	return outcome, reason
}

// removeDirs deletes the case's directories. It runs only for a case that is
// otherwise green, and its own failure turns that case red — a run that cannot
// clean up after itself has not finished, and leaves the next one a surprise.
func (c *Case) removeDirs() error {
	c.mu.Lock()
	dirs := c.dirs
	c.mu.Unlock()
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
	}
	return nil
}

// evidence names the directories a red case keeps, for the verdict message.
func (c *Case) evidence() string {
	c.mu.Lock()
	dirs := c.dirs
	c.mu.Unlock()
	kept := ""
	for _, dir := range dirs {
		kept += "\n\tevidence: " + dir
	}
	return kept
}

func (c *Case) runCleanupChecks() error {
	c.mu.Lock()
	checks := c.checks
	c.mu.Unlock()
	for _, check := range checks {
		if err := check.check(); err != nil {
			return fmt.Errorf("%s: %w", check.what, err)
		}
	}
	return nil
}
