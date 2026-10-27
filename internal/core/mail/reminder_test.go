package mail

import (
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/boottime"
)

// unattendedLab is api's run with one finished end on record, and the letters
// a serving process announced to it since, by when.
type unattendedLab struct {
	endLab
	announced map[string]int64
}

func newUnattendedLab(t *testing.T) unattendedLab {
	t.Helper()
	lab := unattendedLab{endLab: newEndLab(t), announced: map[string]int64{}}
	if err := lab.complete(t, lab.end(t, "first")); err != nil {
		t.Fatal(err)
	}
	return lab
}

// left announces a letter of kind to api at the boot-clock reading at and
// leaves it unread.
func (lab unattendedLab) left(t *testing.T, kind Kind, at int64) Message {
	t.Helper()
	letter := announcedUnread(t, lab.dir, Message{ID: NewID(), From: "web", FromEpoch: lab.web.Epoch(), To: "api", ToEpoch: lab.self.Epoch(), Kind: kind, Text: string(kind) + " text", CreatedAt: time.Now()})
	lab.announced[letter.ID] = at
	return letter
}

func (lab unattendedLab) unattended(t *testing.T, ended int64) []Message {
	t.Helper()
	left, err := Open(lab.dir, "api", lab.self.Epoch()).Unattended(ended, func(id string) (int64, bool) {
		at, ok := lab.announced[id]
		return at, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	return left
}

// A finished turn that leaves a task announced in it unread is reminded of
// that task alone: not of a note or a report, which ask for nothing, nor of a
// task announced before the turn's window; and the reminded turn's end
// reminds of nothing, since that task now lies before its window. The case
// seen live: a worker ended a turn with a task unread, and nothing woke it.
func TestAFinishedTurnIsRemindedOnceOfTheTaskItLeftUnread(t *testing.T) {
	lab := newUnattendedLab(t)
	before := lab.left(t, Task, 1)
	at := boottime.Now()
	task := lab.left(t, Task, at)
	question := lab.left(t, Question, at)
	lab.left(t, Note, at)
	lab.left(t, Finished, at)

	end := lab.end(t, "second")
	if err := lab.complete(t, end); err != nil {
		t.Fatal(err)
	}
	left := lab.unattended(t, end.Ended)
	if len(left) != 2 || !holds(left, task.ID) || !holds(left, question.ID) || holds(left, before.ID) {
		t.Fatalf("reminded of %v, want the task and the question announced in the turn", ids(left))
	}

	reminded := lab.end(t, "third")
	if err := lab.complete(t, reminded); err != nil {
		t.Fatal(err)
	}
	if left := lab.unattended(t, reminded.Ended); len(left) != 0 {
		t.Fatalf("the reminded turn's end reminded again of %v", ids(left))
	}
}

// An interim end says the work goes on: its publication calls it no finish,
// so it is never looked at. An end a later one superseded has that later end's
// window, and reminds of nothing.
func TestAnInterimOrSupersededEndRemindsOfNothing(t *testing.T) {
	lab := newUnattendedLab(t)
	lab.left(t, Task, boottime.Now())
	if err := markPending(lab.dir, "api", lab.self.Epoch(), "waiting on CI", boottime.Now()); err != nil {
		t.Fatal(err)
	}
	interim := lab.end(t, "interim")
	interim.Started = 0
	outcome, err := CompleteTurn(t.Context(), lab.dir, lab.self, interim, "thread-1", TurnEndOptions{})
	if err != nil || !outcome.Interim || outcome.Finished != 0 {
		t.Fatalf("an interim end said %+v, %v; want interim and no finish", outcome, err)
	}

	lab = newUnattendedLab(t)
	earlier := lab.end(t, "earlier")
	lab.left(t, Task, boottime.Now()-1)
	later := lab.end(t, "later")
	if err := lab.complete(t, later); err != nil {
		t.Fatal(err)
	}
	if left := lab.unattended(t, earlier.Ended); len(left) != 0 {
		t.Fatalf("an end a later one superseded reminded of %v", ids(left))
	}
}

// The reminder's notice counts what is waiting and previews the latest
// letter, saying it is a preview only.
func TestAReminderNoticeSaysWhatIsStillWaiting(t *testing.T) {
	first := Message{ID: "1", From: "main", Kind: Task, Text: "do one", CreatedAt: time.Unix(1, 0)}
	second := Message{ID: "2", From: "main", Kind: Question, Text: "which one?", CreatedAt: time.Unix(2, 0)}
	one := first
	one.Reminder = true
	if got, want := Notice(one), "Rewake: 1 unread task still waiting\n  ↳ main task: do one"+previewOnly; got != want {
		t.Fatalf("notice %q, want %q", got, want)
	}
	two := Message{ID: "reminder", Reminder: true, Batch: []Message{second, first}}
	if got, want := Notice(two), "Rewake: 2 unread tasks still waiting\n  ↳ main question: which one?"+previewOnly; got != want {
		t.Fatalf("notice %q, want %q", got, want)
	}
}

func holds(messages []Message, id string) bool {
	for _, message := range messages {
		if message.ID == id {
			return true
		}
	}
	return false
}

func ids(messages []Message) []string {
	var out []string
	for _, message := range messages {
		out = append(out, message.ID)
	}
	return out
}
