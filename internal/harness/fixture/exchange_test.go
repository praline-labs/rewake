//go:build rewakefixture

package fixture

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
)

// startProgram starts the scripted program under a backend with the given
// switches, and answers what Start answered.
func startProgram(t *testing.T, switches ...string) (*backend, error) {
	t.Helper()
	// A short directory of its own: a socket path has a bound, and a test
	// name under the system's temporary directory can pass it.
	dir, err := os.MkdirTemp("", "fx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "f.sock")
	env := append(append(os.Environ(), programEnv+"=1"), switches...)
	b := newBackend(exe, []string{"--connect", socket}, env, dir, socket, "e1")
	t.Cleanup(b.Close)
	return b, b.Start(context.Background(), harness.CompletionHandler{}, nil)
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not happen within 5s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTheExchangeMakesLiveWhatItsProbesAnswer(t *testing.T) {
	b, err := startProgram(t)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Live(); !slices.Equal(got, Served) {
		t.Fatalf("live %v, want %v", got, Served)
	}
	if thread, err := b.Thread(); err != nil || thread != programThrd {
		t.Fatalf("thread %q, %v", thread, err)
	}
	if b.ProcessID() == 0 || b.ProcessID() != b.process.Process.Pid {
		t.Fatalf("process id %d", b.ProcessID())
	}
	if state := b.SessionState(); state.Thread != programThrd || !state.Fresh {
		t.Fatalf("a live telemetry shows no state: %+v", state)
	}
}

// Each switch withholds one capability — left out of the hello, or served
// with a failing probe — and only that one, and what rests on it stops.
func TestEachSwitchWithholdsItsCapability(t *testing.T) {
	for _, capability := range Served {
		for _, how := range []string{switchNo, switchFail} {
			t.Run(capability+"/"+strings.TrimPrefix(how, "FIXTURE_TEST_"), func(t *testing.T) {
				b, err := startProgram(t, how+"="+capability)
				if err != nil {
					t.Fatal(err)
				}
				want := slices.DeleteFunc(slices.Clone(Served), func(c string) bool { return c == capability })
				if got := b.Live(); !slices.Equal(got, want) {
					t.Fatalf("live %v, want %v", got, want)
				}
				withheldStops(t, b, capability)
			})
		}
	}
}

// withheldStops checks that what rests on a capability stops without it.
func withheldStops(t *testing.T, b *backend, capability string) {
	t.Helper()
	b.mu.Lock()
	l := b.link
	b.mu.Unlock()
	switch capability {
	case Wake:
		if _, err := b.Thread(); !errors.Is(err, inbox.ErrThreadUnavailable) {
			t.Fatalf("a conversation without wake: %v", err)
		}
		if _, err := b.Reserve(context.Background(), inbox.Message{}); !errors.Is(err, inbox.ErrThreadUnavailable) {
			t.Fatalf("a reservation without wake: %v", err)
		}
		if result := b.Deliver(context.Background(), inbox.Message{ID: "m1"}); result.State != inbox.Failed {
			t.Fatalf("a delivery without wake: %+v", result)
		}
	case TurnBoundary:
		// A wrapper that would take any end, so only the adapter's refusal
		// keeps one from being published.
		taken := 0
		b.handler = harness.CompletionHandler{
			Publish: func(context.Context, harness.Completion) error { taken++; return nil },
			Confirm: func(context.Context, harness.Completion) (string, error) { taken++; return "", nil },
		}
		if answer := b.turnStarted(l, Frame{Turn: "t1"}); answer.OK {
			t.Fatal("a turn start taken without a turn boundary")
		}
		for _, hold := range []bool{false, true} {
			if answer := b.turnEnded(l, Frame{Turn: "t1", End: "t1/1", Outcome: OutcomeCompleted, Hold: hold}); answer.OK || taken != 0 {
				t.Fatalf("an end taken without a turn boundary: %+v, %d handed on", answer, taken)
			}
		}
	case Telemetry:
		b.activity(l, Frame{State: "working"})
		if state := b.SessionState(); state.Fresh || state.Activity != nil {
			t.Fatalf("a state without telemetry: %+v", state)
		}
	}
}

func TestADisconnectWithdrawsEverythingAtOnce(t *testing.T) {
	b, err := startProgram(t, switchDrop+"=1")
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "the withdrawal", func() bool { return len(b.Live()) == 0 })
	if _, err := b.Thread(); !errors.Is(err, inbox.ErrThreadUnavailable) {
		t.Fatalf("a conversation after the disconnect: %v", err)
	}
	select {
	case <-b.Done():
		t.Fatal("the program ended; the test is of a disconnect")
	default:
	}
}

func TestAReconnectFromTheSameProcessIsAFreshExchange(t *testing.T) {
	b, err := startProgram(t, switchDrop+"=1", switchAgain+"=1")
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "the withdrawal", func() bool { return len(b.Live()) == 0 })
	eventually(t, "the second exchange", func() bool { return slices.Equal(b.Live(), Served) })
}

// A hello from any process but the one the adapter started is refused, even
// one naming the program's pid, so the start fails within its bound.
func TestAHelloFromAnotherProcessIsRefused(t *testing.T) {
	shorten(t)
	if _, err := startProgram(t, switchHelper+"=1"); err == nil || !strings.Contains(err.Error(), "no hello from its own process") {
		t.Fatalf("a helper's hello was taken: %v", err)
	}
}

func TestNoHelloFailsTheStart(t *testing.T) {
	shorten(t)
	if _, err := startProgram(t, switchSilent+"=1"); err == nil {
		t.Fatal("a start without a hello succeeded")
	}
}

// The program's end withdraws everything, even while its connection lives on
// in a process it left behind: the run is over when the program is.
func TestTheProgramsEndWithdrawsEverything(t *testing.T) {
	for _, orphan := range []bool{false, true} {
		t.Run(map[bool]string{false: "alone", true: "orphan"}[orphan], func(t *testing.T) {
			var switches []string
			if orphan {
				switches = append(switches, switchOrphan+"=1")
			}
			b, err := startProgram(t, switches...)
			if err != nil {
				t.Fatal(err)
			}
			if !orphan {
				_ = syscall.Kill(b.pid, syscall.SIGKILL)
			}
			select {
			case <-b.Done():
			case <-time.After(5 * time.Second):
				t.Fatal("the program's end was not seen")
			}
			eventually(t, "the withdrawal", func() bool { return len(b.Live()) == 0 })
		})
	}
}

func shorten(t *testing.T) {
	saved := readinessBound
	readinessBound = 1500 * time.Millisecond
	t.Cleanup(func() { readinessBound = saved })
}
