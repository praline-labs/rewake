package decisionmodel

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The manifest publishes what this package enumerates: a partition's product,
// its feasible worlds, its pairs and the size of its pair cover. A change of a
// value or an exclusion that the manifest does not follow fails here.
func TestTheManifestCountsAreTheModels(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "v2", "stage3-decision-manifest.md"))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string][]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		cells := strings.Split(line, "|")
		if len(cells) == 9 && strings.HasPrefix(strings.TrimSpace(cells[1]), "`") {
			rows[strings.Trim(strings.TrimSpace(cells[1]), "`")] = cells[4:8]
		}
	}
	feasible, cover := 0, 0
	for _, p := range Partitions() {
		product := 1
		for _, d := range p.Dimensions {
			product *= len(d.Values)
		}
		set := p.PairCover()
		if !p.Covers(set) {
			t.Fatalf("%s: the generated set does not cover every pair", p.Name)
		}
		want := []int{product, len(p.All()), p.Pairs(), len(set)}
		row, ok := rows[p.Name]
		if !ok {
			t.Fatalf("the manifest has no row for %s", p.Name)
		}
		for i, cell := range row {
			if got, err := strconv.Atoi(strings.TrimSpace(cell)); err != nil || got != want[i] {
				t.Errorf("%s: the manifest says %q where the model enumerates %d", p.Name, cell, want[i])
			}
		}
		feasible += want[1]
		cover += want[3]
	}
	for _, total := range []string{strconv.Itoa(feasible) + " in all", strconv.Itoa(cover) + " in all"} {
		if !strings.Contains(string(raw), total) {
			t.Errorf("the manifest does not say %q", total)
		}
	}
	if len(rows) != len(Partitions()) {
		t.Errorf("the manifest has %d partitions, the model %d", len(rows), len(Partitions()))
	}
}

// Every excluded combination is refused, and a set missing one pair, or
// holding an excluded world, is no cover.
func TestExcludedWorldsAreNeverFeasible(t *testing.T) {
	excluded := map[string][]string{
		"decision": {
			"landed/mark-unreadable/returns-absent/arrived/none/recipient-verified/none/s-alive",
			"not-landed/mark-unreadable/returns-published/arrived/none/recipient-verified/none/s-alive",
			"landed/mark-unreadable/retired-proof/arrived/none/person-r-alive/none/s-alive",
			"landed/mark-unreadable/returns-before-decision/arrived/same-words/recipient-verified/none/s-alive",
			"not-landed/effect-met/stays-unknown/arrived/none/recipient-verified/r-reads/s-alive",
			"landed/mark-unreadable/stays-unknown/arrived/none/recipient-verified/sweep-promotes/s-alive",
			"landed/effect-met/stays-unknown/arrived/none/recipient-verified/new-run-drops-marks/s-alive",
		},
		"resend": {
			"none-then-published/none/r-ends-before-decision/s-alive",
			"unknown-mark-landed/none/r-alive/s-alive",
			"none-then-published/arrived/r-alive/s-alive",
			"unknown-intent-not/arrived/r-ends-letter-to-mark/s-alive",
			"moot-no-write/none/r-alive/s-alive",
		},
		"mailbox": {"child-unknown-left/none/arrived"},
	}
	for _, p := range Partitions() {
		for _, name := range excluded[p.Name] {
			w := World{Partition: p.Name, Values: strings.Split(name, "/"), dims: p.Dimensions}
			if p.Feasible(w) {
				t.Errorf("%s: %s is feasible", p.Name, name)
			}
		}
		set := p.PairCover()
		if len(set) > 1 && p.Covers(set[1:]) {
			t.Errorf("%s: a set short of its first world still covers", p.Name)
		}
	}
}

// The outcomes the model's table of two generations gives, with x the
// landing of R.
func TestTwoGenerationsCountByTheirTruth(t *testing.T) {
	for _, x := range []bool{true, false} {
		base := 0
		if x {
			base = 1
		}
		cases := []struct {
			prime   bool
			second  string
			landing int
			right   bool
		}{
			{true, Arrived, base + 1, true},
			{true, NotArrived, base + 2, false},
			{false, Arrived, base, false},
			{false, NotArrived, base + 1, true},
		}
		for _, c := range cases {
			chain := []Copy{{Landed: x, Decided: NotArrived}, {Landed: c.prime, Decided: c.second}}
			if c.second == NotArrived {
				chain = append(chain, Copy{Landed: true})
			}
			out := Expect(chain)
			right := c.right && !x
			if out.Landings != c.landing || out.Right != right {
				t.Errorf("x=%v R' %v %s: %+v, want %d landings, right %v", x, c.prime, c.second, out, c.landing, right)
			}
		}
	}
	if out := Expect([]Copy{{Landed: false, Decided: Arrived}}); !out.Lost || out.Copies != 1 {
		t.Errorf("a wrong arrived over nothing landed: %+v", out)
	}
}

// A note goes to each run that did not decide verified and still lives, once
// when the recipient is main.
func TestNotesGoToThoseWhoDidNotDecide(t *testing.T) {
	lead := Run{Name: "lead", Epoch: "r", Alive: true}
	main := Run{Name: "boss", Epoch: "m", Alive: true}
	if got := Told(lead, main, "r", true); len(got) != 1 || got[0] != main {
		t.Errorf("verified by r: %v", got)
	}
	if got := Told(lead, lead, "", true); len(got) != 1 {
		t.Errorf("the person, r is main: %v", got)
	}
	if got := Told(Run{Name: "lead", Epoch: "r"}, main, "", false); len(got) != 0 {
		t.Errorf("r ended, no main: %v", got)
	}
}
