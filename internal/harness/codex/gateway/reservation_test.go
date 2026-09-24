package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestReservationWaitsOutsideAdmissionForResumeReads(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	socketResume(t, ui, server)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := make(chan reserveResult, 1)
	go func() { r, err := g.Reserve(ctx); result <- reserveResult{r, err} }()
	// No request or gate is held: inventory, backfill and approval replies still flow.
	exchange(t, ui, server, `{"id":3,"method":"thread/loaded/list","params":{}}`, `{"id":3,"result":{"data":["A","B","C"]}}`)
	exchange(t, ui, server, `{"id":4,"method":"thread/read","params":{"threadId":"B"}}`, `{"id":4,"result":{"thread":{"id":"B"}}}`)
	select {
	case <-result:
		t.Fatal("reserved before final backfill")
	default:
	}
	write(t, ui, []byte(`{"id":90,"result":{"decision":"accept"}}`))
	if m := metadata(t, string(readWithin(t, server))); m.id != "n:90" {
		t.Fatal("approval blocked", m)
	}
	exchange(t, ui, server, `{"id":5,"method":"thread/read","params":{"threadId":"C"}}`, `{"id":5,"result":{"thread":{"id":"C"}}}`)
	reserved := <-result
	if reserved.err != nil {
		t.Fatal(reserved.err)
	}
	defer reserved.reservation.Close()
	done := make(chan error, 1)
	go func() {
		_, err := reserved.reservation.Deliver(ctx, "task", MailboxNotice{Notice: "notice"}, nil)
		done <- err
	}()
	injection := metadata(t, string(readWithin(t, server)))
	if injection.thread != "A" {
		t.Fatal(injection)
	}
	write(t, server, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"work"}}}`, injection.idText)))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	noticeDisplayParams(t, readWithin(t, ui))
	if !g.Binding().Ready {
		t.Fatal("ordinary overlap invalidated routing")
	}
}

func TestReservationCoversReadabilityAndQueuedABASwitch(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := g.Reserve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Prepare(func(thread string) error {
		if thread != "A" {
			t.Fatal(thread)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	write(t, ui, []byte(`{"id":2,"method":"thread/resume","params":{"threadId":"B","config":{},"runtimeWorkspaceRoots":[]}}`))
	done := make(chan error, 1)
	go func() { _, err := r.Deliver(ctx, "id", MailboxNotice{Notice: "notice"}, nil); done <- err }()
	m := metadata(t, string(readWithin(t, server)))
	if m.method != "turn/start" || m.thread != "A" {
		t.Fatal("selection overtook reserved work", m)
	}
	write(t, server, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"work"}}}`, m.idText)))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	noticeDisplayParams(t, readWithin(t, ui))
	r.Close()
	_ = readWithin(t, server)
	write(t, server, []byte(`{"id":2,"result":{"thread":{"id":"B","canAcceptDirectInput":true}}}`))
	_ = readWithin(t, ui)
	exchange(t, ui, server, `{"id":3,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":3,"result":{"thread":{"id":"A","canAcceptDirectInput":true}}}`)
	if _, err := r.Deliver(ctx, "late", MailboxNotice{Notice: "notice"}, nil); err == nil {
		t.Fatal("old A reservation survived A-B-A")
	}
}

func TestReservationDeadlineSendsNothingAndKeepsNativeConnection(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	socketResume(t, ui, server)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := g.Reserve(ctx); err == nil {
		t.Fatal("open read phase admitted")
	}
	exchange(t, ui, server, `{"id":3,"method":"thread/goal/get","params":{"threadId":"A"}}`, `{"id":3,"result":{}}`)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	r, err := g.Reserve(ctx2)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
}

func TestReservationExpiresWithoutHoldingNativeLifecycle(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	r, err := g.Reserve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	<-ctx.Done()
	exchange(t, ui, server, `{"id":2,"method":"thread/list"}`, `{"id":2,"result":{}}`)
	if err := r.Prepare(func(string) error { t.Fatal("expired reservation published unread mail"); return nil }); err == nil {
		t.Fatal("expired reservation accepted")
	}
}

func TestReservationWaitsForNativeSettingsACKWithoutBlockingApprovals(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	write(t, ui, []byte(`{"id":2,"method":"thread/settings/update","params":{"threadId":"A","permissions":"read-only"}}`))
	_ = readWithin(t, native)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan reserveResult, 1)
	go func() { r, err := g.Reserve(ctx); result <- reserveResult{r, err} }()
	write(t, ui, []byte(`{"id":90,"result":{"decision":"accept"}}`))
	if m := metadata(t, string(readWithin(t, native))); m.id != "n:90" {
		t.Fatal("settings wait blocked approval")
	}
	select {
	case <-result:
		t.Fatal("reserved before settings ACK")
	default:
	}
	write(t, native, []byte(`{"id":2,"result":{}}`))
	_ = readWithin(t, ui)
	reserved := <-result
	if reserved.err != nil {
		t.Fatal(reserved.err)
	}
	reserved.reservation.Close()
	if !g.Binding().Ready {
		t.Fatal("settings update lost selected conversation")
	}
}

func TestReservationRefusesCapacityAndMaintenanceBeforeReadability(t *testing.T) {
	for _, maintenance := range []bool{false, true} {
		t.Run(fmt.Sprint(maintenance), func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			c := g.currentConnection()
			c.mu.Lock()
			if maintenance {
				c.admitted.manual["A"] = &manualWork{hold: time.Now().Add(time.Minute)}
			} else {
				for i := 0; i < 64; i++ {
					c.admitted.pending[fmt.Sprint(i)] = admittedRequest{binding: c.state.Binding}
				}
			}
			c.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if r, err := g.Reserve(ctx); err == nil {
				r.Close()
				t.Fatal("unavailable ledger admitted readable work")
			}
			exchange(t, ui, native, `{"id":2,"method":"thread/list"}`, `{"id":2,"result":{}}`)
		})
	}
}

func TestUnusedReservationsReleaseLedgerCapacity(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for i := 0; i < 80; i++ {
		r, err := g.Reserve(ctx)
		if err != nil {
			t.Fatal(err)
		}
		r.Close()
	}
	c := g.currentConnection()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.admitted.pending) != 0 {
		t.Fatal("unused reservations retained admission capacity")
	}
}

func TestReservationScopeChangesAcrossNewAndRepeatedResume(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	scopes := map[Binding]bool{}
	capture := func() {
		t.Helper()
		r, err := g.Reserve(ctx)
		if err != nil {
			t.Fatal(err)
		}
		scope := r.binding
		r.Close()
		if scopes[scope] {
			t.Fatalf("reused reservation scope %+v", scope)
		}
		scopes[scope] = true
	}
	capture()
	exchange(t, ui, server, `{"id":2,"method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`, `{"id":2,"result":{"thread":{"id":"B","canAcceptDirectInput":true}}}`)
	capture()
	for _, id := range []int{3, 5} {
		exchange(t, ui, server, fmt.Sprintf(`{"id":%d,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, id), fmt.Sprintf(`{"id":%d,"result":{"thread":{"id":"A","canAcceptDirectInput":true}}}`, id))
		exchange(t, ui, server, fmt.Sprintf(`{"id":%d,"method":"thread/goal/get","params":{"threadId":"A"}}`, id+1), fmt.Sprintf(`{"id":%d,"result":{}}`, id+1))
		capture()
	}
}
