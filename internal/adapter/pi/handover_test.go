package pi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
)

// busyWith waits until the run's Busy names why, within a bound.
func busyWith(t *testing.T, r *run, why string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(r.Busy(), why) {
		if time.Now().After(deadline) {
			t.Fatalf("Busy says %q, want %q", r.Busy(), why)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := r.HandOver(); err == nil {
		t.Fatalf("handed over while %s", why)
	}
}

// The extension's one connection is handed over with the listener, and the
// next image serves it as the run's current one: the extension's next turn
// reaches the core, a notice reaches the extension, and the extension never
// reconnected. Before that, an open turn, an end being published and a
// request waiting for its answer each keep the run from going, as pi sends
// nothing again; the state's poll does not.
func TestAnIdlePiRunHandsItsConnectionOver(t *testing.T) {
	gate := make(chan struct{})
	ends := make(chan adapter.TurnEnd, 16)
	handler := adapter.TurnHandler{
		Capture: func() *mail.ReadBoundary { return &mail.ReadBoundary{Through: 7} },
		Publish: func(_ context.Context, end adapter.TurnEnd) error { <-gate; ends <- end; return nil },
	}
	rig := newRig(t, func(r *run) { r.ReportTurns(handler) })
	wakes, tables := make(chan frame, 4), make(chan frame, 4)
	held := make(chan struct{})
	l := rig.client(t, "conversation", func(l *link, got frame) {
		if got.Op == "tools" {
			tables <- got
		}
		if got.Op == "wake" {
			wakes <- got
			<-held
		}
		l.answer(got, frame{OK: true})
	})
	before := rig.run
	if why := before.Busy(); why != "" {
		t.Fatalf("idle after its hello, Busy says %q", why)
	}

	askOK(t, l, frame{Op: "turn-start", Thread: "conversation", Turn: "t1"})
	busyWith(t, before, "a turn is open")
	answered := make(chan error, 1)
	go func() {
		_, err := l.ask(context.Background(), frame{Op: "turn-end", Thread: "conversation", Turn: "t1", Outcome: "answered", Text: "done"})
		answered <- err
	}()
	busyWith(t, before, "being published")
	close(gate)
	if err := receive(t, answered); err != nil {
		t.Fatal(err)
	}
	receive(t, ends)

	delivered := make(chan mail.Result, 1)
	go func() {
		delivered <- before.Deliver(context.Background(), mail.Message{ID: "m1", From: "main", DeliveryThread: "conversation"})
	}()
	receive(t, wakes)
	busyWith(t, before, "waits for its answer")
	close(held)
	if result := receive(t, delivered); result.State != mail.Delivered {
		t.Fatalf("the notice: %+v", result)
	}
	if why := before.Busy(); why != "" {
		t.Fatalf("idle, Busy says %q", why)
	}

	// What the face's translator holds goes over in its memory file: a
	// block streaming in the old image settles under its id in the next.
	opened := before.translate.take([]raw{{Event: "agent_start"}, {Event: "message_update", Type: "text_delta", Delta: "Hel"}})
	handed, err := before.HandOver()
	if err != nil {
		t.Fatal(err)
	}
	if len(handed.Files) != 3 {
		t.Fatalf("handed over %d files", len(handed.Files))
	}
	// The exec: the old image's descriptors go, the socket's path and the
	// copies handed over stay.
	aside := before.socket + ".aside"
	if err := os.Rename(before.socket, aside); err != nil {
		t.Fatal(err)
	}
	before.Close()
	if err := os.Rename(aside, before.socket); err != nil {
		t.Fatal(err)
	}
	for len(rig.live) > 0 {
		<-rig.live
	}

	after := newRun(before.file, before.socket, "run-epoch")
	live := make(chan []adapter.Capability, 16)
	after.ReportLive(func(caps []adapter.Capability) { live <- caps })
	after.ReportTurns(adapter.TurnHandler{
		Capture: func() *mail.ReadBoundary { return &mail.ReadBoundary{Through: 8} },
		Publish: func(_ context.Context, end adapter.TurnEnd) error { ends <- end; return nil },
	})
	if err := after.Adopt(t.Context(), nil, handed); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(after.Close)
	if settled := after.translate.take([]raw{{Event: "message_update", Type: "text_end", Content: json.RawMessage(`"Hello"`)}}); len(settled) != 1 || len(opened) != 2 || settled[0].ID != opened[1].ID || settled[0].Turn != opened[0].Turn {
		t.Fatalf("the block streaming across the handover: %+v, then %+v", opened, settled)
	}
	if caps := receive(t, live); len(caps) != 3 {
		t.Fatalf("the adopted run's live set: %v", caps)
	}
	// The next image may offer another table: it sends its own.
	if table := receive(t, tables); table.Thread != "conversation" || len(table.Tools) == 0 {
		t.Fatalf("the adopted run's table: %+v", table)
	}
	askOK(t, l, frame{Op: "turn-start", Thread: "conversation", Turn: "t2"})
	askOK(t, l, frame{Op: "turn-end", Thread: "conversation", Turn: "t2", Outcome: "answered", Text: "after"})
	if end := receive(t, ends); end.Event != ID+"/t2" || end.Answer != "after" || end.Boundary.Through != 8 {
		t.Fatalf("the end after the handover: %+v", end)
	}
	if result := after.Deliver(t.Context(), mail.Message{ID: "m2", From: "main", DeliveryThread: "conversation"}); result.State != mail.Delivered {
		t.Fatalf("a notice after the handover: %+v", result)
	}
	if wake := receive(t, wakes); wake.Op != "wake" {
		t.Fatalf("the extension heard %+v", wake)
	}
	if l.gone() {
		t.Fatal("the extension's connection closed with the old image")
	}
}

// handedShapes pins each version of pi's handed state to its fields: a
// switch of the room's build admits only a build that takes the same
// handover.pi, so a changed handedState raises handoverVersion.
var handedShapes = map[int]string{
	1: `Version int "version"; PID int "pid"; Start uint64 "start"; Thread string "thread"; Next int64 "next"`,
	2: `Version int "version"; PID int "pid"; Start uint64 "start"; Thread string "thread"; Next int64 "next"; Unread []uint8 "unread,omitempty"`,
	3: `Version int "version"; PID int "pid"; Start uint64 "start"; Thread string "thread"; Next int64 "next"; Unread []uint8 "unread,omitempty"; Cache *pi.cacheSeen "cache,omitempty" {Thread string "thread"; Of json.RawMessage "of,omitempty"; Read int64 "read"; Write int64 "write"; Input int64 "input"; TTL int64 "ttl,omitempty"; ExpiresAt int64 "expiresAt,omitempty"}; Translator int "translator"`,
}

func TestThePiHandoverChangesWithItsShape(t *testing.T) {
	if got := shapeOf(reflect.TypeFor[handedState]()); got != handedShapes[handoverVersion] {
		t.Fatalf("handedState is\n%s\nand handover.pi %d pins\n%s\na changed handedState raises handoverVersion", got, handoverVersion, handedShapes[handoverVersion])
	}
}

// shapeOf is a struct's fields, and those of a struct of this package one
// holds, so a changed field of what the handover nests changes it too.
func shapeOf(typ reflect.Type) string {
	fields := make([]string, 0, typ.NumField())
	for index := range typ.NumField() {
		field := typ.Field(index)
		shape := fmt.Sprintf("%s %s %q", field.Name, field.Type, field.Tag.Get("json"))
		if nested := field.Type; nested.Kind() == reflect.Pointer && nested.Elem().Kind() == reflect.Struct && nested.Elem().PkgPath() == typ.PkgPath() {
			shape += " {" + shapeOf(nested.Elem()) + "}"
		}
		fields = append(fields, shape)
	}
	return strings.Join(fields, "; ")
}

// What the extension sends after the snapshot is not taken by the image that
// goes: the reader halts, a frame half read goes over with the handover, and
// the rest waits in the socket, so the next image answers a turn's start the
// old one never saw and publishes its end. A handover that does not exec
// resumes the reader.
func TestAFrameAfterTheSnapshotWaitsForTheNextImage(t *testing.T) {
	ends := make(chan adapter.TurnEnd, 4)
	through := uint64(7)
	handler := adapter.TurnHandler{
		Capture: func() *mail.ReadBoundary { return &mail.ReadBoundary{Through: through} },
		Publish: func(_ context.Context, end adapter.TurnEnd) error { ends <- end; return nil },
	}
	rig := newRig(t, func(r *run) { r.ReportTurns(handler) })
	l := rig.client(t, "conversation", func(l *link, got frame) { l.answer(got, frame{OK: true}) })
	before := rig.run

	failed, err := before.HandOver()
	if err != nil {
		t.Fatal(err)
	}
	failed.Resume()
	for _, file := range failed.Files {
		_ = file.Close()
	}
	askOK(t, l, frame{Op: "turn-start", Thread: "conversation", Turn: "t0"})
	askOK(t, l, frame{Op: "turn-end", Thread: "conversation", Turn: "t0", Outcome: "answered", Text: "before"})
	receive(t, ends)

	half := make(chan frame, 1)
	l.mu.Lock()
	l.waiting[900] = half
	l.mu.Unlock()
	raw := `{"op":"state","id":900,"thread":"conversation","state":{"sequence":9,"activity":"idle"}}` + "\n"
	if _, err := l.conn.Write([]byte(raw[:30])); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	handed, err := before.HandOver()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.conn.Write([]byte(raw[30:])); err != nil {
		t.Fatal(err)
	}
	started := make(chan frame, 1)
	go func() {
		reply, _ := l.ask(context.Background(), frame{Op: "turn-start", Thread: "conversation", Turn: "late"})
		started <- reply
	}()
	select {
	case reply := <-started:
		t.Fatalf("the image that goes answered a start sent after its snapshot: %+v", reply)
	case <-time.After(200 * time.Millisecond):
	}

	aside := before.socket + ".aside"
	if err := os.Rename(before.socket, aside); err != nil {
		t.Fatal(err)
	}
	before.Close()
	if err := os.Rename(aside, before.socket); err != nil {
		t.Fatal(err)
	}
	through = 8
	after := newRun(before.file, before.socket, "run-epoch")
	after.ReportTurns(handler)
	if err := after.Adopt(t.Context(), nil, handed); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(after.Close)
	receive(t, half)
	if reply := receive(t, started); !reply.OK {
		t.Fatalf("the next image's answer to the start: %+v", reply)
	}
	askOK(t, l, frame{Op: "turn-end", Thread: "conversation", Turn: "late", Outcome: "answered", Text: "report"})
	if end := receive(t, ends); end.Event != ID+"/late" || end.Boundary.Through != 8 {
		t.Fatalf("the end of the start sent after the snapshot: %+v", end)
	}
}
