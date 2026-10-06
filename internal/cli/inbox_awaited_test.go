package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/harness/claude/telemetry"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// awaitedRoom is a main session, lead, and what it can find in its workers'
// mailboxes. Every message is written where rewake itself would leave it, and
// a read is a real read by the worker's run.
type awaitedRoom struct {
	t    *testing.T
	dir  string
	lead registry.Session
}

func newAwaitedRoom(t *testing.T) *awaitedRoom {
	t.Helper()
	dir := liveSession(t, "lead")
	markMain(t, dir, "lead")
	lead, _ := registry.Lookup(dir, "lead")
	room := &awaitedRoom{t: t, dir: dir, lead: lead}
	room.asLead()
	return room
}

func (r *awaitedRoom) asLead() {
	r.t.Setenv(state.SessionEnv, r.lead.Name)
	r.t.Setenv(epochEnv, r.lead.Epoch())
}

// put writes a message into a worker's mailbox: "inbox" where send leaves it,
// "unread" where an announced one waits, "done" where a read or refused one
// goes. A status is written beside it when one is given.
func (r *awaitedRoom) put(to registry.Session, where string, status inbox.State, detail string, fields map[string]any) string {
	r.t.Helper()
	time.Sleep(time.Microsecond)
	id := fmt.Sprintf("%019d-000000000000", time.Now().UnixNano())
	base := map[string]any{"id": id, "from": r.lead.Name, "fromEpoch": r.lead.Epoch(), "to": to.Name, "toEpoch": to.Epoch(), "kind": "task", "createdAt": time.Now()}
	for key, value := range fields {
		base[key] = value
	}
	directory := map[string]string{"inbox": state.InboxPath(r.dir, to.Name), "unread": state.UnreadPath(r.dir, to.Name), "done": state.DonePath(r.dir, to.Name)}[where]
	if err := os.MkdirAll(directory, 0o700); err != nil {
		r.t.Fatal(err)
	}
	encoded, _ := json.Marshal(base)
	if err := os.WriteFile(filepath.Join(directory, id+".json"), encoded, 0o600); err != nil {
		r.t.Fatal(err)
	}
	if status != "" {
		encoded, _ := json.Marshal(inbox.Status{State: status, Detail: detail, At: time.Now()})
		if err := os.WriteFile(filepath.Join(state.InboxPath(r.dir, to.Name), id+".status"), encoded, 0o600); err != nil {
			r.t.Fatal(err)
		}
	}
	return id
}

// read has the worker's run read everything announced to it, as rewake inbox
// run in that session would.
func (r *awaitedRoom) read(worker registry.Session) {
	r.t.Helper()
	r.t.Setenv(state.SessionEnv, worker.Name)
	r.t.Setenv(epochEnv, worker.Epoch())
	defer r.asLead()
	if code, _, errOut := run("inbox"); code != ExitOK {
		r.t.Fatalf("inbox as %s: %d %s", worker.Name, code, errOut)
	}
}

// sentAndRead is a task from lead that the worker has read.
func (r *awaitedRoom) sentAndRead(worker registry.Session, text string) string {
	r.t.Helper()
	id := r.put(worker, "unread", inbox.Delivered, "", map[string]any{"text": text})
	r.read(worker)
	return id
}

func (r *awaitedRoom) turnEnds(worker registry.Session, result inbox.TurnEnd) {
	r.t.Helper()
	if err := completeTurn(r.dir, worker, result, ""); err != nil {
		r.t.Fatal(err)
	}
}

func awaitedJSON(t *testing.T) awaitedModel {
	t.Helper()
	code, out, errOut := run("inbox", "--awaited", "--json")
	if code != ExitOK {
		t.Fatalf("inbox --awaited: %d %s %s", code, out, errOut)
	}
	var model awaitedModel
	if err := json.Unmarshal([]byte(out), &model); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return model
}

// byID flattens the model to what each message's state reads as.
func byID(model awaitedModel) map[string]awaitedView {
	views := map[string]awaitedView{}
	for _, recipient := range model.Recipients {
		for _, message := range recipient.Messages {
			views[message.ID] = message
		}
	}
	return views
}

