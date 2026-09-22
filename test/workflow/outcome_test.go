// Package workflow holds the workflow suite: scenarios that exercise a built
// rewake binary end to end, rather than its packages in process.
//
// Every file here is a test file on purpose: they stay out of an ordinary
// `go build ./...`, so the suite adds nothing to the build surface, and test
// scaffolding stays visibly test scaffolding. The five required checks still
// compile and analyze them, so nothing here can rot unnoticed.
//
// # Conventions a scenario has to follow
//
// Six of them are invisible from the call site, so they are written down
// rather than learnt by breaking something:
//
//   - Start is the first call in a scenario. It registers the finaliser, and
//     t.Cleanup runs last-in-first-out: registering it first is what makes the
//     verdict come after every process and directory cleanup registered later.
//   - Directories belong to the case, through RemoveOnFinish. t.TempDir deletes
//     at its own point in that order, which is before the cleanup checks run —
//     and a check cannot look for a leftover socket in a directory that is
//     already gone.
//   - The switch is REWAKE_WORKFLOW, and its value is read: 1/true/yes/on turn
//     the scenarios on, 0/false/no/off and an empty value leave them off, and
//     anything else stops the run instead of guessing.
//   - Scenarios never run in parallel — no t.Parallel, and nothing that starts
//     one case while another is live. Cleanup identifies a case's processes by
//     descent from this process, so two cases at once would each see the
//     other's work as nobody's: signals sent to a neighbour's processes, and
//     "pid N was still running" recorded against a case that never started it.
//     The regression that makes /proc unreadable would take down whatever else
//     was opening a file at that moment. Parallelism needs a real boundary —
//     a cgroup or a pid namespace — before it can be considered.
//   - Commands run through Case.Output or Case.OutputAllowingFailure, never
//     through cmd.Run or cmd.Output directly. A command started outside them
//     compiles fine and is never registered with the case, so the termination
//     budget does not apply to it and it surfaces later as a stray — a symptom
//     reported far from its cause.
//   - A scenario that starts a session ends it itself, before the case
//     finishes. Cleanup would otherwise end it, and correctly report that the
//     scenario left work running — again a symptom far from its cause.
//   - One case steps outside the isolation, and only that one: the schema
//     check generates the protocol schema with the Codex installed on the
//     machine, because inside a case `codex` is the shim. Nothing runs inside
//     the case and no credentials are touched — a file is generated and read.
//     Every other case stays inside its private world.
//
// # Where things are
//
//   - outcome_test.go: outcomes, the classifier, this documentation.
//   - case_test.go: Spec and Case — observations, deadlines, cleanup checks,
//     the verdict.
//   - process_test.go: the rest of Case — running commands and owning the
//     processes they start.
//   - termination_test.go: how long cleanup may take, and the loop that does it.
//   - adopted_test.go: finding descendants that left their process group.
//   - isolation_test.go: the private world of a case and its cleanup checks.
//   - suite_test.go: TestMain, the switch, building the binary under test.
//   - stub_scenario_test.go: the stage-0 scenario, the shape to copy.
//   - websocket_test.go: the small WebSocket the Codex wire needs.
//   - codexshim_test.go: the server half of the shim that plays a Codex
//     session, and codexshim_client_test.go the half the wrapper proxies.
//   - codexsession_test.go: launching a session on that shim and reading what
//     it reports about itself.
//   - codex_ready_scenario_test.go and codex_ready_controls_test.go: the first
//     scenario with a real harness, and its negative controls.
//   - schema_test.go: getting the schema from the installed Codex.
//   - schema_check_test.go: the closed register of schema keywords and the
//     walker that applies them; schema_kinds_test.go the JSON value kinds it
//     compares against; schema_register_test.go what neither may let past.
//   - codex_shape_test.go: the case that checks the shim's replies with it.
//   - codexshim_shape_test.go: the shape half of the shim's obligations.
//   - codexshim_contract_test.go: what the shim owes the protocol in
//     behavior — handshake ownership, silence, event order, refusals.
//   - classification_test.go, regression_test.go, regression_processes_test.go
//     and publication_test.go: the suite's tests of itself.
//
// The exported names here are not an API: nothing outside a test file can
// import them. They are exported for the move that stage 2 expects — when the
// fixtures need packages of their own, these helpers become an ordinary
// package next door and the names become real. Renaming twice would cost more
// than the inaccuracy.
package workflow

// Outcome is the category of a case or of a single observation inside it. The
// categories come from docs/check-runner.md and are not interchangeable: a
// missing observation is not a skip, and a skipped case is not a success.
type Outcome string

const (
	// Pass: every required observation was made and satisfied.
	Pass Outcome = "pass"
	// Fail: an observation was made and contradicted what the case claims.
	Fail Outcome = "fail"
	// Skip: a deliberate selection or policy decision, with a reason.
	Skip Outcome = "skip"
	// Unsupported: the harness lacks the capability the observation needs.
	Unsupported Outcome = "unsupported"
	// Incomplete: the case ran but a required observation was never made.
	Incomplete Outcome = "incomplete"
	// NotRun: no attempt was made at all.
	NotRun Outcome = "not-run"
)

// Green reports whether an outcome may be presented as a success. Only Pass
// qualifies: the whole point of the suite is that "we never looked" and "we
// looked and it was right" do not collapse into the same color.
func (o Outcome) Green() bool { return o == Pass }

// observation is one observation as the case saw it.
type observation struct {
	Name    string
	Detail  string
	Outcome Outcome
}

// classify decides a case's outcome from its declared observations, the
// records made for them, and whether cleanup succeeded. It is a pure function
// so the classifier itself can be tested without running a scenario.
//
// Precedence: a contradicted observation outranks a missing one, a missing one
// outranks an unsupported capability. Cleanup failure fails the case however
// well the observations went — a case that leaves processes or sockets behind
// has not finished, and the project has been burnt by publishing success
// before the late error arrived.
//
// Anything that is not a satisfied observation, a contradicted one or an
// absent capability counts as missing. Only three outcomes can be recorded
// today, but this function is deliberately separate from the methods that
// record them: a later Skipped or NotRun method must not open a silent path to
// green just because the switch already compiled.
func classify(required []string, made map[string]observation, cleanupErr error) (Outcome, string) {
	if cleanupErr != nil {
		return Fail, "cleanup failed: " + cleanupErr.Error()
	}
	var missing, unsupported []string
	for _, name := range required {
		made, ok := made[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		switch made.Outcome {
		case Pass:
		case Fail:
			return Fail, "observation " + quote(name) + " not satisfied: " + made.Detail
		case Unsupported:
			unsupported = append(unsupported, name)
		default:
			// Skip, Incomplete, NotRun, or anything added later: the
			// observation exists but carries no evidence, which is exactly
			// what Incomplete means.
			missing = append(missing, name+" (recorded as "+string(made.Outcome)+")")
		}
	}
	if len(missing) > 0 {
		return Incomplete, "required observation never made: " + list(missing)
	}
	if len(unsupported) > 0 {
		return Unsupported, "capability absent for: " + list(unsupported)
	}
	return Pass, ""
}

func quote(s string) string { return `"` + s + `"` }

func list(names []string) string {
	out := ""
	for i, name := range names {
		if i > 0 {
			out += ", "
		}
		out += quote(name)
	}
	return out
}
