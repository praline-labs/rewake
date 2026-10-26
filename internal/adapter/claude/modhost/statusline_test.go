package modhost

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/session"
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

// The effort shown is the one whose source changed last: the status line's
// setting follows /effort before a request, and a request's own effort wins
// once it differs from the last one, while a sample repeating it does not.
func TestTheEffortShownIsTheSourceThatChangedLast(t *testing.T) {
	server, socket := servingWith(t, func(s *Server) { s.cfg.Effort = "high" })
	if got := server.SessionState(); got.Effort == nil || *got.Effort != "high" || got.EffortSource != session.EffortFromLaunch {
		t.Fatalf("before anything: %+v", got.Effort)
	}
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	sample := testState()
	for step, round := range []struct {
		line, request, want string
	}{
		{request: "medium", want: "medium"},
		{line: "medium", want: "medium"},
		{line: "max", want: "max"},
		{request: "medium", want: "max"},
		{request: "max", want: "max"},
		{request: "low", want: "low"},
		{line: "max", want: "low"},
		{line: "high", want: "high"},
	} {
		if round.request != "" {
			sample.Sequence++
			sample.Effort = &round.request
			postValue(t, socket, PathModState, sample)
		}
		if round.line != "" {
			if status, _, err := post(socket, PathStatusLine, []byte(`{"effort":"`+round.line+`"}`)); err != nil || status != 200 {
				t.Fatalf("step %d: the status line: %d %v", step, status, err)
			}
		}
		if got := server.SessionState(); got.Effort == nil || *got.Effort != round.want || got.EffortSource != "" {
			t.Fatalf("step %d: effort %v (%q), want %q", step, got.Effort, got.EffortSource, round.want)
		}
	}
	if server.SessionState().CacheLife != nil {
		t.Fatal("an effort alone set the cache's life")
	}
}

// The model's name is the status line's display_name: kept once told, changed
// by a later one (/model), and not blanked by a line without it.
func TestTheModelNameIsTheStatusLinesOwn(t *testing.T) {
	server, socket := serving(t)
	if got := server.SessionState().ModelName; got != nil {
		t.Fatalf("before anything: %q", *got)
	}
	for _, round := range []struct{ body, want string }{
		{`{"modelName":"Haiku 5.5"}`, "Haiku 5.5"},
		{`{"effort":"low"}`, "Haiku 5.5"},
		{`{"modelName":"Opus 5.5"}`, "Opus 5.5"},
	} {
		if status, _, err := post(socket, PathStatusLine, []byte(round.body)); err != nil || status != 200 {
			t.Fatalf("%s: %d %v", round.body, status, err)
		}
		if got := server.SessionState().ModelName; got == nil || *got != round.want {
			t.Fatalf("%s: name %v, want %q", round.body, got, round.want)
		}
	}
}
