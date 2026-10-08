package endpoint

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// Admission of a transport's request as it arrives, and a Close that drains
// the calls it took.

// A request that arrives after the transport was withdrawn runs nothing: the
// hello's proof does not carry over to it.
func TestARequestAfterTheTransportIsWithdrawnRunsNothing(t *testing.T) {
	var checked atomic.Bool
	served, path := transportEndpoint(t, func(c *Config) {
		c.Check = func(words []string) ([]string, string) { checked.Store(true); return words, "" }
	})
	openTurn(served, "th", "t1")
	seenCall(served, "th", "t1", "late", []string{"inbox"})
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)
	_, _ = conn.Write([]byte(`{"role":"transport"}` + "\n"))
	if line, err := reader.ReadString('\n'); err != nil || strings.Contains(line, "error") {
		t.Fatalf("the hello: %q %v", line, err)
	}
	served.SetTransport(0, 0)
	call := inboxCall("late", `{}`)
	raw, _ := json.Marshal(request{ID: 1, Op: opCall, Call: &call})
	_, _ = conn.Write(append(raw, '\n'))
	line, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(line, "nothing ran") {
		t.Fatalf("the answer: %q %v", line, err)
	}
	if checked.Load() {
		t.Fatal("a request after the transport was withdrawn reached the operation's check")
	}
}

// A call the endpoint took before Close began runs and answers: its child
// still confirms, whether it had not yet started, not yet greeted the
// endpoint, or greeted it and not yet asked when Close began, and the socket
// closes only after it.
func TestCloseLetsTheCallsItTookConfirmTheirChildren(t *testing.T) {
	for _, before := range []string{"its child starts", "its child confirms", "its child asks"} {
		t.Run(before, func(t *testing.T) {
			served, path := transportEndpoint(t)
			openTurn(served, "th", "t1")
			seenCall(served, "th", "t1", "last", []string{"inbox"})
			reached, release := make(chan struct{}), make(chan struct{})
			free := sync.OnceFunc(func() { close(release) })
			defer free()
			switch before {
			case "its child starts":
				var once sync.Once
				oldFault := state.Fault
				state.Fault = func(op, at string) error {
					if op == state.OpStep && at == path+"/start" {
						once.Do(func() { close(reached); <-release })
					}
					return nil
				}
				defer func() { state.Fault = oldFault }()
			case "its child confirms", "its child asks":
				hold := helperChildHold
				if before == "its child asks" {
					// Admitted before Close, the child's connection is one
					// Close leaves open.
					hold = helperChildHoldGreeted
				}
				dir := t.TempDir()
				served.mu.Lock()
				served.childEnv = append(served.childEnv, hold+"="+dir)
				served.mu.Unlock()
				go func() {
					for until := time.Now().Add(10 * time.Second); time.Now().Before(until); time.Sleep(10 * time.Millisecond) {
						if _, err := os.Stat(filepath.Join(dir, "started")); err == nil {
							close(reached)
							break
						}
					}
					<-release
					_ = os.WriteFile(filepath.Join(dir, "go"), nil, 0o600)
				}()
			}
			answered := make(chan ToolAnswer, 1)
			problems := make(chan error, 1)
			go func() {
				answer, err := CallTool(path, inboxCall("last", `{}`), 10*time.Second)
				answered <- answer
				problems <- err
			}()
			select {
			case <-reached:
			case <-time.After(5 * time.Second):
				t.Fatalf("the call never got to before %s", before)
			}
			closed := make(chan struct{})
			go func() { served.Close(); close(closed) }()
			select {
			case <-served.done:
			case <-time.After(time.Second):
				t.Fatal("Close did not begin")
			}
			select {
			case <-closed:
				t.Fatal("Close returned before the call it took")
			case <-time.After(100 * time.Millisecond):
			}
			free()
			answer, err := <-answered, <-problems
			if err != nil || answer.IsError || strings.Join(answer.Texts, "") != "ran: inbox\n" {
				t.Fatalf("Close cut a call it took before %s: %+v %v", before, answer, err)
			}
			select {
			case <-closed:
			case <-time.After(5 * time.Second):
				t.Fatal("Close did not return after its last call")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("the socket outlived Close: %v", err)
			}
		})
	}
}
