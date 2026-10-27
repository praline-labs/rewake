package adapter

// RunProcess names the harness process of one run: what a tracker that reads
// the harness's own state needs of the session, and nothing more of it.
type RunProcess struct {
	PID   int
	Start uint64
}

// ThreadTracker is a harness whose conversation can change within one run and
// whose adapter learns it from the harness's own state, outside any service of
// the run: the host asks it, for each delivery, which conversation the run is
// in now.
type ThreadTracker interface {
	Thread(RunProcess) (string, error)
}

// ThreadSource is a run that follows its conversation itself: the host asks it
// which one a message is delivered into. It is the question a ThreadTracker
// answers, asked of what hears the harness rather than of its state, and an
// empty answer means unknown — never a refusal, because a conversation nobody
// has named yet must not stop the delivery.
type ThreadSource interface {
	Thread() (string, error)
}

// Resumer is a harness whose conversation a later launch continues by its id:
// the arguments after the launch command that resume it, for a terminal
// manager that restarts the session after its own restart (docs/launch.md#herdr).
type Resumer interface {
	ResumeArgs(conversation string) []string
}
