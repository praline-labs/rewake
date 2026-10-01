package server_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/receipt"
)

// A client that stops reading the server's stdout
// (docs/mail-bridge-server.md#running-the-child): its stdout is a pipe of one
// page, open and not drained. Protocol replies fill it ahead of the end of
// stdin, or a call's answer waits on it with stdin ending before the write
// or during it. Whatever the client does with stdout, the server ends within
// outputWait of the end of its stdin, or of its input backing up behind the
// stuck output; an answer that never left is no read, and the letter shows
// to a later call. While stdin is open and nothing backs up, a stuck answer
// only waits, and lands whole once the client reads.

// outputWait is the server's own; the test allows it a second and a half.
const outputWait = 2 * time.Second

func TestAClientThatStopsReading(t *testing.T) {
	t.Parallel()
	for _, leg := range []string{
		"replies fill it, then stdin ends",
		"replies fill it, stdin open",
		"an answer waits, stdin ends before its write",
		"an answer waits, stdin ends during its write",
		"an answer waits, stdin open",
	} {
		t.Run(leg, func(t *testing.T) {
			t.Parallel()
			if strings.HasPrefix(leg, "replies") {
				repliesFillIt(t, strings.HasSuffix(leg, "stdin ends"))
				return
			}
			anAnswerWaits(t, strings.TrimPrefix(leg, "an answer waits, "))
		})
	}
}

func repliesFillIt(t *testing.T, closes bool) {
	r := newRig(t, bridge.CodexTransport)
	u := startUndrained(t, r)
	for i := 1; i <= 256; i++ {
		u.write(t, map[string]any{"jsonrpc": "2.0", "id": i, "method": "ping"})
	}
	since := time.Now()
	if closes {
		u.closeStdin(t)
	}
	u.endsWithin(t, since, outputWait+1500*time.Millisecond)
}

// The fill replies are pings with ids that make each reply 64 bytes: 63 of
// them leave 64 bytes of the page, less than any answer.
const fillReplies = 63

func anAnswerWaits(t *testing.T, eof string) {
	r := newRig(t, bridge.CodexTransport)
	h := newHold(t)
	t.Cleanup(h.free)
	r.fault = h.specAt("server", "answer")
	u := startUndrained(t, r)
	id := r.letter("a task from web")
	u.fill(t)
	c := toolCall{id: "held-answer", turn: r.nextTurn(), words: []string{"inbox"}}
	r.observe(c.turn, c.id, c.words)
	u.write(t, map[string]any{"jsonrpc": "2.0", "id": "the-call", "method": "tools/call", "params": map[string]any{
		"name": "rewake", "arguments": map[string]any{"words": c.words}, "_meta": r.meta(c.turn, c.id),
	}})
	h.reached(t)
	if eof == "stdin ends before its write" {
		u.closeStdin(t)
	}
	h.release(t)
	since := time.Now()
	if eof == "stdin ends during its write" {
		time.Sleep(200 * time.Millisecond)
		since = time.Now()
		u.closeStdin(t)
	}
	if eof == "stdin open" {
		select {
		case err := <-u.done:
			t.Fatalf("the server ended with stdin open and nothing backed up: %v", err)
		case <-time.After(outputWait + time.Second):
		}
		lines := u.readLines(t, fillReplies+1)
		if last := lines[fillReplies]; !strings.Contains(last, `"the-call"`) || !strings.Contains(last, "a task from web") {
			t.Fatalf("the answer that waited: %s", last)
		}
		since = time.Now()
		u.closeStdin(t)
		u.endsWithin(t, since, 1500*time.Millisecond)
		return
	}
	u.endsWithin(t, since, outputWait+1500*time.Millisecond)
	// Nothing of the answer left the server: the page holds the fill
	// replies alone, and the letter is unread.
	if lines := u.readLines(t, -1); len(lines) != fillReplies {
		t.Fatalf("the client got %d lines, the last %s", len(lines), lines[len(lines)-1])
	}
	if !r.unread(id) {
		t.Fatal("an answer that never left read the letter")
	}
	r.fault = ""
	r.start()
	again := r.call("inbox")
	if again.ended || again.result.IsError || !strings.Contains(again.result.text(), "a task from web") {
		t.Fatalf("a later call: %+v", again.result)
	}
	if err := r.complete(again, true); err != nil || r.unread(id) {
		t.Fatalf("the later call's read: %v, unread %v", err, r.unread(id))
	}
}

// A client that reads again within outputWait gets every reply, in order,
// with stdin open or ended; with stdin open the server goes on serving.
func TestAReaderThatResumesInTimeGetsEveryReply(t *testing.T) {
	t.Parallel()
	for _, eof := range []string{"stdin open", "stdin ended"} {
		t.Run(eof, func(t *testing.T) {
			t.Parallel()
			r := newRig(t, bridge.CodexTransport)
			u := startUndrained(t, r)
			for i := 1; i <= 128; i++ {
				u.write(t, map[string]any{"jsonrpc": "2.0", "id": i, "method": "ping"})
			}
			if eof == "stdin ended" {
				u.closeStdin(t)
			}
			time.Sleep(outputWait / 2)
			for i, line := range u.readLines(t, 128) {
				var reply struct {
					ID int `json:"id"`
				}
				if err := json.Unmarshal([]byte(line), &reply); err != nil || reply.ID != i+1 {
					t.Fatalf("reply %d once output resumed: %s (%v)", i, line, err)
				}
			}
			if eof == "stdin open" {
				u.write(t, map[string]any{"jsonrpc": "2.0", "id": 129, "method": "ping"})
				if line := u.readLines(t, 1)[0]; !strings.Contains(line, `"id":129`) {
					t.Fatalf("the server did not go on: %s", line)
				}
				u.closeStdin(t)
			}
			u.endsWithin(t, time.Now(), time.Second)
		})
	}
}

