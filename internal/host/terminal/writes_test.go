package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"
)

// syscalls stands in for write(2) on a terminal: it keeps the bytes of each
// call, and answers it as answer says, or takes the whole call.
type syscalls struct {
	mu     sync.Mutex
	calls  []string
	answer func(call int, p []byte) (int, error)
}

func (s *syscalls) write(_ int, p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, string(p))
	if s.answer != nil {
		return s.answer(len(s.calls), p)
	}
	return len(p), nil
}

// opened is a terminal with no program whose write(2) is s.
func opened(t *testing.T, s *syscalls) *Terminal {
	t.Helper()
	term, err := Open(Size{Cols: 10, Rows: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = term.Close() })
	term.writeMu.Lock()
	term.write = s.write
	term.writeMu.Unlock()
	return term
}

// Every post of keys is one write(2), whole, and posts sent at once never
// interleave: ESC O and C written apart read as Alt+O and a stray C.
func TestEachPostOfKeysIsOneWrite(t *testing.T) {
	var s syscalls
	term := opened(t, &s)
	if err := term.Keys([]byte("\x1bOC")); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.calls, []string{"\x1bOC"}) {
		t.Fatalf("one key: %q", s.calls)
	}
	var posts []string
	var wg sync.WaitGroup
	for i := range 16 {
		post := fmt.Sprintf("\x1b[%d;5~text%d\r", i, i)
		posts = append(posts, post)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := term.Keys([]byte(post)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got := slices.Clone(s.calls[1:])
	slices.Sort(got)
	slices.Sort(posts)
	if !slices.Equal(got, posts) {
		t.Fatalf("posts at once were written as %q", s.calls[1:])
	}
}

// A write(2) that takes part of a post is the only one: the rest is never
// written after it, and the caller learns how much went.
func TestAShortWriteIsSaidAndNotFinished(t *testing.T) {
	s := syscalls{answer: func(int, []byte) (int, error) { return 3, nil }}
	term := opened(t, &s)
	err := term.Keys([]byte("hello world"))
	var short *ShortWrite
	if !errors.As(err, &short) || *short != (ShortWrite{Written: 3, Of: 11}) {
		t.Fatalf("a short write answered %v", err)
	}
	if !slices.Equal(s.calls, []string{"hello world"}) {
		t.Fatalf("the rest was written after a short write: %q", s.calls)
	}
}

// A write(2) that would block, or is interrupted, before taking a byte is
// made again with the whole post; one that fails otherwise is not. The call
// that would block writes to the terminal's side, so the poller sees the
// terminal again as a full buffer draining would make it.
func TestAWriteThatTookNothingIsMadeAgain(t *testing.T) {
	var term *Terminal
	s := syscalls{answer: func(call int, p []byte) (int, error) {
		switch call {
		case 1:
			if _, err := term.Side().Write([]byte("x")); err != nil {
				t.Error(err)
			}
			return -1, syscall.EAGAIN
		case 2:
			return -1, syscall.EINTR
		}
		return len(p), nil
	}}
	term = opened(t, &s)
	if err := term.Keys([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.calls, []string{"abc", "abc", "abc"}) {
		t.Fatalf("the calls: %q", s.calls)
	}
	failing := syscalls{answer: func(int, []byte) (int, error) { return -1, syscall.EIO }}
	term = opened(t, &failing)
	if err := term.Keys([]byte("abc")); !errors.Is(err, syscall.EIO) || len(failing.calls) != 1 {
		t.Fatalf("a failed write: %v after %d calls", err, len(failing.calls))
	}
}

// A post over the bound is refused with nothing written.
func TestAPostOverTheBoundIsRefused(t *testing.T) {
	var s syscalls
	term := opened(t, &s)
	if err := term.Keys(make([]byte, maxKeys)); err != nil {
		t.Fatalf("a post at the bound: %v", err)
	}
	if err := term.Keys(make([]byte, maxKeys+1)); err == nil {
		t.Fatal("a post over the bound was taken")
	}
	if len(s.calls) != 1 {
		t.Fatalf("%d writes, want the one at the bound", len(s.calls))
	}
}

// Resizes, reads of the screen and keys at once: the race detector holds
// the locking, and the screen settles at the last size with every row
// covering it.
func TestResizesRaceNothing(t *testing.T) {
	term, _ := started(t, "size", Size{Cols: 20, Rows: 4})
	var g grid
	until(t, term, &g, showing("size 20x4"))
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := range 60 {
			if err := term.Resize(Size{Cols: 10 + i%50, Rows: 2 + i%9}); err != nil {
				t.Error(err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		var seq uint64
		for ctx.Err() == nil {
			read, stop := context.WithTimeout(ctx, 10*time.Millisecond)
			s := term.Screen(read, seq)
			stop()
			for _, row := range s.Changed {
				if colsOf(row.Runs) != s.Size.Cols {
					t.Errorf("a row of %d columns on a screen of %d", colsOf(row.Runs), s.Size.Cols)
				}
			}
			seq = s.Seq
		}
	}()
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
			_ = term.Keys([]byte("k"))
			time.Sleep(time.Millisecond)
		}
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()
	if err := term.Resize(Size{Cols: 33, Rows: 7}); err != nil {
		t.Fatal(err)
	}
	until(t, term, &g, showing("size 33x7"))
	for i, row := range g.rows {
		if colsOf(row) != 33 {
			t.Errorf("row %d covers %d columns after the last resize", i, colsOf(row))
		}
	}
}
