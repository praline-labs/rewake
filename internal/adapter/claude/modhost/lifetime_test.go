package modhost

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// runLife stands in for the wrapper's lifetime of a run: it counts the
// handlers it admits, and once the run has ended and the last of them is
// dismissed, it lets go of the room's lease, as the wrapper does.
type runLife struct {
	mu      sync.Mutex
	room    string
	count   int
	ended   bool
	release chan struct{}
}

func (l *runLife) Admit() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ended {
		return false
	}
	l.count++
	return true
}

func (l *runLife) Dismiss() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.count--
	l.letGo()
}

func (l *runLife) end() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ended = true
	l.letGo()
}

func (l *runLife) letGo() {
	if l.ended && l.count == 0 {
		buildtest.Release(l.room)
		close(l.release)
	}
}

func (l *runLife) counted() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.count
}

// stalledInOpen waits until a goroutine of this process sleeps in a FIFO's
// open, waiting for its reader.
func stalledInOpen(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		paths, _ := filepath.Glob("/proc/self/task/*/wchan")
		for _, path := range paths {
			if data, _ := os.ReadFile(path); strings.TrimSpace(string(data)) == "wait_for_partner" {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no writer stalled in the FIFO's open")
}

// A handler stalled in its write when the run ends does not hold the close,
// which returns within its bound, and does not outlive the lease: it stays
// counted among the run's workers, so the lease goes only after its write.
// The FIFO at the record's temporary path stalls the real write past the
// state's guard.
func TestAWriteStalledAtCloseKeepsTheLeaseUntilItEnds(t *testing.T) {
	server, socket := serving(t)
	room := filepath.Dir(filepath.Dir(filepath.Dir(server.cfg.Record)))
	life := &runLife{room: room, release: make(chan struct{})}
	server.CountWorkers(life)
	fifo := server.cfg.Record + ".new"
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	answered := make(chan struct{})
	go func() {
		defer close(answered)
		_, _, _ = post(socket, PathHello, validHello("stalled"))
	}()
	stalledInOpen(t)
	began := time.Now()
	server.Close()
	if took := time.Since(began); took > closeWait+time.Second {
		t.Fatalf("Close waited %v for a stalled handler, past its bound", took)
	}
	life.end()
	if life.counted() != 1 {
		t.Fatalf("the stalled handler is not counted among the run's workers: %d", life.counted())
	}
	if err := state.WriteFile(filepath.Join(room, "still-held"), nil, 0o600); err != nil {
		t.Fatalf("the lease went while a handler could still write: %v", err)
	}
	reader, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	select {
	case <-life.release:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler never ended")
	}
	data := make([]byte, 4096)
	if n, _ := reader.Read(data); !strings.Contains(string(data[:n]), `"loadId":"stalled"`) {
		t.Fatalf("the stalled write: %q", data[:n])
	}
	if err := state.WriteFile(filepath.Join(room, "after"), nil, 0o600); err == nil {
		t.Fatal("the lease was kept after the last handler ended")
	}
	<-answered
}

// A handler that has not begun its write when Close comes writes nothing:
// the check comes right before the write, under the lock that orders it.
func TestAHandlerPastTheCloseWritesNothing(t *testing.T) {
	server, socket := serving(t)
	room := filepath.Dir(filepath.Dir(filepath.Dir(server.cfg.Record)))
	life := &runLife{room: room, release: make(chan struct{})}
	server.CountWorkers(life)
	server.bindMu.Lock()
	answer := make(chan int, 1)
	go func() {
		status, _, _ := post(socket, PathHello, validHello("late"))
		answer <- status
	}()
	// The handler is admitted; its binding waits for the lock.
	deadline := time.Now().Add(2 * time.Second)
	for life.counted() != 1 {
		if time.Now().After(deadline) {
			t.Fatal("the hello never reached its binding")
		}
		time.Sleep(time.Millisecond)
	}
	server.Close()
	server.bindMu.Unlock()
	<-answer
	if _, ok := server.Load(); ok {
		t.Fatal("a hello past the close was bound")
	}
	if _, err := os.Stat(server.cfg.Record); !os.IsNotExist(err) {
		t.Fatalf("a hello past the close was recorded: %v", err)
	}
}

// Hellos at once leave the binding and its record agreeing: the record holds
// the load Load holds.
func TestHellosAtOnceLeaveTheRecordWithTheLoad(t *testing.T) {
	server, socket := serving(t)
	for round := range 5 {
		var wg sync.WaitGroup
		for i := range 8 {
			wg.Go(func() {
				if status, text, err := post(socket, PathHello, validHello(string(rune('a'+round))+string(rune('a'+i)))); err != nil || status != http.StatusOK {
					t.Errorf("a hello: %d %s %v", status, text, err)
				}
			})
		}
		wg.Wait()
		load, _ := server.Load()
		data, err := os.ReadFile(server.cfg.Record)
		if err != nil {
			t.Fatal(err)
		}
		var recorded HelloRequest
		if err := json.Unmarshal(data, &recorded); err != nil || recorded != load {
			t.Fatalf("round %d: the record holds %+v, the run %+v", round, recorded, load)
		}
	}
}
