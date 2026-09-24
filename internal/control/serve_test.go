package control

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestServeAnswersWhatAskAsks(t *testing.T) {
	dir := prepared(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var seen atomic.Value
	go Serve(ctx, dir, 5*time.Millisecond, func(_ context.Context, r Request) Answer {
		seen.Store(r)
		before, after := int64(900), int64(90)
		return Answer{ID: "someone else's", Outcome: Done, TokensBefore: &before, TokensAfter: &after}
	})
	answer, err := Ask(context.Background(), dir, Request{Action: Compact, From: "lead"}, fast)
	if err != nil {
		t.Fatal(err)
	}
	asked, _ := seen.Load().(Request)
	if answer.Outcome != Done || answer.ID != asked.ID || *answer.TokensBefore != 900 || *answer.TokensAfter != 90 {
		t.Fatalf("answer %+v for %+v", answer, asked)
	}
	if asked.Action != Compact || asked.From != "lead" {
		t.Fatalf("served %+v", asked)
	}
	if names := left(t, dir); len(names) != 0 {
		t.Fatalf("left %v", names)
	}
}

func TestServeCarriesARequestOutOnce(t *testing.T) {
	dir := prepared(t)
	request := Request{ID: NewID(), Action: Interrupt, From: "lead"}
	if err := writeRequest(dir, request); err != nil {
		t.Fatal(err)
	}
	var acted atomic.Int32
	act := func(context.Context, Request) Answer { acted.Add(1); return Answer{Outcome: Done} }
	last := serveOnce(context.Background(), dir, "", act)
	// A later look at the same request, and a fresh server that finds its
	// mark, both leave it alone.
	last = serveOnce(context.Background(), dir, last, act)
	serveOnce(context.Background(), dir, "", act)
	if acted.Load() != 1 || last != request.ID {
		t.Fatalf("acted %d times, last %q", acted.Load(), last)
	}
	if answer, ok := readAnswer(dir, request.ID); !ok || answer.Outcome != Done {
		t.Fatalf("answer %+v, %v", answer, ok)
	}
}

func TestServeLeavesARequestWithdrawnAsItIsMarkedUndone(t *testing.T) {
	for name, move := range map[string]func(dir string){
		"withdrawn": func(dir string) { _ = os.Remove(RequestPath(dir)) },
		"replaced":  func(dir string) { _ = writeRequest(dir, Request{ID: NewID(), Action: Compact, From: "lead"}) },
	} {
		t.Run(name, func(t *testing.T) {
			dir := prepared(t)
			request := Request{ID: NewID(), Action: Compact, From: "lead"}
			if err := writeRequest(dir, request); err != nil {
				t.Fatal(err)
			}
			saved := afterMark
			afterMark = func(dir, _ string) { move(dir) }
			t.Cleanup(func() { afterMark = saved })
			acted := false
			serveOnce(context.Background(), dir, "", func(context.Context, Request) Answer { acted = true; return Answer{Outcome: Done} })
			answer, ok := readAnswer(dir, request.ID)
			if acted || !ok || answer.Outcome != Refused || answer.Reason != Withdrawn {
				t.Fatalf("acted %v, answer %+v", acted, answer)
			}
		})
	}
}

func TestServeTakesNoRequestWithAForeignID(t *testing.T) {
	dir := prepared(t)
	if err := os.WriteFile(RequestPath(dir), []byte(`{"id":"../escape","action":"compact","from":"lead"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	acted := false
	serveOnce(context.Background(), dir, "", func(context.Context, Request) Answer { acted = true; return Answer{} })
	if names := left(t, dir); acted || len(names) != 1 {
		t.Fatalf("acted %v, left %v", acted, names)
	}
}
