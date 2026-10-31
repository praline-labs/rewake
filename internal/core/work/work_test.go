package work

import (
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
)

// A run's work is the open briefs bound to it, oldest first, each with its
// ticks, and then the reports it waits on: a brief of another run, of an
// earlier run of its name or closed is not its work, and none is nothing.
func TestARunsWorkIsItsBriefsAndTheReportsItAwaits(t *testing.T) {
	home := taskbrief.Home{Room: buildtest.Room(t, filepath.Join(t.TempDir(), "state"), "room")}
	lead := taskbrief.Author{Name: "lead", Epoch: "1.1.boot"}
	run := taskbrief.Author{Name: "write", Epoch: "2.2.boot"}
	earlier := taskbrief.Author{Name: "write", Epoch: "0.0.boot"}
	if got := Of(home, run.Name, run.Epoch); got != "" {
		t.Fatalf("with no briefs: %q", got)
	}
	for _, text := range []string{"# One\n- [ ] a\n- [ ] b\n- [ ] c\n", "# Two\n", "# Other\n- [ ] x\n", "# Earlier\n", "# Closed\n"} {
		if _, err := taskbrief.New(home, lead, text, "", nil); err != nil {
			t.Fatal(err)
		}
	}
	for id, to := range map[int]taskbrief.Author{1: run, 2: run, 3: lead, 4: earlier, 5: run} {
		if err := taskbrief.Bind(home, id, to, taskbrief.Work); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := taskbrief.Tick(home, 1, run, 1, 2, true); err != nil {
		t.Fatal(err)
	}
	if _, err := taskbrief.Close(home, 5); err != nil {
		t.Fatal(err)
	}
	if got := Of(home, run.Name, run.Epoch); got != "brief 1 1/3 · brief 2" {
		t.Fatalf("the worker's work: %q", got)
	}
	briefs, _, err := taskbrief.List(home, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := Line(briefs, lead.Name, lead.Epoch, 3); got != "brief 3 0/1 · ⧗ 3" {
		t.Fatalf("with reports awaited: %q", got)
	}
	if got := Line(nil, lead.Name, lead.Epoch, 0); got != "" {
		t.Fatalf("with nothing: %q", got)
	}
}
