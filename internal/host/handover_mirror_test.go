package host

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mirror"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/host/switching"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// idle is a service that can always be handed over.
type idle struct{}

func (idle) Start(context.Context, func(string)) error { return nil }
func (idle) Close()                                    {}
func (idle) Busy() string                              { return "" }
func (idle) HandOver() (adapter.HandedOver, error)     { return adapter.HandedOver{}, nil }
func (idle) Adopt(context.Context, func(string), adapter.HandedOver) error {
	return nil
}

// mirrored is the fake harness with rewake's face offered.
type mirrored struct{ *fakeHarness }

func (mirrored) Mirrors() {}

// faceReads connects a face to the run's mirror and answers the texts of the
// items its snapshot draws.
func faceReads(t *testing.T, socket string) ([]string, net.Conn) {
	t.Helper()
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var texts []string
	lines := bufio.NewScanner(conn)
	lines.Buffer(nil, 1<<20)
	for lines.Scan() {
		var line mirror.Line
		if err := json.Unmarshal(lines.Bytes(), &line); err != nil {
			t.Fatal(err)
		}
		if line.Op == mirror.LineOpItem {
			texts = append(texts, line.Item.Text)
		}
		if line.Op == mirror.LineOpEnd {
			return texts, conn
		}
	}
	t.Fatalf("the face's snapshot did not end: %v", lines.Err())
	return nil, nil
}

// A run that serves rewake's face is handed over with a face attached: its
// model goes to the next image in a memory file, and the face, whose
// connection ended with the old image, reconnects and draws the conversation
// as it was.
func TestAFaceReconnectsToTheModelHandedOver(t *testing.T) {
	base, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	room := buildtest.Room(t, base, state.DefaultRoom)
	if err := state.EnsureSubdir(filepath.Join(room, "sock")); err != nil {
		t.Fatal(err)
	}
	socket := registry.MirrorFor(room, "api", "e1")
	request := Request{Harness: mirrored{&fakeHarness{}}, Dir: room, Mirror: true}
	model, stop, err := startMirror(request, "api", "e1")
	if err != nil {
		t.Fatal(err)
	}
	model.Apply(mirror.Event{Kind: mirror.EventPrompt, ID: "p1", Text: "what came before"})
	drawn, face := faceReads(t, socket)
	if len(drawn) != 1 {
		t.Fatalf("the face draws %q", drawn)
	}

	r := &reexec{request: request, service: idle{}, model: model}
	if kind, why := r.Answer(); kind != switching.Ready {
		t.Fatalf("with a face attached, the run answers %s: %s", kind, why)
	}
	file, err := handModel(model)
	if err != nil {
		t.Fatal(err)
	}
	stop()
	if _, err := io.ReadAll(face); err != nil {
		t.Fatalf("the face's connection outlived its image: %v", err)
	}
	_ = face.Close()

	request.Handover = &Handover{Count: 1, model: file}
	adopted, stopAdopted, err := startMirror(request, "api", "e1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stopAdopted)
	again, face := faceReads(t, socket)
	_ = face.Close()
	if len(again) != 1 || again[0] != "what came before" {
		t.Fatalf("the face reconnected to the next image draws %q", again)
	}
	adopted.Apply(mirror.Event{Kind: mirror.EventPrompt, ID: "p2", Text: "after"})
	if after, face := faceReads(t, socket); len(after) != 2 {
		t.Fatalf("after the handover the face draws %q", after)
	} else {
		_ = face.Close()
	}
}

// workersOnly is a real harness's service: it hands a worker's run over.
type workersOnly struct{ idle }

func (workersOnly) WorkersOnly() {}

// Main's wrapper of a real harness cannot hand over yet, idle or not, and a
// worker's of the same harness can; the fixture's main can, as the switch's
// test.
func TestMainsWrapperOfARealHarnessCannotHandOver(t *testing.T) {
	for _, test := range []struct {
		role    string
		service adapter.Service
		want    string
	}{
		{role.Main.ID, workersOnly{}, switching.Cannot},
		{role.Write.ID, workersOnly{}, switching.Ready},
		{role.Main.ID, idle{}, switching.Ready},
	} {
		r := &reexec{session: registry.Session{Name: "s", Role: test.role}, service: test.service}
		if kind, why := r.Answer(); kind != test.want {
			t.Errorf("%s's %T answers %s %q, want %s", test.role, test.service, kind, why, test.want)
		}
	}
}

// A SIGTERM that comes after an adopting image's first catch and before its
// run's waits for the run, which forwards it to the harness; the image lives.
func TestASignalBeforeTheRunsCatchWaitsForTheRun(t *testing.T) {
	CatchSignals()
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	incoming, restore := takeSignals()
	defer restore()
	select {
	case got := <-incoming:
		if got != syscall.SIGTERM {
			t.Fatalf("the run took %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the SIGTERM caught before the run did not reach it")
	}
	if caught.incoming != nil {
		t.Fatal("the early catch is taken twice")
	}
}
