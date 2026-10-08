package server

import (
	"encoding/json"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
)

// encoder is the one writer of the server's stdout: every message goes
// through send, which checks the whole JSON-RPC message against
// bridge.ResultCap before a byte is written. A test fails when any other code
// reaches the server's stdout.
type encoder struct {
	mu  sync.Mutex
	out io.Writer

	// waiting counts the messages taken and not yet written, and moved is
	// when the output last moved: a message written, or one taken while
	// none waited. A client that stops reading stops it.
	state   sync.Mutex
	waiting int
	moved   time.Time
}

func newEncoder(out io.Writer) *encoder { return &encoder{out: out} }

// message is a JSON-RPC message the server writes.
type message struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// The JSON-RPC error codes the server answers with.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeNoMethod       = -32601
	codeInvalidParams  = -32602
)

// result is a tool result.
type result struct {
	Content []item `json:"content"`
	IsError bool   `json:"isError"`
}

type item struct {
	Kind string `json:"type"`
	Text string `json:"text"`
}

// answer is what a call answers: a child's output, or the server's own line
// in place of one, which is always an error.
type answer struct {
	stdout, stderr string
	code           int
	// substituted says no child's answer is in it.
	substituted bool
}

func (a answer) result() result {
	out := result{Content: []item{{Kind: "text", Text: a.stdout}}, IsError: a.substituted || a.code != 0}
	if a.stderr != "" {
		out.Content = append(out.Content, item{Kind: "text", Text: a.stderr})
	}
	if a.code != 0 {
		out.Content = append(out.Content, item{Kind: "text", Text: "exit status " + strconv.Itoa(a.code)})
	}
	return out
}

// substitute is the server's own answer: one line, an error.
func substitute(line string) answer {
	return answer{stdout: line, substituted: true}
}

// overBound replaces an answer that would not fit. It comes only from a child
// that broke its own bound, and matches no record.
const overBound = "Rewake: the answer of this call did not fit one tool result; run the same words in the shell.\n"

// reply writes a call's answer, or what replaces it whole when it does not fit.
func (e *encoder) reply(id json.RawMessage, a answer) {
	if !e.send(message{Version: "2.0", ID: id, Result: a.result()}) {
		e.send(message{Version: "2.0", ID: id, Result: substitute(overBound).result()})
	}
}

// fail writes a JSON-RPC error.
func (e *encoder) fail(id json.RawMessage, code int, text string) {
	if id == nil {
		id = json.RawMessage("null")
	}
	e.send(message{Version: "2.0", ID: id, Error: &rpcError{Code: code, Message: text}})
}

// send writes one message if it fits, and says whether it did.
func (e *encoder) send(value message) bool {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > bridge.ResultCap {
		return false
	}
	e.state.Lock()
	if e.waiting == 0 {
		e.moved = time.Now()
	}
	e.waiting++
	e.state.Unlock()
	defer func() {
		e.state.Lock()
		e.waiting--
		e.moved = time.Now()
		e.state.Unlock()
	}()
	e.mu.Lock()
	defer e.mu.Unlock()
	_, _ = e.out.Write(append(encoded, '\n'))
	return true
}

// stuck says whether a message has waited for wait with nothing written
// meanwhile. A message is at most the result cap, so a client that reads at
// all takes one off well within it.
func (e *encoder) stuck(wait time.Duration) bool {
	e.state.Lock()
	defer e.state.Unlock()
	return e.waiting > 0 && time.Since(e.moved) >= wait
}
