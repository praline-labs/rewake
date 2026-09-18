package gateway

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestTerminalReadBoundarySurvivesDelayedACK(t *testing.T) {
	var reads atomic.Uint64
	reads.Store(1)
	out := make(chan Completion, 2)
	g, ui, peers, _ := setupConfig(t, Config{ReadSequence: reads.Load, Complete: func(c Completion) { out <- c }})
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	write(t, ui, []byte(`{"id":2,"method":"turn/start","params":{"threadId":"A"}}`))
	_ = readWithin(t, native)
	unknownRead(t, ui, native)
	for _, raw := range []string{
		`{"method":"turn/started","params":{"threadId":"A","turn":{"id":"T"}}}`,
		`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"T","status":"completed"}}}`,
	} {
		write(t, native, []byte(raw))
		_ = readWithin(t, ui)
	}
	reads.Store(2)
	write(t, native, []byte(`{"id":2,"result":{"turn":{"id":"T"}}}`))
	_ = readWithin(t, ui)
	select {
	case result := <-out:
		if result.ReadThrough == nil || *result.ReadThrough != 1 {
			t.Fatalf("late ACK resampled reads: %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("completion lost")
	}
}

func TestGapReadBoundaryComesFromIdleNotGraceExpiry(t *testing.T) {
	var reads atomic.Uint64
	reads.Store(1)
	out := make(chan Completion, 2)
	g, ui, peers, _ := setupConfig(t, Config{ReadSequence: reads.Load, Complete: func(c Completion) { out <- c }})
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	for _, raw := range []string{
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"active"}}}`,
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`,
	} {
		write(t, native, []byte(raw))
		_ = readWithin(t, ui)
	}
	reads.Store(2)
	select {
	case result := <-out:
		if result.ReadThrough == nil || *result.ReadThrough != 1 {
			t.Fatalf("grace delay resampled reads: %+v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("gap lost")
	}
}