// Every stage a task can stand at shows as that stage, and what owes nothing —
// a note, a task from a plain shell, one from lead's earlier run, one already
// reported on — is not listed at all.
func TestAwaitedShowsWhereEachTaskStands(t *testing.T) {
	room := newAwaitedRoom(t)
	working := otherRun(t, room.dir, "working")
	paused := otherRun(t, room.dir, "paused")
	stopped := otherRun(t, room.dir, "halted")
	aborted := otherRun(t, room.dir, "aborted")
	done := otherRun(t, room.dir, "done")
	refused := otherRun(t, room.dir, "refused")

	// Read before this run's task: a wait recorded for another run of the same
	// sender replaces the one before it.
	room.put(working, "unread", inbox.Delivered, "", map[string]any{"fromEpoch": "an-earlier-run", "text": "from before"})
	room.read(working)
	owed := room.sentAndRead(working, "fix the parser\nand the tests")
	room.put(working, "unread", inbox.Delivered, "", map[string]any{"kind": "notify", "text": "heads-up"})
	room.put(working, "unread", inbox.Delivered, "", map[string]any{"from": "shell", "fromEpoch": "", "text": "from a shell"})
	room.read(working)
	unread := room.put(working, "unread", inbox.Delivered, "", map[string]any{"kind": "question", "text": "which port?"})
	held := room.put(working, "inbox", inbox.Held, "waiting for the person to approve", map[string]any{"text": "held one"})
	undelivered := room.put(working, "inbox", inbox.Pending, "a compaction is running", map[string]any{"text": "just sent"})

	pending := room.sentAndRead(paused, "run the suite")
	if err := markPending(room.dir, paused.Name, paused.Epoch(), "the suite is running", markAt); err != nil {
		t.Fatal(err)
	}
	room.turnEnds(paused, inbox.TurnEnd{Boundary: boundaryNow(t, room.dir, paused), ID: "p/1", Text: "started the suite", Started: markAt - 1, Ended: boottime.Now()})

	halted := room.sentAndRead(stopped, "refactor")
	room.turnEnds(stopped, inbox.TurnEnd{Boundary: boundaryNow(t, room.dir, stopped), ID: "s/1", Text: telemetry.StoppedText, Stopped: true})
	// A main's rewake interrupt is not a person's Esc, and the listing says
	// which it was.
	interrupted := room.sentAndRead(aborted, "rename")
	room.turnEnds(aborted, inbox.TurnEnd{Boundary: boundaryNow(t, room.dir, aborted), ID: "a/1", Text: telemetry.InterruptedText("lead"), Stopped: true})

	settled := room.sentAndRead(done, "small fix")
	room.turnEnds(done, inbox.TurnEnd{Boundary: boundaryNow(t, room.dir, done), ID: "d/1", Text: "fixed"})

	failed := room.put(refused, "done", inbox.Failed, "the harness refused the notice", map[string]any{"text": "never landed"})

	views := byID(awaitedJSON(t))
	want := map[string]inbox.Stage{
		owed: inbox.StageOwed, unread: inbox.StageUnread, held: inbox.StageHeld, undelivered: inbox.StageUndelivered,
		pending: inbox.StagePending, halted: inbox.StageStopped, interrupted: inbox.StageStopped, failed: inbox.StageFailed,
	}
	for id, stage := range want {
		if views[id].State != stage || views[id].Gone != "" {
			t.Errorf("%s: %+v, want %s", id, views[id], stage)
		}
	}
	if len(views) != len(want) {
		t.Errorf("listed %d messages, want %d: %+v", len(views), len(want), views)
	}
	if _, listed := views[settled]; listed {
		t.Errorf("a task reported on is still listed")
	}
	if views[owed].Text != "fix the parser\nand the tests" || views[held].Detail != "waiting for the person to approve" || views[pending].Detail != "the suite is running\n\nstarted the suite" || views[unread].Kind != inbox.Question {
		t.Errorf("the machine form lost the text or the detail: %+v", views)
	}

	code, out, _ := run("inbox", "--awaited")
	if code != ExitOK || !strings.HasPrefix(out, "Rewake: waiting on 7 reports; 1 more will not come:\n\nto working\n") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for _, line := range []string{
		owed + " · task · ", " · read, being worked on\nfix the parser\n",
		unread + " · question · ", " · delivered, unread\nwhich port?\n",
		" · held: waiting for the person to approve\nheld one\n",
		" · not delivered yet: a compaction is running\njust sent\n",
		" · pending: the suite is running\nrun the suite\n",
		" · stopped: the person at the keyboard stopped this turn\nrefactor\n",
		" · stopped: lead interrupted this turn with rewake interrupt\nrename\n",
		" · not delivered, no report coming: the harness refused the notice\nnever landed\n",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("the listing lacks %q:\n%s", line, out)
		}
	}
	if strings.Contains(out, "and the tests") {
		t.Errorf("the text form printed more than the first line:\n%s", out)
	}
}

