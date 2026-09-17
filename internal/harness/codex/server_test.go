package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

func runtimeFixture(t *testing.T) (*serverSession, <-chan harness.Completion) {
	t.Helper()
	fakeServerExecutable(t)
	codexHome(t, "")
	dir := t.TempDir()
	socket := filepath.Join(dir, "s.sock")
	request := harness.LaunchRequest{Name: "api", Dir: dir, Room: "work", Epoch: "1.2", Socket: socket}
	env := harness.SessionEnv(request, nil)
	server := newServer(socket, []string{"app-server", "--listen", "unix://" + socket}, env, dir)
	outcomes := make(chan harness.Completion, 10)
	if err := server.Start(context.Background(), func(outcome harness.Completion) error { outcomes <- outcome; return nil }, func(string) {}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	tui, err := connectRPC(ctx, socket, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	err = tui.call(ctx, "thread/start", map[string]any{}, nil)
	tui.close()
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if thread, err := server.Thread(); err == nil && thread == fixtureRoot {
			return server, outcomes
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("server did not publish its TUI thread")
	return nil, nil
}

func emitFixture(t *testing.T, s *serverSession, method string, params any) {
	t.Helper()
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.call(ctx, "fixture/emit", map[string]any{"method": method, "params": params}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestOwnedServerDeliversAndTracksOnlyTheTUI(t *testing.T) {
	s, _ := runtimeFixture(t)
	if group, err := syscall.Getpgid(s.process.Process.Pid); err != nil || group != s.process.Process.Pid || group == syscall.Getpgrp() {
		t.Fatalf("server shares terminal signals: group=%d err=%v", group, err)
	}
	raw, err := os.ReadFile(s.path + ".env")
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]string
	_ = json.Unmarshal(raw, &env)
	if env["REWAKE_SESSION"] != "api" || env["REWAKE_ROOM"] != "work" {
		t.Fatalf("server identity=%v", env)
	}
	result := s.Deliver(context.Background(), inbox.Message{ID: "one", Text: "notice"})
	if result.State != inbox.Delivered || result.Via != "app-server" {
		t.Fatalf("delivery=%+v", result)
	}
	for _, thread := range []map[string]any{
		{"id": "child", "source": "vscode", "originator": "rewake", "parentThreadId": fixtureRoot},
		{"id": "foreign", "source": "vscode", "originator": "another-client"},
		{"id": "exec", "source": "exec", "originator": "rewake"},
	} {
		emitFixture(t, s, "thread/started", map[string]any{"thread": thread})
	}
	if thread, _ := s.Thread(); thread != fixtureRoot {
		t.Fatalf("selected a non-TUI thread: %s", thread)
	}
	emitFixture(t, s, "thread/started", map[string]any{"thread": map[string]string{"id": "new", "source": "vscode", "originator": "rewake"}})
	emitFixture(t, s, "thread/closed", map[string]string{"threadId": fixtureRoot})
	if result := s.Deliver(context.Background(), inbox.Message{DeliveryThread: fixtureRoot}); result.State != inbox.Failed {
		t.Fatal("delivered into the closed thread")
	}
	if result := s.Deliver(context.Background(), inbox.Message{ID: "new-task", DeliveryThread: "new"}); result.State != inbox.Delivered {
		t.Fatalf("fresh thread did not accept its first input: %+v", result)
	}
	emitFixture(t, s, "thread/started", map[string]any{"thread": map[string]string{"id": "refused", "source": "cli", "originator": "rewake"}})
	if result := s.Deliver(context.Background(), inbox.Message{}); result.State != inbox.Failed || !strings.Contains(result.Detail, "server refused the thread") {
		t.Fatalf("lost server error: %+v", result)
	}
	s.Close()
	select {
	case <-s.Done():
	default:
		t.Fatal("server survived session shutdown")
	}
}

func TestServerReportsTerminalEventsAndReconnects(t *testing.T) {
	s, outcomes := runtimeFixture(t)
	beginSubscribedTurn(t, s, fixtureRoot)
	emitFixture(t, s, "error", map[string]any{"threadId": fixtureRoot, "willRetry": true})
	emitFixture(t, s, "turn/completed", map[string]any{"threadId": "child", "turn": map[string]string{"id": "child-turn", "status": "failed"}})
	select {
	case result := <-outcomes:
		t.Fatalf("nonterminal or child result=%+v", result)
	default:
	}
	for _, status := range []string{"completed", "failed", "interrupted"} {
		emitFixture(t, s, "turn/completed", map[string]any{"threadId": fixtureRoot, "turn": map[string]any{"id": status, "status": status, "items": []map[string]string{{"type": "agentMessage", "text": "final text"}}, "error": map[string]string{"message": "original error"}}})
		select {
		case result := <-outcomes:
			want := map[string]inbox.Kind{"completed": inbox.Finished, "failed": inbox.Error, "interrupted": inbox.Stopped}[status]
			if result.Kind != want || result.ID != fixtureRoot+"/"+status {
				t.Fatalf("result=%+v", result)
			}
			if status == "failed" && result.Text != "original error" {
				t.Fatal("error text changed")
			}
		case <-time.After(time.Second):
			t.Fatal("terminal event lost")
		}
	}
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = client.call(ctx, "fixture/replace-disconnect", nil, nil)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		changed := s.client != nil && s.client != client
		s.mu.Unlock()
		if changed {
			if thread, err := s.Thread(); err != nil || thread != "after-disconnect" {
				t.Fatalf("lost restored root: %s %v", thread, err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("transport did not reconnect")
}
