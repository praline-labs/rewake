package modhost

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The probe of .scratch/reexec-probe.md at the mod's endpoint: an image that
// hands its listener and its order of loads to the next has the unchanged
// mod's next poll answered 200, where a fresh server answers 400 and the
// mod's heartbeat stops; a superseded load stays refused, and the live
// generation is reported proven to the new image's run.
func TestAHandedOverEndpointAnswersTheModsNextPoll(t *testing.T) {
	short := func(s *Server) { s.cfg.Hold = 20 * time.Millisecond }
	before, socket := servingWith(t, short)
	for _, load := range []string{"older", "current"} {
		if status, text, err := post(socket, PathHello, validHello(load)); err != nil || status != http.StatusOK {
			t.Fatalf("the hello of %s: %d %s %v", load, status, text, err)
		}
	}
	poll, _ := json.Marshal(AliveRequest{LoadID: "current", Generation: 2})
	if status, text, _ := post(socket, PathModAlive, poll); status != http.StatusOK {
		t.Fatalf("the poll before the handover: %d %s", status, text)
	}

	handed, file, err := before.HandOver()
	if err != nil {
		t.Fatal(err)
	}
	// The exec: the old image's descriptor goes, the socket's path and the
	// copy handed over stay.
	_ = before.listener.UnixListener.Close()

	// A fresh server for the same load, as a new image without the
	// handover would be: the mod's poll is refused and its heartbeat stops.
	fresh := New(Config{Socket: filepath.Join(filepath.Dir(socket), "fresh.sock"), Role: "main", Wait: 2 * time.Second, Hold: 20 * time.Millisecond})
	if err := fresh.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.Close)
	fresh.HarnessStarted(handed.Harness)
	if status, text, _ := post(fresh.cfg.Socket, PathModAlive, poll); status != http.StatusBadRequest {
		t.Fatalf("a fresh server answered the poll %d %s, want 400", status, text)
	}

	var heard heard
	after := New(Config{Socket: socket, Role: "main", Wait: 2 * time.Second, Hold: 20 * time.Millisecond, Live: heard.hear})
	if err := after.Adopt(handed, file); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(after.Close)
	if status, text, _ := post(socket, PathModAlive, poll); status != http.StatusOK {
		t.Fatalf("the poll after the handover: %d %s", status, text)
	}
	stale, _ := json.Marshal(AliveRequest{LoadID: "older", Generation: 1})
	if status, _, _ := post(socket, PathModAlive, stale); status != http.StatusGone {
		t.Fatalf("the superseded load's poll after the handover: %d, want 410", status)
	}
	again, text, _ := post(socket, PathHello, validHello("current"))
	var answer HelloAnswer
	if again != http.StatusOK || json.Unmarshal([]byte(text), &answer) != nil || answer.Generation != 2 {
		t.Fatalf("the current load's hello again: %d %s, want its generation 2 kept", again, text)
	}
	heard.want(t, transition{handed.Transition + 1, true})
	if _, err := os.Stat(socket); err != nil {
		t.Fatalf("the socket's path went with the old image: %v", err)
	}
}
