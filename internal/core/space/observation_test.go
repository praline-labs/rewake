package space

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A close that observed the worker but not the checkout is refused: an
// observation nobody made must not read as a checkout seen gone, which would
// close the space and drop the reminder of a checkout still there (SP2, SP7).
func TestACloseNeedsTheCheckoutObserved(t *testing.T) {
	dir := buildtest.Dir(t)
	record, checkout := seated(t, dir, "unseen")
	if got, err := Close(dir, record.ID, End{At: time.Now().UTC(), Why: "worker ended", Worker: WorkerEnded}); err == nil {
		t.Fatalf("closed with no observation of the checkout: %s", got.State)
	}
	if got, err := Load(dir, record.ID); err != nil || got.State != Open {
		t.Fatalf("the refused close changed the record: %+v %v", got, err)
	}
	if _, err := os.Stat(checkout); err != nil {
		t.Fatal(err)
	}
}

// A checkout that could not be looked at closes the space kept, and it is
// reminded until it is seen gone.
func TestAnUnknownCheckoutKeepsTheSpace(t *testing.T) {
	dir := buildtest.Dir(t)
	record, checkout := seated(t, dir, "unlooked")
	got, err := Close(dir, record.ID, observed(WorkerEnded, Seen{Presence: Unknown}))
	if err != nil || got.State != Kept {
		t.Fatalf("an unknown checkout: %+v %v", got, err)
	}
	if reminders := Reminders([]Record{got}); len(reminders) != 1 || reminders[0].Path != checkout {
		t.Fatalf("an unknown checkout is not reminded: %+v", reminders)
	}
	if _, err := Close(dir, record.ID, observed(WorkerEnded, Seen{Presence: Unknown, Changes: true})); err == nil {
		t.Fatal("a second close of a kept space")
	}
	for name, end := range map[string]End{
		"findings in an unknown checkout": observed(WorkerEnded, Seen{Presence: Unknown, Commits: true}),
		"a presence nobody defined":       observed(WorkerEnded, Seen{Presence: "elsewhere"}),
	} {
		other, _ := seated(t, dir, "other")
		if _, err := Close(dir, other.ID, end); err == nil {
			t.Errorf("%s: closed", name)
		}
		if _, err := Close(dir, other.ID, observed(WorkerEnded, Seen{Presence: Gone})); err != nil {
			t.Fatal(err)
		}
	}
}

// A closed record whose end omits the checkout, or omits its presence, does
// not read: the file says nothing of the checkout, and reading that as gone
// would forget a checkout that may be there.
func TestARecordWithoutTheCheckoutObservedDoesNotRead(t *testing.T) {
	for _, absent := range []string{"checkout", "presence", "null checkout"} {
		t.Run(absent, func(t *testing.T) {
			dir := buildtest.Dir(t)
			record, _ := seated(t, dir, "unread")
			record.State = Closed
			record.End = &End{At: time.Now().UTC(), Why: "worker ended", Worker: WorkerEnded, Checkout: Seen{Presence: Gone}}
			raw, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			var data map[string]any
			if err := json.Unmarshal(raw, &data); err != nil {
				t.Fatal(err)
			}
			end := data["end"].(map[string]any)
			switch absent {
			case "checkout":
				delete(end, "checkout")
			case "presence":
				delete(end["checkout"].(map[string]any), "presence")
			default:
				end["checkout"] = nil
			}
			if raw, err = json.Marshal(data); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(state.SpacePath(dir, record.ID), raw, 0o600); err != nil {
				t.Fatal(err)
			}
			if got, err := Load(dir, record.ID); err == nil {
				t.Fatalf("a missing %s read as observed: %s %+v", absent, got.State, got.End.Checkout)
			}
		})
	}
}
