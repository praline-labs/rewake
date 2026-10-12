package endpoint

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	tool "github.com/praline-labs/rewake/internal/bridge"
)

// hold says hello on a connection it keeps open, and returns the answer.
func hold(t *testing.T, path string, greeting hello) (*net.UnixConn, string) {
	t.Helper()
	raw, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	conn := raw.(*net.UnixConn)
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	encoded, _ := json.Marshal(greeting)
	if _, err := conn.Write(append(encoded, '\n')); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var answer response
	_ = json.Unmarshal(line, &answer)
	return conn, answer.Error
}

// A child's hello is admitted only from a process below the harness, of this
// build, carrying this launch's capability; a transport's only from the
// process the run named.
func TestAHelloIsChecked(t *testing.T) {
	_, path := testEndpoint(t, testTransport)
	cases := []struct {
		greeting hello
		refusal  string
	}{
		{hello{Role: roleChild, Capability: "secret"}, ""},
		{hello{Role: roleChild, Capability: "guess"}, "capability"},
		{hello{Role: roleChild}, "capability"},
		{hello{Role: roleTransport, Capability: "secret"}, "no tool transport"},
		// The server's hello, which no adapter has made since S8, left in S10.
		{hello{Role: "server", Capability: "secret"}, "role"},
		{hello{Role: "admin", Capability: "secret"}, "role"},
	}
	for _, test := range cases {
		_, refusal := hold(t, path, test.greeting)
		if test.refusal == "" && refusal != "" || !strings.Contains(refusal, test.refusal) {
			t.Fatalf("%+v: %q", test.greeting, refusal)
		}
	}

	unrooted, unrootedPath := testEndpoint(t, testTransport)
	unrooted.SetRoots()
	if _, refusal := hold(t, unrootedPath, hello{Role: roleChild, Capability: "secret"}); !strings.Contains(refusal, "not started") {
		t.Fatalf("before the harness started: %q", refusal)
	}
	_, outsidePath := testEndpoint(t, testTransport, func(c *Config) {
		c.Descends = func(int, int) error { return errors.New("not below") }
	})
	if _, refusal := hold(t, outsidePath, hello{Role: roleChild, Capability: "secret"}); !strings.Contains(refusal, "does not run below") {
		t.Fatalf("a process outside the harness: %q", refusal)
	}
	_, otherPath := testEndpoint(t, testTransport, func(c *Config) {
		c.SameBuild = func(int) error { return errors.New("another image") }
	})
	if _, refusal := hold(t, otherPath, hello{Role: roleChild, Capability: "secret"}); !strings.Contains(refusal, "another build") {
		t.Fatalf("another build: %q", refusal)
	}
}

// Each role asks only its own question.
func TestARoleAsksOnlyItsOwnQuestion(t *testing.T) {
	_, path := testEndpoint(t, testTransport)
	conn, refusal := hold(t, path, hello{Role: roleChild, Capability: "secret"})
	if refusal != "" {
		t.Fatal(refusal)
	}
	call := ToolCall{Tool: "inbox", CallID: "c1"}
	encoded, _ := json.Marshal(request{ID: 7, Op: opCall, Call: &call})
	_, _ = conn.Write(append(encoded, '\n'))
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var answer response
	_ = json.Unmarshal(line, &answer)
	if answer.ID != 7 || !strings.Contains(answer.Error, "may not ask") || answer.Answer != nil {
		t.Fatalf("a child asked to run a call: %+v", answer)
	}
}

// The connections are bounded: sixteen at once.
func TestTheConnectionsAreBounded(t *testing.T) {
	_, path := testEndpoint(t, testTransport)
	for range maxOthers {
		if _, refusal := hold(t, path, hello{Role: roleChild, Capability: "secret"}); refusal != "" {
			t.Fatal(refusal)
		}
	}
	if _, refusal := hold(t, path, hello{Role: roleChild, Capability: "secret"}); !strings.Contains(refusal, "too many") {
		t.Fatalf("a seventeenth connection: %q", refusal)
	}
}

// A child and a transport that cannot reach the endpoint fail as
// unreachable, which their callers tell from a refusal.
func TestAMissingEndpointIsUnreachable(t *testing.T) {
	path := t.TempDir() + "/none.ctx"
	if err := Confirm(path, tool.Ticket{}); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("confirm: %v", err)
	}
	if _, err := CallTool(path, ToolCall{Tool: "inbox"}, time.Second); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("call: %v", err)
	}
}
