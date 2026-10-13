package terminal

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

// recorder keeps each write it is given, so a test sees the boundaries the
// program's side would.
type recorder struct {
	mu     sync.Mutex
	writes []string
}

func (r *recorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes = append(r.writes, string(p))
	return len(p), nil
}

// Every post of keys is one write, whole, and posts sent at once never
// interleave: ESC O and C written apart read as Alt+O and a stray C.
func TestEachPostOfKeysIsOneWrite(t *testing.T) {
	term, err := Open(Size{Cols: 10, Rows: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = term.Close() }()
	var r recorder
	term.writeMu.Lock()
	term.out = &r
	term.writeMu.Unlock()
	if err := term.Keys([]byte("\x1bOC")); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.writes, []string{"\x1bOC"}) {
		t.Fatalf("one key: %q", r.writes)
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
	got := slices.Clone(r.writes[1:])
	slices.Sort(got)
	slices.Sort(posts)
	if !slices.Equal(got, posts) {
		t.Fatalf("posts at once were written as %q", r.writes[1:])
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