// A recipient whose run ended, or was replaced by a new one, owes lead nothing
// any more: its tasks are named as having no report coming, not as owed.
func TestAwaitedNamesARecipientThatIsGone(t *testing.T) {
	room := newAwaitedRoom(t)
	ended := otherRun(t, room.dir, "ended")
	replaced := otherRun(t, room.dir, "replaced")
	lost := room.sentAndRead(ended, "long job")
	unreadLost := room.put(ended, "unread", inbox.Delivered, "", map[string]any{"text": "never read"})
	swept := room.sentAndRead(replaced, "another job")
	if err := os.Remove(state.SessionPath(room.dir, ended.Name)); err != nil {
		t.Fatal(err)
	}
	next := otherRun(t, room.dir, "replaced")
	// The new run sweeps the old run's wait records, as its wrapper does.
	if err := os.RemoveAll(filepath.Join(state.AwaitingPath(room.dir, replaced.Name), replaced.Epoch())); err != nil {
		t.Fatal(err)
	}

	views := byID(awaitedJSON(t))
	for id, gone := range map[string]string{lost: "ended", unreadLost: "ended", swept: "replaced"} {
		if views[id].Gone != gone {
			t.Errorf("%s: %+v, want gone %s", id, views[id], gone)
		}
	}
	code, out, _ := run("inbox", "--awaited")
	if code != ExitOK || !strings.HasPrefix(out, "Rewake: waiting on no reports; 3 will not come:") ||
		!strings.Contains(out, " · no report coming: ended ended\nlong job\n") ||
		!strings.Contains(out, " · no report coming: replaced was replaced by a new run\nanother job\n") {
		t.Fatalf("exit %d:\n%s", code, out)
	}

	// The new run's own work is owed as usual.
	fresh := room.sentAndRead(next, "fresh job")
	if view := byID(awaitedJSON(t))[fresh]; view.State != inbox.StageOwed || view.Gone != "" {
		t.Fatalf("the new run's task: %+v", view)
	}
}

// Nothing sent, or everything answered, is one short line.
func TestAwaitedIsOneLineWhenNothingIsOwed(t *testing.T) {
	room := newAwaitedRoom(t)
	worker := otherRun(t, room.dir, "worker")
	room.sentAndRead(worker, "a task")
	room.turnEnds(worker, inbox.TurnEnd{Boundary: boundaryNow(t, room.dir, worker), ID: "w/1", Text: "done"})
	code, out, _ := run("inbox", "--awaited")
	if code != ExitOK || out != "Rewake: nobody owes you a report.\n" {
		t.Fatalf("exit %d: %q", code, out)
	}
	if model := awaitedJSON(t); len(model.Recipients) != 0 || model.Session != "lead" {
		t.Fatalf("got %+v", model)
	}
}

