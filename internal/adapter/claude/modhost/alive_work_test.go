package modhost

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// The heartbeat tells the load the run's room work: a poll that shows what
// the run has is held, one that shows something else is answered at once, a
// change while a poll is held answers it within a reread, and a poll that
// names no work, a load that shows none, waits out the hold.
func TestTheHeartbeatAnswersTheRunsWorkAsItChanges(t *testing.T) {
	var shown atomic.Value
	shown.Store("brief 57 1/6")
	_, socket := servingWith(t, func(s *Server) {
		s.cfg.Hold, s.cfg.WorkEvery = 2*time.Second, 20*time.Millisecond
		s.cfg.Work = func() string { return shown.Load().(string) }
	})
	_, generation, _ := sayHello(t, socket, "load", 0)
	ask := func(work *string) (AliveAnswer, time.Duration) {
		t.Helper()
		data, _ := json.Marshal(AliveRequest{LoadID: "load", Generation: generation, Work: work})
		began := time.Now()
		status, text, err := post(socket, PathModAlive, data)
		if err != nil || status != http.StatusOK {
			t.Fatalf("poll: %d %s %v", status, text, err)
		}
		var answer AliveAnswer
		if err := json.Unmarshal([]byte(text), &answer); err != nil {
			t.Fatal(err)
		}
		return answer, time.Since(began)
	}
	empty := ""
	if answer, took := ask(&empty); answer.Work == nil || *answer.Work != "brief 57 1/6" || took > time.Second {
		t.Fatalf("a load that shows other work: %+v after %v", answer, took)
	}
	current := "brief 57 1/6"
	go func() {
		time.Sleep(200 * time.Millisecond)
		shown.Store("brief 57 2/6")
	}()
	if answer, took := ask(&current); answer.Work == nil || *answer.Work != "brief 57 2/6" || took < 150*time.Millisecond || took > time.Second {
		t.Fatalf("a tick while held: %+v after %v", answer, took)
	}
	if answer, took := ask(nil); answer.Work == nil || *answer.Work != "brief 57 2/6" || took < 1500*time.Millisecond {
		t.Fatalf("a load that names no work: %+v after %v", answer, took)
	}
}
