package mail

import (
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	dm "github.com/praline-labs/rewake/internal/core/mail/decisionmodel"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// playGenerations plays one world of the generations partition: R, landed or
// not, turns unknown at its mark; each copy after it turns unknown at its
// mark as it is first tried — written before or not, as the world's truth
// for it says — and r, verified and alive, decides each before the next runs
// in the admitted barrier after the decision; the copy after the last
// decision publishes on its first attempt.
func playGenerations(t *testing.T, w dm.World, cut matrixCut) []string {
	gens := generationsOf(w.Value("generations"))
	s := newMatrixScene(t, true)
	s.access.cut, s.access.flush = cut.at, cut.flush
	r := s.report
	if w.Value("truth") == dm.Landed {
		s.land(r, "damaged")
	} else {
		s.writeMark(r, "damaged")
	}
	s.initialLanding = s.access.written(r.ID)
	if err := s.barrier(); err == nil {
		t.Fatal("the barrier went past R's unknown outcome")
	}
	chain := &copyChain{s: s, parent: r}
	s.access.mu.Lock()
	s.access.beforeOp = chain.before
	s.access.mu.Unlock()

	copies := []Message{r}
	for i, choice := range gens.choices {
		current := copies[i]
		chain.next(current, i+1 < len(gens.choices), gens.landed(i+1))
		if err := s.product(func() error {
			_, err := s.decide(current, choice, true)
			return err
		}); err != nil {
			t.Fatalf("the decision on copy %d was not made: %v", i, err)
		}
		if choice == ChoiceNotArrived {
			copies = append(copies, Message{ID: s.resentOf(current), To: "web", ToEpoch: r.ToEpoch})
		}
	}
	for range 2 {
		_ = s.product(s.barrier)
	}
	s.checkGenerations(t, w, gens, copies)
	return s.access.steps()
}

// generations is a world's chain: the choice on each copy decided, and the
// truth of each copy after R that turns unknown.
type generations struct {
	choices []string
	truths  []bool
}

// landed is the truth of copy i, R being 0; the copy after the last decided
// publishes, and lands.
func (g generations) landed(i int) bool {
	if i-1 < len(g.truths) {
		return g.truths[i-1]
	}
	return true
}

func generationsOf(value string) generations {
	switch value {
	case dm.OneGeneration:
		return generations{choices: []string{ChoiceNotArrived}}
	case dm.ChainOfForty:
		g := generations{}
		for i := range 40 {
			g.choices = append(g.choices, ChoiceNotArrived)
			if i > 0 {
				g.truths = append(g.truths, false)
			}
		}
		return g
	}
	landed := strings.HasPrefix(value, "two-landed-")
	second := ChoiceArrived
	if strings.HasSuffix(value, "-not-arrived") {
		second = ChoiceNotArrived
	}
	return generations{choices: []string{ChoiceNotArrived, second}, truths: []bool{landed}}
}

// copyChain turns each copy unknown at its mark as its resend is installed:
// one that lands, at the published mark after its letter; one that does
// not, at its letter. The mark turns unreadable and the write fails, as a
// fault of the file system would.
type copyChain struct {
	s       *matrixScene
	mu      sync.Mutex
	parent  Message
	unknown bool
	landed  bool
	target  string
	letter  bool
	fired   bool
}

// next arms the chain for the copy the decision on parent sends.
func (c *copyChain) next(parent Message, unknown, landed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.parent, c.unknown, c.landed = parent, unknown, landed
	c.target, c.letter, c.fired = "", false, false
}

func (c *copyChain) before(op, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	sep := string(filepath.Separator)
	if c.target == "" && op == "publish" && strings.Contains(path, sep+"journal"+sep+"decided-") {
		c.target = c.s.resentOf(c.parent)
		return nil
	}
	if !c.unknown || c.target == "" || c.fired || op != "write" {
		return nil
	}
	copied := Message{ID: c.target, To: "web", ToEpoch: c.parent.ToEpoch}
	isMark := path == c.s.markPath(copied)
	isLetter := path == filepath.Join(state.InboxPath(c.s.dir, "web"), c.target+".json")
	if isLetter {
		c.letter = true
	}
	if c.landed && isMark && c.letter || !c.landed && isLetter {
		c.fired = true
		c.s.writeMark(copied, "damaged")
		return &fs.PathError{Op: op, Path: path, Err: syscall.EIO}
	}
	return nil
}

// checkGenerations holds a chain to the model: each copy written at most
// once and as its truth says, the landings the model predicts, every wait
// cleared, nothing open, and each decision's note to main written.
func (s *matrixScene) checkGenerations(t *testing.T, w dm.World, g generations, copies []Message) {
	t.Helper()
	want := len(g.choices)
	if g.choices[want-1] == ChoiceNotArrived {
		want++
	}
	if len(copies) != want {
		t.Fatalf("%d copies for %d decisions", len(copies), len(g.choices))
	}
	var chain []dm.Copy
	landings := 0
	for i, copied := range copies {
		landed := i == 0 && w.Value("truth") == dm.Landed || i > 0 && g.landed(i)
		n := s.access.written(copied.ID)
		if n > 1 || (n == 1) != landed {
			t.Errorf("copy %d was written %d times; its truth lands it %v", i, n, landed)
		}
		landings += n
		c := dm.Copy{Landed: landed}
		if i < len(g.choices) {
			c.Decided = matrixModelChoice(g.choices[i])
			o := s.outcome(t, copied)
			if o.decisions != 1 {
				t.Fatalf("%d decisions on copy %d", o.decisions, i)
			}
			s.checkNotes(t, *o.decision, true, "")
		}
		chain = append(chain, c)
	}
	if want := dm.Expect(chain); landings != want.Landings {
		t.Errorf("%d landings; the model says %+v", landings, want)
	}
	o := s.outcome(t, s.report)
	if o.waits != 0 || o.closure == nil || len(o.open) != 0 {
		t.Errorf("waits %d, closure %+v, open %v", o.waits, o.closure, o.open)
	}
}
