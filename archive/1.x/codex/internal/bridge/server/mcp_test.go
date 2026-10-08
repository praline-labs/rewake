package server_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
)

// rpcAnswer is a JSON-RPC answer as the client reads it.
type rpcAnswer struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decode(t *testing.T, line json.RawMessage) rpcAnswer {
	t.Helper()
	var answer rpcAnswer
	if err := json.Unmarshal(line, &answer); err != nil {
		t.Fatalf("an answer that does not parse: %s", line)
	}
	return answer
}

func (c *mcpClient) strayLine(t *testing.T) rpcAnswer {
	t.Helper()
	select {
	case line := <-c.stray:
		return decode(t, line)
	case <-time.After(5 * time.Second):
		t.Fatal("no answer with id null")
		return rpcAnswer{}
	}
}

// The protocol around the one tool: initialize echoes the version asked,
// ping answers, the list holds one tool, and everything else is an error.
func TestTheServerSpeaksMCP(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	server := startServer(t, r.env())
	early := decode(t, server.request(t, "tools/call", map[string]any{"name": "rewake", "arguments": map[string]any{"words": []string{"whoami"}}}))
	if early.Error == nil || early.Error.Code != -32600 {
		t.Fatalf("a call before initialize: %+v", early)
	}
	hello := decode(t, server.request(t, "initialize", map[string]any{"protocolVersion": "2025-03-26"}))
	var info struct {
		Version string `json:"protocolVersion"`
		Server  struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	if json.Unmarshal(hello.Result, &info) != nil || info.Version != "2025-03-26" || info.Server.Name != "rewake" {
		t.Fatalf("initialize: %s", hello.Result)
	}
	if ping := decode(t, server.request(t, "ping", nil)); string(ping.Result) != "{}" {
		t.Fatalf("ping: %+v", ping)
	}
	var list struct {
		Tools []struct {
			Name   string `json:"name"`
			Schema struct {
				Required []string `json:"required"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	if json.Unmarshal(decode(t, server.request(t, "tools/list", nil)).Result, &list) != nil || len(list.Tools) != 1 || list.Tools[0].Name != "rewake" || list.Tools[0].Schema.Required[0] != "words" {
		t.Fatalf("tools/list: %+v", list)
	}
	for _, test := range []struct {
		method string
		params any
		code   int
	}{
		{"resources/list", nil, -32601},
		{"tools/call", map[string]any{"name": "other", "arguments": map[string]any{"words": []string{"whoami"}}}, -32602},
		{"tools/call", map[string]any{"name": "rewake", "arguments": map[string]any{"words": "whoami"}}, -32602},
		{"tools/call", map[string]any{"name": "rewake", "arguments": map[string]any{"words": []string{"whoami"}, "more": 1}}, -32602},
		{"tools/call", map[string]any{"name": "rewake", "arguments": map[string]any{}}, -32602},
	} {
		answer := decode(t, server.request(t, test.method, test.params))
		if answer.Error == nil || answer.Error.Code != test.code {
			t.Fatalf("%s %v: %+v", test.method, test.params, answer)
		}
	}
	// A notification gets no answer, whatever its method.
	server.write(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/unknown"})
	decode(t, server.request(t, "ping", nil))
	select {
	case line := <-server.stray:
		t.Fatalf("a notification was answered: %s", line)
	default:
	}
}

// What comes in is bounded: a frame over 256 KiB and an id over 128 bytes
// encoded, or of another kind, are refused with id null, run nothing, and the
// server goes on.
func TestTheServerBoundsWhatComesIn(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	server := r.start()
	server.writeRaw(t, []byte("not json"))
	if answer := server.strayLine(t); answer.Error == nil || answer.Error.Code != -32700 || string(answer.ID) != "null" {
		t.Fatalf("a frame that is no JSON: %+v", answer)
	}
	long := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"rewake","arguments":{"words":["send","--notify","web","` + strings.Repeat("x", 300<<10) + `"]}}}`
	server.writeRaw(t, []byte(long))
	if answer := server.strayLine(t); answer.Error == nil || answer.Error.Code != -32600 || string(answer.ID) != "null" {
		t.Fatalf("a frame over its bound: %+v", answer)
	}
	for _, id := range []string{`"` + strings.Repeat("i", 127) + `"`, `{"n":1}`, `[1]`, `1.5`} {
		server.writeRaw(t, []byte(`{"jsonrpc":"2.0","id":`+id+`,"method":"ping"}`))
		if answer := server.strayLine(t); answer.Error == nil || answer.Error.Code != -32600 || string(answer.ID) != "null" {
			t.Fatalf("the id %s: %+v", id, answer)
		}
	}
	server.writeRaw(t, []byte(`{"jsonrpc":"2.0","id":"`+strings.Repeat("i", 126)+`","method":"ping"}`))
	if answer := server.strayLine(t); answer.Error != nil {
		t.Fatalf("an id at its bound: %+v", answer)
	}
	if ping := decode(t, server.request(t, "ping", nil)); ping.Error != nil {
		t.Fatal("the server did not go on")
	}
	if headsUps(r) != 0 {
		t.Fatal("a frame over its bound ran")
	}
}

// Four calls run at once; a fifth is refused as busy, nothing run.
func TestAFifthCallIsBusy(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	server := r.start()
	r.nextTurn()
	// Calls the harness never reported wait out the observation's two
	// seconds, holding their slots.
	var waiting sync.WaitGroup
	for i := range 4 {
		waiting.Add(1)
		go func() {
			defer waiting.Done()
			server.call(t, []string{"whoami"}, r.meta("turn-1", "held-"+string(rune('a'+i))))
		}()
	}
	time.Sleep(300 * time.Millisecond)
	started := time.Now()
	result, _ := server.call(t, []string{"whoami"}, r.meta("turn-1", "fifth"))
	if !result.IsError || !strings.Contains(result.text(), "four tool calls are running") || time.Since(started) > time.Second {
		t.Fatalf("a fifth call: %+v", result)
	}
	waiting.Wait()
}

// A canceled call's answer is dropped; the server answers the next.
func TestACancelledCallIsNotAnswered(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	server := r.start()
	r.nextTurn()
	id, waiter := server.send(t, "tools/call", map[string]any{"name": "rewake", "arguments": map[string]any{"words": []string{"whoami"}}, "_meta": r.meta("turn-1", "canceled")})
	server.write(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/cancelled", //nolint:misspell // the protocol names the method so
		"params":  map[string]any{"requestId": id},
	})
	select {
	case line := <-waiter:
		t.Fatalf("a canceled call was answered: %s", line)
	case <-time.After(3 * time.Second):
	}
	if ping := decode(t, server.request(t, "ping", nil)); ping.Error != nil {
		t.Fatal("the server did not go on")
	}
}

// At the end of its stdin the server finishes the calls it took, answers
// them, and exits.
func TestTheServerFinishesItsCallsAtTheEnd(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	server := r.start()
	r.nextTurn()
	r.observe("turn-1", "last", []string{"whoami"})
	_, waiter := server.send(t, "tools/call", map[string]any{"name": "rewake", "arguments": map[string]any{"words": []string{"whoami"}}, "_meta": r.meta("turn-1", "last")})
	_ = server.in.Close()
	line, ok := server.wait(t, waiter)
	if !ok || !strings.Contains(string(line), "api") {
		t.Fatalf("the last call: %s", line)
	}
	select {
	case <-server.ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the server outlived its stdin")
	}
}
