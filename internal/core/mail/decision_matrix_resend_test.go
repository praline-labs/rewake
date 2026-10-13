package mail

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	dm "github.com/praline-labs/rewake/internal/core/mail/decisionmodel"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// playResend plays one world of the resend partition: R, never landed, its
// mark unreadable, is decided "did not arrive" — by the person when r ended
// before it — and the resend R' meets, on its path, the evidence and the end
// of r its values name. An unknown R' is decided in the barrier after it,
// by r while it lives and by the person otherwise; R” then runs as any
// copy does. Every event of the path is placed by the step the product
// reaches, so a cut before it moves the event with the path.
func playResend(t *testing.T, w dm.World, cut matrixCut) []string {
	v := w.Value
	prime, life, second := v("r-prime"), v("r-life"), v("second-choice")
	verified := v("decider") == dm.ByRecipient
	s := newMatrixScene(t, verified)
	s.access.cut, s.access.flush = cut.at, cut.flush
	r := s.report
	s.writeMark(r, "damaged")
	if err := s.barrier(); err == nil {
		t.Fatal("the barrier went past R's unknown outcome")
	}
	if life == dm.REndsBeforeD {
		s.endR()
	}
	path := newResendPath(s, r, prime, life)
	s.access.mu.Lock()
	s.access.beforeOp, s.access.onStat, s.access.onCut = path.before, path.stat, path.cut
	s.access.mu.Unlock()

	alive := v("sender") == dm.SenderAlive
	var answer DecideAnswer
	if err := s.product(func() error {
		var err error
		answer, err = s.decide(r, ChoiceNotArrived, verified)
		return err
	}); err != nil {
		t.Fatalf("the decision was not made: %v", err)
	}
	if alive || v("sender") == dm.SenderEndsAfter {
		_ = s.product(s.barrier)
	}
	if life == dm.REndsUnknown {
		s.endR()
	}
	prime2 := path.copied
	if second != dm.None {
		// The person decides once r ended; r, while it lives.
		byR := verified && !s.r.ended
		copied := Message{ID: prime2, To: "web", ToEpoch: r.ToEpoch}
		if err := s.product(func() error {
			_, err := s.decide(copied, matrixChoice(second), byR)
			return err
		}); err != nil {
			t.Fatalf("the decision on R' was not made: %v", err)
		}
	}
	if alive {
		_ = s.product(s.barrier)
	}
	s.recoverAll(alive, r, prime2, second, verified)
	if prime2 == "" {
		t.Fatal("the resend never ran")
	}

	path.checkPlaced(t)
	s.checkResend(t, w, answer.Decision.ID, path)
	return s.access.steps()
}

