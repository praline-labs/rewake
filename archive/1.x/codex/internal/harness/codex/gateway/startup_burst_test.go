package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartupRequestBurstPreservesOrderingAndApprovalBypass(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	if err := g.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	locked := true
	defer func() {
		if locked {
			<-g.gate
		}
	}()
	var requests [][]byte
	for i := 0; i < 56; i++ {
		raw := []byte(fmt.Sprintf(`{"id":%d,"method":"config/read","params":{"includeLayers":true}}`, i+10))
		requests = append(requests, raw)
		write(t, ui, raw)
	}
	// The native reader must pass this response even with an entire startup burst
	// parked behind injected admission. Blocking on a full FIFO would deadlock it.
	approval := []byte(`{"id":"approval","result":{"decision":"accept"}}`)
	write(t, ui, approval)
	if got := readWithin(t, server); !bytes.Equal(got, approval) {
		t.Fatalf("approval did not bypass startup: %s", got)
	}
	<-g.gate
	locked = false
	for _, want := range requests {
		if got := readWithin(t, server); !bytes.Equal(got, want) {
			t.Fatal("startup ordering or bytes changed")
		}
	}
	if !g.Binding().Ready {
		t.Fatal("startup burst lost primary binding")
	}
}

func TestStartupResponseBurstSurvivesBusyTUIWriter(t *testing.T) {
	var received atomic.Int64
	g, ui, peers, _ := setupConfig(t, Config{Record: func(r Record) {
		if r.Direction == "server-message" && r.Method == "fixture/startup" {
			received.Add(1)
		}
	}})
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	c := g.currentConnection()
	c.down.writeGate <- struct{}{}
	locked := true
	defer func() {
		if locked {
			<-c.down.writeGate
		}
	}()
	var responses [][]byte
	for i := 0; i < 56; i++ {
		raw := []byte(fmt.Sprintf(`{"method":"fixture/startup","params":{"index":%d}}`, i))
		responses = append(responses, raw)
		write(t, server, raw)
	}
	deadline := time.Now().Add(time.Second)
	for received.Load() != 56 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if received.Load() != 56 || c.ctx.Err() != nil {
		t.Fatal("startup burst blocked reader or disconnected native TUI")
	}
	<-c.down.writeGate
	locked = false
	for _, want := range responses {
		if got := readWithin(t, ui); !bytes.Equal(got, want) {
			t.Fatal("startup response ordering or bytes changed")
		}
	}
}

func TestTransportQueuesBoundBytesAndItems(t *testing.T) {
	for _, response := range []bool{false, true} {
		c := &connection{work: make(chan func(), transportQueueItems), toUI: make(chan []byte, transportQueueItems)}
		queue := func(raw []byte) bool {
			if response {
				return c.queueResponse(raw)
			}
			return c.queueRequest(raw, func() {})
		}
		block := bytes.Repeat([]byte("x"), 1<<20)
		for i := 0; i < transportQueueBytes/len(block); i++ {
			if !queue(block) {
				t.Fatal("byte budget refused early")
			}
		}
		if queue([]byte("x")) {
			t.Fatal("byte budget exceeded")
		}
		if response {
			c.responseBytes.Add(-int64(cap(<-c.toUI)))
		} else {
			(<-c.work)()
		}
		if !queue(block) {
			t.Fatal("processed frame did not release byte budget")
		}
	}
	c := &connection{work: make(chan func(), transportQueueItems), toUI: make(chan []byte, transportQueueItems)}
	for i := 0; i < transportQueueItems; i++ {
		if !c.queueRequest(nil, func() {}) || !c.queueResponse(nil) {
			t.Fatal("item budget refused early")
		}
	}
	if c.queueRequest(nil, func() {}) || c.queueResponse(nil) {
		t.Fatal("item budget exceeded")
	}
}

func TestCloseDiagnosticRedactsValuesAndReportsFirstCause(t *testing.T) {
	closed := make(chan CloseInfo, 2)
	_, ui, peers, _ := setupConfig(t, Config{Closed: func(info CloseInfo) { closed <- info }})
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	write(t, ui, []byte(`{"id":"one","id":"SECRET","method":"thread/start"}`))
	select {
	case info := <-closed:
		if info.Direction != "tui-to-server" || info.Reason != "projection" || info.Error != "duplicate routing field" || strings.Contains(fmt.Sprint(info), "SECRET") {
			t.Fatalf("unsafe or missing close cause: %+v", info)
		}
	case <-time.After(time.Second):
		t.Fatal("close cause missing")
	}
	if got := closeError(errors.New("SECRET /private/path")); got != "transport-or-control-error" {
		t.Fatal("unknown error contents retained")
	}
	select {
	case <-closed:
		t.Fatal("duplicate close diagnostic")
	default:
	}
}
