package gateway

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func messageOfSize(size int) []byte {
	raw := bytes.Repeat([]byte("x"), size)
	copy(raw, `{"id":7,"result":{"padding":"`)
	copy(raw[len(raw)-3:], `"}}`)
	return raw
}

func sendFragment(conn net.Conn, op byte, final bool, raw []byte) error {
	if final {
		op |= 128
	}
	head := binary.BigEndian.AppendUint64([]byte{op, 127}, uint64(len(raw)))
	if _, err := conn.Write(head); err != nil {
		return err
	}
	_, err := io.Copy(conn, bytes.NewReader(raw))
	return err
}

func TestExactNativeMessageLimitAndNeighborBudget(t *testing.T) {
	raw := messageOfSize(maxMessage)
	for mode := range 3 {
		fragmented, masked := mode == 2, mode == 0
		a, b := net.Pipe()
		// Cover masked input and unmasked fragmented native output at the ceiling.
		receiver := testSocket(a, masked)
		writer := testSocket(b, !masked)
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		_ = a.SetDeadline(time.Now().Add(25 * time.Second))
		done := make(chan error, 1)
		go func() {
			if fragmented {
				if err := sendFragment(b, 1, false, raw[:len(raw)/2]); err != nil {
					done <- err
					return
				}
				done <- sendFragment(b, 0, true, raw[len(raw)/2:])
				return
			}
			done <- writer.writeFrameContext(ctx, 1, raw)
		}()
		got, err := receiver.readMessage()
		_ = a.Close()
		_ = b.Close()
		cancel()
		if err != nil || !bytes.Equal(got, raw) || cap(got) > maxMessage {
			t.Fatalf("fragmented=%v: read=%d capacity=%d err=%v", fragmented, len(got), cap(got), err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if projected, err := project(raw); err != nil || projected.id != "n:7" {
		t.Fatalf("maximum-size projection failed: %v", err)
	}
	c := &connection{work: make(chan func(), transportQueueItems), toUI: make(chan []byte, transportQueueItems)}
	neighbor := messageOfSize(16 << 20)
	if !c.queueRequest(raw, func() {}) || !c.queueResponse(raw) || !c.queueRequest(neighbor, func() {}) || !c.queueResponse(neighbor) {
		t.Fatal("maximum native message and neighboring service data must fit together")
	}
	if c.queueRequest([]byte{0}, func() {}) || c.queueResponse([]byte{0}) {
		t.Fatal("queue accepted one byte beyond its budget")
	}
	(<-c.work)()
	c.responseBytes.Add(-int64(cap(<-c.toUI)))
	if !c.queueRequest(raw, func() {}) || !c.queueResponse(raw) {
		t.Fatal("completed message did not release full retained capacity")
	}
}

func TestMessageLimitPlusOneRefusesBeforePayload(t *testing.T) {
	for _, fragmented := range []bool{false, true} {
		a, b := net.Pipe()
		_ = a.SetDeadline(time.Now().Add(15 * time.Second))
		done := make(chan error, 1)
		go func() {
			if fragmented {
				if err := sendFragment(b, 1, false, bytes.Repeat([]byte("x"), maxMessage)); err != nil {
					done <- err
					return
				}
				_, err := b.Write([]byte{0x80, 1})
				done <- err
				return
			}
			_, err := b.Write(binary.BigEndian.AppendUint64([]byte{0x81, 127}, maxMessage+1))
			done <- err
		}()
		_, err := testSocket(a, false).readMessage()
		_ = a.Close()
		_ = b.Close()
		wantStage := "read-frame"
		if fragmented {
			wantStage = "read-message"
		}
		assertSizeError(t, err, wantStage, maxMessage+1)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	raw := make([]byte, maxMessage+1)
	_, err := project(raw)
	assertSizeError(t, err, "projection-message", maxMessage+1)
	// A rejected write must not need a connection or send even its header.
	err = (&socketClient{}).writeFrameContext(context.Background(), 1, raw)
	assertSizeError(t, err, "write-message", maxMessage+1)
}

func assertSizeError(t *testing.T, err error, stage string, size uint64) {
	t.Helper()
	var limit *sizeError
	if !errors.As(err, &limit) || limit.Stage != stage || limit.Size != size || limit.Limit != maxMessage {
		t.Fatalf("wrong bounded refusal: %#v", err)
	}
}

func TestQueueAccountsFragmentCapacityAndOpaqueEscapes(t *testing.T) {
	c := &connection{work: make(chan func(), 4), toUI: make(chan []byte, 4)}
	raw := make([]byte, 1, 1<<20)
	if !c.queueRequest(raw, func() {}) || !c.queueResponse(raw) || c.requestBytes.Load() != 1<<20 || c.responseBytes.Load() != 1<<20 {
		t.Fatal("retained spare capacity escaped queue accounting")
	}
	for _, value := range []string{`""`, `"a\\"`, `"a\"b"`, `"a\\\"b"`, `"a\\\\"`} {
		input := []byte(value + `,"later"`)
		if end := skipValue(input, 0); end != len(value) {
			t.Fatalf("escaped quote scanned past value: end=%d length=%d", end, len(value))
		}
	}
}

func TestOversizedHeaderDiagnosticContainsOnlySize(t *testing.T) {
	closed := make(chan CloseInfo, 1)
	_, _, peers, _ := setupConfig(t, Config{Closed: func(info CloseInfo) { closed <- info }})
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	if _, err := server.conn.Write(binary.BigEndian.AppendUint64([]byte{0x81, 127}, maxMessage+1)); err != nil {
		t.Fatal(err)
	}
	select {
	case info := <-closed:
		if info.Direction != "server-to-tui" || info.SizeStage != "read-frame" || info.MessageBytes != maxMessage+1 || info.LimitBytes != maxMessage {
			t.Fatalf("missing size diagnostic: %+v", info)
		}
	case <-time.After(time.Second):
		t.Fatal("no oversized-header diagnostic")
	}
}
