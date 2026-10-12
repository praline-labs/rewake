package leaserig

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Two builds of one version are two builds: while a session of one holds a
// room, the other's writers are refused and its launch is refused naming the
// holder, and the room's build stays.
func TestDifferentImagesWithSameVersionCannotShareRoom(t *testing.T) {
	if buildA.version != buildB.version || buildA.id == buildB.id {
		t.Fatalf("the two builds: %+v, %+v", buildA, buildB)
	}
	w := newWorld(t)
	session := w.heldAt(buildA, "default", "leased", launch("default")...)
	if got := w.record("default"); got != buildA.id {
		t.Fatalf("the launch wrote %q", got)
	}
	code, errOut := w.run(buildB, "default", "", send()...)
	if code != 1 || !strings.Contains(errOut, "belongs to build "+short(buildA.id)) || !strings.Contains(errOut, "this is build "+short(buildB.id)) {
		t.Fatalf("the other build's writer: %d %q", code, errOut)
	}
	code, errOut = w.run(buildB, "default", "", launch("default")...)
	if code != 1 || !strings.Contains(errOut, "is held by build "+short(buildA.id)) || !strings.Contains(errOut, "pid "+strconv.Itoa(session.pid())+" (rewake)") {
		t.Fatalf("the other build's launch: %d %q", code, errOut)
	}
	if got := w.record("default"); got != buildA.id {
		t.Fatalf("the refused launch left %q", got)
	}
}

// A byte-identical copy of the binary — another path, another inode — is the
// same build: it joins a room the original holds, as a writer and as a
// launch, and the room's build stays. (The tool's peer check takes a copy
// the same way: host/endpoint's TestPeerCheckIsTheBuildsContent.)
func TestArtifactCopyCanJoinRoomAndUseTools(t *testing.T) {
	w := newWorld(t)
	w.heldAt(buildA, "default", "leased", launch("default")...)
	copied := copyOf(t, buildA)
	w.heldAt(copied, "default", "leased", send()...)
	w.heldAt(copied, "default", "leased", launch("default")...)
	if got := w.record("default"); got != buildA.id {
		t.Fatalf("the copy changed the build to %q", got)
	}
}

// A command of the old build still running keeps a launch of the new one
// out, whichever build the room records, until it exits or is killed; then
// the new build's launch switches the room.
func TestOldCommandBlocksNewBuildUntilExitOrKill(t *testing.T) {
	for _, end := range []string{"exit", "kill"} {
		t.Run(end, func(t *testing.T) {
			w := newWorld(t)
			w.heldAt(buildA, "default", "leased", launch("default")...).kill()
			old := w.heldAt(buildA, "default", "leased", send()...)
			code, errOut := w.run(buildB, "default", "", launch("default")...)
			if code != 1 || !strings.Contains(errOut, "pid "+strconv.Itoa(old.pid())) {
				t.Fatalf("the new build's launch beside the old command: %d %q", code, errOut)
			}
			if got := w.record("default"); got != buildA.id {
				t.Fatalf("the refused launch left %q", got)
			}
			if end == "exit" {
				old.let()
			} else {
				old.kill()
			}
			w.heldAt(buildB, "default", "leased", launch("default")...)
			if got := w.record("default"); got != buildB.id {
				t.Fatalf("the launch after the old command's %s left %q", end, got)
			}
		})
	}
}

// A child an old session left behind — a hook, a shell command — that writes
// after the room switched to a new build is refused and writes nothing,
// while the new build holds the room and after it let go.
func TestOrphanChildRefusesAfterBuildSwitch(t *testing.T) {
	w := newWorld(t)
	w.heldAt(buildA, "default", "leased", launch("default")...).kill()
	session := w.heldAt(buildB, "default", "leased", launch("default")...)
	for _, phase := range []string{"while the new build holds the room", "after it let go"} {
		code, errOut := w.run(buildA, "default", "", send()...)
		if code != 1 || !strings.Contains(errOut, "belongs to build "+short(buildB.id)) {
			t.Fatalf("the orphan %s: %d %q", phase, code, errOut)
		}
		inbox, _ := filepath.Glob(filepath.Join(w.room("default"), "inbox", "*", "*.json"))
		if len(inbox) != 0 {
			t.Fatalf("the orphan %s wrote %v", phase, inbox)
		}
		session.kill()
	}
}

// A launch into an empty base ended or failed at any step of its bootstrap,
// its lease lock's creation or its build's publication leaves no session, and
// the next launch completes the room and holds it.
func TestBootstrapRecoversAfterEveryStep(t *testing.T) {
	steps := acquireSteps(t)
	for index, at := range steps {
		n := index + 1
		plans := []string{"other:crash=" + strconv.Itoa(n)}
		if !strings.HasPrefix(at, "step ") {
			plans = append(plans, "other:fail="+strconv.Itoa(n))
		}
		for _, plan := range plans {
			t.Run(strings.ReplaceAll(plan+" "+at, "/", "_"), func(t *testing.T) {
				w := newWorld(t)
				code, errOut := w.run(buildA, "default", plan, launch("default")...)
				if code == 0 {
					t.Fatalf("the faulted launch went on: %q", errOut)
				}
				sessions, _ := os.ReadDir(filepath.Join(w.room("default"), "sessions"))
				if len(sessions) != 0 {
					t.Fatalf("a faulted launch left a session: %v", sessions)
				}
				w.heldAt(buildA, "default", "leased", launch("default")...)
				if got := w.record("default"); got != buildA.id {
					t.Fatalf("the next launch left %q", got)
				}
				for _, level := range []string{"", "rooms", "rooms/default", "rooms/default/sessions", "rooms/default/inbox", "rooms/default/sock"} {
					info, err := os.Lstat(filepath.Join(w.base, level))
					if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
						t.Fatalf("level %q after recovery: %v, %v", level, info, err)
					}
				}
			})
		}
	}
}

// acquireSteps are the durable steps of a launch into an empty base up to
// the one after its lease, from a clean run's log: "<op> <path>" each.
func acquireSteps(t *testing.T) []string {
	t.Helper()
	w := newWorld(t)
	log := filepath.Join(t.TempDir(), "fault.log")
	p := w.start(buildA, "default", "other:log="+log+";other:holdat=leased@PAUSE", launch("default")...)
	if !p.waitHeld() {
		t.Fatalf("the clean launch ended: %s", p.stderr.String())
	}
	p.kill()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var steps []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.SplitN(line, " ", 3)
		if len(fields) != 3 || fields[1] == "read" {
			continue
		}
		steps = append(steps, fields[1]+" "+strings.TrimPrefix(fields[2], w.base))
		if fields[1] == "step" && fields[2] == "leased" {
			break
		}
	}
	var made, locked, published bool
	for _, s := range steps {
		made = made || strings.HasPrefix(s, "step bootstrap")
		locked = locked || strings.HasSuffix(s, "/build.lock")
		published = published || s == "write /rooms/default/build"
	}
	if !made || !locked || !published || steps[len(steps)-1] != "step leased" {
		t.Fatalf("the clean launch's steps are not an empty base's acquisition: %v", steps)
	}
	t.Logf("%d steps: %v", len(steps), steps)
	return steps
}
