package modhost

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
)

// timers stands in for the lapse watch's clock: each timer waits until the
// test fires it, stopped or not, as a timer that fires while it is stopped.
type timers struct {
	mu  sync.Mutex
	all []func()
}

func (c *timers) after(_ time.Duration, f func()) func() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.all = append(c.all, f)
	return func() bool { return true }
}

func (c *timers) fire(t *testing.T, i int) {
	t.Helper()
	c.mu.Lock()
	if i >= len(c.all) {
		c.mu.Unlock()
		t.Fatalf("timer %d was never started; %d were", i, len(c.all))
	}
	f := c.all[i]
	c.mu.Unlock()
	f()
}

func (c *timers) latest() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.all) - 1
}

// transition is one report the run hears from its endpoint.
type transition struct {
	number uint64
	proven bool
}

type heard struct {
	mu   sync.Mutex
	list []transition
}

func (h *heard) hear(number uint64, proven bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.list = append(h.list, transition{number, proven})
}

func (h *heard) all() []transition {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.list)
}

func (h *heard) want(t *testing.T, want ...transition) {
	t.Helper()
	if got := h.all(); !slices.Equal(got, want) {
		t.Fatalf("the run heard %v, want %v", got, want)
	}
}

// heartbeating is a run's endpoint whose lapse timers the test fires and
// whose polls are held for hold.
func heartbeating(t *testing.T, hold time.Duration) (*Server, string, *timers, *heard) {
	t.Helper()
	clock, run := &timers{}, &heard{}
	server, socket := servingWith(t, func(s *Server) {
		s.after, s.cfg.Live, s.cfg.Hold = clock.after, run.hear, hold
	})
	return server, socket, clock, run
}

func helloOf(load string, seq int) []byte {
	data, _ := json.Marshal(HelloRequest{LoadID: load, SessionID: "conversation-1", HarnessVersion: "2.1.296", LoadSeq: seq})
	return data
}

// sayHello posts a load's hello and answers its status, the generation it
// was given and, on a 410, the number its refusal names.
func sayHello(t *testing.T, socket, load string, seq int) (status, generation, holder int) {
	t.Helper()
	status, text, err := post(socket, PathHello, helloOf(load, seq))
	if err != nil {
		t.Fatal(err)
	}
	generation, holder = decodeHello(t, status, text)
	return status, generation, holder
}

func decodeHello(t *testing.T, status int, text string) (generation, holder int) {
	t.Helper()
	if status == http.StatusOK {
		var answer HelloAnswer
		if err := json.Unmarshal([]byte(text), &answer); err != nil {
			t.Fatalf("a hello answered %s", text)
		}
		return answer.Generation, 0
	}
	var refusal Refusal
	if err := json.Unmarshal([]byte(text), &refusal); err != nil {
		t.Fatalf("a hello refused with %s", text)
	}
	if refusal.LoadSeq != nil {
		holder = *refusal.LoadSeq
	}
	return 0, holder
}

