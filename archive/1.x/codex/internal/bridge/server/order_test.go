package server_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// A turn's end captured while an acknowledgment writes waits for it and takes
// the snapshot it closes with: the read it commits is inside the end's
// boundary, whatever happens to the mailbox after.
func TestAnEndCapturedDuringAnAcknowledgmentIncludesIt(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	r.start()
	id := r.letter("the letter of the order test")
	r.nextTurn()
	c := r.call("inbox")

	entered, release := make(chan struct{}), make(chan struct{})
	// Released at the latest when the test ends, so a test that fails
	// while the acknowledgment is held does not hang in its cleanup.
	free := sync.OnceFunc(func() { close(release) })
	t.Cleanup(free)
	var once sync.Once
	r.plan(&wrapperPlan{pause: func(op, _ string) {
		// The endpoint's steps are no write of the acknowledgment's.
		if op == state.OpRead || op == state.OpStep {
			return
		}
		once.Do(func() {
			close(entered)
			<-release
		})
	}})
	acknowledged := make(chan error, 1)
	go func() { acknowledged <- r.complete(c, true) }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the acknowledgment never began writing")
	}
	captured := make(chan *inbox.ReadBoundary, 1)
	go func() { boundary, _ := r.endpoint.Gate().Capture(); captured <- boundary }()
	deadline := time.Now().Add(5 * time.Second)
	for r.endpoint.Gate().Noted() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the end was never noted")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-captured:
		t.Fatal("the end was captured while the acknowledgment was writing")
	case <-time.After(100 * time.Millisecond):
	}
	free()
	if err := <-acknowledged; err != nil {
		t.Fatalf("the acknowledgment: %v", err)
	}
	var boundary *inbox.ReadBoundary
	select {
	case boundary = <-captured:
	case <-time.After(5 * time.Second):
		t.Fatal("the end was not captured after the acknowledgment closed")
	}
	if r.unread(id) {
		t.Fatal("the acknowledgment read nothing")
	}
	if after := r.clock.Snapshot(); boundary == nil || boundary.Through != after.Through {
		t.Fatalf("the boundary %+v is not the acknowledgment's closing %+v", boundary, after)
	}
}

// Two servers asking for one call's ticket at once — a harness that
// restarted its server mid-call — get one ticket between them, and the
// harness's record may come after both asked.
func TestOneCallRunsOnceAcrossServers(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	first, second := r.start(), startServer(t, r.env())
	second.initialize(t)
	r.nextTurn()
	words := []string{"send", "--notify", "--wait", "0", "web", faultHeadsUp}
	answers := make(chan callResult, 2)
	for _, server := range []*mcpClient{first, second} {
		go func() {
			result, _ := server.call(t, words, r.meta("turn-1", "shared"))
			answers <- result
		}()
	}
	time.Sleep(200 * time.Millisecond)
	r.observe("turn-1", "shared", words)
	ran := 0
	for range 2 {
		if answer := <-answers; !answer.IsError || !strings.Contains(answer.text(), "issued no ticket") {
			ran++
		}
	}
	if ran != 1 || headsUps(r) != 1 {
		t.Fatalf("%d calls ran, web holds %d copies", ran, headsUps(r))
	}
}
