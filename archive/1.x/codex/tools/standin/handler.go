package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	idPrefix     = "toolu_standin_"
	maxBody      = 64 << 20
	quoteLimit   = 200
	notOffered   = "the tool is not offered"
	stopToolUse  = "tool_use"
	stopEndTurn  = "end_turn"
	standinModel = "standin"
)

// Options configure the handler; the zero value of a field takes the default.
type Options struct {
	Tool  string        // tool to call, default mcp__rewake__rewake
	Words []string      // input words, default ["whoami"]
	Calls int           // tool calls per turn, default 1
	Delay time.Duration // wait before each answer
	Log   io.Writer     // one JSON line per request; nil for none
}

type handler struct {
	opts Options
	next atomic.Int64
	mu   sync.Mutex // serializes log lines
}

// NewHandler returns the stand-in's HTTP handler.
func NewHandler(opts Options) http.Handler {
	if opts.Tool == "" {
		opts.Tool = "mcp__rewake__rewake"
	}
	if len(opts.Words) == 0 {
		opts.Words = []string{"whoami"}
	}
	if opts.Calls < 1 {
		opts.Calls = 1
	}
	return &handler{opts: opts}
}

type block struct {
	Kind      string          `json:"type"`
	Text      string          `json:"text"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

type message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type request struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Tools    []struct {
		Name string `json:"name"`
	} `json:"tools"`
	Stream bool `json:"stream"`
}

// blocks reads a content field that is either a string or a block list.
func blocks(raw json.RawMessage) []block {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []block{{Kind: "text", Text: s}}
	}
	var bs []block
	_ = json.Unmarshal(raw, &bs)
	return bs
}

// resultText flattens a tool_result's content to its text.
func resultText(raw json.RawMessage) string {
	var parts []string
	for _, b := range blocks(raw) {
		if b.Kind == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, " ")
}

// scan returns how many tool results of ours the turn holds, counted since the
// last user message that carries no tool_result (the plain user text), and the
// text of the newest one, which is what the final reply quotes.
func scan(msgs []message) (n int, last string) {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "user" {
			continue
		}
		own := false
		for _, b := range blocks(msgs[i].Content) {
			if b.Kind != "tool_result" {
				continue
			}
			own = true
			if strings.HasPrefix(b.ToolUseID, idPrefix) {
				if n == 0 {
					last = resultText(b.Content)
				}
				n++
			}
		}
		if !own {
			break
		}
	}
	return n, last
}

func quote(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > quoteLimit {
		s = s[:quoteLimit]
	}
	return strings.ToValidUTF8(s, "")
}

// answer is what one request is answered with: a text reply or one tool call.
type answer struct {
	text   string
	toolID string
	tool   string
	input  string // JSON of the tool input
}

func (a answer) stop() string {
	if a.tool != "" {
		return stopToolUse
	}
	return stopEndTurn
}

func (a answer) kind() string {
	if a.tool != "" {
		return "tool_use:" + a.tool
	}
	return "text"
}

func (h *handler) script(req request) answer {
	offered := false
	for _, t := range req.Tools {
		if t.Name == h.opts.Tool {
			offered = true
		}
	}
	n, last := scan(req.Messages)
	switch {
	case n > 0 && (n >= h.opts.Calls || !offered):
		return answer{text: "tool result received: " + quote(last)}
	case offered:
		in, _ := json.Marshal(map[string][]string{"words": h.opts.Words})
		return answer{
			tool:   h.opts.Tool,
			toolID: fmt.Sprintf("%s%d", idPrefix, h.next.Add(1)),
			input:  string(in),
		}
	}
	return answer{text: notOffered}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/v1/messages/count_tokens" && r.Method == http.MethodPost:
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, maxBody))
		writeJSON(w, http.StatusOK, map[string]int{"input_tokens": 10})
		h.log(r.URL.Path, nil, "count_tokens")
	case r.URL.Path == "/v1/messages" && r.Method == http.MethodPost:
		h.messages(w, r)
	default:
		writeJSON(w, http.StatusNotFound, errorBody("not_found_error", "no such endpoint"))
		h.log(r.URL.Path, nil, "404")
	}
}

func (h *handler) messages(w http.ResponseWriter, r *http.Request) {
	var req request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid_request_error", "body is not valid JSON"))
		h.log(r.URL.Path, nil, "400")
		return
	}
	names := make([]string, 0, len(req.Tools))
	for _, t := range req.Tools {
		names = append(names, t.Name)
	}
	a := h.script(req)
	if h.opts.Delay > 0 {
		select {
		case <-time.After(h.opts.Delay):
		case <-r.Context().Done():
			return
		}
	}
	model := req.Model
	if model == "" {
		model = standinModel
	}
	if req.Stream {
		writeStream(w, model, a)
	} else {
		writeJSON(w, http.StatusOK, messageBody(model, a))
	}
	h.log(r.URL.Path, names, a.kind())
}

func errorBody(kind, msg string) map[string]any {
	return map[string]any{"type": "error", "error": map[string]string{"type": kind, "message": msg}}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *handler) log(path string, names []string, answered string) {
	if h.opts.Log == nil {
		return
	}
	if names == nil {
		names = []string{}
	}
	line, _ := json.Marshal(map[string]any{
		"time":          time.Now().UTC().Format(time.RFC3339Nano),
		"path":          path,
		"tools_offered": len(names) > 0,
		"tool_names":    names,
		"answered":      answered,
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	_, _ = h.opts.Log.Write(append(line, '\n'))
}
