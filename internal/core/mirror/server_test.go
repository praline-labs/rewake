package mirror

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func serving(t *testing.T, m *Model) string {
	t.Helper()
	// A socket's path is bounded; the test's own temporary directory is
	// too deep for one.
	base, err := os.MkdirTemp("", "mirror")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	dir := buildtest.Room(t, base, "room")
	path := state.SocketPath(dir, "write-claude", "1-2-3.mirror")
	server, err := Serve(path, m)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Error(err)
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the socket outlived its server: %v", err)
		}
	})
	return path
}

func next(t *testing.T, reader *bufio.Reader) Line {
	t.Helper()
	raw, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var line Line
	if err := json.Unmarshal(raw, &line); err != nil {
		t.Fatal(err)
	}
	return line
}

// A face gets the snapshot, then the changes, and its prompt reaches the
// harness and is answered on the same stream.
func TestAFaceGetsTheSnapshotThenChangesAndSendsAPrompt(t *testing.T) {
	m := New()
	m.Apply(Event{Kind: EventPrompt, ID: "p", Text: "before"})
	taken := make(chan string, 1)
	m.Take(func(_ context.Context, input Input) error {
		taken <- input.Text
		return nil
	})
	conn, err := net.Dial("unix", serving(t, m))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(conn)
	hello := next(t, reader)
	if hello.Op != LineOpHello || *hello.V != Version || len(hello.Inputs) != 1 {
		t.Fatalf("hello: %+v", hello)
	}
	if begin := next(t, reader); begin.Op != LineOpBegin || *begin.Items != 1 {
		t.Fatalf("begin: %+v", begin)
	}
	if item := next(t, reader); item.Op != LineOpItem || item.Item.Text != "before" {
		t.Fatalf("item: %+v", item)
	}
	if end := next(t, reader); end.Op != LineOpEnd {
		t.Fatalf("end: %+v", end)
	}
	m.Apply(Event{Kind: EventDelta, ID: "b", Text: "after"})
	if add := next(t, reader); add.Op != LineOpAdd || add.Item.Text != "after" {
		t.Fatalf("add: %+v", add)
	}
	if _, err := conn.Write([]byte(`{"id":"i1","kind":"prompt","text":"go on"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	if got := <-taken; got != "go on" {
		t.Fatalf("the harness got %q", got)
	}
	if answer := next(t, reader); answer.Op != LineOpAnswer || *answer.ID != "i1" || !*answer.Ok {
		t.Fatalf("answer: %+v", answer)
	}
	if _, err := conn.Write([]byte(`{"id":"i2","kind":"steer","text":"x"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	if answer := next(t, reader); *answer.Ok || answer.Detail == nil {
		t.Fatalf("an unknown input was taken: %+v", answer)
	}
}
