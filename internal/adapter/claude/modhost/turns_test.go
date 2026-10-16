package modhost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
)

// core stands in for the wrapper's side of a run's turns: it counts the reads
// so every capture is a boundary of its own, and keeps what it was handed.
type core struct {
	mu      sync.Mutex
	reads   uint64
	starts  []int64
	ends    []adapter.TurnEnd
	answers []error
	// held, when set, holds each publication until it is closed.
	held chan struct{}
}

func (c *core) handler() adapter.TurnHandler {
	return adapter.TurnHandler{
		Started: func(_, _ string, at int64) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.starts = append(c.starts, at)
		},
		Capture: func() *mail.ReadBoundary {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.reads++
			return &mail.ReadBoundary{Mailbox: "api", Epoch: "1", Through: c.reads}
		},
		Publish: func(_ context.Context, end adapter.TurnEnd) error {
			c.mu.Lock()
			held := c.held
			c.ends = append(c.ends, end)
			var answer error
			if len(c.answers) > 0 {
				answer, c.answers = c.answers[0], c.answers[1:]
			}
			c.mu.Unlock()
			if held != nil {
				<-held
			}
			return answer
		},
	}
}

func (c *core) published() []adapter.TurnEnd {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]adapter.TurnEnd{}, c.ends...)
}

// turning is a run's endpoint with its turns going to a core, and load-1's
// hello admitted.
func turning(t *testing.T) (*Server, string, *core) {
	t.Helper()
	server, socket := serving(t)
	c := &core{}
	server.ReportTurns(c.handler())
	if status, text, err := post(socket, PathHello, validHello("load-1")); err != nil || status != http.StatusOK {
		t.Fatalf("the hello: %d %s %v", status, text, err)
	}
	return server, socket, c
}

func endBody(endID, reason, answer string) []byte {
	data, _ := json.Marshal(TurnEndRequest{
		LoadID: "load-1", Conversation: "conversation-1", TurnID: "turn-1", EndID: endID,
		Reason: EndReason(reason), Answer: &answer,
	})
	return data
}

// sendEnd posts an end and answers its status and the receipt it decoded to.
func sendEnd(t *testing.T, socket string, body []byte) (int, *Receipt) {
	t.Helper()
	status, text, err := post(socket, PathTurnEnd, body)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		return status, nil
	}
	var receipt Receipt
	if err := json.Unmarshal([]byte(text), &receipt); err != nil {
		t.Fatalf("a 200 that is no receipt: %s", text)
	}
	return status, &receipt
}

// Each reason reaches the core as its end reason, under the end's event, with
// the answer, the conversation, a boundary and no start; the receipt names
// the turn and the end. A turn's start is passed on with no time.
func TestEachReasonReachesTheCoreAsItsEndReason(t *testing.T) {
	_, socket, c := turning(t)
	start, _ := json.Marshal(TurnStartRequest{LoadID: "load-1", Conversation: "conversation-1", TurnID: "turn-1"})
	if status, text, err := post(socket, PathTurnStart, start); err != nil || status != http.StatusOK || !strings.Contains(text, `"turnId":"turn-1"`) {
		t.Fatalf("a start: %d %s %v", status, text, err)
	}
	if len(c.starts) != 1 || c.starts[0] != 0 {
		t.Fatalf("the start was passed on at %v, want once with no time", c.starts)
	}
	want := map[string]adapter.EndReason{"answered": adapter.Answered, "aborted": adapter.Aborted, "refused": adapter.Refused, "failed": adapter.Failed}
	for reason, mapped := range want {
		status, receipt := sendEnd(t, socket, endBody("end-"+reason, reason, "words of "+reason))
		if status != http.StatusOK || receipt.TurnID != "turn-1" || receipt.EndID == nil || *receipt.EndID != "end-"+reason {
			t.Fatalf("%s: %d %+v", reason, status, receipt)
		}
		ends := c.published()
		end := ends[len(ends)-1]
		if end.Event != "claude/end-"+reason || end.Reason != mapped || end.Answer != "words of "+reason ||
			end.Thread != "conversation-1" || end.Boundary == nil || end.Started != 0 || end.Ended == 0 || end.Check() != nil {
			t.Fatalf("%s reached the core as %+v", reason, end)
		}
	}
	if status, _ := sendEnd(t, socket, endBody("end-x", "finished", "w")); status != http.StatusBadRequest {
		t.Fatalf("a reason the protocol does not know: %d", status)
	}
}

