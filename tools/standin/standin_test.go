package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const toolName = "mcp__rewake__rewake"

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec
}

func reqBody(stream bool, tools []string, messages string) string {
	ts := []map[string]string{}
	for _, n := range tools {
		ts = append(ts, map[string]string{"name": n})
	}
	b, _ := json.Marshal(map[string]any{"model": "m", "stream": stream, "tools": ts})
	return strings.TrimSuffix(string(b), "}") + `,"messages":` + messages + "}"
}

const userText = `[{"role":"user","content":"hello secret-text"}]`

func resultMsgs(ids ...string) string {
	s := `[{"role":"user","content":[{"type":"text","text":"hi"}]}`
	for _, id := range ids {
		s += `,{"role":"assistant","content":[{"type":"tool_use","id":"` + id + `","name":"x","input":{}}]}`
		s += `,{"role":"user","content":[{"type":"tool_result","tool_use_id":"` + id +
			`","content":[{"type":"text","text":"line one\nline two"}]},{"type":"text","text":"reminder"}]}`
	}
	return s + "]"
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return m
}

func firstBlock(m map[string]any) map[string]any {
	return m["content"].([]any)[0].(map[string]any)
}

func TestToolCall(t *testing.T) {
	h := NewHandler(Options{Words: []string{"send", "bob"}})
	m := decode(t, post(t, h, "/v1/messages?beta=true", reqBody(false, []string{"Bash", toolName}, userText)))
	b := firstBlock(m)
	if m["stop_reason"] != "tool_use" || b["type"] != "tool_use" || b["name"] != toolName {
		t.Fatalf("got %v", m)
	}
	if b["id"] != "toolu_standin_1" {
		t.Fatalf("id %v", b["id"])
	}
	in := b["input"].(map[string]any)["words"].([]any)
	if len(in) != 2 || in[0] != "send" || in[1] != "bob" {
		t.Fatalf("words %v", in)
	}
	if m["usage"].(map[string]any)["input_tokens"] == nil {
		t.Fatal("no usage")
	}
	m = decode(t, post(t, h, "/v1/messages", reqBody(false, []string{toolName}, userText)))
	if firstBlock(m)["id"] != "toolu_standin_2" {
		t.Fatalf("ids do not count up: %v", m)
	}
}

func TestToolResultAnswersText(t *testing.T) {
	h := NewHandler(Options{})
	m := decode(t, post(t, h, "/v1/messages", reqBody(false, []string{toolName}, resultMsgs("toolu_standin_1"))))
	if m["stop_reason"] != "end_turn" || firstBlock(m)["text"] != "tool result received: line one line two" {
		t.Fatalf("got %v", m)
	}
	long := strings.Repeat("x", 500)
	msgs := `[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_standin_1","content":"` + long + `"}]}]`
	m = decode(t, post(t, h, "/v1/messages", reqBody(false, nil, msgs)))
	if got := firstBlock(m)["text"]; got != "tool result received: "+long[:200] {
		t.Fatalf("not cut at 200 bytes: %v", got)
	}
}

func TestForeignToolResultIgnored(t *testing.T) {
	h := NewHandler(Options{})
	m := decode(t, post(t, h, "/v1/messages", reqBody(false, []string{toolName}, resultMsgs("toolu_other"))))
	if m["stop_reason"] != "tool_use" {
		t.Fatalf("got %v", m)
	}
}

func TestNotOffered(t *testing.T) {
	h := NewHandler(Options{})
	m := decode(t, post(t, h, "/v1/messages", reqBody(false, []string{"Bash"}, userText)))
	if m["stop_reason"] != "end_turn" || firstBlock(m)["text"] != "the tool is not offered" {
		t.Fatalf("got %v", m)
	}
}

func TestCustomToolName(t *testing.T) {
	h := NewHandler(Options{Tool: "other"})
	m := decode(t, post(t, h, "/v1/messages", reqBody(false, []string{"other"}, userText)))
	if firstBlock(m)["name"] != "other" {
		t.Fatalf("got %v", m)
	}
}

func TestMultiCall(t *testing.T) {
	h := NewHandler(Options{Calls: 2})
	m := decode(t, post(t, h, "/v1/messages", reqBody(false, []string{toolName}, userText)))
	if m["stop_reason"] != "tool_use" {
		t.Fatalf("first: %v", m)
	}
	m = decode(t, post(t, h, "/v1/messages", reqBody(false, []string{toolName}, resultMsgs("toolu_standin_1"))))
	if m["stop_reason"] != "tool_use" {
		t.Fatalf("second call missing: %v", m)
	}
	m = decode(t, post(t, h, "/v1/messages", reqBody(false, []string{toolName}, resultMsgs("toolu_standin_1", "toolu_standin_2"))))
	if m["stop_reason"] != "end_turn" {
		t.Fatalf("third should be text: %v", m)
	}
	// A new plain user text starts a new turn: the old results no longer count.
	again := strings.TrimSuffix(resultMsgs("toolu_standin_1", "toolu_standin_2"), "]") +
		`,{"role":"assistant","content":"done"},{"role":"user","content":"again"}]`
	m = decode(t, post(t, h, "/v1/messages", reqBody(false, []string{toolName}, again)))
	if m["stop_reason"] != "tool_use" {
		t.Fatalf("new turn: %v", m)
	}
}

