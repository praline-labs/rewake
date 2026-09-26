package inbox

import (
	"testing"
	"time"
)

// A recall does not wait for company: it exists to stop work a preview may
// have started, and every second in the window is time spent on that work.
func TestARecallIsAnnouncedAtOnce(t *testing.T) {
	f := serveWindow(t, Window{Quiet: 3 * time.Second, Cap: 4 * time.Second})
	withdrawn := message("delete the staging bucket")
	withdrawn.ToEpoch = "5.5"
	written := time.Now()
	note, err := Recall(f.dir, withdrawn)
	if err != nil {
		t.Fatal(err)
	}
	notices, at := f.seen(1, 5*time.Second)
	if notices[0].ID != note.ID || at[0].Sub(written) > 2*time.Second {
		t.Fatalf("the recall went out %s after it was written: %+v", at[0].Sub(written), notices[0])
	}
}

// Nor does an edit's replacement, of a notify as of a task: it sends no
// recall, so its own preview is what sets the old notice aside.
func TestAReplacedNotifyIsAnnouncedAtOnce(t *testing.T) {
	f := serveWindow(t, Window{Quiet: 3 * time.Second, Cap: 4 * time.Second})
	replacement := message("the build is green after all")
	replacement.Kind, replacement.ToEpoch, replacement.Replaces = Note, "5.5", NewID()
	written := time.Now()
	if err := Put(f.dir, replacement); err != nil {
		t.Fatal(err)
	}
	notices, at := f.seen(1, 5*time.Second)
	if notices[0].ID != replacement.ID || at[0].Sub(written) > 2*time.Second {
		t.Fatalf("the replacement went out %s after it was written: %+v", at[0].Sub(written), notices[0])
	}
}
