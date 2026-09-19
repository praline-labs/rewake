package codex

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

func TestMailDuringNativeACKFormsNextExactGroup(t *testing.T) {
	f, first, release := nativePendingBatches(t)
	var next []inbox.Message
	for _, kind := range []inbox.Kind{inbox.Task, inbox.Question, inbox.Note, inbox.Finished} {
		next = append(next, f.put(t, kind, "queued-"+string(kind)))
	}
	unread, err := inbox.PeekUnread(f.dir, f.server.Name, f.server.Epoch)
	if err != nil || len(unread) != 2 {
		t.Fatal("in-flight arrivals joined old readable group", err)
	}
	select {
	case <-f.captured:
		t.Fatal("parallel dispatch bypassed reservation ACK")
	default:
	}
	release()
	// ACK completion alone permits the next dispatch. No native terminal, status
	// transition, owner input or inbox overview is provided.
	f.notice(t, next)
	f.wait(t, next, inbox.Delivered)
	f.wait(t, first, inbox.Delivered)
	unread, err = inbox.PeekUnread(f.dir, f.server.Name, f.server.Epoch)
	if err != nil || len(unread) != 6 {
		t.Fatalf("unread=%d err=%v", len(unread), err)
	}
	members := map[string]bool{}
	for _, m := range append(first, next...) {
		members[m.ID] = true
	}
	for _, m := range unread {
		if !members[m.ID] {
			t.Fatalf("unexpected unread ID %s", m.ID)
		}
		delete(members, m.ID)
	}
	if len(members) != 0 {
		t.Fatal("lost independent unread members")
	}
	select {
	case <-f.captured:
		t.Fatal("dispatch replayed old announced work")
	case <-time.After(1100 * time.Millisecond):
	}
}

func nativePendingBatches(t *testing.T) (*reviewBatchFixture, []inbox.Message, func()) {
	t.Helper()
	peers := make(chan net.Conn, 1)
	ack := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(ack) }) }
	firstACK := true
	backend, captured := gitDeliveryFixtureTraffic(t, role.Write, "active", func() {
		if firstACK {
			firstACK = false
			<-ack
		}
	}, func(c net.Conn) { peers <- c })
	f := &reviewBatchFixture{dir: t.TempDir(), backend: backend, native: <-peers, captured: captured}
	if err := f.native.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	f.server = &inbox.Server{Dir: f.dir, Name: "receiver", Epoch: "epoch", Reserve: backend.Reserve}
	first := []inbox.Message{f.put(t, inbox.Note, "old-one"), f.put(t, inbox.Note, "old-two")}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); f.server.Serve(ctx) }()
	t.Cleanup(func() { release(); cancel(); <-done })
	f.notice(t, first)
	return f, first, release
}

func TestNativeReadinessCollectsWaitingMixedArrivals(t *testing.T) {
	backend, captured := gitDeliveryFixtureMode(t, role.Write, "idle", true, nil, nil)
	f := &reviewBatchFixture{dir: t.TempDir(), backend: backend, captured: captured}
	entered, ready := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(ready) }) }
	firstReserve := true
	f.server = &inbox.Server{Dir: f.dir, Name: "receiver", Epoch: "epoch", Reserve: func(ctx context.Context, m inbox.Message) (inbox.Reservation, error) {
		if firstReserve {
			firstReserve = false
			close(entered)
			select {
			case <-ready:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return backend.Reserve(ctx, m)
	}}
	members := []inbox.Message{f.put(t, inbox.Note, "first")}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); f.server.Serve(ctx) }()
	t.Cleanup(func() { release(); cancel(); <-done })
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("readiness was not requested")
	}
	for _, kind := range []inbox.Kind{inbox.Task, inbox.Question, inbox.Note, inbox.Finished} {
		members = append(members, f.put(t, kind, "waiting-"+string(kind)))
	}
	if unread, err := inbox.PeekUnread(f.dir, f.server.Name, f.server.Epoch); err != nil || len(unread) != 0 {
		t.Fatal("unreserved mail became readable", err)
	}
	release()
	f.notice(t, members)
	f.wait(t, members, inbox.Delivered)
}
