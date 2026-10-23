package modhost

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// The status line is a command the harness starts, so its endpoint takes the
// harness's descendants where every other takes only the harness's own
// process; a process beside the harness is refused there too, and the cache's
// life reaches the snapshot as an absolute expiry.
func TestTheStatusLineIsServedToTheHarnessesDescendantsOnly(t *testing.T) {
	server, socket := serving(t)
	body := `{"ttl":"1h","expiresAt":4102444800,"warm":true}`
	child := exec.Command(os.Args[0])
	child.Env = append(os.Environ(), helperSocket+"="+socket, helperStatusLine+"="+body)
	if out, err := child.Output(); err != nil || strings.TrimSpace(string(out)) != "200" {
		t.Fatalf("a process below the harness: %s %v", out, err)
	}
	life := server.SessionState().CacheLife
	if life == nil || life.TTL != "1h" || !life.Warm || life.ExpiresAt == nil || life.ExpiresAt.Unix() != 4102444800 {
		t.Fatalf("the cache's life: %+v", life)
	}
	if left := life.Remaining(time.Unix(4102444800-90, 0)); left != 90*time.Second {
		t.Fatalf("remaining: %v", left)
	}
	if life.Remaining(time.Unix(4102444801, 0)) != 0 {
		t.Fatal("an expired cache has time left")
	}
	if status, _, err := post(socket, PathStatusLine, []byte(`{"warm":false,"ttl":"1d"}`)); err != nil || status != 200 {
		t.Fatalf("the harness's own process: %d %v", status, err)
	}
	if life := server.SessionState().CacheLife; life == nil || life.TTL != "" || life.Warm {
		t.Fatalf("an unknown tier is not kept: %+v", life)
	}
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
	if status, _, err := post(socket, PathStatusLine, []byte(body)); err == nil {
		t.Fatalf("a process beside the harness was answered: %d", status)
	}
}
