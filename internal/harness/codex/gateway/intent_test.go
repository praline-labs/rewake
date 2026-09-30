package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/sessionstate"
)

const (
	resumeX       = `{"id":1,"method":"thread/resume","params":{"threadId":"X","config":{},"runtimeWorkspaceRoots":[]}}`
	activeWriterX = `{"id":1,"error":{"code":-32600,"message":"thread X already has an active writer"}}`
	startY        = `{"id":2,"method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`
	startedY      = `{"id":2,"result":{"thread":{"id":"Y","canAcceptDirectInput":true}}}`
)

func reserveHeld(t *testing.T, g *Gateway) *HoldError {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := g.Reserve(ctx)
	if err == nil {
		r.Close()
		t.Fatal("reserved a conversation the launch did not ask for")
	}
	var held *HoldError
	if !errors.As(err, &held) || !errors.Is(err, ErrUnintended) {
		t.Fatalf("not a hold: %v", err)
	}
	return held
}

func reserveFree(t *testing.T, g *Gateway, thread string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := g.Reserve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.Thread() != thread {
		t.Fatalf("reserved %s, want %s", r.Thread(), thread)
	}
}

// refuseX has the terminal resume X and the server refuse it, as in the
// incident, and reads the warning the terminal is shown for it.
func refuseX(t *testing.T, ui, server *socketClient) {
	t.Helper()
	exchange(t, ui, server, resumeX, activeWriterX)
	_ = warning(t, ui, "X")
}

// warning reads what the terminal is shown next and requires it to be the
// hold's warning.
func warning(t *testing.T, ui *socketClient, thread string) string {
	t.Helper()
	var shown struct {
		Method string `json:"method"`
		Params struct {
			ThreadID string `json:"threadId"`
			Message  string `json:"message"`
		} `json:"params"`
	}
	if err := json.Unmarshal(readWithin(t, ui), &shown); err != nil || shown.Method != "warning" || shown.Params.ThreadID != thread {
		t.Fatalf("not a warning for %s: %+v %v", thread, shown, err)
	}
	return shown.Params.Message
}

// The incident of September 29, 2026: the launch resumed X, the server refused
// it for another writer, the terminal started Y, and the notice went into Y.
func TestAResumeRefusedThenAStartHoldsDeliveries(t *testing.T) {
	g, ui, peers, _ := setupConfig(t, Config{Intent: LaunchIntent{Resume: true, Thread: "X"}, Name: "worker"})
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	refuseX(t, ui, server)
	exchange(t, ui, server, startY, startedY)
	held := reserveHeld(t, g)
	if held.Hold.Reason != sessionstate.HoldUnintended || held.Hold.Expected != "X" || held.Hold.Selected != "Y" {
		t.Fatalf("%+v", held.Hold)
	}
	for _, want := range []string{"X", "Y", "rewake accept worker Y", "/resume"} {
		if !strings.Contains(held.Hold.Detail, want) {
			t.Fatalf("the refusal does not name %q: %s", want, held.Hold.Detail)
		}
	}
	if message := warning(t, ui, "Y"); !strings.Contains(message, "rewake accept worker Y") {
		t.Fatal(message)
	}
	if state := g.SessionState(); state.DeliveryHold == nil || state.DeliveryHold.Selected != "Y" {
		t.Fatalf("the hold is not in the session's state: %+v", state.DeliveryHold)
	}
	// /new starts another conversation: still not the launch's.
	exchange(t, ui, server, `{"id":3,"method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`, `{"id":3,"result":{"thread":{"id":"Z","canAcceptDirectInput":true}}}`)
	if held := reserveHeld(t, g); held.Hold.Selected != "Z" {
		t.Fatalf("%+v", held.Hold)
	}
	_ = warning(t, ui, "Z")
	if err := g.Accept("Y"); !errors.Is(err, ErrNotSelected) {
		t.Fatalf("accepted a conversation no longer selected: %v", err)
	}
	if err := g.Accept("Z"); err != nil {
		t.Fatal(err)
	}
	reserveFree(t, g, "Z")
	if err := g.Accept("Z"); !errors.Is(err, ErrNothingHeld) {
		t.Fatalf("accepted twice: %v", err)
	}
	// After the acceptance the terminal navigates as it always did.
	exchange(t, ui, server, `{"id":4,"method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`, `{"id":4,"result":{"thread":{"id":"W","canAcceptDirectInput":true}}}`)
	reserveFree(t, g, "W")
}

// A resume of the intended conversation lifts the hold for good: switching
// later is ordinary navigation.
func TestResumingTheIntendedConversationLiftsTheHold(t *testing.T) {
	g, ui, peers, _ := setupConfig(t, Config{Intent: LaunchIntent{Resume: true, Thread: "X"}, Name: "worker"})
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	refuseX(t, ui, server)
	exchange(t, ui, server, startY, startedY)
	_ = reserveHeld(t, g)
	_ = warning(t, ui, "Y")
	exchange(t, ui, server, `{"id":3,"method":"thread/resume","params":{"threadId":"X","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":3,"result":{"thread":{"id":"X","canAcceptDirectInput":true}}}`)
	// The terminal's next request ends the resume's reads.
	exchange(t, ui, server, `{"id":5,"method":"thread/goal/get","params":{"threadId":"X"}}`, `{"id":5,"result":{}}`)
	reserveFree(t, g, "X")
	if g.Hold() != nil {
		t.Fatal(g.Hold())
	}
	exchange(t, ui, server, `{"id":4,"method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`, `{"id":4,"result":{"thread":{"id":"Y","canAcceptDirectInput":true}}}`)
	reserveFree(t, g, "Y")
}