// The view only reads: every file in the state directory is the same, byte for
// byte and in its times, before and after, and no lock file appears.
func TestAwaitedChangesNothingOnDisk(t *testing.T) {
	room := newAwaitedRoom(t)
	worker := otherRun(t, room.dir, "worker")
	gone := otherRun(t, room.dir, "gone")
	room.sentAndRead(worker, "a task")
	room.put(worker, "inbox", "", "", map[string]any{"text": "waiting"})
	room.sentAndRead(gone, "orphaned")
	if err := os.Remove(state.SessionPath(room.dir, gone.Name)); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, room.dir)
	for _, args := range [][]string{{"inbox", "--awaited"}, {"inbox", "--awaited", "--json"}} {
		if code, _, errOut := run(args...); code != ExitOK {
			t.Fatalf("%v: %s", args, errOut)
		}
	}
	if after := snapshotTree(t, room.dir); after != before {
		t.Fatalf("the state directory changed:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// --awaited is used alone and takes no value; outside a session there is no
// run to have sent anything.
func TestAwaitedRefusesAWrongCall(t *testing.T) {
	newAwaitedRoom(t)
	for _, args := range [][]string{
		{"inbox", "--awaited", "--owed"},
		{"inbox", "--awaited", "--peek"},
		{"inbox", "--awaited", "--message=1780000000000000000-012345abcdef"},
		{"inbox", "--awaited=yes"},
	} {
		if code, _, errOut := run(args...); code != ExitUsage || !strings.Contains(errOut, "--awaited") || !strings.Contains(errOut, "full help:") {
			t.Errorf("%v: exit %d, %s", args, code, errOut)
		}
	}
	t.Setenv(state.SessionEnv, "")
	if code, _, errOut := run("inbox", "--awaited"); code != ExitUsage || !strings.Contains(errOut, "not part of a rewake session") {
		t.Fatalf("outside a session: exit %d, %s", code, errOut)
	}
}

// The latest report decides between pending and stopped. Every report on one
// wait shares its id prefix, so it is the time that says which came last.
func TestAwaitedTakesTheLatestInterimOrStop(t *testing.T) {
	room := newAwaitedRoom(t)
	worker := otherRun(t, room.dir, "worker")
	task := room.sentAndRead(worker, "a long task")
	room.turnEnds(worker, inbox.TurnEnd{Boundary: boundaryNow(t, room.dir, worker), ID: "w/1", Text: "interrupted", Stopped: true})
	if view := byID(awaitedJSON(t))[task]; view.State != inbox.StageStopped {
		t.Fatalf("after the stop: %+v", view)
	}
	time.Sleep(2 * time.Millisecond)
	if err := markPending(room.dir, worker.Name, worker.Epoch(), "resumed, still running", markAt); err != nil {
		t.Fatal(err)
	}
	room.turnEnds(worker, inbox.TurnEnd{Boundary: boundaryNow(t, room.dir, worker), ID: "w/2", Text: "resumed", Started: markAt - 1, Ended: boottime.Now()})
	if view := byID(awaitedJSON(t))[task]; view.State != inbox.StagePending || view.Detail != "resumed, still running\n\nresumed" {
		t.Fatalf("after the interim that followed the stop: %+v", view)
	}
}

// A recipient that answered and then ended, with nobody taking its name, keeps
// its wait records: nothing sweeps them until a new run starts. So an answered
// task stays answered after lead's own mailbox has swept the report away.
func TestAwaitedTrustsAnEndedRunsRecords(t *testing.T) {
	room := newAwaitedRoom(t)
	worker := otherRun(t, room.dir, "worker")
	room.sentAndRead(worker, "a small task")
	room.turnEnds(worker, inbox.TurnEnd{Boundary: boundaryNow(t, room.dir, worker), ID: "w/1", Text: "done"})
	for _, report := range finishedFor(t, room.dir, room.lead.Name) {
		if err := os.Remove(report); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(state.SessionPath(room.dir, worker.Name)); err != nil {
		t.Fatal(err)
	}
	if model := awaitedJSON(t); len(model.Recipients) != 0 {
		t.Fatalf("an answered task shows again: %+v", model)
	}
}
