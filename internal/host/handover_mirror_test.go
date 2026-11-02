package host

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
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

// A run that serves rewake's face is handed over as any other, a model of
// its own in the next image, which says what it does not hold; while a face
// draws the session the switch waits, as the face would see the conversation
// go.
func TestARunWithAMirrorWaitsOnlyForItsFaces(t *testing.T) {
	base, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	room := buildtest.Room(t, base, state.DefaultRoom)
	if err := state.EnsureSubdir(filepath.Join(room, "sock")); err != nil {
		t.Fatal(err)
	}
	request := Request{Harness: mirrored{&fakeHarness{}}, Dir: room, Mirror: true, Handover: &Handover{Count: 1}}
	model, faces, stop, err := startMirror(request, "api", "e1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	var notice string
	for _, line := range model.Snapshot() {
		if line.Op == mirror.LineOpItem && line.Item.Kind == mirror.ItemKindNotice {
			notice = line.Item.Text
		}
	}
	if !strings.Contains(notice, "native face") {
		t.Fatalf("the adopted run's model begins with %q", notice)
	}

	r := &reexec{request: request, service: idle{}, faces: faces}
	if kind, why := r.Answer(); kind != switching.Ready {
		t.Fatalf("with no face, the run answers %s: %s", kind, why)
	}
	conn, err := net.Dial("unix", registry.MirrorFor(room, "api", "e1"))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for faces() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if kind, why := r.Answer(); kind != switching.Busy || !strings.Contains(why, "face") {
		t.Fatalf("with a face attached, the run answers %s: %s", kind, why)
	}
	_ = conn.Close()
	for faces() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if kind, why := r.Answer(); kind != switching.Ready {
		t.Fatalf("once the face left, the run answers %s: %s", kind, why)
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
