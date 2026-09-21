package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Isolation is the private world one case runs in. Establishing it can fail,
// and when it does the case stops: falling back to the developer's real
// environment would run the case against the owner's live sessions and call
// whatever came out a result.
//
// The boundaries here are the ones the free tiers need — a private HOME, state
// directory and Codex home, and a PATH that leads to the shims rather than to
// anything installed. Network, mount and PID namespaces stay mandatory for the
// paid tier only; the reasons are in docs/check-runner-proposal.md.
type Isolation struct {
	// Home is the private HOME for anything the case launches.
	Home string
	// StateDir is REWAKE_DIR: the mailbox, registry and locks of this case.
	StateDir string
	// CodexHome is the private CODEX_HOME, kept separate from Home so a
	// scenario can point one harness elsewhere without moving the other.
	//
	// It is the one harness-specific detail in the common layer, and it is
	// here because isolation needs it before any harness code exists. It moves
	// into the Codex fixture when the fixtures appear.
	CodexHome string
	// ShimDir is first on PATH. It is empty in stage 0; the harness shims that
	// will live here arrive with the first harness-specific scenario.
	ShimDir string

	binary  string
	forCase *Case
}

// Isolate builds the private world for a case, or stops the case trying.
func Isolate(t *testing.T, c *Case, binary string) *Isolation {
	t.Helper()
	base, err := os.MkdirTemp("", "rewake-case-")
	if err != nil {
		t.Fatalf("workflow: isolation failed, cannot create the case directory: %v", err)
	}
	// Registered before anything else can fail: every Fatalf below would
	// otherwise leave this directory in /tmp for good.
	c.RemoveOnFinish(base)
	iso := &Isolation{
		Home:      filepath.Join(base, "home"),
		StateDir:  filepath.Join(base, "state"),
		CodexHome: filepath.Join(base, "codex"),
		ShimDir:   filepath.Join(base, "shim"),
		binary:    binary,
	}
	for _, dir := range []string{iso.Home, iso.StateDir, iso.CodexHome, iso.ShimDir} {
		// 0o700 rather than the umask default: the state directory refuses to
		// be group- or world-readable, and a case that cannot write its own
		// receipts is not a finding about rewake.
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("workflow: isolation failed, cannot create %s: %v", dir, err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Fatalf("workflow: isolation failed, cannot restrict %s: %v", dir, err)
		}
	}
	if binary == "" {
		t.Fatal("workflow: isolation failed, no binary was built for this run")
	}
	if err := iso.verify(); err != nil {
		t.Fatalf("workflow: isolation failed: %v", err)
	}
	iso.registerCleanupChecks(c)
	iso.forCase = c
	return iso
}

