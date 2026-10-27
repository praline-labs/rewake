package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPinnedBuild is a room that keeps its build across an upgrade
// (docs/rules/state.md#the-rooms-own-copy), on the fixture column: the case
// seen live was every room breaking at once when rewake was upgraded, its
// sessions' `rewake` now the new build and refused by the room.
//
//   - main and a worker are launched from an install of their own; the
//     install is then replaced by another build, as npm replaces it.
//   - The worker's `rewake send`, found on its PATH, still works.
//   - A person's `rewake list` of the new build is handed to the room's copy.
//   - Once the sessions end, a new launch takes the new build, and the
//     room's copy with it.
//
// What it does not prove: npm's own way of replacing the files, or a real
// harness's tools, which find the copy through the same environment.
func TestPinnedBuild(t *testing.T) {
	binary := enterScenario(t, "pinned-build")
	c := Start(t, Spec{
		Name:         "pinned-build",
		Harness:      fixtureColumn.harness,
		Observations: pinnedObservations,
		Deadline:     90 * time.Second,
	})
	for _, finding := range playPinned(t, c, Isolate(t, c, binary), binary) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsPinnedSend   = "after the install is replaced by another build, a worker's rewake send on its PATH still works"
	obsPinnedHanded = "a person's command of the new build is handed to the room's own copy"
	obsPinnedNext   = "once the sessions end, a new launch takes the new build and its copy"
	pinnedText      = "pinned-build: still here"
)

var pinnedObservations = []string{obsPinnedSend, obsPinnedHanded, obsPinnedNext}

func playPinned(t *testing.T, c *Case, iso *Isolation, built string) []telemetryFinding {
	t.Helper()
	col := fixtureColumn.harness
	original, err := os.ReadFile(built)
	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(iso.Home, "install", "rewake")
	if err := os.MkdirAll(filepath.Dir(installed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := replaceBinary(installed, original); err != nil {
		t.Fatal(err)
	}
	// The case's own install: the sessions start from it and the shim on
	// PATH runs it, and replacing it touches no other case.
	iso.binary = installed
	room := filepath.Join(iso.StateDir, "v2", "rooms", iso.HomeRoom())

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	unjudged := func(detail string, args ...any) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range pinnedObservations {
			out = append(out, telemetryFinding{observation: observation, detail: fmt.Sprintf(detail, args...)})
		}
		return out
	}

	asks := newRequests(iso, "worker")
	worker := startHarnessSession(t, c, iso, col, "worker", "--general", asks.env())
	lead := startHarnessSession(t, c, iso, col, "lead", "--main", shimInboxJSON+"=1")
	if err := sessionsUp(c, worker, lead); err != nil {
		stopSession(t, c, worker)
		stopSession(t, c, lead)
		return unjudged("%v", err)
	}
	// Another build: the same program with bytes of its own, so another id.
	newer := append(append([]byte{}, original...), []byte("\nanother build\n")...)
	if err := replaceBinary(installed, newer); err != nil {
		t.Fatal(err)
	}
	newID := idOf(newer)

	var found []telemetryFinding
	code, out, _ := asks.ask(c, "send", lead.name, pinnedText)
	found = append(found, finding(obsPinnedSend, code == 0 && recordOf(room) == idOf(original),
		"the worker's send exited %d (%s); the room's build is %.12s, the first install's %.12s", code, firstLine(out), recordOf(room), idOf(original)))

	// Its stderr kept aside: the line saying the room's copy answers is the
	// hand-off, which the listing alone would not show.
	said := filepath.Join(iso.Home, "list.stderr")
	list := iso.Command()
	list.Path, list.Args = "/bin/sh", []string{"/bin/sh", "-c", `"$0" list 2>"$1"`, installed, said}
	listed, err := iso.Output(list)
	stderr, _ := os.ReadFile(said)
	found = append(found, finding(obsPinnedHanded, err == nil && strings.Contains(string(listed), worker.name) && strings.Contains(string(stderr), "own copy answers"),
		"rewake list of the new build: %v, %q, stderr %q", err, firstLine(string(listed)), stderr))

	stopSession(t, c, worker)
	stopSession(t, c, lead)
	again := startHarnessSession(t, c, iso, col, "next", "--main")
	defer stopSession(t, c, again)
	if err := sessionsUp(c, again); err != nil {
		return append(found, finding(obsPinnedNext, false, "the launch of the new build: %v", err))
	}
	copied, _ := os.ReadFile(filepath.Join(room, "bin", "rewake"))
	found = append(found, finding(obsPinnedNext, recordOf(room) == newID && idOf(copied) == newID,
		"the room's build %.12s and its copy %.12s; the new install is %.12s", recordOf(room), idOf(copied), newID))
	return found
}

// replaceBinary puts content at path by rename, as an installer replaces a
// file a running process may execute.
func replaceBinary(path string, content []byte) error {
	temporary := path + ".new"
	if err := os.WriteFile(temporary, content, 0o755); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func idOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// recordOf is the room's build record, or what could not be read of it.
func recordOf(room string) string {
	data, err := os.ReadFile(filepath.Join(room, "build"))
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(data))
}
