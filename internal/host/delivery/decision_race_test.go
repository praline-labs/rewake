package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/registry/registrytest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// decisionLab is api stopped on its report R to web, answering web's
// question: R's letter landed beside a mark that does not read, so whether it
// was written is unknown and only a decision settles it. web is served by
// this process and its delivery server is real.
type decisionLab struct {
	twoSessionLab
	question mail.Message
	report   mail.Message
	server   *Server
}

func newDecisionLab(t *testing.T) decisionLab {
	t.Helper()
	lab := decisionLab{twoSessionLab: newTwoSessionLab(t)}
	web := lab.web.Epoch()
	lab.question = mail.Message{ID: mail.NewID(), From: "web", FromEpoch: web, To: "api", ToEpoch: lab.run, Kind: mail.Question, Text: "is it done?", CreatedAt: time.Now()}
	encoded, _ := json.Marshal(lab.question)
	writeRaw(t, filepath.Join(state.DonePath(lab.dir, "api"), lab.question.ID+".json"), string(encoded))
	read, _ := json.Marshal(mail.Status{State: mail.Read, At: time.Now()})
	writeRaw(t, filepath.Join(state.InboxPath(lab.dir, "api"), lab.question.ID+".status"), string(read))
	if err := mailtest.Owe(lab.dir, "api", lab.run, "web", web, lab.question.ID); err != nil {
		t.Fatal(err)
	}
	// The question was delivered into a conversation a later run of api may
	// resume and take its wait over.
	if err := mailtest.WriteDeliveryThread(lab.dir, "api", lab.question.ID, "thread-1"); err != nil {
		t.Fatal(err)
	}
	waiters, err := mail.ReadWaiters(lab.dir, "api", lab.run)
	if err != nil || len(waiters) != 1 {
		t.Fatalf("the wait: %v %v", waiters, err)
	}
	lab.report = mail.Message{
		ID: mail.ReportID("api", lab.run, waiters[0]), From: "api", FromEpoch: lab.run, To: "web", ToEpoch: web,
		Kind: mail.Finished, Text: "the answer", InReplyTo: []string{lab.question.ID}, CreatedAt: time.Now(),
	}
	if err := mail.WriteJournal(lab.dir, "api", "end", mail.TurnJournal{Epoch: lab.run, Reports: []mail.Message{lab.report}, Clear: waiters}); err != nil {
		t.Fatal(err)
	}
	if err := mail.PutOnce(lab.dir, lab.report); err != nil {
		t.Fatal(err)
	}
	writeRaw(t, mailtest.OncePath(lab.dir, "web", web, lab.report.ID), "damaged")
	if err := lab.reconcile(t); err == nil {
		t.Fatal("the barrier went past R's unknown outcome")
	}
	lab.server = &Server{Dir: lab.dir, Name: "web", Epoch: web, attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, arrivals: map[string]*arrival{}, recent: map[string]recentAnnouncement{}, held: map[string][]mail.Message{}, followed: true}
	return lab
}

// copyOf is the id R goes again under once a decision said it did not
// arrive, "" before.
func (l decisionLab) copyOf() string {
	views, err := mail.Decisions(l.dir, "api")
	if err != nil {
		return ""
	}
	for _, view := range views {
		if view.Report == l.report.ID && view.Decision != nil {
			return view.Decision.Resent()
		}
	}
	return ""
}

// deliver is web's delivery server announcing and settling a letter: it
// becomes readable, and its outcome is recorded and settled in a section.
func (l decisionLab) deliver(id string) {
	_ = withLock(l.dir, "web", func() error { return mail.LinkUnread(l.dir, "web", id) })
	l.server.attempts = map[string]time.Time{}
	l.server.publish(id, mail.Result{State: mail.Delivered})
}

