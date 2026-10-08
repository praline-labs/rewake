package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

type rpcReply struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("app-server (%d): %s", e.Code, e.Message) }

type rpcClient struct {
	socket  *socketClient
	next    atomic.Uint64
	mu      sync.Mutex
	pending map[uint64]chan rpcReply
	done    chan struct{}
	err     error
	onEvent func(string, json.RawMessage)
}

func connectRPC(ctx context.Context, path string, onEvent func(string, json.RawMessage)) (*rpcClient, error) {
	return dialRPC(ctx, path, onEvent, nil)
}

// dialRPC connects and initializes, keeping initialize's answer in
// answered when it is not nil: the startup probe reads the server's
// version from it.
func dialRPC(ctx context.Context, path string, onEvent func(string, json.RawMessage), answered *json.RawMessage) (*rpcClient, error) {
	socket, err := dialSocket(ctx, path)
	if err != nil {
		return nil, err
	}
	client := &rpcClient{socket: socket, pending: make(map[uint64]chan rpcReply), done: make(chan struct{}), onEvent: onEvent}
	go client.read()
	params := map[string]any{"clientInfo": map[string]string{"name": "rewake", "version": "1"}, "capabilities": map[string]bool{"experimentalApi": true}}
	var result any
	if answered != nil {
		result = answered
	}
	if err := client.call(ctx, "initialize", params, result); err != nil {
		client.close()
		return nil, err
	}
	if err := client.send(ctx, map[string]string{"method": "initialized"}); err != nil {
		client.close()
		return nil, err
	}
	return client, nil
}

func (c *rpcClient) send(ctx context.Context, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.socket.writeFrameContext(ctx, 1, raw)
}

func (c *rpcClient) call(ctx context.Context, method string, params, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := c.next.Add(1)
	reply := make(chan rpcReply, 1)
	c.mu.Lock()
	c.pending[id] = reply
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	if err := c.send(ctx, map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		select {
		case value := <-reply:
			return rpcResult(value, result)
		default:
		}
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		return err
	case value := <-reply:
		return rpcResult(value, result)
	}
}

func rpcResult(value rpcReply, result any) error {
	if value.Error != nil {
		return value.Error
	}
	if result != nil {
		return json.Unmarshal(value.Result, result)
	}
	return nil
}

func (c *rpcClient) read() {
	var terminal error
	defer func() { c.mu.Lock(); c.err = terminal; c.mu.Unlock(); close(c.done) }()
	for {
		raw, err := c.socket.readMessage()
		if err != nil {
			terminal = fmt.Errorf("app-server connection lost: %w", err)
			return
		}
		var reply rpcReply
		if err := json.Unmarshal(raw, &reply); err != nil {
			terminal = fmt.Errorf("invalid app-server JSON: %w", err)
			return
		}
		if reply.Method != "" {
			if c.onEvent != nil {
				c.onEvent(reply.Method, reply.Params)
			}
			continue
		}
		c.mu.Lock()
		var id uint64
		_ = json.Unmarshal(reply.ID, &id)
		waiting := c.pending[id]
		c.mu.Unlock()
		if waiting != nil {
			select {
			case waiting <- reply:
			default:
			}
		}
	}
}

func (c *rpcClient) close() {
	_ = c.socket.writeFrame(8, []byte{3, 232})
	_ = c.socket.conn.Close()
	<-c.done
}
