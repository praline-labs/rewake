package inbox

import "testing"

// stopsOnRecord answers the open occurrences of a mailbox's stop.
func stopsOnRecord(t *testing.T, dir, name string) []openStop {
	t.Helper()
	open, err := stopState(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	return open
}