func poll(t *testing.T, socket, load string, generation int) int {
	t.Helper()
	data, _ := json.Marshal(AliveRequest{LoadID: load, Generation: generation})
	status, _, err := post(socket, PathModAlive, data)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

// A generation no poll renews within the lapse withdraws what the mod
// proved; its polls are refused 409 then, and the load's hello again opens a
// new generation, which proves it once more and is polled.
func TestALapseWithdrawsWhatTheModProvedAndANewHelloRestoresIt(t *testing.T) {
	_, socket, clock, run := heartbeating(t, time.Millisecond)
	status, first, _ := sayHello(t, socket, "load-1", 1)
	if status != http.StatusOK || poll(t, socket, "load-1", first) != http.StatusOK {
		t.Fatalf("the hello: %d, generation %d", status, first)
	}
	clock.fire(t, clock.latest())
	run.want(t, transition{1, true}, transition{2, false})
	if status := poll(t, socket, "load-1", first); status != http.StatusConflict {
		t.Fatalf("a poll of the lapsed generation was answered %d", status)
	}
	status, second, _ := sayHello(t, socket, "load-1", 1)
	if status != http.StatusOK || second == first {
		t.Fatalf("the hello after the lapse: %d, generation %d after %d", status, second, first)
	}
	run.want(t, transition{1, true}, transition{2, false}, transition{3, true})
	if status := poll(t, socket, "load-1", second); status != http.StatusOK {
		t.Fatalf("a poll of the new generation was answered %d", status)
	}
}

// A hello sent again because its answer was lost is the same load's: it
// keeps its live generation and reports nothing.
func TestAHelloRetriedAfterItsAnswerWasLostKeepsItsGeneration(t *testing.T) {
	_, socket, _, run := heartbeating(t, time.Millisecond)
	_, first, _ := sayHello(t, socket, "load-1", 1)
	status, again, _ := sayHello(t, socket, "load-1", 1)
	if status != http.StatusOK || again != first {
		t.Fatalf("the retried hello: %d, generation %d after %d", status, again, first)
	}
	run.want(t, transition{1, true})
}

// heldBody holds a request's body until released, telling when it is read.
type heldBody struct {
	io.Reader
	reading, release chan struct{}
}

func (b *heldBody) Read(p []byte) (int, error) {
	if b.reading != nil {
		close(b.reading)
		b.reading = nil
		<-b.release
	}
	return b.Reader.Read(p)
}

// An earlier load's hello held while its body is read, as B's hello with a
// higher number binds, is refused 410 when it is released, naming B's
// number; so is an earlier load's first hello arriving only after B's; and B
// stays the run's load.
func TestAnEarlierLoadsHelloAfterALaterOnesIsRefused(t *testing.T) {
	server, socket, _, run := heartbeating(t, time.Millisecond)
	body := &heldBody{Reader: bytes.NewReader(helloOf("load-a", 1)), reading: make(chan struct{}), release: make(chan struct{})}
	reading := body.reading
	request := httptest.NewRequest(http.MethodPost, PathHello, body)
	request = request.WithContext(context.WithValue(request.Context(), peerKey{}, peer{pid: os.Getpid()}))
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.serve(response, request)
	}()
	<-reading
	status, generation, _ := sayHello(t, socket, "load-b", 2)
	if status != http.StatusOK {
		t.Fatalf("the later load's hello: %d", status)
	}
	close(body.release)
	<-done
	if _, holder := decodeHello(t, response.Code, response.Body.String()); response.Code != http.StatusGone || holder != 2 {
		t.Fatalf("the earlier load's held hello: %d %s", response.Code, response.Body.String())
	}
	if status, _, holder := sayHello(t, socket, "load-c", 1); status != http.StatusGone || holder != 2 {
		t.Fatalf("an earlier load's first hello after the later one's: %d, naming %d", status, holder)
	}
	if load, _ := server.Load(); load.LoadID != "load-b" || poll(t, socket, "load-b", generation) != http.StatusOK {
		t.Fatalf("the run's load after the earlier hellos: %+v", load)
	}
	run.want(t, transition{1, true})
}

// A hello of another load with the current load's number — a counter whose
// write did not land — is bound as the later load, and the earlier one is
// superseded: its polls and its hellos are refused 410.
func TestAHelloOfAnotherLoadWithTheCurrentNumberBindsAsTheLater(t *testing.T) {
	server, socket, _, run := heartbeating(t, time.Millisecond)
	_, first, _ := sayHello(t, socket, "load-a", 3)
	if status, _, _ := sayHello(t, socket, "load-b", 3); status != http.StatusOK {
		t.Fatalf("the later load's hello: %d", status)
	}
	if load, _ := server.Load(); load.LoadID != "load-b" {
		t.Fatalf("the run's load: %+v", load)
	}
	if status := poll(t, socket, "load-a", first); status != http.StatusGone {
		t.Fatalf("a poll of the superseded load was answered %d", status)
	}
	if status, _, _ := sayHello(t, socket, "load-a", 4); status != http.StatusGone {
		t.Fatalf("the superseded load's hello under a higher number was answered %d", status)
	}
	run.want(t, transition{1, true}, transition{2, true})
}

// A poll of a generation the load no longer holds — sent before a new hello
// of the same load opened the next — is refused 409, and the new
// generation's poll renews it.
func TestAPollDelayedAcrossANewHelloOfTheSameLoadIsRefused(t *testing.T) {
	_, socket, clock, _ := heartbeating(t, time.Millisecond)
	_, first, _ := sayHello(t, socket, "load-1", 1)
	clock.fire(t, clock.latest())
	_, second, _ := sayHello(t, socket, "load-1", 1)
	if status := poll(t, socket, "load-1", first); status != http.StatusConflict {
		t.Fatalf("the delayed poll of generation %d was answered %d", first, status)
	}
	if status := poll(t, socket, "load-1", second); status != http.StatusOK {
		t.Fatalf("the poll of generation %d was answered %d", second, status)
	}
}

// A lapse timer of an earlier arm or generation fires to no effect: the
// poll that re-armed the watch made the hello's old, and a later load's
// hello made both old, the hello's sharing its first arm with the new load's.
func TestALapseTimerOfAnEarlierTransitionDoesNothing(t *testing.T) {
	_, socket, clock, run := heartbeating(t, time.Millisecond)
	_, generation, _ := sayHello(t, socket, "load-1", 1)
	byHello := clock.latest()
	poll(t, socket, "load-1", generation)
	byPoll := clock.latest()
	clock.fire(t, byHello)
	sayHello(t, socket, "load-2", 2)
	clock.fire(t, byHello)
	clock.fire(t, byPoll)
	run.want(t, transition{1, true}, transition{2, true})
}

