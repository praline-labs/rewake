package mail

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// The final evidence is read in the recipient's section and the section is
// kept until D is published (docs/v2/stage3-decision-recovery.md, the
// decision's record): r's read, its sweep or evidence returning wait for the
// commit when they come after the read, and D stands; when they come first,
// decide reads what they left and refuses it as proven. The read cleans up
// after no ended run. A decision on a report to the sender's own name never
// takes its lock twice, and two mailboxes deciding on each other's reports
// at once both finish, bounded.
func TestDecisionFinalEvidenceIsLockedThroughCommit(t *testing.T) {
	activities := map[string]func(s *matrixScene, r Message){
		"evidence returns": func(s *matrixScene, r Message) { s.writeMark(r, onceIntent) },
		"r reads":          func(s *matrixScene, r Message) { s.readR(r) },
		"r sweeps": func(s *matrixScene, _ Message) {
			SweepFinishedLocked(s.dir, "web", s.r.epoch())
		},
	}
	for name, activity := range activities {
		t.Run(name+" after the final read waits for the commit", func(t *testing.T) {
			t.Parallel()
			s, r := newContractScene(t)
			s.land(r, "damaged")
			atCommit, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			s.access.mu.Lock()
			s.access.beforeOp = func(op, path string) error {
				if op == "publish" && path == DecisionPath(s.dir, "api", r.ID) {
					once.Do(func() {
						close(atCommit)
						<-release
					})
				}
				return nil
			}
			s.access.mu.Unlock()
			decided := make(chan error, 1)
			go func() {
				_, err := s.decide(r, ChoiceArrived, false)
				decided <- err
			}()
			waitFor(t, atCommit, "the commit")
			entered, done := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(done)
				_ = state.WithMailboxLock(context.Background(), s.dir, "web", func() error {
					close(entered)
					activity(s, r)
					return nil
				})
			}()
			select {
			case <-entered:
				t.Fatal("r's mailbox was entered between the final read and the commit")
			case <-time.After(300 * time.Millisecond):
			}
			close(release)
			if err := waitErr(t, decided, "the decision"); err != nil {
				t.Fatalf("the decision: %v", err)
			}
			waitFor(t, done, "r's activity")
			first, err := os.ReadFile(DecisionPath(s.dir, "api", r.ID))
			if err != nil {
				t.Fatal(err)
			}
			_ = s.barrier()
			if again, _ := os.ReadFile(DecisionPath(s.dir, "api", r.ID)); string(again) != string(first) {
				t.Error("evidence after D changed it")
			}
			if n := s.access.written(r.ID); n != 1 {
				t.Errorf("R was written %d times", n)
			}
		})

		t.Run(name+" before the final read is what decide reads", func(t *testing.T) {
			t.Parallel()
			s, r := newContractScene(t)
			s.land(r, "damaged")
			holding, proceed := make(chan struct{}), make(chan struct{})
			go func() {
				_ = state.WithMailboxLock(context.Background(), s.dir, "web", func() error {
					close(holding)
					<-proceed
					// The mark read again, as the activity leaves it.
					s.writeMark(r, oncePublished)
					activity(s, r)
					return nil
				})
			}()
			waitFor(t, holding, "r's mailbox lock")
			decided := make(chan error, 1)
			go func() {
				_, err := s.decide(r, ChoiceArrived, false)
				decided <- err
			}()
			time.Sleep(100 * time.Millisecond) //nolint:forbidigo // let decide reach the recipient's lock
			close(proceed)
			err := waitErr(t, decided, "the decision")
			if outcomeOf(err) != DecisionProven || fileExists(DecisionPath(s.dir, "api", r.ID)) {
				t.Fatalf("decide after the evidence returned answered %v", err)
			}
		})
	}

	t.Run("an ended run's record stays", func(t *testing.T) {
		t.Parallel()
		s, r := newContractScene(t)
		s.endR()
		if _, err := s.decide(r, ChoiceArrived, false); err != nil {
			t.Fatalf("the decision once r ended: %v", err)
		}
		// The final read is made in a mailbox section: a lookup that cleaned
		// up after r there would take r's name's lock under it.
		if _, err := os.Lstat(state.SessionPath(s.dir, "web")); err != nil {
			t.Errorf("the decision removed r's record: %v", err)
		}
	})

	t.Run("a report to the sender's own name", func(t *testing.T) {
		t.Parallel()
		s := newMatrixScene(t, false)
		api := startRoleRun(t, s.dir, "api", "")
		failure := s.journalOf("fail", api.session.Epoch(), Message{To: "api", ToEpoch: api.session.Epoch(), Kind: Error, Text: "the turn failed"})
		s.writeMark(failure, onceIntent)
		s.placeHoldsNoLetter([2]string{"api", failure.ID})
		for range 2 {
			_ = s.barrier()
		}
		decided := make(chan error, 1)
		go func() {
			_, err := s.decide(failure, ChoiceArrived, false)
			decided <- err
		}()
		if err := waitErr(t, decided, "the decision on a report to self"); err != nil {
			t.Fatalf("the decision on a report to self: %v", err)
		}
		if !fileExists(DecisionPath(s.dir, "api", failure.ID)) {
			t.Error("no decision on the report to self")
		}
	})

	t.Run("opposing mailboxes", func(t *testing.T) {
		t.Parallel()
		// An unreadable mark in a mailbox stops it as a record before any
		// step, so each report's unknown is its letter's place instead.
		s := newMatrixScene(t, false)
		r := s.report
		api := startRoleRun(t, s.dir, "api", "")
		back := Message{ID: NewID(), From: "web", FromEpoch: s.r.epoch(), To: "api", ToEpoch: api.session.Epoch(), Kind: Finished, Text: "web's answer", CreatedAt: time.Now()}
		if err := WriteJournal(s.dir, "web", "end", TurnJournal{Epoch: s.r.epoch(), Reports: []Message{back}}); err != nil {
			t.Fatal(err)
		}
		s.writeMark(r, onceIntent)
		s.writeMark(back, onceIntent)
		s.placeHoldsNoLetter([2]string{"web", r.ID}, [2]string{"api", back.ID})
		if err := s.barrier(); err == nil {
			t.Fatal("api's barrier went past R's unknown outcome")
		}
		webBarrier := func() error {
			return state.WithMailboxLock(context.Background(), s.dir, "web", func() error {
				return Reconcile(context.Background(), s.dir, "web")
			})
		}
		if err := webBarrier(); err == nil {
			t.Fatal("web's barrier went past its report's unknown outcome")
		}
		subject, err := FindSubject(s.dir, "web", back.ID)
		if err != nil {
			t.Fatal(err)
		}
		sides := []struct{ sender, report string }{{"api", r.ID}, {"web", back.ID}}
		// api's decision, holding its own mailbox, waits at web's for web's
		// decision to take web's own and come for api's: each holds the lock
		// the other asks for. The first lock asked of web's mailbox is api's
		// decision coming for it, the second of api's is web's decision.
		apiAt, webAt := make(chan struct{}), make(chan struct{})
		asked := map[string]int{}
		s.access.mu.Lock()
		s.access.beforeLock = func(mailbox string) {
			s.access.mu.Lock()
			asked[mailbox]++
			n := asked[mailbox]
			s.access.mu.Unlock()
			switch {
			case mailbox == state.InboxPath(s.dir, "web") && n == 1:
				close(apiAt)
				select {
				case <-webAt:
				case <-time.After(time.Second):
				}
			case mailbox == state.InboxPath(s.dir, "api") && n == 2:
				close(webAt)
			}
		}
		s.access.mu.Unlock()
		type answer struct {
			side int
			err  error
		}
		answers := make(chan answer, 2)
		go func() {
			_, err := s.decide(r, ChoiceArrived, false)
			answers <- answer{0, err}
		}()
		go func() {
			<-apiAt
			_, err := Decide(context.Background(), s.dir, DecideRequest{Sender: "web", Report: back.ID, Choice: ChoiceArrived, By: DecidedBy{Kind: ByPerson}, Expect: subject})
			answers <- answer{1, err}
		}()
		bound := time.After(2*senderLockWait + 2*recipientLockWait)
		busy := map[int]bool{}
		for range 2 {
			select {
			case a := <-answers:
				switch {
				case a.err == nil:
				case outcomeOf(a.err) == DecisionBusy:
					busy[a.side] = true
				default:
					t.Errorf("%s's decision across opposing mailboxes answered %v", sides[a.side].sender, a.err)
				}
			case <-bound:
				t.Fatal("two mailboxes deciding on each other's reports did not finish")
			}
		}
		for i, side := range sides {
			if fileExists(DecisionPath(s.dir, side.sender, side.report)) == busy[i] {
				t.Errorf("%s's decision answered busy %v beside its record", side.sender, busy[i])
			}
			if !busy[i] {
				continue
			}
			subject, err := FindSubject(s.dir, side.sender, side.report)
			if err == nil {
				_, err = Decide(context.Background(), s.dir, DecideRequest{Sender: side.sender, Report: side.report, Choice: ChoiceArrived, By: DecidedBy{Kind: ByPerson}, Expect: subject})
			}
			if err != nil {
				t.Errorf("deciding %s's report alone afterwards: %v", side.sender, err)
			}
		}
	})
}

// placeHoldsNoLetter has the place of each letter, a mailbox and an id, hold
// what is no letter, seen only by the pass that writes: an unknown only an
// effect meets, whose occurrence names no path the decision reads.
func (s *matrixScene) placeHoldsNoLetter(letters ...[2]string) {
	s.access.mu.Lock()
	defer s.access.mu.Unlock()
	s.access.stat = func(path string, plan bool) (fs.FileInfo, error) {
		for _, l := range letters {
			if !plan && filepath.Base(path) == l[1]+".json" && filepath.Dir(path) == state.InboxPath(s.dir, l[0]) {
				return notRegular{filepath.Base(path)}, nil
			}
		}
		return nil, nil
	}
}
