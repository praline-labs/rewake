package space

import (
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
)

// The mark of main's end moves the open spaces of that run to closing, so
// their hosts end the workers at once, and touches no space of another run
// or one not open.
func TestMainsEndingClosesOnlyItsOpenSpaces(t *testing.T) {
	dir := buildtest.Dir(t)
	mine, _ := seated(t, dir, "mine")
	other := opening(t, "other")
	other.Main.Run = "4343.1700.0f0e0d0c-0b0a-4908-8706-050403020100"
	if err := Create(dir, other); err != nil {
		t.Fatal(err)
	}
	if _, err := SetHost(dir, other.ID, "6262.1750.0f0e0d0c-0b0a-4908-8706-050403020100"); err != nil {
		t.Fatal(err)
	}
	if _, err := Opened(dir, other.ID, "6363.1751.0f0e0d0c-0b0a-4908-8706-050403020100"); err != nil {
		t.Fatal(err)
	}
	pending := created(t, dir, "pending")

	if err := BeginClosingFor(dir, mine.Main.Run, MainEnding); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]State{mine.ID: Closing, other.ID: Open, pending.ID: Opening} {
		got, err := Load(dir, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.State != want {
			t.Fatalf("space %s is %s, not %s", got.Name, got.State, want)
		}
	}
	if got, _ := Load(dir, mine.ID); got.Closing != MainEnding {
		t.Fatalf("closing for %q", got.Closing)
	}
}