// An end sent after a lapse is taken, and taking it makes nothing live.
func TestAnEndAfterALapseIsTakenAndMakesNothingLive(t *testing.T) {
	server, socket, clock, run := heartbeating(t, time.Millisecond)
	c := &core{}
	server.ReportTurns(c.handler())
	sayHello(t, socket, "load-1", 1)
	clock.fire(t, clock.latest())
	if status, receipt := sendEnd(t, socket, endBody("end-1", "answered", "done")); status != http.StatusOK || receipt == nil {
		t.Fatalf("the end after the lapse was answered %d", status)
	}
	if len(c.published()) != 1 {
		t.Fatalf("the core took %d ends", len(c.published()))
	}
	run.want(t, transition{1, true}, transition{2, false})
}

// A lapse timer that fires after the close reports nothing.
func TestALapseAfterTheCloseReportsNothing(t *testing.T) {
	server, socket, clock, run := heartbeating(t, time.Millisecond)
	sayHello(t, socket, "load-1", 1)
	server.Close()
	clock.fire(t, clock.latest())
	run.want(t, transition{1, true})
}

// A poll is held for the hold with no lock held: a hello and an end are
// answered while it waits, and it is answered once the hold ends.
func TestAHeldPollHoldsNoLock(t *testing.T) {
	const hold = 2 * time.Second
	server, socket, _, _ := heartbeating(t, hold)
	server.ReportTurns((&core{}).handler())
	_, generation, _ := sayHello(t, socket, "load-1", 1)
	renewed := make(chan struct{})
	var once sync.Once
	server.mu.Lock()
	server.deciding = func() { once.Do(func() { close(renewed) }) }
	server.mu.Unlock()
	began := time.Now()
	answered := make(chan int)
	go func() {
		data, _ := json.Marshal(AliveRequest{LoadID: "load-1", Generation: generation})
		status, _, _ := post(socket, PathModAlive, data)
		answered <- status
	}()
	<-renewed
	time.Sleep(50 * time.Millisecond)
	if status, _, _ := sayHello(t, socket, "load-1", 1); status != http.StatusOK {
		t.Fatalf("a hello during the held poll: %d", status)
	}
	if status, _ := sendEnd(t, socket, endBody("end-1", "answered", "done")); status != http.StatusOK {
		t.Fatalf("an end during the held poll: %d", status)
	}
	if waited := time.Since(began); waited >= hold {
		t.Fatalf("the hello and the end waited %v, past the poll's hold", waited)
	}
	if status := <-answered; status != http.StatusOK || time.Since(began) < hold {
		t.Fatalf("the poll was answered %d after %v", status, time.Since(began))
	}
}

// A poll whose harness was replaced after its first peer check is cut off
// before its effect and renews nothing.
func TestAPollFromAReplacedHarnessRenewsNothing(t *testing.T) {
	server, socket, clock, _ := heartbeating(t, time.Millisecond)
	_, generation, _ := sayHello(t, socket, "load-1", 1)
	armed := clock.latest()
	server.deciding = func() {
		server.HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: 1})
	}
	data, _ := json.Marshal(AliveRequest{LoadID: "load-1", Generation: generation})
	if status, text, err := post(socket, PathModAlive, data); err == nil {
		t.Fatalf("a poll from a replaced harness was answered: %d %s", status, text)
	}
	if clock.latest() != armed {
		t.Fatal("a poll from a replaced harness renewed the heartbeat")
	}
}

// A poll held before its decision while the run closes renews nothing.
func TestAPollPastTheCloseRenewsNothing(t *testing.T) {
	server, socket, clock, _ := heartbeating(t, time.Millisecond)
	_, generation, _ := sayHello(t, socket, "load-1", 1)
	armed := clock.latest()
	data, _ := json.Marshal(AliveRequest{LoadID: "load-1", Generation: generation})
	entered, release := make(chan struct{}), make(chan struct{})
	server.deciding = func() {
		close(entered)
		<-release
	}
	answered := make(chan int)
	go func() {
		status, _, _ := post(socket, PathModAlive, data)
		answered <- status
	}()
	<-entered
	server.Close()
	close(release)
	if status := <-answered; status == http.StatusOK {
		t.Fatal("a poll past the close was answered 200")
	}
	if clock.latest() != armed {
		t.Fatal("a poll past the close renewed the heartbeat")
	}
}