// checkPlaced fails a world whose path never met the point its values place
// an event on: such a world would pass while playing another.
func (p *resendPath) checkPlaced(t *testing.T) {
	t.Helper()
	want := []string{"installed"}
	if event, ok := map[string]string{
		dm.REndsInSection: "in-section", dm.REndsToIntent: "to-intent", dm.REndsToMark: "to-mark",
		dm.REndsToSave: "to-save", dm.REndsToTail: "to-tail",
	}[p.life]; ok {
		want = append(want, event)
	}
	if p.prime == dm.UnknownMarkLanded || p.prime == dm.UnknownMarkNot {
		want = append(want, "mark-unknown")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, event := range want {
		if !p.once[event] {
			t.Errorf("the path never reached %s", event)
		}
	}
}

// resendPath places a resend world's events on R'’s path.
type resendPath struct {
	s           *matrixScene
	r           Message
	prime, life string
	once        map[string]bool
	mu          sync.Mutex
	copied      string
	letter      bool
	// mootByCut is a cut that fired before R' was written, with r already
	// ended: the attempt it stopped wrote nothing, and the next reads r
	// ended, so R' is moot whatever its truth.
	mootByCut bool
}

func newResendPath(s *matrixScene, r Message, prime, life string) *resendPath {
	return &resendPath{s: s, r: r, prime: prime, life: life, once: map[string]bool{}}
}

// first says, once, that the event named happens now.
func (p *resendPath) first(event string) bool {
	if p.once[event] {
		return false
	}
	p.once[event] = true
	return true
}

func (p *resendPath) markPath() string {
	path, _ := oncePath(p.s.dir, "web", p.r.ToEpoch, p.copied)
	return path
}

func (p *resendPath) letterIn(path string) bool {
	return p.copied != "" && filepath.Base(path) == p.copied+".json" && filepath.Dir(path) == state.InboxPath(p.s.dir, "web")
}

// before is told each durable step of the product: the installation of R'
// sets what R'’s mailbox holds before its first attempt, and r ends at the
// point of the path its value names.
func (p *resendPath) before(op, path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	sep := string(filepath.Separator)
	if op == "publish" && strings.Contains(path, sep+"journal"+sep+"decided-") && p.first("installed") {
		p.copied = p.s.resentOf(p.r)
		p.setUp()
		if p.life == dm.REndsBeforeLive {
			p.s.endR()
		}
		return nil
	}
	if p.copied == "" {
		return nil
	}
	isMark := op == "write" && path == p.markPath()
	isLetter := op == "write" && p.letterIn(path)
	if (isMark || isLetter) && p.life == dm.REndsToIntent && p.first("to-intent") {
		p.s.endR()
	}
	if isMark && p.letter && p.life == dm.REndsToMark && p.first("to-mark") {
		p.s.endR()
	}
	if op == "write" && p.letter && strings.Contains(path, sep+"journal"+sep+"decided-") && p.life == dm.REndsToSave && p.first("to-save") {
		p.s.endR()
	}
	if op == "rename" && strings.Contains(path, sep+"journal"+sep+"decided-") && strings.HasSuffix(path, doneSuffix) && p.life == dm.REndsToTail && p.first("to-tail") {
		p.s.endR()
	}
	// An unknown at the published mark: the mark turns unreadable as the
	// attempt goes to write the letter, or the mark after it, and that write
	// fails.
	if p.prime == dm.UnknownMarkLanded && isMark && p.letter || p.prime == dm.UnknownMarkNot && isLetter {
		if p.first("mark-unknown") {
			p.s.writeMark(Message{ID: p.copied, To: "web", ToEpoch: p.r.ToEpoch}, "damaged")
			return &fs.PathError{Op: op, Path: path, Err: syscall.EIO}
		}
	}
	if isLetter {
		p.letter = true
	}
	return nil
}

// cut notes a cut that leaves R' moot.
func (p *resendPath) cut() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mootByCut = p.s.r.ended && (p.copied == "" || p.s.access.written(p.copied) == 0)
}

// stat ends r inside the section, after the attempt read r live and before
// it reads R'’s evidence.
func (p *resendPath) stat(path string, plan bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !plan && p.letterIn(path) && p.life == dm.REndsInSection && p.first("in-section") {
		p.s.endR()
	}
}

// setUp leaves R'’s mailbox as the world's evidence says before its first
// attempt: an intent with no letter; a mark that does not read, beside the
// letter or not; or an intent, beside the letter or not, in a mailbox that
// cannot be searched for it.
func (p *resendPath) setUp() {
	copied := Message{ID: p.copied, From: "api", FromEpoch: p.s.epoch, To: "web", ToEpoch: p.r.ToEpoch, Kind: Finished, Text: "the answer", Resends: p.r.ID}
	switch p.prime {
	case dm.AbsentThenPublished:
		p.s.writeMark(copied, onceIntent)
	case dm.UnknownIntentLanded, dm.UnknownIntentNot:
		if p.prime == dm.UnknownIntentLanded {
			p.s.landDirect(copied)
		}
		p.s.writeMark(copied, "damaged")
	case dm.UnknownLetterLanded, dm.UnknownLetterNot:
		if p.prime == dm.UnknownLetterLanded {
			p.s.landDirect(copied)
		}
		p.s.writeMark(copied, onceIntent)
		p.s.faultLetter(p.copied)
	}
}

// resentOf is the id of the copy the decision on report sends, read from
// the decision itself.
func (s *matrixScene) resentOf(report Message) string {
	raw, err := os.ReadFile(DecisionPath(s.dir, "api", report.ID))
	if err != nil {
		return ""
	}
	var d Decision
	if json.Unmarshal(raw, &d) != nil {
		return ""
	}
	resend, err := d.resendJournal()
	if err != nil || len(resend.Reports) != 1 {
		return ""
	}
	return resend.Reports[0].ID
}

// landDirect writes a copy into web's mailbox as an earlier attempt did, by
// no step the matrix cuts, and counts it landed.
func (s *matrixScene) landDirect(copied Message) {
	if err := (world{dir: s.dir, files: osAccess{}}).put(copied); err != nil {
		s.t.Error(err)
		return
	}
	s.access.mu.Lock()
	s.access.letters[copied.ID]++
	s.access.mu.Unlock()
}

