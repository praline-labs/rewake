package upgraderig

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fullMatrix runs every feasible world instead of those covering each pair;
// it is run before S17 lands and again at the stage's acceptance.
var fullMatrix = os.Getenv("REWAKE_UPGRADE_MATRIX") == "full"

// worlds are the worlds a matrix runs: every feasible one, or a set covering
// every feasible pair of values, logged with its coverage.
func worlds(t *testing.T, stopsOf func(string) bool) []scenario {
	t.Helper()
	var all []scenario
	for _, o := range olds {
		for _, s := range stops {
			for _, e := range entries {
				for _, l := range launches {
					sc := scenario{o, s, e, l}
					if stopsOf(s) && feasible(sc) {
						all = append(all, sc)
					}
				}
			}
		}
	}
	chosen := all
	if !fullMatrix {
		chosen = coverPairs(all)
	}
	t.Logf("%d worlds of %d feasible, every feasible pair of values covered (%d pairs)", len(chosen), len(all), len(pairsOf(all)))
	return chosen
}

// pair is two values of two dimensions.
type pair struct {
	i, j int
	a, b string
}

func pairsIn(s scenario) []pair {
	values := s.values()
	var pairs []pair
	for i := range values {
		for j := i + 1; j < len(values); j++ {
			pairs = append(pairs, pair{i, j, values[i], values[j]})
		}
	}
	return pairs
}

func pairsOf(all []scenario) map[pair]bool {
	pairs := map[pair]bool{}
	for _, s := range all {
		for _, p := range pairsIn(s) {
			pairs[p] = true
		}
	}
	return pairs
}

// coverPairs picks, greedily and in a fixed order, worlds until every pair
// of values some feasible world holds is held by a chosen one.
func coverPairs(all []scenario) []scenario {
	uncovered := pairsOf(all)
	var chosen []scenario
	for len(uncovered) > 0 {
		best, gain := -1, 0
		for index, s := range all {
			n := 0
			for _, p := range pairsIn(s) {
				if uncovered[p] {
					n++
				}
			}
			if n > gain {
				best, gain = index, n
			}
		}
		chosen = append(chosen, all[best])
		for _, p := range pairsIn(all[best]) {
			delete(uncovered, p)
		}
	}
	return chosen
}

// TestLegacyClearCrashMatrix kills the clearing after each of its durable
// steps — each rename, each flush, each record and removal and each named
// step — in every world the matrix chooses with that stop, and then requires
// the next launch to find a state of the table: no tree removed before its
// validation, live state still refusing, and a quiet one cleared. The other
// stops — none and the pauses — run in TestLegacyClearConcurrentMatrix.
func TestLegacyClearCrashMatrix(t *testing.T) {
	for _, s := range worlds(t, func(stop string) bool { return stop == "kill each" }) {
		t.Run(s.name(), func(t *testing.T) {
			t.Parallel()
			points := killPoints(t, s)
			for _, n := range points {
				t.Run(strconv.Itoa(n.index)+" "+strings.ReplaceAll(n.step, "/", "_"), func(t *testing.T) {
					crashAt(t, s, n.index)
				})
			}
		})
	}
}

// point is one durable step of a clearing, counted as the fault plan counts.
type point struct {
	index int
	step  string
}

// killPoints are the durable steps from the lease's grant to the launch's
// leave to go on, from a run of the world with nothing killed.
func killPoints(t *testing.T, s scenario) []point {
	t.Helper()
	w := newWorld(t)
	w.set(s)
	log := filepath.Join(t.TempDir(), "fault.log")
	p := w.launch("r1", join("other:log="+log, holds(s, ""), s.fault()))
	w.follow(p, s, "")
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var points []point
	counted, from := 0, false
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.SplitN(line, " ", 3)
		if len(fields) != 3 || fields[1] == "read" || fields[1] == "note" {
			continue
		}
		counted++
		step := fields[1] + " " + strings.TrimPrefix(fields[2], w.base)
		if from {
			points = append(points, point{counted, step})
		}
		if step == "step leased" {
			from = true
		}
		if step == "step launch cleared" {
			break
		}
	}
	if len(points) == 0 {
		// Refused at admission: the clearing made no durable step to stop at,
		// and the world runs once with nothing killed.
		if !strings.Contains(string(data), "step leased") {
			t.Fatalf("no lease in the log:\n%s", data)
		}
		return []point{{1 << 20, "no step"}}
	}
	return points
}

// crashAt runs a world with its clearing killed at the n-th durable step, a
// second launch beside it in a world of two, and a launch after.
func crashAt(t *testing.T, s scenario, n int) {
	w := newWorld(t)
	w.set(s)
	before := w.unrelated()
	first := w.launch("r1", join("other:crash="+strconv.Itoa(n), holds(s, ""), s.fault()))
	var second *process
	if s.launches == "two" {
		second = w.launch("r2", "other:holdat=launch cleared@PAUSE")
	}
	happened := w.follow(first, s, "")
	if second != nil {
		if end, said := second.end(); end == killed {
			t.Fatalf("the second launch was killed: %s", said)
		}
	}
	w.untouched(before)
	w.lettersKept()
	w.recover(s, happened)
}

