package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

func runtimeFixture(t *testing.T, publisher ...func(harness.Completion) error) (*serverSession, *rpcClient, <-chan harness.Completion) {
	t.Helper()
	fakeServerExecutable(t)
	codexHome(t, "")
	dir := t.TempDir()
	socket := filepath.Join(dir, "s.sock")
	request := harness.LaunchRequest{Name: "api", Dir: dir, Room: "work", Epoch: "1.2", Socket: socket}
	server := newServer(socket, []string{"app-server", "--listen", "unix://" + socket}, harness.SessionEnv(request, nil), dir)
	outcomes := make(chan harness.Completion, 10)
	emit := func(result harness.Completion) error { outcomes <- result; return nil }
	if len(publisher) > 0 {
		emit = publisher[0]
	}
	if err := server.Start(context.Background(), harness.CompletionHandler{Publish: func(_ context.Context, c harness.Completion) error { return emit(c) }}, func(string) {}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tui, err := connectRPC(ctx, socket, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tui.close)
	if err := tui.call(ctx, "thread/start", map[string]any{"threadSource": "user", "runtimeWorkspaceRoots": []string{dir}}, nil); err != nil {
		t.Fatal(err)
	}
	return server, tui, outcomes
}

func emitFixture(t *testing.T, client *rpcClient, method string, params any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.call(ctx, "fixture/emit", map[string]any{"method": method, "params": params}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestOwnedServerDeliversAndTracksOnlyAcceptedIntent(t *testing.T) {
	s, ui, _ := runtimeFixture(t)
	if group, err := syscall.Getpgid(s.process.Process.Pid); err != nil || group != s.process.Process.Pid || group == syscall.Getpgrp() {
		t.Fatalf("server signal group=%d %v", group, err)
	}
	raw, err := os.ReadFile(s.upstream + ".env")
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]string
	_ = json.Unmarshal(raw, &env)
	if env["REWAKE_SESSION"] != "api" || env["REWAKE_ROOM"] != "work" {
		t.Fatal(env)
	}
	result := s.Deliver(context.Background(), inbox.Message{ID: "one", Text: "notice"})
	if result.State != inbox.Delivered || result.Via != "app-server" {
		t.Fatal(result)
	}
	for _, id := range []string{"child", "foreign", "new"} {
		emitFixture(t, ui, "thread/started", map[string]any{"thread": map[string]string{"id": id, "source": "vscode", "originator": "rewake"}})
	}
	if thread, _ := s.Thread(); thread != fixtureRoot {
		t.Fatalf("broadcast changed target: %s", thread)
	}
	if result := s.Deliver(context.Background(), inbox.Message{ID: "stale", DeliveryThread: "other"}); result.State != inbox.Failed {
		t.Fatal(result)
	}
	s.Close()
	select {
	case <-s.Done():
	default:
		t.Fatal("server survived shutdown")
	}
}

func TestServerReportsScopedTerminalOutcomes(t *testing.T) {
	s, ui, outcomes := runtimeFixture(t)
	for _, status := range []string{"interrupted", "failed", "completed"} {
		result := s.Deliver(context.Background(), inbox.Message{ID: status})
		if result.State != inbox.Delivered {
			t.Fatal(result)
		}
		emitFixture(t, ui, "turn/started", map[string]any{"threadId": fixtureRoot, "turn": map[string]string{"id": "active-turn"}})
		emitFixture(t, ui, "item/completed", map[string]any{"threadId": fixtureRoot, "turnId": "active-turn", "item": map[string]string{"type": "agentMessage", "text": "final text"}})
		emitFixture(t, ui, "turn/completed", map[string]any{"threadId": fixtureRoot, "turn": map[string]any{"id": "active-turn", "status": status, "error": map[string]string{"message": "original error"}}})
		// The fixed fixture turn can continue after stopped, but a terminal outcome deduplicates it.
		if status == "completed" {
			select {
			case result := <-outcomes:
				t.Fatalf("duplicate terminal result: %+v", result)
			case <-time.After(50 * time.Millisecond):
			}
			continue
		}
		select {
		case result := <-outcomes:
			want := inbox.Error
			if status == "interrupted" {
				want = inbox.Stopped
			}
			if result.Kind != want || result.ID != fixtureRoot+"/active-turn" {
				t.Fatal(result)
			}
		case <-time.After(time.Second):
			t.Fatal("terminal result lost")
		}
	}
}
