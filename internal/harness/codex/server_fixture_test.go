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
	"testing"
	"time"
)

type serverThread struct {
	ID         string          `json:"id"`
	Source     json.RawMessage `json:"source"`
	Originator string          `json:"originator"`
	Status     threadStatus    `json:"status"`
}

func tuiThread(thread serverThread) bool { return thread.Originator == "rewake" }

const fixtureRoot = "0199ab12-1234-7123-8123-123456789abc"

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
				// Its own literal, not verifiedServerVersion: a fixture echoing
				// the constant back would make the match test tautological and
				// hide a typo in the pin.
				fmt.Println("codex-cli 0.155.1")
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
		mode := "thread/start"
		for _, arg := range args {
			if arg == "resume" {
				mode = "thread/resume"
			}
		}
		if client.call(ctx, mode, map[string]any{"threadId": fixtureRoot, "excludeTurns": true, "threadSource": "user", "config": map[string]any{}, "runtimeWorkspaceRoots": []string{"/work"}}, nil) != nil {
			client.close()
			cancel()
			os.Exit(7)
		}
		if mode == "thread/resume" {
			_ = client.call(ctx, "thread/goal/get", map[string]string{"threadId": fixtureRoot}, nil)
			_ = os.WriteFile(socket+".resumed", []byte("resumed without thread/started"), 0o600)
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
	subscribers := make(map[net.Conn]map[string]bool)
	root := serverThread{ID: fixtureRoot, Source: json.RawMessage(`"vscode"`), Originator: "rewake"}
	loaded := false
	var rolloutAt time.Time
	resumeAttempts := 0
	delay, _ := time.ParseDuration(os.Getenv("RW_SERVER_ROLLOUT_DELAY"))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { mu.Lock(); delete(clients, conn); delete(subscribers, conn); mu.Unlock(); _ = conn.Close() }()
		sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		_, _ = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(sum[:]))
		_ = rw.Flush()
		mu.Lock()
		clients[conn] = false
		subscribers[conn] = make(map[string]bool)
		mu.Unlock()
		for {
			_, data, masked, err := readClientFrame(rw)
			if err != nil || !masked {
				return
			}
			var request struct {
				ID     json.RawMessage `json:"id"`
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
				clients[conn] = true
			case "initialized":
				mu.Unlock()
				continue
			case "thread/start":
				rolloutAt = time.Time{}
				root.Status.Kind = "idle"
				loaded = true
				subscribers[conn][root.ID] = true
				result = map[string]any{"thread": root, "canAcceptDirectInput": true}
				for peer, ready := range clients {
					if ready {
						serverMessage(peer, map[string]any{"method": "thread/started", "params": map[string]any{"thread": root, "canAcceptDirectInput": true}})
					}
				}
			case "thread/resume":
				resumeAttempts++
				var params struct {
					ExcludeTurns bool `json:"excludeTurns"`
				}
				_ = json.Unmarshal(request.Params, &params)
				if !params.ExcludeTurns {
					serverMessage(conn, map[string]any{"id": request.ID, "error": map[string]any{"code": -32600, "message": "fixture forbids history"}})
					mu.Unlock()
					continue
				}
				if reason := os.Getenv("RW_SERVER_RESUME_ERROR"); loaded && reason != "" {
					serverMessage(conn, map[string]any{"id": request.ID, "error": map[string]any{"code": -32600, "message": reason}})
					mu.Unlock()
					continue
				}
				if loaded && (rolloutAt.IsZero() || time.Now().Before(rolloutAt)) {
					serverMessage(conn, map[string]any{"id": request.ID, "error": map[string]any{"code": -32600, "message": "no rollout found for thread id " + root.ID}})
					mu.Unlock()
					continue
				}
				if !loaded {
					rolloutAt = time.Now()
				}
				subscribers[conn][root.ID] = true
				wasLoaded := loaded
				loaded = true
				result = map[string]any{"thread": root, "canAcceptDirectInput": true}
				if !wasLoaded && os.Getenv("RW_SERVER_NO_STATUS") != "1" {
					for peer, ready := range clients {
						if ready {
							serverMessage(peer, map[string]any{"method": "thread/status/changed", "params": map[string]any{"threadId": root.ID, "status": map[string]string{"type": "idle"}}})
						}
					}
				}
			case "thread/unsubscribe":
				var params struct {
					ThreadID string `json:"threadId"`
				}
				_ = json.Unmarshal(request.Params, &params)
				delete(subscribers[conn], params.ThreadID)
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
				if rolloutAt.IsZero() {
					rolloutAt = time.Now().Add(delay)
				}
				root.Status.Kind = "active"
				for peer, ready := range clients {
					if ready {
						serverMessage(peer, map[string]any{"method": "thread/status/changed", "params": map[string]any{"threadId": root.ID, "status": root.Status}})
					}
				}
				result = map[string]any{"turn": map[string]string{"id": "active-turn"}}
			case "thread/loaded/list":
				ids := []string{}
				if loaded {
					ids = append(ids, root.ID)
				}
				page, err := fixtureLoadedPage(ids, request.Params)
				if err != nil {
					serverMessage(conn, map[string]any{"id": request.ID, "error": map[string]any{"code": -32600, "message": err.Error()}})
					mu.Unlock()
					continue
				}
				result = page
			case "thread/read":
				var params map[string]any
				_ = json.Unmarshal(request.Params, &params)
				if params["includeTurns"] != false {
					serverMessage(conn, map[string]any{"id": request.ID, "error": map[string]any{"code": -32600, "message": "fixture forbids history"}})
					mu.Unlock()
					continue
				}
				result = map[string]any{"thread": root, "canAcceptDirectInput": true}
			case "fixture/replace-disconnect":
				root.ID = "after-disconnect"
				rolloutAt = time.Now()
				mu.Unlock()
				return
			case "fixture/disconnect":
				mu.Unlock()
				return
			case "fixture/inspect":
				result = map[string]any{"resumeAttempts": resumeAttempts, "subscribed": subscribers[conn][root.ID]}
			case "fixture/emit":
				var event any
				_ = json.Unmarshal(request.Params, &event)
				var metadata struct {
					Method string `json:"method"`
					Params struct {
						Status   threadStatus `json:"status"`
						Thread   serverThread `json:"thread"`
						ThreadID string       `json:"threadId"`
					} `json:"params"`
				}
				_ = json.Unmarshal(request.Params, &metadata)
				if metadata.Method == "thread/started" && tuiThread(metadata.Params.Thread) {
					root = metadata.Params.Thread
					rolloutAt = time.Time{}
					loaded = true
				}
				if metadata.Method == "thread/closed" && metadata.Params.ThreadID == root.ID {
					loaded = false
				}
				if metadata.Method == "thread/status/changed" && metadata.Params.ThreadID == root.ID {
					root.Status = metadata.Params.Status
					if root.Status.Kind == "active" && rolloutAt.IsZero() {
						rolloutAt = time.Now().Add(delay)
					}
				}
				scoped := strings.HasPrefix(metadata.Method, "turn/") || strings.HasPrefix(metadata.Method, "item/") || metadata.Method == "error"
				for peer, ready := range clients {
					if ready && (!scoped || subscribers[peer][metadata.Params.ThreadID]) {
						serverMessage(peer, event)
					}
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
