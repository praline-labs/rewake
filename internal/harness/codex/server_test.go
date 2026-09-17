package codex

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

func TestServerProcessHelper(_ *testing.T) {
	if os.Getenv("RW_SERVER_HELPER") != "1" {
		return
	}
	args := os.Args
	for _, arg := range args {
		if arg == "--version" {
			if version := os.Getenv("RW_SERVER_VERSION"); version != "" {
				fmt.Println(version)
			} else {
				fmt.Println("codex-cli 0.154.0")
			}
			os.Exit(0)
		}
	}
	var socket string
	for i, arg := range args {
		if arg == "--listen" && i+1 < len(args) {
			socket = strings.TrimPrefix(args[i+1], "unix://")
		}
	}
	if socket == "" {
		for i, arg := range args {
			if arg == "--remote" && i+1 < len(args) {
				socket = strings.TrimPrefix(args[i+1], "unix://")
			}
		}
		if socket == "" {
			os.Exit(2)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		client, err := connectRPC(ctx, socket, nil)
		if err != nil {
			os.Exit(4)
		}
		for i, arg := range args {
			if arg == "fixture-event" && i+1 < len(args) {
				var event any
				if json.Unmarshal([]byte(args[i+1]), &event) != nil {
					os.Exit(5)
				}
				if client.call(ctx, "fixture/emit", event, nil) != nil {
					os.Exit(6)
				}
				client.close()
				cancel()
				os.Exit(0)
			}
		}
		cancel()
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(os.Getenv("RW_SERVER_TUI_EXIT")); err == nil {
				client.close()
				os.Exit(0)
			}
			time.Sleep(20 * time.Millisecond)
		}
		client.close()
		os.Exit(0)
	}
	markers := map[string]string{}
	for _, key := range []string{"REWAKE_SESSION", "REWAKE_EPOCH", "REWAKE_ROOM", "REWAKE_DIR"} {
		markers[key] = os.Getenv(key)
	}
	_ = os.WriteFile(socket+".pid", []byte(fmt.Sprint(os.Getpid())), 0o600)
	raw, _ := json.Marshal(markers)
	_ = os.WriteFile(socket+".env", raw, 0o600)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		os.Exit(3)
	}
	var mu sync.Mutex
	clients := make(map[net.Conn]bool)
	root := serverThread{ID: "root", Source: json.RawMessage(`"vscode"`), Originator: "rewake"}
	announced := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { mu.Lock(); delete(clients, conn); mu.Unlock(); _ = conn.Close() }()
		sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		_, _ = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(sum[:]))
		_ = rw.Flush()
		mu.Lock()
		clients[conn] = true
		mu.Unlock()
		for {
			_, data, masked, err := readClientFrame(rw)
			if err != nil || !masked {
				return
			}
			var request struct {
				ID     uint64          `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if json.Unmarshal(data, &request) != nil {
				return
			}
			mu.Lock()
			result := any(map[string]any{})
			switch request.Method {
			case "initialize":
			case "initialized":
				if !announced {
					serverMessage(conn, map[string]any{"method": "thread/started", "params": map[string]any{"thread": root}})
					announced = true
				}
				mu.Unlock()
				continue
			case "turn/start":
				var params struct {
					ThreadID string `json:"threadId"`
				}
				_ = json.Unmarshal(request.Params, &params)
				if params.ThreadID == "refused" {
					serverMessage(conn, map[string]any{"id": request.ID, "error": map[string]any{"code": -32600, "message": "server refused the thread"}})
					mu.Unlock()
					continue
				}
				result = map[string]any{"turn": map[string]string{"id": "active-turn"}}
			case "thread/loaded/list":
				result = map[string]any{"data": []string{root.ID}}
			case "thread/read":
				result = map[string]any{"thread": root}
			case "fixture/replace-disconnect":
				root.ID = "after-disconnect"
				mu.Unlock()
				return
			case "fixture/disconnect":
				mu.Unlock()
				return
			case "fixture/emit":
				var event any
				_ = json.Unmarshal(request.Params, &event)
				var metadata struct {
					Method string `json:"method"`
					Params struct {
						Thread serverThread `json:"thread"`
					} `json:"params"`
				}
				_ = json.Unmarshal(request.Params, &metadata)
				if metadata.Method == "thread/started" && tuiThread(metadata.Params.Thread) {
					root = metadata.Params.Thread
				}
				for peer := range clients {
					serverMessage(peer, event)
				}
			}
			serverMessage(conn, map[string]any{"id": request.ID, "result": result})
			mu.Unlock()
		}
	})
	_ = http.Serve(listener, handler)
	os.Exit(0)
}

func fakeServerExecutable(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	text := "#!/bin/sh\nexec \"$RW_SERVER_TEST_EXE\" -test.run=TestServerProcessHelper -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(text), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("RW_SERVER_HELPER", "1")
	t.Setenv("RW_SERVER_TEST_EXE", os.Args[0])
}

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
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if thread, err := server.Thread(); err == nil && thread == "root" {
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
		{"id": "child", "source": "vscode", "originator": "rewake", "parentThreadId": "root"},
		{"id": "foreign", "source": "vscode", "originator": "another-client"},
		{"id": "exec", "source": "exec", "originator": "rewake"},
	} {
		emitFixture(t, s, "thread/started", map[string]any{"thread": thread})
	}
	if thread, _ := s.Thread(); thread != "root" {
		t.Fatalf("selected a non-TUI thread: %s", thread)
	}
	emitFixture(t, s, "thread/started", map[string]any{"thread": map[string]string{"id": "new", "source": "vscode", "originator": "rewake"}})
	emitFixture(t, s, "thread/closed", map[string]string{"threadId": "root"})
	if result := s.Deliver(context.Background(), inbox.Message{DeliveryThread: "root"}); result.State != inbox.Failed {
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
	emitFixture(t, s, "error", map[string]any{"threadId": "root", "willRetry": true})
	emitFixture(t, s, "turn/completed", map[string]any{"threadId": "child", "turn": map[string]string{"id": "child-turn", "status": "failed"}})
	select {
	case result := <-outcomes:
		t.Fatalf("nonterminal or child result=%+v", result)
	default:
	}
	for _, status := range []string{"completed", "failed", "interrupted"} {
		emitFixture(t, s, "turn/completed", map[string]any{"threadId": "root", "turn": map[string]any{"id": status, "status": status, "items": []map[string]string{{"type": "agentMessage", "text": "final text"}}, "error": map[string]string{"message": "original error"}}})
		select {
		case result := <-outcomes:
			want := map[string]inbox.Kind{"completed": inbox.Finished, "failed": inbox.Error, "interrupted": inbox.Stopped}[status]
			if result.Kind != want || result.ID != "root/"+status {
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
