package inbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/state"
)

// writeMark writes a waiting send's mark with the given content and time.
func writeMark(t *testing.T, dir, question string, content []byte, touched time.Time) {
	t.Helper()
	marks := state.AnsweringPath(dir, "web")
	if err := state.EnsureSubdir(marks); err != nil {
		t.Fatal(err)
	}
	mark := filepath.Join(marks, question)
	if err := os.WriteFile(mark, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(mark, touched, touched); err != nil {
		t.Fatal(err)
	}
}

// A mark is judged by the boot clock reading it holds: a step of the wall
// clock that makes its file time look ten seconds old leaves a live send
// waiting, and its report is handed to it rather than announced.
func TestAMarkIsJudgedByItsBootReading(t *testing.T) {
	cases := []struct {
		name    string
		content []byte
		touched time.Duration
		want    bool
	}{
		{"a fresh reading, a file time stepped back", markContent(boottime.Now()), -10 * time.Second, true},
		{"an old reading, a fresh file time", markContent(boottime.Now() - int64(10*time.Second)), 0, false},
		{"a reading from before a reboot", markContent(boottime.Now() + int64(time.Hour)), 0, false},
		{"no reading, a fresh file time", nil, 0, true},
		{"no reading, an old file time", nil, -10 * time.Second, false},
		{"half a reading, a fresh file time", []byte("12"), 0, true},
	}
	for _, c := range cases {
		dir := stateDir(t)
		question := NewID()
		writeMark(t, dir, question, c.content, time.Now().Add(c.touched))
		report := Message{ID: NewID(), From: "api", To: "web", Kind: Finished, InReplyTo: []string{question}}
		if got := awaitedHere(dir, "web", report); got != c.want {
			t.Errorf("%s: awaited %v, want %v", c.name, got, c.want)
		}
	}
}

// The heartbeat writes the reading into a mark and never brings a removed one
// back: the report taken as the answer removes the mark while the send is
// still printing, and a mark written back would hide that report.
func TestTheHeartbeatDoesNotBringAMarkBack(t *testing.T) {
	dir := stateDir(t)
	question := NewID()
	release, err := ReserveAnswer(dir, "web", question)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	mark := filepath.Join(state.AnsweringPath(dir, "web"), question)
	if !markFresh(mark) {
		t.Fatal("a mark just reserved is not fresh")
	}
	if err := os.Remove(mark); err != nil {
		t.Fatal(err)
	}
	time.Sleep(answerPoll + 300*time.Millisecond)
	if _, err := os.Stat(mark); !os.IsNotExist(err) {
		t.Fatalf("the heartbeat wrote the removed mark back: %v", err)
	}
}
