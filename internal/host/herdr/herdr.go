// Package herdr reports main's run to herdr, a terminal workspace manager for
// coding agents, when main runs in one of its panes. herdr finds an agent by
// its pane's foreground process, which in rewake's shell is the shell, so main
// says itself which agent it is and what it is doing (docs/launch.md#herdr),
// under a name of rewake's own with the harness's shown in its place.
// Workers never report: they run in no pane of herdr's.
package herdr

import (
	"context"
	"encoding/json"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/praline-labs/rewake/internal/core/session"
)

const (
	// Source names rewake's reports to herdr, stable across runs: herdr
	// keeps the pane for the source that holds it.
	Source = "rewake"
	// command is the word that starts rewake from a person's PATH.
	command = "rewake"
	// labelPrefix makes the agent's name one herdr does not know: herdr
	// drops, exiting 0, every report naming a known agent it has seen leave
	// the pane, until it sees that agent's process there again, which in
	// rewake's shell it never does (herdr 0.9.5, docs/research-herdr.md).
	labelPrefix = "rewake-"

	maxResumeArgs  = 64
	maxResumeBytes = 8 << 10

	// A report never slows main: one in flight, cut after this.
	reportTimeout = 2 * time.Second
	poll          = 250 * time.Millisecond
)

// retryAfter is how long a failed report waits before it is tried again, not
// at every tick; a variable for the tests.
var retryAfter = 5 * time.Second

// Pane is the herdr pane a run reports to, from the variables herdr gives
// every process in it.
type Pane struct {
	ID  string
	Bin string
}

// FromEnv is the pane a run is in, when it runs inside herdr with every
// variable herdr sets; outside herdr there is none and nothing reports.
func FromEnv(getenv func(string) string) (Pane, bool) {
	pane := Pane{ID: getenv("HERDR_PANE_ID"), Bin: getenv("HERDR_BIN_PATH")}
	if getenv("HERDR_ENV") != "1" || pane.ID == "" || pane.Bin == "" || getenv("HERDR_SOCKET_PATH") == "" {
		return Pane{}, false
	}
	return pane, true
}

// The states herdr takes.
const (
	Idle    = "idle"
	Working = "working"
	Blocked = "blocked"
	Unknown = "unknown"
)

// StateOf is herdr's state for a run's snapshot, with the message a blocked
// state carries, read off the snapshot's verdict
// (docs/session-state.md#activity): a compaction is work, a session waiting
// on something is blocked with the verdict's word as the message, and an
// activity unknown or stale is unknown — never idle, which would tell a
// person or a script waiting on it that it is done.
func StateOf(snapshot session.Snapshot) (string, string) {
	verdict := snapshot.Judge()
	switch {
	case verdict.Stale:
		return Unknown, ""
	case verdict.Blocked != "":
		return Blocked, verdict.Activity
	case verdict.Activity == session.ActivityIdle:
		return Idle, ""
	case verdict.Activity == session.ActivityWorking || verdict.Activity == session.ActivityCompacting:
		return Working, ""
	}
	return Unknown, ""
}

// ValidResume says herdr would take argv as a resume command: a plain command
// name first, no apostrophe or control character in any argument, at most 64
// arguments and 8 KiB. herdr refuses the whole report otherwise, the state with
// it, so a command that breaks the rules is left out instead.
func ValidResume(argv []string) bool {
	if len(argv) == 0 || len(argv) > maxResumeArgs || argv[0] == "" || strings.ContainsRune(argv[0], '/') {
		return false
	}
	size := 0
	for _, arg := range argv {
		size += len(arg) + 1
		if strings.ContainsRune(arg, '\'') || strings.ContainsFunc(arg, unicode.IsControl) {
			return false
		}
	}
	return size <= maxResumeBytes
}

// Config is one run's reporting.
type Config struct {
	Pane Pane
	// Agent is the harness's id: herdr shows it, while the reports go under
	// rewake's own name for it.
	Agent string
	// State is the run's telemetry.
	State func() session.Snapshot
	// Resume is what follows the command word to continue a conversation,
	// nil when the harness cannot.
	Resume func(conversation string) []string
	// Build is this build's id: the resume command is reported only when
	// the rewake on PATH is this build.
	Build string
	// Run runs one herdr command; nil runs the program.
	Run func(ctx context.Context, bin string, args ...string) error
	// Probe asks the rewake on PATH for its build id; nil runs it.
	Probe func(ctx context.Context) string
}

// report is one state report.
type report struct {
	state, message, session string
	resume                  []string
}

func (r report) equal(other report) bool {
	return r.state == other.state && r.message == other.message && r.session == other.session && slices.Equal(r.resume, other.resume)
}

