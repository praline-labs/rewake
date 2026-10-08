package workflow

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// scenarioSession launches a real rewake session whose harness is the shim, and
// gives a scenario what it needs to observe rewake's own state.
type scenarioSession struct {
	name     string
	state    string // where the session writes what `rewake list` told it
	mailbox  string // where it writes what its own `rewake inbox` returned
	exitFile string // the file that asks this session to stop
	turns    string // where it records the turns it accepted
	// delivered is where it records the id of every message a delivery named.
	delivered string
	// ready is the file a session creates once it can receive.
	ready string
	// groups is where it records each delivery as one group, reads is where it
	// records each inbox call made under shimReadEach, and sends is where a
	// sender records what became of each letter it sent.
	groups  string
	reads   string
	sends   string
	home    string
	process *owned
}

// mailboxRead is what this session's own `rewake inbox` returned, as the
// session wrote it. A scenario reads this file rather than the mailbox itself:
// the point is what the session saw, not what the test can see.
func (s *scenarioSession) mailboxRead() string {
	raw, err := os.ReadFile(s.mailbox)
	if err != nil {
		return ""
	}
	return string(raw)
}

// deliveredIDs are the message ids this session was told about, one per line,
// with an unreadable file and an empty one answering alike — both mean the
// scenario has nothing to correlate against, and both make it fail.
// as the session recorded them when the delivery arrived. A report names the
// messages it settles; these are what that naming has to match.
func (s *scenarioSession) deliveredIDs() []string {
	raw, err := os.ReadFile(s.delivered)
	if err != nil {
		return nil
	}
	var ids []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			ids = append(ids, line)
		}
	}
	return ids
}

// alive reports whether the session is still running — the process and the
// group it owns. A session that left before the case was judged took its
// evidence with it, and every observation recorded from its files describes a
// session that no longer exists.
func (s *scenarioSession) alive() bool {
	return !s.process.finished() && s.process.groupAlive()
}

// acceptedTurns is what this session recorded about the turns it accepted.
func (s *scenarioSession) acceptedTurns() string {
	raw, err := os.ReadFile(s.turns)
	if err != nil {
		return ""
	}
	return string(raw)
}

// startHarnessSession launches a session on a column: the program for that
// harness goes on the case's PATH under the harness's own name, the built
// rewake goes beside it (the session runs `rewake list` itself), and the
// session is launched to run it.
func startHarnessSession(t *testing.T, c *Case, iso *Isolation, harness, name, role string, controls ...string) *scenarioSession {
	t.Helper()
	return startSessionWith(t, c, iso, harness, name, role, nil, nil, controls...)
}

// startSessionWith is startHarnessSession with more of rewake's own launch
// flags, which go before the harness word, and arguments for the harness,
// which go after it.
func startSessionWith(t *testing.T, c *Case, iso *Isolation, harness, name, role string, flags, harnessArgs []string, controls ...string) *scenarioSession {
	t.Helper()
	return startSessionIn(t, c, iso, "", harness, name, role, flags, harnessArgs, controls...)
}

// startSessionIn is startSessionWith launched from dir rather than the case's
// home, for a scenario about where a launch works.
func startSessionIn(t *testing.T, c *Case, iso *Isolation, dir, harness, name, role string, flags, harnessArgs []string, controls ...string) *scenarioSession {
	t.Helper()
	installShim(t, c, iso)
	// rewake appends the harness to the requested name.
	session := &scenarioSession{
		name:  name + "-" + harness,
		state: filepath.Join(iso.Home, name+".state.json"),
		// Per session: a shared stop file would end every session at once.
		exitFile:  filepath.Join(iso.Home, name+".exit"),
		turns:     filepath.Join(iso.Home, name+".turns"),
		delivered: filepath.Join(iso.Home, name+".delivered"),
		groups:    filepath.Join(iso.Home, name+".groups"),
		reads:     filepath.Join(iso.Home, name+".reads"),
		sends:     filepath.Join(iso.Home, name+".sends"),
		ready:     filepath.Join(iso.Home, name+".listening"),
	}

	// Only one session in a room may be main, and only a main sees telemetry.
	// A scenario that needs two sessions gives that role to the one whose
	// telemetry it has to observe.
	launch := iso.Command(append(append(append([]string{"--name", name, role}, flags...), harness), harnessArgs...)...)
	if dir != "" {
		launch.Dir = dir
	}
	launch.Env = append(iso.Env(),
		shimEnv+"=1",
		shimHarness+"="+harness,
		"RW_SHIM_TEST_EXE="+testExecutable(t),
		shimStateFile+"="+session.state,
		shimReadyFile+"="+session.ready,
	)
	launch.Env = append(launch.Env, controls...)
	// The mailbox the session reads with its own rewake, and what the scenario
	// reads back afterwards.
	session.mailbox = filepath.Join(iso.Home, name+".mailbox")
	launch.Env = append(launch.Env, shimMailboxFile+"="+session.mailbox, shimExitFile+"="+session.exitFile,
		shimTurnsFile+"="+session.turns, shimDeliveredFile+"="+session.delivered,
		shimGroupsFile+"="+session.groups, shimReadsFile+"="+session.reads,
		shimSendsFile+"="+session.sends)
	// Not marked as an expected failure: the shim is *asked* to stop and
	// exits cleanly, so anything else is a real failure of the session and has
	// to reach the verdict. The earlier blanket "failure expected" here hid an
	// exit code of 42 completely.
	release := c.captureStderr(launch, iso.Home, name)
	process, err := c.start(launch, false)
	release()
	if err != nil {
		t.Fatalf("launching the session: %v", err)
	}
	session.home = iso.Home
	session.process = process
	return session
}

