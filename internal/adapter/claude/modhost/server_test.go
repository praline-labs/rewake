package modhost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// helperSocket makes the test binary a process that says hello on the socket
// it names and prints the status it was answered with.
const helperSocket = "REWAKE_MODHOST_HELPER_SOCKET"

func TestMain(m *testing.M) {
	if socket := os.Getenv(helperSocket); socket != "" {
		status, _, err := post(socket, PathHello, validHello("helper"))
		if err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
		fmt.Println(status)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func validHello(load string) []byte {
	data, _ := json.Marshal(HelloRequest{LoadID: load, SessionID: "conversation-1", HarnessVersion: "2.1.296"})
	return data
}

// post asks the server at socket one request on a connection of its own.
func post(socket, path string, body []byte) (int, string, error) {
	client := http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}}
	answer, err := client.Post("http://rewake"+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = answer.Body.Close() }()
	text, err := io.ReadAll(answer.Body)
	return answer.StatusCode, string(text), err
}

// serving is a run's mod endpoint in a room of its own, whose harness is
// this test process unless the test names another.
func serving(t *testing.T) (*Server, string) {
	t.Helper()
	base, err := os.MkdirTemp("", "mh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	room := buildtest.Room(t, base, state.DefaultRoom)
	run := filepath.Join(room, "runs", "main.1")
	if err := state.EnsureSubdir(run); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(room, "sock", "main.1.mod.sock")
	server := New(Config{Socket: socket, Role: "main", Record: filepath.Join(run, "hello.json"), Wait: 2 * time.Second})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	server.HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: start})
	return server, socket
}

// The harness's own process is answered, and its hello binds its load to the
// run; a reload's hello replaces it, and is what the run's record says.
func TestTheHarnessesHelloBindsItsLoad(t *testing.T) {
	server, socket := serving(t)
	status, text, err := post(socket, PathHello, validHello("first"))
	if err != nil || status != http.StatusOK {
		t.Fatalf("the harness's hello: %d %s %v", status, text, err)
	}
	var answer HelloAnswer
	if err := json.Unmarshal([]byte(text), &answer); err != nil || answer != (HelloAnswer{Role: "main", Generation: 1}) {
		t.Fatalf("the answer: %s", text)
	}
	if _, _, err := post(socket, PathHello, validHello("second")); err != nil {
		t.Fatal(err)
	}
	if load, ok := server.Load(); !ok || load.LoadID != "second" {
		t.Fatalf("the bound load after a reload: %+v %v", load, ok)
	}
	recorded, err := os.ReadFile(server.cfg.Record)
	if err != nil || !strings.Contains(string(recorded), `"loadId":"second"`) {
		t.Fatalf("the run's record of the load: %s %v", recorded, err)
	}
}

// A process below the harness — a model's shell child — is refused, as is a
// process of the same user beside it: only the harness's own process is
// served, whatever the path or body (SP4).
func TestOnlyTheHarnessesOwnProcessIsServed(t *testing.T) {
	server, socket := serving(t)
	child := exec.Command(os.Args[0])
	child.Env = append(os.Environ(), helperSocket+"="+socket)
	out, err := child.Output()
	if err != nil || strings.TrimSpace(string(out)) != "403" {
		t.Fatalf("a process below the harness: %s %v", out, err)
	}
	// The harness is now a process this one started; this process is
	// another of the same user, not below it.
	beside := exec.Command("sleep", "30")
	if err := beside.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = beside.Process.Kill(); _ = beside.Wait() })
	start, err := proc.StartTime(beside.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	server.HarnessStarted(adapter.RunProcess{PID: beside.Process.Pid, Start: start})
	for _, path := range []string{PathHello, PathSpaceKeys, "/v1/elsewhere"} {
		status, text, err := post(socket, path, validHello("beside"))
		if err != nil || status != http.StatusForbidden || !strings.Contains(text, "not the harness's own process") {
			t.Fatalf("a process beside the harness on %s: %d %s %v", path, status, text, err)
		}
	}
	if _, ok := server.Load(); ok {
		t.Fatal("a refused process bound a load")
	}
}

