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

// The owner, November 1, 2026: main's hint under its input leaves out the
// reports it waits on, which stand on its strip; a worker's keeps them. A
// running mod of an older build still drops a part that starts with `⧗ `
// for main, which a text already cut has none of, so it passes unchanged.
func TestMainsTailLeavesOutTheReportsItAwaits(t *testing.T) {
	for _, c := range []struct{ line, main, worker string }{
		{"brief 57 3/6 · ⧗ 2", "brief 57 3/6", "brief 57 3/6 · ⧗ 2"},
		{"brief 1 · brief 2 1/2 · ⧗ 3", "brief 1 · brief 2 1/2", "brief 1 · brief 2 1/2 · ⧗ 3"},
		{"⧗ 2", "", "⧗ 2"},
		{"", "", ""},
	} {
		if got := TailOf(c.line, true); got != c.main {
			t.Errorf("main %q: %q, want %q", c.line, got, c.main)
		}
		if got := TailOf(c.line, false); got != c.worker {
			t.Errorf("worker %q: %q, want %q", c.line, got, c.worker)
		}
		if got := TailOf(c.main, true); got != c.main {
			t.Errorf("cut text %q cut again: %q", c.main, got)
		}
	}
}