// TestDecisionRacesWithWrapperAndDelivery holds a decision against everything
// that may run beside it (docs/v2/stage3-decision-recovery.md): the person's
// same and opposite choices at once and their retries, api's barrier as its
// turn end retries it, a later run of api adopting the waits of its
// conversation, and web's delivery server settling R and R's copy and
// sweeping. One decision stands and every answer agrees with it; R's copy is
// written once and R never again; every change to a mailbox is made in its
// section; the wait the decision closed is taken over by nobody; and R's
// letter, the proof it landed, stays. A server whose lock cannot be taken
// settles no copy.
func TestDecisionRacesWithWrapperAndDelivery(t *testing.T) {
	t.Run("an unusable lock settles no copy", unusableLockSettlesNoCopy)
	for round := range 4 {
		t.Run(fmt.Sprint("round ", round), func(t *testing.T) {
			lab := newDecisionLab(t)
			subject, err := mail.FindSubject(lab.dir, "api", lab.report.ID)
			if err != nil {
				t.Fatal(err)
			}
			mailboxes := []string{state.InboxPath(lab.dir, "api"), state.InboxPath(lab.dir, "web")}
			var mu sync.Mutex
			var unlocked []string
			written := map[string]int{}
			var adopted []string
			state.Fault = func(op, path string) error {
				if op == state.OpRead || op == state.OpStep || op == state.OpNote {
					return nil
				}
				for _, mailbox := range mailboxes {
					if !strings.HasPrefix(path, mailbox+string(filepath.Separator)) {
						continue
					}
					free := lockFree(filepath.Join(mailbox, ".lock"))
					mu.Lock()
					if free {
						unlocked = append(unlocked, op+" "+path)
					}
					if (op == state.OpWrite || op == state.OpPublish) && filepath.Dir(path) == mailboxes[1] {
						written[strings.TrimSuffix(filepath.Base(path), ".json")]++
					}
					mu.Unlock()
				}
				return nil
			}
			t.Cleanup(func() { state.Fault = nil })
			later := registry.RunEpoch(4194001, 8, registrytest.Boot(t))

			type answer struct {
				choice string
				got    mail.DecideAnswer
				err    error
			}
			answers := make(chan answer, 12)
			var wg sync.WaitGroup
			start := make(chan struct{})
			for i := range 6 {
				choice := []string{mail.ChoiceArrived, mail.ChoiceNotArrived}[(i+round)%2]
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					for range 2 {
						got, err := mail.Decide(context.Background(), lab.dir, mail.DecideRequest{Sender: "api", Report: lab.report.ID, Choice: choice, By: mail.DecidedBy{Kind: mail.ByPerson}, Expect: subject})
						answers <- answer{choice, got, err}
					}
				}()
			}
			actors := []func(){
				func() { _ = lab.reconcile(t) },
				func() {
					_ = withLock(lab.dir, "api", func() error {
						ids, err := mail.AdoptWaits(lab.dir, "api", later, "thread-1")
						mu.Lock()
						adopted = append(adopted, ids...)
						mu.Unlock()
						return err
					})
				},
			}
			// A later run resumes the conversation while api is stopped, and
			// again as the decision goes on.
			actors[1]()
			for _, act := range actors {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					for range 6 {
						act()
					}
				}()
			}
			// web's delivery server is one goroutine, as it is in its process.
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for range 6 {
					lab.deliver(lab.report.ID)
					if copied := lab.copyOf(); copied != "" {
						if _, err := os.Stat(filepath.Join(state.InboxPath(lab.dir, "web"), copied+".json")); err == nil {
							lab.deliver(copied)
						}
					}
					lab.server.sweepFinished()
				}
			}()
			close(start)
			finished := make(chan struct{})
			go func() {
				wg.Wait()
				close(finished)
			}()
			select {
			case <-finished:
			case <-time.After(30 * time.Second):
				t.Fatal("the race did not finish")
			}
			close(answers)
			if err := lab.reconcile(t); err != nil {
				t.Fatalf("the barrier after the race: %v", err)
			}
			state.Fault = nil

			views, err := mail.Decisions(lab.dir, "api")
			if err != nil || len(views) != 1 || views[0].Decision == nil {
				t.Fatalf("the decisions: %+v, %v", views, err)
			}
			d := views[0].Decision
			for a := range answers {
				switch {
				case a.err == nil && a.choice == d.Choice && a.got.Decision.ID == d.ID:
				case a.err != nil && a.choice != d.Choice && outcomeOf(a.err) == mail.DecisionOpposite:
				case a.err != nil && outcomeOf(a.err) == mail.DecisionBusy:
				default:
					t.Errorf("%s answered %+v, %v beside decision %s %s", a.choice, a.got.Decision, a.err, d.ID, d.Choice)
				}
			}
			if len(unlocked) != 0 {
				t.Errorf("changes made outside their mailbox's section: %v", unlocked)
			}
			if n := written[lab.report.ID]; n != 0 {
				t.Errorf("R was written %d times", n)
			}
			copied := d.Resent()
			if d.Choice == mail.ChoiceNotArrived && (copied == "" || written[copied] != 1) {
				t.Errorf("R's copy %q was written %d times", copied, written[copied])
			}
			for id, n := range written {
				if n > 1 && strings.Contains(id, "-") && !strings.Contains(id, ".") {
					t.Errorf("letter %s was written %d times", id, n)
				}
			}
			// Taken over before R's outcome is settled, the question would be
			// owed by a run that never answers it while its answer may be out.
			if waits, err := mail.ReadWaiters(lab.dir, "api", later); err != nil || len(waits) != 0 || len(adopted) != 0 {
				t.Errorf("the later run took over %v; waits %+v, %v", adopted, waits, err)
			}
			if waits, err := mail.ReadWaiters(lab.dir, "api", lab.run); err != nil || len(waits) != 0 {
				t.Errorf("the wait R answered stands: %+v, %v", waits, err)
			}
			stands := false
			for _, stage := range []string{state.InboxPath(lab.dir, "web"), state.UnreadPath(lab.dir, "web"), state.DonePath(lab.dir, "web")} {
				if _, err := os.Stat(filepath.Join(stage, lab.report.ID+".json")); err == nil {
					stands = true
				}
			}
			if !stands {
				t.Error("R's letter was retired")
			}
		})
	}
}

