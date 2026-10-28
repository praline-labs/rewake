package spacehost

import (
	"bufio"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/screen"
)

// A client that connects and sends no mode is dropped, not held.
func TestAClientThatSendsNoModeIsDropped(t *testing.T) {
	old := modeWithin.Swap(int64(100 * time.Millisecond))
	t.Cleanup(func() { modeWithin.Store(old) })
	socket := served(t, `sleep 30`)
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil || strings.Contains(err.Error(), "timeout") {
		t.Fatalf("the silent client was not closed: %v", err)
	}
}

// A snapshot is the screen as the emulator holds it, in one line, and asks
// nothing of the worker's size.
func TestASnapshotAnswersTheScreenAsOneFrame(t *testing.T) {
	socket := served(t, `printf '\033[1;31mred\033[0m 世界'; sleep 30`)
	var frame screen.Frame
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.Dial("unix", socket)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Write([]byte{modeSnapshot}); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(conn).ReadBytes('\n')
		_ = conn.Close()
		if err != nil {
			t.Fatal(err)
		}
		frame = screen.Frame{}
		if err := json.Unmarshal(line, &frame); err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(frame.Text()[0], "red 世界") || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := frame.Check(); err != nil {
		t.Fatal(err)
	}
	if frame.Source != screen.SourceHost || frame.Cols != 60 || frame.Rows != 10 {
		t.Fatalf("frame = %s %dx%d", frame.Source, frame.Cols, frame.Rows)
	}
	if got := frame.Text()[0]; got != "red 世界" {
		t.Fatalf("first line = %q", got)
	}
	first := frame.Lines[0][0]
	if first.T != "red" || first.Fg != "#cd0000" || first.A&screen.Bold == 0 {
		t.Fatalf("first run = %+v", first)
	}
}