// stop asks the session to end the way a person would, and waits for it. A
// scenario ends its own session: leaving that to the case's cleanup would
// report every scenario as having left work running.
func (s *scenarioSession) stop(c *Case) error {
	if err := os.WriteFile(s.exitFile, nil, 0o600); err != nil {
		return err
	}
	c.Note("waiting for the session to end")
	for !c.Expired() {
		if s.process.finished() && !s.process.groupAlive() {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return errors.New("the session did not end after being asked to")
}

// replaceScript puts an executable script at path by writing it beside and
// renaming it over, never by rewriting the file in place. A case starts more
// than one session, each installing the shims again, while an earlier session
// of the same case is running them: a script open for writing when another
// process execs it fails that exec with ETXTBSY, and a rename swaps the file
// without ever holding one open.
func replaceScript(path, content string) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	_, writeErr := temporary.WriteString(content)
	closeErr := temporary.Close()
	if err := errors.Join(writeErr, closeErr, os.Chmod(temporary.Name(), 0o700)); err != nil {
		_ = os.Remove(temporary.Name())
		return err
	}
	return os.Rename(temporary.Name(), path)
}

// installShim writes the programs the session needs on its PATH: one per
// harness — `claude` and `fixture` — each of which re-executes this
// test binary as that harness, and `rewake`, which is the binary built for this
// run. The isolation deliberately keeps an
// installed rewake off PATH; this puts *ours* there, which is the one the
// scenario means to measure.
func installShim(t *testing.T, c *Case, iso *Isolation) {
	t.Helper()
	// One script per harness, each naming which fixture to be. The wrapper
	// runs the harness by name, so the name of the file is what decides which
	// column a session belongs to.
	for _, harness := range []string{"claude", "fixture"} {
		script := "#!/bin/sh\nexec env " + shimHarness + "=" + harness +
			" \"$RW_SHIM_TEST_EXE\" -test.run=TestShimHelper -- \"$@\"\n"
		if err := replaceScript(filepath.Join(iso.ShimDir, harness), script); err != nil {
			t.Fatalf("installing the %s shim: %v", harness, err)
		}
	}
	link := "#!/bin/sh\nexec " + iso.binary + " \"$@\"\n"
	if err := replaceScript(filepath.Join(iso.ShimDir, "rewake"), link); err != nil {
		t.Fatalf("installing rewake for the session: %v", err)
	}
	// Checked, not assumed: a typo here would not fail loudly. The session
	// would simply never run rewake, no state file would appear, and the
	// scenario would die on its deadline complaining about registration —
	// pointing at the wrong thing entirely.
	found, ok, err := iso.Lookup("rewake")
	if err != nil {
		t.Fatalf("checking the installed rewake: %v", err)
	}
	if !ok {
		t.Fatal("rewake is not on the case's PATH after being installed")
	}
	check := exec.Command(found, "--help")
	check.Env = iso.Env()
	out, err := c.Output(check)
	if err != nil || len(out) == 0 {
		t.Fatalf("the rewake on the case's PATH does not run: %v", err)
	}
}

func testExecutable(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("locating the test binary: %v", err)
	}
	return path
}

// TestShimHelper is the process each harness script re-executes. It does
// nothing unless it was started as the shim.
func TestShimHelper(_ *testing.T) {}

// listing is the part of `rewake list --json` a scenario reads. Telemetry is
// nested and only present for a verified main, which is why the session is
// launched as one.
type listing struct {
	Sessions []struct {
		Name      string `json:"name"`
		Harness   string `json:"harness"`
		Telemetry struct {
			Activity  string `json:"activity"`
			Selection string `json:"selection"`
			Thread    string `json:"primaryThread"`
		} `json:"telemetry"`
	} `json:"sessions"`
}

// await reads what the session last wrote, until the predicate holds or the
// case runs out of time. The file is written by the session, not the test.
func (s *scenarioSession) await(c *Case, what string, ready func(listing) bool) (listing, bool) {
	var last listing
	c.Note("waiting for " + what)
	for !c.Expired() {
		raw, err := os.ReadFile(s.state)
		if err == nil && len(raw) > 0 {
			var current listing
			if json.Unmarshal(raw, &current) == nil {
				last = current
				if ready(current) {
					c.Note(what)
					return current, true
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return last, false
}

// find returns the session's own row. rewake names a session after the
// harness it launched — `worker` becomes `worker-fixture` — so the scenario
// matches on the name rewake gave it, not on the one it asked for.
func (l listing) find(name string) (string, string, string, bool) {
	for _, session := range l.Sessions {
		if session.Name == name {
			return session.Telemetry.Activity, session.Telemetry.Selection, session.Telemetry.Thread, true
		}
	}
	return "", "", "", false
}