// unusableLockSettlesNoCopy: a delivery server whose lock cannot be taken
// records the outcome of R's copy alone and settles nothing (lockOrAlone):
// the copy's waiting form stays for a pass that holds the lock, which settles
// it.
func unusableLockSettlesNoCopy(t *testing.T) {
	lab := newDecisionLab(t)
	subject, err := mail.FindSubject(lab.dir, "api", lab.report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mail.Decide(context.Background(), lab.dir, mail.DecideRequest{Sender: "api", Report: lab.report.ID, Choice: mail.ChoiceNotArrived, By: mail.DecidedBy{Kind: mail.ByPerson}, Expect: subject}); err != nil {
		t.Fatal(err)
	}
	copied := lab.copyOf()
	waiting := filepath.Join(state.InboxPath(lab.dir, "web"), copied+".json")
	if _, err := os.Stat(waiting); copied == "" || err != nil {
		t.Fatalf("the copy %q: %v", copied, err)
	}
	if err := withLock(lab.dir, "web", func() error { return mail.LinkUnread(lab.dir, "web", copied) }); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(state.InboxPath(lab.dir, "web"), ".lock")
	if err := os.Chmod(lock, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lock, 0o600) })
	lab.server.publish(copied, mail.Result{State: mail.Delivered})
	if status, ok, err := mail.ReadStatus(lab.dir, "web", copied); err != nil || !ok || status.State != mail.Delivered {
		t.Fatalf("the copy's outcome is not on record: %+v, %v, %v", status, ok, err)
	}
	if _, err := os.Stat(waiting); err != nil {
		t.Fatalf("the copy was settled without the lock: %v", err)
	}
	if err := os.Chmod(lock, 0o600); err != nil {
		t.Fatal(err)
	}
	lab.deliver(copied)
	if _, err := os.Stat(waiting); !os.IsNotExist(err) {
		t.Fatalf("a pass holding the lock did not settle the copy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state.UnreadPath(lab.dir, "web"), copied+".json")); err != nil {
		t.Fatalf("the copy's readable form went: %v", err)
	}
}

// lockFree answers whether nobody holds the mailbox lock at path: it can be
// taken exclusive now through a descriptor of its own, let go at once.
func lockFree(path string) bool {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil
}

func outcomeOf(err error) mail.DecisionOutcome {
	var refusal *mail.DecisionError
	if errors.As(err, &refusal) {
		return refusal.Outcome
	}
	return ""
}