// resume --last and the picker name no conversation at launch: the terminal's
// first resume does, before its reply, so a refused one still pins it.
func TestTheFirstResumePinsAnUnnamedIntent(t *testing.T) {
	g, ui, peers, _ := setupConfig(t, Config{Intent: LaunchIntent{Resume: true}, Name: "worker"})
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	refuseX(t, ui, server)
	exchange(t, ui, server, startY, startedY)
	if held := reserveHeld(t, g); held.Hold.Expected != "X" {
		t.Fatalf("%+v", held.Hold)
	}
	_ = warning(t, ui, "Y")
	// A later resume of another conversation does not re-pin.
	exchange(t, ui, server, `{"id":3,"method":"thread/resume","params":{"threadId":"V","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":3,"result":{"thread":{"id":"V","canAcceptDirectInput":true}}}`)
	if held := reserveHeld(t, g); held.Hold.Expected != "X" || held.Hold.Selected != "V" {
		t.Fatalf("%+v", held.Hold)
	}
}

// A launch that asked to resume and never did holds a fresh conversation too.
func TestAResumeLaunchThatStartsFreshIsHeld(t *testing.T) {
	g, ui, peers, _ := setupConfig(t, Config{Intent: LaunchIntent{Resume: true}, Name: "worker"})
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	exchange(t, ui, server, startY, startedY)
	held := reserveHeld(t, g)
	if held.Hold.Expected != "" || held.Hold.Selected != "Y" || !strings.Contains(held.Hold.Detail, "without resuming") {
		t.Fatalf("%+v", held.Hold)
	}
}

// A fresh launch carries no intent: its first conversation takes deliveries
// as before, and there is nothing to accept.
func TestAFreshLaunchHoldsNothing(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	reserveFree(t, g, "A")
	if err := g.Accept("A"); !errors.Is(err, ErrNothingHeld) {
		t.Fatal(err)
	}
}

// Accepting needs a conversation selected now.
func TestAcceptRefusesWithoutASelection(t *testing.T) {
	g, ui, peers, _ := setupConfig(t, Config{Intent: LaunchIntent{Resume: true, Thread: "X"}, Name: "worker"})
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	refuseX(t, ui, server)
	if err := g.Accept("X"); !errors.Is(err, ErrNoSelection) {
		t.Fatal(err)
	}
}

func (g *Gateway) intentWaiting() bool {
	g.intent.mu.Lock()
	defer g.intent.mu.Unlock()
	return g.intent.waiting
}

// The terminal is told once per hold, not at every refused reservation.
func TestTheWarningIsShownOnce(t *testing.T) {
	g, ui, peers, _ := setupConfig(t, Config{Intent: LaunchIntent{Resume: true, Thread: "X"}, Name: "worker"})
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	refuseX(t, ui, server)
	exchange(t, ui, server, startY, startedY)
	_ = warning(t, ui, "Y")
	_ = reserveHeld(t, g)
	g.currentConnection().warnHold(g.Binding(), g.Hold())
	_ = ui.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if raw, err := ui.readMessage(); err == nil {
		t.Fatalf("shown again: %s", raw)
	}
	_ = ui.conn.SetReadDeadline(time.Time{})
}

