package host

// Main's authorship policy is frozen once and carried by the record: a
// launch writes it, an adoption reads it back, and no later step opens the
// alias file (docs/roles.md, docs/rules/state.md).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/core/role"
)

// The record the launch publishes holds the policy, and an adoption of that
// run takes it from the record even after the project file says the opposite.
func TestAnAdoptionTakesTheRunsFrozenPolicyNotTheProjectFile(t *testing.T) {
	dir := stateDir(t)
	start := selfStart(t)
	boot := thisBoot(t)
	session, err := claimName(Request{
		Dir: dir, Harness: &fakeHarness{}, Name: "lead", Role: role.Main, MainImplements: true,
	}, os.Getpid(), start, boot, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !session.MainImplements {
		t.Fatal("the launch did not write the policy into the record")
	}
	// A later edit of the project file must not reach an adoption: it names
	// the opposite, and the record still says true.
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".rewake"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".rewake", "aliases.toml"), []byte("[main]\nimplements = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)

	handed := &Handover{Version: HandoverVersion, Harness: "fake", Dir: dir, Name: session.Name, Epoch: session.Epoch(), Role: role.Main.ID}
	adopted, err := (adopting{handed}).claim(handed.Request(&fakeHarness{}), os.Getpid(), start, boot, dir)
	if err != nil {
		t.Fatalf("adopting a main run: %v", err)
	}
	if !adopted.MainImplements {
		t.Fatal("the adoption lost main's policy")
	}
	// The handover itself carries no second copy, so a run takes the value
	// from the record alone and the two can never disagree.
	r := &run{request: handed.Request(&fakeHarness{})}
	r.takeAuthorship(adopted)
	if !r.request.MainImplements {
		t.Fatal("a run took its policy from anywhere but the record")
	}
}

// An older image has no field for the policy and would drop it on its next
// record update. The handover contract refuses it instead, and the room takes
// the ordinary restart rather than silently forgetting an opt-in.
func TestAnOlderImageCannotAdoptAPolicyBearingRun(t *testing.T) {
	if HandoverVersion != 8 {
		t.Fatalf("HandoverVersion is %d", HandoverVersion)
	}
	if _, err := ReadHandover(`{"version":7,"harness":"pi","dir":"/tmp/x","name":"main-pi","role":"main"}`); err == nil {
		t.Fatal("version 7 was adopted by a build that speaks 8")
	}
}
