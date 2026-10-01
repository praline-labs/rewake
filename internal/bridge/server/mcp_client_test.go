package server_test

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
)

// mcpClient is the harness's end of a server's stdio.
type mcpClient struct {
	command *exec.Cmd
	in      io.WriteCloser
	mu      sync.Mutex
	next    int
	answers map[string]chan json.RawMessage
	// stray are lines that answered no request: an error whose id the
	// server could not read.
	stray  chan json.RawMessage
	ended  chan struct{}
	closed sync.Once
}

func startServer(t *testing.T, env []string) *mcpClient {
	t.Helper()
	command := exec.Command(binary, "bridge-serve")
	command.Env = env
	in, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	c := &mcpClient{command: command, in: in, answers: map[string]chan json.RawMessage{}, stray: make(chan json.RawMessage, 16), ended: make(chan struct{})}
	go c.read(out)
	t.Cleanup(c.close)
	return c
}

func (c *mcpClient) read(out io.Reader) {
	defer close(c.ended)
	lines := bufio.NewScanner(out)
	lines.Buffer(nil, 4<<20)
	for lines.Scan() {
		line := append(json.RawMessage(nil), lines.Bytes()...)
		var head struct {
			ID json.RawMessage `json:"id"`
		}
		_ = json.Unmarshal(line, &head)
		c.mu.Lock()
		waiter, ok := c.answers[string(head.ID)]
		delete(c.answers, string(head.ID))
		c.mu.Unlock()
		if ok {
			waiter <- line
			continue
		}
		select {
		case c.stray <- line:
		default:
		}
	}
}

// send writes one message and returns where its answer arrives.
func (c *mcpClient) send(t *testing.T, method string, params any) (json.RawMessage, chan json.RawMessage) {
	t.Helper()
	c.mu.Lock()
	c.next++
	id := json.RawMessage(strconv.Itoa(c.next))
	waiter := make(chan json.RawMessage, 1)
	c.answers[string(id)] = waiter
	c.mu.Unlock()
	c.write(t, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	return id, waiter
}

func (c *mcpClient) write(t *testing.T, message any) {
	t.Helper()
	encoded, _ := json.Marshal(message)
	c.writeRaw(t, encoded)
}

func (c *mcpClient) writeRaw(t *testing.T, line []byte) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.in.Write(append(line, '\n')); err != nil {
		t.Logf("the server's stdin: %v", err)
	}
}

// wait reads an answer; false when the server ended first.
func (c *mcpClient) wait(t *testing.T, waiter chan json.RawMessage) (json.RawMessage, bool) {
	t.Helper()
	select {
	case line := <-waiter:
		return line, true
	case <-c.ended:
		select {
		case line := <-waiter:
			return line, true
		default:
			return nil, false
		}
	case <-time.After(45 * time.Second):
		t.Fatal("the server did not answer")
		return nil, false
	}
}

func (c *mcpClient) request(t *testing.T, method string, params any) json.RawMessage {
	t.Helper()
	_, waiter := c.send(t, method, params)
	line, ok := c.wait(t, waiter)
	if !ok {
		t.Fatalf("the server ended before it answered %s", method)
	}
	return line
}

func (c *mcpClient) initialize(t *testing.T) {
	t.Helper()
	c.request(t, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "rig", "version": "1"}})
	c.write(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
}

// callResult is a tools/call's result.
type callResult struct {
	Content []struct {
		Kind string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// text is the whole answer: stdout, stderr and the exit status in turn.
func (r callResult) text() string {
	var text strings.Builder
	for _, item := range r.Content {
		text.WriteString(item.Text)
	}
	return text.String()
}

// call makes a tools/call; ended says the server ended before it answered.
func (c *mcpClient) call(t *testing.T, words []string, meta map[string]any) (callResult, bool) {
	t.Helper()
	_, waiter := c.send(t, "tools/call", map[string]any{"name": "rewake", "arguments": map[string]any{"words": words}, "_meta": meta})
	line, ok := c.wait(t, waiter)
	if !ok {
		return callResult{}, true
	}
	var answer struct {
		Result *callResult      `json:"result"`
		Error  *json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(line, &answer); err != nil || answer.Result == nil {
		t.Fatalf("a tools/call answered with no result: %s", line)
	}
	if len(line) > bridge.ResultCap+1024 {
		t.Fatalf("a result of %d bytes", len(line))
	}
	return *answer.Result, false
}

// close ends the server's stdin and waits for it, as a harness that ends
// its tool server does.
func (c *mcpClient) close() {
	c.closed.Do(func() {
		_ = c.in.Close()
		done := make(chan struct{})
		go func() { _ = c.command.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(40 * time.Second):
			_ = c.command.Process.Kill()
			<-done
		}
	})
}