// Start reports the run's state to its pane each time it changes, until the
// returned stop, which releases the pane. Failures are ignored: a report
// that fails is tried again later with the state as it is then.
func Start(parent context.Context, config Config) (stop func()) {
	if config.Run == nil {
		config.Run = run
	}
	if config.Probe == nil {
		config.Probe = probe
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	seq := sequence{}
	reported := false
	go func() {
		defer close(done)
		// herdr reruns the command in a new shell of the pane, so its word
		// must start this build: another would bring main back under another
		// build's state.
		word := ""
		if config.Resume != nil && config.Build != "" && config.Probe(ctx) == config.Build {
			word = command
		}
		var last *report
		var retry time.Time
		// held is whether a report of this run has been accepted: only then
		// does herdr take a resume command, which it refuses with
		// resume_not_accepted from a reporter not yet holding the pane
		// (herdr 0.9.5, docs/research-herdr.md).
		held := false
		// named is whether herdr took the harness's id as the name it
		// shows; once a run is enough, herdr keeps it for the source.
		named := false
		ticker := time.NewTicker(poll)
		defer ticker.Stop()
		for {
			current := config.report(word)
			if !held {
				current.resume = nil
			}
			if (last == nil || !last.equal(current)) && !time.Now().Before(retry) {
				reported = true
				attempt, cancelAttempt := context.WithTimeout(ctx, reportTimeout)
				err := config.Run(attempt, config.Pane.Bin, config.args(current, seq.next())...)
				cancelAttempt()
				if err == nil {
					last, held = &current, true
				} else {
					// A failure may mean the hold is gone: state alone first.
					last, held, retry = nil, false, time.Now().Add(retryAfter)
				}
			}
			if held && !named && !time.Now().Before(retry) {
				attempt, cancelAttempt := context.WithTimeout(ctx, reportTimeout)
				err := config.Run(attempt, config.Pane.Bin, "pane", "report-metadata", config.Pane.ID,
					"--source", Source, "--agent", config.label(), "--display-agent", config.Agent,
					"--seq", strconv.FormatInt(seq.next(), 10))
				cancelAttempt()
				if named = err == nil; !named {
					retry = time.Now().Add(retryAfter)
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() {
		cancel()
		<-done
		if !reported {
			return
		}
		release, cancelRelease := context.WithTimeout(context.WithoutCancel(parent), reportTimeout)
		defer cancelRelease()
		_ = config.Run(release, config.Pane.Bin, "pane", "release-agent", config.Pane.ID,
			"--source", Source, "--agent", config.label(), "--seq", strconv.FormatInt(seq.next(), 10))
	}
}

// label is the name the reports go under, the harness's id made one herdr
// does not know.
func (c Config) label() string { return labelPrefix + c.Agent }

func (c Config) report(word string) report {
	snapshot := c.State()
	state, message := StateOf(snapshot)
	current := report{state: state, message: message, session: snapshot.Thread}
	if word != "" && snapshot.Thread != "" {
		if argv := append([]string{word}, c.Resume(snapshot.Thread)...); ValidResume(argv) {
			current.resume = argv
		}
	}
	return current
}

func (c Config) args(r report, seq int64) []string {
	args := []string{
		"pane", "report-agent", c.Pane.ID, "--source", Source, "--agent", c.label(),
		"--state", r.state, "--seq", strconv.FormatInt(seq, 10),
	}
	if r.message != "" {
		args = append(args, "--message", r.message)
	}
	if r.session != "" {
		args = append(args, "--agent-session-id", r.session)
	}
	if len(r.resume) > 0 {
		args = append(append(args, "--"), r.resume...)
	}
	return args
}

// sequence numbers the reports by the wall clock in microseconds, so they
// keep rising across main's restarts as herdr requires, and by one past the
// last where the clock did not move on.
type sequence struct{ last int64 }

func (s *sequence) next() int64 {
	s.last = max(time.Now().UnixMicro(), s.last+1)
	return s.last
}

// run starts a herdr command with nothing of main's terminal: a report must
// not write over the screen main draws.
func run(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.WaitDelay = 500 * time.Millisecond
	return cmd.Run()
}

// probe is the build id of the rewake the person's PATH finds, empty when
// there is none or it does not say.
func probe(ctx context.Context) string {
	path, err := exec.LookPath(command)
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, reportTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version", "--json")
	cmd.WaitDelay = 500 * time.Millisecond
	out, err := cmd.Output()
	var version struct {
		Build string `json:"build"`
	}
	if err != nil || json.Unmarshal(out, &version) != nil {
		return ""
	}
	return version.Build
}
