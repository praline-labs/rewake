package inbox

import "testing"

func TestLaterDeliveryThreadsAreCompared(t *testing.T) {
	dir := stateDir(t)
	if err := recordDeliveryThread(dir, "api", "first", "current"); err != nil {
		t.Fatal(err)
	}
	if err := recordDeliveryThread(dir, "api", "second", "previous"); err != nil {
		t.Fatal(err)
	}
	if !ReportThreadChanged(dir, "api", []string{"first", "second"}, "current") {
		t.Fatal("later mismatched delivery omitted from shared report")
	}
}