// faultLetter makes every stat of one letter's places fail, in every pass.
func (s *matrixScene) faultLetter(id string) {
	stages := []string{state.InboxPath(s.dir, "web"), state.UnreadPath(s.dir, "web"), state.DonePath(s.dir, "web")}
	s.access.mu.Lock()
	defer s.access.mu.Unlock()
	s.access.stat = func(path string, _ bool) (fs.FileInfo, error) {
		for _, stage := range stages {
			if filepath.Dir(path) == stage && filepath.Base(path) == id+".json" {
				return nil, &fs.PathError{Op: "stat", Path: path, Err: syscall.EIO}
			}
		}
		return nil, nil
	}
}

// recoverAll is the recovery of a resend world: the sender's barriers while
// it lives, or else the same words of the last deciding command.
func (s *matrixScene) recoverAll(alive bool, r Message, copied, second string, verified bool) {
	if alive {
		for range 2 {
			_ = s.product(s.barrier)
		}
		return
	}
	_ = s.product(func() error {
		_, err := s.decide(r, ChoiceNotArrived, verified)
		return err
	})
	if second != dm.None {
		_ = s.product(func() error {
			_, err := s.decide(Message{ID: copied, To: "web", ToEpoch: r.ToEpoch}, matrixChoice(second), verified && !s.r.ended)
			return err
		})
	}
}

// checkResend holds a resend world to the model: R never written; R' landed
// as its truth says, at most once; R” for a second "did not arrive", landed
// when r lives to read it; the landings the model predicts; the waits
// cleared and nothing left open; and each decision's notes.
func (s *matrixScene) checkResend(t *testing.T, w dm.World, first string, path *resendPath) {
	t.Helper()
	v := w.Value
	prime, life, second := v("r-prime"), v("r-life"), v("second-choice")
	verified := v("decider") == dm.ByRecipient
	r := s.report
	o := s.outcome(t, r)
	if o.decisions != 1 || o.decision == nil || o.decision.ID != first || o.decision.Choice != ChoiceNotArrived {
		t.Fatalf("the decision on R: %d, %+v", o.decisions, o.decision)
	}
	if n := s.access.written(r.ID); n != 0 {
		t.Errorf("R was written %d times", n)
	}
	landed := map[string]bool{
		dm.NoneThenPublished: true, dm.AbsentThenPublished: true,
		dm.UnknownIntentLanded: true, dm.UnknownLetterLanded: true, dm.UnknownMarkLanded: true,
	}[prime] && (!path.mootByCut || s.access.written(path.copied) != 0)
	if n := s.access.written(path.copied); n > 1 || (n == 1) != landed {
		t.Errorf("R' was written %d times; its truth lands it %v", n, landed)
	}
	chain := []dm.Copy{{Decided: dm.NotArrived}, {Landed: landed, Decided: second}}
	if second == dm.None {
		chain[1].Decided = ""
	}
	landings := s.access.written(path.copied)
	if second != dm.None {
		inner := s.outcome(t, Message{ID: path.copied})
		if inner.decisions != 1 || inner.decision.Choice != matrixChoice(second) {
			t.Fatalf("the decision on R': %d, %+v", inner.decisions, inner.decision)
		}
		byR := verified && life == dm.RAlive
		ended := s.endedFate()
		if life == dm.RAlive {
			ended = ToldWritten
		}
		s.checkNotes(t, *inner.decision, byR, ended)
		if second == dm.NotArrived {
			again := life == dm.RAlive
			if n := s.access.written(inner.resent); n > 1 || (n == 1) != again {
				t.Errorf("R'' was written %d times; r lives to read it: %v", n, again)
			}
			chain = append(chain, dm.Copy{Landed: again})
			landings += s.access.written(inner.resent)
		}
	}
	if want := dm.Expect(chain); landings != want.Landings {
		t.Errorf("%d landings; the model says %+v", landings, want)
	}
	if o.waits != 0 || o.closure == nil || o.closure.Decision != first || len(o.open) != 0 {
		t.Errorf("waits %d, closure %+v, open %v", o.waits, o.closure, o.open)
	}
	fate := ToldWritten
	switch {
	case life == dm.REndsBeforeD || life == dm.REndsBeforeLive:
		fate = s.endedFate()
	case s.access.cut > 0 && life != dm.RAlive && life != dm.REndsUnknown:
		// r ends on J”s path, in the barrier that tells the notes; a cut
		// before them moves them past that end.
		fate = toldEither
	}
	s.checkNotes(t, *o.decision, verified, fate)
}

// endedFate is what a note to r comes to once r ended: told run-ended on the
// proof of its process gone, and owed when r is this process, whose record
// went but whose process lives — its end is not proven (Q11).
func (s *matrixScene) endedFate() string {
	if s.r.child == nil {
		return ""
	}
	return ToldEnded
}