// A harness that is gone or replaced is not served: the start time is part of
// who the harness is, and it is checked again before the answer.
func TestAReplacedHarnessIsNotServed(t *testing.T) {
	server, socket := serving(t)
	server.HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: 1})
	if status, text, _ := post(socket, PathHello, validHello("a")); status != http.StatusForbidden || !strings.Contains(text, "gone or was replaced") {
		t.Fatalf("a replaced harness: %d %s", status, text)
	}
	start, _ := proc.StartTime(os.Getpid())
	server.HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: start})
	server.answering = func() { server.HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: 1}) }
	if status, text, err := post(socket, PathHello, validHello("b")); err == nil {
		t.Fatalf("a harness replaced while its request ran was answered: %d %s", status, text)
	}
}

// A request that comes before the host names the harness waits for it, and
// is refused when the name does not come.
func TestARequestBeforeTheHarnessIsNamedWaits(t *testing.T) {
	base, err := os.MkdirTemp("", "mh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	room := buildtest.Room(t, base, state.DefaultRoom)
	socket := filepath.Join(room, "sock", "w.mod.sock")
	server := New(Config{Socket: socket, Role: "general", Wait: 300 * time.Millisecond})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	if status, text, _ := post(socket, PathHello, validHello("early")); status != http.StatusServiceUnavailable || !strings.Contains(text, "not known") {
		t.Fatalf("a request no harness was named for: %d %s", status, text)
	}
	start, _ := proc.StartTime(os.Getpid())
	go func() {
		time.Sleep(100 * time.Millisecond)
		server.HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: start})
	}()
	if status, text, _ := post(socket, PathHello, validHello("late")); status != http.StatusOK {
		t.Fatalf("a request answered once the harness was named: %d %s", status, text)
	}
}

// A body is bounded before it is parsed, is exactly the request the schema
// describes, and names what it must; the endpoints of later steps say they
// are not in this build, and an unknown path is not found.
func TestARequestIsTheSchemasOrRefused(t *testing.T) {
	server, socket := serving(t)
	huge := []byte(`{"loadId":"` + strings.Repeat("x", MaxBodyBytes) + `","sessionId":"s","harnessVersion":"v"}`)
	cases := []struct {
		name, path string
		body       []byte
		status     int
	}{
		{"over the bound", PathHello, huge, http.StatusRequestEntityTooLarge},
		{"an unknown field", PathHello, []byte(`{"loadId":"l","sessionId":"s","harnessVersion":"v","serves":"all"}`), http.StatusBadRequest},
		{"an empty load", PathHello, []byte(`{"loadId":"","sessionId":"s","harnessVersion":"v"}`), http.StatusBadRequest},
		{"a missing version", PathHello, []byte(`{"loadId":"l","sessionId":"s"}`), http.StatusBadRequest},
		{"two values", PathHello, []byte(`{"loadId":"l","sessionId":"s","harnessVersion":"v"} {}`), http.StatusBadRequest},
		{"not JSON", PathHello, []byte(`hello`), http.StatusBadRequest},
		{"a later step's endpoint", PathSpaceScreen, []byte(`{}`), http.StatusNotImplemented},
		{"an unknown path", "/v1/elsewhere", []byte(`{}`), http.StatusNotFound},
	}
	for _, c := range cases {
		status, text, err := post(socket, c.path, c.body)
		var refusal Refusal
		if err != nil || status != c.status || json.Unmarshal([]byte(text), &refusal) != nil || refusal.Error == "" {
			t.Errorf("%s: %d %s %v, want %d with a refusal", c.name, status, text, err, c.status)
		}
	}
	if _, ok := server.Load(); ok {
		t.Fatal("a refused hello bound a load")
	}
	request, _ := http.NewRequest(http.MethodGet, "http://rewake"+PathHello, nil)
	client := http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}
	answer, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = answer.Body.Close()
	if answer.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("a GET: %d", answer.StatusCode)
	}
}

// Every request is a connection of its own: the server closes each after its
// answer, so no check carries over to a later request on the same connection.
func TestEveryRequestIsAConnectionOfItsOwn(t *testing.T) {
	_, socket := serving(t)
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	body := string(validHello("k"))
	one := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: rewake\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", PathHello, len(body), body)
	if _, err := io.WriteString(conn, one+one); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	all, _ := io.ReadAll(conn)
	if got := strings.Count(string(all), "HTTP/1.1 200"); got != 1 || !strings.Contains(string(all), "Connection: close") {
		t.Fatalf("two requests on one connection were answered %d times:\n%s", got, all)
	}
}
