package pi

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

type rig struct {
	run    *run
	live   chan []adapter.Capability
	ends   chan adapter.TurnEnd
	starts chan int64
}

func newRig(t *testing.T, edit func(*run)) *rig {
	t.Helper()
	base, err := os.MkdirTemp("", "pi-wire-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	dir := buildtest.Room(t, base, "default")
	r := newRun(filepath.Join(dir, "extension.ts"), filepath.Join(dir, "p.sock"))
	rig := &rig{run: r, live: make(chan []adapter.Capability, 16), ends: make(chan adapter.TurnEnd, 16), starts: make(chan int64, 16)}
	r.ReportLive(func(caps []adapter.Capability) { rig.live <- caps })
	r.ReportTurns(adapter.TurnHandler{
		Started: func(_, _ string, at int64) { rig.starts <- at },
		Capture: func() *mail.ReadBoundary { return &mail.ReadBoundary{Through: 7} },
		Publish: func(_ context.Context, end adapter.TurnEnd) error { rig.ends <- end; return nil },
	})
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	r.HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: start})
	if edit != nil {
		edit(r)
	}
	if err := r.Start(t.Context(), func(string) {}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return rig
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(3 * time.Second):
		t.Fatal("exchange timed out")
		var zero T
		return zero
	}
}

func (rig *rig) client(t *testing.T, thread string, handle func(*link, frame)) *link {
	t.Helper()
	conn, err := net.Dial("unix", rig.run.socket)
	if err != nil {
		t.Fatal(err)
	}
	l := newLink(conn)
	t.Cleanup(l.close)
	go l.serve(bufio.NewReader(conn), func(got frame) {
		if handle != nil {
			handle(l, got)
		} else {
			l.answer(got, frame{OK: true})
		}
	})
	reply, err := l.ask(t.Context(), frame{Op: "hello", Protocol: protocolVersion, State: &sample{Sequence: 1, Activity: "idle"}, Thread: thread})
	if err != nil || !reply.OK {
		t.Fatalf("hello: %+v, %v", reply, err)
	}
	expected := 3
	if rig.run.controlDir != "" {
		expected++
	}
	if caps := receive(t, rig.live); len(caps) != expected {
		t.Fatalf("live: %v", caps)
	}
	return l
}

func askOK(t *testing.T, l *link, request frame) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	reply, err := l.ask(ctx, request)
	if err != nil || !reply.OK {
		t.Fatalf("%s: %+v, %v", request.Op, reply, err)
	}
}

func TestTheWireCarriesNoticesAndSettledEnds(t *testing.T) {
	rig := newRig(t, nil)
	if result := rig.run.Deliver(t.Context(), mail.Message{}); result.State != mail.Pending {
		t.Fatalf("before hello: %+v", result)
	}
	notices := make(chan frame, 4)
	l := rig.client(t, "conversation", func(l *link, got frame) { notices <- got; l.answer(got, frame{OK: true, Steered: true}) })
	result := rig.run.Deliver(t.Context(), mail.Message{ID: "letter", From: "main", DeliveryThread: "conversation"})
	notice := receive(t, notices)
	if result.State != mail.Delivered || notice.Op != "wake" || notice.Notice == "" || notice.Thread != "conversation" {
		t.Fatalf("delivery: %+v %+v", result, notice)
	}
	for _, reason := range []string{"answered", "aborted", "failed"} {
		askOK(t, l, frame{Op: "turn-start", Thread: "conversation", Turn: reason})
		if at := receive(t, rig.starts); at != 0 {
			t.Fatalf("arrival invented a native clock: %d", at)
		}
		askOK(t, l, frame{Op: "turn-end", Thread: "conversation", Turn: reason, Outcome: reason, Text: "unchanged final text"})
		end := receive(t, rig.ends)
		if end.Reason != adapter.EndReason(reason) || end.Answer != "unchanged final text" || end.Thread != "conversation" || end.Event != ID+"/"+reason || end.Boundary.Through != 7 || end.Started <= 0 || end.Ended < end.Started {
			t.Fatalf("end: %+v", end)
		}
	}
	l.close()
	if caps := receive(t, rig.live); len(caps) != 0 {
		t.Fatalf("disconnect kept live: %v", caps)
	}
	if _, err := rig.run.Thread(); !errors.Is(err, mail.ErrThreadUnavailable) {
		t.Fatalf("thread after disconnect: %v", err)
	}
}

func TestAReplacementConnectionOwnsTheConversation(t *testing.T) {
	rig := newRig(t, nil)
	old := rig.client(t, "old", nil)
	fresh := rig.client(t, "fresh", nil)
	receive(t, old.closed)
	thread, err := rig.run.Thread()
	if err != nil || thread != "fresh" {
		t.Fatalf("replacement: %q %v", thread, err)
	}
	if result := rig.run.Deliver(t.Context(), mail.Message{DeliveryThread: "old"}); result.State != mail.Failed {
		t.Fatalf("stale delivery: %+v", result)
	}
	askOK(t, fresh, frame{Op: "turn-start", Thread: "fresh", Turn: "new"})
	reply, err := fresh.ask(t.Context(), frame{Op: "turn-end", Thread: "old", Turn: "new", Outcome: "answered"})
	if err != nil || reply.OK {
		t.Fatalf("stale end: %+v %v", reply, err)
	}
}

