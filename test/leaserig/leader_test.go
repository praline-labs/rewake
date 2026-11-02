package leaserig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The switch of a running room (docs/rules/state.md#switching-a-running-room)
// led by rewake restart rewake, between real processes: the room runs build A
// from an install that an update then replaced with B, and the room's own
// copy hands the command to A, whose leader, a process of its own session,
// leads the room to B. No session of the room runs, so the leader has no
// participant to ask.

// installed is a room of build A, launched from an install whose rewake is
// then replaced by B, as an update does: the image the leader is called by.
func installed(t *testing.T, w *world) image {
	t.Helper()
	install := copyOf(t, buildA)
	// The launch never gets to a harness; its lease has made the room A's
	// and placed the room's copy by then.
	if _, errOut := w.run(install, "default", "", launch("default")...); w.copyID() != buildA.id {
		t.Fatalf("the room's copy after the launch of A is %s: %s", w.copyID(), errOut)
	}
	data, err := os.ReadFile(buildB.path)
	if err != nil {
		t.Fatal(err)
	}
	temp := install.path + ".new"
	if err := os.WriteFile(temp, data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temp, install.path); err != nil {
		t.Fatal(err)
	}
	return image{path: install.path, id: buildB.id, version: buildB.version}
}

// copyID is the build of the room's own copy, or "missing".
func (w *world) copyID() string {
	w.t.Helper()
	data, err := os.ReadFile(filepath.Join(w.room("default"), "bin", "rewake"))
	if errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	if err != nil {
		w.t.Fatal(err)
	}
	for _, of := range []image{buildA, buildB, buildC} {
		known, _ := os.ReadFile(of.path)
		if string(known) == string(data) {
			return of.id
		}
	}
	return "another"
}

// phase is the phase of the room's switch mark, or "none".
func (w *world) phase() string {
	w.t.Helper()
	data, err := os.ReadFile(filepath.Join(w.room("default"), "switch"))
	if errors.Is(err, os.ErrNotExist) {
		return "none"
	}
	var mark struct{ Phase string }
	if err != nil || json.Unmarshal(data, &mark) != nil {
		w.t.Fatalf("the switch mark: %q, %v", data, err)
	}
	return mark.Phase
}

func restart() []string { return []string{"restart", "rewake"} }

// With nothing in the room to ask, the switch goes through: the build, the
// room's copy, and no mark of it left once a writer passes.
func TestARoomWithNothingToAskSwitches(t *testing.T) {
	w := newWorld(t)
	install := installed(t, w)
	code, errOut := w.run(install, "default", "", restart()...)
	if code != 0 {
		t.Fatalf("restart rewake: %d %s", code, errOut)
	}
	if got, copied := w.record("default"), w.copyID(); got != buildB.id || copied != buildB.id {
		t.Fatalf("after the switch the build is %s and the copy %s", short(got), copied)
	}
}

// A leader killed at any step of its switch leaves one build in the room, and
// a mark that holds the room as long as something may be parked: none to
// the commit's build write, then the target, with the room's copy put in line
// by the next launch of it when the leader died before the link.
func TestALeaderKilledAtEachStepLeavesOneBuild(t *testing.T) {
	for _, c := range []struct {
		step, build, copied, phase string
	}{
		{"switch claimed", buildA.id, buildA.id, "asking"},
		{"switch dropping", buildA.id, buildA.id, "dropping"},
		{"switch exclusive", buildA.id, buildA.id, "dropping"},
		{"switch marked", buildA.id, buildA.id, "committed"},
		{"switch build", buildB.id, buildA.id, "committed"},
		{"switch linked", buildB.id, buildB.id, "committed"},
	} {
		t.Run(c.step, func(t *testing.T) {
			w := newWorld(t)
			install := installed(t, w)
			w.heldAt(install, "default", c.step, restart()...).killAll()
			if got, copied, phase := w.record("default"), w.copyID(), w.phase(); got != c.build || copied != c.copied || phase != c.phase {
				t.Fatalf("killed at %s: build %s, copy %s, mark %s", c.step, short(got), copied, phase)
			}
			switch c.phase {
			case "asking":
				// Its leader gone, the mark holds nothing: the next writer
				// removes it.
				w.run(buildA, "default", "", send()...)
				if phase := w.phase(); phase != "none" {
					t.Fatalf("a stale mark stays %s", phase)
				}
			case "committed":
				// A launch of a third build is refused while the mark lives.
				code, errOut := w.run(buildC, "default", "", launch("default")...)
				if code != 1 || !strings.Contains(errOut, "is switching to build "+short(buildB.id)) {
					t.Fatalf("a foreign launch under the committed mark: %d %s", code, errOut)
				}
				if c.build == buildB.id {
					// The next of the room's launches is B's, and puts the copy
					// in line with the build.
					if _, errOut := w.run(install, "default", "", launch("default")...); w.copyID() != buildB.id {
						t.Fatalf("the copy after the room's launch is %s: %s", w.copyID(), errOut)
					}
				}
			}
			if got := w.record("default"); got != c.build {
				t.Fatalf("the build moved to %s", short(got))
			}
		})
	}
}

// While a switch drops the room's holders, a launch of another build waits
// and is refused, and the switch commits the build it was led to.
func TestAForeignLaunchDuringParkingCannotOverturnTheSwitch(t *testing.T) {
	w := newWorld(t)
	install := installed(t, w)
	leader := w.heldAt(install, "default", "switch dropping", restart()...)
	code, errOut := w.run(buildC, "default", "", launch("default")...)
	if code != 1 || !strings.Contains(errOut, "has not finished within") {
		t.Fatalf("a foreign launch while the switch drops: %d %s", code, errOut)
	}
	leader.let()
	if leader.code() != 0 {
		t.Fatalf("the leader after the foreign launch: %d %s", leader.code(), leader.stderr.String())
	}
	if got, copied := w.record("default"), w.copyID(); got != buildB.id || copied != buildB.id {
		t.Fatalf("after the switch the build is %s and the copy %s", short(got), copied)
	}
}
