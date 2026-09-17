package codex

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func serverMessage(c net.Conn, value any) {
	data, _ := json.Marshal(value)
	frame := []byte{0x81}
	if len(data) < 126 {
		frame = append(frame, byte(len(data)))
	} else {
		frame = append(frame, 126)
		frame = binary.BigEndian.AppendUint16(frame, uint16(len(data)))
	}
	_, _ = c.Write(append(frame, data...))
}

func TestRPCCorrelatesRepliesAndKeepsServerErrors(t *testing.T) {
	path := socketFixture(t, func(c net.Conn, r *bufio.Reader) {
		_, _, _, _ = readClientFrame(r)
		serverMessage(c, map[string]any{"id": 1, "result": map[string]any{}})
		_, _, _, _ = readClientFrame(r)
		requests := make([]map[string]any, 2)
		for i := range requests {
			_, data, _, _ := readClientFrame(r)
			_ = json.Unmarshal(data, &requests[i])
		}
		serverMessage(c, map[string]any{"method": "thread/started", "params": map[string]string{"id": "new-thread"}})
		for i := 1; i >= 0; i-- {
			request := requests[i]
			if request["method"] == "refuse" {
				serverMessage(c, map[string]any{"id": request["id"], "error": map[string]any{"code": -32600, "message": "original refusal"}})
			} else {
				serverMessage(c, map[string]any{"id": request["id"], "result": map[string]string{"value": "accepted"}})
			}
		}
		_, _, _, _ = readClientFrame(r)
	})
	event := make(chan string, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := connectRPC(ctx, path, func(method string, _ json.RawMessage) { event <- method })
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		var result struct{ Value string }
		if err := client.call(ctx, "accept", struct{}{}, &result); err != nil || result.Value != "accepted" {
			t.Errorf("reply=%+v err=%v", result, err)
		}
	}()
	go func() {
		defer wg.Done()
		err := client.call(ctx, "refuse", struct{}{}, nil)
		var refusal *rpcError
		if !errors.As(err, &refusal) || refusal.Message != "original refusal" {
			t.Errorf("lost refusal: %v", err)
		}
	}()
	wg.Wait()
	if got := <-event; got != "thread/started" {
		t.Fatal(got)
	}
}

func TestRPCRequestIsBoundedWhenServerDoesNotReply(t *testing.T) {
	path := socketFixture(t, func(c net.Conn, r *bufio.Reader) {
		_, _, _, _ = readClientFrame(r)
		serverMessage(c, map[string]any{"id": 1, "result": map[string]any{}})
		_, _, _, _ = readClientFrame(r)
		_, _, _, _ = readClientFrame(r)
		_, _, _, _ = readClientFrame(r)
	})
	client, err := connectRPC(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := client.call(ctx, "blocked", struct{}{}, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unbounded request: %v", err)
	}
}
