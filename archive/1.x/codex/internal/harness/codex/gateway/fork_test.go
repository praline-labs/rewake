package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func forkPair(t *testing.T, ui, native *socketClient, id int, parent, child string) {
	t.Helper()
	exchange(t, ui, native, fmt.Sprintf(`{"id":%d,"method":"thread/fork","params":{"threadId":%q,"threadSource":"user","config":{},"runtimeWorkspaceRoots":[]}}`, id, parent), fmt.Sprintf(`{"id":%d,"result":{"thread":{"id":%q,"canAcceptDirectInput":true,"status":{"type":"idle"}}}}`, id, child))
}

func assertTarget(t *testing.T, g *Gateway, thread string) {
	t.Helper()
	b := g.Binding()
	if !b.Ready || b.Thread != thread {
		t.Fatalf("want %s: %+v", thread, b)
	}
}

func TestCLIForkRequiresExplicitLaunchContext(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			g, ui, peers, _ := setup(t, enabled)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			exchange(t, ui, native, `{"id":0,"method":"initialize"}`, `{"id":0,"result":{}}`)
			exchange(t, ui, native, `{"id":1,"method":"thread/read","params":{"threadId":"parent"}}`, `{"id":1,"result":{"thread":{"id":"parent","canAcceptDirectInput":true}}}`)
			forkPair(t, ui, native, 2, "parent", "child")
			if enabled {
				assertTarget(t, g, "child")
			} else if g.Binding().Ready {
				t.Fatal("unconfigured fork promoted a primary")
			}
		})
	}
}

