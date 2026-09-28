package grant

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
)

func ownRun(t *testing.T) string {
	t.Helper()
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%d.%d", os.Getpid(), start)
}

func endedRun(t *testing.T) string {
	t.Helper()
	child := exec.Command("true")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	start, err := proc.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	return fmt.Sprintf("%d.%d", child.Process.Pid, start)
}

// The copies say which grants a conversation had, by message, from every run
// that journaled them; a grant over, or one in another conversation, is not
// among them.
func TestHintsNameTheGrantsOfAConversation(t *testing.T) {
	dir := t.TempDir()
	main := ownRun(t)
	first := []Entry{
		{Path: "/w/lib", Message: "m1", Outcome: Granted, Thread: "t1", From: "lead", FromEpoch: main},
		{Path: "/w/lib/.git", For: "/w/lib", Message: "m1", Outcome: Granted, Thread: "t1", From: "lead", FromEpoch: main},
		{Path: "/w/old", Message: "m0", Outcome: Revoked, Thread: "t1", From: "lead", FromEpoch: main},
		{Path: "/w/other", Message: "m2", Outcome: Granted, Thread: "t2", From: "lead", FromEpoch: main},
	}
	second := []Entry{
		{Path: "/w/lib", Message: "m1", Outcome: Revoking, Thread: "t1", From: "lead", FromEpoch: main},
		{Path: "/w/doc", Message: "m3", Outcome: Granted, Thread: "t1", From: "lead", FromEpoch: main},
	}
	if err := Save(dir, "writer", "1.1", first); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, "writer", "2.2", second); err != nil {
		t.Fatal(err)
	}
	hints := Hints(dir, "t1")
	if len(hints) != 2 {
		t.Fatalf("hints %+v", hints)
	}
	byMessage := map[string]Hint{}
	for _, hint := range hints {
		byMessage[hint.Message] = hint
	}
	if lib := byMessage["m1"]; !slices.Equal(lib.Paths, []string{"/w/lib", "/w/lib/.git"}) || lib.FromEpoch != main || lib.From != "lead" {
		t.Errorf("m1: %+v", lib)
	}
	if doc := byMessage["m3"]; !slices.Equal(doc.Paths, []string{"/w/doc"}) {
		t.Errorf("m3: %+v", doc)
	}
	if Hints(dir, "") != nil {
		t.Error("hints for no conversation")
	}
}

// The copy of an ended run stays while it names a grant its main could still
// confirm again, and goes once that main has ended too.
func TestTheCopyOfAnEndedRunStaysWhileItsMainCouldConfirm(t *testing.T) {
	dir := t.TempDir()
	restorable := []Entry{{Path: "/w/a", Message: "m1", Outcome: Granted, Thread: "t1", From: "lead", FromEpoch: ownRun(t)}}
	orphaned := []Entry{{Path: "/w/b", Message: "m2", Outcome: Granted, Thread: "t1", From: "lead", FromEpoch: endedRun(t)}}
	unthreaded := []Entry{{Path: "/w/c", Message: "m3", Outcome: Granted, From: "lead", FromEpoch: ownRun(t)}}
	runs := map[string][]Entry{"1.1": restorable, "2.2": orphaned, "3.3": unthreaded}
	for run, entries := range runs {
		if err := Save(dir, "gone", run, entries); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-2 * sweepGrace)
		if err := os.Chtimes(journalPath(dir, "gone", run), old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := Save(dir, "gone", "4.4", nil); err != nil {
		t.Fatal(err)
	}
	if Load(dir, "gone", "1.1") == nil {
		t.Error("a copy its main could still confirm was swept")
	}
	for _, run := range []string{"2.2", "3.3"} {
		if _, err := os.Stat(journalPath(dir, "gone", run)); !os.IsNotExist(err) {
			t.Errorf("run %s: the copy stayed: %v", run, err)
		}
	}
}