// A turn of a load the run never admitted is refused and reaches nothing.
func TestATurnWithoutAnAdmittedHelloIsRefused(t *testing.T) {
	server, socket := serving(t)
	c := &core{}
	server.ReportTurns(c.handler())
	if status, _ := sendEnd(t, socket, endBody("end-1", "answered", "w")); status != http.StatusBadRequest {
		t.Fatalf("an end before any hello: %d", status)
	}
	start, _ := json.Marshal(TurnStartRequest{LoadID: "load-1", Conversation: "c", TurnID: "t"})
	if status, _, _ := post(socket, PathTurnStart, start); status != http.StatusBadRequest {
		t.Fatalf("a start before any hello: %d", status)
	}
	if len(c.published()) != 0 || len(c.starts) != 0 {
		t.Fatal("a turn of no admitted load reached the core")
	}
}

// An end the core did not take is answered 503, and its retry — after later
// reads, as a lost receipt's would be — hands the core the first capture:
// the same event, boundary and times, none of what was read since.
func TestARetriedEndHandsTheCoreItsFirstCapture(t *testing.T) {
	_, socket, c := turning(t)
	c.answers = []error{errors.New("the mailbox is busy")}
	if status, receipt := sendEnd(t, socket, endBody("end-1", "answered", "done")); status != http.StatusServiceUnavailable || receipt != nil {
		t.Fatalf("a failed publication: %d %+v", status, receipt)
	}
	c.handler().Capture() // a read acknowledged after the first arrival
	time.Sleep(10 * time.Millisecond)
	for range 2 {
		if status, receipt := sendEnd(t, socket, endBody("end-1", "answered", "done")); status != http.StatusOK || receipt == nil {
			t.Fatalf("the retry: %d", status)
		}
	}
	ends := c.published()
	if len(ends) != 3 {
		t.Fatalf("the core was handed %d attempts", len(ends))
	}
	for _, end := range ends[1:] {
		if end.Event != ends[0].Event || *end.Boundary != *ends[0].Boundary || end.Ended != ends[0].Ended || end.Started != ends[0].Started {
			t.Fatalf("a retry was captured again: %+v, first %+v", end, ends[0])
		}
	}
}

// Two copies of one end at once are captured once.
func TestTwoCopiesOfAnEndAtOnceAreCapturedOnce(t *testing.T) {
	_, socket, c := turning(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { sendEnd(t, socket, endBody("end-1", "answered", "done")) })
	}
	wg.Wait()
	ends := c.published()
	for _, end := range ends {
		if *end.Boundary != *ends[0].Boundary || end.Ended != ends[0].Ended {
			t.Fatalf("copies captured apart: %+v and %+v", end, ends[0])
		}
	}
}

// An end id sent again with other content is refused and handed to nobody.
func TestAKnownEndWithOtherContentIsRefused(t *testing.T) {
	_, socket, c := turning(t)
	sendEnd(t, socket, endBody("end-1", "answered", "done"))
	if status, _ := sendEnd(t, socket, endBody("end-1", "answered", "other words")); status != http.StatusConflict {
		t.Fatalf("other content under a known end id: %d", status)
	}
	if len(c.published()) != 1 {
		t.Fatal("the conflicting end reached the core")
	}
}

// An end of a run proven gone is answered 410 with no receipt.
func TestAnEndOfARunGoneIsAnswered410WithNoReceipt(t *testing.T) {
	_, socket, c := turning(t)
	c.answers = []error{adapter.ErrRunGone}
	if status, receipt := sendEnd(t, socket, endBody("end-1", "answered", "done")); status != http.StatusGone || receipt != nil {
		t.Fatalf("an end of a run gone: %d %+v", status, receipt)
	}
}

// A request held before its decision while the run closes captures and
// publishes nothing, whether its end is new or captured before.
func TestARequestDelayedAcrossCloseTakesNoEffect(t *testing.T) {
	for _, cached := range []bool{false, true} {
		server, socket, c := turning(t)
		if cached {
			c.answers = []error{errors.New("busy")}
			sendEnd(t, socket, endBody("end-1", "answered", "done"))
		}
		before := len(c.published())
		reads := c.reads
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		server.deciding = func() {
			once.Do(func() { close(entered) })
			<-release
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _, _ = post(socket, PathTurnEnd, endBody("end-1", "answered", "done"))
		}()
		<-entered
		server.Close()
		close(release)
		<-done
		time.Sleep(50 * time.Millisecond)
		if len(c.published()) != before || c.reads != reads {
			t.Fatalf("cached %v: a request past the close captured or published", cached)
		}
	}
}