// recover runs a launch after the others and requires the state of the
// table: refused while a condition that stays is there, cleared otherwise.
func (w *world) recover(s scenario, happened bool) {
	w.t.Helper()
	end, said := w.launch("r3", "other:holdat=launch cleared@PAUSE").end()
	stays := persists(s) && (s.eventStep() == "" || happened)
	switch {
	case stays && end != refused:
		w.t.Fatalf("the launch after went on beside live state: %s %s", end, said)
	case stays:
		w.lettersKept()
	case end != went:
		w.t.Fatalf("the launch after a quiet world: %s %s", end, said)
	case len(w.trees()) != 0:
		w.t.Fatalf("the launch after a quiet world left %v", w.trees())
	}
}

// TestLegacyClearConcurrentMatrix runs the worlds the matrix chooses with no
// stop and with a pause at each named step of the clearing; in a world of two
// launches, the second starts beside the first, or while the first is paused
// holding the clear lock — where it waits the clear bound and refuses. Each
// first launch ends as the protocol says, and leaves the trees it says.
func TestLegacyClearConcurrentMatrix(t *testing.T) {
	for _, s := range worlds(t, func(stop string) bool { return stop != "kill each" }) {
		t.Run(s.name(), func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			w.set(s)
			before := w.unrelated()
			pause := map[string]string{"pause admitted": "legacy admitted", "pause set aside": "legacy set aside", "pause validated": "legacy validated"}[s.stop]
			first := w.launch("r1", join(holds(s, pause), s.fault()))
			var second *process
			if s.launches == "two" && s.stop == "none" {
				second = w.launch("r2", "other:holdat=launch cleared@PAUSE")
			}
			var end, said string
			happened := w.follow(first, s, pause, func(end_, said_ string) { end, said = end_, said_ })
			w.untouched(before)
			if second != nil {
				end2, said2 := second.end()
				w.lettersKept()
				if _, live := atAdmission[s.old]; live && end2 != refused {
					t.Fatalf("the second launch went on beside live state: %s", said2)
				}
				if expect(s).end == went && s.eventStep() == "" && end2 != went {
					t.Fatalf("the second launch beside a quiet world: %s %s", end2, said2)
				}
				w.recover(s, happened)
				return
			}
			want := expect(s)
			if end != want.end || !strings.Contains(said, want.says) {
				t.Fatalf("the launch %s, saying:\n%s\nwant %s, saying %q", end, said, want.end, want.says)
			}
			if got := w.trees(); strings.Join(got, ",") != strings.Join(want.trees, ",") {
				t.Fatalf("trees %v, want %v; it said:\n%s", got, want.trees, said)
			}
			w.lettersKept()
		})
	}
}

// holds is the fault plan's holds for a world: at its condition's step, at
// a pause, and at the launch's leave to go on, in the order they come.
func holds(s scenario, pause string) string {
	var steps []string
	for _, step := range []string{"legacy admitted", "legacy set aside", "legacy validated"} {
		if step == s.eventStep() || step == pause {
			steps = append(steps, step)
		}
	}
	return "other:holdat=" + strings.Join(append(steps, "launch cleared"), ",") + "@PAUSE"
}

func join(parts ...string) string {
	var kept []string
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, ";")
}

// follow drives a launch of a world to its end: the condition brought about
// at its step, and at a pause in a world of two the second launch run there,
// required to wait the clear bound and refuse. It answers whether the
// condition arrived, and hands the end to report.
func (w *world) follow(p *process, s scenario, pause string, report ...func(string, string)) bool {
	w.t.Helper()
	happened := false
	for {
		step := p.heldAt()
		if step == s.eventStep() && step != "" && !happened {
			happened = w.event(s)
		}
		if step == pause && step != "" && s.launches == "two" {
			started := time.Now()
			end, said := w.launch("r2", "other:holdat=launch cleared@PAUSE").end()
			if end != refused || !strings.Contains(said, "another launch is checking the state of rewake 1.x; launch again") {
				w.t.Fatalf("a launch beside a paused clearing: %s %s", end, said)
			}
			if waited := time.Since(started); waited < clearWait {
				w.t.Fatalf("a launch beside a paused clearing refused after %s, within the clear bound", waited)
			}
		}
		if step == "launch cleared" || step == "" {
			break
		}
		p.resume()
	}
	end, said := p.end()
	for _, r := range report {
		r(end, said)
	}
	return happened
}

// untouched requires what lies beside the trees to be as it was.
func (w *world) untouched(before string) {
	w.t.Helper()
	if got := w.unrelated(); got != before {
		w.t.Fatalf("the clearing changed what lies beside the trees:\n%s\nwas\n%s", got, before)
	}
}

// lettersKept requires every room left in rooms/ or a candidate to hold its
// letter: nothing of a tree is removed before its validation.
func (w *world) lettersKept() {
	w.t.Helper()
	for _, kind := range w.trees() {
		if kind == "validated" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(w.base, map[string]string{"rooms": "rooms", "candidate": ".rooms-1x-candidate-*"}[kind], "*"))
		for _, room := range matches {
			name := filepath.Base(room)
			letter := map[string]string{"old": "m1", "left": "m-left"}[name]
			if letter == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(room, "inbox", "web", letter+".json")); err != nil {
				w.t.Fatalf("room %s of a tree not validated lost its letter: %v", room, err)
			}
		}
	}
}
