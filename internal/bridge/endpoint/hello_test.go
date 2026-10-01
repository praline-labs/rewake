package endpoint

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
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

// A hello is admitted only from a process below the harness, of this build,
// carrying this launch's capability where its role needs one.
func TestAHelloIsChecked(t *testing.T) {
	_, path := testEndpoint(t, bridge.CodexTransport)
	cases := []struct {
		greeting hello
		refusal  string
	}{
		{hello{Role: roleServer, Capability: "secret"}, ""},
		{hello{Role: roleChild, Capability: "secret"}, ""},
		{hello{Role: roleHook}, ""},
		{hello{Role: roleServer, Capability: "guess"}, "capability"},
		{hello{Role: roleChild}, "capability"},
		{hello{Role: "admin", Capability: "secret"}, "role"},
	}
	for _, test := range cases {
		_, refusal := hold(t, path, test.greeting)
		if test.refusal == "" && refusal != "" || !strings.Contains(refusal, test.refusal) {
			t.Fatalf("%+v: %q", test.greeting, refusal)
		}
	}

	unrooted, unrootedPath := testEndpoint(t, bridge.CodexTransport)
	unrooted.SetRoots()
	if _, refusal := hold(t, unrootedPath, hello{Role: roleHook}); !strings.Contains(refusal, "not started") {
		t.Fatalf("before the harness started: %q", refusal)
	}
	_, outsidePath := testEndpoint(t, bridge.CodexTransport, func(c *Config) {
		c.Descends = func(int, int) error { return errors.New("not below") }
	})
	if _, refusal := hold(t, outsidePath, hello{Role: roleServer, Capability: "secret"}); !strings.Contains(refusal, "does not run below") {
		t.Fatalf("a process outside the harness: %q", refusal)
	}
	_, otherPath := testEndpoint(t, bridge.CodexTransport, func(c *Config) {
		c.SameBuild = func(int) error { return errors.New("another image") }
	})
	if _, refusal := hold(t, otherPath, hello{Role: roleHook}); !strings.Contains(refusal, "another build") {
		t.Fatalf("another build: %q", refusal)
	}
}

// Each role asks only its own question.
func TestARoleAsksOnlyItsOwnQuestion(t *testing.T) {
	_, path := testEndpoint(t, bridge.CodexTransport)
	conn, refusal := hold(t, path, hello{Role: roleHook})
	if refusal != "" {
		t.Fatal(refusal)
	}
	asked := TicketRequest{Transport: bridge.CodexTransport, CallID: "c1"}
	encoded, _ := json.Marshal(request{ID: 7, Op: opTicket, Ask: &asked})
	_, _ = conn.Write(append(encoded, '\n'))
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var answer response
	_ = json.Unmarshal(line, &answer)
	if answer.ID != 7 || !strings.Contains(answer.Error, "may not ask") || answer.Ticket != nil {
		t.Fatalf("a hook asked for a ticket: %+v", answer)
	}
}

// The connections are bounded: sixteen besides the servers, four servers.
func TestTheConnectionsAreBounded(t *testing.T) {
	_, path := testEndpoint(t, bridge.CodexTransport)
	for range maxOthers {
		if _, refusal := hold(t, path, hello{Role: roleHook}); refusal != "" {
			t.Fatal(refusal)
		}
	}
	if _, refusal := hold(t, path, hello{Role: roleHook}); !strings.Contains(refusal, "too many") {
		t.Fatalf("a seventeenth connection: %q", refusal)
	}
	for range maxServers {
		if _, refusal := hold(t, path, hello{Role: roleServer, Capability: "secret"}); refusal != "" {
			t.Fatal(refusal)
		}
	}
	if _, refusal := hold(t, path, hello{Role: roleServer, Capability: "secret"}); !strings.Contains(refusal, "too many") {
		t.Fatalf("a fifth server: %q", refusal)
	}
}

// A hook and a child that cannot reach the endpoint fail as unreachable,
// which their callers tell from a refusal.
func TestAMissingEndpointIsUnreachable(t *testing.T) {
	path := t.TempDir() + "/none.ctx"
	if err := Observe(path, []byte(`{}`), time.Second); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("observe: %v", err)
	}
	if err := Confirm(path, bridge.Ticket{}); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("confirm: %v", err)
	}
	if _, err := Dial(path, "secret"); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("dial: %v", err)
	}
	if err := Observe(path, []byte(`not json`), time.Second); err == nil || errors.Is(err, ErrUnreachable) {
		t.Fatalf("observe of a broken input: %v", err)
	}
}

// A server's connection lives as long as the wrapper serves it; its end
// tells the server to dial again.
func TestAServersConnectionEndsWithTheWrapper(t *testing.T) {
	served, path := testEndpoint(t, bridge.CodexTransport)
	client, err := Dial(path, "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	time.Sleep(shortExchange / 50)
	served.Close()
	select {
	case <-client.Gone():
	case <-time.After(3 * time.Second):
		t.Fatal("the connection outlived its wrapper")
	}
	if _, err := client.Ticket(codexRequest("th", "t1", "c1", words), time.Second); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a ticket after the wrapper closed: %v", err)
	}
}