// A publication begun before the close keeps the room's lease until it
// returns: its handler stays counted among the run's workers.
func TestABegunPublicationHoldsTheLease(t *testing.T) {
	server, socket, c := turning(t)
	room := filepath.Dir(filepath.Dir(filepath.Dir(server.cfg.Record)))
	life := &runLife{room: room, release: make(chan struct{})}
	server.CountWorkers(life)
	c.held = make(chan struct{})
	done := make(chan int, 1)
	go func() {
		status, _, _ := post(socket, PathTurnEnd, endBody("end-1", "answered", "done"))
		done <- status
	}()
	deadline := time.Now().Add(2 * time.Second)
	for len(c.published()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the publication never began")
		}
		time.Sleep(time.Millisecond)
	}
	server.Close()
	life.end()
	if life.counted() != 1 {
		t.Fatalf("a begun publication is not counted among the run's workers: %d", life.counted())
	}
	close(c.held)
	select {
	case <-life.release:
	case <-time.After(5 * time.Second):
		t.Fatal("the lease was never let go")
	}
	<-done
}

// An end's body is bounded by its own bound, past the other requests'.
func TestAnEndsBodyIsBoundedByItsOwnBound(t *testing.T) {
	_, socket, c := turning(t)
	long := strings.Repeat("é", MaxBodyBytes)
	if status, _ := sendEnd(t, socket, endBody("end-1", "answered", long)); status != http.StatusOK {
		t.Fatalf("an end past the other requests' bound and within its own: %d", status)
	}
	if got := c.published(); len(got) != 1 || got[0].Answer != long {
		t.Fatal("the long answer did not reach the core whole")
	}
	if status, _ := sendEnd(t, socket, endBody("end-2", "answered", strings.Repeat("x", MaxEndBodyBytes))); status != http.StatusRequestEntityTooLarge {
		t.Fatalf("an end past its bound: %d", status)
	}
}

// mod/ended answers only the runs proven ended, to an admitted load, and
// changes nothing.
func TestModEndedAnswersOnlyRunsProvenEnded(t *testing.T) {
	server, socket, _ := turning(t)
	server.cfg.Ended = func(epoch string) bool { return strings.HasPrefix(epoch, "gone") }
	ask := func(load string, epochs []string) (int, string) {
		body, _ := json.Marshal(EndedRequest{LoadID: load, Epochs: epochs})
		status, text, err := post(socket, PathModEnded, body)
		if err != nil {
			t.Fatal(err)
		}
		return status, text
	}
	status, text := ask("load-1", []string{"gone.1", "alive.2", "gone.3"})
	var answer EndedAnswer
	if status != http.StatusOK || json.Unmarshal([]byte(text), &answer) != nil || strings.Join(answer.Ended, ",") != "gone.1,gone.3" {
		t.Fatalf("mod/ended: %d %s", status, text)
	}
	if status, text := ask("load-1", nil); status != http.StatusOK || !strings.Contains(text, `"ended":[]`) {
		t.Fatalf("mod/ended of none: %d %s", status, text)
	}
	if status, _ := ask("load-9", []string{"gone.1"}); status != http.StatusBadRequest {
		t.Fatalf("mod/ended of a load never admitted: %d", status)
	}
	if status, _ := ask("load-1", make([]string, 33)); status != http.StatusBadRequest {
		t.Fatalf("mod/ended of 33 runs: %d", status)
	}
}

// A start or an end whose harness was replaced after its first peer check is
// cut off before its effect: the peer is checked again before each, so
// nothing reaches the core and the end is not captured.
func TestATurnFromAReplacedHarnessHasNoEffect(t *testing.T) {
	for _, path := range []string{PathTurnStart, PathTurnEnd} {
		server, socket, c := turning(t)
		reads := c.reads
		server.deciding = func() {
			server.HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: 1})
		}
		body := endBody("end-1", "answered", "done")
		if path == PathTurnStart {
			body, _ = json.Marshal(TurnStartRequest{LoadID: "load-1", Conversation: "c", TurnID: "t"})
		}
		if status, text, err := post(socket, path, body); err == nil {
			t.Fatalf("%s from a replaced harness was answered: %d %s", path, status, text)
		}
		if len(c.published()) != 0 || len(c.starts) != 0 || c.reads != reads || len(server.ends) != 0 {
			t.Fatalf("%s from a replaced harness took effect", path)
		}
	}
}
