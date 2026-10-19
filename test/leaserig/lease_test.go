package leaserig

import (
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
// launch, and the room's build stays.
func TestArtifactCopyCanJoinRoom(t *testing.T) {
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
