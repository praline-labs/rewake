package wrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/cutover"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

// A launch proves the earlier writers stopped before it writes anything,
// then records its run and binds itself as the name's successor before its
// session record is published (docs/protocol-cutover.md#the-launch).
func TestALaunchRecordsItsRunBeforeItIsReady(t *testing.T) {
	dir := stateDir(t)
	refused := &cutover.RefusalError{Name: "api-fake", Blocking: []cutover.Process{{PID: 42, Kind: cutover.EarlierRewake, Detail: "a rewake of the earlier build"}}}
	request := Request{Dir: dir, Name: "api", Harness: &fakeHarness{}}
	var seen string
	refuse := func(_, name string) error { seen = name; return refused }
	if _, err := claimName(request, os.Getpid(), selfStart(t), thisBoot(t), dir, refuse); !errors.Is(err, refused) || seen != "api-fake" {
		t.Fatalf("refused launch: %v, checked %q", err, seen)
	}
	epoch := registry.RunEpoch(os.Getpid(), selfStart(t), thisBoot(t))
	if _, found, _ := registry.ReadRunRecord(dir, "api-fake", epoch); found {
		t.Error("a refused launch recorded its run")
	}
	if state, _, _ := registry.Successor(dir, "api-fake"); state != registry.SuccessorNone {
		t.Errorf("a refused launch bound itself: %v", state)
	}
	if _, err := registry.Load(dir, "api-fake"); !errors.Is(err, registry.ErrNotFound) {
		t.Errorf("a refused launch published: %v", err)
	}

	session, err := claimName(request, os.Getpid(), selfStart(t), thisBoot(t), dir, noWriters)
	if err != nil {
		t.Fatal(err)
	}
	record, found, err := registry.ReadRunRecord(dir, "api-fake", session.Epoch())
	if err != nil || !found || record.Build != registry.BuildStamp || record.Started == 0 || record.PIDNamespace == "" {
		t.Fatalf("run record %+v %v %v", record, found, err)
	}
	if state, bound, _ := registry.Successor(dir, "api-fake"); state != registry.SuccessorReady || bound != session.Epoch() {
		t.Errorf("successor %v %s", state, bound)
	}
	// A launch refused for a name some live run holds binds nothing: bound,
	// it would make every report held for the name moot once it exits.
	if _, err := claimName(request, os.Getpid(), selfStart(t)+1, thisBoot(t), dir, noWriters); err == nil {
		t.Fatal("a second launch took a live name")
	}
	if _, found, _ := registry.ReadRunRecord(dir, "api-fake", registry.RunEpoch(os.Getpid(), selfStart(t)+1, thisBoot(t))); found {
		t.Error("a launch refused for a live name recorded its run")
	}
	if _, bound, _ := registry.Successor(dir, "api-fake"); bound != session.Epoch() {
		t.Errorf("the successor was replaced by %s", bound)
	}
}

// A main of this build, as it starts, is told once of every run of the
// earlier build still running, which a main of that build never heard of.
func TestAMainIsToldOfEarlierRunsAtItsStart(t *testing.T) {
	dir := stateDir(t)
	lead, err := claimName(Request{Dir: dir, Name: "lead", Harness: &fakeHarness{}, Role: role.Main}, os.Getpid(), selfStart(t), thisBoot(t), dir, noWriters)
	if err != nil {
		t.Fatal(err)
	}
	old := registry.Session{Name: "old", ServicePID: os.Getpid(), ServiceStart: selfStart(t), PIDNamespace: proc.Namespace(), StartedAt: time.Now()}
	raw, _ := json.Marshal(old)
	if err := os.WriteFile(state.SessionPath(dir, "old"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		takeHeld(context.Background(), dir, lead)
	}
	notes := availabilityFiles(t, dir, lead.Name)
	if len(notes) != 1 || !strings.Contains(notes[0].Text, "old was started by a rewake build before this one") {
		t.Fatalf("notes %+v", notes)
	}
}
