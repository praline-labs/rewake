package mainhost

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A client of another version is refused in the shape no version changes —
// what the host speaks, and why — and the connection ends, so a client of
// any version can say why it was turned away.
func TestAClientOfAnotherVersionIsRefusedInTheFixedShape(t *testing.T) {
	for _, hello := range []string{`{"hello":"shell","v":2}`, `{"hello":"cli","v":0}`, `not json`} {
		host, client := net.Pipe()
		got := make(chan error, 1)
		go func() {
			_, err := takeHello(host, newLines(host))
			got <- err
			_ = host.Close()
		}()
		_ = client.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := client.Write([]byte(hello + "\n")); err != nil {
			t.Fatal(err)
		}
		line, err := newLines(client).next()
		if err != nil {
			t.Fatalf("%s: no answer: %v", hello, err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(line, &fields); err != nil || len(fields) != 2 || fields["refused"] == nil || string(fields["speaks"]) != "[1]" {
			t.Fatalf("%s: answered %s, want exactly refused and speaks [1]", hello, line)
		}
		var refusal Refusal
		_ = json.Unmarshal(line, &refusal)
		if !strings.HasPrefix(refusal.Refused, "this host speaks 1, the client ") {
			t.Fatalf("%s: refused %q", hello, refusal.Refused)
		}
		if err := <-got; err == nil {
			t.Fatalf("%s: the hello was taken", hello)
		}
		_ = client.Close()
	}
}

// The door takes a restart from a client of this version, hands it to the
// host's loop, and answers what the loop answered: the generation now
// running, or why not.
func TestTheDoorAnswersARestartWithTheLoopsOutcome(t *testing.T) {
	dir, err := os.MkdirTemp("", "door")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	h := &Host{events: make(chan ev, 1)}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mode := make([]byte, 1)
			if _, err := conn.Read(mode); err != nil || mode[0] != modeControl {
				_ = conn.Close()
				continue
			}
			go h.door(conn)
		}
	}()
	go func() {
		for e := range h.events {
			if e.kind != kDoor || e.msg.Part == "face" {
				e.reply <- Message{T: kindFailed, Why: "no part \"face\" to restart: the parts are shell"}
				continue
			}
			e.reply <- Message{T: kindDone, Generation: 2}
		}
	}()
	if gen, err := Ask(path, "shell"); err != nil || gen != 2 {
		t.Fatalf("restart shell: generation %d, %v", gen, err)
	}
	if _, err := Ask(path, "face"); err == nil || !strings.Contains(err.Error(), "the parts are shell") {
		t.Fatalf("restart face: %v", err)
	}
	if _, err := Ask(filepath.Join(dir, "none"), "shell"); err != ErrNoHost {
		t.Fatalf("no host: %v", err)
	}
}

// The closing mark is seen once, a mark cut between two reads included.
func TestTheClosingMarkIsSeenAcrossReads(t *testing.T) {
	for cut := range len(ClosingMark) + 1 {
		seen := 0
		watch := markWatcher(func() { seen++ })
		data := "text " + ClosingMark + " more"
		at := 5 + cut
		watch([]byte(data[:at]))
		watch([]byte(data[at:]))
		watch([]byte(ClosingMark))
		if seen != 1 {
			t.Fatalf("cut at %d: seen %d times", cut, seen)
		}
	}
	seen := 0
	watch := markWatcher(func() { seen++ })
	for _, b := range []byte("x" + ClosingMark) {
		watch([]byte{b})
	}
	if seen != 1 {
		t.Fatalf("byte by byte: seen %d times", seen)
	}
}