func TestBadHelloAndWrongPeerNeverBecomeLive(t *testing.T) {
	for _, which := range []string{"protocol", "thread", "state", "peer", "process-start"} {
		t.Run(which, func(t *testing.T) {
			rig := newRig(t, func(r *run) {
				if which == "peer" {
					r.process.PID++
				}
				if which == "process-start" {
					r.process.Start++
				}
			})
			conn, err := net.Dial("unix", rig.run.socket)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			l := newLink(conn)
			hello := frame{Op: "hello", ID: 1, Protocol: protocolVersion, State: &sample{Sequence: 1, Activity: "idle"}, Thread: "conversation"}
			if which == "protocol" {
				hello.Protocol = 1
			}
			if which == "thread" {
				hello.Thread = ""
			}
			if which == "state" {
				hello.State = nil
			}
			if err := l.send(hello); err != nil && which != "peer" && which != "process-start" {
				t.Fatal(err)
			}
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))
			reply, err := readFrame(bufio.NewReader(conn))
			if err == nil && reply.OK {
				t.Fatal("bad peer or hello admitted")
			}
			select {
			case caps := <-rig.live:
				t.Fatalf("bad hello became live: %v", caps)
			default:
			}
		})
	}
}

func TestEndPublicationKeepsItsCaptureAndDoesNotBlockWakeAnswers(t *testing.T) {
	var calls atomic.Int32
	var through atomic.Uint64
	through.Store(11)
	blocked := make(chan struct{})
	release := make(chan struct{})
	rig := newRig(t, func(r *run) {
		r.handler.Capture = func() *mail.ReadBoundary { return &mail.ReadBoundary{Through: through.Load()} }
		r.handler.Publish = func(_ context.Context, end adapter.TurnEnd) error {
			if end.Boundary.Through != 11 {
				t.Errorf("retry widened capture: %d", end.Boundary.Through)
			}
			if calls.Add(1) == 1 {
				close(blocked)
				<-release
				return errors.New("temporary publish failure")
			}
			return nil
		}
	})
	l := rig.client(t, "conversation", nil)
	askOK(t, l, frame{Op: "turn-start", Thread: "conversation", Turn: "one"})
	done := make(chan frame, 1)
	go func() {
		reply, _ := l.ask(t.Context(), frame{Op: "turn-end", Thread: "conversation", Turn: "one", Outcome: "answered"})
		done <- reply
	}()
	receive(t, blocked)
	through.Store(99)
	result := rig.run.Deliver(t.Context(), mail.Message{DeliveryThread: "conversation"})
	close(release)
	if result.State != mail.Delivered {
		t.Fatalf("reader blocked during publication: %+v", result)
	}
	if reply := receive(t, done); !reply.OK {
		t.Fatalf("end: %+v", reply)
	}
	if calls.Load() != 2 {
		t.Fatalf("publication calls: %d", calls.Load())
	}
}

func TestAnUnacknowledgedNoticeIsNotRetried(t *testing.T) {
	rig := newRig(t, nil)
	l := rig.client(t, "conversation", func(l *link, _ frame) { l.close() })
	result := rig.run.Deliver(t.Context(), mail.Message{DeliveryThread: "conversation"})
	if result.State != mail.Failed {
		t.Fatalf("unknown delivery would be retried: %+v", result)
	}
	receive(t, l.closed)
}

func TestHelloIsAcknowledgedBeforeWakeBecomesLive(t *testing.T) {
	rig := newRig(t, func(r *run) {
		r.ReportLive(func(caps []adapter.Capability) {
			if len(caps) == 0 {
				return
			}
			// Delivery is allowed as soon as the host sees live Wake. Put its
			// first request on the wire at that exact point.
			l, thread := r.connection()
			if err := l.send(frame{Op: "wake", ID: 1, Thread: thread, Notice: "ready mail"}); err != nil {
				t.Error(err)
			}
		})
	})
	conn, err := net.Dial("unix", rig.run.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	l := newLink(conn)
	if err := l.send(frame{Op: "hello", ID: 1, Protocol: protocolVersion, State: &sample{Sequence: 1, Activity: "idle"}, Thread: "conversation"}); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	reader := bufio.NewReader(conn)
	first, err := readFrame(reader)
	if err != nil || first.Op != "answer" || !first.OK {
		t.Fatalf("wake raced hello: %+v %v", first, err)
	}
	second, err := readFrame(reader)
	if err != nil || second.Op != "wake" {
		t.Fatalf("live wake: %+v %v", second, err)
	}
}
