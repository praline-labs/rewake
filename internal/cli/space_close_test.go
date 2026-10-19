package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// openSpace writes the record of an open space of the main run epoch.
func openSpace(t *testing.T, dir, name, epoch string) string {
	t.Helper()
	id, err := space.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := space.Create(dir, space.Record{
		ID: id, Name: name,
		Main:     space.Run{Name: "main-" + agentID, Run: epoch},
		Worker:   space.Worker{Harness: "claude", Name: name, Role: "write"},
		OpenedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := space.SetHost(dir, id, "6161.1750.boot"); err != nil {
		t.Fatal(err)
	}
	if _, err := space.Opened(dir, id, "5151.1800.boot"); err != nil {
		t.Fatal(err)
	}
	return id
}

// rewake space close is main's, for a space of its own run: from a worker,
// from a shell outside a session, for a name no open space holds, it is a
// wrong call; for a space another run of main opened it is refused; and none
// of them moves the record.
func TestSpaceCloseIsRefusedButForMainsOwnOpenSpace(t *testing.T) {
	liveSessionAs(t, "api-"+agentID, "write")
	t.Setenv(state.SessionEnv, "api-"+agentID)
	if code, _, errOut := run("space", "close", "api"); code != ExitUsage || !strings.Contains(errOut, "only main closes a space") {
		t.Fatalf("from a worker: %d %s", code, errOut)
	}
	t.Setenv(state.SessionEnv, "")
	if code, _, errOut := run("space", "close", "api"); code != ExitUsage || !strings.Contains(errOut, "this shell runs in none") {
		t.Fatalf("from a shell: %d %s", code, errOut)
	}

	dir := liveSessionAs(t, "main-"+agentID, "main")
	t.Setenv(state.SessionEnv, "main-"+agentID)
	other := openSpace(t, dir, "other", "100.1700.boot")
	if code, _, errOut := run("space", "close", "web"); code != ExitUsage || !strings.Contains(errOut, "no space named web is open; open spaces: other.") {
		t.Fatalf("no such space: %d %s", code, errOut)
	}
	if code, _, errOut := run("space", "close", "other"); code != ExitFailed || !strings.Contains(errOut, "belongs to another run of main") {
		t.Fatalf("another run's space: %d %s", code, errOut)
	}
	if code, _, errOut := run("space", "close"); code != ExitUsage || !strings.Contains(errOut, "space close takes the space's name") {
		t.Fatalf("no name: %d %s", code, errOut)
	}
	if record, err := space.Load(dir, other); err != nil || record.State != space.Open {
		t.Fatalf("a refused close moved the record: %+v %v", record, err)
	}
}

// A close of main's own open space records it closing with main's reason
// and answers once the space host recorded the end: the stand-in host here
// waits for the closing, as the real one's watch does, and closes the record
// with the signal it took.
func TestSpaceCloseAnswersOnceTheHostRecordedTheEnd(t *testing.T) {
	dir := liveSessionAs(t, "main-"+agentID, "main")
	t.Setenv(state.SessionEnv, "main-"+agentID)
	id := openSpace(t, dir, "api", os.Getenv(state.EpochEnv))

	hostSaw := make(chan string, 1)
	go func() {
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			record, err := space.Load(dir, id)
			if err != nil || record.State != space.Closing {
				continue
			}
			why := record.Closing + "; the worker's process group ended after SIGHUP"
			if _, err := space.Close(dir, id, space.End{At: time.Now().UTC(), Why: why, Worker: space.WorkerEnded, Checkout: space.Seen{Presence: space.Gone}}); err != nil {
				break
			}
			hostSaw <- record.Closing
			return
		}
		hostSaw <- ""
	}()

	code, out, errOut := run("space", "close", "api", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if saw := <-hostSaw; saw != closedByMain {
		t.Fatalf("the host saw the space closing for %q", saw)
	}
	var closed spaceClosed
	if err := json.Unmarshal([]byte(out), &closed); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if closed.Space != "api" || closed.ID != id || closed.Session != "api-claude" || closed.State != space.Closed ||
		closed.Why != closedByMain+"; the worker's process group ended after SIGHUP" || closed.Checkout != "" {
		t.Fatalf("the answer: %+v", closed)
	}
	if code, _, errOut := run("space", "close", "api"); code != ExitUsage || !strings.Contains(errOut, "this room has no open space") {
		t.Fatalf("a second close: %d %s", code, errOut)
	}
}
