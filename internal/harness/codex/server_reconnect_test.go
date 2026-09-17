package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

func TestReconnectOmitsTheInitialCursor(t *testing.T) {
	const root = "0199ab12-1234-7123-8123-123456789abc"
	path := socketFixture(t, func(c net.Conn, r *bufio.Reader) {
		for {
			_, raw, _, err := readClientFrame(r)
			if err != nil {
				return
			}
			var request struct {
				ID     uint64                     `json:"id"`
				Method string                     `json:"method"`
				Params map[string]json.RawMessage `json:"params"`
			}
			if json.Unmarshal(raw, &request) != nil {
				return
			}
			switch request.Method {
			case "initialized":
				continue
			case "thread/loaded/list":
				if cursor, present := request.Params["cursor"]; present && string(cursor) == `""` {
					serverMessage(c, map[string]any{"id": request.ID, "error": map[string]any{"code": -32600, "message": "invalid cursor: "}})
					continue
				}
				serverMessage(c, map[string]any{"id": request.ID, "result": map[string]any{"data": []string{root}, "nextCursor": nil}})
				continue
			case "thread/read":
				serverMessage(c, map[string]any{"id": request.ID, "result": map[string]any{"thread": map[string]string{"id": root, "source": "cli", "originator": "codex_cli_rs"}}})
				continue
			}
			serverMessage(c, map[string]any{"id": request.ID, "result": map[string]any{}})
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := connectRPC(ctx, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	s := newServer(path, nil, nil, "")
	err = s.restore(ctx, client)
	t.Logf("restore=%v current=%q", err, s.current)
	if err != nil || s.current != root {
		t.Fatal("reconnect failed against loaded-list cursor validation")
	}
}

func TestRecoveryKeepsNewerEventsAndRejectsAmbiguity(t *testing.T) {
	for _, mode := range []string{"newer event", "newer event after resume", "two roots"} {
		t.Run(mode, func(t *testing.T) {
			s := newServer("", nil, nil, "")
			s.current = "old"
			s.generation = 1
			var resumed atomic.Int32
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
					result := any(map[string]any{})
					switch q.Method {
					case "initialized":
						continue
					case "thread/loaded/list":
						ids := []string{"old"}
						if mode == "two roots" {
							ids = append(ids, "other")
						}
						result = map[string]any{"data": ids, "nextCursor": nil}
					case "thread/read":
						if q.Params["includeTurns"] != false {
							t.Error("recovery requested history")
						}
						if mode == "newer event" {
							serverMessage(c, map[string]any{"method": "thread/started", "params": map[string]any{"thread": map[string]string{"id": "new", "source": "cli", "originator": "codex_cli_rs"}}})
						}
						result = map[string]any{"thread": map[string]any{"id": q.Params["threadId"], "source": "cli", "originator": "codex_cli_rs"}}
					case "thread/resume":
						resumed.Add(1)
						if mode == "newer event after resume" {
							serverMessage(c, map[string]any{"method": "thread/started", "params": map[string]any{"thread": map[string]string{"id": "new", "source": "cli", "originator": "codex_cli_rs"}}})
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
			if mode == "two roots" {
				s.client = client
			}
			err = s.restore(ctx, client)
			if mode == "two roots" {
				if result := s.Deliver(ctx, inbox.Message{}); result.State != inbox.Failed {
					t.Fatal("ambiguous discovery delivered to the old root")
				}
			}
			client.close()
			if strings.HasPrefix(mode, "newer event") {
				wantResumed := int32(0)
				if mode == "newer event after resume" {
					wantResumed = 1
				}
				if err != nil || s.current != "new" || resumed.Load() != wantResumed {
					t.Fatalf("new thread lost: current=%s resumed=%d err=%v", s.current, resumed.Load(), err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "2 loaded root") || resumed.Load() != 0 {
				t.Fatalf("ambiguous recovery: resumed=%d err=%v", resumed.Load(), err)
			}
		})
	}
}

func TestRecoveryFollowsOnlyReturnedCursors(t *testing.T) {
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = fmt.Sprintf("0199ab12-1234-7123-8123-%012x", i)
	}
	var pages atomic.Int32
	path := socketFixture(t, func(c net.Conn, r *bufio.Reader) {
		for {
			_, raw, _, err := readClientFrame(r)
			if err != nil {
				return
			}
			var q struct {
				ID     uint64          `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if json.Unmarshal(raw, &q) != nil {
				return
			}
			var result any = map[string]any{}
			switch q.Method {
			case "initialized":
				continue
			case "thread/loaded/list":
				pages.Add(1)
				page, err := fixtureLoadedPage(ids, q.Params)
				if err != nil {
					serverMessage(c, map[string]any{"id": q.ID, "error": map[string]any{"code": -32600, "message": err.Error()}})
					continue
				}
				result = page
			case "thread/read":
				var params struct {
					ID string `json:"threadId"`
				}
				_ = json.Unmarshal(q.Params, &params)
				source := "exec"
				if params.ID == ids[100] {
					source = "cli"
				}
				result = map[string]any{"thread": map[string]string{"id": params.ID, "source": source, "originator": "codex_cli_rs"}}
			}
			serverMessage(c, map[string]any{"id": q.ID, "result": result})
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := connectRPC(ctx, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	s := newServer(path, nil, nil, "")
	if err := s.restore(ctx, client); err != nil || s.current != ids[100] || pages.Load() != 2 {
		t.Fatalf("pagination: current=%s pages=%d err=%v", s.current, pages.Load(), err)
	}
}
