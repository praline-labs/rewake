package channel

import (
	"strings"
	"testing"
	"time"
)

func failing(at time.Duration) Record {
	r := New(Codex, true, "", stampAt(0))
	r.Fold(Event{Kind: Hello, Generation: 1, At: stampAt(at - time.Second)})
	r.Fold(Event{Kind: Closed, Generation: 1, Alive: true, At: stampAt(at)})
	return r
}

// One notice per key per ten minutes, and the current category goes once
// its window ends if it differs from the last one told.
func TestTheKeyWindowHoldsARepeatUntilItEnds(t *testing.T) {
	r := failing(time.Second)
	var n Notices
	main := Recipient{Role: ToMain, Name: "m", Epoch: "1"}
	first, ok := n.Plan(&r, main, stampAt(2*time.Second))
	if !ok {
		t.Fatal("the failure was not told")
	}
	n.Settle(first.Seq, Landed, stampAt(2*time.Second).Boot)
	r.Fold(Event{Kind: Validated, At: stampAt(3 * time.Second), Issued: stampAt(3 * time.Second).Boot})
	if p, ok := n.Plan(&r, main, stampAt(4*time.Second)); !ok || !strings.HasPrefix(p.Body, FirstMainWorks) {
		t.Fatalf("recovery not told: %+v", p)
	} else {
		n.Settle(p.Seq, Landed, stampAt(4*time.Second).Boot)
	}
	r.Fold(Event{Kind: NotObserved, At: stampAt(5 * time.Second)})
	r.Fold(Event{Kind: Validated, At: stampAt(6 * time.Second), Issued: stampAt(6 * time.Second).Boot})
	r.Fold(Event{Kind: Closed, Generation: 1, Alive: true, At: stampAt(7 * time.Second)})
	r.Fold(Event{Kind: Hello, Generation: 2, At: stampAt(8 * time.Second)})
	r.Fold(Event{Kind: Closed, Generation: 2, Alive: true, At: stampAt(9 * time.Second)})
	if _, ok := n.Plan(&r, main, stampAt(10*time.Second)); ok {
		t.Fatal("the same key was told again within its window")
	}
	p, ok := n.Plan(&r, main, stampAt(2*time.Second+KeyWindow))
	if !ok || !strings.HasPrefix(p.Body, FirstMainFailing) {
		t.Fatalf("the suppressed failure was not told when its window ended: %+v", p)
	}
}

func TestNoMoreThanSixAnHourToOneRecipient(t *testing.T) {
	var n Notices
	main := Recipient{Role: ToMain, Name: "m", Epoch: "1"}
	classes := []Kind{NotObserved, CannotStart, TimerPassed}
	landed := 0
	for i := 0; i < 20; i++ {
		at := time.Duration(i) * time.Minute
		r := New(Codex, true, "", stampAt(0))
		switch i % 2 {
		case 0:
			r.Fold(Event{Kind: classes[(i/2)%len(classes)], At: stampAt(at)})
		default:
			r.Fold(Event{Kind: Validated, At: stampAt(at), Issued: stampAt(at).Boot})
		}
		if p, ok := n.Plan(&r, main, stampAt(at)); ok {
			n.Settle(p.Seq, Landed, stampAt(at).Boot)
			landed++
		}
	}
	if landed > HourLimit {
		t.Fatalf("%d notices landed within twenty minutes", landed)
	}
}

// A main of a new epoch gets the current category once under its own
// number, unless it is "tool" and nothing was told before.
func TestANewMainGetsTheCurrentCategory(t *testing.T) {
	r := failing(time.Second)
	var n Notices
	old := Recipient{Role: ToMain, Name: "m", Epoch: "1"}
	fresh := Recipient{Role: ToMain, Name: "m", Epoch: "2"}
	a, _ := n.Plan(&r, old, stampAt(2*time.Second))
	b, ok := n.Plan(&r, fresh, stampAt(3*time.Second))
	if !ok || a.Seq == b.Seq || a.ID("w", "e") == b.ID("w", "e") {
		t.Fatalf("the new epoch was not told under its own number: %+v %+v", a, b)
	}
	working := New(Claude, true, "", stampAt(0))
	working.Fold(Event{Kind: Validated, At: stampAt(time.Second), Issued: stampAt(time.Second).Boot})
	if _, ok := n.Plan(&working, Recipient{Role: ToMain, Name: "m", Epoch: "3"}, stampAt(4*time.Second)); ok {
		t.Fatal("a new main was told the tool works with nothing told before")
	}
}

// A publication's identity is fixed: the same number gives the same ID,
// and a dropped one is never taken up again.
func TestAPublicationKeepsItsIdentityAndADropStays(t *testing.T) {
	r := failing(time.Second)
	var n Notices
	worker := Recipient{Role: ToWorker, Name: "w", Epoch: "1"}
	p, ok := n.Plan(&r, worker, stampAt(2*time.Second))
	if !ok || !p.Advice {
		t.Fatal("the worker's shell advice was not fixed")
	}
	if p.ID("w", "1") != n.Unsent()[0].ID("w", "1") {
		t.Fatal("the retried publication has another ID")
	}
	n.Settle(p.Seq, Dropped, 0)
	if len(n.Unsent()) != 0 {
		t.Fatal("a dropped notice is still to send")
	}
	if _, ok := n.Plan(&r, worker, stampAt(time.Hour)); ok {
		t.Fatal("the dropped notice's key was told again")
	}
}
