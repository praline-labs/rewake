package registry

// mainImplements is optional: absent means main delegates, so an old record
// stays valid and a delegating main writes no field at all.

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func TestTheAuthorshipPolicyIsAnOptionalRecordField(t *testing.T) {
	dir := stateDir(t, map[int]uint64{10: 100})
	leader := session("lead", 10, 100)
	leader.Role = role.Main.ID
	leader.MainImplements = true
	if err := Publish(dir, leader); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir, "lead")
	if err != nil || !loaded.MainImplements {
		t.Fatalf("the policy did not survive publishing: %+v %v", loaded, err)
	}
	// A read-modify-write of another field keeps it: the two writers a run has
	// change what they know and never what they do not.
	if err := Modify(dir, "lead", loaded.Epoch(), func(s *Session) { s.HarnessPID = 1 }); err != nil {
		t.Fatal(err)
	}
	if again, err := Load(dir, "lead"); err != nil || !again.MainImplements || again.HarnessPID != 1 {
		t.Fatalf("a record update lost the policy: %+v %v", again, err)
	}
	raw, err := os.ReadFile(state.SessionPath(dir, "lead"))
	if err != nil {
		t.Fatal(err)
	}
	if !jsonHas(raw, "mainImplements", true) {
		t.Fatalf("the record does not say %q: %s", "mainImplements", raw)
	}

	// A delegating main writes no field, and an old record without it reads as
	// false: the default is the absence.
	delegating := session("old", 11, 110)
	delegating.Role = role.Main.ID
	encoded, err := encode(delegating)
	if err != nil {
		t.Fatal(err)
	}
	if jsonHas(encoded, "mainImplements", false) || jsonHas(encoded, "mainImplements", true) {
		t.Fatalf("a delegating record carries the field: %s", encoded)
	}
	if err := state.WriteAtomic(state.SessionPath(dir, "old"), encoded); err != nil {
		t.Fatal(err)
	}
	if old, err := Load(dir, "old"); err != nil || old.MainImplements {
		t.Fatalf("an old record is not delegate: %+v %v", old, err)
	}
}

// jsonHas reports whether raw holds key with this boolean value.
func jsonHas(raw []byte, key string, want bool) bool {
	var values map[string]any
	if json.Unmarshal(raw, &values) != nil {
		return false
	}
	got, ok := values[key]
	return ok && got == want
}
