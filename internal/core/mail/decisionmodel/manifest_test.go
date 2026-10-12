package decisionmodel

import (
	"os"
	"path/filepath"
	"slices"
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
// holding an excluded world, is no cover. The worlds are named by their
// values in the partition's order: for decision, truth, at-decision, later,
// choice, second, decider, r-at-decision, r-after-decision, activity, sender;
// for resend, r-prime, second-choice, decider, r-life, sender.
func TestExcludedWorldsAreNeverFeasible(t *testing.T) {
	excluded := map[string][]string{
		"decision": {
			"landed/mark-unreadable/returns-absent/arrived/none/recipient-verified/r-alive/r-lives-on/none/s-alive",
			"not-landed/mark-unreadable/returns-published/arrived/none/recipient-verified/r-alive/r-lives-on/none/s-alive",
			"landed/mark-unreadable/retired-proof/arrived/none/person/r-alive/r-lives-on/none/s-alive",
			"landed/mark-unreadable/returns-before-decision/arrived/same-words/recipient-verified/r-alive/r-lives-on/none/s-alive",
			"not-landed/effect-met/stays-unknown/arrived/none/recipient-verified/r-alive/r-lives-on/r-reads/s-alive",
			"landed/effect-met/stays-unknown/arrived/none/recipient-verified/r-alive/r-lives-on/new-run-drops-marks/s-alive",
		},
		"resend": {
			"none-then-published/none/person/r-ends-before-decision/s-alive",
			"unknown-mark-landed/none/recipient-verified/r-alive/s-alive",
			"none-then-published/arrived/recipient-verified/r-alive/s-alive",
			"unknown-intent-not/arrived/recipient-verified/r-ends-letter-to-mark/s-alive",
			"moot-no-write/none/recipient-verified/r-alive/s-alive",
		},
		"mailbox": {"child-unknown-left/none/arrived"},
	}
	for _, p := range Partitions() {
		for _, name := range excluded[p.Name] {
			if feasible(t, p, name) {
				t.Errorf("%s: %s is feasible", p.Name, name)
			}
		}
		set := p.PairCover()
		if len(set) > 1 && p.Covers(set[1:]) {
			t.Errorf("%s: a set short of its first world still covers", p.Name)
		}
	}
}

// feasible builds the named world of p, failing the test when the name is
// no world of it.
func feasible(t *testing.T, p Partition, name string) bool {
	t.Helper()
	values := strings.Split(name, "/")
	if len(values) != len(p.Dimensions) {
		t.Fatalf("%s: %s has %d values for %d dimensions", p.Name, name, len(values), len(p.Dimensions))
	}
	for i, v := range values {
		if !slices.Contains(p.Dimensions[i].Values, v) {
			t.Fatalf("%s: %q is no value of %s", p.Name, v, p.Dimensions[i].Name)
		}
	}
	return p.Feasible(World{Partition: p.Name, Values: values, dims: p.Dimensions})
}

// The schedules the model allows and those it does not, by when r lives and
// who may decide: life and authority at the decision are apart from r's life
// and the activity in its mailbox after it (M5, M6, M8 of the model).
func TestTheSchedulesKeepLifeAtTheDecisionApartFromLater(t *testing.T) {
	partition := map[string]Partition{}
	for _, p := range Partitions() {
		partition[p.Name] = p
	}
	cases := []struct {
		partition, world string
		allowed          bool
		why              string
	}{
		{
			"decision", "landed/effect-met/retired-proof/arrived/none/recipient-verified/r-alive/r-ends-after-decision/new-run-drops-marks/s-alive", true,
			"verified while r lived, then r ended and a new run retired its proof",
		},
		{
			"decision", "landed/effect-met/stays-unknown/not-arrived/none/recipient-verified/r-alive/r-ends-after-decision/new-run-drops-marks/s-alive", true,
			"verified while r lived, then a new run dropped r's marks",
		},
		{
			"decision", "landed/mark-unreadable/returns-published/arrived/none/person/r-alive/r-lives-on/sweep-promotes/s-alive", true,
			"a mark that did not read at D, repaired, then promoted by the sweep",
		},
		{
			"decision", "landed/mark-unreadable/stays-unknown/arrived/none/person/r-ends-before-decision/none/r-reads/s-alive", true,
			"r read R before it ended, and the person decided after",
		},
		{
			"decision", "landed/effect-met/retired-proof/arrived/none/person/r-ends-before-decision/none/new-run-drops-marks/s-alive", true,
			"r ended before the decision, a new run retired its proof",
		},
		{
			"decision", "landed/effect-met/stays-unknown/arrived/none/recipient-verified/r-ends-before-decision/none/none/s-alive", false,
			"a decision verified by r after r ended",
		},
		{
			"decision", "landed/effect-met/stays-unknown/arrived/none/person/r-ends-before-decision/r-lives-on/none/s-alive", false,
			"r living on after it ended",
		},
		{
			"decision", "landed/effect-met/stays-unknown/arrived/none/person/r-alive/none/none/s-alive", false,
			"r alive at D with nothing said of it after",
		},
		{
			"decision", "landed/effect-met/retired-proof/arrived/none/person/r-alive/r-lives-on/new-run-drops-marks/s-alive", false,
			"a new run of the name while r lives on",
		},
		{
			"decision", "landed/mark-unreadable/stays-unknown/arrived/none/person/r-alive/r-lives-on/sweep-promotes/s-alive", false,
			"the sweep promoting a mark that never read again",
		},
		{
			"resend", "moot-no-write/none/person/r-ends-before-decision/s-alive", true,
			"the person decided after r ended, and the resend found r ended",
		},
		{
			"resend", "moot-no-write/none/recipient-verified/r-ends-before-liveness/s-alive", true,
			"verified while r lived, r ended before the resend's liveness check",
		},
		{
			"resend", "moot-no-write/none/recipient-verified/r-ends-before-decision/s-alive", false,
			"a resend of a decision verified by r after r ended",
		},
	}
	for _, c := range cases {
		if got := feasible(t, partition[c.partition], c.world); got != c.allowed {
			t.Errorf("%s (%s): feasible %v, want %v", c.why, c.world, got, c.allowed)
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