// A resume refused with nothing selected after it — the terminal showing the
// held conversation read-only, as its source does — holds deliveries at once,
// publishes the hold and tells the terminal (acceptance of September 29, 2026,
// finding 3).
func TestARefusedResumeWithNothingSelectedHolds(t *testing.T) {
	g, ui, peers, _ := setupConfig(t, Config{Intent: LaunchIntent{Resume: true, Thread: "X"}, Name: "worker"})
	server := <-peers
	exchange(t, ui, server, resumeX, activeWriterX)
	if message := warning(t, ui, "X"); !strings.Contains(message, "already has an active writer") {
		t.Fatal(message)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := g.Reserve(ctx)
	var held *HoldError
	if !errors.As(err, &held) || held.Hold.Expected != "X" || held.Hold.Selected != "" || time.Since(started) > 60*time.Millisecond {
		t.Fatalf("not held at once: %v", err)
	}
	if state := g.SessionState(); state.DeliveryHold == nil || state.DeliveryHold.Expected != "X" {
		t.Fatalf("the hold is not published: %+v", state.DeliveryHold)
	}
	if err := g.Accept("X"); !errors.Is(err, ErrNoSelection) {
		t.Fatal(err)
	}
	// The hold outlives the connection.
	_ = server.conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for g.currentConnection() != nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := g.Reserve(ctx); !errors.As(err, &held) || held.Hold.Expected != "X" {
		t.Fatalf("the hold was lost with the connection: %v", err)
	}
}

// The same with the conversation opened read-only instead of refused.
func TestAReadOnlyResumeHolds(t *testing.T) {
	g, ui, peers, _ := setupConfig(t, Config{Intent: LaunchIntent{Resume: true, Thread: "X"}, Name: "worker"})
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	exchange(t, ui, server, resumeX, `{"id":1,"result":{"thread":{"id":"X","canAcceptDirectInput":false}}}`)
	_ = warning(t, ui, "X")
	if hold := g.Hold(); hold == nil || !strings.Contains(hold.Detail, "without the right to write") {
		t.Fatalf("%+v", hold)
	}
}

// A resume still on its way is not a hold to publish, but a delivery that
// outwaits it stays pending rather than failing.
func TestAnOwedResumeKeepsTheDeliveryPending(t *testing.T) {
	g, ui, peers, _ := setupConfig(t, Config{Intent: LaunchIntent{Resume: true, Thread: "X"}, Name: "worker"})
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	write(t, ui, []byte(resumeX))
	_ = readWithin(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if _, err := g.Reserve(ctx); !errors.Is(err, ErrUnintended) {
		t.Fatalf("the owed resume failed the delivery: %v", err)
	}
	if hold := g.Hold(); hold != nil {
		t.Fatalf("an owed resume was published as a hold: %+v", hold)
	}
}

// The acceptance checks the selection and lifts the hold in one step: the
// selection cannot change between the two (acceptance of September 29, 2026,
// finding 1). The probe stops Accept at the intent's lock and tries to change
// the selection meanwhile; it must find the connection locked.
func TestAcceptChecksAndLiftsInOneStep(t *testing.T) {
	g, ui, peers, _ := setupConfig(t, Config{Intent: LaunchIntent{Resume: true, Thread: "X"}, Name: "worker"})
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	exchange(t, ui, server, startY, startedY)
	_ = warning(t, ui, "Y")
	c := g.currentConnection()
	g.intent.mu.Lock()
	done := make(chan error, 1)
	go func() { done <- g.Accept("Y") }()
	reached := false
	for range 200 {
		stack := make([]byte, 1<<20)
		if n := runtime.Stack(stack, true); strings.Contains(string(stack[:n]), "(*Gateway).Accept(") {
			reached = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !reached {
		g.intent.mu.Unlock()
		t.Fatal("the acceptance did not start")
	}
	time.Sleep(20 * time.Millisecond)
	changed := c.mu.TryLock()
	if changed {
		c.state.Generation++
		c.state.Thread = "Z"
		c.mu.Unlock()
	}
	g.intent.mu.Unlock()
	err := <-done
	if changed && (err == nil || g.Hold() == nil) {
		t.Fatalf("the selection changed to Z while Y was accepted: %v", err)
	}
	if !changed && err != nil {
		t.Fatal(err)
	}
}

// The mail is admitted before the hold ends; an admission that cannot be
// recorded keeps the hold, and a later publication records it.
func TestTheLiftAdmitsTheMailFirst(t *testing.T) {
	var admitted, failing atomic.Bool
	failing.Store(true)
	admit := func() error {
		if failing.Load() {
			return errors.New("disk full")
		}
		admitted.Store(true)
		return nil
	}
	g, ui, peers, _ := setupConfig(t, Config{Intent: LaunchIntent{Resume: true, Thread: "X"}, Name: "worker", Admit: admit})
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	refuseX(t, ui, server)
	exchange(t, ui, server, startY, startedY)
	_ = warning(t, ui, "Y")
	if err := g.Accept("Y"); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("accepted without admitting the mail: %v", err)
	}
	_ = reserveHeld(t, g)
	exchange(t, ui, server, `{"id":3,"method":"thread/resume","params":{"threadId":"X","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":3,"result":{"thread":{"id":"X","canAcceptDirectInput":true}}}`)
	if shown := warning(t, ui, "X"); !strings.Contains(shown, "disk full") {
		t.Fatalf("the terminal was not told why the mail stays closed: %s", shown)
	}
	exchange(t, ui, server, `{"id":5,"method":"thread/goal/get","params":{"threadId":"X"}}`, `{"id":5,"result":{}}`)
	// The intended conversation is selected and the mail is not open: a
	// delivery now would reach a worker whose inbox refuses (acceptance of
	// September 30, 2026, finding 1).
	if held := reserveHeld(t, g); held.Hold.Reason != sessionstate.HoldMailClosed || !strings.Contains(held.Hold.Detail, "disk full") {
		t.Fatalf("%+v", held.Hold)
	}
	if hold := g.SessionState().DeliveryHold; hold == nil || hold.Reason != sessionstate.HoldMailClosed {
		t.Fatalf("the failed admission was not published: %+v", hold)
	}
	failing.Store(false)
	_ = g.SessionState()
	if g.intentWaiting() || !admitted.Load() {
		t.Fatal("the lift was not recorded on the next publication")
	}
	if hold := g.SessionState().DeliveryHold; hold != nil {
		t.Fatalf("%+v", hold)
	}
	reserveFree(t, g, "X")
}
