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

// codexSession launches a real rewake session whose harness is the shim, and
// gives a scenario what it needs to observe rewake's own state.
type codexSession struct {
	name     string
	state    string // where the session writes what `rewake list` told it
	accepted string // where it writes the conversation id the server gave it
	mailbox  string // where it writes what its own `rewake inbox` returned
	exitFile string // the file that asks this session to stop
	turns    string // where it records the turns it accepted
	// delivered is where it records the id of every message a delivery named.
	delivered string
	// groups is where it records each delivery as one group, and reads is
	// where it records each inbox call made under shimReadEach.
	groups  string
	reads   string
	home    string
	process *owned
}

// mailboxRead is what this session's own `rewake inbox` returned, as the
// session wrote it. A scenario reads this file rather than the mailbox itself:
// the point is what the session saw, not what the test can see.
func (s *codexSession) mailboxRead() string {
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
func (s *codexSession) deliveredIDs() []string {
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
func (s *codexSession) alive() bool {
	return !s.process.finished() && s.process.groupAlive()
}

// acceptedTurns is what this session recorded about the turns it accepted.
func (s *codexSession) acceptedTurns() string {
	raw, err := os.ReadFile(s.turns)
	if err != nil {
		return ""
	}
	return string(raw)
}

// acceptedThread is the conversation the server named, as the session's own
// client learnt it from its reply. A scenario compares telemetry against this
// rather than against an id the test picked, because for a new conversation
// the client does not choose one.
func (s *codexSession) acceptedThread() string {
	raw, err := os.ReadFile(s.accepted)
	if err != nil {
		return ""
	}
	return string(raw)
}

// startCodexSession puts the shim on the case's PATH as `codex`, makes the
// built rewake reachable to the session (it has to run `rewake list` itself),
// and launches the wrapper as main so the session may see its own telemetry.
func startCodexSession(t *testing.T, c *Case, iso *Isolation, name, role string, controls ...string) *codexSession {
	t.Helper()
	installShim(t, c, iso)
	// rewake appends the harness to the requested name.
	session := &codexSession{
		name:     name + "-codex",
		state:    filepath.Join(iso.Home, name+".state.json"),
		accepted: filepath.Join(iso.Home, name+".accepted"),
		// Per session: a shared stop file would end every session at once.
		exitFile:  filepath.Join(iso.Home, name+".exit"),
		turns:     filepath.Join(iso.Home, name+".turns"),
		delivered: filepath.Join(iso.Home, name+".delivered"),
		groups:    filepath.Join(iso.Home, name+".groups"),
		reads:     filepath.Join(iso.Home, name+".reads"),
	}

	// Only one session in a room may be main, and only a main sees telemetry.
	// A scenario that needs two sessions gives that role to the one whose
	// selection it has to observe.
	launch := iso.Command("--name", name, role, "codex")
	launch.Env = append(iso.Env(),
		shimEnv+"=1",
		"RW_SHIM_TEST_EXE="+testExecutable(t),
		shimStateFile+"="+session.state,
		shimAcceptedFile+"="+session.accepted,
	)
	launch.Env = append(launch.Env, controls...)
	// The mailbox the session reads with its own rewake, and what the scenario
	// reads back afterwards.
	session.mailbox = filepath.Join(iso.Home, name+".mailbox")
	launch.Env = append(launch.Env, shimMailboxFile+"="+session.mailbox, shimExitFile+"="+session.exitFile,
		shimTurnsFile+"="+session.turns, shimDeliveredFile+"="+session.delivered,
		shimGroupsFile+"="+session.groups, shimReadsFile+"="+session.reads)
	// Not marked as an expected failure: the shim is *asked* to stop and
	// exits cleanly, so anything else is a real failure of the session and has
	// to reach the verdict. The earlier blanket "failure expected" here hid an
	// exit code of 42 completely.
	process, err := c.start(launch, false)
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
func (s *codexSession) stop(c *Case) error {
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

// installShim writes the two programs the session needs on its PATH: `codex`,
// which re-executes this test binary as the harness, and `rewake`, which is
// the binary built for this run. The isolation deliberately keeps an
// installed rewake off PATH; this puts *ours* there, which is the one the
// scenario means to measure.
func installShim(t *testing.T, c *Case, iso *Isolation) {
	t.Helper()
	script := "#!/bin/sh\nexec \"$RW_SHIM_TEST_EXE\" -test.run=TestCodexShimHelper -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(iso.ShimDir, "codex"), []byte(script), 0o700); err != nil {
		t.Fatalf("installing the codex shim: %v", err)
	}
	link := "#!/bin/sh\nexec " + iso.binary + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(iso.ShimDir, "rewake"), []byte(link), 0o700); err != nil {
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

// TestCodexShimHelper is the process the `codex` script re-executes. It does
// nothing unless it was started as the shim.
func TestCodexShimHelper(_ *testing.T) {}

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
func (s *codexSession) await(c *Case, what string, ready func(listing) bool) (listing, bool) {
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

// latest is what the session last reported, without waiting.
func (s *codexSession) latest() listing {
	var current listing
	raw, err := os.ReadFile(s.state)
	if err == nil {
		_ = json.Unmarshal(raw, &current)
	}
	return current
}

// find returns the session's own row. rewake names a session after the
// harness it launched — `worker` becomes `worker-codex` — so the scenario
// matches on the name rewake gave it, not on the one it asked for.
func (l listing) find(name string) (string, string, string, bool) {
	for _, session := range l.Sessions {
		if session.Name == name {
			return session.Telemetry.Activity, session.Telemetry.Selection, session.Telemetry.Thread, true
		}
	}
	return "", "", "", false
}