// verify checks the things that would quietly spoil a case, rather than
// restating what the constructor just did: that the directories exist and are
// usable, that nothing identifying the session running the test survives into
// the child, and that no rewake is reachable through the case's PATH.
func (iso *Isolation) verify() error {
	for name, dir := range map[string]string{
		"HOME": iso.Home, "REWAKE_DIR": iso.StateDir, "CODEX_HOME": iso.CodexHome,
	} {
		if !filepath.IsAbs(dir) {
			return fmt.Errorf("%s is not absolute: %s", name, dir)
		}
		info, err := os.Stat(dir)
		if err != nil {
			return fmt.Errorf("%s is unusable: %w", name, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory: %s", name, dir)
		}
	}
	// Not a re-read of what Env just built — that would only restate the line
	// above. What matters is that nothing identifying this session survives
	// into the child: inheriting one makes the case believe it is the session
	// running it, and everything it then observes is about the wrong session.
	for _, leaking := range []string{"REWAKE_SESSION=", "REWAKE_EPOCH=", "REWAKE_ROOM="} {
		for _, entry := range iso.Env() {
			if strings.HasPrefix(entry, leaking) {
				return fmt.Errorf("the case environment carries %s, which belongs to the session running the test", entry)
			}
		}
	}
	found, ok, err := iso.Lookup("rewake")
	if err != nil {
		return err
	}
	if ok {
		return fmt.Errorf("a rewake is reachable through the case PATH at %s, so a scenario could measure the wrong binary", found)
	}
	return nil
}

// registerCleanupChecks states what must be true once the case is over. They
// run after the case body and before its result is classified, so a run that
// leaves a live socket or a session record behind fails rather than passes.
//
// They look at the two specific places where a leftover means an unfinished
// process, not at a suffix across the whole tree. Delivered mail is also
// <id>.json and stays in the mailbox on purpose — a durable mailbox keeping
// what it was given is its correct final state, not litter — so a suffix rule
// would fail the first real scenario for no defect, and get weakened. A
// weakened cleanup check catches nothing.
func (iso *Isolation) registerCleanupChecks(c *Case) {
	c.CheckCleanup("no live socket left behind", iso.noLiveSockets)
	c.CheckCleanup("no unpublished outcome left behind", iso.noPendingOutcomes)
	c.CheckCleanup("no session record left behind", func() error {
		// Everything except the lock. The per-name lock file is part of the
		// registry's machinery and outlives any single session by design, so
		// requiring the directory to be empty would fail every scenario that
		// ever launched one. Excluding it is not the same as accepting only
		// `.json`: an atomic write leaves a temporary file beside the record
		// if the process dies mid-write, and a half-written registry entry is
		// exactly the leftover of an unfinished session this must catch.
		found, err := iso.inEveryRoom("sessions", func(name string) bool {
			return !strings.HasSuffix(name, ".lock")
		})
		if err != nil {
			return err
		}
		if len(found) > 0 {
			return fmt.Errorf("still present: %s", strings.Join(found, ", "))
		}
		return nil
	})
}

// noLiveSockets looks for sockets by file type. The server keeps ordinary
// artifacts beside its socket — `<name>.sock.up.log`, `<name>.sock.gateway.log`
// and `<name>.sock.outcomes.json` — and Close removes the sockets, not those.
// Forbidding every file in the directory would fail the first working Codex
// scenario over its normal leftovers, and the check would then be weakened
// until it caught nothing.
//
// Matching on the name instead would be the same mistake one level down: the
// wrapper's upstream socket is `<name>.sock.up`, which a `.sock` suffix misses
// entirely. A socket is a socket because of what it is, not what it is called.
func (iso *Isolation) noLiveSockets() error {
	found, err := iso.inEveryRoomByType("sock", func(info os.FileInfo) bool {
		return info.Mode()&os.ModeSocket != 0
	})
	if err != nil {
		return err
	}
	if len(found) > 0 {
		return fmt.Errorf("still present: %s", strings.Join(found, ", "))
	}
	return nil
}

// noPendingOutcomes reads the completion journal rather than banning it. An
// empty journal is how a session that published everything leaves it; entries
// still in it mean results the session never managed to publish, which is a
// case that did not finish.
func (iso *Isolation) noPendingOutcomes() error {
	journals, err := iso.inEveryRoom("sock", func(name string) bool {
		return strings.HasSuffix(name, ".outcomes.json")
	})
	if err != nil {
		return err
	}
	for _, journal := range journals {
		raw, err := os.ReadFile(journal)
		if err != nil {
			return fmt.Errorf("reading %s: %w", journal, err)
		}
		var pending []json.RawMessage
		if err := json.Unmarshal(raw, &pending); err != nil {
			return fmt.Errorf("unreadable journal %s: %w", journal, err)
		}
		if len(pending) > 0 {
			return fmt.Errorf("%d outcome(s) never published, journaled in %s", len(pending), journal)
		}
	}
	return nil
}

// inEveryRoomByType collects the entries of <state>/rooms/*/<sub> whose file
// type matches.
func (iso *Isolation) inEveryRoomByType(sub string, matches func(os.FileInfo) bool) ([]string, error) {
	return iso.collect(sub, func(entry os.DirEntry) (bool, error) {
		info, err := entry.Info()
		if err != nil {
			return false, err
		}
		return matches(info), nil
	})
}

// inEveryRoom collects the entries of <state>/rooms/*/<sub> whose name matches.
func (iso *Isolation) inEveryRoom(sub string, matches func(string) bool) ([]string, error) {
	return iso.collect(sub, func(entry os.DirEntry) (bool, error) {
		return matches(entry.Name()), nil
	})
}

func (iso *Isolation) collect(sub string, matches func(os.DirEntry) (bool, error)) ([]string, error) {
	rooms, err := os.ReadDir(filepath.Join(iso.StateDir, "rooms"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var found []string
	for _, room := range rooms {
		if !room.IsDir() {
			continue
		}
		dir := filepath.Join(iso.StateDir, "rooms", room.Name(), sub)
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			ok, err := matches(entry)
			if err != nil {
				return nil, err
			}
			if ok {
				found = append(found, filepath.Join(dir, entry.Name()))
			}
		}
	}
	return found, nil
}

// Env is the environment for anything this case launches. It is built from
// nothing rather than filtered from the developer's, so a variable nobody
// thought about cannot leak in and change what is being measured.
func (iso *Isolation) Env() []string {
	return []string{
		"PATH=" + iso.ShimDir + string(os.PathListSeparator) + "/usr/bin" + string(os.PathListSeparator) + "/bin",
		"HOME=" + iso.Home,
		"REWAKE_DIR=" + iso.StateDir,
		"CODEX_HOME=" + iso.CodexHome,
		// Deliberately absent: REWAKE_SESSION, REWAKE_EPOCH and REWAKE_ROOM.
		// Inheriting them makes the case believe it is the session running it.
	}
}

// Lookup resolves an executable against the case's PATH rather than the test
// process's. A scenario uses it to observe that something is absent — the
// absence of a stray harness or rewake is part of the isolation, so it is
// checked rather than assumed.
func (iso *Isolation) Lookup(name string) (string, bool, error) {
	for _, dir := range filepath.SplitList(iso.pathValue()) {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			// Not "it is not there": we could not tell. A scenario asking
			// this question is usually asking it to prove an absence, and an
			// unreadable answer must not read as one.
			return "", false, fmt.Errorf("looking for %s in %s: %w", name, dir, err)
		}
		if info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}
		return candidate, true, nil
	}
	return "", false, nil
}

func (iso *Isolation) pathValue() string {
	for _, entry := range iso.Env() {
		if value, ok := strings.CutPrefix(entry, "PATH="); ok {
			return value
		}
	}
	return ""
}

// Command prepares the built rewake with this case's environment. Scenarios
// run rewake only through here, which keeps a stray binary from PATH out of
// the results. Run it with Case.Output: the case owns the process group, ends
// it on its deadline and joins it before classifying.
func (iso *Isolation) Command(args ...string) *exec.Cmd {
	command := exec.Command(iso.binary, args...)
	command.Env = iso.Env()
	command.Dir = iso.Home
	return command
}

// Output runs a prepared command under the case that owns this isolation.
func (iso *Isolation) Output(command *exec.Cmd) ([]byte, error) {
	return iso.forCase.Output(command)
}