type event struct {
	name string
	data map[string]any
}

func events(t *testing.T, rec *httptest.ResponseRecorder) []event {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	var out []event
	var cur event
	sc := bufio.NewScanner(rec.Body)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			cur = event{name: strings.TrimPrefix(line, "event: ")}
		case strings.HasPrefix(line, "data: "):
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &cur.data); err != nil {
				t.Fatal(err)
			}
			if cur.data["type"] != cur.name {
				t.Fatalf("event %q carries type %v", cur.name, cur.data["type"])
			}
			out = append(out, cur)
		}
	}
	return out
}

func names(evs []event) string {
	var n []string
	for _, e := range evs {
		n = append(n, e.name)
	}
	return strings.Join(n, ",")
}

const shape = "message_start,content_block_start,content_block_delta,content_block_stop,message_delta,message_stop"

func TestStreamToolCall(t *testing.T) {
	h := NewHandler(Options{Words: []string{"a", "b"}})
	evs := events(t, post(t, h, "/v1/messages", reqBody(true, []string{toolName}, userText)))
	if names(evs) != shape {
		t.Fatalf("shape %s", names(evs))
	}
	start := evs[1].data["content_block"].(map[string]any)
	if start["type"] != "tool_use" || start["name"] != toolName || start["id"] != "toolu_standin_1" {
		t.Fatalf("start %v", start)
	}
	d := evs[2].data["delta"].(map[string]any)
	if d["type"] != "input_json_delta" || d["partial_json"] != `{"words":["a","b"]}` {
		t.Fatalf("delta %v", d)
	}
	if evs[4].data["delta"].(map[string]any)["stop_reason"] != "tool_use" || evs[4].data["usage"] == nil {
		t.Fatalf("message_delta %v", evs[4].data)
	}
}

func TestStreamText(t *testing.T) {
	h := NewHandler(Options{})
	evs := events(t, post(t, h, "/v1/messages", reqBody(true, []string{toolName}, resultMsgs("toolu_standin_1"))))
	if names(evs) != shape {
		t.Fatalf("shape %s", names(evs))
	}
	d := evs[2].data["delta"].(map[string]any)
	if d["type"] != "text_delta" || d["text"] != "tool result received: line one line two" {
		t.Fatalf("delta %v", d)
	}
	if evs[4].data["delta"].(map[string]any)["stop_reason"] != "end_turn" {
		t.Fatalf("message_delta %v", evs[4].data)
	}
}

func TestOtherPaths(t *testing.T) {
	h := NewHandler(Options{})
	rec := post(t, h, "/v1/messages/count_tokens", `{"messages":[]}`)
	if rec.Code != 200 || decode(t, rec)["input_tokens"] != float64(10) {
		t.Fatalf("count_tokens: %d %s", rec.Code, rec.Body)
	}
	for _, c := range []struct{ method, path string }{
		{"GET", "/v1/messages"}, {"POST", "/nope"}, {"GET", "/"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != 404 || decode(t, rec)["type"] != "error" {
			t.Fatalf("%s %s: %d %s", c.method, c.path, rec.Code, rec.Body)
		}
	}
	if rec := post(t, h, "/v1/messages", "{not json"); rec.Code != 400 {
		t.Fatalf("bad json: %d", rec.Code)
	}
}

func TestLogHoldsNoText(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(Options{Log: &buf})
	post(t, h, "/v1/messages", reqBody(false, []string{"Bash", toolName}, userText))
	post(t, h, "/v1/messages", reqBody(false, []string{toolName}, resultMsgs("toolu_standin_1")))
	post(t, h, "/v1/messages/count_tokens", `{}`)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines: %q", buf.String())
	}
	for _, bad := range []string{"secret-text", "line one", "reminder", "hello"} {
		if strings.Contains(buf.String(), bad) {
			t.Fatalf("log leaks %q: %s", bad, buf.String())
		}
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if first["path"] != "/v1/messages" || first["tools_offered"] != true ||
		first["answered"] != "tool_use:"+toolName || len(first["tool_names"].([]any)) != 2 {
		t.Fatalf("line %v", first)
	}
}
