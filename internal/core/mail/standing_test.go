package mail

import (
	"os"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/registry/registrytest"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// publishAPI publishes api as a run of this process, with the role given.
func publishAPI(t *testing.T, dir, role string) registry.Session {
	t.Helper()
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	session := registry.Session{Name: "api", Role: role, ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(t), CWD: dir, StartedAt: time.Now()}
	if err := registry.Publish(dir, session); err != nil {
		t.Fatal(err)
	}
	return session
}

// taskTo is a task of web's run sender-epoch for the given run of api, read by it.
func taskTo(t *testing.T, dir, run, text, addendumTo string) Message {
	t.Helper()
	time.Sleep(time.Microsecond)
	task := Message{ID: NewID(), From: "web", FromEpoch: "sender-epoch", To: "api", ToEpoch: run, Kind: Task, Text: text, AddendumTo: addendumTo, CreatedAt: time.Now()}
	if err := Put(dir, task); err != nil {
		t.Fatal(err)
	}
	readIn(t, dir, run, "", task)
	return task
}

func standingsOf(t *testing.T, dir string) map[string]Standing {
	t.Helper()
	seen := map[string]Standing{}
	for _, item := range must(Standings(dir, "web", "sender-epoch")) {
		seen[item.ID] = item
	}
	return seen
}

// A task is coming while its recipient's run lives, and is named as ended or
// replaced once the run is over, whatever its stage says.
func TestAStandingNamesWhyNoReportIsComing(t *testing.T) {
	dir := stateDir(t)
	session := publishAPI(t, dir, "write")
	live := taskTo(t, dir, session.Epoch(), "live", "")
	replaced := taskTo(t, dir, "an-older-run", "replaced", "")
	if got := standingsOf(t, dir); !got[live.ID].Coming() || got[live.ID].GoneReason() != "" ||
		got[replaced.ID].Coming() || got[replaced.ID].GoneReason() != GoneReplaced {
		t.Fatalf("live %+v, replaced %+v", got[live.ID], got[replaced.ID])
	}
	if err := os.Remove(state.SessionPath(dir, "api")); err != nil {
		t.Fatal(err)
	}
	if got := standingsOf(t, dir); got[live.ID].GoneReason() != GoneEnded {
		t.Fatalf("after the record is gone: %+v", got[live.ID])
	}
}

// The view only informs, so a record that cannot be read is the live run: it
// may still report, and "no report is coming" would have its task given again.
func TestAnUnreadableRecipientRecordIsTheLiveRun(t *testing.T) {
	dir := stateDir(t)
	session := publishAPI(t, dir, "write")
	task := taskTo(t, dir, session.Epoch(), "long job", "")
	if err := os.WriteFile(state.SessionPath(dir, "api"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := standingsOf(t, dir)[task.ID]; !got.Coming() || got.GoneReason() != "" {
		t.Fatalf("the task reads as %+v", got)
	}
	if item, settled, err := StandingOf(dir, "web", "sender-epoch", task); err != nil || settled || item.Gone() {
		t.Fatalf("the check reads %+v %v %v", item, settled, err)
	}
}

// A silent recipient answers nothing, so it is not waited on while it runs.
func TestASilentRecipientIsNotWaitedOn(t *testing.T) {
	dir := stateDir(t)
	session := publishAPI(t, dir, "main")
	task := taskTo(t, dir, session.Epoch(), "to a main", "")
	if _, listed := standingsOf(t, dir)[task.ID]; listed {
		t.Fatal("a task to a silent session is waited on")
	}
	if err := os.Remove(state.SessionPath(dir, "api")); err != nil {
		t.Fatal(err)
	}
	if _, listed := standingsOf(t, dir)[task.ID]; !listed {
		t.Fatal("a task to a session that ended is not named as lost")
	}
}

// An addendum is listed right after the task it adds to, however many tasks
// were sent between, and says which task it adds to.
func TestAnAddendumFollowsItsTask(t *testing.T) {
	dir := stateDir(t)
	session := publishAPI(t, dir, "write")
	first := taskTo(t, dir, session.Epoch(), "first", "")
	other := taskTo(t, dir, session.Epoch(), "other", "")
	addendum := taskTo(t, dir, session.Epoch(), "also", first.ID)
	var order []string
	for _, item := range must(Standings(dir, "web", "sender-epoch")) {
		order = append(order, item.ID)
		if item.ID == addendum.ID && (!item.Nested || item.Under != first.ID) {
			t.Errorf("the addendum reads as %+v", item)
		}
	}
	if len(order) != 3 || order[0] != first.ID || order[1] != addendum.ID || order[2] != other.ID {
		t.Fatalf("order %v, want %s %s %s", order, first.ID, addendum.ID, other.ID)
	}
}