// An effect whose answer never left is found again, not repeated: a send
// that published its heads-up, its answer stuck at the end of stdin, is
// joined by the same words in the same turn and by its retry on a new
// server, and the heads-up stays one.
func TestALostAnswerOfAnEffectIsFoundAgain(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	h := newHold(t)
	t.Cleanup(h.free)
	r.fault = h.specAt("server", "answer")
	u := startUndrained(t, r)
	u.fill(t)
	words := []string{"send", "--notify", "--wait", "0", "web", faultHeadsUp}
	c := toolCall{id: "published-without-answer", turn: r.nextTurn(), words: words}
	r.observe(c.turn, c.id, words)
	u.write(t, map[string]any{"jsonrpc": "2.0", "id": "send-call", "method": "tools/call", "params": map[string]any{
		"name": "rewake", "arguments": map[string]any{"words": words}, "_meta": r.meta(c.turn, c.id),
	}})
	h.reached(t)
	if n := headsUps(r); n != 1 {
		t.Fatalf("the send published %d heads-ups", n)
	}
	token, err := receipt.Bound(r.dir, "api", r.self.Epoch(), bridge.CallKey(r.transport, "conversation", c.id))
	if err != nil {
		t.Fatal(err)
	}
	u.closeStdin(t)
	h.release(t)
	u.endsWithin(t, time.Now(), outputWait+1500*time.Millisecond)
	if lines := u.readLines(t, -1); len(lines) != fillReplies {
		t.Fatalf("the client got %d lines", len(lines))
	}
	r.fault = ""
	r.start()
	for _, again := range [][]string{words, {"retry", token}} {
		next := r.call(again...)
		if next.ended || !strings.Contains(next.result.text(), "pending for web") {
			t.Fatalf("%v answered: %s", again, next.result.text())
		}
		if n := headsUps(r); n != 1 {
			t.Fatalf("%v left %d heads-ups", again, n)
		}
	}
}

// undrained is a server whose stdout the test reads only when it chooses.
type undrained struct {
	in    io.WriteCloser
	out   *os.File
	lines *bufio.Reader
	done  chan error
}

func startUndrained(t *testing.T, r *rig) *undrained {
	t.Helper()
	command := exec.Command(binary, "bridge-serve")
	command.Env = r.env()
	in, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = read.Close() })
	if size, _, errno := syscall.Syscall(syscall.SYS_FCNTL, write.Fd(), syscall.F_SETPIPE_SZ, 4096); errno != 0 || size != 4096 {
		t.Fatalf("the pipe's size: %d, %v", size, errno)
	}
	command.Stdout, command.Stderr = write, os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	_ = write.Close()
	u := &undrained{in: in, out: read, lines: bufio.NewReader(read), done: make(chan error, 1)}
	go func() { u.done <- command.Wait() }()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = read.Close()
	})
	u.write(t, map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18"}})
	u.readLines(t, 1)
	u.write(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return u
}

func (u *undrained) write(t *testing.T, message any) {
	t.Helper()
	line, _ := json.Marshal(message)
	if _, err := u.in.Write(append(line, '\n')); err != nil {
		t.Fatalf("the server's stdin: %v", err)
	}
}

func (u *undrained) closeStdin(t *testing.T) {
	t.Helper()
	if err := u.in.Close(); err != nil {
		t.Fatal(err)
	}
}

// buffered is how many bytes wait in the pipe.
func (u *undrained) buffered(t *testing.T) int {
	t.Helper()
	var n int32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.out.Fd(), syscall.TIOCINQ, uintptr(unsafe.Pointer(&n))); errno != 0 {
		t.Fatal(errno)
	}
	return int(n) + u.lines.Buffered()
}

// readLines reads n lines, or all there are to the end when n is negative.
func (u *undrained) readLines(t *testing.T, n int) []string {
	t.Helper()
	var lines []string
	for n < 0 || len(lines) < n {
		line, err := u.lines.ReadString('\n')
		if err != nil {
			if n < 0 && line == "" {
				return lines
			}
			t.Fatalf("the server's stdout after %d lines: %v", len(lines), err)
		}
		lines = append(lines, line)
	}
	return lines
}

// endsWithin waits for the server to exit, 0, within bound of since.
func (u *undrained) endsWithin(t *testing.T, since time.Time, bound time.Duration) {
	t.Helper()
	select {
	case err := <-u.done:
		if err != nil {
			t.Fatalf("the server exited: %v", err)
		}
		if took := time.Since(since); took > bound {
			t.Fatalf("the server ended %s after, past %s", took, bound)
		}
	case <-time.After(bound + 10*time.Second):
		t.Fatalf("the server was alive %s after, past %s", time.Since(since), bound)
	}
}

// fill leaves 64 bytes of the page free: fillReplies pings whose replies
// take 64 bytes each.
func (u *undrained) fill(t *testing.T) {
	t.Helper()
	for i := range fillReplies {
		u.write(t, map[string]any{"jsonrpc": "2.0", "id": fmt.Sprintf("fill-%021d", i), "method": "ping"})
	}
	for until := time.Now().Add(5 * time.Second); u.buffered(t) != fillReplies*64; time.Sleep(time.Millisecond) {
		if time.Now().After(until) {
			t.Fatalf("the fill replies take %d bytes", u.buffered(t))
		}
	}
}