func TestOrdinaryForkRequiresConfirmedParentDetach(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	old := g.Binding()
	forkPair(t, ui, native, 2, "A", "fork")
	if g.Binding().Ready {
		t.Fatal("creation alone selected fork")
	}
	// Hydration and optional naming preserve a candidate without authorizing work.
	exchange(t, ui, native, `{"id":3,"method":"thread/read","params":{"threadId":"fork","includeTurns":true}}`, `{"id":3,"result":{"thread":{"id":"fork"}}}`)
	exchange(t, ui, native, `{"id":4,"method":"thread/name/set","params":{"threadId":"fork","name":"test"}}`, `{"id":4,"result":{}}`)
	write(t, ui, []byte(`{"id":5,"method":"thread/unsubscribe","params":{"threadId":"A"}}`))
	_ = readWithin(t, native)
	write(t, native, []byte(`{"id":99,"result":{"status":"unsubscribed"}}`))
	_ = readWithin(t, ui)
	if g.Binding().Ready {
		t.Fatal("unrelated detach reply selected fork")
	}
	write(t, native, []byte(`{"id":5,"result":{"status":"unsubscribed"}}`))
	_ = readWithin(t, ui)
	assertTarget(t, g, "fork")
	if g.Binding().Generation == old.Generation {
		t.Fatal("fork reused old generation")
	}
	if _, err := g.Deliver(context.Background(), old, "old", "notice"); err == nil {
		t.Fatal("old parent binding admitted after fork")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := g.Reserve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
}

func TestForkCandidateRejectsUnprovedOrInterruptedSequences(t *testing.T) {
	cases := map[string][]string{
		"read-only child":          {`{"id":2,"result":{"thread":{"id":"B","canAcceptDirectInput":false}}}`, `{"id":3,"method":"thread/unsubscribe","params":{"threadId":"A"}}`, `{"id":3,"result":{"status":"unsubscribed"}}`},
		"same child":               {`{"id":2,"result":{"thread":{"id":"A","canAcceptDirectInput":true}}}`},
		"unknown child":            {`{"id":2,"result":{"thread":{"id":"B"}}}`},
		"wrong parent detach":      {`{"id":2,"result":{"thread":{"id":"B","canAcceptDirectInput":true}}}`, `{"id":3,"method":"thread/unsubscribe","params":{"threadId":"other"}}`, `{"id":3,"result":{"status":"unsubscribed"}}`},
		"detach refused":           {`{"id":2,"result":{"thread":{"id":"B","canAcceptDirectInput":true}}}`, `{"id":3,"method":"thread/unsubscribe","params":{"threadId":"A"}}`, `{"id":3,"error":{"code":-32600}}`},
		"detach lacks status":      {`{"id":2,"result":{"thread":{"id":"B","canAcceptDirectInput":true}}}`, `{"id":3,"method":"thread/unsubscribe","params":{"threadId":"A"}}`, `{"id":3,"result":{}}`},
		"side setup refused":       {`{"id":2,"result":{"thread":{"id":"B","canAcceptDirectInput":true}}}`, `{"id":3,"method":"thread/inject_items","params":{"threadId":"B"}}`, `{"id":3,"error":{"code":-32600}}`},
		"unrelated read":           {`{"id":2,"result":{"thread":{"id":"B","canAcceptDirectInput":true}}}`, `{"id":3,"method":"thread/read","params":{"threadId":"other"}}`, `{"id":4,"method":"thread/unsubscribe","params":{"threadId":"A"}}`, `{"id":4,"result":{"status":"unsubscribed"}}`},
		"detach and side conflict": {`{"id":2,"result":{"thread":{"id":"B","canAcceptDirectInput":true}}}`, `{"id":3,"method":"thread/unsubscribe","params":{"threadId":"A"}}`, `{"id":4,"method":"thread/inject_items","params":{"threadId":"B"}}`, `{"id":3,"result":{"status":"unsubscribed"}}`},
	}
	for name, rows := range cases {
		t.Run(name, func(t *testing.T) {
			s := newState("epoch", 1)
			req(t, &s, startA)
			answer(t, &s, "1", "A")
			req(t, &s, `{"id":2,"method":"thread/fork","params":{"threadId":"A","threadSource":"user","config":{},"runtimeWorkspaceRoots":[]}}`)
			for _, raw := range rows {
				m := metadata(t, raw)
				if m.method != "" {
					if err := s.request(m); err != nil {
						t.Fatal(err)
					}
				} else {
					s.response(m)
				}
			}
			if s.Ready {
				t.Fatal("unproved fork workflow chose a destination", s.Binding)
			}
		})
	}
}

func TestSidePreservesPrimaryAcrossWorkAndClose(t *testing.T) {
	g, ui, peers, _ := setup(t, true)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	exchange(t, ui, native, `{"id":0,"method":"initialize"}`, `{"id":0,"result":{}}`)
	forkPair(t, ui, native, 1, "seed", "A")
	assertTarget(t, g, "A")
	// Explicit startup authority is consumed; a later fork is not another CLI launch.
	forkPair(t, ui, native, 2, "A", "side")
	if g.Binding().Ready {
		t.Fatal("side creation was mistaken for another startup fork")
	}
	exchange(t, ui, native, `{"id":3,"method":"thread/inject_items","params":{"threadId":"side","items":[]}}`, `{"id":3,"result":{}}`)
	assertTarget(t, g, "A")
	binding := g.Binding()
	exchange(t, ui, native, `{"id":4,"method":"turn/start","params":{"threadId":"side"}}`, `{"id":4,"result":{"turn":{"id":"side-turn"}}}`)
	if g.Binding() != binding {
		t.Fatal("side question changed the address")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := g.Reserve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Prepare(func(thread string) error {
		if thread != "A" {
			t.Fatal("message addressed side")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r.Close()
	exchange(t, ui, native, `{"id":5,"method":"thread/read","params":{"threadId":"A"}}`, `{"id":5,"result":{"thread":{"id":"A","canAcceptDirectInput":true}}}`)
	exchange(t, ui, native, `{"id":6,"method":"turn/interrupt","params":{"threadId":"side","turnId":"side-turn"}}`, `{"id":6,"result":{}}`)
	exchange(t, ui, native, `{"id":7,"method":"thread/unsubscribe","params":{"threadId":"side"}}`, `{"id":7,"result":{"status":"unsubscribed"}}`)
	if g.Binding() != binding {
		t.Fatal("closing side required another primary resume")
	}
}

func TestLateForkRepliesCannotOverrideANewerPrimary(t *testing.T) {
	s := newState("epoch", 1)
	req(t, &s, startA)
	answer(t, &s, "1", "A")
	req(t, &s, `{"id":2,"method":"thread/fork","params":{"threadId":"A","threadSource":"user","config":{},"runtimeWorkspaceRoots":[]}}`)
	answer(t, &s, "2", "B")
	req(t, &s, `{"id":3,"method":"thread/unsubscribe","params":{"threadId":"A"}}`)
	req(t, &s, `{"id":4,"method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`)
	answer(t, &s, "4", "C")
	s.response(metadata(t, `{"id":3,"result":{"status":"unsubscribed"}}`))
	if !s.Ready || s.Thread != "C" {
		t.Fatal("late fork detach revived an obsolete child", s.Binding)
	}
}

func TestSideSetupCannotRestoreAClosedPrimary(t *testing.T) {
	s := newState("epoch", 1)
	req(t, &s, startA)
	answer(t, &s, "1", "A")
	req(t, &s, `{"id":2,"method":"thread/fork","params":{"threadId":"A","threadSource":"user","config":{},"runtimeWorkspaceRoots":[]}}`)
	answer(t, &s, "2", "B")
	req(t, &s, `{"id":3,"method":"thread/inject_items","params":{"threadId":"B","items":[]}}`)
	s.closedThread("A")
	s.response(metadata(t, `{"id":3,"result":{}}`))
	if s.Ready {
		t.Fatal("side setup revived a closed primary")
	}
}

func TestSelectedPermissionProfileKeepsExplicitPrimaryIntents(t *testing.T) {
	s := newState("epoch", 1)
	req(t, &s, `{"id":1,"method":"thread/start","params":{"threadSource":"user","permissions":":read-only"}}`)
	answer(t, &s, "1", "A")
	if !s.Ready {
		t.Fatal("explicit profile replaced roots and lost startup authority")
	}
	req(t, &s, `{"id":2,"method":"thread/fork","params":{"threadId":"A","threadSource":"user","config":{},"permissions":":read-only"}}`)
	answer(t, &s, "2", "B")
	req(t, &s, `{"id":3,"method":"thread/unsubscribe","params":{"threadId":"A"}}`)
	s.response(metadata(t, `{"id":3,"result":{"status":"unsubscribed"}}`))
	if !s.Ready || s.Thread != "B" {
		t.Fatal("explicit profile lost ordinary fork", s.Binding)
	}
}
