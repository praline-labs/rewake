package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

func TestResumedTUIIsDiscoveredWithoutThreadStarted(t *testing.T) {
	for _, silent := range []bool{false, true} {
		t.Run(map[bool]string{false: "status event", true: "loaded-list fallback"}[silent], func(t *testing.T) {
			fakeServerExecutable(t)
			codexHome(t, "")
			if silent {
				t.Setenv("RW_SERVER_NO_STATUS", "1")
			}
			dir := t.TempDir()
			socket := filepath.Join(dir, "s.sock")
			server := newServer(socket, []string{"app-server", "--listen", "unix://" + socket}, harness.SessionEnv(harness.LaunchRequest{Name: "api", Dir: dir}, nil), dir)
			if err := server.Start(context.Background(), nil, func(string) {}); err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			server.mu.Lock()
			initial := server.current
			server.mu.Unlock()
			if initial != "" {
				t.Fatal("initialize fabricated a TUI thread")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			tui, err := connectRPC(ctx, socket, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tui.close()
			if err := tui.call(ctx, "thread/resume", map[string]any{"threadId": fixtureRoot, "excludeTurns": true}, nil); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				server.mu.Lock()
				discovered := server.current == fixtureRoot
				server.mu.Unlock()
				if discovered {
					break
				}
				time.Sleep(time.Millisecond)
			}
			server.mu.Lock()
			discovered := server.current == fixtureRoot
			server.mu.Unlock()
			if !discovered {
				t.Fatal("background discovery did not bind the resumed TUI")
			}
			result := server.Deliver(ctx, inbox.Message{ID: "resumed-task", Text: "task after resume"})
			if result.State != inbox.Delivered {
				t.Fatalf("resumed TUI cannot receive: %+v", result)
			}
			if thread, err := server.Thread(); err != nil || thread != fixtureRoot {
				t.Fatalf("thread=%q err=%v", thread, err)
			}
		})
	}
}

func TestUnknownThreadHintsValidateMetadataBeforeReporting(t *testing.T) {
	for _, mode := range []struct {
		name            string
		child, previous bool
	}{{name: "root"}, {name: "resumed replacement", previous: true}, {name: "child", child: true}} {
		t.Run(mode.name, func(t *testing.T) {
			s := newServer("", nil, nil, "")
			var subscriptions atomic.Int32
			if mode.previous {
				s.current = "previous"
			}
			path := socketFixture(t, func(c net.Conn, r *bufio.Reader) {
				for {
					_, raw, _, err := readClientFrame(r)
					if err != nil {
						return
					}
					var q struct {
						ID     uint64         `json:"id"`
						Method string         `json:"method"`
						Params map[string]any `json:"params"`
					}
					if json.Unmarshal(raw, &q) != nil {
						return
					}
					var result any = map[string]any{}
					switch q.Method {
					case "initialized":
						continue
					case "thread/loaded/list":
						result = map[string]any{"data": []string{fixtureRoot}}
					case "thread/read":
						if q.Params["includeTurns"] != false {
							t.Error("discovery read history")
						}
						thread := map[string]any{"id": fixtureRoot, "source": "cli", "originator": "codex_cli_rs"}
						if mode.child {
							thread["parentThreadId"] = "parent"
						}
						result = map[string]any{"thread": thread}
					case "thread/resume":
						subscriptions.Add(1)
						if q.Params["excludeTurns"] != true {
							t.Error("subscription requested history")
						}
					}
					serverMessage(c, map[string]any{"id": q.ID, "result": result})
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			client, err := connectRPC(ctx, path, s.event)
			if err != nil {
				t.Fatal(err)
			}
			defer client.close()
			s.client = client
			hint, _ := json.Marshal(map[string]string{"threadId": fixtureRoot})
			s.event("thread/name/updated", hint)
			select {
			case <-s.discoverWake:
			default:
				t.Fatal("metadata hint did not request discovery")
			}
			raw, _ := json.Marshal(map[string]any{"threadId": fixtureRoot, "turn": map[string]any{"id": "early-result", "status": "completed", "items": []map[string]string{{"type": "agentMessage", "text": "result before discovery"}}}})
			s.event("turn/completed", raw)
			select {
			case <-s.discoverWake:
			default:
				t.Fatal("unknown-thread event did not request discovery")
			}
			err = s.ensureThread(ctx)
			s.mu.Lock()
			defer s.mu.Unlock()
			if mode.child {
				if subscriptions.Load() != 0 {
					t.Fatal("subscribed to a child")
				}
				if err == nil || s.current != "" || len(s.outcomes) != 0 || len(s.delayed) != 0 {
					t.Fatal("child completion became parent work")
				}
			} else if subscriptions.Load() != 1 || err != nil || s.current != fixtureRoot || len(s.outcomes) != 1 || s.outcomes[0].Text != "result before discovery" {
				t.Fatalf("early root result was lost: current=%s outcomes=%+v err=%v", s.current, s.outcomes, err)
			}
		})
	}
}
