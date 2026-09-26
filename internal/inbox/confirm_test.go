package inbox

import "testing"

// The records of the confirmation belong to one run: another run of the name
// reads none, and clearing one leaves the other.
func TestConfirmRecordsBelongToTheirRun(t *testing.T) {
	dir := stateDir(t)
	if err := NoteInterim(dir, "api", "1.1", "the suite is running"); err != nil {
		t.Fatal(err)
	}
	if err := KeepAnswer(dir, "api", "1.1", "the answer"); err != nil {
		t.Fatal(err)
	}
	if line, ok := LastInterim(dir, "api", "1.1"); !ok || line != "the suite is running" {
		t.Errorf("interim: %q %v", line, ok)
	}
	if _, ok := LastInterim(dir, "api", "2.2"); ok {
		t.Error("another run read this run's interim end")
	}
	if _, ok := KeptAnswer(dir, "api", "2.2"); ok {
		t.Error("another run read this run's kept answer")
	}
	if err := ClearInterim(dir, "api"); err != nil {
		t.Fatal(err)
	}
	if _, ok := LastInterim(dir, "api", "1.1"); ok {
		t.Error("the interim end outlived its clearing")
	}
	if text, ok := KeptAnswer(dir, "api", "1.1"); !ok || text != "the answer" {
		t.Errorf("clearing the interim end took the answer: %q %v", text, ok)
	}
	if err := DropKeptAnswer(dir, "api"); err != nil {
		t.Fatal(err)
	}
	if err := DropKeptAnswer(dir, "api"); err != nil {
		t.Errorf("dropping nothing: %v", err)
	}
}

// MarkedWithin answers what TakePending would, and takes nothing.
func TestMarkedWithinTakesNothing(t *testing.T) {
	dir := stateDir(t)
	mark(t, dir, "1.1", "waiting", 50)
	for _, c := range []struct {
		epoch          string
		started, ended int64
		want           bool
	}{
		{"1.1", 40, 60, true},
		{"1.1", 40, 50, true},
		{"1.1", 51, 60, false},
		{"1.1", 40, 49, false},
		{"1.1", 0, 60, false},
		{"1.1", 40, 0, false},
		{"2.2", 40, 60, false},
	} {
		if got := MarkedWithin(dir, "api", c.epoch, c.started, c.ended); got != c.want {
			t.Errorf("%s %d..%d: %v", c.epoch, c.started, c.ended, got)
		}
	}
	if !marked(dir) {
		t.Fatal("MarkedWithin took the mark")
	}
	if text, ok := take(t, dir, "1.1", 40, 60); !ok || text != "waiting" {
		t.Errorf("TakePending disagrees: %q %v", text, ok)
	}
}
