package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPickerOverlapDoesNotReplacePrimaryOrReceiveItsReply(t *testing.T) {
	g, ui, peers, path := setup(t)
	main := <-peers
	defer func() { _ = main.conn.Close() }()
	bindUI(t, g, ui, main)
	binding := g.Binding()
	picker, e := dialSocket(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = picker.conn.Close() }()
	aux := <-peers
	defer func() { _ = aux.conn.Close() }()
	write(t, picker, []byte(`{"id":1,"method":"thread/list","params":{}}`))
	_ = readWithin(t, aux)
	write(t, aux, []byte(`{"id":1,"result":{"data":[]}}`))
	_ = readWithin(t, picker)
	if g.Binding() != binding {
		t.Fatal("picker changed ownership")
	}
	result := make(chan error, 1)
	go func() { _, err := g.Deliver(context.Background(), binding, "notice", "fixture"); result <- err }()
	m := metadata(t, string(readWithin(t, main)))
	write(t, main, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"T"}}}`, m.idText)))
	if e = <-result; e != nil {
		t.Fatal(e)
	}
	// A picker shutdown affects only that pair. The primary can still make requests.
	_ = picker.conn.Close()
	write(t, ui, []byte(`{"id":2,"method":"thread/list","params":{}}`))
	_ = readWithin(t, main)
	if g.Binding() != binding {
		t.Fatal("picker exit invalidated primary")
	}
}

func TestCompetingIntentFailsUnavailableWithoutClosingEitherSocket(t *testing.T) {
	g, ui, peers, path := setup(t)
	main := <-peers
	defer func() { _ = main.conn.Close() }()
	bindUI(t, g, ui, main)
	old := g.Binding()
	other, e := dialSocket(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = other.conn.Close() }()
	aux := <-peers
	defer func() { _ = aux.conn.Close() }()
	write(t, other, []byte(startA))
	_ = readWithin(t, aux)
	write(t, aux, []byte(`{"id":1,"result":{"thread":{"id":"B","canAcceptDirectInput":true}}}`))
	_ = readWithin(t, other)
	if g.Binding().Ready {
		t.Fatal("competing primary chose a winner")
	}
	if _, e = g.Deliver(context.Background(), old, "id", "notice"); e == nil {
		t.Fatal("old fence delivered during conflict")
	}
	write(t, ui, []byte(`{"id":2,"method":"thread/list"}`))
	_ = readWithin(t, main)
	write(t, other, []byte(`{"id":2,"method":"thread/list"}`))
	_ = readWithin(t, aux)
}

func TestOverviewUUIDReadsDoNotCancelStartupIntent(t *testing.T) {
	s := newState("epoch", 1)
	req(t, &s, `{"id":"startup-thread-start-fixture","method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`)
	req(t, &s, `{"id":"12345678-abcd-4321-aaaa-123456789abc","method":"thread/read","params":{"threadId":"seed-A","includeTurns":false}}`)
	answer(t, &s, `"startup-thread-start-fixture"`, "fresh")
	if !s.Ready || s.Thread != "fresh" {
		t.Fatal("background canceled startup", s.Binding)
	}
	req(t, &s, `{"id":2,"method":"thread/read","params":{"threadId":"seed-A","includeTurns":false}}`)
	if s.Ready {
		t.Fatal("interactive ambiguous read silently ignored")
	}
}

func TestSocketEarlierGapSurvivesFollowingCompletion(t *testing.T) {
	g, ui, peers, _ := setup(t)
	main := <-peers
	defer func() { _ = main.conn.Close() }()
	bindUI(t, g, ui, main)
	for _, raw := range []string{
		`{"method":"turn/started","params":{"threadId":"A","turn":{"id":"first"}}}`,
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`,
		`{"method":"turn/started","params":{"threadId":"A","turn":{"id":"second"}}}`,
		`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"second","status":"completed"}}}`,
	} {
		write(t, main, []byte(raw))
		_ = readWithin(t, ui)
	}
	c := g.currentConnection()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.events.expire(time.Now().Add(time.Second))
	out := c.state.events.drain()
	if len(out) != 1 || out[0].ID != "A/first" || out[0].Text != "completion not observed" {
		t.Fatal(out)
	}
}

func TestAuxiliaryGlobalEventsDoNotRetainObservationIntervals(t *testing.T) {
	g, ui, peers, path := setup(t)
	main := <-peers
	defer func() { _ = main.conn.Close() }()
	bindUI(t, g, ui, main)
	picker, e := dialSocket(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = picker.conn.Close() }()
	aux := <-peers
	defer func() { _ = aux.conn.Close() }()
	write(t, picker, []byte(`{"method":"initialized"}`))
	_ = readWithin(t, aux)
	for i := 0; i <= 64; i++ {
		write(t, aux, []byte(fmt.Sprintf(`{"method":"thread/status/changed","params":{"threadId":"other-%d","status":{"type":"active"}}}`, i)))
		_ = readWithin(t, picker)
	}
	g.mu.Lock()
	var helper *connection
	for c := range g.conns {
		if c != g.current {
			helper = c
		}
	}
	g.mu.Unlock()
	if helper == nil {
		t.Fatal("helper was closed by global noise")
	}
	helper.mu.Lock()
	defer helper.mu.Unlock()
	if len(helper.state.events.intervals) != 0 {
		t.Fatal("auxiliary socket retained global intervals")
	}
}
