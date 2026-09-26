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
